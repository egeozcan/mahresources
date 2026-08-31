package application_context

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// uip is a test helper for the *uint ExpectedCount field.
func uip(v uint) *uint { return &v }

// Every fixture in this file builds its own uniquely named in-memory SQLite
// database (createAdminTestContext takes a cache name), so no test depends on
// rows another test made. The counter also uniquifies row names across runs,
// because the shared-cache DSN keeps the database alive within one process and
// -count=2 would otherwise collide on unique names.

var massEditFixtureSeq atomic.Uint64

func massEditName(name string) string {
	return fmt.Sprintf("%s-%d", name, massEditFixtureSeq.Add(1))
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

type massEditFixture struct {
	ctx   *MahresourcesContext
	tag   *models.Tag
	tag2  *models.Tag
	owner *models.Group
	res   []*models.Resource
}

// newMassEditFixture builds a context plus two tags, an owner group and n
// unowned resources, all named after the test.
func newMassEditFixture(t *testing.T, name string, n int) *massEditFixture {
	t.Helper()
	ctx := createAdminTestContext(t, "mass_edit_"+massEditName(name))

	tag := &models.Tag{Name: name + "-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	tag2 := &models.Tag{Name: name + "-tag2"}
	if err := ctx.db.Create(tag2).Error; err != nil {
		t.Fatalf("create second tag: %v", err)
	}
	owner := &models.Group{Name: name + "-owner"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}

	resources := make([]*models.Resource, 0, n)
	for i := 0; i < n; i++ {
		r := &models.Resource{Name: fmt.Sprintf("%s-res-%d", name, i)}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create resource %d: %v", i, err)
		}
		resources = append(resources, r)
	}
	return &massEditFixture{ctx: ctx, tag: tag, tag2: tag2, owner: owner, res: resources}
}

func addTagTo(ctx *MahresourcesContext, resource *models.Resource, tag *models.Tag) {
	_ = ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", resource.ID, tag.ID)
}

func (f *massEditFixture) ids() []uint {
	ids := make([]uint, 0, len(f.res))
	for _, r := range f.res {
		ids = append(ids, r.ID)
	}
	return ids
}

func resTagIDs(t *testing.T, ctx *MahresourcesContext, id uint) []uint {
	t.Helper()
	var ids []uint
	if err := ctx.db.Raw("SELECT tag_id FROM resource_tags WHERE resource_id = ? ORDER BY tag_id", id).Scan(&ids).Error; err != nil {
		t.Fatalf("read tags: %v", err)
	}
	return ids
}

func countJoinRows(t *testing.T, ctx *MahresourcesContext, table, idCol string, id uint) int64 {
	t.Helper()
	var n int64
	if err := ctx.db.Raw("SELECT COUNT(*) FROM "+table+" WHERE "+idCol+" = ?", id).Scan(&n).Error; err != nil {
		t.Fatalf("count join rows in %s: %v", table, err)
	}
	return n
}

// rawJSONColumn reads a raw JSON column, mapping SQL NULL to "".
func rawJSONColumn(t *testing.T, ctx *MahresourcesContext, column string, id uint) string {
	t.Helper()
	var raw sql.NullString
	if err := ctx.db.Raw("SELECT "+column+" FROM resources WHERE id = ?", id).Scan(&raw).Error; err != nil {
		t.Fatalf("read resources.%s: %v", column, err)
	}
	if !raw.Valid {
		return ""
	}
	return raw.String
}

func massResourceMeta(t *testing.T, ctx *MahresourcesContext, id uint) map[string]any {
	t.Helper()
	out := map[string]any{}
	blob := rawJSONColumn(t, ctx, "meta", id)
	if len(blob) > 0 && blob != "null" {
		if err := json.Unmarshal([]byte(blob), &out); err != nil {
			t.Fatalf("unmarshal meta %s: %v", blob, err)
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func massResourceOwnMeta(t *testing.T, ctx *MahresourcesContext, id uint) map[string]any {
	t.Helper()
	blob := rawJSONColumn(t, ctx, "own_meta", id)
	out := map[string]any{}
	if len(blob) > 0 && blob != "null" {
		_ = json.Unmarshal([]byte(blob), &out)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// jsonMapOrEmpty decodes a JSON object column, mapping SQL NULL to {}.
func jsonMapOrEmpty(t *testing.T, blob []byte) map[string]any {
	t.Helper()
	out := map[string]any{}
	if len(blob) > 0 && string(blob) != "null" {
		if err := json.Unmarshal(blob, &out); err != nil {
			t.Fatalf("unmarshal meta %s: %v", blob, err)
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func setResourceMetaRaw(t *testing.T, ctx *MahresourcesContext, id uint, meta, ownMeta string) {
	t.Helper()
	if err := ctx.db.Exec("UPDATE resources SET meta = ?, own_meta = ? WHERE id = ?", meta, ownMeta, id).Error; err != nil {
		t.Fatalf("set raw meta: %v", err)
	}
}

func scopedMassEditContext(ctx *MahresourcesContext, scopeGroup *models.Group) *MahresourcesContext {
	return ctx.WithPrincipal(&auth.Principal{
		UserID:       1,
		Username:     "confined-editor",
		Role:         models.RoleUser,
		ScopeGroupID: &scopeGroup.ID,
	})
}

// ---------------------------------------------------------------------------
// Targeting
// ---------------------------------------------------------------------------

func TestMassEditIDsWithDuplicatesEditsEachOnce(t *testing.T) {
	f := newMassEditFixture(t, "dups", 3)

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.res[0].ID, f.res[0].ID, f.res[1].ID},
		TagsOp: "add",
		TagIds: []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	if result.Affected != 2 {
		t.Errorf("affected=%d, want 2 (each entity edited once)", result.Affected)
	}
	if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", f.res[0].ID); got != 1 {
		t.Errorf("resource 0 has %d tag rows, want 1", got)
	}
	if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", f.res[2].ID); got != 0 {
		t.Errorf("resource 2 was not named but has %d tag rows", got)
	}
}

func TestMassEditFilterModeEditsBeyondTheFirstPage(t *testing.T) {
	f := newMassEditFixture(t, "pages", 1)
	stray := f.res[0]

	const total = 250
	for i := 0; i < total; i++ {
		r := &models.Resource{Name: fmt.Sprintf("pagefilter-%d", i)}
		if err := f.ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create resource %d: %v", i, err)
		}
	}

	origPluckPage, origChunk := massEditPluckPage, massEditChunkSize
	massEditPluckPage = 100 // forces three keyset pages
	massEditChunkSize = 64
	t.Cleanup(func() {
		massEditPluckPage = origPluckPage
		massEditChunkSize = origChunk
	})

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=pagefilter-",
		ExpectedCount: uip(total),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("filter mass edit: %v", err)
	}
	if result.Matched != total || result.Affected != total {
		t.Fatalf("matched=%d affected=%d, want %d", result.Matched, result.Affected, total)
	}

	var tagged int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM resource_tags WHERE tag_id = ?", f.tag.ID).Scan(&tagged).Error; err != nil {
		t.Fatalf("count tagged: %v", err)
	}
	if tagged != total {
		t.Errorf("tagged %d resources, want %d — filter mode must edit beyond the first page", tagged, total)
	}
	if got := len(resTagIDs(t, f.ctx, stray.ID)); got != 0 {
		t.Error("an entity outside the filter was edited")
	}
}

func TestMassEditFilterMRQLResolvesAndFailsClosed(t *testing.T) {
	f := newMassEditFixture(t, "mrql", 4)
	for i, r := range f.res[:4] {
		if err := f.ctx.db.Model(r).Update("name", fmt.Sprintf("mrqled-%d", i)).Error; err != nil {
			t.Fatalf("rename: %v", err)
		}
	}

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        `mrql=name ~ "mrqled-"`,
		ExpectedCount: uip(4),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("mass edit with a valid MRQL filter: %v", err)
	}
	if result.Matched != 4 {
		t.Errorf("matched=%d, want 4", result.Matched)
	}

	// A malformed expression fails; it never falls through to the unfiltered set.
	_, err = f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        `mrql=name ~~~~ "broken"`,
		ExpectedCount: uip(4),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	var mfe *MRQLFilterError
	if err == nil {
		t.Fatal("a malformed MRQL filter was accepted")
	}
	if !errors.As(err, &mfe) {
		t.Fatalf("expected *MRQLFilterError, got %T: %v", err, err)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 1 {
			t.Error("a refused mass edit changed the database")
		}
	}
}

func TestMassEditFilterMetaQueryNarrows(t *testing.T) {
	f := newMassEditFixture(t, "metaq", 4)
	for i, r := range f.res {
		meta := `{"size":100}`
		if i == 3 {
			meta = `{"size":5000}`
		}
		if err := f.ctx.db.Model(r).Update("meta", meta).Error; err != nil {
			t.Fatalf("set meta: %v", err)
		}
	}

	dry, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "MetaQuery=size:GT:1000",
		ExpectedCount: uip(1),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Matched != 1 {
		t.Errorf("matched=%d, want 1 — the MetaQuery inside the filter string was dropped and the set widened", dry.Matched)
	}
}

func TestMassEditFilterIgnoresMaxResults(t *testing.T) {
	f := newMassEditFixture(t, "maxres", 5)
	for _, r := range f.res {
		if err := f.ctx.db.Model(r).Update("name", "maxres-mass").Error; err != nil {
			t.Fatalf("rename: %v", err)
		}
	}

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=maxres-mass&maxResults=2",
		ExpectedCount: uip(5),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	if result.Matched != 5 {
		t.Errorf("matched=%d, want 5 — maxResults is pagination, not predicate", result.Matched)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 1 {
			t.Errorf("resource %d not tagged", r.ID)
		}
	}
}

func TestMassEditExpectedCountMismatchIsAConflict(t *testing.T) {
	f := newMassEditFixture(t, "mismatch", 2)

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=mismatch-res",
		ExpectedCount: uip(99),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err == nil || err != ErrMassEditSetChanged {
		t.Fatalf("expected ErrMassEditSetChanged, got %v", err)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Error("a refused mass edit wrote to the database")
		}
	}
}

func TestMassEditOverTheCapIsRefusedNotTruncated(t *testing.T) {
	f := newMassEditFixture(t, "cap", 4)
	f.ctx.Config.MaxMassEditEntities = 3

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=cap-res",
		ExpectedCount: uip(4),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("expected the ceiling error naming the cap, got %v", err)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Error("a refused mass edit wrote to the database")
		}
	}
}

