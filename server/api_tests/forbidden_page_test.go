package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
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

// Every plugin management endpoint is admin-only, and isSystemPath is where that
// is decided — by exact string match, with no `/v1/plugin/` prefix rule. So an
// endpoint added to that route block and forgotten here falls through to the
// default branch, where a GET is `safe` and yields capRead, which
// principalSatisfies grants to every authenticated principal including a guest.
// It works perfectly for the admin who built it, and nothing fails.
//
// That is exactly what happened to /v1/plugin/schedules: its own doc comment
// claimed it was "admin-only by virtue of the /v1/plugin/ prefix's place in
// isSystemPath", and there is no such prefix. A guest could read every stored
// schedule for any plugin — interval, next due time, run count, and lastError,
// which is arbitrary text from plugin code — and enumerate which plugins exist,
// which the render seams are built to prevent.
//
// The table is the point: it is cheaper to extend than to remember.
func TestPluginCommandHistoryPage_IsAdminOnly(t *testing.T) {
	tc := setupAuthEnv(t)
	for _, role := range []models.Role{models.RoleEditor, models.RoleUser, models.RoleGuest} {
		bearer := roleBearer(t, tc, role)
		res := doReq(tc, http.MethodGet, "/admin/plugin-command-runs",
			map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
		if res.Code != http.StatusForbidden {
			t.Errorf("%s history page status = %d, want 403", role, res.Code)
		}
	}
	admin := roleBearer(t, tc, models.RoleAdmin)
	res := doReq(tc, http.MethodGet, "/admin/plugin-command-runs",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("admin history page status = %d: %s", res.Code, res.Body.String())
	}
}

func TestPluginManagementEndpoints_AreAdminOnly(t *testing.T) {
	tc := setupAuthEnv(t)

	endpoints := []struct{ method, path string }{
		{http.MethodGet, "/v1/plugins/manage"},
		{http.MethodGet, "/v1/plugin/command-runs"},
		{http.MethodGet, "/v1/plugin/command-run"},
		{http.MethodPost, "/v1/plugin/command-run/cancel"},
		{http.MethodGet, "/v1/plugin/schedules?name=whatever"},
		{http.MethodGet, "/v1/plugin/scheduled-downloads?name=whatever"},
		{http.MethodPost, "/v1/plugin/scheduled-downloads/cancel"},
		{http.MethodPost, "/v1/plugin/enable"},
		{http.MethodPost, "/v1/plugin/disable"},
		{http.MethodPost, "/v1/plugin/settings?name=whatever"},
		{http.MethodPost, "/v1/plugin/scopedAccess"},
		{http.MethodPost, "/v1/plugin/purge-data"},
		// The one that runs plugin code rather than reading about it, which makes
		// it the worst of these to leave off the list.
		{http.MethodPost, "/v1/plugin/schedule/run"},
	}

	for _, role := range []models.Role{models.RoleEditor, models.RoleUser, models.RoleGuest} {
		bearer := roleBearer(t, tc, role)
		for _, e := range endpoints {
			res := doReq(tc, e.method, e.path,
				map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
			if res.Code != http.StatusForbidden {
				t.Errorf("%s %s %s should be 403, got %d — add the path to isSystemPath in server/authz_policy.go",
					role, e.method, e.path, res.Code)
			}
		}
	}

	// The control: an admin is not refused and the route exists. A 403 here would
	// mean the assertions above pass because the system rule also blocks admins;
	// a 404 means they pass because the route does not exist. Either measures
	// nothing.
	admin := roleBearer(t, tc, models.RoleAdmin)
	for _, e := range endpoints {
		res := doReq(tc, e.method, e.path,
			map[string]string{"Accept": "application/json", "Authorization": admin}, nil, nil)
		if res.Code == http.StatusForbidden || res.Code == http.StatusNotFound {
			t.Errorf("admin %s %s got %d; this endpoint is unreachable and the assertions above are vacuous",
				e.method, e.path, res.Code)
		}
	}
}

// The shares dashboard lists every shared note in the caller's scope with its
// public /s/<token> URL and renders revoke controls, so it is editor-only for
// reads as well as writes (#120). Its entry in isEditorPath used to gate
// nothing: the route is GET-only and that branch downgrades every safe method
// to capRead. This covers the three registrations the templates map creates,
// HTML and JSON denials, and a subtree-scoped user as well as an unscoped one.
func TestAdminSharesPage_IsEditorOnly(t *testing.T) {
	tc := setupAuthEnv(t)
	shared := tc.CreateDummyNote("issue120 shared note")
	if _, err := tc.AppCtx.ShareNote(shared.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	scope := &models.Group{Name: "issue120-scope"}
	if err := tc.DB.Create(scope).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	scopedUser, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "issue120_scoped_user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scope.ID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	scopedRaw, _, err := tc.AppCtx.CreateApiToken(scopedUser.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	denied := map[string]string{
		"user":        roleBearer(t, tc, models.RoleUser),
		"guest":       roleBearer(t, tc, models.RoleGuest),
		"scoped user": "Bearer " + scopedRaw,
	}
	for who, bearer := range denied {
		for _, path := range []string{"/admin/shares", "/admin/shares.json", "/admin/shares.body"} {
			html := doReq(tc, http.MethodGet, path,
				map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
			if html.Code != http.StatusForbidden {
				t.Errorf("%s GET %s (HTML) = %d, want 403", who, path, html.Code)
			} else if strings.Contains(html.Body.String(), shared.Name) {
				t.Errorf("%s GET %s: 403 page still lists the shared note", who, path)
			}
			js := doReq(tc, http.MethodGet, path,
				map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
			if js.Code != http.StatusForbidden || !strings.Contains(js.Body.String(), "insufficient permissions") {
				t.Errorf("%s GET %s (JSON) = %d %q, want 403 insufficient permissions", who, path, js.Code, js.Body.String())
			}
		}
	}

	// Controls: admin and editor reach the page and see the shared note, so the
	// denials above are not caused by a missing route or an empty page.
	for _, role := range []models.Role{models.RoleAdmin, models.RoleEditor} {
		bearer := roleBearer(t, tc, role)
		res := doReq(tc, http.MethodGet, "/admin/shares",
			map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("%s GET /admin/shares = %d, want 200 — this test measured nothing", role, res.Code)
		}
		if !strings.Contains(res.Body.String(), shared.Name) {
			t.Errorf("%s GET /admin/shares does not list the shared note %q", role, shared.Name)
		}
	}
}
