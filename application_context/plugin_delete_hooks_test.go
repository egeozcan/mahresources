package application_context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// createTestContextWithPlugins builds a context whose PluginManager is wired to
// pluginDir.
//
// cache=private and a per-test DSN, unlike createTestContext's shared in-memory
// DB: these tests count rows a hook created, and a database shared with the rest
// of the package would make those counts depend on what else ran.
func createTestContextWithPlugins(t *testing.T, pluginDir string) *MahresourcesContext {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.ResourceVersion{},
		&models.NoteBlock{},
		&models.LogEntry{},
		&models.PluginKV{},
		&models.PluginState{},
		// The stamp callback falls back to the root admin when no actor is on
		// the context, and resolving root reads this table.
		&models.User{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType:     constants.DbTypeSqlite,
		PluginPath: pluginDir,
	})
}

// These exercise the real chain — a Lua plugin registered against a real
// PluginManager — because the defect being fixed was precisely that the bulk
// paths never reached the dispatcher. A fake sink could be wired to the emit
// helpers and still not prove the callers call them.
//
// Two observation methods, deliberately:
//
//   - A Lua-side counter, read back through an injection slot, proves a hook
//     FIRED. It touches no database, which matters: a before-hook runs inside
//     the caller's open transaction, and mah.db writes are issued on a separate
//     connection, so on SQLite they lose the writer lock and vanish. Counting
//     rows would have reported a before-hook as never firing when it did.
//   - A tag the hook creates proves the hook ran OUTSIDE any transaction, since
//     that is the only condition under which the write can land. That is exactly
//     the after-hook guarantee this package adds, so it is the right assertion
//     for after-hooks and the wrong one for before-hooks.

func newPluginHookTestContext(t *testing.T, luaSource string) *MahresourcesContext {
	t.Helper()

	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "hooktest"), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooktest", "plugin.lua"), []byte(luaSource), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	ctx := createTestContextWithPlugins(t, pluginDir)
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("plugin manager was not wired: the test context has no plugin path")
	}
	if err := pm.EnablePlugin("hooktest"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	t.Cleanup(pm.Close)
	return ctx
}

// hookTagPlugin registers a hook per event that bumps a Lua-side counter and
// also attempts a tag write, and exposes the counters through the "page_bottom" slot.
func hookTagPlugin(events map[string]string) string {
	src := `plugin = { name = "hooktest", version = "1.0", description = "records delete hooks" }
local fires = {}
function init()
`
	for event, prefix := range events {
		src += `    mah.on("` + event + `", function(data)
        fires["` + prefix + `"] = (fires["` + prefix + `"] or 0) + 1
        mah.db.create_tag({ name = "` + prefix + `-" .. tostring(math.floor(data.id)) })
        return data
    end)
`
	}
	src += `    mah.inject("page_bottom", function(ctx)
        local out = {}
        for k, v in pairs(fires) do
            out[#out + 1] = k .. "=" .. tostring(v)
        end
        table.sort(out)
        return table.concat(out, ",")
    end)
end
`
	return src
}

// hookFires reads the plugin's Lua-side counters back. Rendering the slot is the
// only way in: the counters deliberately never touch the database.
func hookFires(t *testing.T, ctx *MahresourcesContext, prefix string) int {
	t.Helper()
	out := ctx.PluginManager().RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	for _, part := range strings.Split(out, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key != prefix {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("probe returned %q for %s: %v", value, prefix, err)
		}
		return n
	}
	return 0
}

func tagNames(t *testing.T, ctx *MahresourcesContext) []string {
	t.Helper()
	var tags []models.Tag
	if err := ctx.db.Find(&tags).Error; err != nil {
		t.Fatalf("load tags: %v", err)
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names
}

func countTagsWithPrefix(t *testing.T, ctx *MahresourcesContext, prefix string) int {
	t.Helper()
	n := 0
	for _, name := range tagNames(t, ctx) {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

func makeHookTestResource(t *testing.T, ctx *MahresourcesContext, name string) *models.Resource {
	t.Helper()
	res := &models.Resource{Name: name, Hash: "hash-" + name, Location: "loc-" + name}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource %s: %v", name, err)
	}
	return res
}

func makeHookTestNote(t *testing.T, ctx *MahresourcesContext, name string) *models.Note {
	t.Helper()
	note := &models.Note{Name: name}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create note %s: %v", name, err)
	}
	return note
}

func resourceExists(t *testing.T, ctx *MahresourcesContext, id uint) bool {
	t.Helper()
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count resource %d: %v", id, err)
	}
	return count > 0
}