func TestMassEditRequiresATarget(t *testing.T) {
	f := newMassEditFixture(t, "notarget", 1)

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{TagsOp: "add", TagIds: []uint{f.tag.ID}})
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\", got %v", err)
	}
}

func TestMassEditDryRunCommitsNothing(t *testing.T) {
	f := newMassEditFixture(t, "dryrun", 2)

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:      f.ids(),
		TagsOp:  "add",
		TagIds:  []uint{f.tag.ID},
		OwnerOp: "set",
		OwnerId: f.owner.ID,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !result.DryRun || result.Matched != 2 || result.Affected != 2 {
		t.Fatalf("unexpected dry-run result %+v", result)
	}
	found := map[string]bool{}
	for _, op := range result.Ops {
		found[op.Op] = true
	}
	if !found["tags.add"] || !found["owner.set"] {
		t.Errorf("dry-run ops missing: %+v", result.Ops)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Error("a dry run wrote to the database")
		}
		var fresh models.Resource
		if err := f.ctx.db.Select("owner_id").First(&fresh, r.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if fresh.OwnerId != nil {
			t.Error("a dry run changed the owner")
		}
	}
}

func TestMassEditRequiresAtLeastOneOp(t *testing.T) {
	f := newMassEditFixture(t, "noops", 1)

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{ID: f.ids()})
	if err == nil || !strings.Contains(err.Error(), "at least one operation") {
		t.Fatalf("expected \"at least one operation\", got %v", err)
	}
}

