package application_context

import (
	"testing"
	"time"
)

func TestMRQLGenerationRateLimiter(t *testing.T) {
	limiter := NewMRQLGenerationRateLimiter(2, time.Minute)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	if !limiter.Allow("user-1", now) {
		t.Fatal("first request should pass")
	}
	if !limiter.Allow("user-1", now.Add(time.Second)) {
		t.Fatal("second request should pass")
	}
	if limiter.Allow("user-1", now.Add(2*time.Second)) {
		t.Fatal("third request in same window should be limited")
	}
	if !limiter.Allow("user-1", now.Add(time.Minute+time.Second)) {
		t.Fatal("request after window should pass")
	}
	if !limiter.Allow("user-2", now.Add(2*time.Second)) {
		t.Fatal("different key should have independent quota")
	}
}

func TestMRQLGenerationRateLimiter_PrunesExpiredKeys(t *testing.T) {
	l := NewMRQLGenerationRateLimiter(2, time.Minute)
	t0 := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		l.Allow(time.Duration(i).String(), t0)
	}
	if !l.Allow("fresh", t0.Add(2*time.Minute)) {
		t.Fatal("fresh key should pass")
	}
	if len(l.keys) != 1 {
		t.Fatalf("expired buckets should be swept, have %d", len(l.keys))
	}
}

func TestMRQLGenerationRateLimiter_CapFailsClosed(t *testing.T) {
	l := NewMRQLGenerationRateLimiter(2, time.Minute)
	l.maxKeys = 3
	t0 := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	for _, k := range []string{"a", "b", "c"} {
		if !l.Allow(k, t0) {
			t.Fatalf("%s should pass", k)
		}
	}
	if l.Allow("d", t0.Add(time.Second)) {
		t.Fatal("new key past the cap must be refused")
	}
	if !l.Allow("a", t0.Add(time.Second)) {
		t.Fatal("existing key must still be allowed")
	}
	if !l.Allow("d", t0.Add(2*time.Minute)) {
		t.Fatal("new key admitted once buckets expire")
	}
}
