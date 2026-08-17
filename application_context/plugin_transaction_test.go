package application_context

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

// mah.db.transaction end-to-end, against a real PluginManager and a real
// database, because the thing being built is precisely a chain: Lua callback →
// Invocation → host adapter → *gorm.DB. A fake at any link would let the test
// pass while the transaction never reached the writes.

// newTwoPluginContext loads two plugins, which is what the interesting case
// needs: plugin A opens the transaction, A's write fires B's hook, and B's
// writes have to land in A's transaction. One plugin cannot show this — the
// re-entry guard deliberately does not notify a plugin of its own writes.
func newTwoPluginContext(t *testing.T, sources map[string]string) *MahresourcesContext {
	t.Helper()

	pluginDir := t.TempDir()
	for name, src := range sources {
		if err := os.MkdirAll(filepath.Join(pluginDir, name), 0o755); err != nil {
			t.Fatalf("mkdir plugin %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, name, "plugin.lua"), []byte(src), 0o644); err != nil {
			t.Fatalf("write plugin %s: %v", name, err)
		}
	}

	ctx := createTestContextWithPlugins(t, pluginDir)
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("plugin manager was not wired: the test context has no plugin path")
	}
	for name := range sources {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin %s: %v", name, err)
		}
	}
	t.Cleanup(pm.Close)
	return ctx
}

// runSlot renders the "run" slot, which is how these plugins are triggered. The
// slot's return value is the plugin's own report of what happened.
func runSlot(ctx *MahresourcesContext, slot string) string {
	return ctx.PluginManager().RenderSlot(context.Background(), slot, map[string]any{}, nil)
}

func countRows(t *testing.T, ctx *MahresourcesContext, model any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := ctx.db.Model(model)
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Count(&n).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	return n
}

// writerPlugin creates a group, a note in it and a tag inside one transaction,
// then optionally fails. The failure is a Lua error raised *after* three
// successful writes, which is the shape the whole feature exists for: without a
// transaction the group and note stay behind.
func writerPlugin(name string, fail bool) string {
	failLine := ""
	if fail {
		failLine = `        error("changed my mind")`
	}
	return `plugin = { name = "` + name + `", version = "1.0", description = "writes several things at once" }
local report = "not run"
function init()
    mah.inject("run", function(ctx)
        local ok, err = mah.db.transaction(function()
            local g = mah.db.create_group({ name = "tx-group" })
            mah.db.create_note({ name = "tx-note", owner_id = g.id })
            mah.db.create_tag({ name = "tx-tag" })
` + failLine + `
        end)
        if ok then
            report = "committed"
        else
            report = "rolled back: " .. tostring(err)
        end
        return report
    end)
end
`
}

func TestPluginTransaction_CommitsEveryWriteTogether(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"writer": writerPlugin("writer", false)})

	if got := runSlot(ctx, "run"); !strings.Contains(got, "committed") {
		t.Fatalf("slot reported %q, want a commit", got)
	}

	if n := countRows(t, ctx, &models.Group{}, "name = ?", "tx-group"); n != 1 {
		t.Errorf("groups named tx-group = %d, want 1", n)
	}
	if n := countRows(t, ctx, &models.Note{}, "name = ?", "tx-note"); n != 1 {
		t.Errorf("notes named tx-note = %d, want 1", n)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "tx-tag"); n != 1 {
		t.Errorf("tags named tx-tag = %d, want 1", n)
	}
}

func TestPluginTransaction_RollsBackEveryWriteTogether(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"writer": writerPlugin("writer", true)})

	got := runSlot(ctx, "run")
	if !strings.Contains(got, "rolled back") {
		t.Fatalf("slot reported %q, want a rollback", got)
	}
	if !strings.Contains(got, "changed my mind") {
		t.Errorf("slot reported %q, which does not carry the Lua error that caused the rollback", got)
	}

	for _, tc := range []struct {
		model any
		name  string
	}{
		{&models.Group{}, "tx-group"},
		{&models.Note{}, "tx-note"},
		{&models.Tag{}, "tx-tag"},
	} {
		if n := countRows(t, ctx, tc.model, "name = ?", tc.name); n != 0 {
			t.Errorf("%T named %s survived the rollback (%d rows): the writes were not one transaction",
				tc.model, tc.name, n)
		}
	}
}

