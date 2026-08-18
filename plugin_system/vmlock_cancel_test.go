package plugin_system

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Waiting for a plugin's VM is the one place in this package that ignores the
// caller's context entirely. LockVM calls mu.Lock(), which takes no context, so
// a request whose client has already gone away keeps a goroutine queued behind
// whatever is running — and what is running can be a 120-second
// mah.http.*_sync call (maxHttpTimeout) or a 30-minute remote fetch, because
// executeSyncHttpRequest deliberately drops the Lua deadline for the holder.
//
// The holder is cancellable and the waiters are not, which is the asymmetry
// these tests close. Nothing here gives any surface a new deadline: a caller
// that is still there waits exactly as long as it does today.

// vmBusyPlugin is a plugin whose page handler holds its VM for `secs` seconds
// inside a Go call, which is what a synchronous HTTP request looks like from
// the outside: the Lua deadline cannot preempt it and the VM lock is held
// throughout.
func vmBusyPlugin(secs string) string {
	return `
plugin = { name = "slowpoke", version = "1.0", description = "holds its VM" }

function slow_page(ctx)
    mah.sleep(` + secs + `)
    return "<p>done</p>"
end

function fast_page(ctx)
    return "<p>fast</p>"
end

function init()
    mah.page("slow", slow_page)
    mah.page("fast", fast_page)
end
`
}

// enableBusyPlugin returns a manager with the busy plugin loaded, plus a
// function that starts the slow page on its own goroutine and returns once that
// page is provably holding the VM lock.
func enableBusyPlugin(t *testing.T, secs string) (*PluginManager, func()) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "slowpoke", vmBusyPlugin(secs))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("slowpoke"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	holdVM := func() {
		started := make(chan struct{})
		go func() {
			close(started)
			_, _ = pm.HandlePage(context.Background(), "slowpoke", "slow", PageContext{Path: "/slow"})
		}()
		<-started
		// Wait until the lock is actually held rather than sleeping a guess: the
		// goroutine above has to get through HandlePage's registry lookup and
		// into mah.sleep before the contention this test is about exists.
		deadline := time.Now().Add(2 * time.Second)
		for {
			mu := pm.VMLock(pm.states[0])
			if mu != nil && !mu.TryLock() {
				return
			}
			if mu != nil {
				mu.Unlock()
			}
			if time.Now().After(deadline) {
				t.Fatal("the slow page never took the VM lock")
			}
			time.Sleep(time.Millisecond)
		}
	}
	return pm, holdVM
}

