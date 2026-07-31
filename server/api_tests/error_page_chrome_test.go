//go:build json1 && fts5

package api_tests

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/server"
)

// Phase 3 guard for the leverage point behind roughly ten findings of the
// 2026-07-29 UI bug hunt (docs/todo.md, WS3).
//
// `HandleError` (server/http_utils/http_helpers.go) is called from 481 sites,
// and for an HTML-accepting request it used to write a self-contained inline
// document: no nav, no header, and one `javascript:history.back()` link. Submit
// a form with a validation error from the tag page, the resource page or the
// admin pages and the reader was ejected from the application onto a dead end.
//
// ws3_error_surface_test.go pins one call site. This is the sweep: **every**
// parameterless POST under /v1/ that rejects an empty submission with a 4xx must
// render the app shell. Routes come from the router, so a handler added later is
// covered without anyone extending a list.

// sideEffectingPOSTs are the parameterless POSTs this sweep must not fire blind.
// Each entry MUST say why. They are covered by their own tests.
var sideEffectingPOSTs = map[string]string{
	// Start real transfers or long-running background work on a valid body, and
	// the sweep sends a body it has not reasoned about.
	"/v1/resource/remote":                   "downloads a remote URL",
	"/v1/download/submit":                   "enqueues a remote download",
	"/v1/jobs/download/submit":              "enqueues a remote download",
	"/v1/admin/similarity/recompute":        "kicks off a full similarity recompute",
	"/v1/admin/similarity/retry-failed":     "kicks off similarity work",
	"/v1/mrql/generate":                     "calls the DeepSeek API",
	"/v1/category/generateTemplate":         "calls the DeepSeek API",
	"/v1/noteType/generateTemplate":         "calls the DeepSeek API",
	"/v1/resourceCategory/generateTemplate": "calls the DeepSeek API",

	// Logging out is stateless here but the response is a redirect, so it never
	// reaches the 4xx branch this test measures; listing it keeps the intent
	// explicit rather than relying on the status filter.
	"/v1/auth/logout": "answers a redirect, never a 4xx",
}

// TestHTMLAcceptingRejectionsKeepTheAppShell sweeps every parameterless /v1/
// POST with an empty form body and a browser Accept header.
func TestHTMLAcceptingRejectionsKeepTheAppShell(t *testing.T) {
	tc := SetupTestEnv(t)

	paths := parameterlessV1POSTs(t, tc)
	if len(paths) < 80 {
		t.Fatalf("route walk found only %d parameterless /v1/ POSTs; expected over a hundred. "+
			"The sweep is not looking at the handlers it claims to guard.", len(paths))
	}

	live := map[string]bool{}
	for _, p := range paths {
		live[p] = true
	}
	for path, reason := range sideEffectingPOSTs {
		if !live[path] {
			t.Errorf("sideEffectingPOSTs[%q] (%s) is not a registered parameterless /v1/ POST "+
				"any more. Remove it, or the exclusion is a hole rather than a decision.", path, reason)
		}
	}

	rejections, htmlRejections := 0, 0
	for _, path := range paths {
		if _, skip := sideEffectingPOSTs[path]; skip {
			continue
		}
		resp := tc.requestWithAccept(http.MethodPost, path, browserAccept, "unused=1")
		if resp.Code < 400 || resp.Code >= 500 {
			continue // not a rejection; nothing for this guard to measure
		}
		rejections++
		body := resp.Body.String()

		// The rule is about pages, not about every rejection. Four endpoints
		// answer a non-HTML body whatever the Accept header asks for, and that
		// is correct: /v1/auth/login is consumed by the login form's fetch and
		// renders its message inline, and the three group export/import routes
		// are driven by adminExport.js / adminImport.js, whose
		// errorMessageFromResponse reads a plain-text body. None of them ever
		// navigates a browser. What must never happen again is HTML that is not
		// the application.
		if !strings.Contains(resp.Header().Get("Content-Type"), "text/html") {
			continue
		}
		htmlRejections++

		if strings.Contains(body, "error-container") {
			t.Errorf("POST %s (%d) rendered the standalone inline error document.\n"+
				"HandleError's HTML branch must extend the app layout, or the reader is\n"+
				"ejected from the application by a validation mistake.", path, resp.Code)
			continue
		}
		if strings.Contains(body, "javascript:history.back()") {
			t.Errorf("POST %s (%d) offers javascript:history.back() as a way out.\n"+
				"That is not a link, it does not work from a fresh tab, and it was the\n"+
				"only escape the old error page had.", path, resp.Code)
		}
		if !strings.Contains(body, `href="/dashboard"`) {
			t.Errorf("POST %s (%d) renders no in-app recovery link.\n"+
				"body: %s", path, resp.Code, truncateBody(body, 300))
		}
		if !strings.Contains(body, "<nav") {
			t.Errorf("POST %s (%d) renders no nav landmark, so the error page is not the app.",
				path, resp.Code)
		}
	}

	// Positive control. This whole sweep is a set of assertions about pages that
	// only exist when a handler rejects something; if nothing rejected, every one
	// of them held for free.
	if rejections < 40 {
		t.Errorf("only %d of %d endpoints rejected an empty submission with a 4xx; "+
			"expected dozens. Either the sweep is not reaching the handlers or they "+
			"stopped validating.", rejections, len(paths))
	}
	// The second half of the control, and the one that matters: the assertions
	// above only run on an HTML body. If HandleError stopped honouring Accept
	// and answered JSON to everyone, every one of them would hold for free while
	// the app's error surface silently disappeared.
	if htmlRejections < 40 {
		t.Errorf("only %d of %d rejections rendered HTML to an HTML-accepting client; "+
			"expected dozens. The chrome assertions above measured almost nothing.",
			htmlRejections, rejections)
	}
}

// parameterlessV1POSTs walks the real router for POST routes under /v1/ with no
// {variable} segments.
func parameterlessV1POSTs(t *testing.T, tc *TestContext) []string {
	t.Helper()

	router := server.BuildPrimaryRouter(tc.AppCtx, afero.NewMemMapFs(), map[string]string{})

	seen := map[string]bool{}
	var out []string
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, perr := route.GetPathTemplate()
		if perr != nil || !strings.HasPrefix(path, "/v1/") || strings.Contains(path, "{") {
			return nil
		}
		methods, _ := route.GetMethods()
		for _, m := range methods {
			if m == http.MethodPost && !seen[path] {
				seen[path] = true
				out = append(out, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("router walk: %v", err)
	}
	sort.Strings(out)
	return out
}