// hookWriterPlugin is the second plugin. It writes a tag from a *before*-hook
// and another from an *after*-hook, which are the two halves of the design and
// behave differently on purpose:
//
//   - before_note_create fires inside the triggering plugin's open transaction,
//     so its write has to join that transaction. The Invocation is the only
//     channel that reaches this plugin's LState.
//   - after_note_create is deferred to the commit, so it never runs inside the
//     transaction at all.
const hookWriterPlugin = `plugin = { name = "hookwriter", version = "1.0", description = "writes from hooks" }
local fired = 0
function init()
    mah.on("before_note_create", function(data)
        mah.db.create_tag({ name = "before-tag" })
        return data
    end)
    mah.on("after_note_create", function(data)
        fired = fired + 1
        mah.db.create_tag({ name = "after-tag" })
        return data
    end)
    mah.inject("fired", function(ctx) return tostring(fired) end)
end
`

// The load-bearing case. Plugin A's write inside its transaction fires plugin
// B's before-hook, and B's own write must land in A's transaction: committed
// with it, and gone if it rolls back.
//
// Without the binding on the Invocation, B's mah.db call rebinds off the
// singleton adapter and asks the pool for a second connection while A's
// transaction holds the first — which on a real SQLite deployment blocks on the
// writer lock until busy_timeout, and here writes somewhere A's rollback cannot
// reach.
func TestPluginTransaction_BeforeHookWritesJoinTheTransaction(t *testing.T) {
	t.Run("committed with it", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{
			"writer":     writerPlugin("writer", false),
			"hookwriter": hookWriterPlugin,
		})
		if got := runSlot(ctx, "run"); !strings.Contains(got, "committed") {
			t.Fatalf("slot reported %q, want a commit", got)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "before-tag"); n != 1 {
			t.Errorf("before-tag rows = %d, want 1: the hook's write did not reach the "+
				"transaction's own handle", n)
		}
	})

	t.Run("rolled back with it", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{
			"writer":     writerPlugin("writer", true),
			"hookwriter": hookWriterPlugin,
		})
		if got := runSlot(ctx, "run"); !strings.Contains(got, "rolled back") {
			t.Fatalf("slot reported %q, want a rollback", got)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "before-tag"); n != 0 {
			t.Errorf("the before-hook's tag survived the rollback (%d rows): it wrote outside "+
				"the transaction", n)
		}
	})
}

// The other half: after-hooks must not fire at all while the transaction is
// open, because an after-hook says a write happened and it has not committed.
// On rollback they are dropped; on commit they run.
func TestPluginTransaction_AfterHooksWaitForTheCommit(t *testing.T) {
	t.Run("rollback drops them", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{
			"writer":     writerPlugin("writer", true),
			"hookwriter": hookWriterPlugin,
		})
		runSlot(ctx, "run")
		if got := runSlot(ctx, "fired"); got != "0" {
			t.Errorf("after_note_create fired %s time(s) for a note that was rolled back; "+
				"a plugin was told about a write that never happened", got)
		}
	})

	t.Run("commit runs them", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{
			"writer":     writerPlugin("writer", false),
			"hookwriter": hookWriterPlugin,
		})
		runSlot(ctx, "run")
		if got := runSlot(ctx, "fired"); got != "1" {
			t.Errorf("after_note_create fired %s time(s) after a committed note, want 1: "+
				"deferring the hook lost it instead of delaying it", got)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "after-tag"); n != 1 {
			t.Errorf("after-tag rows = %d, want 1: the deferred hook ran but its write did not land", n)
		}
	})
}

