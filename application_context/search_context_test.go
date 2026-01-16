package application_context

import (
	"reflect"
	"testing"
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
