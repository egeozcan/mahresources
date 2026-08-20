package application_context

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
)

// awaitEnableInFlight blocks until the manager reports an enable of this plugin
// somewhere between claiming the name and publishing it.
//
// It is the barrier these tests need, and it replaces a fixed sleep. A sleep
// asserts nothing: if the first operation finishes before the second starts, the
// two never overlap, both fail independently, the final state is the one the
// test expects anyway, and the test passes while exercising none of what it
// describes. Waiting on the manager's own marker means the overlap either
// happened or the test says so.
func awaitEnableInFlight(t *testing.T, ctx *MahresourcesContext, pluginName string, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ctx.pluginManager.EnableInFlight(pluginName) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// assertStateAgrees is the whole invariant of settle-time reconciliation, and
// the only assertion these tests need: when everything has finished, the row and
// the process say the same thing about whether this plugin is running.
//
// Asserting a specific boolean instead would be asserting a race's winner. Which
// of two concurrent operators gets their way is not defined and need not be; a
// row that contradicts the process is defined, and is always wrong — the next
// restart either drops a running plugin along with its schedules, or loads one
// nobody successfully enabled.
func assertStateAgrees(t *testing.T, ctx *MahresourcesContext, pluginName string) {
	t.Helper()
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		t.Fatalf("read plugin state: %v", err)
	}
	if running := ctx.pluginManager.IsEnabled(pluginName); running != state.Enabled {
		t.Fatalf("the process and the database disagree: loaded=%v, row.enabled=%v", running, state.Enabled)
	}
}

// A disable lands while an enable is loading, and that enable then fails.
//
// Every local rule for undoing a write breaks on one ordering of this. An
// unconditional restore (the disable putting its own write back) republishes
// "enabled" on top of the failed enable's revert if the revert lands first. A
// restore conditional on the disable's own write breaks the other ordering: the
// enable fails *after* the restore, and its undo no longer matches, so the row
// keeps saying enabled for a plugin that never loaded. The two orderings differ
// by microseconds and neither caller can tell which it is in.
//
// Nobody undoes. Whoever finishes last writes what is true.
func TestADisableDuringAFailingEnableLeavesTheRowAgreeing(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "failslow", failingInitPlugin("failslow", 400_000_000))
	ctx := createTestContextWithPlugins(t, dir)
	if sqlDB, err := ctx.db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	var wg sync.WaitGroup
	var enableErr, disableErr error
	enableDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(enableDone)
		enableErr = ctx.SetPluginEnabled("failslow", true)
	}()

	if !awaitEnableInFlight(t, ctx, "failslow", 5*time.Second) {
		t.Fatal("never observed the enable in flight, so the disable could not have landed " +
			"inside its load and nothing was tested; raise failingInitPlugin's spin count")
	}

	// EnableInFlight is a snapshot, not a lifecycle barrier: it can go true and
	// false again, so having seen it true does not prove the enable is still
	// running now. This does. Unlike the two sibling tests, this one cannot prove
	// the overlap from the second operation's error — a disable landing inside a
	// load is refused with ErrLoadInProgress only if the load outlasts
	// retireDrainTimeout, and otherwise waits it out and succeeds — so the proof
	// has to be that the enable had not returned when the disable was issued.
	select {
	case <-enableDone:
		t.Fatal("the enable finished before the disable was issued, so the two never " +
			"overlapped and nothing was tested; raise failingInitPlugin's spin count")
	default:
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		disableErr = ctx.SetPluginEnabled("failslow", false)
	}()
	wg.Wait()

	if enableErr == nil {
		t.Fatal("the enable succeeded; this test needs its init() to fail")
	}
	// Which answer the disable gets is not asserted, for the reason above; it is
	// reported only if the agreement below fails, where it is the first thing
	// worth knowing.
	t.Logf("the disable that landed inside the load returned: %v", disableErr)

	if ctx.pluginManager.IsEnabled("failslow") {
		t.Fatal("the plugin loaded, so the interleave under test never happened; " +
			"its init() is supposed to fail")
	}
	assertStateAgrees(t, ctx, "failslow")
}

