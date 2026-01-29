package lib

import (
	"testing"
)

func TestGenerateShareToken(t *testing.T) {
	token := GenerateShareToken()

	if len(token) != 32 {
		t.Errorf("Expected token length 32, got %d", len(token))
	}

	// Verify it's hex characters only
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Token contains non-hex character: %c", c)
		}
	}
}

func TestGenerateShareTokenUniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := GenerateShareToken()
		if tokens[token] {
			t.Errorf("Duplicate token generated: %s", token)
		}
		tokens[token] = true
	}
}