func TestMassEditRefusesUnknownVerbs(t *testing.T) {
	f := newMassEditFixture(t, "badverb", 1)

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID: f.ids(), TagsOp: "replce", TagIds: []uint{f.tag.ID}, // typo
	})
	if err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("expected a verb refusal, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RBAC
// ---------------------------------------------------------------------------

type massScopedFixture struct {
	ctx          *MahresourcesContext
	scoped       *MahresourcesContext
	scopeGroup   *models.Group
	outsideGroup *models.Group
	tag          *models.Tag
	inside       []*models.Resource
	outside      []*models.Resource
}

func newMassScopedFixture(t *testing.T, name string, inside, outside int) *massScopedFixture {
	t.Helper()
	ctx := createAdminTestContext(t, "mass_edit_scoped_"+massEditName(name))

	scopeGroup := &models.Group{Name: name + "-scope"}
	if err := ctx.db.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	outGroup := &models.Group{Name: name + "-outside"}
	if err := ctx.db.Create(outGroup).Error; err != nil {
		t.Fatalf("create outside group: %v", err)
	}
	tag := &models.Tag{Name: name + "-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	f := &massScopedFixture{ctx: ctx, scopeGroup: scopeGroup, outsideGroup: outGroup, tag: tag}
	for i := 0; i < inside; i++ {
		r := &models.Resource{Name: fmt.Sprintf("%s-in-%d", name, i), OwnerId: &scopeGroup.ID}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create inside resource: %v", err)
		}
		f.inside = append(f.inside, r)
	}
	for i := 0; i < outside; i++ {
		r := &models.Resource{Name: fmt.Sprintf("%s-out-%d", name, i), OwnerId: &outGroup.ID}
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create outside resource: %v", err)
		}
		f.outside = append(f.outside, r)
	}
	f.scoped = scopedMassEditContext(ctx, scopeGroup)
	return f
}

func TestMassEditScopedFilterModeEditsOnlyItsSubtree(t *testing.T) {
	f := newMassScopedFixture(t, "scopedfilter", 2, 2)

	// All four share one name, so the filter matches in and out of the subtree
	// alike. ExpectedCount 2 is what the scoped reader sees, not the world's 4.
	if err := f.ctx.db.Model(&models.Resource{}).Where("1 = 1").Update("name", "scope-shared").Error; err != nil {
		t.Fatalf("rename: %v", err)
	}

	result, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=scope-shared",
		ExpectedCount: uip(2),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("scoped filter mass edit: %v", err)
	}
	if result.Matched != 2 || result.Affected != 2 {
		t.Errorf("matched=%d affected=%d, want 2/2 — the scoped count, not the world's", result.Matched, result.Affected)
	}
	for _, r := range f.outside {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Error("an out-of-subtree resource was edited")
		}
	}
	for _, r := range f.inside {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 1 {
			t.Errorf("inside resource %d was not edited (%d tag rows)", r.ID, got)
		}
	}
}

func TestMassEditPluckIsScoped(t *testing.T) {
	// The Pluck→Scan regression test. Scan does not run the Query callback
	// chain, so a "rewrite Pluck as Scan for speed" change hands a
	// group-limited principal the whole database; only this shape catches it.
	f := newMassScopedFixture(t, "pluck", 2, 2)

	if err := f.ctx.db.Model(&models.Resource{}).Where("1 = 1").Update("name", "pluck-shared").Error; err != nil {
		t.Fatalf("rename: %v", err)
	}

	ids, err := f.scoped.pluckMassEditTargets(massEditResourceSpec, "name=pluck-shared")
	if err != nil {
		t.Fatalf("pluck: %v", err)
	}
	insideIDs := map[uint]bool{}
	for _, r := range f.inside {
		insideIDs[r.ID] = true
	}
	if len(ids) != len(f.inside) {
		t.Fatalf("resolved %d ids, want %d (only the subtree)", len(ids), len(f.inside))
	}
	for _, id := range ids {
		if !insideIDs[id] {
			t.Fatalf("the target set contains out-of-subtree resource %d — the pluck lost its scope predicate", id)
		}
	}
}

func TestMassEditNamedOutOfSubtreeIDRefusedAndInSubtreeUnchanged(t *testing.T) {
	f := newMassScopedFixture(t, "named", 2, 1)

	_, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.inside[0].ID, f.inside[1].ID, f.outside[0].ID},
		TagsOp: "add",
		TagIds: []uint{f.tag.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "one or more resources not found") {
		t.Fatalf("expected \"one or more resources not found\", got %v", err)
	}
	// The in-subtree ids in the same request are unchanged: the whole edit is
	// refused, not partially applied.
	for _, r := range f.inside {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Errorf("in-subtree resource %d was changed by a refused request", r.ID)
		}
	}
}

func TestMassEditFarEndpointOutsideSubtreeIsRefused(t *testing.T) {
	f := newMassScopedFixture(t, "fargroup", 2, 0)

	_, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.inside[0].ID},
		GroupsOp: "add",
		GroupIds: []uint{f.outsideGroup.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "one or more groups not found") {
		t.Fatalf("expected far-endpoint refusal, got %v", err)
	}
	if got := countJoinRows(t, f.ctx, "groups_related_resources", "resource_id", f.inside[0].ID); got != 0 {
		t.Error("a refused relation op wrote a join row")
	}
}

func TestMassEditTagsAreGlobalForScopedPrincipals(t *testing.T) {
	f := newMassScopedFixture(t, "globaltag", 1, 0)

	result, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.inside[0].ID},
		TagsOp: "add",
		TagIds: []uint{f.tag.ID}, // a tag the scoped principal did not create
	})
	if err != nil {
		t.Fatalf("scoped principal could not add a global tag: %v", err)
	}
	if result.Ops[0].RowsAffected != 1 {
		t.Errorf("rowsAffected=%d, want 1", result.Ops[0].RowsAffected)
	}
}

// ---------------------------------------------------------------------------
// tags: engine parity on SQLite (the Postgres arm is mass_edit_pg_test.go)
// ---------------------------------------------------------------------------

func TestMassEditTagsReplaceGivesExactSetEquality(t *testing.T) {
	f := newMassEditFixture(t, "replace", 3)
	addTagTo(f.ctx, f.res[0], f.tag)
	addTagTo(f.ctx, f.res[1], f.tag)
	addTagTo(f.ctx, f.res[1], f.tag2)

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     f.ids(),
		TagsOp: "replace",
		TagIds: []uint{f.tag2.ID},
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	for _, r := range f.res {
		if got := resTagIDs(t, f.ctx, r.ID); len(got) != 1 || got[0] != f.tag2.ID {
			t.Errorf("resource %d has tags %v, want exactly [%d]", r.ID, got, f.tag2.ID)
		}
	}
}

func TestMassEditTagsReplaceWithEmptyListClearsAll(t *testing.T) {
	// The NOT IN () trap: an empty id list with the NOT IN form deletes
	// nothing, silently turning "replace with nothing" into a no-op.
	f := newMassEditFixture(t, "cleartags", 2)
	addTagTo(f.ctx, f.res[0], f.tag)
	addTagTo(f.ctx, f.res[1], f.tag2)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     f.ids(),
		TagsOp: "replace", // no TagIds: clear all
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	for _, r := range f.res {
		if got := resTagIDs(t, f.ctx, r.ID); len(got) != 0 {
			t.Errorf("resource %d still has tags %v after replace-with-nothing", r.ID, got)
		}
	}
}

