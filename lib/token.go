package lib

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateShareToken generates a cryptographically secure 32-character hex token
func GenerateShareToken() string {
	bytes := make([]byte, 16) // 16 bytes = 128 bits = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		panic(err) // crypto/rand should never fail
	}
	return hex.EncodeToString(bytes)
}
