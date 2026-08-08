package api_tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
)

// setupRateLimitedAuthEnv builds an auth-enabled server that throttles after
// `limit` failed logins per IP within a one-hour window.
func setupRateLimitedAuthEnv(t *testing.T, limit int) *TestContext {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
		c.LoginRateLimit = limit
		c.LoginRateWindow = time.Hour
	})
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return tc
}

func apiLogin(tc *TestContext, username, password string) int {
	body := `{"username":"` + username + `","password":"` + password + `"}`
	return doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil, strings.NewReader(body)).Code
}

// After `limit` failed attempts the next login is throttled with HTTP 429, even
// with correct credentials.
func TestLoginRateLimit_BlocksAfterLimit(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 3)

	for i := 0; i < 3; i++ {
		if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d should be 401, got %d", i+1, code)
		}
	}
	// Limit reached: even the correct password is throttled.
	if code := apiLogin(tc, "admin", "adminpw1"); code != http.StatusTooManyRequests {
		t.Fatalf("attempt past the limit should be 429, got %d", code)
	}
}

// A successful login clears that username's failures but preserves failures for
// the source IP, so switching accounts cannot reset an attacker's IP counter.
func TestLoginRateLimit_SuccessPreservesIPFailures(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 3)

	if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("first failure should be 401, got %d", code)
	}
	if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("second failure should be 401, got %d", code)
	}
	if code := apiLogin(tc, "admin", "adminpw1"); code != http.StatusOK {
		t.Fatalf("correct login (under limit) should be 200, got %d", code)
	}
	if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("third IP failure should be 401, got %d", code)
	}
	if code := apiLogin(tc, "admin", "wrong"); code != http.StatusTooManyRequests {
		t.Fatalf("success must not reset IP failures; got %d, want 429", code)
	}
}

func TestLoginRateLimit_SuccessfulOtherAccountDoesNotResetIP(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 4)
	if _, err := tc.AppCtx.EnsureAdminUser("attacker", "attackerpw1"); err != nil {
		t.Fatalf("create attacker-owned account: %v", err)
	}

	for i := 0; i < 3; i++ {
		if code := apiLogin(tc, "victim", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("victim failure %d should be 401, got %d", i+1, code)
		}
	}
	if code := apiLogin(tc, "attacker", "attackerpw1"); code != http.StatusOK {
		t.Fatalf("attacker's valid login should be 200, got %d", code)
	}
	if code := apiLogin(tc, "another-victim", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("fourth IP failure should be 401, got %d", code)
	}
	if code := apiLogin(tc, "different-victim", "wrong"); code != http.StatusTooManyRequests {
		t.Fatalf("IP failures must survive another account's success; got %d, want 429", code)
	}
}

// With rate-limiting disabled (limit 0, the default) repeated failures are never
// throttled.
func TestLoginRateLimit_DisabledByDefault(t *testing.T) {
	tc := setupAuthEnv(t) // LoginRateLimit defaults to 0
	for i := 0; i < 10; i++ {
		if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d should be 401 (never 429 when disabled), got %d", i+1, code)
		}
	}
}