func TestMassEditTagsAddIsIdempotentAndRemoveOfAbsentIsANoOp(t *testing.T) {
	f := newMassEditFixture(t, "idem", 1)

	for i := 0; i < 2; i++ {
		if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
			ID: f.ids(), TagsOp: "add", TagIds: []uint{f.tag.ID},
		}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", f.res[0].ID); got != 1 {
		t.Errorf("add is not idempotent: %d rows", got)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID: f.ids(), TagsOp: "remove", TagIds: []uint{999999},
	}); err != nil {
		t.Fatalf("remove of an absent tag errored: %v", err)
	}
	if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", f.res[0].ID); got != 1 {
		t.Errorf("remove of an absent tag changed rows: %d", got)
	}
}

func TestMassEditRelatedGroupsDropsSelfEdge(t *testing.T) {
	f := newMassEditFixture(t, "selfedge", 1)
	g := &models.Group{Name: "self-edge-group"}
	if err := f.ctx.db.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	if _, err := f.ctx.MassEditGroups(&query_models.MassEditQuery{
		ID:              []uint{g.ID},
		RelatedGroupsOp: "add",
		RelatedGroupIds: []uint{g.ID, f.owner.ID},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	var selfEdges int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?", g.ID, g.ID).Scan(&selfEdges).Error; err != nil {
		t.Fatalf("count self edges: %v", err)
	}
	if selfEdges != 0 {
		t.Errorf("a self-edge was produced (%d rows); AddRelation refuses and MergeGroups sweeps those", selfEdges)
	}
	var ownerEdges int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?", g.ID, f.owner.ID).Scan(&ownerEdges).Error; err != nil {
		t.Fatalf("count owner edges: %v", err)
	}
	if ownerEdges != 1 {
		t.Errorf("the relation to the unrelated group is missing (%d rows)", ownerEdges)
	}
}

// ---------------------------------------------------------------------------
// meta and series — the highest-value block
// ---------------------------------------------------------------------------

// newSeriesFixture builds a series whose meta is {"k":"from-series","other":
// "series-value"} and one member of it.
func newSeriesFixture(t *testing.T, name string) (*MahresourcesContext, *models.Resource) {
	t.Helper()
	ctx := createAdminTestContext(t, "mass_edit_series_"+massEditName(name))

	series := &models.Series{
		Name: name + "-series",
		Slug: name + "-series",
		Meta: types.JSON(`{"k":"from-series","other":"series-value"}`),
	}
	if err := ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	r := &models.Resource{Name: name + "-member", SeriesID: &series.ID}
	if err := ctx.db.Create(r).Error; err != nil {
		t.Fatalf("create series member: %v", err)
	}
	return ctx, r
}

func TestMassEditMetaMergeNonSeries(t *testing.T) {
	f := newMassEditFixture(t, "mergemeta", 1)
	r := f.res[0]
	setResourceMetaRaw(t, f.ctx, r.ID, `{"keep":1,"gone":2}`, `{}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "merge",
		Meta:   `{"added":true,"gone":null}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, f.ctx, r.ID)
	if meta["added"] != true {
		t.Errorf("key not added: %v", meta)
	}
	if _, present := meta["gone"]; present {
		t.Error("merged null did not delete the existing key")
	}
	if meta["keep"] != float64(1) {
		t.Errorf("unrelated key not preserved: %v", meta)
	}
}

func TestMassEditMetaMergeNullOnSeriesSuppressesTheKey(t *testing.T) {
	ctx, r := newSeriesFixture(t, "supp")
	// The series defines k; the resource's effective meta carries an override.
	setResourceMetaRaw(t, ctx, r.ID, `{"k":"override","z":"own"}`, `{"k":"override"}`)

	if _, err := ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "merge",
		Meta:   `{"k":null}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, ctx, r.ID)
	if _, present := meta["k"]; present {
		t.Errorf("k still present in meta: %v", meta)
	}
	own := massResourceOwnMeta(t, ctx, r.ID)
	if _, present := own["k"]; !present {
		t.Fatalf("own_meta has no explicit null for k: %v", own)
	}
	if own["k"] != nil {
		t.Errorf("own_meta[k] = %v, want null", own["k"])
	}

	// Re-reading through mergeMeta keeps the key suppressed.
	var series models.Series
	if err := ctx.db.First(&series, *r.SeriesID).Error; err != nil {
		t.Fatalf("reload series: %v", err)
	}
	merged, mergeErr := mergeMeta(series.Meta, types.JSON(rawJSONColumn(t, ctx, "own_meta", r.ID)))
	if mergeErr != nil {
		t.Fatalf("mergeMeta: %v", mergeErr)
	}
	suppressed := jsonMapOrEmpty(t, []byte(merged))
	if _, present := suppressed["k"]; present {
		t.Errorf("the series re-inherited the removed key: %v", suppressed)
	}
}

func TestMassEditMetaMergeOntoSQLNULLDoesNotBlowTheColumnAway(t *testing.T) {
	f := newMassEditFixture(t, "nullmeta", 1)
	if err := f.ctx.db.Exec("UPDATE resources SET meta = NULL WHERE id = ?", f.res[0].ID).Error; err != nil {
		t.Fatalf("null the meta: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.res[0].ID},
		MetaOp: "merge",
		Meta:   `{"a":1}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, f.ctx, f.res[0].ID)
	if meta["a"] != float64(1) {
		t.Errorf("merging onto a NULL meta lost the patch: %v", meta)
	}
}

func TestMassEditMetaRemoveKeysOnSeriesResourceSuppressesReinheritance(t *testing.T) {
	ctx, r := newSeriesFixture(t, "removekeys")
	// Series defines k and other; resource overrides k and owns z.
	setResourceMetaRaw(t, ctx, r.ID, `{"k":"v","z":"zv"}`, `{"k":"v","z":"zv"}`)

	if _, err := ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{r.ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"k"},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, ctx, r.ID)
	if _, present := meta["k"]; present {
		t.Errorf("k still in effective meta: %v", meta)
	}
	own := massResourceOwnMeta(t, ctx, r.ID)
	if _, present := own["k"]; !present {
		t.Fatalf("own_meta lost the explicit null that keeps the series from re-inheriting k: %v", own)
	}
	if own["k"] != nil {
		t.Errorf("own_meta[k]=%v, want null", own["k"])
	}
}

