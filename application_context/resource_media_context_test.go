package application_context

import (
	"testing"
)

func TestGetJPEGQuality(t *testing.T) {
	tests := []struct {
		name     string
		width    uint
		height   uint
		expected int
	}{
		// Test smallest dimension threshold (≤100)
		{"100x100", 100, 100, 70},
		{"50x50", 50, 50, 70},
		{"100x50", 100, 50, 70},
		{"50x100", 50, 100, 70},

		// Test 101-200 threshold
		{"150x150", 150, 150, 75},
		{"200x200", 200, 200, 75},
		{"101x50", 101, 50, 75},
		{"50x101", 50, 101, 75},

		// Test 201-400 threshold
		{"300x300", 300, 300, 80},
		{"400x400", 400, 400, 80},
		{"201x100", 201, 100, 80},
		{"100x400", 100, 400, 80},

		// Test >400 threshold
		{"500x500", 500, 500, 85},
		{"800x600", 800, 600, 85},
		{"401x100", 401, 100, 85},
		{"100x401", 100, 401, 85},

		// Edge cases
		{"0x0", 0, 0, 70},
		{"1x1", 1, 1, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJPEGQuality(tt.width, tt.height)
			if result != tt.expected {
				t.Errorf("getJPEGQuality(%d, %d) = %d, expected %d",
					tt.width, tt.height, result, tt.expected)
			}
		})
	}
}

func TestGetJPEGQuality_UsesMaxDimension(t *testing.T) {
	// Verify that the function uses the maximum of width/height
	// A 50x500 image should use quality for 500 (>400), not 50 (≤100)
	result := getJPEGQuality(50, 500)
	if result != 85 {
		t.Errorf("getJPEGQuality(50, 500) = %d, expected 85 (should use max dimension)", result)
	}

	result = getJPEGQuality(500, 50)
	if result != 85 {
		t.Errorf("getJPEGQuality(500, 50) = %d, expected 85 (should use max dimension)", result)
	}
}

func TestTruncateStderr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "short error",
			maxLen:   100,
			expected: "short error",
		},
		{
			name:     "exact length unchanged",
			input:    "exactly ten",
			maxLen:   11,
			expected: "exactly ten",
		},
		{
			name:     "long string truncated",
			input:    "this is a very long error message that should be truncated",
			maxLen:   20,
			expected: "this is a very long ... (truncated)",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "maxLen zero truncates everything",
			input:    "some text",
			maxLen:   0,
			expected: "... (truncated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateStderr(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateStderr(%q, %d) = %q, expected %q",
					tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