// Bulk delete fired neither hook. A plugin that mirrors resources to an external
// system, or vetoes deletion of protected ones, worked for one resource and was
// silently bypassed for fifty.
func TestBulkDeleteResources_FiresBeforeAndAfterHooks(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"before_resource_delete": "before",
		"after_resource_delete":  "after",
	}))

	ids := []uint{
		makeHookTestResource(t, ctx, "r1").ID,
		makeHookTestResource(t, ctx, "r2").ID,
		makeHookTestResource(t, ctx, "r3").ID,
	}

	if err := ctx.BulkDeleteResources(&query_models.BulkQuery{ID: ids}); err != nil {
		t.Fatalf("BulkDeleteResources: %v", err)
	}

	if got := hookFires(t, ctx, "before"); got != 3 {
		t.Errorf("before_resource_delete fired %d times, want 3", got)
	}
	if got := hookFires(t, ctx, "after"); got != 3 {
		t.Errorf("after_resource_delete fired %d times, want 3", got)
	}
	// The after-hook's write landed, which it can only do outside a transaction.
	if got := countTagsWithPrefix(t, ctx, "after-"); got != 3 {
		t.Errorf("after_resource_delete wrote %d tags, want 3: an after-hook must run "+
			"after the commit, where its own writes can succeed (tags: %v)", got, tagNames(t, ctx))
	}
	for _, id := range ids {
		if resourceExists(t, ctx, id) {
			t.Errorf("resource %d survived the bulk delete", id)
		}
	}
}

// The veto has to cover the batch, not one row: a refusal that deleted the first
// two of three would be worse than no veto at all.
func TestBulkDeleteResources_VetoRollsBackWholeBatch(t *testing.T) {
	// The veto lands on the LAST resource on purpose. Vetoing the first proves
	// nothing about rollback — nothing had been deleted yet, so the batch would
	// survive even with no transaction at all.
	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "vetoes the last delete" }
local seen = 0
function init()
    mah.on("before_resource_delete", function(data)
        seen = seen + 1
        if seen == 3 then
            mah.abort("protected")
        end
        return data
    end)
end
`)

	ids := []uint{
		makeHookTestResource(t, ctx, "r1").ID,
		makeHookTestResource(t, ctx, "r2").ID,
		makeHookTestResource(t, ctx, "r3").ID,
	}

	if err := ctx.BulkDeleteResources(&query_models.BulkQuery{ID: ids}); err == nil {
		t.Fatal("BulkDeleteResources succeeded despite a before-hook veto")
	}

	for _, id := range ids {
		if !resourceExists(t, ctx, id) {
			t.Errorf("resource %d was deleted despite the veto: the first two deletions "+
				"must roll back when the third is refused", id)
		}
	}
}

// Merge routes through the same DB-only helper as bulk delete and fired no
// delete hooks for the losers it destroyed.
func TestMergeResources_FiresDeleteHooksForLosers(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"before_resource_delete": "before",
		"after_resource_delete":  "after",
	}))

	winner := makeHookTestResource(t, ctx, "winner")
	loserA := makeHookTestResource(t, ctx, "loserA")
	loserB := makeHookTestResource(t, ctx, "loserB")

	if err := ctx.MergeResources(winner.ID, []uint{loserA.ID, loserB.ID}, false); err != nil {
		t.Fatalf("MergeResources: %v", err)
	}

	if got := hookFires(t, ctx, "before"); got != 2 {
		t.Errorf("before_resource_delete fired %d times, want 2 (one per loser)", got)
	}
	if got := hookFires(t, ctx, "after"); got != 2 {
		t.Errorf("after_resource_delete fired %d times, want 2 (one per loser)", got)
	}
	if got := countTagsWithPrefix(t, ctx, "after-"); got != 2 {
		t.Errorf("after_resource_delete wrote %d tags, want 2 (must run after commit)", got)
	}
	if !resourceExists(t, ctx, winner.ID) {
		t.Error("winner was deleted")
	}
}

// A vetoed loser must roll the merge back whole: a merge that silently kept one
// loser alive leaves the winner holding half its associations.
func TestMergeResources_VetoRollsBackWholeMerge(t *testing.T) {
	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "vetoes deletes" }
function init()
    mah.on("before_resource_delete", function(data)
        mah.abort("protected")
        return data
    end)
end
`)

	winner := makeHookTestResource(t, ctx, "winner")
	loserA := makeHookTestResource(t, ctx, "loserA")
	loserB := makeHookTestResource(t, ctx, "loserB")

	if err := ctx.MergeResources(winner.ID, []uint{loserA.ID, loserB.ID}, false); err == nil {
		t.Fatal("MergeResources succeeded despite a before-hook veto")
	}

	for _, id := range []uint{winner.ID, loserA.ID, loserB.ID} {
		if !resourceExists(t, ctx, id) {
			t.Errorf("resource %d disappeared from a merge that was vetoed", id)
		}
	}
	// Row existence alone is weak: the merge also rewrites the winner's meta
	// with the losers' backups before deleting them, and that must roll back too.
	var reread models.Resource
	if err := ctx.db.First(&reread, winner.ID).Error; err != nil {
		t.Fatalf("re-read winner: %v", err)
	}
	if strings.Contains(string(reread.Meta), "backups") {
		t.Error("the winner kept the merge's backup meta after the merge was vetoed")
	}
}

