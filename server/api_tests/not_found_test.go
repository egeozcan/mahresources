package api_tests

import (
	"encoding/json"
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

// TestNotFoundHandler_JSONResponse verifies that the 404 handler answers JSON for
// a request that asked for JSON.
//
// This test used to document the opposite ("The not found handler currently only
// renders HTML"). Finding 114 of docs/ui-bug-hunt-2026-07-29.md is that
// behaviour: an unmatched path returned the full HTML 404 page, so any client's
// JSON parse blew up instead of surfacing a clean 404. The assertion is inverted
// rather than deleted, and TestNotFoundHandler_IncludesNavigation above is the
// control that the HTML branch still renders the app chrome.
func TestNotFoundHandler_JSONResponse(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodGet, "/this-page-does-not-exist.json", nil)

	if resp.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected a JSON content type, got %q", ct)
	}
	body := resp.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("a .json path must not answer with the HTML 404 page")
	}
	var payload map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body must parse as JSON: %v (body %q)", err, body)
	}
	if payload["error"] == "" {
		t.Errorf(`expected an "error" key, got %v`, payload)
	}
}