// A refused re-enable of a plugin that is running must leave it running and the
// row saying so. The row is written true before the refusal arrives, and the
// refusal ("already enabled") is not a reason to take it back — reverting there
// left the plugin serving with the row saying off, and the next restart dropped
// it and its schedules.
//
// Under settle-time reconciliation this needs no special case: the plugin is
// loaded, so the reconcile writes true.
func TestARefusedReEnableLeavesTheRowAgreeing(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "twice", slowInitPlugin("twice", 1))
	ctx := createTestContextWithPlugins(t, dir)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	if err := ctx.SetPluginEnabled("twice", true); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if err := ctx.SetPluginEnabled("twice", true); err == nil {
		t.Fatal("the second enable was accepted; this test needs it refused")
	}

	if !ctx.pluginManager.IsEnabled("twice") {
		t.Fatal("the refused re-enable stopped the plugin")
	}
	assertStateAgrees(t, ctx, "twice")
}

// Disabling a plugin that is not loaded still moves the row, and the
// scoped-access snapshot keys on that column.
//
// This path takes the idempotent early return — DisablePlugin refuses because
// nothing is loaded, and the caller asked for a state the process is already in
// — so it never reaches the end of the function. Any invalidation written down
// there is skipped, which is how the original code came to have an explicit one
// in this branch. Folding invalidation into the reconcile is only correct if the
// reconcile invalidates whenever it runs: keyed on its own settling UPDATE it
// would do nothing here, because the caller's optimistic write already left the
// row at the value the reconcile would have written.
//
// The cost of getting it wrong is a group-limited principal still being told a
// disabled plugin is reachable, for up to the snapshot's TTL.
func TestDisablingAnUnloadedPluginDropsTheScopedAccessSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "reachable", slowInitPlugin("reachable", 1))
	ctx := createTestContextWithPlugins(t, dir)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	// Enabled in the row, allowed to scoped principals, and deliberately not
	// loaded — the state a restart leaves behind when a plugin fails to load.
	if err := ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", "reachable").
		Updates(map[string]any{"enabled": true, "allow_scoped_principals": true}).Error; err != nil {
		t.Fatalf("seed plugin row: %v", err)
	}
	if !ctx.PluginAllowsScopedPrincipals("reachable") {
		t.Fatal("the plugin does not read as reachable to begin with, so nothing is being tested")
	}

	if err := ctx.SetPluginEnabled("reachable", false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if ctx.PluginAllowsScopedPrincipals("reachable") {
		t.Fatal("a disabled plugin still reads as reachable to group-limited principals: " +
			"the snapshot was not dropped when the row moved")
	}
}

// A transient failure of the settling write must not be the last word.
//
// The row is written optimistically before the act, so if the settle cannot
// happen the row keeps a value the process contradicts — and one direction of
// that is the failure this whole area exists to prevent: a plugin still loaded
// with the row saying off, dropped along with its schedules at the next restart.
// Logging once and returning treated a retryable error as a settled outcome.
//
// The failure is injected rather than raced for: a GORM callback fails the
// settling UPDATE exactly once, which is what a lock or connection blip looks
// like from here. Racing for a real one would give a test that passes whether or
// not the retry exists.
func TestASettlingWriteThatFailsOnceStillSettles(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "blip", failingInitPlugin("blip", 1))
	ctx := createTestContextWithPlugins(t, dir)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	var mu sync.Mutex
	seen := 0
	err := ctx.db.Callback().Update().Before("gorm:update").
		Register("test_fail_the_settling_write_once", func(db *gorm.DB) {
			if db.Statement == nil || db.Statement.Table != "plugin_states" {
				return
			}
			dest, ok := db.Statement.Dest.(map[string]interface{})
			if !ok {
				return
			}
			if _, touchesEnabled := dest["enabled"]; !touchesEnabled {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			seen++
			// 1 is the caller's optimistic write, 2 is the first settling
			// attempt. Fail that one and let the retry through.
			if seen == 2 {
				db.AddError(errors.New("injected transient failure"))
			}
		})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	// Fails to load, so the settle has real work to do: the row says true and the
	// plugin is absent.
	if err := ctx.SetPluginEnabled("blip", true); err == nil {
		t.Fatal("the enable succeeded; this test needs its init() to fail")
	}
	if ctx.pluginManager.IsEnabled("blip") {
		t.Fatal("the plugin loaded; this test needs its init() to fail")
	}

	mu.Lock()
	attempts := seen
	mu.Unlock()
	if attempts < 3 {
		t.Fatalf("the settling write was attempted %d times in total (1 optimistic + %d settling); "+
			"the failed attempt was not retried", attempts, attempts-1)
	}

	assertStateAgrees(t, ctx, "blip")
}
