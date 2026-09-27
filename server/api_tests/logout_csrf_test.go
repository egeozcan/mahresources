package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

// sessionStillValid reports whether a session cookie still authenticates.
func sessionStillValid(t *testing.T, tc *TestContext, cookie *http.Cookie) bool {
	t.Helper()
	me := doReq(tc, http.MethodGet, "/v1/auth/me",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	switch me.Code {
	case http.StatusOK:
		return true
	case http.StatusUnauthorized:
		return false
	default:
		t.Fatalf("/v1/auth/me answered %d: %s", me.Code, me.Body.String())
		return false
	}
}

// A cross-site link is a top-level GET, and SameSite=Lax sends the session cookie
// with it. A GET to /logout therefore must not end the session: it offers the
// sign-out button instead, as a form carrying the session's CSRF token.
func TestLogoutByGETOffersAFormAndKeepsTheSession(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	page := doReq(tc, http.MethodGet, "/logout", map[string]string{
		"Accept":         "text/html",
		"Sec-Fetch-Site": "cross-site",
		"Referer":        "https://elsewhere.example/",
	}, []*http.Cookie{cookie}, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("GET /logout should render the sign-out page, got %d (%s)", page.Code, page.Body.String())
	}
	body := page.Body.String()
	if !strings.Contains(body, `action="/logout"`) || !strings.Contains(strings.ToLower(body), `method="post"`) {
		t.Fatalf("the sign-out page must post a form to /logout, got: %s", body)
	}
	if !strings.Contains(body, `name="csrf_token" value="`+token+`"`) {
		t.Fatalf("the sign-out form must carry the session's CSRF token, got: %s", body)
	}
	if !sessionStillValid(t, tc, cookie) {
		t.Fatalf("GET /logout ended the session")
	}
}

// The header's Sign out control is a native form, so it works without
// JavaScript; it has to carry the token the POST now requires.
func TestTheHeaderSignOutFormCarriesTheCSRFToken(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	page := doReq(tc, http.MethodGet, "/dashboard", map[string]string{"Accept": "text/html"}, []*http.Cookie{cookie}, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("dashboard answered %d", page.Code)
	}
	body := page.Body.String()
	form := strings.Index(body, `action="/logout"`)
	if form < 0 {
		t.Fatalf("the dashboard renders no sign-out form")
	}
	end := strings.Index(body[form:], "</form>")
	if end < 0 || !strings.Contains(body[form:form+end], `name="csrf_token" value="`+token+`"`) {
		t.Fatalf("the header's sign-out form carries no CSRF token: %s", body[form:])
	}
}

func TestLogoutPostWithoutTheCSRFTokenIsRefused(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	for _, path := range []string{"/logout", "/v1/auth/logout"} {
		refused := doReq(tc, http.MethodPost, path, map[string]string{
			"Content-Type": urlEncoded, "Accept": "text/html",
		}, []*http.Cookie{cookie}, strings.NewReader(""))
		if refused.Code != http.StatusForbidden {
			t.Fatalf("POST %s without a CSRF token should be 403, got %d (%s)", path, refused.Code, refused.Body.String())
		}
		if !sessionStillValid(t, tc, cookie) {
			t.Fatalf("a POST %s without a CSRF token ended the session", path)
		}
	}
}

func TestLogoutPostWithTheCSRFTokenEndsTheSession(t *testing.T) {
	tc := setupAuthEnv(t)

	cookie, token := loginCookieAndCSRF(t, tc)
	form := doReq(tc, http.MethodPost, "/logout", map[string]string{
		"Content-Type": urlEncoded, "Accept": "text/html",
	}, []*http.Cookie{cookie}, strings.NewReader("csrf_token="+token))
	if form.Code != http.StatusFound || form.Header().Get("Location") != "/login" {
		t.Fatalf("POST /logout with the token should redirect to /login, got %d %q", form.Code, form.Header().Get("Location"))
	}
	if sessionStillValid(t, tc, cookie) {
		t.Fatalf("POST /logout with the token left the session valid")
	}

	cookie, token = loginCookieAndCSRF(t, tc)
	api := doReq(tc, http.MethodPost, "/v1/auth/logout", map[string]string{"X-CSRF-Token": token}, []*http.Cookie{cookie}, nil)
	if api.Code != http.StatusOK {
		t.Fatalf("POST /v1/auth/logout with the token should be 200, got %d (%s)", api.Code, api.Body.String())
	}
	if sessionStillValid(t, tc, cookie) {
		t.Fatalf("POST /v1/auth/logout with the token left the session valid")
	}
}
