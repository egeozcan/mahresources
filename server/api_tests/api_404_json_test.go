//go:build json1 && fts5

package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Phase 3 guard for finding 114 of the 2026-07-29 UI bug hunt (docs/todo.md).
//
// RenderNotFound always wrote text/html with no path-prefix negotiation, so an
// unmatched URL under /v1/ answered a full HTML 404 document to a JSON client.
// `not_found_test.go:40` used to *document* that ("The not found handler
// currently only renders HTML"); WS3 inverted it and pinned three paths.
//
// This is the sweep the plan asked for: **every** unmatched path under /v1/
// answers JSON, whoever asks. The paths are derived from the router's own route
// table — one near-miss per registered route — so a namespace added later is
// covered without anyone remembering to extend a list.

// TestUnmatchedV1PathsAlwaysAnswerJSON derives a near-miss for every registered
// /v1/ route and asserts each answers a JSON 404.
func TestUnmatchedV1PathsAlwaysAnswerJSON(t *testing.T) {
	tc := SetupTestEnv(t)

	// One unmatched sibling per registered route, plus the shapes a typo takes:
	// a wrong segment, a trailing segment, and a bare namespace.
	probes := map[string]bool{
		"/v1/":                     true,
		"/v1/nonexistent-endpoint": true,
		"/v1/resources/count":      true, // plausible but unregistered
		"/v1/jobs":                 true,
		"/v1/Resources":            true, // wrong case
		"/v1/resource//preview":    true,
	}
	for _, path := range parameterlessV1GETs(t, tc) {
		probes[path+"/no-such-sub-resource"] = true
		probes[strings.TrimSuffix(path, "/")+"X"] = true
	}

	// Both an explicit JSON client and a browser: the rule is keyed on the path
	// prefix, not on the Accept header, because /v1 is the JSON API whoever is
	// asking. (A browser that lands on /v1/typo gets JSON on purpose.)
	accepts := map[string]string{
		"json":    "application/json",
		"browser": browserAccept,
		"none":    "",
	}

	checked := 0
	for path := range probes {
		for label, accept := range accepts {
			resp := tc.requestWithAccept(http.MethodGet, path, accept, "")
			if resp.Code != http.StatusNotFound {
				// A near-miss that happens to match a real route is not this
				// test's business; skip it rather than assert on a live handler.
				continue
			}
			checked++
			ct := resp.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("GET %s (Accept: %s) answered 404 as %q.\n"+
					"Everything under /v1/ is the JSON API; an HTML 404 here reaches a\n"+
					"fetch() as an unparseable body and surfaces as a generic failure.",
					path, label, ct)
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Errorf("GET %s (Accept: %s): 404 body does not parse as JSON: %s",
					path, label, truncateBody(resp.Body.String(), 200))
				continue
			}
			if msg, _ := payload["error"].(string); msg == "" {
				t.Errorf("GET %s (Accept: %s): 404 body carries no \"error\" message: %v",
					path, label, payload)
			}
		}
	}

	if checked < 100 {
		t.Errorf("only %d unmatched-path responses were examined; expected well over 100. "+
			"Every probe matched a live route, so this guard asserted nothing.", checked)
	}
}

// TestUnmatchedNonV1PathsStillAnswerHTML is the positive control, and it is the
// half that stops the fix from being "make every 404 JSON". A reader who
// mistypes a page URL must still get the app's own 404 page — that is finding
// 119's whole point, and it is served by the same handler.
func TestUnmatchedNonV1PathsStillAnswerHTML(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/does-not-exist", "/group/nope", "/admin/nope"} {
		resp := tc.requestWithAccept(http.MethodGet, path, browserAccept, "")
		if resp.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404 — the control needs a real 404 to measure", path, resp.Code)
		}
		if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s answered 404 as %q; a browser outside /v1 must get the app's 404 page", path, ct)
		}
		if !strings.Contains(resp.Body.String(), "Dashboard") {
			t.Errorf("GET %s: the 404 page has lost the app nav", path)
		}
	}
}
