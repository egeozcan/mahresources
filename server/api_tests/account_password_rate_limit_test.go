package api_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// newPasswordUser creates an account and a Bearer token for it.
func newPasswordUser(t *testing.T, tc *TestContext, username, password string, role models.Role) string {
	t.Helper()
	var scope *uint
	if role.AllowsScopeGroup() && role.RequiresScopeGroup() {
		g := &models.Group{Name: "scope-" + username}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("scope group: %v", err)
		}
		scope = &g.ID
	}
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: password, Role: role, ScopeGroupId: scope,
	})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return "Bearer " + raw
}

func changePassword(tc *TestContext, bearer, xff, current, next string) int {
	h := map[string]string{"Content-Type": "application/json", "Authorization": bearer, "X-Forwarded-For": xff}
	body := fmt.Sprintf(`{"currentPassword":%q,"newPassword":%q}`, current, next)
	return doReq(tc, http.MethodPost, "/v1/account/password", h, nil, strings.NewReader(body)).Code
}

func TestChangeOwnPasswordRateLimit_BearerThrottlesAfterLimit(t *testing.T) {
	for i, role := range []models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser, models.RoleGuest} {
		t.Run(string(role), func(t *testing.T) {
			tc := setupTrustedRateLimitedEnv(t, 3)
			name := "victim"
			bearer := newPasswordUser(t, tc, name, "password1", role)
			for n := 0; n < 3; n++ {
				if code := changePassword(tc, bearer, "10.0.0.1", "wrong", "attackerpw1"); code != http.StatusUnauthorized {
					t.Fatalf("wrong attempt %d: want 401, got %d", n+1, code)
				}
			}
			// A fresh IP: only the shared user: key can refuse this.
			if code := changePassword(tc, bearer, "10.0.0.2", "password1", "attackerpw1"); code != http.StatusTooManyRequests {
				t.Fatalf("role %d %s: past the limit want 429, got %d", i, role, code)
			}
			if _, err := tc.AppCtx.AuthenticateUser(name, "password1"); err != nil {
				t.Fatalf("original password must survive: %v", err)
			}
			if _, err := tc.AppCtx.AuthenticateUser(name, "attackerpw1"); err == nil {
				t.Fatal("password must not have changed")
			}
			// The user: key is shared with login, but throttling is per account.
			if code := loginWithXFF(tc, name, "password1", "10.0.0.3"); code != http.StatusTooManyRequests {
				t.Fatalf("login for the throttled account: want 429, got %d", code)
			}
			if code := loginWithXFF(tc, "admin", "wrong", "10.0.0.3"); code != http.StatusUnauthorized {
				t.Fatalf("login for another account: want 401, got %d", code)
			}
		})
	}
}

func TestChangeOwnPasswordRateLimit_FailedLoginsThrottlePasswordChange(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	bearer := newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	for n := 0; n < 3; n++ {
		if code := loginWithXFF(tc, "victim", "wrong", "10.0.0.1"); code != http.StatusUnauthorized {
			t.Fatalf("login %d: %d", n+1, code)
		}
	}
	if code := changePassword(tc, bearer, "10.0.0.2", "password1", "newpassword1"); code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", code)
	}
}

func TestChangeOwnPasswordRateLimit_CookieCSRF(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	rr := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "X-Forwarded-For": "10.0.0.9"}, nil,
		strings.NewReader(`{"username":"victim","password":"password1"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("login: %d", rr.Code)
	}
	cookie := sessionCookie(t, rr)
	csrf := csrfFor(t, tc, cookie)
	post := func(xff, current string) int {
		h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": csrf, "X-Forwarded-For": xff}
		body := fmt.Sprintf(`{"currentPassword":%q,"newPassword":"attackerpw1"}`, current)
		return doReq(tc, http.MethodPost, "/v1/account/password", h, []*http.Cookie{cookie}, strings.NewReader(body)).Code
	}
	for n := 0; n < 3; n++ {
		if code := post("10.0.0.1", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", n+1, code)
		}
	}
	if code := post("10.0.0.2", "password1"); code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", code)
	}
}

func TestChangeOwnPasswordRateLimit_ValidationErrorsNotCharged(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	bearer := newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	for n := 0; n < 5; n++ {
		if code := changePassword(tc, bearer, "10.0.0.1", "wrong", "x"); code != http.StatusBadRequest {
			t.Fatalf("validation attempt %d: want 400, got %d", n+1, code)
		}
	}
	if code := changePassword(tc, bearer, "10.0.0.1", "password1", "newpassword1"); code != http.StatusOK {
		t.Fatalf("correct change after validation errors: want 200, got %d", code)
	}
}

func TestChangeOwnPasswordRateLimit_SuccessClearsAccountFailures(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	bearer := newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	for n := 0; n < 2; n++ {
		if code := changePassword(tc, bearer, "10.0.0.1", "wrong", "newpassword1"); code != http.StatusUnauthorized {
			t.Fatalf("wrong %d: %d", n+1, code)
		}
	}
	if code := changePassword(tc, bearer, "10.0.0.1", "password1", "newpassword1"); code != http.StatusOK {
		t.Fatalf("correct: %d", code)
	}
	for n := 0; n < 2; n++ {
		if code := changePassword(tc, bearer, "10.0.0.7", "wrong", "x1234567"); code != http.StatusUnauthorized {
			t.Fatalf("post-success wrong %d: want 401, got %d", n+1, code)
		}
	}
}

func TestChangeOwnPasswordRateLimit_DisabledByDefault(t *testing.T) {
	tc := setupAuthEnv(t)
	bearer := newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	for n := 0; n < 10; n++ {
		if code := changePassword(tc, bearer, "10.0.0.1", "wrong", "newpassword1"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", n+1, code)
		}
	}
}

func TestChangeOwnPasswordAuthOffStillRejected(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := doReq(tc, http.MethodPost, "/v1/account/password", map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"currentPassword":"a","newPassword":"newpassword1"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

// With -trust-proxy-headers off (the default) the guard must key wrong
// current-password guesses on the socket address, never on X-Forwarded-For:
// otherwise any account could fill the ip: bucket of an address it names, locking
// logins out from there, or spread its guesses over forged per-IP buckets.
func TestChangeOwnPasswordRateLimit_UntrustedXFFIgnored(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 3)
	bearer := newPasswordUser(t, tc, "victim", "password1", models.RoleUser)
	for n := 0; n < 3; n++ {
		if code := changePassword(tc, bearer, "203.0.113.50", "wrong", "attackerpw1"); code != http.StatusUnauthorized {
			t.Fatalf("wrong attempt %d: want 401, got %d", n+1, code)
		}
	}
	login := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login",
			strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteAddr
		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)
		return rr.Code
	}
	if code := login("203.0.113.50:4444"); code != http.StatusOK {
		t.Fatalf("login from the forged address: want 200, got %d", code)
	}
	// The guesses were charged to the socket address httptest requests carry.
	if code := login("192.0.2.1:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("login from the socket address: want 429, got %d", code)
	}
}
