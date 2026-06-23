package server

import (
	"testing"
	"time"
)

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
