package database_scopes

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
			result := convertMetaSortForSQLite(tt.input, "")
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

func TestValidateDateString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"date only", "2024-01-15", true},
		{"datetime with Z", "2024-01-15T10:30:00Z", true},
		{"datetime with offset", "2024-01-15T10:30:00+05:00", true},
		{"datetime with negative offset", "2024-01-15T10:30:00-08:00", true},
		{"datetime with seconds and Z", "2024-01-15T10:30:45Z", true},
		{"not-a-date", "not-a-date", false},
		{"yesterday", "yesterday", false},
		{"random text", "hello world", false},
		{"partial date", "2024-01", false},
		{"empty string", "", false},
		{"unix timestamp", "1705312200", false},
		{"date with slashes", "01/15/2024", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateDateString(tt.input)
			if result != tt.valid {
				t.Errorf("ValidateDateString(%q) = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}

func TestApplyDateRange_RejectsInvalidDates(t *testing.T) {
	db := openTestDB(t)

	// Invalid "before" date should cause an error
	result := ApplyDateRange(db, "", "not-a-date", "")
	if result.Error == nil {
		t.Error("ApplyDateRange should return error for invalid 'before' date 'not-a-date'")
	}

	// Invalid "after" date should cause an error
	db2 := openTestDB(t)
	result = ApplyDateRange(db2, "", "", "yesterday")
	if result.Error == nil {
		t.Error("ApplyDateRange should return error for invalid 'after' date 'yesterday'")
	}

	// Valid dates should not cause an error
	db3 := openTestDB(t)
	result = ApplyDateRange(db3, "", "2024-01-15", "2024-01-01")
	if result.Error != nil {
		t.Errorf("ApplyDateRange should not return error for valid dates, got: %v", result.Error)
	}

	// Empty dates should not cause an error (they are simply ignored)
	db4 := openTestDB(t)
	result = ApplyDateRange(db4, "", "", "")
	if result.Error != nil {
		t.Errorf("ApplyDateRange should not return error for empty dates, got: %v", result.Error)
	}
}

func TestApplyUpdatedDateRange_RejectsInvalidDates(t *testing.T) {
	db := openTestDB(t)

	// Invalid "before" date should cause an error
	result := ApplyUpdatedDateRange(db, "", "not-a-date", "")
	if result.Error == nil {
		t.Error("ApplyUpdatedDateRange should return error for invalid 'before' date")
	}

	// Invalid "after" date should cause an error
	db2 := openTestDB(t)
	result = ApplyUpdatedDateRange(db2, "", "", "yesterday")
	if result.Error == nil {
		t.Error("ApplyUpdatedDateRange should return error for invalid 'after' date")
	}

	// Valid dates should not cause an error
	db3 := openTestDB(t)
	result = ApplyUpdatedDateRange(db3, "", "2024-01-15T10:30:00Z", "2024-01-01")
	if result.Error != nil {
		t.Errorf("ApplyUpdatedDateRange should not return error for valid dates, got: %v", result.Error)
	}
}
