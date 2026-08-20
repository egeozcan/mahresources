package plugin_system

import (
	"errors"
	"testing"
	"time"
)

// The loser of two concurrent enables has to be told apart from every other
// refusal, because it is the one refusal whose durable state is not the loser's
// to undo. The caller cannot work that out from the plugin's loaded state: the
// winner is inside loadPlugin at that moment, so the plugin reads as not
// enabled, which is indistinguishable from a load that failed.
//
// This pins the sentinel onto the path that actually produces it. The
// application layer's half — that a refusal carrying it reverts nothing — is
// TestConcurrentEnableLeavesTheWinnerEnabled.
func TestEnablePluginReportsAnEnableAlreadyInFlight(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "contended", `plugin = { name = "contended", version = "1.0", api_version = 1 }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)

	// Exactly what the winner leaves behind for the whole of its load.
	pm.enabling.Store("contended", struct{}{})

	err = pm.EnablePlugin("contended")
	if !errors.Is(err, ErrEnableInProgress) {
		t.Fatalf("EnablePlugin during an in-flight enable returned %v, want ErrEnableInProgress", err)
	}
	if pm.IsEnabled("contended") {
		t.Fatal("the refused enable loaded the plugin anyway")
	}
}

// The other refusal that reaches the same caller, and the one it must not be
// confused with: the plugin is already loaded, nothing is in flight, and the
// durable state the caller wrote is already correct.
func TestEnablePluginReportsAnAlreadyEnabledPluginDifferently(t *testing.T) {
	dir := t.TempDir()
	pm, err := enablingPlugin(t, dir, "loaded", `plugin = { name = "loaded", version = "1.0", api_version = 1 }
function init() end
`)
	if err != nil {
		t.Fatalf("first enable: %v", err)
	}

	err = pm.EnablePlugin("loaded")
	if err == nil {
		t.Fatal("enabling an already-enabled plugin was accepted")
	}
	if errors.Is(err, ErrEnableInProgress) {
		t.Fatal("an already-enabled plugin reported as an enable in flight; the caller " +
			"would skip a revert it may legitimately need")
	}
}

// The gap this closes is between EnablePlugin claiming a name and its load
// registering in pm.loading. disablePlugin can wait for a load it can see there,
// but the header read sits in front of that registration — it builds a throwaway
// LState and runs the plugin's entire top-level chunk — and for its duration the
// plugin is in no map at all. A disable arriving then was told "not enabled",
// which the caller reasonably persisted.
func TestAwaitEnableVisibleWaitsOutAClaimThatHasNotStartedLoading(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "claimed", `plugin = { name = "claimed", version = "1.0", api_version = 1 }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)

	// Nothing in flight: an answer is available immediately.
	if !pm.awaitEnableVisible("claimed", time.Second) {
		t.Fatal("reported an enable in flight when none was")
	}

	// Claimed, and not yet loading — the gap.
	pm.enabling.Store("claimed", struct{}{})
	start := time.Now()
	if pm.awaitEnableVisible("claimed", 50*time.Millisecond) {
		t.Fatal("reported the enable as visible while it held the name and had registered no load")
	}
	if waited := time.Since(start); waited < 40*time.Millisecond {
		t.Fatalf("gave up after %s without waiting out the 50ms it was given", waited)
	}

	// The enable ending is an answer too: the ordinary path can now read the
	// truth, whether the load published or failed.
	pm.enabling.Delete("claimed")
	if !pm.awaitEnableVisible("claimed", time.Second) {
		t.Fatal("kept waiting after the enable released the name")
	}
}

// The sentinel has to stay distinguishable from the refusal it was carved out
// of, or the caller's idempotent branch — disabling something already disabled
// is a success — stops working.
func TestDisablePluginStillReportsAPluginThatWasNeverEnabled(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dormant", `plugin = { name = "dormant", version = "1.0", api_version = 1 }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)

	err = pm.DisablePlugin("dormant")
	if err == nil {
		t.Fatal("disabling a plugin that was never enabled reported success from the manager")
	}
	if errors.Is(err, ErrLoadInProgress) {
		t.Fatalf("a dormant plugin reported as an enable in flight (%v); the caller would put "+
			"the row back to enabled for a plugin that is not running", err)
	}
}
