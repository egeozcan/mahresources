package application_context

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These drive the real accessor and the real loader. An earlier version of this
// file re-implemented the generation check inline and asserted against its own
// copy, which passed with the production guard deleted — a test of pseudocode.
func newScopedAccessTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.PluginState{}, &models.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	// One connection. `cache=private` gives every connection its OWN in-memory
	// database, so a concurrent reader that opens a second one finds no tables
	// and the test measures fail-closed read errors instead of the cache.
	sqlDB.SetMaxOpenConns(1)
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

func setPluginRow(t *testing.T, ctx *MahresourcesContext, name string, enabled, allowed bool) {
	t.Helper()
	state := models.PluginState{PluginName: name}
	if err := ctx.db.Where("plugin_name = ?", name).FirstOrCreate(&state).Error; err != nil {
		t.Fatalf("seed plugin row: %v", err)
	}
	if err := ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", name).
		Updates(map[string]any{"enabled": enabled, "allow_scoped_principals": allowed}).Error; err != nil {
		t.Fatalf("update plugin row: %v", err)
	}
}

func TestPluginAllowsScopedPrincipals_ReadsWhatIsStored(t *testing.T) {
	ctx := newScopedAccessTestContext(t)

	for _, tc := range []struct {
		name    string
		enabled bool
		allowed bool
		want    bool
	}{
		{"enabled and allowed", true, true, true},
		{"enabled, not allowed", true, false, false},
		// A stale allow on a disabled plugin must not read as reachable:
		// disabling is not consent withdrawal, but it is not an invitation.
		{"disabled but allowed", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPluginRow(t, ctx, "widgets", tc.enabled, tc.allowed)
			ctx.InvalidateScopedPluginAccess()

			if got := ctx.PluginAllowsScopedPrincipals("widgets"); got != tc.want {
				t.Errorf("PluginAllowsScopedPrincipals = %v, want %v", got, tc.want)
			}
		})
	}

	// An unknown plugin, and the empty name a path with no plugin segment
	// produces, are both refusals rather than lookups that happen to miss.
	if ctx.PluginAllowsScopedPrincipals("never-installed") {
		t.Error("an unknown plugin read as reachable")
	}
	if ctx.PluginAllowsScopedPrincipals("") {
		t.Error("the empty plugin name read as reachable")
	}
}

// The cached answer must not outlive the row it came from. This goes through
// InvalidateScopedPluginAccess, which is what SetPluginScopedAccess calls.
func TestPluginAllowsScopedPrincipals_InvalidationIsVisibleImmediately(t *testing.T) {
	ctx := newScopedAccessTestContext(t)
	setPluginRow(t, ctx, "widgets", true, true)

	if !ctx.PluginAllowsScopedPrincipals("widgets") {
		t.Fatal("setup: the plugin should be reachable")
	}

	setPluginRow(t, ctx, "widgets", true, false)
	ctx.InvalidateScopedPluginAccess()

	if ctx.PluginAllowsScopedPrincipals("widgets") {
		t.Error("a revoked plugin was still served from the cache")
	}
}

// The finding this exists for: a load that began before a revocation must not
// publish its answer after it. The invalidation is driven from inside the
// loader's own database read, which is the only way to land it in that window
// deterministically.
//
// The callback does no database work of its own, deliberately: an inner write
// issued while the outer read is still open deadlocks on SQLite's single
// connection. Bumping the generation is the whole of what the guard reads.
func TestLoadScopedPluginAccess_ARevocationDuringTheReadIsNotPublished(t *testing.T) {
	ctx := newScopedAccessTestContext(t)
	setPluginRow(t, ctx, "widgets", true, true)
	ctx.InvalidateScopedPluginAccess()

	var once sync.Once
	const hook = "test:revoke_during_load"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(hook, func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Table != "plugin_states" {
			return
		}
		// The operator's revocation lands while this very read is in flight.
		once.Do(ctx.InvalidateScopedPluginAccess)
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(hook) })

	loaded, err := ctx.loadScopedPluginAccess()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// The loader still answers its own caller — it is at worst as stale as the
	// moment it started — but it must not have become everyone else's answer.
	if !loaded.allowed["widgets"] {
		t.Fatal("setup: the load should have read the pre-revocation row")
	}
	if published := ctx.scopedAccess.snapshot.Load(); published != nil {
		t.Errorf("a load that started before the revocation published anyway (%v), so the permission would come back for a full TTL",
			published.allowed)
	}
}

