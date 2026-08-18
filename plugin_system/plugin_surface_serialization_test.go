package plugin_system

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// What the platform's concurrency model actually is, measured rather than
// asserted in prose.
//
// Two of a plugin's own surfaces cannot run at the same time. Every entry into
// a plugin's Lua takes that plugin's single exclusive vmMutex for the whole
// call -- injections and the other request-serving surfaces through
// lockVMForRequest, hooks through lockVMForHook, async action jobs and drained
// HTTP callbacks through LockVM -- and the lock is held even across a blocking
// mah.http call, which is the property the sync-budget change three commits ago
// was built on.
//
// This is worth measuring rather than reading off the lock code, because the
// mah.kv documentation depends on the answer and the two are written far apart.
// plugin-lua-api.md already states it correctly where it describes the VM ("All
// calls ... acquire this mutex, ensuring single-threaded execution within a
// single plugin") and then denies it where it describes mah.kv. One of those is
// wrong, and which one is a question about the code, not about the page.
//
// The server is the observer because it is the only place with a clock that
// sees the inside of the call. Timing RenderSlot from the outside proves
// nothing: the second caller's interval starts while it is still queued on the
// lock, so the outside intervals overlap even under perfect serialization.
type surfaceVisit struct {
	surface     string
	enter, exit time.Time
}

// recordingServer answers after hold, recording when each request arrived and
// when it was answered. The hold is what makes an overlap visible: without it
// two requests could be separated by nothing more than how long a connection
// takes to set up.
func recordingServer(t *testing.T, hold time.Duration) (*httptest.Server, func() []surfaceVisit) {
	t.Helper()

	var (
		mu     sync.Mutex
		visits []surfaceVisit
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enter := time.Now()
		time.Sleep(hold)
		mu.Lock()
		visits = append(visits, surfaceVisit{
			surface: r.URL.Query().Get("surface"),
			enter:   enter,
			exit:    time.Now(),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return srv, func() []surfaceVisit {
		mu.Lock()
		defer mu.Unlock()
		out := make([]surfaceVisit, len(visits))
		copy(out, visits)
		return out
	}
}

// surfacePlugin registers one plugin on the given slots. Each slot calls the
// recording server and names itself, so a visit says which surface made it.
func surfacePlugin(t *testing.T, name, url string, slots ...string) string {
	t.Helper()

	host, _, err := net.SplitHostPort(strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	var injects strings.Builder
	for _, slot := range slots {
		fmt.Fprintf(&injects, "    mah.inject(%q, visit(%q))\n", slot, name+":"+slot)
	}
	return fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "calls out from a surface",
           api_version = 1, capabilities = { "http", "inject" },
           network = { %q }, allow_private_hosts = true }

local function visit(label)
    return function(ctx)
        local resp = mah.http.get_sync(%q .. "?surface=" .. label, { timeout = 30 })
        if resp.error then
            return "error:" .. resp.error
        end
        return "ok:" .. label
    end
end

function init()
%s
end
`, name, host, url, injects.String())
}

// renderSlotsTogether parks a goroutine on each slot, releases them all at once
// and returns what each render produced. Waiting for every goroutine to be
// parked before letting any of them go leaves a scheduler wakeup, rather than a
// plugin load, between the release and the call.
func renderSlotsTogether(t *testing.T, pm *PluginManager, slots ...string) map[string]string {
	t.Helper()

	var (
		wg    sync.WaitGroup
		ready = make(chan struct{}, len(slots))
		start = make(chan struct{})
		mu    sync.Mutex
		out   = map[string]string{}
	)
	for _, slot := range slots {
		wg.Add(1)
		go func(slot string) {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			rendered := pm.RenderSlot(context.Background(), slot, map[string]any{"path": "/x"}, nil)
			mu.Lock()
			defer mu.Unlock()
			out[slot] = rendered
		}(slot)
	}
	for range slots {
		<-ready
	}
	close(start)
	wg.Wait()

	for _, slot := range slots {
		if !strings.HasPrefix(out[slot], "ok:") {
			t.Fatalf("slot %s rendered %q: every surface has to reach the server, or there is "+
				"nothing to compare", slot, out[slot])
		}
	}
	return out
}

func enableSurfacePlugins(t *testing.T, sources map[string]string) *PluginManager {
	t.Helper()

	dir := t.TempDir()
	for name, src := range sources {
		writePlugin(t, dir, name, src)
	}
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	for name := range sources {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin %s: %v", name, err)
		}
	}
	return pm
}

// overlap reports whether the two recorded visits were inside the plugin at the
// same time. Visits are recorded at answer time, so the second entry is
// whichever was answered last.
func overlap(t *testing.T, visits []surfaceVisit) bool {
	t.Helper()

	if len(visits) != 2 {
		t.Fatalf("the server saw %d requests, want 2", len(visits))
	}
	return visits[1].enter.Before(visits[0].exit)
}

const surfaceHold = 150 * time.Millisecond

// Two surfaces of one plugin, released together, are served one after the other.
func TestPluginSurfaces_CannotRunAtTheSameTime(t *testing.T) {
	srv, visits := recordingServer(t, surfaceHold)
	pm := enableSurfacePlugins(t, map[string]string{
		"twosurfaces": surfacePlugin(t, "twosurfaces", srv.URL, "surface_a", "surface_b"),
	})

	renderSlotsTogether(t, pm, "surface_a", "surface_b")

	if got := visits(); overlap(t, got) {
		t.Fatalf("surface %s was inside the plugin from %s to %s while surface %s was inside it "+
			"until %s: a plugin's own surfaces overlapped. The VM lock no longer serializes them, "+
			"so the mah.kv prose this backs -- which says a lost update needs two separate calls "+
			"into the plugin, or two processes -- now understates the exposure and must be revisited",
			got[1].surface, got[1].enter, got[1].exit, got[0].surface, got[0].exit)
	}
}

// The control, and the reason the test above means anything.
//
// A harness that could never see an overlap would report serialization for any
// platform at all, including one that had none. Two plugins hold two locks and
// really do run at the same time -- the documented behaviour, and the case the
// mah.kv prose confuses with a plugin's own surfaces -- so the same barrier and
// the same clock have to find them overlapping here.
func TestPluginSurfaces_OfDifferentPluginsDoRunAtTheSameTime(t *testing.T) {
	srv, visits := recordingServer(t, surfaceHold)
	pm := enableSurfacePlugins(t, map[string]string{
		"alpha": surfacePlugin(t, "alpha", srv.URL, "surface_a"),
		"beta":  surfacePlugin(t, "beta", srv.URL, "surface_b"),
	})

	renderSlotsTogether(t, pm, "surface_a", "surface_b")

	if got := visits(); !overlap(t, got) {
		t.Fatalf("surface %s ran from %s to %s and surface %s only entered at %s: two separate "+
			"plugins did not overlap, so this harness cannot detect an overlap and its verdict "+
			"of serialization for one plugin's surfaces proves nothing",
			got[0].surface, got[0].enter, got[0].exit, got[1].surface, got[1].enter)
	}
}
