package application_context

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_system"
)

// failingInitPlugin spins long enough to overlap a second caller, then fails.
func failingInitPlugin(name string, spins int) string {
	return fmt.Sprintf(`plugin = { name = %q, version = "1.0", api_version = 1 }
function init()
    local x = 0
    for i = 1, %d do x = x + i end
    error("this plugin refuses to load")
end
`, name, spins)
}

// The winner of two concurrent enables fails to load, and the loser deferred to
// it. Nobody is left to turn the row off.
//
// The loser wrote enabled=true and was refused with ErrEnableInProgress, which
// by design means "the winner owns the outcome, do not undo its write". The
// winner then fails and undoes — but the column it wrote is no longer the one
// standing, because the loser overwrote it with the same value. A conditional
// undo keyed on the writer's own value cannot see that the later write was not a
// *decision* at all: it was a caller that immediately deferred, and abandoned it.
//
// The row is left saying enabled while the plugin is absent, so the next restart
// loads a plugin whose only enable attempt failed.
func TestAFailedWinningEnableTurnsTheRowBackOff(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "faily", failingInitPlugin("faily", 400_000_000))
	ctx := createTestContextWithPlugins(t, dir)
	if sqlDB, err := ctx.db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	var wg sync.WaitGroup
	var winnerErr, loserErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		winnerErr = ctx.SetPluginEnabled("faily", true)
	}()

	// Land the second enable inside the first one's load. Waiting on the
	// manager's marker rather than sleeping a guessed interval: if the two did
	// not overlap they both simply fail, the row ends up false anyway, and the
	// test would pass without ever creating the situation it is named for.
	if !awaitEnableInFlight(t, ctx, "faily", 5*time.Second) {
		t.Fatal("never observed the first enable in flight, so the second did not land " +
			"inside its load and nothing was tested; raise failingInitPlugin's spin count")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		loserErr = ctx.SetPluginEnabled("faily", true)
	}()
	wg.Wait()

	if winnerErr == nil {
		t.Fatal("the winning enable succeeded; its init() is supposed to fail")
	}
	if !errors.Is(loserErr, plugin_system.ErrEnableInProgress) {
		t.Fatalf("the second enable returned %v, want ErrEnableInProgress; it did not defer "+
			"to the first, so the case under test — a loser that leaves the undo to a winner "+
			"that then fails — never arose", loserErr)
	}
	if ctx.pluginManager.IsEnabled("faily") {
		t.Fatal("the plugin loaded, so the interleave under test never happened; " +
			"its init() is supposed to fail")
	}

	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", "faily").First(&state).Error; err != nil {
		t.Fatalf("read plugin state: %v", err)
	}
	if state.Enabled {
		t.Fatal("no enable of this plugin succeeded, and the row says it is enabled: " +
			"the next restart loads a plugin that has never once loaded")
	}
}
