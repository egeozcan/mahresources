package server

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
)

// Per-account throttling: the same username brute-forced from rotating IPs is
// still blocked once the per-user key hits the limit.
func TestLoginRateLimiter_PerUsernameAcrossIPs(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)
	for i, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		attempt, ok := l.reserve(loginKeys(ip, "victim"))
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		attempt.complete(false)
	}
	// A 4th attempt from a brand-new IP is blocked by the per-username key.
	if _, ok := l.reserve(loginKeys("4.4.4.4", "victim")); ok {
		t.Fatalf("per-username limit should block across rotating IPs")
	}
	// A different account from a fresh IP is unaffected.
	attempt, ok := l.reserve(loginKeys("9.9.9.9", "other"))
	if !ok {
		t.Fatalf("a different account from a fresh IP should be allowed")
	}
	attempt.complete(true)
}

func TestLoginRateLimiter_ConcurrentReservations(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)
	const competitors = 20

	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(competitors)
	var checked sync.WaitGroup
	checked.Add(competitors)
	var done sync.WaitGroup
	done.Add(competitors)
	var granted atomic.Int32

	for i := 0; i < competitors; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			attempt, ok := l.reserve([]string{"ip:shared", "user:victim"})
			if ok {
				granted.Add(1)
			}
			checked.Done()
			checked.Wait()
			if ok {
				attempt.complete(false)
			}
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	if got := granted.Load(); got > 3 {
		t.Fatalf("granted %d concurrent reservations with limit 3", got)
	}
}

func TestLoginRateLimiter_SuccessfulOtherAccountPreservesIPFailures(t *testing.T) {
	l := newLoginRateLimiter(4, time.Hour)
	for i := 0; i < 3; i++ {
		attempt, ok := l.reserve(loginKeys("1.2.3.4", "victim"))
		if !ok {
			t.Fatalf("victim failure %d should be reserved", i+1)
		}
		attempt.complete(false)
	}

	attacker, ok := l.reserve(loginKeys("1.2.3.4", "attacker"))
	if !ok {
		t.Fatal("attacker's valid login should be allowed before the limit")
	}
	attacker.complete(true)

	lastFailure, ok := l.reserve(loginKeys("1.2.3.4", "another-victim"))
	if !ok {
		t.Fatal("fourth failed attempt should be reserved")
	}
	lastFailure.complete(false)
	if _, ok := l.reserve(loginKeys("1.2.3.4", "different-victim")); ok {
		t.Fatal("successful login to another account must not clear the IP's failures")
	}
}

func TestLoginRateLimiter_BoundsKeys(t *testing.T) {
	l := newLoginRateLimiter(100, time.Hour)
	l.maxKeys = 10
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	for i := 0; i < 100; i++ {
		attempt, ok := l.reserve([]string{fmt.Sprintf("ip:%d", i)})
		if ok {
			attempt.complete(false)
		}
	}

	l.mu.Lock()
	got := len(l.fails)
	l.mu.Unlock()
	if got > l.maxKeys {
		t.Fatalf("tracked %d keys with configured bound %d", got, l.maxKeys)
	}
	if got != l.maxKeys {
		t.Fatalf("tracked %d keys, want the configured bound %d", got, l.maxKeys)
	}
	if _, ok := l.reserve([]string{"ip:fresh"}); ok {
		t.Fatal("an untracked key must fail closed while the map contains live failures")
	}

	now = base.Add(2 * time.Hour)
	attempt, ok := l.reserve([]string{"ip:fresh"})
	if !ok {
		t.Fatal("a fresh key should be admitted after stale failures are swept")
	}
	attempt.complete(false)
}