func TestMassEditMetaRemoveKeysOnNonSeriesClearsBothColumns(t *testing.T) {
	f := newMassEditFixture(t, "rknoseries", 1)
	setResourceMetaRaw(t, f.ctx, f.res[0].ID, `{"gone":1,"stay":2}`, `{"gone":9}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.res[0].ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"gone"},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	if _, present := massResourceMeta(t, f.ctx, f.res[0].ID)["gone"]; present {
		t.Error("gone still in meta")
	}
	if _, present := massResourceOwnMeta(t, f.ctx, f.res[0].ID)["gone"]; present {
		t.Error("gone still in own_meta")
	}
	if _, present := massResourceMeta(t, f.ctx, f.res[0].ID)["stay"]; !present {
		t.Error("the unrelated key was removed too")
	}
}

func TestMassEditMetaRemoveKeysWithSpace(t *testing.T) {
	// A key containing a space is exactly what the bound, quoted JSON path
	// spelling exists for: $.key would break on it.
	f := newMassEditFixture(t, "spacekey", 1)
	setResourceMetaRaw(t, f.ctx, f.res[0].ID, `{"c d":"2","e":"3"}`, `{}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.res[0].ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"c d"},
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, f.ctx, f.res[0].ID)
	if len(meta) != 1 {
		t.Fatalf("meta = %v, want only {e:3}", meta)
	}
	if _, present := meta["e"]; !present {
		t.Errorf("the wrong key was removed: %v", meta)
	}
}

func TestMassEditMetaRemoveKeysRefusesPathLikeKeys(t *testing.T) {
	// removeKeys is top-level keys only; a key containing "." or "$" would be
	// read as a JSON path by whoever maintains the SQL, so it is refused.
	f := newMassEditFixture(t, "pathkey", 1)
	setResourceMetaRaw(t, f.ctx, f.res[0].ID, `{"a.b":"1","e":"3"}`, `{}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.res[0].ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"a.b"},
	}); err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("expected the path-like key refusal, got %v", err)
	}
	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.res[0].ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"a$b"},
	}); err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("expected the path-like key refusal, got %v", err)
	}
	if _, present := massResourceMeta(t, f.ctx, f.res[0].ID)["a.b"]; !present {
		t.Error("a refused key removal changed the meta")
	}
}

func TestMassEditMetaRemoveKeysOfAbsentKeyIsANoOp(t *testing.T) {
	f := newMassEditFixture(t, "absentkey", 1)
	setResourceMetaRaw(t, f.ctx, f.res[0].ID, `{"e":"3"}`, `{}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.res[0].ID},
		MetaOp:   "removeKeys",
		MetaKeys: []string{"not-there"},
	}); err != nil {
		t.Fatalf("removeKeys of an absent key errored: %v", err)
	}
	if _, present := massResourceMeta(t, f.ctx, f.res[0].ID)["e"]; !present {
		t.Error("an absent key removal disturbed the existing meta")
	}
}

func TestMassEditMetaReplaceOnSeriesResourceComputesOwnMeta(t *testing.T) {
	ctx, r := newSeriesFixture(t, "replace")
	// series meta: {"k":"from-series","other":"series-value"}

	submitted := `{"k":"new-value"}`
	if _, err := ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{r.ID},
		MetaOp: "replace",
		Meta:   submitted,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	meta := massResourceMeta(t, ctx, r.ID)
	if meta["k"] != "new-value" {
		t.Errorf("meta = %v, want the submitted effective value", meta)
	}
	var series models.Series
	if err := ctx.db.First(&series, *r.SeriesID).Error; err != nil {
		t.Fatalf("reload series: %v", err)
	}
	want, err := computeOwnMeta(types.JSON(submitted), series.Meta, true)
	if err != nil {
		t.Fatalf("computeOwnMeta: %v", err)
	}
	got := massResourceOwnMeta(t, ctx, r.ID)
	expected := jsonMapOrEmpty(t, []byte(want))
	if fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Errorf("own_meta = %v, want computeOwnMeta(submitted, series.Meta, true) = %v", got, expected)
	}
}

func TestMassEditMetaReplaceOnNonSeriesSetsEmptyOwnMeta(t *testing.T) {
	f := newMassEditFixture(t, "replmeta", 1)
	setResourceMetaRaw(t, f.ctx, f.res[0].ID, `{"old":1}`, `{"stale":1}`)

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.res[0].ID},
		MetaOp: "replace",
		Meta:   `{"fresh":true}`,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	if got := massResourceMeta(t, f.ctx, f.res[0].ID); got["fresh"] != true || len(got) != 1 {
		t.Errorf("meta = %v, want the submitted object", got)
	}
	own := massResourceOwnMeta(t, f.ctx, f.res[0].ID)
	if len(own) != 0 {
		t.Errorf("own_meta = %v, want {}", own)
	}
}

func TestMassEditOwnerSetUpdatesOwnerAndBumpsUpdatedAt(t *testing.T) {
	f := newMassEditFixture(t, "ownerset", 1)
	past := time.Now().Add(-time.Hour)
	if err := f.ctx.db.Model(f.res[0]).UpdateColumn("updated_at", past).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:      f.ids(),
		OwnerOp: "set",
		OwnerId: f.owner.ID,
	}); err != nil {
		t.Fatalf("mass edit: %v", err)
	}

	var res models.Resource
	if err := f.ctx.db.First(&res, f.res[0].ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if res.OwnerId == nil || *res.OwnerId != f.owner.ID {
		t.Errorf("owner_id = %v, want %d", res.OwnerId, f.owner.ID)
	}
	if !res.UpdatedAt.After(past) {
		t.Errorf("updated_at was not bumped: %v", res.UpdatedAt)
	}
}

