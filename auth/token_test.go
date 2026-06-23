package auth

import "testing"

func TestGenerateTokenUniqueAndHashable(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	b, _ := GenerateToken()
	if a == "" || a == b {
		t.Fatalf("tokens must be non-empty and unique, got %q and %q", a, b)
	}

	h := HashToken(a)
	if len(h) != 64 {
		t.Errorf("HashToken should return a 64-char hex string, got len %d", len(h))
	}
	if HashToken(a) != h {
		t.Error("HashToken must be deterministic")
	}
	if HashToken(b) == h {
		t.Error("different tokens must hash differently")
	}
}

func TestTokenPrefix(t *testing.T) {
	if p := TokenPrefix("abcdefghijklmnop"); p != "abcdefgh" {
		t.Errorf("expected 8-char prefix, got %q", p)
	}
	if p := TokenPrefix("short"); p != "short" {
		t.Errorf("short token should be returned whole, got %q", p)
	}
}
