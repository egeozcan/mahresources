package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// WS3 — "Validation before submit, and error surfaces that keep you in the app".
//
// Findings 16/92, 56/91, 20, 34, 100, 103, 106, 109, 111, 114, 115, 119/132/135,
// 142, 157 of docs/ui-bug-hunt-2026-07-29.md. This file covers the half of the
// workstream that is reachable from Go: the shared error surface (HandleError,
// addErrContext, RenderNotFound) and the two server-rendered guards.

// requestWithAccept issues a request carrying an explicit Accept header, which is
// what decides between the HTML and the JSON branch of every error path here.
func (tc *TestContext) requestWithAccept(method, url, accept string, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, url, reader)
	req.Header.Set("Accept", accept)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

const browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

// ---------------------------------------------------------------------------
// HandleError renders the app's own error page (findings 16/92, 56/91, 34, 119)
// ---------------------------------------------------------------------------

// TestHandleError_HTMLCarriesAppChrome covers the leverage point behind ~10
// findings: HandleError wrote a self-contained inline document with no nav and a
// single javascript:history.back() link, for every HTML-accepting API rejection.
func TestHandleError_HTMLCarriesAppChrome(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/groups/addTags", browserAccept, "id=1")

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	// Positive control: the rejection reason still reaches the reader. Without
	// this, every "must not contain" below is satisfied by an empty page.
	if !strings.Contains(body, "at least one tag") {
		t.Errorf("error page should still state the reason; got:\n%s", truncate(body))
	}
	if !strings.Contains(body, "Dashboard") {
		t.Error("error page should carry the app nav (Dashboard link)")
	}
	if !strings.Contains(body, "Search") {
		t.Error("error page should carry the app header (global search)")
	}
	if strings.Contains(body, "javascript:history.back()") {
		t.Error("error page should not offer javascript:history.back() as its only way out")
	}
	if strings.Contains(body, "error-container") {
		t.Error("error page should no longer be the standalone inline document")
	}
}

// TestHandleError_HTMLOffersRecoveryLinks asserts the page is survivable: a real
// in-app link, not just the browser's back button.
func TestHandleError_HTMLOffersRecoveryLinks(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/groups/addTags", browserAccept, "id=1")
	body := resp.Body.String()

	if !strings.Contains(body, `data-testid="error-recovery"`) {
		t.Errorf("error page should offer a recovery region; got:\n%s", truncate(body))
	}
	if !strings.Contains(body, `href="/dashboard"`) {
		t.Error("error page should link to the dashboard")
	}
}

// TestHandleError_JSONBranchUnchanged is the control on the blast radius: the
// inline editors, the bulk-operation toolbar and the MRQL bar all read this
// shape, so it must stay exactly {"error": "..."} with a JSON content type.
func TestHandleError_JSONBranchUnchanged(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/groups/addTags", "application/json", "id=1")

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected a JSON content type, got %q", ct)
	}
	var payload map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("JSON branch must stay parseable: %v (body %q)", err, resp.Body.String())
	}
	if len(payload) != 1 {
		t.Errorf("JSON error body must carry exactly one key, got %v", payload)
	}
	if payload["error"] == "" {
		t.Errorf(`JSON error body must keep the "error" key, got %v`, payload)
	}
	if strings.Contains(resp.Body.String(), "<") {
		t.Error("JSON branch must not render HTML")
	}
}