// Notes and tags fired their after-hook from inside the still-open transaction,
// so a plugin was told an entity had been deleted before the commit that might
// roll it back.
func TestBulkDeleteNotes_RollbackFiresNoAfterHook(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"after_note_delete": "after",
	}))

	note := makeHookTestNote(t, ctx, "n1")

	// Positive control first: without it, a hook that never fires at all would
	// make the rollback assertion below pass for the wrong reason.
	if err := ctx.BulkDeleteNotes(&query_models.BulkQuery{ID: []uint{note.ID}}); err != nil {
		t.Fatalf("BulkDeleteNotes: %v", err)
	}
	if got := hookFires(t, ctx, "after"); got != 1 {
		t.Fatalf("control: after_note_delete fired %d times on a committed delete, want 1", got)
	}
	if got := countTagsWithPrefix(t, ctx, "after-"); got != 1 {
		t.Fatalf("control: after_note_delete wrote %d tags, want 1 (must run after commit)", got)
	}

	// Now the rollback: a second note plus an id that does not exist.
	survivor := makeHookTestNote(t, ctx, "n2")
	if err := ctx.BulkDeleteNotes(&query_models.BulkQuery{ID: []uint{survivor.ID, survivor.ID + 99999}}); err == nil {
		t.Fatal("BulkDeleteNotes with a missing id unexpectedly succeeded")
	}

	if got := hookFires(t, ctx, "after"); got != 1 {
		t.Errorf("after_note_delete fired %d times, want 1: a rolled-back delete must not "+
			"tell a plugin the note is gone", got)
	}
	var count int64
	if err := ctx.db.Model(&models.Note{}).Where("id = ?", survivor.ID).Count(&count).Error; err != nil {
		t.Fatalf("count note: %v", err)
	}
	if count == 0 {
		t.Error("the surviving note was deleted by a transaction that should have rolled back")
	}
}

// The re-entry guard, through the real pluginDBAdapter rather than a fake.
//
// A plugin that hooks after_tag_create and whose hook creates a tag re-enters
// its own VM one level down. Before the guard that was a permanent deadlock: the
// mutex is not reentrant, the 5s Lua timeout cannot preempt a block inside a Go
// call, and the lock is never released, so every later entry into that plugin —
// including page-wide injections — queued behind it forever.
//
// This is the path that proves the adapter carries the invocation into the CRUD
// chain and back out to the hook dispatcher, which the plugin_system tests
// exercise only against a fake.
func TestPluginHookReentry_ThroughRealAdapter(t *testing.T) {
	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "hooks its own writes" }
