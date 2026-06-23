package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestUserAdminAPI(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	adminH := map[string]string{"Accept": "application/json", "Authorization": admin, "Content-Type": "application/json"}

	// Create a user.
	create := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"newuser","password":"pw","role":"editor"}`))
	if create.Code != http.StatusOK {
		t.Fatalf("admin create user should be 200, got %d (%s)", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "passwordHash") || strings.Contains(create.Body.String(), "\"pw\"") {
		t.Fatalf("create response must not leak the password/hash: %s", create.Body.String())
	}

	// List users includes it.
	list := doReq(tc, http.MethodGet, "/v1/users", adminH, nil, nil).Body.String()
	if !strings.Contains(list, "newuser") {
		t.Fatalf("user list should contain newuser, got: %s", list)
	}

	// Duplicate username conflicts.
	dup := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"newuser","password":"pw","role":"user"}`))
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate username should be 409, got %d", dup.Code)
	}

	// Guest without a scope group is rejected (400).
	badGuest := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"g","password":"pw","role":"guest"}`))
	if badGuest.Code != http.StatusBadRequest {
		t.Fatalf("guest without scope group should be 400, got %d", badGuest.Code)
	}
}

func TestUserAdminAPI_NonAdminForbidden(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)
	h := map[string]string{"Accept": "application/json", "Authorization": editor, "Content-Type": "application/json"}

	resp := doReq(tc, http.MethodPost, "/v1/users", h, nil,
		strings.NewReader(`{"username":"x","password":"pw","role":"user"}`))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("non-admin creating a user should be 403, got %d", resp.Code)
	}
	if list := doReq(tc, http.MethodGet, "/v1/users", h, nil, nil); list.Code != http.StatusForbidden {
		t.Fatalf("non-admin listing users should be 403, got %d", list.Code)
	}
}

func TestAccountSelfService(t *testing.T) {
	tc := setupAuthEnv(t)
	// Create an editor and authenticate as them via a session cookie.
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "selfuser", Password: "origpw", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"selfuser","password":"origpw"}`))
	cookie := sessionCookie(t, login)
	jsonCookie := []*http.Cookie{cookie}
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}

	// Mint an API token for self.
	mint := doReq(tc, http.MethodPost, "/v1/account/tokens", h, jsonCookie, strings.NewReader(`{"name":"cli"}`))
	if mint.Code != http.StatusOK {
		t.Fatalf("mint token should be 200, got %d (%s)", mint.Code, mint.Body.String())
	}
	if !strings.Contains(mint.Body.String(), `"token"`) {
		t.Fatalf("mint response should contain the raw token once: %s", mint.Body.String())
	}

	// The minted token authenticates API calls.
	rawToken := extractJSONString(mint.Body.String(), "token")
	withTok := doReq(tc, http.MethodGet, "/v1/notes",
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + rawToken}, nil, nil)
	if withTok.Code != http.StatusOK {
		t.Fatalf("minted token should authenticate, got %d", withTok.Code)
	}

	// Change own password (wrong current → 401).
	wrong := doReq(tc, http.MethodPost, "/v1/account/password", h, jsonCookie,
		strings.NewReader(`{"currentPassword":"nope","newPassword":"newpw"}`))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current password should be 401, got %d", wrong.Code)
	}
	ok := doReq(tc, http.MethodPost, "/v1/account/password", h, jsonCookie,
		strings.NewReader(`{"currentPassword":"origpw","newPassword":"newpw"}`))
	if ok.Code != http.StatusOK {
		t.Fatalf("password change should be 200, got %d (%s)", ok.Code, ok.Body.String())
	}
	// New password authenticates; old does not.
	if _, err := tc.AppCtx.AuthenticateUser("selfuser", "newpw"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
	if _, err := tc.AppCtx.AuthenticateUser("selfuser", "origpw"); err == nil {
		t.Fatalf("old password should no longer work")
	}
}

func TestAdminAndAccountPagesRender(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw"}`))
	cookie := sessionCookie(t, login)
	htmlH := map[string]string{"Accept": "text/html"}

	users := doReq(tc, http.MethodGet, "/admin/users", htmlH, []*http.Cookie{cookie}, nil)
	if users.Code != http.StatusOK {
		t.Fatalf("/admin/users should render for admin, got %d", users.Code)
	}
	if !strings.Contains(users.Body.String(), "Create user") {
		t.Fatalf("/admin/users should show the create form")
	}

	account := doReq(tc, http.MethodGet, "/account", htmlH, []*http.Cookie{cookie}, nil)
	if account.Code != http.StatusOK {
		t.Fatalf("/account should render, got %d", account.Code)
	}
	if !strings.Contains(account.Body.String(), "API tokens") {
		t.Fatalf("/account should show the API tokens section")
	}
}

// extractJSONString pulls a top-level string field value out of a small JSON
// object body (test helper, not a general parser).
func extractJSONString(body, key string) string {
	marker := `"` + key + `":"`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
