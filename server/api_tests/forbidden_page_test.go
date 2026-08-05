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

// Every admin template path has to be named in isSystemPath by hand, because it
// matches exactly rather than by prefix. A page left out of that list falls through
// to the default branch, where a GET is `safe` and yields capRead — so it works for
// the admin who built it and is *also* readable by every editor, user and guest,
// with nothing failing anywhere.
//
// /admin/users/edit renders one account's username, role, scope group and disabled
// state, so this asserts the deny for each of the three registrations the templates
// map creates (the bare path plus .json and .body). authz strips those two suffixes
// before matching, so a single omission would open all three at once.
func TestAdminUserEditPage_IsAdminOnly(t *testing.T) {
	tc := setupAuthEnv(t)

	for _, role := range []models.Role{models.RoleEditor, models.RoleUser, models.RoleGuest} {
		bearer := roleBearer(t, tc, role)
		for _, path := range []string{
			"/admin/users/edit?id=1",
			"/admin/users/edit.json?id=1",
			"/admin/users/edit.body?id=1",
		} {
			res := doReq(tc, http.MethodGet, path,
				map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
			if res.Code != http.StatusForbidden {
				t.Errorf("%s GET %s should be 403, got %d — add the path to isSystemPath in server/authz_policy.go",
					role, path, res.Code)
			}
		}
	}

	// The control: an admin does reach it, so the assertions above are not passing
	// because the route is missing.
	admin := roleBearer(t, tc, models.RoleAdmin)
	ok := doReq(tc, http.MethodGet, "/admin/users/edit?id=1",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("admin GET /admin/users/edit should be 200, got %d — this test measured nothing", ok.Code)
	}
}