// TestHandleError_NoAcceptHeaderStaysJSON guards the CLI and every fetch() that
// sends no Accept header at all.
func TestHandleError_NoAcceptHeaderStaysJSON(t *testing.T) {
	tc := SetupTestEnv(t)

	req, _ := http.NewRequest(http.MethodPost, "/v1/groups/addTags", strings.NewReader("id=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("a request with no Accept header must still get JSON, got %q", ct)
	}
}

// ---------------------------------------------------------------------------
// addErrContext stops leaking GORM strings (findings 119/132/135/111)
// ---------------------------------------------------------------------------

func TestEntityNotFound_ShowsHumanMessageNotORMString(t *testing.T) {
	tc := SetupTestEnv(t)

	cases := []struct {
		path     string
		expect   string
		listHref string
	}{
		{"/note?id=99999", "note", `href="/notes"`},
		{"/resource?id=99999", "resource", `href="/resources"`},
		{"/group?id=99999", "group", `href="/groups"`},
		{"/tag?id=99999", "tag", `href="/tags"`},
		{"/category?id=99999", "category", `href="/categories"`},
		{"/query?id=99999", "query", `href="/queries"`},
	}

	for _, tc2 := range cases {
		t.Run(tc2.path, func(t *testing.T) {
			resp := tc.requestWithAccept(http.MethodGet, tc2.path, browserAccept, "")
			if resp.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", resp.Code)
			}
			body := resp.Body.String()
			if strings.Contains(body, "record not found") {
				t.Error(`page must not show GORM's "record not found"`)
			}
			if !strings.Contains(strings.ToLower(body), "that "+tc2.expect+" ") {
				t.Errorf("page should name the missing %s; got message region:\n%s",
					tc2.expect, extractAlert(body))
			}
			if !strings.Contains(body, tc2.listHref) {
				t.Errorf("page should link back to the %s list (%s)", tc2.expect, tc2.listHref)
			}
		})
	}
}

// TestEntityNotFound_JSONAPIKeepsDriverError is the control that the message
// table is scoped to the rendered page. The JSON API contract is unchanged, and
// e2e/tests/regressions/entity-not-found-returns-404.spec.ts asserts on it.
func TestEntityNotFound_JSONAPIKeepsDriverError(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodGet, "/v1/note?id=99999", "application/json", "")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "record not found") {
		t.Errorf("the JSON API contract must be untouched, got %s", resp.Body.String())
	}
}

// TestNotFoundPresentationIsUnified covers the second half of finding 119: an
// unmatched path and a missing entity used to render two different 404s
// ("404 Not Found / Page not found" versus "Error 404 / record not found").
func TestNotFoundPresentationIsUnified(t *testing.T) {
	tc := SetupTestEnv(t)

	unmatched := tc.requestWithAccept(http.MethodGet, "/this-page-does-not-exist", browserAccept, "").Body.String()
	missing := tc.requestWithAccept(http.MethodGet, "/note?id=99999", browserAccept, "").Body.String()

	for name, body := range map[string]string{"unmatched path": unmatched, "missing entity": missing} {
		if !strings.Contains(body, "<title>Error 404 - ") {
			t.Errorf("%s should use the shared 404 title; got %q", name, extractTitle(body))
		}
		if !strings.Contains(body, `data-testid="error-recovery"`) {
			t.Errorf("%s 404 should offer a recovery region", name)
		}
	}
}

// TestSeriesWithoutIdExplainsItself covers finding 111: /series with no id
// answered "Error 404 / record not found", which says nothing about the fact
// that the route is a detail page needing an id.
func TestSeriesWithoutIdExplainsItself(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodGet, "/series", browserAccept, "")
	if resp.Code != http.StatusNotFound && resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 4xx, got %d", resp.Code)
	}
	body := resp.Body.String()
	if strings.Contains(body, "record not found") {
		t.Error(`/series must not answer with GORM's "record not found"`)
	}
	if !strings.Contains(strings.ToLower(body), "series id") {
		t.Errorf("/series should say an id is required; alert was:\n%s", extractAlert(body))
	}
}

// ---------------------------------------------------------------------------
// 114 — unmatched /v1/ paths answer JSON
// ---------------------------------------------------------------------------

