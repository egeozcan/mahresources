package database_scopes

import "testing"

func TestConvertMetaSortForSQLite(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "meta with double arrow no direction",
			input:    "meta->>'key_name'",
			expected: "json_extract(meta, '$.key_name')",
		},
		{
			name:     "meta with double arrow desc",
			input:    "meta->>'key_name' desc",
			expected: "json_extract(meta, '$.key_name') desc",
		},
		{
			name:     "meta with double arrow asc",
			input:    "meta->>'key_name' asc",
			expected: "json_extract(meta, '$.key_name') asc",
		},
		{
			name:     "meta with single arrow",
			input:    "meta->'key_name'",
			expected: "json_extract(meta, '$.key_name')",
		},
		{
			name:     "meta with single arrow desc",
			input:    "meta->'key_name' desc",
			expected: "json_extract(meta, '$.key_name') desc",
		},
		{
			name:     "non-meta column unchanged",
			input:    "created_at desc",
			expected: "created_at desc",
		},
		{
			name:     "simple column unchanged",
			input:    "name",
			expected: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMetaSortForSQLite(tt.input)
			if result != tt.expected {
				t.Errorf("convertMetaSortForSQLite(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateSortColumn(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"name", true},
		{"name desc", true},
		{"name asc", true},
		{"created_at", true},
		{"created_at desc", true},
		{"meta->>'key_name'", true},
		{"meta->>'key_name' desc", true},
		{"meta->'key_name'", true},
		{"meta->'key_name' asc", true},
		{"", false},
		{"invalid-column", false},
		{"name; DROP TABLE users", false},
		{"meta->>'KEY'", false}, // uppercase not allowed
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ValidateSortColumn(tt.input)
			if result != tt.valid {
				t.Errorf("ValidateSortColumn(%q) = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}
