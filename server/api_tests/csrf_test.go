package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// csrfFor returns the CSRF token for a cookie-authenticated session, read back
// from /v1/auth/me (as a real browser would obtain it from the page meta tag).
func csrfFor(t *testing.T, tc *TestContext, cookie *http.Cookie) string {
	t.Helper()
	me := doReq(tc, http.MethodGet, "/v1/auth/me",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("/v1/auth/me should be 200, got %d", me.Code)
	}
	var body struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/auth/me: %v", err)
	}
	if body.CsrfToken == "" {
		t.Fatalf("/v1/auth/me must expose a non-empty csrfToken, got: %s", me.Body.String())
	}
	return body.CsrfToken
}

// loginCookieAndCSRF logs in as admin via the JSON API and returns the session
// cookie plus the session's CSRF token.
func loginCookieAndCSRF(t *testing.T, tc *TestContext) (*http.Cookie, string) {
	t.Helper()
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login should be 200, got %d (%s)", login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)
	return cookie, csrfFor(t, tc, cookie)
}

const urlEncoded = "application/x-www-form-urlencoded"

// A cookie-authenticated state-changing request without a CSRF token is rejected.
func TestCSRF_CookiePostWithoutTokenIsRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, []*http.Cookie{cookie},
		strings.NewReader("name=csrf-none"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cookie POST without CSRF token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "CSRF") {
		t.Fatalf("403 body should mention CSRF, got: %s", rr.Body.String())
	}
}

// The token is accepted via the X-CSRF-Token header, the csrf_token query
// parameter, and (for urlencoded bodies) the csrf_token form field.
func TestCSRF_TokenAcceptedViaHeaderQueryAndField(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	// Header
	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "X-CSRF-Token": token},
		[]*http.Cookie{cookie}, strings.NewReader("name=csrf-header"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with X-CSRF-Token header should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	// Query parameter
	rr = doReq(tc, http.MethodPost, "/v1/tag?csrf_token="+token,
		map[string]string{"Content-Type": urlEncoded}, []*http.Cookie{cookie},
		strings.NewReader("name=csrf-query"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with csrf_token query param should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	// Urlencoded body field
	rr = doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, []*http.Cookie{cookie},
		strings.NewReader("name=csrf-field&csrf_token="+token))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with csrf_token body field should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// A wrong token is rejected.
func TestCSRF_WrongTokenRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "X-CSRF-Token": "deadbeef"},
		[]*http.Cookie{cookie}, strings.NewReader("name=csrf-wrong"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST with wrong CSRF token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// Bearer-authenticated (token) requests are exempt: they carry no ambient cookie.
func TestCSRF_BearerExempt(t *testing.T) {
	tc := setupAuthEnv(t)
	bearer := roleBearer(t, tc, models.RoleAdmin)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "Authorization": bearer}, nil,
		strings.NewReader("name=csrf-bearer"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("Bearer POST should be exempt from CSRF, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// Safe methods (GET) are never CSRF-checked.
func TestCSRF_SafeGetNotChecked(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodGet, "/v1/tags",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("cookie GET should be 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// With auth disabled the CSRF check is a no-op: a tokenless POST succeeds.
func TestCSRF_AuthDisabledNoOp(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, nil,
		strings.NewReader("name=csrf-authoff"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("auth-off POST must not be CSRF-blocked, got %d (%s)", rr.Code, rr.Body.String())
	}
}
