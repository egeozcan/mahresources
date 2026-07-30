package api_tests

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/server"
)

// WS13 round 3 — the share server's lifecycle after Start.
//
// Round 2 made the bind synchronous and recorded a positive "listening" fact, and
// stopped there: Start was the only half of the lifecycle anyone looked at. Stop
// never cleared the flag, so a process that shut its share server down went on
// telling every note page that sharing worked; the Serve goroutine dereferenced
// the mutable s.server rather than the server it was handed, so a restart could
// serve one listener through another server's handler; and neither Start nor Stop
// took a lock, so two of them racing could leave the flag disagreeing with what
// was actually bound.

// freeShareTestPort asks the kernel for a port and releases it. A tiny race is
// possible and acceptable — a lost race fails with a bind error that names itself.
func freeShareTestPort(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	_, port, _ := net.SplitHostPort(probe.Addr().String())
	probe.Close()
	return port
}

// Stop shuts the listener down, so nothing serves /s/<token> any more — and the
// context has to say so, or every note page goes on advertising a link that will
// not resolve. This is finding 7 arriving through the back door: the check exists
// and the state it reads is stale.
func TestShareServerStopClearsTheListeningFlag(t *testing.T) {
	port := freeShareTestPort(t)
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
		c.SharePort = port
	})

	ss := server.NewShareServer(tc.AppCtx)
	if err := ss.Start("127.0.0.1", port); err != nil {
		t.Fatalf("Start on the free port %s returned %v", port, err)
	}

	// Positive control: sharing really was on before Stop, so the assertion below
	// is about Stop and not about a context that never reported healthy.
	if !tc.AppCtx.ShareEnabled() {
		t.Fatalf("ShareEnabled() is false right after a successful Start — this test measured nothing")
	}

	if err := ss.Stop(); err != nil {
		t.Fatalf("Stop returned %v", err)
	}

	if tc.AppCtx.ShareServerListening() {
		t.Error("the context still reports a listening share server after Stop")
	}
	if tc.AppCtx.ShareEnabled() {
		t.Error("ShareEnabled() is still true after Stop, so the note sidebar goes on " +
			"offering /s/ links nothing serves")
	}
}

// The endpoint half of the same fact: after Stop, POST /v1/note/share must refuse
// rather than mint a token for a URL nothing answers.
func TestShareEndpointIsRefusedAfterTheShareServerStops(t *testing.T) {
	port := freeShareTestPort(t)
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
		c.SharePort = port
	})

	ss := server.NewShareServer(tc.AppCtx)
	if err := ss.Start("127.0.0.1", port); err != nil {
		t.Fatalf("Start returned %v", err)
	}

	note := tc.CreateDummyNote("ws13r3 share lifecycle note")

	// Positive control: sharing works while the server is up, so the 503 below is
	// caused by Stop and not by a note the endpoint always refuses.
	ok := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("share while listening = %d, want 200: %s — this test measured nothing",
			ok.Code, ok.Body.String())
	}

	if err := ss.Stop(); err != nil {
		t.Fatalf("Stop returned %v", err)
	}

	other := tc.CreateDummyNote("ws13r3 share lifecycle note 2")
	after := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", other.ID), nil)
	if after.Code != http.StatusServiceUnavailable {
		t.Errorf("share after Stop = %d, want 503: %s", after.Code, after.Body.String())
	}
}

// Stop then Start again. Two things have to hold: the flag comes back, and the
// listener that is serving belongs to the server that opened it. The second is
// what the captured-server fix is about — the Serve goroutine used to read
// s.server, which Start had already replaced.
func TestShareServerRestartsAndServesThroughTheNewServer(t *testing.T) {
	firstPort := freeShareTestPort(t)
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
		c.SharePort = firstPort
	})

	ss := server.NewShareServer(tc.AppCtx)
	if err := ss.Start("127.0.0.1", firstPort); err != nil {
		t.Fatalf("first Start returned %v", err)
	}
	if err := ss.Stop(); err != nil {
		t.Fatalf("Stop returned %v", err)
	}
	if tc.AppCtx.ShareServerListening() {
		t.Fatalf("still listening after Stop — covered by its own test; this one needs it false")
	}

	secondPort := freeShareTestPort(t)
	if err := ss.Start("127.0.0.1", secondPort); err != nil {
		t.Fatalf("restart on %s returned %v", secondPort, err)
	}
	defer ss.Stop()

	if !tc.AppCtx.ShareServerListening() {
		t.Error("the context is not marked listening after a restart")
	}

	// The restarted server really answers on the new port. A 404 for an unknown
	// token is the right answer and proves the route is wired; a connection error
	// is what a listener served by the wrong server produces.
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/s/no-such-token", secondPort))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("GET /s/no-such-token on the restarted server = %d, want 404", resp.StatusCode)
			}
			// The old port must be closed, or Stop did not stop anything.
			if old, oldErr := http.Get(fmt.Sprintf("http://127.0.0.1:%s/s/no-such-token", firstPort)); oldErr == nil {
				old.Body.Close()
				t.Errorf("the first port %s still answers after Stop", firstPort)
			}
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("the restarted share server never answered on %s: %v", secondPort, lastErr)
}

// Start and Stop from several goroutines at once. The assertion is the agreement
// between the flag and reality: whatever the last operation was, the context must
// not claim a listening server the process does not have. Run this with -race.
func TestShareServerStartStopIsSafeUnderConcurrency(t *testing.T) {
	port := freeShareTestPort(t)
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
		c.SharePort = port
	})

	ss := server.NewShareServer(tc.AppCtx)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				// A second bind of the same port fails, which is a legitimate
				// outcome here; the point is that it does not corrupt state.
				_ = ss.Start("127.0.0.1", port)
			} else {
				_ = ss.Stop()
			}
		}(i)
	}
	wg.Wait()

	// Settle: stop, then start once, and require the flag to match.
	_ = ss.Stop()
	if tc.AppCtx.ShareServerListening() {
		t.Error("listening flag is set after a final Stop")
	}
	if err := ss.Start("127.0.0.1", port); err != nil {
		t.Fatalf("final Start returned %v", err)
	}
	defer ss.Stop()
	if !tc.AppCtx.ShareServerListening() {
		t.Error("listening flag is clear after a final successful Start")
	}
}