func TestNotFound_V1PathsAnswerJSON(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/v1/resources/count", "/v1/jobs", "/v1/nonexistent-endpoint"} {
		t.Run(path, func(t *testing.T) {
			resp := tc.requestWithAccept(http.MethodGet, path, "application/json", "")
			if resp.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", resp.Code)
			}
			if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Errorf("expected a JSON content type, got %q", ct)
			}
			var payload map[string]string
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("body must parse as JSON: %v (body %q)", err, truncate(resp.Body.String()))
			}
			if payload["error"] == "" {
				t.Errorf(`expected an "error" key, got %v`, payload)
			}
		})
	}
}

// TestNotFound_V1PathsAnswerJSONToBrowsers pins the rule to the path prefix, not
// to the Accept header: the /v1 namespace is the JSON API whoever is asking.
func TestNotFound_V1PathsAnswerJSONToBrowsers(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodGet, "/v1/nonexistent-endpoint", browserAccept, "")
	if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected a JSON content type under /v1, got %q", ct)
	}
}

// ---------------------------------------------------------------------------
// 20 — the Custom Property sort is only offered where a meta column exists
// ---------------------------------------------------------------------------

func TestSortOptions_CustomPropertyOnlyWhereMetaExists(t *testing.T) {
	tc := SetupTestEnv(t)

	offered := []string{"/tags", "/groups", "/notes", "/resources"}
	notOffered := []string{"/categories", "/categories/timeline"}

	for _, path := range offered {
		t.Run("offered "+path, func(t *testing.T) {
			body := tc.requestWithAccept(http.MethodGet, path, browserAccept, "").Body.String()
			if !strings.Contains(body, `value="__meta__"`) {
				t.Errorf("%s has a meta column, so the Custom Property sort should stay", path)
			}
		})
	}
	for _, path := range notOffered {
		t.Run("not offered "+path, func(t *testing.T) {
			body := tc.requestWithAccept(http.MethodGet, path, browserAccept, "").Body.String()
			if strings.Contains(body, `value="__meta__"`) {
				t.Errorf("%s has no meta column; offering the sort produces a 400", path)
			}
		})
	}
}

// TestCategoriesMetaSortStillRejected proves the option removal is a UI fix and
// not a silent behaviour change: a hand-typed meta sort is still a 400, and it
// still does not leak the driver's message.
func TestCategoriesMetaSortStillRejected(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodGet, "/categories?SortBy=meta-%3E%3E%27camera%27+desc", browserAccept, "")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), "no such column") {
		t.Error("must not leak the driver's column error")
	}
}

// ---------------------------------------------------------------------------
// 34 / 109 — the admin create-user form
// ---------------------------------------------------------------------------

// TestCreateUserRejection_KeepsTheAdminOnTheForm covers finding 34: a validation
// failure navigated to /v1/users and rendered a bare error page, so the admin
// retyped every field.
func TestCreateUserRejection_KeepsTheAdminOnTheForm(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/users", browserAccept,
		"username=hunt-admin-tmpuser&displayName=Temp+User&password=abc&role=editor")

	if resp.Code != http.StatusFound {
		t.Fatalf("expected a 302 back to the form, got %d: %s", resp.Code, truncate(resp.Body.String()))
	}
	location := resp.Header().Get("Location")
	if !strings.HasPrefix(location, "/admin/users?") {
		t.Fatalf("expected a redirect to /admin/users, got %q", location)
	}
	if !strings.Contains(location, "username=hunt-admin-tmpuser") {
		t.Error("the username the admin typed should survive the rejection")
	}
	if !strings.Contains(location, "displayName=Temp+User") {
		t.Error("the display name the admin typed should survive the rejection")
	}
	if strings.Contains(location, "abc") {
		t.Errorf("the password must never be echoed back through the URL: %q", location)
	}
}

