package models

import "testing"

func TestLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "Hi", 10, "Hi"},
		{"exact length ASCII", "abcde", 5, "abcde"},
		{"over limit ASCII", "abcdefgh", 5, "ab..."},
		{"well under limit", "ab", 5, "ab"},
		{"multi-byte within limit", "café", 5, "café"},
		{"multi-byte exact rune limit", "日本語テス", 5, "日本語テス"},
		{"multi-byte over rune limit", "日本語テストBB", 5, "日本..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := limit(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("limit(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}
