package plugin_system

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingMRQLExecutor records how many queries actually reached the executor,
// which is what the cache is supposed to reduce.
type countingMRQLExecutor struct {
	calls atomic.Int64
}

func (c *countingMRQLExecutor) ExecuteMRQL(ctx context.Context, query string, opts MRQLExecOptions) (*MRQLResult, error) {
	c.calls.Add(1)
	return &MRQLResult{EntityType: "resource", Mode: "flat", Items: []map[string]any{{"id": float64(1)}}}, nil
}

// Every VM entry point except the shortcode renderer used to build its deadline
// from context.Background(), which put the per-request MRQL cache out of reach:
// a page running the same query in a header count and a body table paid twice,
// while the docs said results were cached.
func TestRequestContext_MRQLCacheReachableFromPage(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ctx-test", `
plugin = { name = "ctx-test", version = "1.0", description = "request context" }
function init()
    mah.page("dash", function(ctx)
        local a = mah.db.mrql_query("type=resource", { limit = 10 })
        local b = mah.db.mrql_query("type=resource", { limit = 10 })
        return "<p>" .. #a.items .. #b.items .. "</p>"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	exec := &countingMRQLExecutor{}
	mgr.SetMRQLExecutor(exec)
	if err := mgr.EnablePlugin("ctx-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html, err := mgr.HandlePage(WithMRQLCache(context.Background()), "ctx-test", "dash", PageContext{Method: "GET"})
	if err != nil {
		t.Fatalf("HandlePage: %v", err)
	}
	if !strings.Contains(html, "11") {
		t.Errorf("page rendered %q, want both queries to return one item", html)
	}
	if got := exec.calls.Load(); got != 1 {
		t.Errorf("executor ran %d times, want 1 — the page's MRQL cache was not reachable", got)
	}
}

// Without a cache in the context nothing is cached, which is the pre-existing
// behaviour and must stay correct rather than merely uncached.
func TestRequestContext_NoCacheStillWorks(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ctx-test", `
plugin = { name = "ctx-test", version = "1.0", description = "request context" }
function init()
    mah.page("dash", function(ctx)
        local a = mah.db.mrql_query("type=resource", { limit = 10 })
        local b = mah.db.mrql_query("type=resource", { limit = 10 })
        return "<p>" .. #a.items .. #b.items .. "</p>"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	exec := &countingMRQLExecutor{}
	mgr.SetMRQLExecutor(exec)
	if err := mgr.EnablePlugin("ctx-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if _, err := mgr.HandlePage(context.Background(), "ctx-test", "dash", PageContext{Method: "GET"}); err != nil {
		t.Fatalf("HandlePage: %v", err)
	}
	if got := exec.calls.Load(); got != 2 {
		t.Errorf("executor ran %d times, want 2 without a cache", got)
	}
}

// A cancelled request must stop the plugin, not let it hold the VM lock — which
// is exclusive across every other surface of that plugin — for the full 30s.
func TestRequestContext_CancellationStopsThePage(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ctx-test", `
plugin = { name = "ctx-test", version = "1.0", description = "request context" }
function init()
    mah.page("spin", function(ctx)
        local n = 0
        while true do n = n + 1 end
        return "<p>unreachable</p>"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("ctx-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = mgr.HandlePage(reqCtx, "ctx-test", "spin", PageContext{Method: "GET"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the cancelled page render to fail")
	}
	// luaPageTimeout is 30s; the cancel must beat it by a wide margin.
	if elapsed > 5*time.Second {
		t.Errorf("cancelled render took %v — cancellation did not propagate", elapsed)
	}
}

// Hooks and async jobs have no request, so they must keep working when handed
// no context at all.
func TestRequestContext_NilContextFallsBackToBackground(t *testing.T) {
	// A typed nil rather than the literal: passing nil is the whole point of
	// this test, and staticcheck's SA1012 fires on the literal at a call site.
	var noContext context.Context
	got := vmParentContext(noContext)
	if got == nil {
		t.Fatal("vmParentContext(nil) returned nil")
	}
	if got.Err() != nil {
		t.Errorf("vmParentContext(nil) should not be already cancelled")
	}
	// No request means nothing to be cancelled by.
	if recovered := vmRequestContext(got); recovered.Done() != nil {
		t.Error("vmRequestContext should give an uncancellable context when there is no request")
	}
}

// A real context stays cancellable through the wrapper, and is recoverable
// undeadlined for work allowed to outlive the Lua timeout (a 120s sync HTTP
// call against a 5s render deadline).
func TestRequestContext_ParentCancellationSurvivesWrapping(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	wrapped := vmParentContext(parent)

	if wrapped.Err() != nil {
		t.Fatal("wrapped context should start live")
	}

	recovered := vmRequestContext(wrapped)
	if recovered != parent {
		t.Errorf("vmRequestContext should recover the caller's own context")
	}

	cancel()
	if wrapped.Err() == nil {
		t.Error("cancelling the parent must cancel the wrapped context")
	}
	if recovered.Err() == nil {
		t.Error("cancelling the parent must cancel the recovered context")
	}
}

// The recovered context must NOT carry the Lua deadline, or a sync HTTP call
// would be capped at the 5s render timeout instead of its own 120s.
func TestRequestContext_RecoveredContextDropsTheLuaDeadline(t *testing.T) {
	parent := context.Background()
	timeoutCtx, cancel := context.WithTimeout(vmParentContext(parent), 5*time.Millisecond)
	defer cancel()

	recovered := vmRequestContext(timeoutCtx)
	if _, hasDeadline := recovered.Deadline(); hasDeadline {
		t.Error("the recovered context must not inherit the Lua deadline")
	}

	time.Sleep(20 * time.Millisecond)
	if timeoutCtx.Err() == nil {
		t.Fatal("the Lua context should have expired")
	}
	if recovered.Err() != nil {
		t.Error("the recovered context must outlive the expired Lua deadline")
	}
}

// Injections are the widest render surface there is — six slots live in the
// base layout, so they run on every page. They looked like the one seam with no
// request to inherit, because the template tag takes no context parameter; in
// fact the tag already reads a request context to decide whether plugin code
// may run at all, and simply did not pass it on.
func TestRequestContext_CancellationStopsAnInjection(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "spinner", `
plugin = { name = "spinner", version = "1.0", description = "slot that spins" }
function init()
    mah.inject("page_bottom", function(ctx)
        local n = 0
        while true do n = n + 1 end
        return "<p>unreachable</p>"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("spinner"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	mgr.RenderSlot(reqCtx, "page_bottom", map[string]any{})
	elapsed := time.Since(start)

	// luaExecTimeout is 5s; the cancel must beat it by a wide margin.
	if elapsed > 2*time.Second {
		t.Errorf("cancelled injection took %v — cancellation did not propagate", elapsed)
	}
}

// And an injection can reach the per-request MRQL cache, which it could not
// while its deadline hung off Background.
func TestRequestContext_MRQLCacheReachableFromAnInjection(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "counter", `
plugin = { name = "counter", version = "1.0", description = "slot that queries twice" }
function init()
    mah.inject("page_bottom", function(ctx)
        local a = mah.db.mrql_query("type=resource", { limit = 10 })
        local b = mah.db.mrql_query("type=resource", { limit = 10 })
        return "<p>" .. #a.items .. #b.items .. "</p>"
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	exec := &countingMRQLExecutor{}
	mgr.SetMRQLExecutor(exec)
	if err := mgr.EnablePlugin("counter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	html := mgr.RenderSlot(WithMRQLCache(context.Background()), "page_bottom", map[string]any{})
	if !strings.Contains(html, "11") {
		t.Errorf("slot rendered %q, want both queries to return one item", html)
	}
	if got := exec.calls.Load(); got != 1 {
		t.Errorf("executor ran %d times, want 1 — the injection's MRQL cache was not reachable", got)
	}
}
