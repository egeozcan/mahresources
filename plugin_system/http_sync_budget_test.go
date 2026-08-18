package plugin_system

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// A synchronous mah.http call made while rendering must not outlive the render.
//
// This reverses a decision this file used to record as "a 5s render timeout
// must not cap a 120s call". The reasoning was that a plugin author asking for
// 120 seconds should get 120 seconds, and taken on its own that is right. What
// it leaves out is what the call is holding while it waits: the plugin's single
// VM lock, which is exclusive across every one of that plugin's surfaces. A
// render that blocks for two minutes does not just render slowly. It takes the
// plugin's pages, shortcodes, blocks, display types, API endpoints and hooks
// with it, for every user, for two minutes -- to produce a fragment of a page
// whose own budget expired 115 seconds earlier and whose output will be
// discarded.
//
// So the cap is the caller's remaining budget wherever there is one, and in
// practice that is every production caller: 30s for a page, 5s for a hook, an
// injection or a drained callback, 5m for an async job. This is deliberately
// not a render-only rule. A hook blocks the write that fired it, and a drained
// callback holds the lock every other surface of that plugin needs, so both
// deserve the same ceiling. An async job's 5 minutes leaves a 120s request
// untouched, which is the case the original decision was really protecting and
// the one that survives.

// slowServer answers after the given delay, or when the request is cancelled.
func slowServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("late"))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// syncBudgetPlugin calls get_sync with a timeout far longer than any render
// budget and reports what came back.
func syncBudgetPlugin(t *testing.T, url string) string {
	t.Helper()
	host, _, err := net.SplitHostPort(strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	return `
plugin = { name = "slowfetch", version = "1.0", description = "fetches slowly",
           api_version = 1, capabilities = { "http", "inject" },
           network = { "` + host + `" }, allow_private_hosts = true }

function fetch(ctx)
    local resp = mah.http.get_sync("` + url + `", { timeout = 100 })
    if resp.error then
        return "error:" .. resp.error
    end
    return "ok:" .. tostring(resp.status)
end

function init()
    mah.inject("page_bottom", fetch)
end
`
}

func enableSyncBudgetPlugin(t *testing.T, url string) *PluginManager {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "slowfetch", syncBudgetPlugin(t, url))
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("slowfetch"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

// A render's synchronous HTTP call is capped at the render's own budget.
//
// An injection rather than a page, for two reasons. Injections carry
// luaExecTimeout (5s) where pages carry luaPageTimeout (30s), so the budget is
// comfortably shorter than the server's delay and the two outcomes are far
// apart in time -- with a page, a 30s budget against a 30s server would measure
// the same either way, and the test would prove nothing. They are also the
// widest render surface there is: six slots sit in the base layout, so every
// page in the application pays for this one.
func TestSyncHttp_RenderIsCappedAtItsOwnBudget(t *testing.T) {
	srv := slowServer(t, 30*time.Second)
	pm := enableSyncBudgetPlugin(t, srv.URL)

	start := time.Now()
	out := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{"path": "/x"}, nil)
	elapsed := time.Since(start)

	// Uncapped this runs until the server answers at 30s (its own timeout asks
	// for 100). Capped it ends at the 5s budget. Generous slack for a loaded
	// machine, while staying far from 30.
	if elapsed > 15*time.Second {
		t.Fatalf("the slot render took %s; a synchronous call inside a render must be capped at "+
			"the render's remaining budget, not run on to the plugin's own timeout", elapsed)
	}
	// The call must fail rather than silently succeed early.
	if !strings.Contains(out, "error:") {
		t.Fatalf("got %q, want the plugin to have seen its request fail; a capped call has to "+
			"report an error, not return a response it never received", out)
	}
}

// A requested timeout shorter than the budget is still the one that applies.
//
// The cap is a ceiling, not a replacement. Getting this wrong in the other
// direction is easy and quiet: a plugin that deliberately asks for 1s so it can
// fall back fast would sit there for the render's whole 5s, which is the exact
// stall this batch exists to remove, reintroduced by the fix for it.
func TestSyncHttp_ShorterRequestedTimeoutStillWins(t *testing.T) {
	srv := slowServer(t, 30*time.Second)
	dir := t.TempDir()
	host, _, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	writePlugin(t, dir, "quickgiveup", `
plugin = { name = "quickgiveup", version = "1.0", description = "gives up fast",
           api_version = 1, capabilities = { "http", "inject" },
           network = { "`+host+`" }, allow_private_hosts = true }

function fetch(ctx)
    local resp = mah.http.get_sync("`+srv.URL+`", { timeout = 1 })
    if resp.error then
        return "error:" .. resp.error
    end
    return "ok:" .. tostring(resp.status)
end

function init()
    mah.inject("page_bottom", fetch)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("quickgiveup"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	start := time.Now()
	out := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{"path": "/x"}, nil)
	elapsed := time.Since(start)

	// The budget is 5s and the plugin asked for 1s, so anything near 5s means
	// the ceiling was applied as the value.
	if elapsed > 3*time.Second {
		t.Fatalf("the call took %s for a plugin that asked for 1s; the caller's budget is a "+
			"ceiling on the requested timeout, not a substitute for it", elapsed)
	}
	if !strings.Contains(out, "error:") {
		t.Fatalf("got %q, want the plugin's own 1s timeout to have fired", out)
	}
}

// A caller with no deadline keeps the timeout the plugin asked for.
//
// What this pins is that the cap is conditional on a budget existing, not that
// any particular production caller lacks one -- they all set a deadline, so the
// contextless call here is constructed rather than sampled. The property still
// has to hold: the rule is "inherit a ceiling", and a caller that never set one
// must not have a number invented for it. Without this, capping unconditionally
// at some render-sized constant would pass every other test in this file.
func TestSyncHttp_NoDeadlineKeepsThePluginsOwnTimeout(t *testing.T) {
	srv := slowServer(t, 2*time.Second)
	pm := enableSyncBudgetPlugin(t, srv.URL)

	L := stateForPlugin(t, pm, "slowfetch")
	mu := pm.LockVM(L)
	if mu == nil {
		t.Fatal("could not take the VM lock")
	}
	defer mu.Unlock()

	fn, ok := L.GetGlobal("fetch").(*lua.LFunction)
	if !ok {
		t.Fatal("plugin has no fetch function")
	}

	// No Lua context at all, which is what a background caller looks like: the
	// call must be allowed to take the 2s the server needs rather than being
	// clipped to a budget that does not exist.
	start := time.Now()
	err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, goToLuaTable(L, map[string]any{}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("calling fetch with no deadline: %v", err)
	}
	got := lua.LVAsString(L.Get(-1))
	L.Pop(1)

	if !strings.HasPrefix(got, "ok:") {
		t.Fatalf("got %q, want a successful response; a caller with no deadline must keep the "+
			"timeout the plugin asked for", got)
	}
	if elapsed < time.Second {
		t.Fatalf("returned in %s, sooner than the server's own 2s delay; the response cannot "+
			"have been real", elapsed)
	}
}

// The boundaries of the cap, asserted directly.
//
// The end-to-end tests above are necessary but not sufficient, and it is worth
// being precise about why: an implementation returning one millisecond for every
// call that has a deadline satisfies all of them, and so does one capping at a
// constant that happens to fall inside their timing slack. They pin that a cap
// exists and roughly where; this pins what it actually computes.
func TestEffectiveSyncTimeout(t *testing.T) {
	const head = syncHttpBudgetHeadroom

	cases := []struct {
		name      string
		requested time.Duration
		remaining time.Duration
		hasBudget bool
		want      time.Duration
	}{
		{
			name:      "no budget leaves the request untouched",
			requested: 100 * time.Second,
			hasBudget: false,
			want:      100 * time.Second,
		},
		{
			name:      "a budget far larger than the request changes nothing",
			requested: 10 * time.Second,
			remaining: time.Hour,
			hasBudget: true,
			want:      10 * time.Second,
		},
		{
			name:      "a request smaller than the usable budget wins",
			requested: time.Second,
			remaining: 5 * time.Second,
			hasBudget: true,
			want:      time.Second,
		},
		{
			name:      "a budget smaller than the request caps it, less the headroom",
			requested: 100 * time.Second,
			remaining: 5 * time.Second,
			hasBudget: true,
			want:      5*time.Second - head,
		},
		{
			// The case that decides whether the headroom is a real subtraction
			// or decoration: the request fits the remainder but not the usable
			// part of it.
			name:      "a request that fits the remainder but not the headroom is trimmed",
			requested: 5 * time.Second,
			remaining: 5 * time.Second,
			hasBudget: true,
			want:      5*time.Second - head,
		},
		{
			name:      "exactly the headroom leaves nothing",
			requested: time.Second,
			remaining: head,
			hasBudget: true,
			want:      0,
		},
		{
			name:      "less than the headroom leaves nothing",
			requested: time.Second,
			remaining: head / 2,
			hasBudget: true,
			want:      0,
		},
		{
			// Zero remaining is a real expired budget, not a stand-in for
			// "no budget". The call site builds the pair as (0, false) when
			// there is no deadline, so an implementation keying off the zero
			// instead of the flag would hand an exhausted caller its full
			// requested timeout, and would pass every other case here.
			name:      "a budget at exactly zero is expired, not absent",
			requested: time.Second,
			remaining: 0,
			hasBudget: true,
			want:      0,
		},
		{
			// Zero, never negative: a negative duration would read as an expired
			// context in some places and as no deadline in others.
			name:      "a budget already spent leaves nothing",
			requested: time.Second,
			remaining: -3 * time.Second,
			hasBudget: true,
			want:      0,
		},
		{
			// time.Until saturates here for a deadline far enough in the past.
			// Subtracting the headroom before comparing would wrap this to a
			// huge positive and hand back the full requested timeout -- an
			// expired budget reading as an enormous one.
			name:      "a saturated negative budget does not wrap around",
			requested: time.Second,
			remaining: time.Duration(math.MinInt64),
			hasBudget: true,
			want:      0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveSyncTimeout(tc.requested, tc.remaining, tc.hasBudget)
			if got != tc.want {
				t.Fatalf("effectiveSyncTimeout(requested=%s, remaining=%s, hasBudget=%v) = %s, want %s",
					tc.requested, tc.remaining, tc.hasBudget, got, tc.want)
			}
			if got < 0 {
				t.Fatalf("returned a negative duration (%s); callers pass this straight to "+
					"context.WithTimeout", got)
			}
		})
	}
}
