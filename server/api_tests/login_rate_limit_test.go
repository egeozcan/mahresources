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

// A successful login before the limit resets the counter.
func TestLoginRateLimit_SuccessResets(t *testing.T) {
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
	// Counter reset: three more failures are allowed before throttling.
	for i := 0; i < 3; i++ {
		if code := apiLogin(tc, "admin", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("post-reset failure %d should be 401 (not throttled), got %d", i+1, code)
		}
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
