package fts

import "testing"

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTerm  string
		wantMode  SearchMode
		wantDist  int
	}{
		{
			name:     "empty string",
			input:    "",
			wantTerm: "",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			wantTerm: "",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "simple word",
			input:    "hello",
			wantTerm: "hello",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "word with leading/trailing spaces",
			input:    "  hello  ",
			wantTerm: "hello",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "prefix search",
			input:    "typ*",
			wantTerm: "typ",
			wantMode: ModePrefix,
			wantDist: 0,
		},
		{
			name:     "prefix search with spaces",
			input:    "  hello world*  ",
			wantTerm: "hello world",
			wantMode: ModePrefix,
			wantDist: 0,
		},
		{
			name:     "fuzzy search default distance",
			input:    "~test",
			wantTerm: "test",
			wantMode: ModeFuzzy,
			wantDist: 1,
		},
		{
			name:     "fuzzy search with distance 2",
			input:    "~2test",
			wantTerm: "test",
			wantMode: ModeFuzzy,
			wantDist: 2,
		},
		{
			name:     "fuzzy search with distance 3",
			input:    "~3hello",
			wantTerm: "hello",
			wantMode: ModeFuzzy,
			wantDist: 3,
		},
		{
			name:     "fuzzy search distance capped at 3",
			input:    "~9test",
			wantTerm: "test",
			wantMode: ModeFuzzy,
			wantDist: 3,
		},
		{
			name:     "fuzzy search zero distance becomes 1",
			input:    "~0test",
			wantTerm: "test",
			wantMode: ModeFuzzy,
			wantDist: 1,
		},
		{
			name:     "fuzzy search with spaces",
			input:    "  ~2hello  ",
			wantTerm: "hello",
			wantMode: ModeFuzzy,
			wantDist: 2,
		},
		{
			name:     "special characters removed",
			input:    "hello@#$world",
			wantTerm: "helloworld",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "allowed special chars preserved",
			input:    "hello-world_test.go",
			wantTerm: "hello-world_test.go",
			wantMode: ModeExact,
			wantDist: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSearchQuery(tt.input)
			if got.Term != tt.wantTerm {
				t.Errorf("ParseSearchQuery(%q).Term = %q, want %q", tt.input, got.Term, tt.wantTerm)
			}
			if got.Mode != tt.wantMode {
				t.Errorf("ParseSearchQuery(%q).Mode = %v, want %v", tt.input, got.Mode, tt.wantMode)
			}
			if got.FuzzyDist != tt.wantDist {
				t.Errorf("ParseSearchQuery(%q).FuzzyDist = %d, want %d", tt.input, got.FuzzyDist, tt.wantDist)
			}
		})
	}
}

func TestSanitizeSearchTerm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal text",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "special characters removed",
			input: "hello@#$%^&*()world",
			want:  "helloworld",
		},
		{
			name:  "allowed characters preserved",
			input: "hello-world_123.test",
			want:  "hello-world_123.test",
		},
		{
			name:  "unicode letters preserved",
			input: "café résumé",
			want:  "café résumé",
		},
		{
			name:  "SQL injection attempt sanitized",
			input: "'; DROP TABLE users;--",
			want:  "DROP TABLE users--",
		},
		{
			name:  "leading/trailing spaces trimmed",
			input: "  test  ",
			want:  "test",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only special characters",
			input: "@#$%^&*()",
			want:  "",
		},
		{
			name:  "brackets and quotes removed",
			input: "test[0]\"quoted\"",
			want:  "test0quoted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSearchTerm(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeSearchTerm(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeForFTS(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no quotes",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single quote",
			input: "it's",
			want:  "it''s",
		},
		{
			name:  "multiple quotes",
			input: "it's a 'test'",
			want:  "it''s a ''test''",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only quotes",
			input: "'''",
			want:  "''''''",
		},
		{
			name:  "quote at beginning",
			input: "'hello",
			want:  "''hello",
		},
		{
			name:  "quote at end",
			input: "hello'",
			want:  "hello''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeForFTS(tt.input)
			if got != tt.want {
				t.Errorf("EscapeForFTS(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
