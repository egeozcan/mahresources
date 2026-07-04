package api_tests

import (
	"net/http"
	"testing"
)

// TestPublicAssetsCORSHeader verifies /public/ responses carry a wildcard
// Access-Control-Allow-Origin header. The template live-preview pane renders
// into a sandboxed iframe (opaque origin), and module scripts are always
// fetched in CORS mode — without this header the browser blocks the app
// bundle inside the preview and nothing hydrates. /public/ is auth-exempt
// static content, so the wildcard exposes nothing that was not already
// world-readable.
func TestPublicAssetsCORSHeader(t *testing.T) {
	tc := SetupTestEnv(t)

	// The header must be present regardless of whether the asset exists on
	// disk (the test working directory has no ./public), because the browser
	// applies the CORS check to error responses too.
	rr := tc.MakeRequest(http.MethodGet, "/public/dist/main.js", nil)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*' on /public asset, got %q (status %d)", got, rr.Code)
	}

	// Non-public routes must not grow the header.
	rr = tc.MakeRequest(http.MethodGet, "/v1/groups", nil)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin on API route, got %q", got)
	}
}
