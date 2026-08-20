package application_context

import (
	"errors"
	"sync"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_system"
)

// A disable that could not act must not report that it did.
//
// SetPluginEnabled(name, false) writes enabled=false and then treats every
// DisablePlugin refusal as idempotent success whenever the plugin does not read
// as loaded. During a load it does not read as loaded — the manager publishes it
// only at the very end — so a disable landing mid-enable was answered with
// {"ok":true,"enabled":false} while the load went on to publish. The plugin then
// serves pages, fires hooks and writes through mah.db while /plugins/manage,
// which renders the row, shows it as off; nothing reconciles the two until the
// next restart drops the plugin and its schedules.
//
// It is the same mistake the enable branch made and the same reason: the loaded
// state is being asked a question it cannot answer, because "mid-load" and
// "never loaded" look identical from there. The manager reports the difference
// as ErrLoadInProgress; this asserts that the caller acts on it.
//
// The assertion is agreement rather than a specific outcome. Whichever call
// lands last, the process and the database must say the same thing about
// whether this plugin is running.
func TestDisableDuringAnEnableDoesNotReportSuccess(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "slow", slowInitPlugin("slow", 500_000_000))
	ctx := createTestContextWithPlugins(t, dir)
	if sqlDB, err := ctx.db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	var enableErr error
	enabled := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(enabled)
		enableErr = ctx.SetPluginEnabled("slow", true)
	}()

	// Wait for the manager to report the load in flight, then disable once.
	//
	// This replaced a loop that retried the disable until one attempt took longer
	// than three seconds, on the reasoning that the sentinel could not be used to
	// *detect* the interleave because swallowing it was the defect under test. It
	// could not, but the manager's own in-flight marker can, and it says so
	// exactly rather than inferring it from a duration — which a loaded CI host
	// could satisfy without any overlap at all. With the interleave established
	// independently, the error becomes the assertion instead of the detector.
	if !awaitEnableInFlight(t, ctx, "slow", 5*time.Second) {
		t.Fatal("never observed the enable in flight, so no disable could land inside it " +
			"and nothing was tested; raise slowInitPlugin's spin count")
	}
	refusalErr := ctx.SetPluginEnabled("slow", false)
	wg.Wait()

	if enableErr != nil {
		t.Fatalf("the enable itself failed, so the interleave under test never happened: %v", enableErr)
	}

	// Agreement alone is too weak to pin this: an implementation with no sentinel
	// at all, which simply waited out the load and then disabled the plugin,
	// would leave process and row both false and satisfy it. What that
	// implementation would not do is report the refusal, and reporting it is the
	// contract. Detection still cannot key on the error — swallowing it is the
	// defect — so the elapsed wait finds the interleave and the error is asserted
	// once found.
	if !errors.Is(refusalErr, plugin_system.ErrLoadInProgress) {
		t.Fatalf("a disable that landed inside the load returned %v, want ErrLoadInProgress; "+
			"a refusal reported as success leaves the caller believing the plugin is off "+
			"while the load goes on to publish", refusalErr)
	}

	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", "slow").First(&state).Error; err != nil {
		t.Fatalf("read plugin state: %v", err)
	}
	if running := ctx.pluginManager.IsEnabled("slow"); running != state.Enabled {
		t.Fatalf("the process and the database disagree: loaded=%v, row.enabled=%v. "+
			"A disable that was refused because the enable was still in flight reported "+
			"success and left the row off; the plugin is serving with nothing to reconcile "+
			"it until the next restart drops it and its schedules.", running, state.Enabled)
	}
}