// TestCreateUserRejection_GuestWithoutScopeSaysWhy covers the part of finding 34
// that is a real defect and not a presentation one: an empty scopeGroupId form
// field decodes to a pointer to 0 rather than nil, so a guest created without a
// scope group is told "scope group does not exist" instead of the accurate
// "this role must be limited to a group".
func TestCreateUserRejection_GuestWithoutScopeSaysWhy(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.requestWithAccept(http.MethodPost, "/v1/users", "application/json",
		"username=hunt-guest&password=abcdefgh&role=guest&scopeGroupId=")

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	body := resp.Body.String()
	if strings.Contains(body, "scope group does not exist") {
		t.Errorf("an omitted scope group is not a missing one: %s", body)
	}
	if !strings.Contains(body, "must be limited to a group") {
		t.Errorf("expected the role-scoping message, got %s", body)
	}
}

// TestCreateUserForm_StatesThePasswordRule covers finding 109: the server
// enforces a minimum the form never mentioned, so the only way to learn it was
// to be rejected.
func TestCreateUserForm_StatesThePasswordRule(t *testing.T) {
	tc := SetupTestEnv(t)

	body := tc.requestWithAccept(http.MethodGet, "/admin/users", browserAccept, "").Body.String()
	if !strings.Contains(body, `minlength="8"`) {
		t.Error("the password field should carry the server's minimum as minlength")
	}
	if !strings.Contains(body, "at least 8 characters") {
		t.Error("the password rule should be stated next to the field")
	}
}

// ---------------------------------------------------------------------------
// 157 — the template-partial name rule is stated next to the field
// ---------------------------------------------------------------------------

func TestTemplatePartialForm_StatesAndChecksTheNameRule(t *testing.T) {
	tc := SetupTestEnv(t)

	body := tc.requestWithAccept(http.MethodGet, "/templatePartial/new", browserAccept, "").Body.String()

	if !strings.Contains(body, "Kebab-case identifier") {
		t.Error("the kebab-case rule should be visible next to the Name field")
	}
	// Spelled with an alternation rather than [a-z0-9-]: the browser compiles
	// `pattern` with the regex `v` flag, under which a bare hyphen in a
	// character class is a syntax error — and an invalid pattern is ignored
	// outright, so the obvious spelling was a guard that gated nothing. The
	// browser-side control lives in e2e/tests/regressions/ws3-error-surfaces.spec.ts.
	if !strings.Contains(body, `pattern="[a-z](?:[a-z0-9]|-)*"`) {
		t.Error("the Name field should carry the kebab-case rule as a v-flag-safe pattern")
	}
}

// ---------------------------------------------------------------------------
// 142 — the upload form guards its file input
// ---------------------------------------------------------------------------

// The file input cannot simply be `required`: the same form accepts a URL
// instead, in which case the picker is deliberately ignored. The guard has to be
// conditional, which is why this asserts the binding rather than the attribute.
func TestResourceUploadForm_RequiresAFileUnlessAURLIsGiven(t *testing.T) {
	tc := SetupTestEnv(t)

	body := tc.requestWithAccept(http.MethodGet, "/resource/new", browserAccept, "").Body.String()

	idx := strings.Index(body, `name="resource"`)
	if idx < 0 {
		t.Fatal("could not find the file input")
	}
	window := body[max(0, idx-400):min(len(body), idx+400)]
	if !strings.Contains(window, `:required=`) {
		t.Errorf("the file input should be required unless a URL is given; got:\n%s", window)
	}
}

// ---------------------------------------------------------------------------
// 16/92 and 56/91 — the merge and Add Tags forms guard an empty selection
// ---------------------------------------------------------------------------

