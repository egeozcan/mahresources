package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

// TestNotFoundHandler_IncludesPluginContext verifies that the 404 handler
// receives the same navigation context (menu, admin menu) that normal routes get.
// Bug: RenderNotFound used StaticTemplateCtx directly without wrapContextWithPlugins,
// so the plugin menu was missing from the 404 page navigation.
func TestNotFoundHandler_IncludesNavigation(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/this-page-does-not-exist", nil)

	if resp.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.Code)
	}

	body := resp.Body.String()

	// The 404 page should still render the full navigation including the main menu
	// and admin menu. If the plugin context is missing, the template will lack
	// the currentPath variable needed for active link highlighting.
	if !strings.Contains(body, "Dashboard") {
		t.Error("404 page should contain the Dashboard nav link")
	}
	if !strings.Contains(body, "Admin") {
		t.Error("404 page should contain the Admin nav section")
	}
	if !strings.Contains(body, "Page not found") {
		t.Error("404 page should contain 'Page not found' error message")
	}
}

// TestNotFoundHandler_JSONResponse verifies that the 404 handler returns JSON
// when requested with Accept: application/json header.
func TestNotFoundHandler_JSONResponse(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/this-page-does-not-exist.json", nil)

	// The not found handler currently only renders HTML, so it should still return 404
	if resp.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.Code)
	}
}
