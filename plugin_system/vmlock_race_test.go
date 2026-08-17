package plugin_system

import (
	"context"
	"sync"
	"testing"
)

// TestVMLockConcurrentDisable exercises the race between VMLock and
// DisablePlugin. VMLock read pm.vmLocks with no lock held while DisablePlugin
// deletes from that map under pm.mu, which is an unsynchronised map read racing
// a map write: under -race this reports, and in production Go can abort the
// process with "concurrent map read and map write". If the delete wins the race,
// VMLock returns nil and the injection/hook/page renderers dereference it.
//
// Injections run on every rendered page, so this is the widest exposure: toggling
// a plugin from the management UI under load could panic a request goroutine.
//
// Run with -race for this to be meaningful.
func TestVMLockConcurrentDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "racer", `
plugin = { name = "racer", version = "1.0", description = "race probe" }

function render_slot(ctx)
    return "<b>slot</b>"
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

	// Readers: keep rendering the slot, which takes the VM lock every time.
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 400; j++ {
				// Must not panic even once the plugin is gone.
				_ = pm.RenderSlot(context.Background(), "page_bottom", map[string]any{"path": "/x"}, nil)
			}
		}()
	}

	// Writer: disable the plugin underneath them, which is what an operator
	// toggling it off from the management UI does while pages are rendering.
	//
	// This deliberately disables once rather than cycling disable/enable in a
	// loop. Rapid cycling is still unsafe and reports under -race: L.Close()
	// releases the state's registry, so the next enable can allocate its registry
	// over the freed one, and a render that captured a registry entry before the
	// disable then reads memory the new state is writing. Keying liveness on the
	// *lua.LState pointer cannot see that, because the pointer is not what got
	// reused. Fixing it needs a generation stamped on both the registry entries
	// and the VM registry, which is a larger change than the lock lifecycle this
	// test covers; cycling here would report that separate defect instead.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := pm.DisablePlugin("racer"); err != nil {
			t.Errorf("DisablePlugin: %v", err)
		}
	}()

	wg.Wait()
}

// TestVMLockReturnsNilForUnknownState pins the contract the render paths rely
// on: a state the manager no longer tracks has no lock, and callers must be able
// to detect that rather than dereferencing nil.
func TestVMLockReturnsNilForUnknownState(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "solo", `
plugin = { name = "solo", version = "1.0", description = "solo" }
function init() end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("solo"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	pm.mu.RLock()
	state := pm.states[0]
	pm.mu.RUnlock()

	if got := pm.VMLock(state); got == nil {
		t.Fatal("VMLock returned nil for a live state")
	}

	if err := pm.DisablePlugin("solo"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	if got := pm.VMLock(state); got != nil {
		t.Fatal("VMLock returned a lock for a disabled state, want nil")
	}
}
