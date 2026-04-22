package api_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/server"
)

// TestShareServer_SecurityHeaders verifies BH-032: the share server must set a
// baseline set of security headers on shared-note responses. Specifically:
//   - X-Frame-Options: DENY (clickjacking protection)
//   - X-Content-Type-Options: nosniff (MIME type sniffing off)
//   - Referrer-Policy: no-referrer (stops share tokens leaking via the Referer
//     header when a shared note embeds an external-hosted image or font)
//   - Content-Security-Policy: set (strict default-src 'self')
//   - Strict-Transport-Security: set
func TestShareServer_SecurityHeaders(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("BH-032 share headers")
	token, err := tc.AppCtx.ShareNote(note.ID)
	if err != nil {
		t.Fatalf("share note: %v", err)
	}

	ss := server.NewShareServer(tc.AppCtx)
	handler := ss.Handler()
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	required := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	for hdr, want := range required {
		got := w.Header().Get(hdr)
		if got != want {
			t.Errorf("%s: expected %q, got %q", hdr, want, got)
		}
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header missing")
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("Strict-Transport-Security header missing")
	}
}

// TestPrimaryServer_SecurityHeaders verifies BH-032: the primary server applies
// the CSP-free subset of the share server's security headers on every
// response (clickjacking, MIME sniffing, Referer suppression, HSTS). The
// strict CSP the share server ships is intentionally NOT applied to the
// primary — the primary's template set includes inline scripts emitted by
// shortcodes and plugin-provided HTML that a default-src 'self' CSP
// rejects. A tighter primary-server CSP is tracked as a follow-up so it
// can be rolled out independently.
func TestPrimaryServer_SecurityHeaders(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/dashboard", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /dashboard returned %d: %s", resp.Code, resp.Body.String())
	}

	required := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	for hdr, want := range required {
		got := resp.Header().Get(hdr)
		if got != want {
			t.Errorf("%s: expected %q, got %q", hdr, want, got)
		}
	}
	if resp.Header().Get("Strict-Transport-Security") == "" {
		t.Error("Strict-Transport-Security header missing")
	}
	// CSP is deliberately NOT applied to the primary server yet — see the
	// withPrimarySecurityHeaders docstring for the rationale.
	if got := resp.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("primary server must not ship Content-Security-Policy yet (follow-up work): got %q", got)
	}
}

// TestShareServer_SecurityHeaders_ErrorPath verifies headers are applied even on
// 404s so a forged/expired token doesn't bypass nosniff. BH-032.
func TestShareServer_SecurityHeaders_ErrorPath(t *testing.T) {
	tc := SetupTestEnv(t)
	ss := server.NewShareServer(tc.AppCtx)
	req := httptest.NewRequest(http.MethodGet, "/s/doesnotexist", nil)
	w := httptest.NewRecorder()
	ss.Handler().ServeHTTP(w, req)

	if !strings.EqualFold(w.Header().Get("X-Content-Type-Options"), "nosniff") {
		t.Error("nosniff missing on error path")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("X-Frame-Options missing on error path")
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Error("Referrer-Policy missing on error path")
	}
}
