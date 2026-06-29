package api_tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// setupAuthEnv builds an auth-enabled test server with a bootstrapped admin
// account (admin / adminpw1).
func setupAuthEnv(t *testing.T) *TestContext {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
	})
	// Pin the in-memory test DB to a single connection. With mode=memory&
	// cache=private each new connection is a separate, empty database, so under
	// any concurrent access a token/session lookup can hit a fresh connection and
	// spuriously fail auth (401 instead of the expected 403). Real deployments use
	// a file/WAL or Postgres DB where connections share data, so this only affects
	// the in-memory test harness.
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return tc
}

func doReq(tc *TestContext, method, path string, headers map[string]string, cookies []*http.Cookie, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

func sessionCookie(t *testing.T, rr *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == "mr_session" {
			return c
		}
	}
	t.Fatalf("expected mr_session cookie, got %v", rr.Result().Cookies())
	return nil
}

func TestAuthDisabled_NoLoginRequired(t *testing.T) {
	tc := SetupTestEnv(t) // auth off
	rr := doReq(tc, http.MethodGet, "/v1/notes", map[string]string{"Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth-off GET /v1/notes should be 200, got %d", rr.Code)
	}
}

func TestAuthEnabled_UnauthenticatedApiGets401(t *testing.T) {
	tc := setupAuthEnv(t)
	rr := doReq(tc, http.MethodGet, "/v1/notes", map[string]string{"Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API should be 401, got %d", rr.Code)
	}
}

func TestAuthEnabled_UnauthenticatedHtmlRedirects(t *testing.T) {
	tc := setupAuthEnv(t)
	rr := doReq(tc, http.MethodGet, "/dashboard", map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusFound {
		t.Fatalf("unauthenticated HTML should redirect (302), got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Fatalf("redirect should target /login, got %q", loc)
	}
}

func TestAuthEnabled_LoginPageIsPublic(t *testing.T) {
	tc := setupAuthEnv(t)
	rr := doReq(tc, http.MethodGet, "/login", map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("login page should be public (200), got %d", rr.Code)
	}
}

func TestAuthEnabled_ApiLoginThenCookieAccess(t *testing.T) {
	tc := setupAuthEnv(t)

	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("valid API login should be 200, got %d (%s)", login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)

	// Authenticated request with the session cookie succeeds.
	rr := doReq(tc, http.MethodGet, "/v1/notes", map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("authenticated request should be 200, got %d", rr.Code)
	}

	// /v1/auth/me reflects the identity.
	me := doReq(tc, http.MethodGet, "/v1/auth/me", nil, []*http.Cookie{cookie}, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("/v1/auth/me should be 200, got %d", me.Code)
	}
	if !strings.Contains(me.Body.String(), `"username":"admin"`) || !strings.Contains(me.Body.String(), `"role":"admin"`) {
		t.Fatalf("/v1/auth/me body unexpected: %s", me.Body.String())
	}
}

func TestAuthEnabled_AuthenticatedHtmlRenders(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)

	rr := doReq(tc, http.MethodGet, "/dashboard", map[string]string{"Accept": "text/html"}, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("authenticated dashboard should render 200, got %d", rr.Code)
	}
	// The base layout's account control should render for a logged-in user.
	if !strings.Contains(rr.Body.String(), "Sign out") {
		t.Fatalf("dashboard should show the account/logout control")
	}
}

func TestAuthEnabled_ApiLoginWrongCredentials(t *testing.T) {
	tc := setupAuthEnv(t)
	rr := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"nope"}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credentials should be 401, got %d", rr.Code)
	}
}

func TestAuthEnabled_BearerToken(t *testing.T) {
	tc := setupAuthEnv(t)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "cliuser", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(user.ID, "cli", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	rr := doReq(tc, http.MethodGet, "/v1/notes",
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + raw}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid bearer token should be 200, got %d", rr.Code)
	}

	bad := doReq(tc, http.MethodGet, "/v1/notes",
		map[string]string{"Accept": "application/json", "Authorization": "Bearer not-a-token"}, nil, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer token should be 401, got %d", bad.Code)
	}
}

func TestAuthEnabled_LogoutRevokesSession(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)

	logout := doReq(tc, http.MethodPost, "/v1/auth/logout", nil, []*http.Cookie{cookie}, nil)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout should be 200, got %d", logout.Code)
	}

	// The old cookie no longer works.
	rr := doReq(tc, http.MethodGet, "/v1/notes", map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("request after logout should be 401, got %d", rr.Code)
	}
}

func TestAuthEnabled_WebLoginForm(t *testing.T) {
	tc := setupAuthEnv(t)

	form := doReq(tc, http.MethodPost, "/login",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil,
		strings.NewReader("username=admin&password=adminpw1"))
	if form.Code != http.StatusFound {
		t.Fatalf("web login should redirect (302), got %d", form.Code)
	}
	if loc := form.Header().Get("Location"); loc != "/dashboard" {
		t.Fatalf("web login should redirect to /dashboard, got %q", loc)
	}
	sessionCookie(t, form) // must set a cookie

	bad := doReq(tc, http.MethodPost, "/login",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil,
		strings.NewReader("username=admin&password=wrong"))
	if bad.Code != http.StatusFound {
		t.Fatalf("failed web login should redirect (302), got %d", bad.Code)
	}
	if loc := bad.Header().Get("Location"); !strings.HasPrefix(loc, "/login?error=1") {
		t.Fatalf("failed web login should redirect to /login?error=1, got %q", loc)
	}
}
