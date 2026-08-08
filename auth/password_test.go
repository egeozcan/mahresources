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

func TestMaxPasswordBytesMatchesValidationBoundary(t *testing.T) {
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes)); err != nil {
		t.Fatalf("ValidatePassword(MaxPasswordBytes) = %v, want nil", err)
	}
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes+1)); err != ErrPasswordTooLong {
		t.Fatalf("ValidatePassword(MaxPasswordBytes+1) = %v, want ErrPasswordTooLong", err)
	}
}

func TestValidatePassword(t *testing.T) {
	// Below the minimum is rejected.
	for _, s := range []string{"", "a", "1234567"} {
		if err := ValidatePassword(s); err != ErrPasswordTooShort {
			t.Errorf("ValidatePassword(%q) = %v, want ErrPasswordTooShort", s, err)
		}
	}
	// At or above the minimum is accepted.
	for _, s := range []string{"12345678", "correct horse battery staple"} {
		if err := ValidatePassword(s); err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", s, err)
		}
	}
	// The minimum counts Unicode code points, not UTF-8 bytes.
	if err := ValidatePassword("界界界界界界界"); err != ErrPasswordTooShort {
		t.Errorf("ValidatePassword(7 Unicode code points) = %v, want ErrPasswordTooShort", err)
	}
	if err := ValidatePassword("界界界界界界界界"); err != nil {
		t.Errorf("ValidatePassword(8 Unicode code points) = %v, want nil", err)
	}
	// Bcrypt's maximum still counts encoded bytes.
	if err := ValidatePassword(strings.Repeat("界", 25)); err != ErrPasswordTooLong {
		t.Errorf("ValidatePassword(75 UTF-8 bytes) = %v, want ErrPasswordTooLong", err)
	}
	if err := ValidatePassword(strings.Repeat("a", 73)); err != ErrPasswordTooLong {
		t.Errorf("ValidatePassword(73 bytes) = %v, want ErrPasswordTooLong", err)
	}
}