// The re-entry guard must keep working inside a transaction.
//
// A plugin is deliberately not notified of writes it makes while it is already
// running: hooks are dispatched synchronously on the calling goroutine, so
// going back for a VM mutex that goroutine already holds blocks until
// hookLockWait and then fails the write. The guard reads the call chain off the
// invocation the write was dispatched with — so if a transaction hands every
// nested call the chain it was *opened* with, the nested plugin is missing from
// it and the guard stops seeing itself.
//
// The plugin below writes a tag from its own before_note_create hook, and the
// tag write fires before_tag_create, which it also hooks. Inside a transaction
// opened by a different plugin, that second dispatch is the one that must still
// be skipped.
func TestPluginTransaction_ReentryGuardSurvivesIt(t *testing.T) {
	// The hook records the names it is told about, not a count: the triggering
	// plugin creates a tag of its own, and that dispatch is legitimate — this
	// plugin is not running when it happens. Only "guard-tag", the tag this
	// plugin writes from inside its own hook, must never come back.
	const selfHooking = `plugin = { name = "selfhook", version = "1.0", description = "hooks its own writes" }
local toldAbout = ""
function init()
    mah.on("before_note_create", function(data)
        mah.db.create_tag({ name = "guard-tag" })
        return data
    end)
    mah.on("before_tag_create", function(data)
        toldAbout = toldAbout .. tostring(data.name) .. ","
        return data
    end)
    mah.inject("toldAbout", function(ctx) return toldAbout end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{
		"writer":   writerPlugin("writer", false),
		"selfhook": selfHooking,
	})

	start := time.Now()
	got := runSlot(ctx, "run")
	elapsed := time.Since(start)

	if !strings.Contains(got, "committed") {
		t.Fatalf("slot reported %q, want a commit: the re-entrant hook dispatch failed the write", got)
	}
	told := runSlot(ctx, "toldAbout")
	if !strings.Contains(told, "tx-tag") {
		t.Fatalf("before_tag_create was told about %q, which does not include the triggering "+
			"plugin's own tag — the hook is not firing at all, so this proves nothing", told)
	}
	if strings.Contains(told, "guard-tag") {
		t.Errorf("before_tag_create was told about %q: the plugin was notified of a write it made "+
			"while it was already running, so the transaction handed the nested call a stale "+
			"call chain", told)
	}
	// The failure mode is a lock wait, not just a wrong count: the guard not
	// firing means a blocking TryLockVMWithin for hookLockWait (5s) with the
	// database write lock held.
	if elapsed > 3*time.Second {
		t.Errorf("the transaction took %s: a re-entrant hook dispatch blocked on a VM mutex "+
			"this goroutine already held", elapsed)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "guard-tag"); n != 1 {
		t.Errorf("guard-tag rows = %d, want 1", n)
	}
}

// Opening a transaction must not widen what the plugin can see or lose who it
// is acting as. Both ride on the *gorm.DB handle, and WithPrincipal rewrites
// that handle — so binding the principal after the transaction would swap the
// transactional handle back out, and binding neither would run the whole
// transaction unscoped and unattributed.
func TestPluginTransaction_KeepsTheCallersScopeAndActor(t *testing.T) {
	const src = `plugin = { name = "scoped", version = "1.0", description = "opens a transaction from a hook" }
local seen = ""
function init()
    mah.on("after_note_create", function(data)
        mah.db.transaction(function()
            local names = {}
            for _, row in ipairs(mah.db.query_groups({ limit = 100 }) or {}) do
                names[#names + 1] = row.name
            end
            table.sort(names)
            seen = table.concat(names, "|")
            mah.db.create_tag({ name = "tx-tag" })
        end)
        return data
    end)
    mah.inject("seen", function(ctx) return seen end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{"scoped": src})
	principal, inside := scopeProbeFixture(t, ctx)

	scoped := ctx.WithPrincipal(principal)
	if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: inside.ID},
	}); err != nil {
		t.Fatalf("confined user could not create a note in its own subtree: %v", err)
	}

	seen := runSlot(ctx, "seen")
	if seen == "" {
		t.Fatal("the hook's transaction saw no groups at all, so this proves nothing about scope")
	}
	if strings.Contains(seen, "outside") {
		t.Errorf("a transaction opened by a principal confined to %q read %q: the subtree filter "+
			"was dropped when the transaction handle was installed", inside.Name, seen)
	}

	var creators []*uint
	if err := ctx.db.Model(&models.Tag{}).Where("name = ?", "tx-tag").
		Pluck("created_by_user_id", &creators).Error; err != nil {
		t.Fatalf("read tx-tag creator: %v", err)
	}
	if len(creators) != 1 {
		t.Fatalf("tx-tag rows = %d, want 1: the transaction's write did not land", len(creators))
	}
	if creators[0] == nil {
		t.Fatal("a tag written inside a plugin transaction has a NULL creator: the actor was " +
			"lost when the transaction handle was installed")
	}
	if want := principal.UserID; *creators[0] != want {
		t.Errorf("tx-tag created_by_user_id = %d, want %d", *creators[0], want)
	}
}

