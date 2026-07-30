package api_tests

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server"
)

// WS13 — sharing, from the 2026-07-29 UI bug hunt.
//
//	7  — POST /v1/note/share minted a token with no check that anything serves /s/
//	51 — a share-server bind failure was logged and swallowed
//
// 128 (Unshare with no confirmation) is browser-only: the confirm lives in an
// Alpine handler, so it is pinned in e2e/tests/regressions/ws13-sharing.spec.ts.

// setupShareEnabledTestEnv is SetupTestEnv with a share port configured, for tests
// that are about shared notes rather than about the availability gate.
//
// Finding 7 made POST /v1/note/share answer 503 when no share server is
// configured, and SetupTestEnv leaves SharePort empty — so every test that shares
// a note as *setup* has to say it wants sharing. The port is never bound; only
// ShareEnabled() reads it.
// setupShareEnabledTestEnv stands in for a running share server: a configured port
// *and* the listening marker server.ShareServer.Start sets once net.Listen has
// succeeded. Both are needed, because ShareEnabled() is not the absence of an
// observed failure — a context that merely has a port configured has no share
// server, and minting a token for it is finding 7.
func setupShareEnabledTestEnv(t *testing.T) *TestContext {
	t.Helper()
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.SharePort = "18399"
		c.ShareBindAddress = "127.0.0.1"
	})
	tc.AppCtx.MarkShareServerListening()
	return tc
}

// Finding 7. share_handlers.go called ShareNote and returned "/s/" + token with no
// gate. Measured against an instance started exactly the way the report describes
// (SHARE_PORT unset): POST /v1/note/share answered
//
//	200 {"shareToken":"e32d5abd…","shareUrl":"/s/e32d5abd…"}
//
// GET /s/<token> on the only running server answered 404, and /admin/shares listed
// the note as shared. The note *sidebar* has been gated on ShareEnabled() for a
// while — with the port unset the page renders zero "Share Note" buttons — so the
// UI half of this finding was already fixed and the endpoint was the whole of what
// was left.
func TestShareEndpointIsRefusedWhenNoShareServerIsRunning(t *testing.T) {
	tc := SetupTestEnv(t) // SetupTestEnv leaves SharePort empty
	note := tc.CreateDummyNote("ws13 unshareable note")

	if tc.AppCtx.ShareEnabled() {
		t.Fatal("sharing reports enabled with no share port configured — precondition wrong")
	}

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if resp.Code == http.StatusOK {
		t.Errorf("a share token was minted with no share server: %s", resp.Body.String())
	}
	if resp.Code != http.StatusServiceUnavailable {
		t.Errorf("POST /v1/note/share = %d, want 503", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "share") {
		t.Errorf("the refusal does not explain itself: %s", resp.Body.String())
	}

	// Nothing was persisted, so /admin/shares cannot list a note as shared.
	var reloaded models.Note
	if err := tc.DB.First(&reloaded, note.ID).Error; err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if reloaded.ShareToken != nil && *reloaded.ShareToken != "" {
		t.Errorf("the note was marked shared anyway: token %q", *reloaded.ShareToken)
	}
}

// Positive control for finding 7: with a share port configured the endpoint works
// exactly as before. Without this, "sharing is refused" is satisfied by an endpoint
// that never works.
func TestShareEndpointStillWorksWhenSharingIsConfigured(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.SharePort = "18384"
		c.ShareBindAddress = "127.0.0.1"
	})
	tc.AppCtx.MarkShareServerListening()
	note := tc.CreateDummyNote("ws13 shareable note")

	if !tc.AppCtx.ShareEnabled() {
		t.Fatal("sharing reports disabled with a share port configured and no failure observed")
	}

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/note/share = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, _ := out["shareToken"].(string)
	if len(token) != 32 {
		t.Errorf("shareToken = %q, want a 32-character hex token", token)
	}

	// Revoking must stay available whatever ShareEnabled() says — a note already
	// marked shared has to be un-shareable even if the share server has since died.
	tc.AppCtx.MarkShareServerFailed()
	if tc.AppCtx.ShareEnabled() {
		t.Error("ShareEnabled() is still true after a share-server failure")
	}
	unshare := tc.MakeRequest(http.MethodDelete, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if unshare.Code != http.StatusOK {
		t.Errorf("DELETE /v1/note/share = %d after a share-server failure, want 200: %s",
			unshare.Code, unshare.Body.String())
	}
}

// Finding 7, the door Batch 11 left open. ShareEnabled() was
// `ShareConfigured() && !ShareServerFailed()`, and the failure flag starts false —
// so a context built with SharePort set whose share server was never started
// reported "no failure observed" and enabled sharing. main.go is not the only
// caller of CreateServer (tests, tooling, and any embedder are others), and for
// every one of them the endpoint would mint a token for a /s/ route no process
// serves, which is finding 7 word for word.
//
// The predicate is positive now: a share server must have said it is listening.
func TestShareIsRefusedWhenAPortIsConfiguredButNoServerWasStarted(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.SharePort = "18397"
		c.ShareBindAddress = "127.0.0.1"
	})
	note := tc.CreateDummyNote("ws13 configured-but-unstarted")

	if !tc.AppCtx.ShareConfigured() {
		t.Fatal("the port did not reach the config — this test measured nothing")
	}
	if tc.AppCtx.ShareEnabled() {
		t.Error("ShareEnabled() is true for a share server that was never started")
	}

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if resp.Code != http.StatusServiceUnavailable {
		t.Errorf("POST /v1/note/share = %d, want 503 — a token whose /s/ URL nothing "+
			"serves must not be minted: %s", resp.Code, resp.Body.String())
	}

	// Positive control: the same context with the listening marker set — which is
	// what ShareServer.Start does after net.Listen succeeds — shares fine. Without
	// it, "503" is satisfied by an endpoint that refuses unconditionally.
	tc.AppCtx.MarkShareServerListening()
	if !tc.AppCtx.ShareEnabled() {
		t.Fatal("ShareEnabled() is false with a configured port and a listening server")
	}
	ok := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if ok.Code != http.StatusOK {
		t.Errorf("POST /v1/note/share = %d once the share server is listening, want 200: %s",
			ok.Code, ok.Body.String())
	}
}

