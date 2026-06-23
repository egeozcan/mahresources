package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// An authorization denial on a browser navigation renders the styled 403 page
// (app chrome) rather than a bare http.Error string. API/JSON denials stay
// machine-readable.
func TestForbidden_HTMLStyledPage(t *testing.T) {
	tc := setupAuthEnv(t)
	// An editor may not reach the admin-only Users page.
	bearer := roleBearer(t, tc, models.RoleEditor)

	html := doReq(tc, http.MethodGet, "/admin/users",
		map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if html.Code != http.StatusForbidden {
		t.Fatalf("editor GET /admin/users should be 403, got %d", html.Code)
	}
	if ct := html.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("HTML 403 should have text/html content-type, got %q", ct)
	}
	body := html.Body.String()
	if !strings.Contains(body, "<html") {
		t.Fatalf("HTML 403 should render the styled page (app chrome), got: %s", body)
	}
	if !strings.Contains(strings.ToLower(body), "permission") {
		t.Fatalf("HTML 403 should explain the denial, got: %s", body)
	}

	// JSON denials remain machine-readable.
	js := doReq(tc, http.MethodGet, "/admin/users",
		map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
	if js.Code != http.StatusForbidden {
		t.Fatalf("editor JSON GET /admin/users should be 403, got %d", js.Code)
	}
	if !strings.Contains(js.Body.String(), "insufficient permissions") {
		t.Fatalf("JSON 403 should carry the error message, got: %s", js.Body.String())
	}
}