// mah.kv and mah.log write to the same database as everything else, and neither
// goes through BindInvocation, so before this change both reached for the
// singleton's handle — a second connection, taken while the transaction holds
// the first.
//
// The commit case is the one that catches it. A write that cannot get a
// connection fails, so the rollback case looks correct however the plumbing is
// wired; only "the value written inside a committed transaction is there
// afterwards" distinguishes a joined write from an escaped one.
func kvPlugin(fail bool) string {
	failLine := ""
	if fail {
		failLine = `            error("no")`
	}
	return `plugin = { name = "kvwriter", version = "1.0", description = "stores a cursor" }
function init()
    mah.inject("run", function(ctx)
        mah.kv.set("cursor", "before")
        mah.db.transaction(function()
            mah.kv.set("cursor", "after")
            mah.log("info", "inside the transaction")
` + failLine + `
        end)
        return tostring(mah.kv.get("cursor"))
    end)
end
`
}

func TestPluginTransaction_StoredDataAndLogsJoinIt(t *testing.T) {
	t.Run("committed with it", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{"kvwriter": kvPlugin(false)})
		if got := runSlot(ctx, "run"); got != "after" {
			t.Errorf("mah.kv.get after a committed transaction = %q, want %q: the kv write "+
				"did not reach the transaction's handle", got, "after")
		}
		if n := countRows(t, ctx, &models.LogEntry{}, "message = ?", "inside the transaction"); n != 1 {
			t.Errorf("log rows = %d, want 1: mah.log inside a transaction did not reach its handle", n)
		}
	})

	t.Run("rolled back with it", func(t *testing.T) {
		ctx := newTwoPluginContext(t, map[string]string{"kvwriter": kvPlugin(true)})
		if got := runSlot(ctx, "run"); got != "before" {
			t.Errorf("mah.kv.get after a rolled-back transaction = %q, want %q", got, "before")
		}
		if n := countRows(t, ctx, &models.LogEntry{}, "message = ?", "inside the transaction"); n != 0 {
			t.Errorf("log rows = %d, want 0: the log line survived the rollback", n)
		}
	})
}

// Everything that waits on the network or the filesystem is refused, by name,
// with the reason. The rule is one sentence and the test holds it to it.
const refusalPlugin = `plugin = { name = "refused", version = "1.0", description = "tries what it must not" }
local report = "not run"
function init()
    mah.inject("run", function(ctx)
        local out = {}
        mah.db.transaction(function()
            local _, err = mah.db.create_resource_from_data("aGk=", { name = "x" })
            out[#out + 1] = "data:" .. tostring(err)

            local resp = mah.http.get_sync("https://example.com/")
            out[#out + 1] = "http:" .. tostring(resp.error)

            local ok, serr = pcall(function() mah.sleep(1) end)
            out[#out + 1] = "sleep:" .. tostring(ok) .. ":" .. tostring(serr)

            local _, derr = mah.db.delete_resource(1)
            out[#out + 1] = "delete:" .. tostring(derr)

            local _, _, gerr = mah.db.get_resource_data(1)
            out[#out + 1] = "getdata:" .. tostring(gerr)
        end)
        report = table.concat(out, " ")
        return report
    end)
end
`

