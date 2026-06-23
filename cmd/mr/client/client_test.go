package client

import (
	"path/filepath"
	"testing"
)

// Stored tokens are bound to a server origin: a token saved for one server is
// never returned for (and so never sent to) a different host.
func TestTokenOriginBinding(t *testing.T) {
	t.Setenv("MR_TOKEN", "") // ignore any ambient global override
	t.Setenv("MR_TOKEN_FILE", filepath.Join(t.TempDir(), "creds.json"))

	const (
		srvA = "https://a.example.com"
		srvB = "http://b.example.com:9000"
	)
	if err := StoreToken(srvA, "tok-a"); err != nil {
		t.Fatalf("StoreToken A: %v", err)
	}
	if err := StoreToken(srvB, "tok-b"); err != nil {
		t.Fatalf("StoreToken B: %v", err)
	}

	if got := ResolveToken(srvA); got != "tok-a" {
		t.Fatalf("ResolveToken(A) = %q, want tok-a", got)
	}
	if got := ResolveToken(srvB); got != "tok-b" {
		t.Fatalf("ResolveToken(B) = %q, want tok-b", got)
	}
	// A server we never logged into gets no token (no cross-origin disclosure).
	if got := ResolveToken("https://evil.example.com"); got != "" {
		t.Fatalf("ResolveToken(unknown) = %q, want empty", got)
	}
	// Trailing slash / path / case differences resolve to the same origin.
	if got := ResolveToken("https://A.example.com/v1/"); got != "tok-a" {
		t.Fatalf("ResolveToken(A variant) = %q, want tok-a", got)
	}

	// Logout for A leaves B intact.
	if err := ClearToken(srvA); err != nil {
		t.Fatalf("ClearToken A: %v", err)
	}
	if got := ResolveToken(srvA); got != "" {
		t.Fatalf("after ClearToken(A), ResolveToken(A) = %q, want empty", got)
	}
	if got := ResolveToken(srvB); got != "tok-b" {
		t.Fatalf("after ClearToken(A), ResolveToken(B) = %q, want tok-b", got)
	}
}

// The MR_TOKEN env var remains an explicit global override.
func TestTokenEnvOverride(t *testing.T) {
	t.Setenv("MR_TOKEN_FILE", filepath.Join(t.TempDir(), "creds.json"))
	t.Setenv("MR_TOKEN", "env-token")
	if got := ResolveToken("https://anything.example.com"); got != "env-token" {
		t.Fatalf("ResolveToken with MR_TOKEN set = %q, want env-token", got)
	}
}