local fires = 0
function init()
    mah.on("after_tag_create", function(data)
        fires = fires + 1
        mah.db.create_tag({ name = "echo-" .. tostring(math.floor(data.id)) })
        return data
    end)
    mah.inject("page_bottom", function(ctx)
        return "after=" .. tostring(fires)
    end)
end
`)

	done := make(chan error, 1)
	go func() {
		_, err := ctx.CreateTag(&query_models.TagCreator{Name: "seed"})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("CreateTag did not return: the after_tag_create hook re-entered its own VM " +
			"and deadlocked on its own VM mutex")
	}

	// Fired once for the seed tag. The echo tag it created fired the event a
	// second time, and that dispatch was skipped because the plugin was still
	// running one frame up — so the recursion terminates instead of running away.
	if got := hookFires(t, ctx, "after"); got != 1 {
		t.Errorf("after_tag_create fired %d times, want 1", got)
	}
	if got := countTagsWithPrefix(t, ctx, "echo-"); got != 1 {
		t.Errorf("the hook wrote %d echo tags, want 1: the write it makes must still happen "+
			"(tags: %v)", got, tagNames(t, ctx))
	}
}

func TestBulkDeleteTags_RollbackFiresNoAfterHook(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"after_tag_delete": "deleted",
	}))

	victim := &models.Tag{Name: "victim"}
	if err := ctx.db.Create(victim).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	if err := ctx.BulkDeleteTags(&query_models.BulkQuery{ID: []uint{victim.ID}}); err != nil {
		t.Fatalf("BulkDeleteTags: %v", err)
	}
	if got := hookFires(t, ctx, "deleted"); got != 1 {
		t.Fatalf("control: after_tag_delete fired %d times on a committed delete, want 1", got)
	}
	if got := countTagsWithPrefix(t, ctx, "deleted-"); got != 1 {
		t.Fatalf("control: after_tag_delete wrote %d tags, want 1 (must run after commit)", got)
	}

	survivor := &models.Tag{Name: "survivor"}
	if err := ctx.db.Create(survivor).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := ctx.BulkDeleteTags(&query_models.BulkQuery{ID: []uint{survivor.ID, survivor.ID + 99999}}); err == nil {
		t.Fatal("BulkDeleteTags with a missing id unexpectedly succeeded")
	}
	if got := hookFires(t, ctx, "deleted"); got != 1 {
		t.Errorf("after_tag_delete fired %d times, want 1: a rolled-back delete must not "+
			"tell a plugin the tag is gone", got)
	}
	// Without this, dropping the transaction while still suppressing effects on
	// error would delete the tag and pass.
	var count int64
	if err := ctx.db.Model(&models.Tag{}).Where("id = ?", survivor.ID).Count(&count).Error; err != nil {
		t.Fatalf("count tag: %v", err)
	}
	if count == 0 {
		t.Error("the surviving tag was deleted by a transaction that should have rolled back")
	}
}

// MergeTags deleted its losers through the single-item DeleteTag from inside its
// own transaction, so it had the same mis-timed after-hook as bulk delete. It is
// not covered by the bulk tests because it is a different call path.
func TestMergeTags_FiresDeleteHooksAfterCommit(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"after_tag_delete": "deleted",
	}))

	winner := &models.Tag{Name: "winner"}
	loser := &models.Tag{Name: "loser"}
	for _, tag := range []*models.Tag{winner, loser} {
		if err := ctx.db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	if err := ctx.MergeTags(winner.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("MergeTags: %v", err)
	}

	if got := hookFires(t, ctx, "deleted"); got != 1 {
		t.Errorf("after_tag_delete fired %d times, want 1 (one per loser)", got)
	}
	// The hook's own write landed, which it can only do outside the transaction.
	if got := countTagsWithPrefix(t, ctx, "deleted-"); got != 1 {
		t.Errorf("after_tag_delete wrote %d tags, want 1: it must run after the commit", got)
	}
}

// A vetoed loser must roll the whole merge back, associations included.
func TestMergeTags_VetoRollsBackTheMerge(t *testing.T) {
	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "vetoes tag deletes" }
function init()
    mah.on("before_tag_delete", function(data)
        mah.abort("protected")
        return data
    end)
end
`)

	winner := &models.Tag{Name: "winner"}
	loser := &models.Tag{Name: "loser"}
	for _, tag := range []*models.Tag{winner, loser} {
		if err := ctx.db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	if err := ctx.MergeTags(winner.ID, []uint{loser.ID}); err == nil {
		t.Fatal("MergeTags succeeded despite a before-hook veto")
	}

	for _, id := range []uint{winner.ID, loser.ID} {
		var count int64
		if err := ctx.db.Model(&models.Tag{}).Where("id = ?", id).Count(&count).Error; err != nil {
			t.Fatalf("count tag: %v", err)
		}
		if count == 0 {
			t.Errorf("tag %d disappeared from a merge that was vetoed", id)
		}
	}
	// The backup blob the merge writes into the winner's meta must roll back too.
	var reread models.Tag
	if err := ctx.db.First(&reread, winner.ID).Error; err != nil {
		t.Fatalf("re-read winner: %v", err)
	}
	if strings.Contains(string(reread.Meta), "backups") {
		t.Error("the winner kept the merge's backup meta after the merge was vetoed")
	}
}

