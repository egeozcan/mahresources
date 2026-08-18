package plugin_system

import (
	"context"
	"sync"
	"testing"
)

// Rapidly cycling a plugin off and on must not corrupt a render already under
// way.
//
// TestVMLockConcurrentDisable deliberately declines to provoke this and records
// why: L.Close() frees the state's call-frame stack, gopher-lua returns those
// segments to a process-global sync.Pool (state.go: segmentPool), and the next
// enable can draw the same memory. A render holding a registry entry from before
// the disable would then be reading what the new state is writing -- an ABA,
// since the identity the code compares is equal across a change that matters.
//
// It does not reproduce, and this test passes today. That is worth stating
// plainly rather than leaving as implied coverage: it was written to fail, run
// under -race with GOGC=1, 120 cycles against eight readers rendering
// continuously, and stayed clean. The protocol appears to be why. Teardown takes
// the VM lock before closing, so nothing is executing on the state when its
// stack is freed; and a captured entry keeps the old *lua.LState reachable, so
// its address cannot be handed to the replacement either.
//
// So this is a regression guard for a hazard that is currently closed, not a
// demonstration of one that is open. It earns its place by pinning that the
// cycling path stays safe -- the teardown ordering it depends on is easy to
// disturb, and nothing else exercises enable and disable racing live renders.
func TestVMLockRapidEnableDisableCycle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "racer", `
plugin = { name = "racer", version = "1.0", description = "race probe" }

function render_slot(ctx)
    local s = ""
    for i = 1, 200 do
        s = s .. "x"
    end
    return "<b>" .. s .. "</b>"
end

function init()
    mah.inject("page_bottom", render_slot)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("racer"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	var wg sync.WaitGroup

	// Readers run until the cycling finishes rather than for a fixed count, so
	// they are still rendering during every enable and disable. A fixed number
	// of iterations races the writer's much slower loop and can be over before
	// the first cycle completes, which is a test that exercises nothing.
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				_ = pm.RenderSlot(context.Background(), "page_bottom", map[string]any{"path": "/x"}, nil)
			}
		}()
	}

	// Writer: cycle the plugin, which is what an operator does when a plugin
	// misbehaves -- toggle it off, toggle it back on -- and what a reload does.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for j := 0; j < 120; j++ {
			if err := pm.DisablePlugin("racer"); err != nil {
				t.Errorf("DisablePlugin: %v", err)
				return
			}
			if err := pm.EnablePlugin("racer"); err != nil {
				t.Errorf("EnablePlugin: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
