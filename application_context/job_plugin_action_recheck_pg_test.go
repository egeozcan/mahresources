//go:build postgres && json1 && fts5

package application_context

import (
	"os"
	"path/filepath"
	"testing"

	"mahresources/plugin_system"
)

// TestAReCheckThatCannotFinishGivesTheClaimBackOnPostgres is the engine parity of
// the admission's bound. On PostgreSQL the claim serializes the budget with a
// transaction-scoped advisory lock and the scoped reads run on a pool of real
// connections, so a claim given back has to free its capacity there too, and a
// read cancelled by the bound has to come back from the server rather than from
// SQLite's own interrupt.
func TestAReCheckThatCannotFinishGivesTheClaimBackOnPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	if old := ctx.PluginManager(); old != nil {
		old.Close()
	}
	pluginDir := t.TempDir()
	pluginPath := filepath.Join(pluginDir, pluginActionTestPlugin)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		t.Fatalf("create plugin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginPath, "plugin.lua"), []byte(pluginActionTestSource), 0o644); err != nil {
		t.Fatalf("write plugin source: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(pluginDir)
	if err != nil {
		t.Fatalf("create plugin manager: %v", err)
	}
	ctx.pluginManager = pm
	if err := ctx.registerPluginActionJobKind(ctx.JobService()); err != nil {
		t.Fatalf("install plugin action host seam: %v", err)
	}
	t.Cleanup(pm.Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}

	exerciseAReCheckThatCannotFinish(t, ctx)
}