func TestLoginRateLimiter_BoundsKeysRetainsPendingReservations(t *testing.T) {
	l := newLoginRateLimiter(2, time.Hour)
	l.maxKeys = 1
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	pending, ok := l.reserve([]string{"ip:pending"})
	if !ok {
		t.Fatal("initial reservation should be admitted")
	}
	now = base.Add(2 * time.Hour)
	if _, ok := l.reserve([]string{"ip:fresh"}); ok {
		t.Fatal("capacity sweep must not evict an active reservation")
	}
	pending.complete(true)
	fresh, ok := l.reserve([]string{"ip:fresh"})
	if !ok {
		t.Fatal("fresh key should be admitted after the active reservation completes")
	}
	fresh.complete(true)
}

// Database contention is not a wrong password. Charging it to the limiter would
// let a stall lock out an account nobody attacked; clearing prior failures would
// let an attacker wipe their own record by inducing one.
func TestLoginRateLimiter_UnavailableAttemptIsNeitherFailureNorSuccess(t *testing.T) {
	l := newLoginRateLimiter(2, time.Hour)
	keys := []string{"ip:1.2.3.4", "user:victim"}

	if _, err := runLoginAttempt(l, keys, func() error { return errors.New("wrong password") }); err == nil {
		t.Fatal("a credential failure should surface as an error")
	}
	if _, err := runLoginAttempt(l, keys, func() error {
		return application_context.ErrLoginUnavailable
	}); !errors.Is(err, application_context.ErrLoginUnavailable) {
		t.Fatalf("unavailable attempt err=%v, want ErrLoginUnavailable", err)
	}

	// One real failure was recorded, so with a limit of two there is still room.
	// Had the stall counted, this reservation would be refused.
	attempt, ok := l.reserve(keys)
	if !ok {
		t.Fatal("a database stall must not consume the account's login attempts")
	}
	attempt.complete(false)
	if _, ok := l.reserve(keys); ok {
		t.Fatal("two recorded failures should exhaust the limit")
	}

	// And it must not have erased the failure that came before it.
	fresh := newLoginRateLimiter(1, time.Hour)
	if _, err := runLoginAttempt(fresh, keys, func() error { return errors.New("wrong password") }); err == nil {
		t.Fatal("a credential failure should surface as an error")
	}
	runLoginAttempt(fresh, keys, func() error { return application_context.ErrLoginUnavailable })
	if _, ok := fresh.reserve(keys); ok {
		t.Fatal("a database stall must not clear an attacker's accumulated failures")
	}
}

func TestLoginRateLimiter_PanicCompletesReservationAsFailure(t *testing.T) {
	l := newLoginRateLimiter(1, time.Hour)
	l.maxKeys = 1
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	panicValue := &struct{ message string }{"authentication panic"}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		runLoginAttempt(l, []string{"ip:panic"}, func() error {
			panic(panicValue)
		})
	}()
	if recovered != panicValue {
		t.Fatalf("panic did not propagate unchanged: got %#v, want %#v", recovered, panicValue)
	}

	if _, ok := l.reserve([]string{"ip:panic"}); ok {
		t.Fatal("a panicked authentication must convert its reservation to a failure")
	}
	now = base.Add(2 * time.Hour)
	fresh, ok := l.reserve([]string{"ip:fresh"})
	if !ok {
		t.Fatal("the converted failure should expire and release bounded key capacity")
	}
	fresh.complete(true)
}

