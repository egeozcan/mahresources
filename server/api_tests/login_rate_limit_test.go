package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
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

func setupTrustedRateLimitedEnv(t *testing.T, limit int) *TestContext {
	tc := setupRateLimitedAuthEnv(t, limit)
	tc.AppCtx.Config.TrustProxyHeaders = true
	return tc
}

func loginWithXFF(tc *TestContext, username, password, xff string) int {
	body := `{"username":"` + username + `","password":"` + password + `"}`
	return doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "X-Forwarded-For": xff}, nil, strings.NewReader(body)).Code
}

// A client that prepends forged entries to X-Forwarded-For (an appending proxy
// keeps them on the left) must not get a fresh per-IP bucket per forged value.
func TestLoginRateLimit_ForgedLeftmostXFFDoesNotEvadeIPBucket(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	for i := 1; i <= 3; i++ {
		xff := fmt.Sprintf("10.0.%d.%d, 203.0.113.9", i, i)
		if code := loginWithXFF(tc, fmt.Sprintf("victim%d", i), "wrong", xff); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i, code)
		}
	}
	if code := loginWithXFF(tc, "victim4", "wrong", "10.0.4.4, 203.0.113.9"); code != http.StatusTooManyRequests {
		t.Fatalf("forged leftmost entry evaded the IP bucket: got %d, want 429", code)
	}
}

func TestLoginRateLimit_TrustedXFFSeparatesRealClients(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 3)
	for i := 1; i <= 3; i++ {
		loginWithXFF(tc, fmt.Sprintf("victim%d", i), "wrong", "10.0.0.1, 203.0.113.9")
	}
	if code := loginWithXFF(tc, "victim4", "wrong", "10.0.0.1, 203.0.113.10"); code != http.StatusUnauthorized {
		t.Fatalf("a different real client must have its own bucket: got %d", code)
	}
}

func TestLoginSessionIP_UsesRightmostXFF(t *testing.T) {
	tc := setupTrustedRateLimitedEnv(t, 5)
	if code := loginWithXFF(tc, "admin", "adminpw1", "6.6.6.6, 203.0.113.9"); code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	var s models.Session
	if err := tc.DB.Order("id DESC").First(&s).Error; err != nil {
		t.Fatal(err)
	}
	if s.IP != "203.0.113.9" {
		t.Fatalf("session IP %q, want 203.0.113.9", s.IP)
	}
}
