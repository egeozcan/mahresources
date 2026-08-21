package plugin_system

import (
	"testing"
	"time"
)

// A top-level after-hook must give up on a busy VM rather than parking the
// caller on it.
//
// lockVMForHook's top-level branch used LockVMWithContext, whose comment reads
// "wait as long as its caller waits". That is right for a before-hook, whose
// reqCtx carries the request's own deadline and which may be the hook that would
// veto. RunAfterHooks has neither property: it passes context.Background() by
// design, because the request that opened a plugin transaction may be gone by
// the time the drain runs. Unbounded plus deadline-less meant an already
// committed write could hold a user's request on another goroutine's busy VM for
// as long as that VM stayed busy — which one mah.http sync call can make
// minutes.
//
// Skipping is the accepted outcome here and always has been: the dispatcher's
// own comment says a busy after-hook is "a missed notification rather than a
// bypassed guard", and delivery is best-effort in five other ways already,
// including silently at shutdown.
func TestRunAfterHooksGivesUpOnABusyVM(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "watcher", `
plugin = { name = "watcher", version = "1.0", description = "observes" }
function init()
    mah.on("after_note_delete", function(data) return data end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("watcher"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	hooks := pm.GetHooks("after_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}

	// Somebody else is using that VM and will not let go.
	held := pm.LockVM(hooks[0].state)
	if held == nil {
		t.Fatal("could not hold the watcher VM")
	}
	defer held.Unlock()

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		pm.RunAfterHooks(NewInvocation(0), "after_note_delete", map[string]any{"id": float64(1)})
		done <- time.Since(start)
	}()

	select {
	case waited := <-done:
		// It must have waited for the bound rather than returning instantly,
		// and it must not have waited materially longer.
		if waited > hookLockWait+10*time.Second {
			t.Errorf("RunAfterHooks waited %s, want about %s", waited, hookLockWait)
		}
	case <-time.After(hookLockWait + 20*time.Second):
		t.Fatal("RunAfterHooks never returned: a committed write is parked on a busy plugin VM")
	}
}

// The other direction, and the reason this is a pair: bounding the wait must not
// turn a free VM into a skipped hook. Same plugin, same dispatch, nobody holding
// the lock — the handler runs.
func TestRunAfterHooksStillRunsWhenTheVMIsFree(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "watcher", `
plugin = { name = "watcher", version = "1.0", description = "observes" }
ran = false
function init()
    mah.on("after_note_delete", function(data)
        ran = true
        return data
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("watcher"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	hooks := pm.GetHooks("after_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}

	pm.RunAfterHooks(NewInvocation(0), "after_note_delete", map[string]any{"id": float64(1)})

	mu := pm.LockVM(hooks[0].state)
	if mu == nil {
		t.Fatal("could not take the watcher VM after dispatch")
	}
	defer mu.Unlock()
	if ran := hooks[0].state.GetGlobal("ran"); ran.String() != "true" {
		t.Errorf("the after-hook did not run on a free VM (ran = %s)", ran.String())
	}
}

// The bound is per dispatch, not per hook. Two plugins both observing the same
// event, both with busy VMs, must cost the caller one budget between them --
// not one each. Bounding per hook bounds nothing a user can feel: N observers
// would stack N waits onto a single already-committed write.
func TestRunAfterHooksSpendsOneBudgetAcrossEveryHook(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"watcher-a", "watcher-b"} {
		writePlugin(t, dir, name, `
plugin = { name = "`+name+`", version = "1.0", description = "observes" }
function init()
    mah.on("after_note_delete", function(data) return data end)
end
`)
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"watcher-a", "watcher-b"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	hooks := pm.GetHooks("after_note_delete")
	if len(hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(hooks))
	}

	// Both VMs busy, so neither can be acquired without waiting.
	var held []*vmMutex
	for _, hook := range hooks {
		mu := pm.LockVM(hook.state)
		if mu == nil {
			t.Fatal("could not hold a watcher VM")
		}
		held = append(held, mu)
	}
	defer func() {
		for _, mu := range held {
			mu.Unlock()
		}
	}()

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		pm.RunAfterHooks(NewInvocation(0), "after_note_delete", map[string]any{"id": float64(1)})
		done <- time.Since(start)
	}()

	select {
	case waited := <-done:
		// Two busy hooks, one budget. Anything approaching 2x means the bound is
		// per hook and does not bound the dispatch.
		if waited >= 2*hookLockWait {
			t.Errorf("two busy hooks cost %s, which is per-hook bounding; want about %s for the whole dispatch",
				waited, hookLockWait)
		}
	case <-time.After(2*hookLockWait + 20*time.Second):
		t.Fatal("RunAfterHooks never returned")
	}
}
