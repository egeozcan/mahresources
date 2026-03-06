package api_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPluginAPI_OversizedContentLength(t *testing.T) {
	tc := SetupTestEnv(t)

	// Send a request with Content-Length larger than the allowed limit (1MB)
	body := strings.NewReader("{}")
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 2 * 1024 * 1024 // 2MB declared

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "too large") {
		t.Errorf("expected error containing 'too large', got %q", resp["error"])
	}
}

func TestPluginAPI_BodyReadError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Use a reader that returns an error
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", &errorReader{err: io.ErrUnexpectedEOF})
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for body read error, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "read") {
		t.Errorf("expected error containing 'read', got %q", resp["error"])
	}
}

func TestPluginAPI_MethodNotAllowedAtRouteLevel(t *testing.T) {
	tc := SetupTestEnv(t)

	// PATCH should be rejected (only GET/POST/PUT/DELETE allowed)
	req, _ := http.NewRequest("PATCH", "/v1/plugins/some-plugin/data", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// Should get 405 Method Not Allowed
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for PATCH, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestPluginAPI_UnknownPluginReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// No plugin named "some-plugin" exists, should get 404
	req, _ := http.NewRequest("GET", "/v1/plugins/some-plugin/data", nil)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "plugin not found" {
		t.Errorf("expected error 'plugin not found', got %q", resp["error"])
	}
}

func TestPluginAPI_ReservedPathManageReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// "manage" is a reserved path prefix — should not be treated as a plugin name
	req, _ := http.NewRequest("GET", "/v1/plugins/manage/something", nil)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// The explicit /v1/plugins/manage route returns plugin list (200) for GET,
	// but /v1/plugins/manage/something should hit the catch-all and return 404
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for reserved path, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestPluginAPI_ActualBodyExceedsLimit(t *testing.T) {
	tc := SetupTestEnv(t)

	// Send a body that's larger than 1MB but without setting Content-Length
	largeBody := bytes.Repeat([]byte("x"), 1<<20+100) // slightly over 1MB
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // unknown length (chunked)

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// errorReader always returns the given error on Read.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}
