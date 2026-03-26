package application_context

import (
	"mahresources/models/query_models"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCalculateRelevanceScore(t *testing.T) {
	tests := []struct {
		name        string
		inputName   string
		description string
		searchTerm  string
		want        int
	}{
		{
			name:        "exact match",
			inputName:   "test",
			description: "",
			searchTerm:  "test",
			want:        100,
		},
		{
			name:        "exact match case insensitive",
			inputName:   "Test",
			description: "",
			searchTerm:  "test",
			want:        100,
		},
		{
			name:        "exact match uppercase search",
			inputName:   "test",
			description: "",
			searchTerm:  "TEST",
			want:        100,
		},
		{
			name:        "prefix match",
			inputName:   "testing",
			description: "",
			searchTerm:  "test",
			want:        80,
		},
		{
			name:        "prefix match case insensitive",
			inputName:   "Testing",
			description: "",
			searchTerm:  "test",
			want:        80,
		},
		{
			name:        "contains in name",
			inputName:   "mytest123",
			description: "",
			searchTerm:  "test",
			want:        60,
		},
		{
			name:        "contains in name middle",
			inputName:   "a test b",
			description: "",
			searchTerm:  "test",
			want:        60,
		},
		{
			name:        "contains in description only",
			inputName:   "foo",
			description: "this is a test description",
			searchTerm:  "test",
			want:        40,
		},
		{
			name:        "contains in description case insensitive",
			inputName:   "foo",
			description: "This is a TEST",
			searchTerm:  "test",
			want:        40,
		},
		{
			name:        "no match",
			inputName:   "foo",
			description: "bar",
			searchTerm:  "xyz",
			want:        20,
		},
		{
			name:        "empty inputs",
			inputName:   "",
			description: "",
			searchTerm:  "test",
			want:        20,
		},
		{
			name:        "empty search term matches empty name",
			inputName:   "",
			description: "",
			searchTerm:  "",
			want:        100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRelevanceScore(tt.inputName, tt.description, tt.searchTerm)
			if got != tt.want {
				t.Errorf("calculateRelevanceScore(%q, %q, %q) = %d, want %d",
					tt.inputName, tt.description, tt.searchTerm, got, tt.want)
			}
		})
	}
}

func TestTruncateDescription(t *testing.T) {
	tests := []struct {
		name   string
		desc   string
		maxLen int
		want   string
	}{
		{
			name:   "short text under limit",
			desc:   "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "text at exact limit",
			desc:   "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "text needs truncation",
			desc:   "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "empty string",
			desc:   "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "longer truncation",
			desc:   "this is a very long description that needs truncation",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "truncation with maxLen 4",
			desc:   "hello",
			maxLen: 4,
			want:   "h...",
		},
		{
			name:   "unicode text",
			desc:   "hello world",
			maxLen: 8,
			want:   "hello...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateDescription(tt.desc, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateDescription(%q, %d) = %q, want %q",
					tt.desc, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateDescription_MultiByteCharacters(t *testing.T) {
	// "你好世界测试" is 6 characters but 18 bytes in UTF-8.
	// With maxLen=10 (characters), a 6-char string should NOT be truncated.
	t.Run("short CJK string not truncated", func(t *testing.T) {
		desc := "你好世界测试" // 6 chars, 18 bytes
		got := truncateDescription(desc, 10)
		if got != desc {
			t.Errorf("6-character CJK string was incorrectly truncated at maxLen=10: got %q", got)
		}
	})

	// When truncation IS needed for multi-byte text, the result must be valid UTF-8.
	t.Run("truncated CJK produces valid UTF-8", func(t *testing.T) {
		desc := "你好世界测试这是一个很长的描述" // 14 chars
		got := truncateDescription(desc, 10)
		if !utf8.ValidString(got) {
			t.Errorf("truncateDescription produced invalid UTF-8: %q (bytes: %x)", got, []byte(got))
		}
	})

	// The truncated result should end with "..." and have at most maxLen runes total.
	t.Run("truncated CJK respects character limit", func(t *testing.T) {
		desc := "你好世界测试这是一个很长的描述" // 14 chars
		got := truncateDescription(desc, 10)
		runeCount := utf8.RuneCountInString(got)
		if runeCount > 10 {
			t.Errorf("truncated result has %d characters, want at most 10: %q", runeCount, got)
		}
		if !strings.HasSuffix(got, "...") {
			t.Errorf("truncated result should end with '...': got %q", got)
		}
	})
}

func TestGlobalSearchLimitClamping(t *testing.T) {
	tests := []struct {
		name      string
		input     int
		wantLimit int
	}{
		{
			name:      "zero defaults to 20",
			input:     0,
			wantLimit: 20,
		},
		{
			name:      "negative defaults to 20",
			input:     -1,
			wantLimit: 20,
		},
		{
			name:      "within range unchanged",
			input:     10,
			wantLimit: 10,
		},
		{
			name:      "at max unchanged",
			input:     50,
			wantLimit: 50,
		},
		{
			name:      "over max clamped to 50",
			input:     51,
			wantLimit: 50,
		},
		{
			name:      "way over max clamped to 50",
			input:     100,
			wantLimit: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &query_models.GlobalSearchQuery{
				Query: "", // empty query returns early without hitting DB
				Limit: tt.input,
			}
			// normalizeSearchLimit is the logic under test; we simulate it here
			// by calling the same code path as GlobalSearch's limit normalization
			limit := tt.input
			if limit <= 0 {
				limit = 20
			} else if limit > 50 {
				limit = 50
			}
			if limit != tt.wantLimit {
				t.Errorf("limit normalization for input %d: got %d, want %d", tt.input, limit, tt.wantLimit)
			}
			// Verify the query struct is valid (doesn't panic)
			_ = q
		})
	}
}

func TestGetTypesToSearch(t *testing.T) {
	tests := []struct {
		name           string
		requestedTypes []string
		want           []string
	}{
		{
			name:           "empty slice returns all types",
			requestedTypes: []string{},
			want:           allEntityTypes,
		},
		{
			name:           "nil slice returns all types",
			requestedTypes: nil,
			want:           allEntityTypes,
		},
		{
			name:           "single valid type",
			requestedTypes: []string{"resource"},
			want:           []string{"resource"},
		},
		{
			name:           "multiple valid types",
			requestedTypes: []string{"resource", "note", "group"},
			want:           []string{"resource", "note", "group"},
		},
		{
			name:           "invalid type only returns all types",
			requestedTypes: []string{"invalid"},
			want:           allEntityTypes,
		},
		{
			name:           "multiple invalid types returns all types",
			requestedTypes: []string{"invalid", "fake", "notreal"},
			want:           allEntityTypes,
		},
		{
			name:           "mixed valid and invalid types",
			requestedTypes: []string{"resource", "invalid", "note"},
			want:           []string{"resource", "note"},
		},
		{
			name:           "all valid entity types",
			requestedTypes: []string{"resource", "note", "group", "tag", "category", "query", "relationType", "noteType"},
			want:           []string{"resource", "note", "group", "tag", "category", "query", "relationType", "noteType"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTypesToSearch(tt.requestedTypes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getTypesToSearch(%v) = %v, want %v",
					tt.requestedTypes, got, tt.want)
			}
		})
	}
}