func TestMassEditOwnerClearUnscopedWorksAndScopedIsRefused(t *testing.T) {
	f := newMassEditFixture(t, "clearowner", 1)
	if err := f.ctx.db.Model(f.res[0]).Update("owner_id", f.owner.ID).Error; err != nil {
		t.Fatalf("set owner: %v", err)
	}

	if _, err := f.ctx.MassEditResources(&query_models.MassEditQuery{ID: f.ids(), OwnerOp: "clear"}); err != nil {
		t.Fatalf("unscoped clear: %v", err)
	}
	var res models.Resource
	if err := f.ctx.db.First(&res, f.res[0].ID).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if res.OwnerId != nil {
		t.Error("owner was not cleared")
	}

	// A scoped principal may not clear: an owner-less row is in nobody's
	// subtree, so the eviction is permanent from their point of view. Re-own
	// the row first so the scoped principal can see it at all.
	if err := f.ctx.db.Model(&models.Resource{}).Where("id = ?", f.res[0].ID).Update("owner_id", f.owner.ID).Error; err != nil {
		t.Fatalf("re-own: %v", err)
	}
	scoped := scopedMassEditContext(f.ctx, f.owner)
	_, err := scoped.MassEditResources(&query_models.MassEditQuery{ID: f.ids(), OwnerOp: "clear"})
	if err != ErrMassEditOwnerClearScoped {
		t.Fatalf("expected ErrMassEditOwnerClearScoped, got %v", err)
	}
	var count int64
	f.ctx.db.Model(&models.Resource{}).Where("id = ? AND owner_id IS NOT NULL", f.res[0].ID).Count(&count)
	if count != 1 {
		t.Error("a refused owner clear changed the owner")
	}
}

func TestMassEditOwnerSetToOutOfSubtreeGroupIsRefused(t *testing.T) {
	f := newMassScopedFixture(t, "outowner", 1, 0)

	_, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:      []uint{f.inside[0].ID},
		OwnerOp: "set",
		OwnerId: f.outsideGroup.ID,
	})
	if err == nil || !strings.Contains(err.Error(), "owner group not found") {
		t.Fatalf("expected \"owner group not found\", got %v", err)
	}
}

// ---------------------------------------------------------------------------
// groups: owner is the parent
// ---------------------------------------------------------------------------