func TestPluginTransaction_RefusesCallsThatWaitOnIO(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"refused": refusalPlugin})

	got := runSlot(ctx, "run")
	for _, want := range []string{
		"mah.db.create_resource_from_data cannot run inside mah.db.transaction",
		"mah.http.get_sync cannot run inside mah.db.transaction",
		"mah.sleep cannot run inside mah.db.transaction",
		// A different reason from the other four, and the message has to say so:
		// this one is refused because the filesystem has no rollback.
		"mah.db.delete_resource cannot run inside mah.db.transaction",
		"restores the row but not the bytes",
		// A read, refused for the same reason as the writes that wait: it pulls
		// bytes off a filesystem that may be remote, under the write lock.
		"mah.db.get_resource_data cannot run inside mah.db.transaction",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("slot reported %q, which does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "sleep:true") {
		t.Errorf("mah.sleep returned normally inside a transaction; it must raise: %q", got)
	}
	// Nothing was created despite the attempt.
	if n := countRows(t, ctx, &models.Resource{}, ""); n != 0 {
		t.Errorf("resources = %d, want 0: a refused call created one anyway", n)
	}
}

// The asynchronous HTTP calls are refused for the other reason: they hold no
// lock, but the request is on the wire the moment it is made and a rollback
// cannot recall a POST to a webhook.
//
// The refusal RAISES rather than arriving through the callback, unlike every
// other refusal on that surface, and that is the point of this test. The
// callback is queued for the drain goroutine, which runs it after the calling
// frame has returned and released its VM lock while the transaction is still
// open — so a mah.db write from a queued callback would run outside the
// transaction. Answering the refusal through that channel would build the exact
// escape the refusal exists to prevent.
func TestPluginTransaction_RefusesAsyncHttpByRaising(t *testing.T) {
	const src = `plugin = { name = "asynchttp", version = "1.0", description = "fires a webhook inside a transaction" }
local callbackRan = false
function init()
    mah.inject("run", function(ctx)
        local ok, err = mah.db.transaction(function()
            mah.db.create_tag({ name = "before-the-webhook" })
            mah.http.post("https://example.com/webhook", "{}", function(resp)
                callbackRan = true
            end)
        end)
        return tostring(ok) .. ":" .. tostring(err)
    end)
    mah.inject("callbackRan", function(ctx) return tostring(callbackRan) end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{"asynchttp": src})

	got := runSlot(ctx, "run")
	if !strings.Contains(got, "mah.http.post cannot run inside mah.db.transaction") {
		t.Errorf("slot reported %q, want the raised refusal", got)
	}
	if !strings.Contains(got, "a rollback cannot recall it") {
		t.Errorf("slot reported %q, which does not give the reason", got)
	}
	if strings.HasPrefix(got, "true") {
		t.Errorf("slot reported %q: the transaction committed despite the refusal", got)
	}

	// Nothing may be queued for the drain goroutine, because a callback that ran
	// there could write outside the still-open transaction.
	for i := 0; i < 25; i++ {
		if ran := runSlot(ctx, "callbackRan"); ran != "false" {
			t.Fatalf("the async callback ran (%q): the refusal was queued rather than raised", ran)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "before-the-webhook"); n != 0 {
		t.Errorf("before-the-webhook rows = %d, want 0: the raised refusal did not roll the "+
			"transaction back", n)
	}
}

// The same calls must still work outside a transaction — a refusal that leaked
// out of the transaction would break every plugin that fetches.
func TestPluginTransaction_RefusalsDoNotLeakOutside(t *testing.T) {
	const src = `plugin = { name = "outside", version = "1.0", description = "sleeps outside a transaction" }
local report = "not run"
function init()
    mah.inject("run", function(ctx)
        local ok, err = pcall(function() mah.sleep(0) end)
        report = tostring(ok) .. ":" .. tostring(err)
        return report
    end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{"outside": src})
	if got := runSlot(ctx, "run"); !strings.HasPrefix(got, "true") {
		t.Errorf("mah.sleep outside a transaction reported %q, want success", got)
	}
}

