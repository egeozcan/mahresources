package api_tests

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// WS3, second half: three surfaces that had a message and rendered it badly —
// the query runner (100), the duplicate-upload banner (103) and the import
// parser (106). The import parser's own message is covered where it is produced,
// in archive/reader_message_test.go.

// ---------------------------------------------------------------------------
// 100 — the query runner renders the server's message, not its response body
// ---------------------------------------------------------------------------

// The runner lives in an inline Alpine expression in displayQuery.tpl, so this
// asserts the rendered markup. The behaviour it stands for is covered by
// e2e/tests/regressions/ws3-error-surfaces.spec.ts.
func TestQueryRunner_ReadsTheErrorMessageNotTheBody(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/query",
		"application/json",
		"Name=ws3-broken-query&Text="+url.QueryEscape("SELECT * FROM nonexistent_table_xyz"))
	if resp.Code != http.StatusOK {
		t.Fatalf("could not create query: %d %s", resp.Code, resp.Body.String())
	}

	body := tc.requestWithAccept(http.MethodGet, "/query?id=1", browserAccept, "").Body.String()
	// Positive control: this really is the query page.
	if !strings.Contains(body, "ws3-broken-query") {
		t.Fatalf("control failed: /query?id=1 is not the query just created")
	}
	if strings.Contains(body, "x.text()") {
		t.Error("the runner should parse the JSON error, not render the raw response body")
	}
	if !strings.Contains(body, "errorMessageFromResponse") {
		t.Error("the runner should route failures through the shared message reader")
	}
}

// ---------------------------------------------------------------------------
// 103 — the duplicate-upload error links to the resource it collided with
// ---------------------------------------------------------------------------

func TestDuplicateUpload_LinksToTheCollidingResource(t *testing.T) {
	tc := SetupTestEnv(t)

	payload := []byte("ws3-duplicate-upload-fixture-bytes")

	body, contentType := makeMultipartUpload(t, "resource", "ws3-dup.txt", payload, map[string]string{"Name": "ws3 original"})
	first := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, contentType)
	if first.Code != http.StatusOK {
		t.Fatalf("first upload should succeed, got %d: %s", first.Code, truncate(first.Body.String()))
	}

	body2, contentType2 := makeMultipartUpload(t, "resource", "ws3-dup.txt", payload, map[string]string{"Name": "ws3 duplicate"})
	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", body2)
	req.Header.Set("Content-Type", contentType2)
	req.Header.Set("Accept", browserAccept)
	second := httptest.NewRecorder()
	tc.Router.ServeHTTP(second, req)

	if second.Code != http.StatusFound {
		t.Fatalf("expected a redirect back to the form, got %d: %s", second.Code, truncate(second.Body.String()))
	}
	location := second.Header().Get("Location")

	if strings.Contains(location, "following+errors+were+encountered") {
		t.Error(`"following errors were encountered" is broken grammar and internal wording`)
	}
	if strings.Contains(location, "same+parent") {
		t.Error(`"same parent" is internal jargon`)
	}
	if !strings.Contains(location, "errorResourceId=") {
		t.Errorf("the rejection should name the colliding resource so the form can link it: %q", location)
	}

	// And the form renders that id as a link, not as bare text.
	form := tc.requestWithAccept(http.MethodGet, location, browserAccept, "").Body.String()
	// Positive control: the banner is on the page at all.
	if !strings.Contains(form, `data-testid="form-error-banner"`) {
		t.Fatal("control failed: no error banner rendered")
	}
	if !strings.Contains(form, `href="/resource?id=1"`) {
		t.Errorf("the duplicate banner should link to the colliding resource; banner was:\n%s", extractBanner(form))
	}
}

// TestDuplicateUpload_JSONCallersKeepTheStructuredDetail is the control that the
// wording change did not disturb the API shape: fetch callers read `details[]`
// with a `resourceId`, which is where the id belonged all along.
func TestDuplicateUpload_JSONCallersKeepTheStructuredDetail(t *testing.T) {
	tc := SetupTestEnv(t)

	payload := []byte("ws3-duplicate-json-fixture-bytes")
	body, contentType := makeMultipartUpload(t, "resource", "ws3-dup-json.txt", payload, map[string]string{"Name": "ws3 json original"})
	if first := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, contentType); first.Code != http.StatusOK {
		t.Fatalf("first upload should succeed, got %d", first.Code)
	}

	body2, contentType2 := makeMultipartUpload(t, "resource", "ws3-dup-json.txt", payload, map[string]string{"Name": "ws3 json duplicate"})
	second := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body2, contentType2)

	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", second.Code, truncate(second.Body.String()))
	}
	if !strings.Contains(second.Body.String(), `"existingResourceId":1`) {
		t.Errorf("the structured detail must keep the colliding id, got %s", second.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 106 — the import error is printed once
// ---------------------------------------------------------------------------

func TestImportPage_PrintsTheErrorOnce(t *testing.T) {
	tc := SetupTestEnv(t)

	page := tc.requestWithAccept(http.MethodGet, "/admin/import", browserAccept, "").Body.String()

	// Positive control: the red box is still there.
	if !strings.Contains(page, `data-testid="import-error"`) {
		t.Fatal("control failed: the import error region is missing")
	}
	if strings.Contains(page, `'Error: ' + job?.error`) {
		t.Error("the parse-progress line should not repeat the error the red box already shows")
	}
}

// extractBanner returns the rendered form-error banner, so a failure message
// shows what the reader would actually have seen.
func extractBanner(body string) string {
	start := strings.Index(body, `data-testid="form-error-banner"`)
	if start < 0 {
		return "(no banner)"
	}
	end := start + 600
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}

// ---------------------------------------------------------------------------
// Found live while verifying 106: the compare pages printed their message twice
// ---------------------------------------------------------------------------

// Not one of the hunt's findings — caught by walking /group/compare on the
// seeded instance. Both compare templates rendered errorMessage in their own
// body *and* inherited layouts/base.tpl's alert region, so a reader who opened
// either page without arguments saw the same sentence twice. Same shape as
// finding 106.
func TestComparePages_PrintTheirMessageOnce(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/group/compare", "/resource/compare"} {
		t.Run(path, func(t *testing.T) {
			body := tc.requestWithAccept(http.MethodGet, path, browserAccept, "").Body.String()

			// Positive control: the page still explains itself.
			if !strings.Contains(body, "Pick a") {
				t.Fatalf("control failed: no message rendered on %s", path)
			}
			if n := strings.Count(body, "and use Compare"); n != 1 {
				t.Errorf("expected the message exactly once on %s, found it %d times", path, n)
			}
		})
	}
}
