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
