package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

// Per-account throttling: the same username brute-forced from rotating IPs is
// still blocked once the per-user key hits the limit.
func TestLoginRateLimiter_PerUsernameAcrossIPs(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)
	for i, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		keys := loginKeys(ip, "victim")
		if !l.allowedAll(keys) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		l.recordFailureAll(keys)
	}
	// A 4th attempt from a brand-new IP is blocked by the per-username key.
	if l.allowedAll(loginKeys("4.4.4.4", "victim")) {
		t.Fatalf("per-username limit should block across rotating IPs")
	}
	// A different account from a fresh IP is unaffected.
	if !l.allowedAll(loginKeys("9.9.9.9", "other")) {
		t.Fatalf("a different account from a fresh IP should be allowed")
	}
}

// clientIP ignores X-Forwarded-For unless trustProxy is set.
func TestClientIP_XFFTrust(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "10.0.0.5:1111"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := clientIP(r, false); got != "10.0.0.5" {
		t.Fatalf("untrusted clientIP should use RemoteAddr, got %q", got)
	}
	if got := clientIP(r, true); got != "1.2.3.4" {
		t.Fatalf("trusted clientIP should use X-Forwarded-For, got %q", got)
	}
}

func TestLoginRateLimiter_BlocksAfterLimit(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)

	for i := 0; i < 3; i++ {
		if !l.allowed("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		l.recordFailure("1.2.3.4")
	}
	if l.allowed("1.2.3.4") {
		t.Fatalf("4th attempt should be blocked after 3 failures")
	}
	// A different IP is unaffected.
	if !l.allowed("5.6.7.8") {
		t.Fatalf("a different IP must not be throttled")
	}
}

func TestLoginRateLimiter_ResetClears(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)
	l.recordFailure("ip")
	l.recordFailure("ip")
	l.recordFailure("ip")
	if l.allowed("ip") {
		t.Fatalf("should be blocked before reset")
	}
	l.reset("ip")
	if !l.allowed("ip") {
		t.Fatalf("reset should clear the counter")
	}
}

func TestLoginRateLimiter_WindowExpiry(t *testing.T) {
	l := newLoginRateLimiter(2, time.Minute)
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	l.recordFailure("ip")
	l.recordFailure("ip")
	if l.allowed("ip") {
		t.Fatalf("blocked while within window")
	}
	// Advance past the window: stale failures are pruned.
	now = base.Add(2 * time.Minute)
	if !l.allowed("ip") {
		t.Fatalf("attempts should be allowed again after the window elapses")
	}
}

func TestLoginRateLimiter_DisabledWhenZero(t *testing.T) {
	l := newLoginRateLimiter(0, time.Hour)
	for i := 0; i < 100; i++ {
		l.recordFailure("ip")
	}
	if !l.allowed("ip") {
		t.Fatalf("limit 0 must disable throttling")
	}
	// A nil limiter is also a safe no-op.
	var nilL *loginRateLimiter
	if !nilL.allowed("ip") {
		t.Fatalf("nil limiter must allow")
	}
}