// Item 01's headline claim, end to end through the real adapter: a plugin write
// lands with the triggering user as its creator.
//
// Before this package every mah.db call ran on one adapter captured at wiring
// time whose context had no principal, so under -auth everything a plugin
// created was attributed to nobody. The assertion is against a re-read row, not
// against anything handed in: the stamp callback writes *through* the caller's
// pointer, so asserting on an in-memory value can pass while the column is NULL.
func TestPluginWrite_IsAttributedToTriggeringUser(t *testing.T) {
	const actor = 4242

	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "writes a tag" }
function init()
    mah.inject("page_bottom", function(c)
        mah.db.create_tag({ name = "made-by-plugin" })
        return "ok"
    end)
end
`)

	reqCtx := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: actor})
	if out := ctx.PluginManager().RenderSlot(reqCtx, "page_bottom", map[string]any{}, nil); out != "ok" {
		t.Fatalf("slot output = %q, want %q", out, "ok")
	}

	var tag models.Tag
	if err := ctx.db.Where("name = ?", "made-by-plugin").First(&tag).Error; err != nil {
		t.Fatalf("the plugin write did not land: %v", err)
	}
	if tag.CreatedByUserId == nil {
		t.Fatal("CreatedByUserId is NULL: the plugin write was attributed to nobody")
	}
	if *tag.CreatedByUserId != actor {
		t.Errorf("CreatedByUserId = %d, want %d", *tag.CreatedByUserId, actor)
	}
}

// The controls for the test above. Each asserts the *exact* creator rather than
// merely "not the other test's actor" — a stamp that hardcoded any non-nil value,
// or that leaked an actor across requests, would slip past a not-equal check.
//
// The auth-disabled system principal is the interesting one. It is a real
// principal with a real user id (root's), but recording it as the actor would
// claim a specific human made the change when with auth off every request is the
// same implicit administrator. It must behave exactly like no principal at all,
// which is also what keeps a plugin job ownerless under auth-off, matching an
// async action submitted through the API.
func TestPluginWrite_NoActorLeavesTheStampToTheDefault(t *testing.T) {
	direct := func(t *testing.T, ctx *MahresourcesContext) *uint {
		t.Helper()
		// What an ordinary, non-plugin create produces in this same context, so
		// the assertion is "the plugin path behaves like every other path"
		// rather than a hardcoded expectation about root resolution.
		tag := &models.Tag{Name: "made-directly"}
		if err := ctx.db.Create(tag).Error; err != nil {
			t.Fatalf("direct create: %v", err)
		}
		var reread models.Tag
		if err := ctx.db.Where("name = ?", "made-directly").First(&reread).Error; err != nil {
			t.Fatalf("re-read direct tag: %v", err)
		}
		return reread.CreatedByUserId
	}

	cases := []struct {
		name string
		ctx  func() context.Context
	}{
		{"no principal", func() context.Context { return context.Background() }},
		{"auth-disabled system principal", func() context.Context {
			return auth.WithPrincipal(context.Background(), &auth.Principal{SuperUser: true, UserID: 99})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "writes a tag" }
function init()
    mah.inject("page_bottom", function(c)
        mah.db.create_tag({ name = "made-by-plugin" })
        return "ok"
    end)
end
`)
			want := direct(t, ctx)

			if out := ctx.PluginManager().RenderSlot(tc.ctx(), "page_bottom", map[string]any{}, nil); out != "ok" {
				t.Fatalf("slot output = %q", out)
			}

			var tag models.Tag
			if err := ctx.db.Where("name = ?", "made-by-plugin").First(&tag).Error; err != nil {
				t.Fatalf("the plugin write did not land: %v", err)
			}
			switch {
			case want == nil && tag.CreatedByUserId != nil:
				t.Errorf("CreatedByUserId = %d, want NULL to match a direct create", *tag.CreatedByUserId)
			case want != nil && tag.CreatedByUserId == nil:
				t.Errorf("CreatedByUserId = NULL, want %d to match a direct create", *want)
			case want != nil && *tag.CreatedByUserId != *want:
				t.Errorf("CreatedByUserId = %d, want %d to match a direct create", *tag.CreatedByUserId, *want)
			}
		})
	}
}

