package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// tokenBytes is the number of random bytes in a session/API token (256 bits).
const tokenBytes = 32

// GenerateToken returns a cryptographically-random, URL-safe token string.
// The raw token is the only form ever shown to a client; only its hash is stored.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the hex SHA-256 of a raw token (64 chars), suitable for DB
// storage and constant-cost lookup. Tokens are high-entropy random values, so a
// fast hash (vs bcrypt) is appropriate and avoids per-request hashing cost.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// TokenPrefix returns a short, non-secret display fragment of a raw token.
func TokenPrefix(raw string) string {
	const n = 8
	if len(raw) <= n {
		return raw
	}
	return raw[:n]
}