// Concurrent readers must not produce a torn or stale published answer, and the
// loader must not deadlock on its own lock. Run with -race.
func TestPluginAllowsScopedPrincipals_ConcurrentReadersAndARevocation(t *testing.T) {
	ctx := newScopedAccessTestContext(t)
	setPluginRow(t, ctx, "widgets", true, true)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				ctx.PluginAllowsScopedPrincipals("widgets")
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		setPluginRow(t, ctx, "widgets", true, false)
		ctx.InvalidateScopedPluginAccess()
	}()
	wg.Wait()

	if ctx.PluginAllowsScopedPrincipals("widgets") {
		t.Error("after the revocation settled, the plugin still reads as reachable")
	}
}

// A context with no cache (a bare struct literal, which several tests build)
// still has to answer from the database rather than panic or answer yes.
func TestPluginAllowsScopedPrincipals_WorksWithoutTheCache(t *testing.T) {
	ctx := newScopedAccessTestContext(t)
	setPluginRow(t, ctx, "widgets", true, true)

	uncached := *ctx
	uncached.scopedAccess = nil

	if !uncached.PluginAllowsScopedPrincipals("widgets") {
		t.Error("a context without the cache did not read the stored value")
	}
}

// The rule publish() actually enforces: an invalidation during the read
// discards that read's answer. Ordering two concurrent loaders is NOT attempted
// — no local clock can say which of them observed the database later — so what
// bounds a missed revocation is the TTL, and that is stated in the code rather
// than asserted here.
func TestScopedAccessPublish_AnInvalidationDuringTheReadDiscardsIt(t *testing.T) {
	cache := &scopedPluginAccess{}
	snapshot := &scopedAccessSnapshot{
		allowed:    map[string]bool{"widgets": true},
		generation: cache.generation.Load(),
		observedBy: time.Now(),
	}

	cache.generation.Add(1) // the operator revokes while the read is in flight

	if cache.publish(snapshot) {
		t.Error("a load that began before an invalidation was published anyway")
	}
	if cache.snapshot.Load() != nil {
		t.Error("the stale snapshot became everyone else's answer")
	}
}

// The bound the TTL claims only holds if a stalled loader cannot restart the
// clock. This is the interleaving: the read observed the old permission, a
// revocation committed elsewhere, and the loader woke up much later. Publishing
// it would trust that answer for another full TTL measured from long after it
// stopped being true — and a repeatedly slow loader would do it again.
func TestScopedAccessPublish_AnAnswerOlderThanTheTTLIsNotPublished(t *testing.T) {
	cache := &scopedPluginAccess{}
	stalled := &scopedAccessSnapshot{
		allowed:    map[string]bool{"widgets": true},
		generation: cache.generation.Load(),
		observedBy: time.Now().Add(-2 * scopedAccessTTL),
	}

	if cache.publish(stalled) {
		t.Error("an answer already older than the TTL was published, so it would be trusted for another one")
	}
	if cache.snapshot.Load() != nil {
		t.Error("the expired snapshot became everyone else's answer")
	}
}

// The interleaving the whole bound turns on: the read saw the permission, a
// revocation committed elsewhere, and this loader woke up after the answer had
// expired. Refusing to publish it is not enough — the loader's own caller must
// not be served it either, or one request is authorized by an answer the cache
// itself judged too old.
//
// Driven with a one-nanosecond lifetime rather than a sleep, so it exercises
// the real loader instead of the clock.
func TestLoadScopedPluginAccess_AnAnswerThatOutlivedItsWindowIsRefused(t *testing.T) {
	ctx := newScopedAccessTestContext(t)
	setPluginRow(t, ctx, "widgets", true, true)

	ctx.scopedAccess.ttl = time.Nanosecond
	ctx.InvalidateScopedPluginAccess()

	if _, err := ctx.loadScopedPluginAccess(); err == nil {
		t.Error("a load that outlived its own freshness window returned an answer instead of refusing")
	}

	// And the caller of record reads that refusal as "no", like any other
	// failure to find out.
	if ctx.PluginAllowsScopedPrincipals("widgets") {
		t.Error("an expired read authorized the caller anyway")
	}

	// The ordinary lifetime still answers, or this would be a cache that never
	// caches rather than one that refuses stale answers.
	ctx.scopedAccess.ttl = 0
	ctx.InvalidateScopedPluginAccess()
	if !ctx.PluginAllowsScopedPrincipals("widgets") {
		t.Error("with the normal lifetime the plugin should be reachable")
	}
}