func TestMassEditGroupSelfOwnershipRefused(t *testing.T) {
	f := newMassEditFixture(t, "gself", 0)
	g := &models.Group{Name: "self-owner-group"}
	if err := f.ctx.db.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	_, err := f.ctx.MassEditGroups(&query_models.MassEditQuery{
		ID:      []uint{g.ID},
		OwnerOp: "set",
		OwnerId: g.ID,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be its own owner") {
		t.Fatalf("expected self-ownership refusal, got %v", err)
	}
}

func TestMassEditGroupOwnershipCycleRefusedNotRepaired(t *testing.T) {
	f := newMassEditFixture(t, "cycle", 0)
	parent := &models.Group{Name: "cycle-parent"}
	if err := f.ctx.db.Create(parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child := &models.Group{Name: "cycle-child", OwnerId: &parent.ID}
	if err := f.ctx.db.Create(child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err := f.ctx.MassEditGroups(&query_models.MassEditQuery{
		ID:      []uint{parent.ID},
		OwnerOp: "set",
		OwnerId: child.ID, // the child is below the target: a cycle
	})
	if err != ErrMassEditOwnershipCycle {
		t.Fatalf("expected ErrMassEditOwnershipCycle, got %v", err)
	}
	var fresh models.Group
	if err := f.ctx.db.First(&fresh, parent.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh.OwnerId != nil {
		t.Errorf("the refusal repaired the row instead: owner_id is now %v", fresh.OwnerId)
	}
}

func TestMassEditGroupLegalReparentCommits(t *testing.T) {
	f := newMassEditFixture(t, "reparent", 0)
	newParent := &models.Group{Name: "new-parent"}
	if err := f.ctx.db.Create(newParent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	targets := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		g := &models.Group{Name: fmt.Sprintf("reparent-%d", i)}
		if err := f.ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
		targets = append(targets, g.ID)
	}

	_, err := f.ctx.MassEditGroups(&query_models.MassEditQuery{
		ID:      targets,
		OwnerOp: "set",
		OwnerId: newParent.ID,
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	for _, id := range targets {
		var group models.Group
		if err := f.ctx.db.First(&group, id).Error; err != nil {
			t.Fatalf("reload group %d: %v", id, err)
		}
		if group.OwnerId == nil || *group.OwnerId != newParent.ID {
			t.Errorf("group %d owner = %v, want %d", id, group.OwnerId, newParent.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// atomicity and chunking
// ---------------------------------------------------------------------------

func TestMassEditIsAtomicWhenALaterOpFails(t *testing.T) {
	// Op 3 of 3 fails (a SQLite trigger raises on the owner update of one row);
	// ops 1 and 2 must leave zero rows changed. Chunk size 1 makes the failure
	// land in a LATER chunk than the writes ops 1 and 2 already committed to
	// the transaction, pinning rollback across chunk boundaries too.
	f := newMassEditFixture(t, "atomic", 3)
	origChunk := massEditChunkSize
	massEditChunkSize = 1
	t.Cleanup(func() { massEditChunkSize = origChunk })
	setResourceMetaRaw(t, f.ctx, f.res[2].ID, `{"orig":true}`, `{}`)
	trigger := fmt.Sprintf(`
		CREATE TRIGGER mass_edit_atomic_abort BEFORE UPDATE ON resources FOR EACH ROW
		WHEN NEW.id = %d AND NEW.owner_id IS NOT NULL
		BEGIN
			SELECT RAISE(ABORT, 'injected owner failure');
		END
	`, f.res[1].ID)
	if err := f.ctx.db.Exec(trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:      f.ids(),
		TagsOp:  "add",
		TagIds:  []uint{f.tag.ID},
		MetaOp:  "merge",
		Meta:    `{"touched":true}`,
		OwnerOp: "set",
		OwnerId: f.owner.ID,
	})
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("expected the injected failure, got %v", err)
	}

	if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", f.res[0].ID); got != 0 {
		t.Error("op 1 survived a rolled-back transaction")
	}
	if meta := massResourceMeta(t, f.ctx, f.res[0].ID); len(meta) != 0 {
		t.Errorf("op 2 survived a rolled-back transaction: %v", meta)
	}
}

func TestMassEditChunkBoundariesAreCorrect(t *testing.T) {
	f := newMassEditFixture(t, "chunks", 5)

	orig := massEditChunkSize
	massEditChunkSize = 2
	t.Cleanup(func() { massEditChunkSize = orig })

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:     f.ids(),
		TagsOp: "add",
		TagIds: []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("mass edit: %v", err)
	}
	if result.Ops[0].RowsAffected != 5 {
		t.Errorf("rowsAffected=%d, want 5 across three chunks", result.Ops[0].RowsAffected)
	}
	for _, r := range f.res {
		if got := resTagIDs(t, f.ctx, r.ID); len(got) != 1 {
			t.Errorf("resource %d has %d tag rows, want 1", r.ID, got)
		}
	}
}

// ---------------------------------------------------------------------------
// regressions from the final review
// ---------------------------------------------------------------------------

func TestMassEditIDsModeOverCapIsRefused(t *testing.T) {
	// The ceiling is a lock-duration budget, not a filter-mode rule: an
	// explicit selection holds the same write lock.
	f := newMassEditFixture(t, "idcap", 3)
	f.ctx.Config.MaxMassEditEntities = 2
	t.Cleanup(func() { f.ctx.Config.MaxMassEditEntities = 0 })

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		ID:      f.ids(),
		OwnerOp: "clear",
	})
	if err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("expected the ceiling error, got %v", err)
	}
	for _, r := range f.res {
		var fresh models.Resource
		if err := f.ctx.db.First(&fresh, r.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if fresh.OwnerId != nil {
			t.Error("a refused mass edit changed the owner")
		}
	}
}

func TestMassEditFilterModeWithEmptyFilterTargetsEverythingVisible(t *testing.T) {
	// "Mass edit all results" on an unfiltered list page sends an empty filter
	// string; that must mean the unfiltered (but still scope-confined) set, not
	// a request error.
	f := newMassEditFixture(t, "allres", 4)

	result, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "",
		ExpectedCount: uip(4),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if err != nil {
		t.Fatalf("mass edit with an empty filter: %v", err)
	}
	if result.Matched != 4 {
		t.Errorf("matched=%d, want 4", result.Matched)
	}
	for _, r := range f.res {
		if got := resTagIDs(t, f.ctx, r.ID); len(got) != 1 {
			t.Errorf("resource %d not tagged by the unfiltered edit", r.ID)
		}
	}
}

func TestMassEditCountAndPluckDisagreementIsAConflict(t *testing.T) {
	// The Count/Pluck pair is check-then-act across two queries; the
	// materialized id set is authoritative, so a set that shrank between the
	// two queries must be answered with the same conflict a stale
	// ExpectedCount gets. The seam makes the interleaving deterministic: the
	// world deletes one matching row after the Count, before the Pluck.
	f := newMassEditFixture(t, "pluckmismatch", 4)

	origHook := massEditBetweenCountAndPluck
	massEditBetweenCountAndPluck = func() {
		if err := f.ctx.db.Where("name = ?", "pluckmismatch-res-3").Delete(&models.Resource{}).Error; err != nil {
			t.Errorf("hook delete: %v", err)
		}
	}
	t.Cleanup(func() { massEditBetweenCountAndPluck = origHook })

	_, err := f.ctx.MassEditResources(&query_models.MassEditQuery{
		Target:        "filter",
		Filter:        "name=pluckmismatch-res",
		ExpectedCount: uip(4),
		TagsOp:        "add",
		TagIds:        []uint{f.tag.ID},
	})
	if !errors.Is(err, ErrMassEditSetChanged) {
		t.Fatalf("expected ErrMassEditSetChanged, got %v", err)
	}
	for _, r := range f.res {
		if got := countJoinRows(t, f.ctx, "resource_tags", "resource_id", r.ID); got != 0 {
			t.Error("a refused mass edit wrote to the database")
		}
	}
}

func TestMassEditScopedRemoveOfOutOfSubtreeFarEndpointIsRefused(t *testing.T) {
	// The raw DELETE gets no scope callback: without the far-endpoint
	// validation, a group-limited principal could break a link to a group it
	// cannot even see — an edge it never knew existed, gone without a trace.
	f := newMassScopedFixture(t, "scopedrm", 2, 0)
	outGroup := f.outsideGroup

	// Pre-existing edge: inside resource is related to the OUTSIDE group.
	if err := f.ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)",
		f.inside[0].ID, outGroup.ID).Error; err != nil {
		t.Fatalf("seed edge: %v", err)
	}

	_, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.inside[0].ID},
		GroupsOp: "remove",
		GroupIds: []uint{outGroup.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "one or more groups not found") {
		t.Fatalf("expected far-endpoint refusal, got %v", err)
	}
	if got := countJoinRows(t, f.ctx, "groups_related_resources", "resource_id", f.inside[0].ID); got != 1 {
		t.Error("a refused remove destroyed the out-of-subtree edge")
	}
}

func TestMassEditScopedReplaceSparesOutOfSubtreeEdges(t *testing.T) {
	// Replace deletes "everything not in the new set". For a scoped principal
	// that must mean everything VISIBLE not in the new set: an existing edge to
	// an out-of-subtree group is spared, the way deleteRelationsSparingUnseen
	// spares unseen relation edges.
	f := newMassScopedFixture(t, "scopedrepl", 2, 0)
	outGroup := f.outsideGroup
	// inside the subtree: a child of the scope group
	insideGroup := &models.Group{Name: "scopedrepl-inside-group", OwnerId: &f.scopeGroup.ID}
	if err := f.ctx.db.Create(insideGroup).Error; err != nil {
		t.Fatalf("create inside group: %v", err)
	}

	// Existing edges: one to a visible (in-subtree) group, one to an invisible
	// (out-of-subtree) one. The scope allow-list is a snapshot taken when the
	// principal was bound, so the in-subtree group is created before the
	// scoped context is re-derived here.
	if err := f.ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)",
		f.inside[0].ID, insideGroup.ID).Error; err != nil {
		t.Fatalf("seed inside edge: %v", err)
	}
	if err := f.ctx.db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)",
		f.inside[0].ID, outGroup.ID).Error; err != nil {
		t.Fatalf("seed outside edge: %v", err)
	}
	scoped := scopedMassEditContext(f.ctx, f.scopeGroup)

	_, err := scoped.MassEditResources(&query_models.MassEditQuery{
		ID:       []uint{f.inside[0].ID},
		GroupsOp: "replace",
		GroupIds: []uint{f.scopeGroup.ID},
	})
	if err != nil {
		t.Fatalf("scoped replace: %v", err)
	}

	var visibleEdge, hiddenEdge int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM groups_related_resources WHERE resource_id = ? AND group_id = ?",
		f.inside[0].ID, insideGroup.ID).Scan(&visibleEdge).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM groups_related_resources WHERE resource_id = ? AND group_id = ?",
		f.inside[0].ID, outGroup.ID).Scan(&hiddenEdge).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if visibleEdge != 0 {
		t.Error("the visible edge was not replaced away")
	}
	if hiddenEdge != 1 {
		t.Error("the replace silently destroyed an edge to an out-of-subtree group")
	}

	// And the new edge was added.
	var newEdge int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM groups_related_resources WHERE resource_id = ? AND group_id = ?",
		f.inside[0].ID, f.scopeGroup.ID).Scan(&newEdge).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if newEdge != 1 {
		t.Error("the replacement edge was not added")
	}
}