// An after-delete hook must run once the delete is *fully* done, files included.
//
// The ShouldRemoveSource decision is taken inside the transaction, from a hash
// reference count. If the hook runs before the file phase and stores content,
// the file it writes can be removed by that stale decision. Single-item
// DeleteResource has always fired its hook after the file work; the bulk paths
// are now consistent with it.
//
// The probe is a filesystem timeline rather than anything in the database: by
// the time any hook runs the row is already gone, so a database-shaped probe
// reports "deleted" whichever order the two phases run in — which is exactly how
// the first version of this test passed against the unfixed code.
type fsOrderRecorder struct {
	afero.Fs
	mu       sync.Mutex
	timeline []string
}

func (r *fsOrderRecorder) note(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeline = append(r.timeline, event)
}

func (r *fsOrderRecorder) Remove(name string) error {
	r.note("remove:" + name)
	return r.Fs.Remove(name)
}

func (r *fsOrderRecorder) Create(name string) (afero.File, error) {
	// The backup copy the delete itself writes lives under /deleted/ and is part
	// of the file phase, not of anything a hook did.
	if !strings.HasPrefix(name, "/deleted/") {
		r.note("create:" + name)
	}
	return r.Fs.Create(name)
}

func (r *fsOrderRecorder) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.timeline...)
}

func TestBulkDeleteResources_AfterHookRunsAfterFileCleanup(t *testing.T) {
	ctx := newPluginHookTestContext(t, `
plugin = { name = "hooktest", version = "1.0", description = "writes from its after-hook" }
function init()
    mah.on("after_resource_delete", function(data)
        mah.db.create_resource_from_data("cGF5bG9hZC10d28=", { name = "from-hook" })
        return data
    end)
end
`)

	res := makeHookTestResource(t, ctx, "r1")
	if err := afero.WriteFile(ctx.fs, res.GetCleanLocation(), []byte("payload"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	// Wrap the filesystem only now, so seeding does not appear on the timeline.
	rec := &fsOrderRecorder{Fs: ctx.fs}
	ctx.fs = rec

	if err := ctx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{res.ID}}); err != nil {
		t.Fatalf("BulkDeleteResources: %v", err)
	}

	events := rec.events()
	removeAt, createAt := -1, -1
	for i, e := range events {
		if strings.HasPrefix(e, "remove:") && removeAt == -1 {
			removeAt = i
		}
		if strings.HasPrefix(e, "create:") && createAt == -1 {
			createAt = i
		}
	}
	if removeAt == -1 {
		t.Fatalf("the deleted resource's file was never removed (timeline: %v)", events)
	}
	if createAt == -1 {
		t.Fatalf("the after-hook never wrote anything, so this proves nothing (timeline: %v)", events)
	}
	if createAt < removeAt {
		t.Errorf("the after-hook wrote at %d, before the file phase removed at %d (timeline: %v): "+
			"a hook that stores content can have its own file removed by the stale "+
			"ShouldRemoveSource decision", createAt, removeAt, events)
	}
}