// Deferring an after-hook must delay it, not re-address it.
//
// A plugin is never notified of a write it made while it was already running.
// Inside a transaction the notification is queued and dispatched later, and by
// then the plugin is no longer running — but the write was still made while it
// was, so the rule has to hold. What decides it is the call chain the hook was
// *raised* with, so the queue has to carry it; draining everything through the
// opener's chain tells the nested plugin about its own write.
func TestPluginTransaction_DeferredHooksKeepTheChainTheyWereRaisedWith(t *testing.T) {
	const nested = `plugin = { name = "nested", version = "1.0", description = "writes from a hook and hooks that write" }
local ownWrite = 0
function init()
    mah.on("before_note_create", function(data)
        mah.db.create_tag({ name = "nested-tag" })
        return data
    end)
    mah.on("after_tag_create", function(data)
        if tostring(data.name) == "nested-tag" then
            ownWrite = ownWrite + 1
        end
        return data
    end)
    mah.inject("ownWrite", function(ctx) return tostring(ownWrite) end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{
		"writer": writerPlugin("writer", false),
		"nested": nested,
	})

	if got := runSlot(ctx, "run"); !strings.Contains(got, "committed") {
		t.Fatalf("slot reported %q, want a commit", got)
	}
	if got := runSlot(ctx, "ownWrite"); got != "0" {
		t.Errorf("the nested plugin was told about its own write %s time(s), want 0: the "+
			"deferred hook was dispatched with the opener's call chain instead of the one it "+
			"was raised with", got)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "nested-tag"); n != 1 {
		t.Errorf("nested-tag rows = %d, want 1", n)
	}
}

// A coroutine cannot yield across a Go call boundary, and the transaction's
// callback is invoked from Go. Left open, a yield inside the callback abandoned
// the frame silently — the whole render came back empty with no error anywhere.
// The refusal has to be the visible kind, and the VM has to survive it.
func TestPluginTransaction_RefusedFromACoroutine(t *testing.T) {
	const src = `plugin = { name = "coro", version = "1.0", description = "yields inside a transaction" }
function init()
    mah.inject("run", function(ctx)
        local co = coroutine.create(function()
            local ok, err = mah.db.transaction(function()
                mah.db.create_tag({ name = "coro-tag" })
                coroutine.yield("suspended")
            end)
            return "tx:" .. tostring(ok) .. ":" .. tostring(err)
        end)
        local resumed, value = coroutine.resume(co)
        return "resumed=" .. tostring(resumed) .. " value=" .. tostring(value)
    end)
    mah.inject("alive", function(ctx) return "still here" end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{"coro": src})

	got := runSlot(ctx, "run")
	if !strings.Contains(got, "cannot be called from a coroutine") {
		t.Errorf("slot reported %q, want the coroutine refusal", got)
	}
	if !strings.Contains(got, "resumed=true") {
		t.Errorf("slot reported %q: the coroutine did not run to completion", got)
	}
	if alive := runSlot(ctx, "alive"); alive != "still here" {
		t.Errorf("a later render returned %q: the refused call left the VM unusable", alive)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "coro-tag"); n != 0 {
		t.Errorf("coro-tag rows = %d, want 0: the refused transaction wrote anyway", n)
	}
}