// A request that is cancelled while queued behind a busy VM must stop waiting.
//
// This is the wake-up path, deliberately: the context is live when the call
// starts and is cancelled while it is already blocked. A test that pre-cancelled
// would pass against a mere ctx.Err() check on the way in, which is not the
// defect — the defect is that a goroutine already parked on mu.Lock() has no way
// to be told the request is gone.
func TestHandlePage_CancelledWhileWaitingForBusyVM(t *testing.T) {
	pm, holdVM := enableBusyPlugin(t, "3")
	holdVM()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := pm.HandlePage(ctx, "slowpoke", "fast", PageContext{Path: "/fast"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error for a request abandoned while waiting for a busy VM")
	}
	// Generous: the holder runs for 3s, so anything under a second proves the
	// waiter stopped on the cancellation rather than on the holder finishing.
	if elapsed > time.Second {
		t.Fatalf("waited %s for a cancelled request; it should have given up when the context was cancelled, "+
			"not when the plugin's 3s call finished", elapsed)
	}
	// The error has to say which of the two things happened. "No longer
	// available" is what a *disabled* plugin returns, and reporting an abandoned
	// request that way sends an operator looking for a plugin that is fine.
	//
	// Asserted here rather than in a test of its own: a pre-cancelled context
	// fails the Lua call too, so a separate test would pass on the handler's
	// "context canceled" without the wait ever having been cancellable.
	if !strings.Contains(err.Error(), "busy") {
		t.Fatalf("error %q should say the plugin was busy and the request was abandoned", err)
	}
	if strings.Contains(err.Error(), "no longer available") {
		t.Fatalf("error %q reports an abandoned request as a missing plugin", err)
	}
}

// A caller that is still there keeps waiting for as long as it takes. This is
// the property that makes C1 a defect fix rather than a policy change: no
// surface gains a deadline it did not have.
func TestHandlePage_UncancelledCallerStillWaitsOutABusyVM(t *testing.T) {
	pm, holdVM := enableBusyPlugin(t, "1")
	holdVM()

	start := time.Now()
	out, err := pm.HandlePage(context.Background(), "slowpoke", "fast", PageContext{Path: "/fast"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("HandlePage: %v", err)
	}
	if !strings.Contains(out, "fast") {
		t.Fatalf("got %q, want the fast page's output", out)
	}
	if elapsed < 500*time.Millisecond {
		t.Fatalf("returned after %s, which is sooner than the 1s holder could have released the VM; "+
			"an uncancelled caller must wait rather than fail fast", elapsed)
	}
}

// A non-positive wait means "do not block", and it has to keep meaning that.
//
// The two zero-values disagree, which is the whole trap. LockWithin reads a zero
// wait as "no deadline of my own" — correct for LockVM, which wants to block
// forever — while TryLockVMWithin is the function that exists to *bound* a wait,
// so for it an unbounded zero recreates the cross-goroutine cycle it was written
// to break. Passing wait straight through therefore turns the safest-looking
// argument into a permanent hang. Today's only caller passes hookLockWait, so
// this is latent; it is exactly the kind of latent that gets found by a deadlock.
func TestTryLockVMWithin_ZeroWaitDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", "plugin = { name = \"one\", version = \"1.0\", description = \"\" }\nfunction init() end")
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("one"); err != nil {
		t.Fatal(err)
	}
	pm.mu.RLock()
	L := pm.states[0]
	pm.mu.RUnlock()

	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}
	heldReleased := false
	defer func() {
		if !heldReleased {
			held.Unlock()
		}
	}()

	// On its own goroutine: the defect under test is an unbounded block, so
	// calling it inline would hang the package run instead of failing here.
	type result struct {
		mu   *vmMutex
		busy bool
	}
	done := make(chan result, 1)
	go func() {
		mu, busy := pm.TryLockVMWithin(L, 0)
		done <- result{mu, busy}
	}()

	select {
	case got := <-done:
		if got.mu != nil {
			got.mu.Unlock()
			t.Fatal("TryLockVMWithin acquired a lock that was already held")
		}
		if !got.busy {
			t.Fatal("TryLockVMWithin reported the plugin as gone; it is alive and merely busy")
		}
	case <-time.After(2 * time.Second):
		// Failing is not enough; the failure has to be survivable. With the
		// defect present that goroutine is parked on the lock and will take it
		// the moment we release, and it never unlocks what it returns — so
		// leaving it to the deferred release would hand pm.Close a lock nobody
		// gives back, and the package would hang instead of reporting this.
		// Release here, then put back whatever it takes.
		held.Unlock()
		heldReleased = true
		if got := <-done; got.mu != nil {
			got.mu.Unlock()
		}
		t.Fatal("TryLockVMWithin(L, 0) blocked; a non-positive wait must not block at all")
	}
}

// RenderSlot has no error to return, so its cancellation behaviour is that it
// stops rather than that it reports. Injections run on every page, six slots per
// layout, so a slot render queued behind a busy plugin is the widest exposure of
// the defect.
func TestRenderSlot_CancelledWhileWaitingForBusyVM(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "slowpoke", `
plugin = { name = "slowpoke", version = "1.0", description = "holds its VM" }

function slow_page(ctx)
    mah.sleep(3)
    return "<p>done</p>"
end

function banner(ctx)
    return "<b>banner</b>"
end

function init()
    mah.page("slow", slow_page)
    mah.inject("page_bottom", banner)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("slowpoke"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	go func() {
		_, _ = pm.HandlePage(context.Background(), "slowpoke", "slow", PageContext{Path: "/slow"})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu := pm.VMLock(pm.states[0])
		if mu != nil && !mu.TryLock() {
			break
		}
		if mu != nil {
			mu.Unlock()
		}
		if time.Now().After(deadline) {
			t.Fatal("the slow page never took the VM lock")
		}
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	out := pm.RenderSlot(ctx, "page_bottom", map[string]any{"path": "/x"}, nil)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("RenderSlot waited %s behind a busy plugin after its request was cancelled", elapsed)
	}
	if out != "" {
		t.Fatalf("got %q, want nothing rendered for an abandoned request", out)
	}
}