func TestLoginRateLimiter_VeryLongUsernameUsesFixedSizeKeys(t *testing.T) {
	l := newLoginRateLimiter(2, time.Hour)
	username := strings.Repeat("Ab", 128*1024)

	first, ok := l.reserve(loginKeys("1.1.1.1", "  "+username+"  "))
	if !ok {
		t.Fatal("first long-username attempt should be admitted")
	}
	first.complete(false)
	second, ok := l.reserve(loginKeys("2.2.2.2", strings.ToLower(username)))
	if !ok {
		t.Fatal("normalized long-username attempt should share the account key")
	}
	second.complete(false)
	if _, ok := l.reserve(loginKeys("3.3.3.3", username)); ok {
		t.Fatal("ordinary per-account throttling must apply to a very long username")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.fails {
		if got := len(key); got != 33 {
			t.Fatalf("retained login key is %d bytes, want fixed 33-byte kind+SHA-256 key", got)
		}
	}
}

func TestLoginRateLimiter_CapacityRejectionsDoNotSweepEveryTime(t *testing.T) {
	l := newLoginRateLimiter(100, time.Hour)
	l.maxKeys = 4
	base := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return base }
	var sweepVisits int
	l.onSweepVisit = func() { sweepVisits++ }

	for i := 0; i < l.maxKeys; i++ {
		attempt, ok := l.reserve([]string{fmt.Sprintf("ip:full-%d", i)})
		if !ok {
			t.Fatalf("key %d should fill available capacity", i)
		}
		attempt.complete(false)
	}
	for i := 0; i < 20; i++ {
		if _, ok := l.reserve([]string{fmt.Sprintf("ip:rejected-%d", i)}); ok {
			t.Fatalf("new key %d should fail closed at capacity", i)
		}
	}
	if sweepVisits != l.maxKeys {
		t.Fatalf("capacity rejections visited %d keys, want one bounded sweep of %d keys", sweepVisits, l.maxKeys)
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
		attempt, ok := l.reserve([]string{"ip:1.2.3.4"})
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		attempt.complete(false)
	}
	if _, ok := l.reserve([]string{"ip:1.2.3.4"}); ok {
		t.Fatalf("4th attempt should be blocked after 3 failures")
	}
	// A different IP is unaffected.
	attempt, ok := l.reserve([]string{"ip:5.6.7.8"})
	if !ok {
		t.Fatalf("a different IP must not be throttled")
	}
	attempt.complete(true)
}

func TestLoginRateLimiter_SuccessClearsUsernameFailures(t *testing.T) {
	l := newLoginRateLimiter(3, time.Hour)
	for i := 0; i < 2; i++ {
		attempt, ok := l.reserve([]string{"user:alice"})
		if !ok {
			t.Fatalf("failure %d should be allowed", i+1)
		}
		attempt.complete(false)
	}
	attempt, ok := l.reserve([]string{"user:alice"})
	if !ok {
		t.Fatal("successful attempt under the limit should be allowed")
	}
	attempt.complete(true)

	for i := 0; i < 3; i++ {
		attempt, ok = l.reserve([]string{"user:alice"})
		if !ok {
			t.Fatalf("post-success failure %d should be allowed", i+1)
		}
		attempt.complete(false)
	}
	if _, ok := l.reserve([]string{"user:alice"}); ok {
		t.Fatal("username should be blocked after three new failures")
	}
}

func TestLoginRateLimiter_WindowExpiry(t *testing.T) {
	l := newLoginRateLimiter(2, time.Minute)
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		attempt, ok := l.reserve([]string{"ip"})
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		attempt.complete(false)
	}
	if _, ok := l.reserve([]string{"ip"}); ok {
		t.Fatalf("blocked while within window")
	}
	// Advance past the window: stale failures are pruned.
	now = base.Add(2 * time.Minute)
	attempt, ok := l.reserve([]string{"ip"})
	if !ok {
		t.Fatalf("attempts should be allowed again after the window elapses")
	}
	attempt.complete(true)
}

func TestLoginRateLimiter_DisabledWhenZero(t *testing.T) {
	l := newLoginRateLimiter(0, time.Hour)
	for i := 0; i < 100; i++ {
		attempt, ok := l.reserve([]string{"ip"})
		if !ok {
			t.Fatal("limit 0 must disable throttling")
		}
		attempt.complete(false)
	}
	// A nil limiter is also a safe no-op.
	var nilL *loginRateLimiter
	attempt, ok := nilL.reserve([]string{"ip"})
	if !ok {
		t.Fatal("nil limiter must allow")
	}
	attempt.complete(false)
}