// Finding 51. share_server.go ran ListenAndServe in a goroutine and returned nil
// regardless, so main.go's log.Fatalf was unreachable. Reproduced verbatim on the
// unfixed binary by holding the port first — note the ordering, which is what made
// it invisible:
//
//	Share server starting on 127.0.0.1:8384
//	Share server available at http://127.0.0.1:8384
//	Share server error: listen tcp 127.0.0.1:8384: bind: address already in use
//
// The process stayed up on :8273 and answered 301, /admin/settings went on
// advertising "Share port 8384", /v1/logs held exactly one entry (a note create)
// and nothing about the share server, and every /s/<token> was dead.
func TestShareServerStartReturnsABindFailure(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
	})

	// Take a port and hold it, so the share server cannot have it.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not open a blocking listener: %v", err)
	}
	defer blocker.Close()
	_, port, err := net.SplitHostPort(blocker.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	ss := server.NewShareServer(tc.AppCtx)
	startErr := ss.Start("127.0.0.1", port)
	if startErr == nil {
		t.Fatalf("Start returned nil for a port already in use (%s) — main.go's log.Fatalf "+
			"can never fire and the deployment comes up advertising a dead share port", port)
	}
	if !strings.Contains(startErr.Error(), port) {
		t.Errorf("the error does not name the port it could not bind: %v", startErr)
	}

	if tc.AppCtx.ShareServerListening() {
		t.Error("the context reports a listening share server after a bind failure, " +
			"so the UI cannot know sharing is dead")
	}
	if tc.AppCtx.ShareEnabled() {
		t.Error("ShareEnabled() is true after a bind failure")
	}
}

// Positive control for 51: a free port binds, returns nil, and leaves the context
// healthy. Without it, "Start returns an error" is satisfied by a Start that always
// fails.
func TestShareServerStartSucceedsOnAFreePort(t *testing.T) {
	// Ask the kernel for a free port, then release it immediately. A tiny race is
	// possible and acceptable: this is a control, and a spurious failure here would
	// name the bind error rather than mislead.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	_, port, _ := net.SplitHostPort(probe.Addr().String())
	probe.Close()

	// The config's SharePort has to match, because ShareEnabled() reads it — "an
	// operator asked for sharing" and "the bind worked" are two different facts and
	// the predicate needs both.
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.ShareBindAddress = "127.0.0.1"
		c.SharePort = port
	})

	ss := server.NewShareServer(tc.AppCtx)
	if err := ss.Start("127.0.0.1", port); err != nil {
		t.Fatalf("Start on the free port %s returned %v", port, err)
	}
	defer ss.Stop()

	if !tc.AppCtx.ShareServerListening() {
		t.Error("the context was not marked listening after a successful bind")
	}
	if !tc.AppCtx.ShareEnabled() {
		t.Error("ShareEnabled() is false after a successful bind")
	}
}

// Finding 51's UI half. With the port configured but the server dead, the note
// sidebar must not offer to mint a new link, must say the existing one does not
// work, and must still offer Unshare — hiding the panel outright would leave a note
// publicly marked shared with no way to revoke it.
func TestNotePageExplainsADeadShareServer(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.SharePort = "18385"
		c.ShareBindAddress = "127.0.0.1"
	})
	tc.AppCtx.MarkShareServerListening()
	note := tc.CreateDummyNote("ws13 dead share note")

	// Share it while sharing works, then kill the share server.
	share := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID), nil)
	if share.Code != http.StatusOK {
		t.Fatalf("could not share the note (%d): %s — this test measured nothing",
			share.Code, share.Body.String())
	}
	tc.AppCtx.MarkShareServerFailed()

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/note?id=%d", note.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /note = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	if !strings.Contains(body, "share-server-down-warning") {
		t.Error("the sidebar does not warn that the share link is dead")
	}
	if !strings.Contains(body, "Unshare") {
		t.Error("the sidebar dropped Unshare, so a shared note cannot be revoked")
	}
	if strings.Contains(body, "Share Note") {
		t.Error("the sidebar still offers to mint another dead link")
	}

	// /admin/settings must not go on advertising the port as if it worked.
	settings := tc.MakeRequest(http.MethodGet, "/admin/settings", nil)
	if settings.Code != http.StatusOK {
		t.Fatalf("GET /admin/settings = %d", settings.Code)
	}
	if !strings.Contains(settings.Body.String(), "NOT serving") {
		t.Error("/admin/settings shows the share port with no indication nothing is listening")
	}
}

// The healthy-path control for the two assertions above.
func TestNotePageOffersSharingWhenTheShareServerIsHealthy(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.SharePort = "18386"
		c.ShareBindAddress = "127.0.0.1"
	})
	tc.AppCtx.MarkShareServerListening()
	note := tc.CreateDummyNote("ws13 healthy share note")

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/note?id=%d", note.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /note = %d, want 200", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "Share Note") {
		t.Error("a healthy deployment does not offer Share Note")
	}
	if strings.Contains(body, "share-server-down-warning") {
		t.Error("a healthy deployment warns that the share server is down")
	}
}
