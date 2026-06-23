package auth

import (
	"strings"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || hash == "correct horse battery staple" {
		t.Fatalf("hash must be non-empty and not the plaintext, got %q", hash)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Error("CheckPassword should accept the correct password")
	}
	if CheckPassword(hash, "wrong password") {
		t.Error("CheckPassword should reject an incorrect password")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	h1, _ := HashPassword("samepassword")
	h2, _ := HashPassword("samepassword")
	if h1 == h2 {
		t.Error("two hashes of the same password should differ (salted)")
	}
}

func TestHashPasswordTooLong(t *testing.T) {
	long := strings.Repeat("a", 100)
	if _, err := HashPassword(long); err != ErrPasswordTooLong {
		t.Errorf("expected ErrPasswordTooLong, got %v", err)
	}
}
