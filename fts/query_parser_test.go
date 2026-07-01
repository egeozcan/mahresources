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
			wantMode: ModePrefix,
			wantDist: 0,
		},
		{
			name:     "explicit exact search with quotes",
			input:    "\"hello\"",
			wantTerm: "hello",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "explicit exact search with equals",
			input:    "=hello",
			wantTerm: "hello",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "short term defaults to exact",
			input:    "hi",
			wantTerm: "hi",
			wantMode: ModeExact,
			wantDist: 0,
		},
		{
			name:     "short term with explicit prefix",
			input:    "hi*",
			wantTerm: "hi",
			wantMode: ModePrefix,
			wantDist: 0,
		},
		{
			name:     "word with leading/trailing spaces",
			input:    "  hello  ",
			wantTerm: "hello",
			wantMode: ModePrefix,
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
			wantMode: ModePrefix,
			wantDist: 0,
		},
		{
			name:     "allowed special chars preserved (hyphens become spaces)",
			input:    "hello-world_test.go",
			wantTerm: "hello world_test.go",
			wantMode: ModePrefix,
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

// TestParseSearchQueryRawTerm locks in that RawTerm preserves hyphens (used by
// the Postgres provider so a query tokenizes like the stored tsvector) while
// Term still collapses them (SQLite FTS5). See fts/postgres.go BuildSearchScope.
func TestParseSearchQueryRawTerm(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTerm    string
		wantRawTerm string
		wantMode    SearchMode
	}{
		{
			name:        "hyphenated term: Term collapses, RawTerm preserves",
			input:       "invoice 2024-3q",
			wantTerm:    "invoice 2024 3q",
			wantRawTerm: "invoice 2024-3q",
			wantMode:    ModePrefix,
		},
		{
			name:        "no hyphen: Term and RawTerm identical",
			input:       "hello world",
			wantTerm:    "hello world",
			wantRawTerm: "hello world",
			wantMode:    ModePrefix,
		},
		{
			name:        "explicit exact keeps hyphen in RawTerm",
			input:       "=well-known",
			wantTerm:    "well known",
			wantRawTerm: "well-known",
			wantMode:    ModeExact,
		},
		{
			name:        "prefix suffix keeps hyphen in RawTerm",
			input:       "2024-q*",
			wantTerm:    "2024 q",
			wantRawTerm: "2024-q",
			wantMode:    ModePrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSearchQuery(tt.input)
			if got.Term != tt.wantTerm {
				t.Errorf("ParseSearchQuery(%q).Term = %q, want %q", tt.input, got.Term, tt.wantTerm)
			}
			if got.RawTerm != tt.wantRawTerm {
				t.Errorf("ParseSearchQuery(%q).RawTerm = %q, want %q", tt.input, got.RawTerm, tt.wantRawTerm)
			}
			if got.Mode != tt.wantMode {
				t.Errorf("ParseSearchQuery(%q).Mode = %v, want %v", tt.input, got.Mode, tt.wantMode)
			}
		})
	}
}

func TestSanitizeSearchTermKeepHyphen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "preserves hyphen", input: "well-known", want: "well-known"},
		{name: "preserves multiple hyphens", input: "a-b-c", want: "a-b-c"},
		{name: "keeps digits and letters", input: "2024-3q", want: "2024-3q"},
		{name: "drops dangerous chars but keeps hyphen", input: "a'b<c>-d", want: "abc-d"},
		{name: "keeps underscore and dot", input: "a_b.c-d", want: "a_b.c-d"},
		{name: "trims surrounding space", input: "  x-y  ", want: "x-y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeSearchTermKeepHyphen(tt.input); got != tt.want {
				t.Errorf("sanitizeSearchTermKeepHyphen(%q) = %q, want %q", tt.input, got, tt.want)
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
			name:  "allowed characters preserved (hyphens become spaces)",
			input: "hello-world_123.test",
			want:  "hello world_123.test",
		},
		{
			name:  "unicode letters preserved",
			input: "café résumé",
			want:  "café résumé",
		},
		{
			name:  "SQL injection attempt sanitized",
			input: "'; DROP TABLE users;--",
			want:  "DROP TABLE users",
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
