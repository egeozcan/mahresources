// Package auth holds authentication and authorization primitives that are
// independent of the data-access layer: password hashing, token generation, and
// the request-time Principal that carries identity + role + group scope.
package auth

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for password hashing. DefaultCost (10) is a
// reasonable balance for an app meant for small private deployments.
const bcryptCost = bcrypt.DefaultCost

const (
	// MinPasswordLength is the minimum length (in Unicode code points) for a newly-set password.
	// Existing accounts are not re-validated on login — only password changes,
	// account creation, and bootstrap enforce this.
	MinPasswordLength = 8

	// MaxPasswordBytes is bcrypt's maximum password input length in UTF-8 bytes.
	// Longer input is rejected instead of being silently truncated.
	MaxPasswordBytes = 72
)

// ErrPasswordTooLong is returned when a password exceeds MaxPasswordBytes.
var ErrPasswordTooLong = fmt.Errorf("password must be at most %d bytes", MaxPasswordBytes)

// ErrPasswordTooShort is returned when a password is shorter than
// MinPasswordLength.
var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ValidatePassword enforces the password policy (currently a minimum length).
// Callers handle the empty-password case themselves (returning their own
// "password required" error) before delegating here.
func ValidatePassword(plaintext string) error {
	if utf8.RuneCountInString(plaintext) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(plaintext) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", ErrPasswordTooLong
		}
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