func TestMassEditScopedReplaceSpareRuleUsesTheFarModelScopeColumn(t *testing.T) {
	// Regression for the round-4 review: notes and resources are scoped by
	// their OWNER's subtree, not their id. A replace must spare an edge whose
	// far note/resource is live but owned outside the subtree, and remove one
	// whose far row is owned inside — a group-id IN-comparison would get both
	// wrong by numerical accident.
	f := newMassScopedFixture(t, "farnote", 2, 0)
	outGroup := f.outsideGroup

	outNote := &models.Note{Name: "far-scope-out-note", OwnerId: &outGroup.ID}
	if err := f.ctx.db.Create(outNote).Error; err != nil {
		t.Fatalf("create out note: %v", err)
	}
	inNote := &models.Note{Name: "far-scope-in-note", OwnerId: &f.scopeGroup.ID}
	if err := f.ctx.db.Create(inNote).Error; err != nil {
		t.Fatalf("create in note: %v", err)
	}

	// Pre-existing edges: one to a live out-of-subtree note (spared), one to a
	// live in-subtree note (replaced away).
	if err := f.ctx.db.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)",
		f.inside[0].ID, outNote.ID).Error; err != nil {
		t.Fatalf("seed out edge: %v", err)
	}
	if err := f.ctx.db.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)",
		f.inside[0].ID, inNote.ID).Error; err != nil {
		t.Fatalf("seed in edge: %v", err)
	}

	// The scope snapshot is taken at WithPrincipal, so the scoped context is
	// re-derived after every fixture row exists.
	scoped := scopedMassEditContext(f.ctx, f.scopeGroup)
	_, err := scoped.MassEditResources(&query_models.MassEditQuery{
		ID:      []uint{f.inside[0].ID},
		NotesOp: "replace", // no NoteIds: clear all visible
	})
	if err != nil {
		t.Fatalf("scoped replace: %v", err)
	}

	var inEdges, outEdges int64
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM resource_notes WHERE resource_id = ? AND note_id = ?",
		f.inside[0].ID, inNote.ID).Scan(&inEdges).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.ctx.db.Raw("SELECT COUNT(*) FROM resource_notes WHERE resource_id = ? AND note_id = ?",
		f.inside[0].ID, outNote.ID).Scan(&outEdges).Error; err != nil {
		t.Fatal(err)
	}
	if inEdges != 0 {
		t.Error("the visible edge was not replaced away — the visibility test used the wrong scope column")
	}
	if outEdges != 1 {
		t.Error("the replace silently destroyed an edge to a live out-of-subtree note")
	}
}

func TestMassEditScopedTagReplaceUsesThePlainShape(t *testing.T) {
	// Tags are global: a scoped replace removes every edge not in the new set,
	// exactly like an unscoped one — tags have no owner and no subtree.
	f := newMassScopedFixture(t, "tagglobal", 1, 0)
	tag := &models.Tag{Name: "tagglobal-replacement"}
	if err := f.ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := f.ctx.db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)",
		f.inside[0].ID, f.tag.ID).Error; err != nil {
		t.Fatalf("seed edge: %v", err)
	}

	if _, err := f.scoped.MassEditResources(&query_models.MassEditQuery{
		ID:     []uint{f.inside[0].ID},
		TagsOp: "replace",
		TagIds: []uint{tag.ID},
	}); err != nil {
		t.Fatalf("scoped tag replace: %v", err)
	}
	if got := resTagIDs(t, f.ctx, f.inside[0].ID); len(got) != 1 || got[0] != tag.ID {
		t.Errorf("tags %v, want exactly the replacement — a scoped replace must not preserve old tags", got)
	}
}

func TestValidateAndLockIDsAcceptsDuplicateIDs(t *testing.T) {
	// Regression for the lockIDs refactor: the original implementation
	// deduplicated before comparing, so a caller legitimately passing the same
	// id twice (the upload path's association lists) must not be refused.
	f := newMassEditFixture(t, "duplock", 1)

	if err := validateAndLockIDs(f.ctx.db, &models.Resource{}, []uint{f.res[0].ID, f.res[0].ID}, "resources"); err != nil {
		t.Fatalf("duplicate ids were refused: %v", err)
	}
	if err := validateAndLockIDs(f.ctx.db, &models.Resource{}, []uint{f.res[0].ID, 999999}, "resources"); err == nil {
		t.Fatal("a genuinely missing id was not refused")
	}
}

func TestMassEditExhaustedBindBudgetRefusesAtParse(t *testing.T) {
	// Regression for the zero-budget sentinel: an exhausted budget is a
	// legitimate 0, not "no check". A scoped replace whose allow-list consumes
	// the whole statement budget must be refused at PARSE time — DryRun and
	// confirmed submit identically — instead of dying at apply time with a
	// driver error.
	f := newMassScopedFixture(t, "zerobudget", 2, 0)
	scoped := scopedMassEditContext(f.ctx, f.scopeGroup)

	orig := massEditBindBudgetOverride
	massEditBindBudgetOverride = 0
	t.Cleanup(func() { massEditBindBudgetOverride = orig })

	for _, dryRun := range []bool{false, true} {
		_, err := scoped.MassEditResources(&query_models.MassEditQuery{
			ID:       []uint{f.inside[0].ID},
			GroupsOp: "replace",
			GroupIds: []uint{f.scopeGroup.ID},
			DryRun:   dryRun,
		})
		if err == nil || !strings.Contains(err.Error(), "parameter budget") {
			t.Fatalf("dryRun=%v: expected the parameter-budget refusal, got %v", dryRun, err)
		}
	}
	// Nothing was written by either attempt.
	if got := countJoinRows(t, f.ctx, "groups_related_resources", "resource_id", f.inside[0].ID); got != 0 {
		t.Error("a refused mass edit wrote to the database")
	}
}
