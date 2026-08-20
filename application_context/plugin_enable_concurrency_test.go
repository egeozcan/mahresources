package application_context

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"mahresources/models"
	"mahresources/plugin_system"
)

// slowInitPlugin's init() burns a measurable amount of time inside loadPlugin,
// which is the window a second enable has to land in. Lua rather than a Go
// sleep because init() is the only part of a load a plugin controls, and it is
// deliberately unbounded — the load deadline is dropped before it runs, since a
// context set across init() would be inherited by any coroutine created there
// and cancelled out from under it.
func slowInitPlugin(name string, spins int) string {
	return fmt.Sprintf(`plugin = { name = %q, version = "1.0", api_version = 1 }
function init()
    local x = 0
    for i = 1, %d do x = x + i end
end
`, name, spins)
}

// Two operators enabling one plugin at the same instant used to switch it off.
//
// Both write enabled=true and both call EnablePlugin; one wins and loads, the
// other is refused immediately. The loser then asked "is it enabled?" to decide
// whether to undo its own write — and the honest answer during the winner's load
// is no, because the plugin is not in the manager's map until loadPlugin
// finishes. So the loser reverted the row while the winner was still loading,
// and the winner never rewrites it: the process ends up running a plugin the
// database says is off, which the next restart resolves by dropping it, taking
// its schedules with it.
//
// The state cannot answer this; only the refusal can. ErrEnableInProgress says
// the winner owns the outcome — it wrote the same value and will revert it
// itself if its load fails.
func TestConcurrentEnableLeavesTheWinnerEnabled(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "slow", slowInitPlugin("slow", 8_000_000))
	ctx := createTestContextWithPlugins(t, dir)
	// cache=private hands every new connection its own empty database, so a
	// concurrent test must not let the pool grow.
	if sqlDB, err := ctx.db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	// The interleave is what the test needs, not merely two goroutines: if the
	// winner finishes before the loser reaches the manager, the loser is refused
	// with "already enabled" instead and this proves nothing. Retry rather than
	// hope, and fail loudly if the window never opens.
	var interleaved bool
	for attempt := 1; attempt <= 3 && !interleaved; attempt++ {
		results := make([]error, 2)
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i] = ctx.SetPluginEnabled("slow", true)
			}(i)
		}
		wg.Wait()

		var winners, losers int
		for _, err := range results {
			switch {
			case err == nil:
				winners++
			case errors.Is(err, plugin_system.ErrEnableInProgress):
				losers++
			}
		}
		if winners != 1 {
			t.Fatalf("attempt %d: %d of 2 concurrent enables succeeded, want exactly 1 (%v)",
				attempt, winners, results)
		}
		if losers == 1 {
			interleaved = true
			break
		}
		// No overlap this time. Put it back and try again.
		if err := ctx.SetPluginEnabled("slow", false); err != nil {
			t.Fatalf("attempt %d: disable between attempts: %v", attempt, err)
		}
	}
	if !interleaved {
		t.Fatal("the two enables never overlapped, so nothing was tested; " +
			"raise slowInitPlugin's spin count")
	}

	if !ctx.pluginManager.IsEnabled("slow") {
		t.Fatal("the winner's load did not leave the plugin running")
	}
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", "slow").First(&state).Error; err != nil {
		t.Fatalf("read plugin state: %v", err)
	}
	if !state.Enabled {
		t.Fatal("the loser reverted the winner's write: the plugin is running with the " +
			"row saying off, and the next restart drops it and its schedules")
	}
}