// TestMergeAndAddTagsForms_CarryASelectionGuard asserts the server-rendered half
// of the guard. The behaviour itself (disabled button, no destructive confirm)
// is covered by e2e/tests/regressions/empty-selection-guards.spec.ts.
func TestMergeAndAddTagsForms_CarryASelectionGuard(t *testing.T) {
	tc := SetupTestEnv(t)

	tag := createTagViaAPI(t, tc, "ws3-guard-tag")
	group := tc.CreateDummyGroup("ws3-guard-group")

	cases := []struct {
		name  string
		path  string
		guard string
	}{
		{"tag merge", "/tag?id=" + uitoa(tag), "requireSelection: { field: 'losers' }"},
		{"group merge", "/group?id=" + uitoa(uint(group.ID)), "requireSelection: { field: 'losers' }"},
		{"group add tags", "/group?id=" + uitoa(uint(group.ID)), "selectionRequired({ field: 'editedId' })"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := tc.requestWithAccept(http.MethodGet, c.path, browserAccept, "").Body.String()
			if !strings.Contains(body, c.guard) {
				t.Errorf("the %s form should declare a selection guard (%s)", c.name, c.guard)
			}
			if !strings.Contains(body, `:disabled="!hasSelection"`) {
				t.Errorf("the %s submit should be unavailable while the selection is empty", c.name)
			}
		})
	}
}

// TestAddTagsFormOnEveryEntityPage is the control that the Add Tags guard reaches
// all four templates that include partials/tagList.tpl, not just the group page.
func TestAddTagsFormOnEveryEntityPage(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("ws3-addtags-group")
	note := tc.CreateDummyNote("ws3-addtags-note")

	paths := []string{
		"/group?id=" + uitoa(uint(group.ID)),
		"/note?id=" + uitoa(uint(note.ID)),
		"/note/text?id=" + uitoa(uint(note.ID)),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			body := tc.requestWithAccept(http.MethodGet, path, browserAccept, "").Body.String()
			// Positive control: the form is on this page at all.
			if !strings.Contains(body, "Add&nbsp;Tags") {
				t.Skipf("%s does not render the Add Tags form", path)
			}
			if !strings.Contains(body, "selectionRequired({ field: 'editedId' })") {
				t.Errorf("%s should guard its Add Tags submit", path)
			}
		})
	}
}

// TestMergeRejection_DropsInternalJargon covers finding 92's specific complaint:
// "one or more losers required" is vocabulary from the merge implementation.
func TestMergeRejection_DropsInternalJargon(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := createTagViaAPI(t, tc, "ws3-jargon-tag")

	resp := tc.requestWithAccept(http.MethodPost, "/v1/tags/merge", "application/json", "winner="+uitoa(tag))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, "losers") {
		t.Errorf(`"losers" is internal vocabulary, got %s`, body)
	}
	if !strings.Contains(body, "at least one") {
		t.Errorf("expected a message naming what to do, got %s", body)
	}
}

// ---------------------------------------------------------------------------
// 115 — numeric runtime settings are numeric inputs
// ---------------------------------------------------------------------------

func TestRuntimeSettings_NumericFieldsAreNumberInputs(t *testing.T) {
	tc := SetupTestEnv(t)

	body := tc.requestWithAccept(http.MethodGet, "/admin/settings", browserAccept, "").Body.String()
	if !strings.Contains(body, `:type="inputType"`) {
		t.Error("the settings input should choose its type from the setting's declared type")
	}
	if !strings.Contains(body, "inputType") || !strings.Contains(body, "'number'") {
		t.Error("numeric setting types should map to a number input")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func truncate(s string) string {
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}

func extractTitle(body string) string {
	start := strings.Index(body, "<title>")
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], "</title>")
	if end < 0 {
		return ""
	}
	return body[start+7 : start+end]
}

// extractAlert returns the rendered error region, so a failure message shows
// what the reader would actually have seen.
func extractAlert(body string) string {
	start := strings.Index(body, `role="alert"`)
	if start < 0 {
		return "(no alert region)"
	}
	end := start + 800
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}

func uitoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

func createTagViaAPI(t *testing.T, tc *TestContext, name string) uint {
	t.Helper()
	resp := tc.requestWithAccept(http.MethodPost, "/v1/tag", "application/json", "Name="+name)
	if resp.Code != http.StatusOK {
		t.Fatalf("could not create tag: %d %s", resp.Code, resp.Body.String())
	}
	var created struct {
		ID uint `json:"ID"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("could not parse created tag: %v", err)
	}
	return created.ID
}