// mah.abort inside a transaction must still abort rather than being flattened
// into an ordinary error, or a before-hook's veto would reach RunBeforeHooks
// looking like a database failure.
func TestPluginTransaction_AbortStaysAnAbort(t *testing.T) {
	const src = `plugin = { name = "aborter", version = "1.0", description = "aborts inside a transaction" }
local report = "not run"
function init()
    mah.inject("run", function(ctx)
        local ok, err = pcall(function()
            mah.db.transaction(function()
                mah.db.create_tag({ name = "doomed-tag" })
                mah.abort("nope")
            end)
        end)
        report = tostring(ok) .. ":" .. tostring(err)
        return report
    end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{"aborter": src})

	got := runSlot(ctx, "run")
	if strings.HasPrefix(got, "true") {
		t.Errorf("mah.abort inside a transaction did not raise: %q", got)
	}
	if !strings.Contains(got, "PLUGIN_ABORT: nope") {
		t.Errorf("slot reported %q, which does not carry the abort marker; the veto was flattened "+
			"into an ordinary error", got)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "doomed-tag"); n != 0 {
		t.Errorf("the aborted transaction committed its tag (%d rows)", n)
	}
}

// A transaction opened inside a transaction becomes a savepoint on it: the
// inner one can fail on its own without taking the outer down, and the outer
// failing still takes everything.
func TestPluginTransaction_NestedTransactionIsASavepoint(t *testing.T) {
	t.Run("inner failure rolls back only the inner writes", func(t *testing.T) {
		const src = `plugin = { name = "nester", version = "1.0", description = "nests transactions" }
function init()
    mah.inject("run", function(ctx)
        local ok, err = mah.db.transaction(function()
            mah.db.create_tag({ name = "outer-tag" })
            local innerOk = mah.db.transaction(function()
                mah.db.create_tag({ name = "inner-tag" })
                error("inner fails")
            end)
            if innerOk then error("inner transaction reported success after erroring") end
        end)
        return tostring(ok) .. ":" .. tostring(err)
    end)
end
`
		ctx := newTwoPluginContext(t, map[string]string{"nester": src})

		got := runSlot(ctx, "run")
		if !strings.HasPrefix(got, "true") {
			t.Fatalf("the outer transaction did not commit: %q", got)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "outer-tag"); n != 1 {
			t.Errorf("outer-tag rows = %d, want 1: the inner failure took the outer down with it", n)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "inner-tag"); n != 0 {
			t.Errorf("inner-tag rows = %d, want 0: the inner transaction did not roll back", n)
		}
	})

	t.Run("outer failure takes the committed inner with it", func(t *testing.T) {
		const src = `plugin = { name = "nester", version = "1.0", description = "nests transactions" }
function init()
    mah.inject("run", function(ctx)
        local ok, err = mah.db.transaction(function()
            local innerOk = mah.db.transaction(function()
                mah.db.create_tag({ name = "inner-tag" })
            end)
            if not innerOk then error("inner transaction refused") end
            error("outer fails")
        end)
        return tostring(ok) .. ":" .. tostring(err)
    end)
end
`
		ctx := newTwoPluginContext(t, map[string]string{"nester": src})

		got := runSlot(ctx, "run")
		if strings.Contains(got, "inner transaction refused") {
			t.Fatalf("the nested transaction was refused: %q", got)
		}
		if n := countRows(t, ctx, &models.Tag{}, "name = ?", "inner-tag"); n != 0 {
			t.Errorf("inner-tag survived the outer rollback (%d rows): the savepoint was released "+
				"as a real commit", n)
		}
	})
}

// The same thing across plugins, which is where it actually bites: a
// before-hook runs inside the *triggering* plugin's transaction, so a plugin
// wrapping its own work in mah.db.transaction is nesting without knowing it.
// Its savepoint must not be able to discard the caller's writes.
func TestPluginTransaction_NestedFromAnotherPluginsHook(t *testing.T) {
	const hookNester = `plugin = { name = "hooknester", version = "1.0", description = "opens its own transaction from a hook" }
function init()
    mah.on("before_note_create", function(data)
        mah.db.transaction(function()
            mah.db.create_tag({ name = "hook-inner-tag" })
            error("the hook's own work fails")
        end)
        return data
    end)
end
`
	ctx := newTwoPluginContext(t, map[string]string{
		"writer":     writerPlugin("writer", false),
		"hooknester": hookNester,
	})

	if got := runSlot(ctx, "run"); !strings.Contains(got, "committed") {
		t.Fatalf("slot reported %q: the hook's failed inner transaction took the caller's down", got)
	}
	if n := countRows(t, ctx, &models.Group{}, "name = ?", "tx-group"); n != 1 {
		t.Errorf("tx-group rows = %d, want 1: a hook's rolled-back savepoint discarded the "+
			"triggering plugin's writes", n)
	}
	if n := countRows(t, ctx, &models.Tag{}, "name = ?", "hook-inner-tag"); n != 0 {
		t.Errorf("hook-inner-tag rows = %d, want 0: the hook's own rollback did not take effect", n)
	}
}
