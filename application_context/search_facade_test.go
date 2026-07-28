package application_context

import (
	"strings"
	"sync"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Tests for the transaction-free but very much principal-dependent semantics of
// global search. They live in package application_context because they need
// WithPrincipal and the callbacks registerScopeCallbacks installs.
//
// Search has two pieces of shared, process-lifetime state that a naive
// extraction would silently duplicate per call: the result cache and the FTS
// provider/enabled flags. Duplicating either is quiet and damaging — a per-call
// cache never hits and never invalidates, and a per-call FTS flag reads false,
// downgrading every search to the LIKE fallback. Neither shows up as an error.

func searchNames(t *testing.T, ctx *MahresourcesContext, term string) string {
	t.Helper()
	resp, err := ctx.GlobalSearch(&query_models.GlobalSearchQuery{Query: term})
	if err != nil {
		t.Fatalf("GlobalSearch(%q): %v", term, err)
	}
	var b strings.Builder
	for _, r := range resp.Results {
		b.WriteString(r.Name)
		b.WriteByte(' ')
	}
	return b.String()
}

// Search runs its per-type queries on ctx.db, so the GORM scope callbacks are
// what confine a group-limited principal. A search service holding a handle
// captured at construction would query unscoped and return another tenant's
// rows.
func TestSearchFacade_ObservesDerivedHandleNotSingleton(t *testing.T) {
	ctx := createFacadeContext(t, "search_handle")

	root := &models.Group{Name: "shx-root"}
	mustCreateFacade(t, ctx, root)
	inside := &models.Group{Name: "shx-inside", OwnerId: &root.ID}
	mustCreateFacade(t, ctx, inside)
	outside := &models.Group{Name: "shx-outside-secret"}
	mustCreateFacade(t, ctx, outside)

	// Control: unscoped search reaches all three, so the absences below are
	// enforcement rather than an empty result set.
	all := searchNames(t, ctx, "shx")
	for _, want := range []string{"shx-root", "shx-inside", "shx-outside-secret"} {
		if !strings.Contains(all, want) {
			t.Fatalf("control invalid: unscoped search did not find %q, got: %s", want, all)
		}
	}

	got := searchNames(t, ctx.WithPrincipal(scopedGuest(root.ID)), "shx")
	if strings.Contains(got, "shx-outside-secret") {
		t.Errorf("search through a principal-derived context returned an out-of-subtree group; "+
			"the service did not observe the derived handle, got: %s", got)
	}
	if !strings.Contains(got, "shx-root") || !strings.Contains(got, "shx-inside") {
		t.Errorf("scoped search lost in-subtree results, got: %s", got)
	}
}

// The result cache must be one shared object, not one per call or per derived
// context. Asserted behaviourally so it survives the extraction: a stale read
// proves the cache is live, and the read after invalidation proves the
// invalidation reached the same cache the reader consults.
//
// This matters beyond tidiness: InvalidateSearchCacheByType has 31 call sites
// across 12 files in this package, every one of them a write path relying on
// the next search seeing fresh data.
func TestSearchFacade_CacheIsSharedAndInvalidatable(t *testing.T) {
	ctx := createFacadeContext(t, "search_cache_shared")

	tag := &models.Tag{Name: "shq-cached-tag"}
	mustCreateFacade(t, ctx, tag)

	if got := searchNames(t, ctx, "shq"); !strings.Contains(got, "shq-cached-tag") {
		t.Fatalf("control invalid: the tag was not searchable to begin with, got: %s", got)
	}

	// Rename behind the cache's back, bypassing every invalidation call site.
	if err := ctx.db.Model(&models.Tag{}).Where("id = ?", tag.ID).
		Update("name", "shq-renamed").Error; err != nil {
		t.Fatalf("rename tag: %v", err)
	}

	// Control: the stale name is still served, which is what proves the first
	// search populated a cache that the second one reads. Without this, the
	// assertion after invalidation would pass against no cache at all.
	if got := searchNames(t, ctx, "shq"); !strings.Contains(got, "shq-cached-tag") {
		t.Fatalf("control invalid: the search cache did not serve a stale result, so there is no "+
			"cache for the invalidation below to prove anything about, got: %s", got)
	}

	// Invalidate through a DERIVED context. It must reach the same cache the
	// singleton reads, or every write path in this package silently stops
	// invalidating anything.
	ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleEditor}).
		InvalidateSearchCacheByType(EntityTypeTag)

	got := searchNames(t, ctx, "shq")
	if strings.Contains(got, "shq-cached-tag") {
		t.Errorf("the stale name survived invalidation issued through a derived context; the cache "+
			"is not shared, got: %s", got)
	}
	if !strings.Contains(got, "shq-renamed") {
		t.Errorf("expected the renamed tag after invalidation, got: %s", got)
	}
}

// InitFTS mutates provider/enabled state on the context at startup. Derived
// contexts created afterwards must observe it, or every request-scoped search
// silently downgrades to the LIKE fallback.
func TestSearchFacade_FTSStateVisibleToDerivedContexts(t *testing.T) {
	ctx := createFacadeContext(t, "search_fts_state")

	if err := ctx.InitFTS(); err != nil {
		t.Skipf("InitFTS unavailable in this build: %v", err)
	}
	if !ctx.ftsAvailable() {
		t.Fatal("control invalid: InitFTS reported success but FTS is not enabled")
	}

	root := &models.Group{Name: "shf-root"}
	mustCreateFacade(t, ctx, root)

	derived := ctx.WithPrincipal(scopedGuest(root.ID))
	if !derived.ftsAvailable() {
		t.Error("a context derived after InitFTS reports FTS disabled; request-scoped searches would " +
			"all fall back to LIKE")
	}
}

// Concurrent InitFTS against in-flight searches. GlobalSearch fans out over one
// goroutine per entity type and each consults FTS state, so once the state is
// shared by a single Service rather than copied into every derived context,
// these reads race any write.
//
// Production only writes at startup, but "only at startup" is a convention, not
// a guarantee, and the failure mode is a torn read of provider/enabled rather
// than an error. Run under -race this test fails against plain struct fields and
// passes against the atomic snapshot.
func TestSearchFacade_FTSStateIsRaceFree(t *testing.T) {
	ctx := createFacadeContext(t, "search_fts_race")

	root := &models.Group{Name: "shr-root"}
	mustCreateFacade(t, ctx, root)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchNames(t, ctx, "shr")
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ctx.InitFTS()
		}()
	}
	wg.Wait()
}
