package commands

import (
	"strings"
	"testing"
)

func TestFormatFileSize_NegativeValues(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		wantNeg  bool   // should start with "-"
		wantUnit string // expected unit suffix
	}{
		{"negative small", -500, true, "B"},
		{"negative KB", -2048, true, "KB"},
		{"negative MB", -5242880, true, "MB"},
		{"negative GB", -1073741824, true, "GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFileSize(tt.bytes)
			if tt.wantNeg && !strings.HasPrefix(got, "-") {
				t.Errorf("formatFileSize(%d) = %q, want negative prefix", tt.bytes, got)
			}
			if !strings.HasSuffix(got, tt.wantUnit) {
				t.Errorf("formatFileSize(%d) = %q, want unit %q (large negative values should not collapse to bytes)",
					tt.bytes, got, tt.wantUnit)
			}
		})
	}
}

func TestFormatFileSize_PositiveValues(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		wantUnit string
	}{
		{"zero", 0, "B"},
		{"small", 500, "B"},
		{"KB", 2048, "KB"},
		{"MB", 5242880, "MB"},
		{"GB", 1073741824, "GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFileSize(tt.bytes)
			if !strings.HasSuffix(got, tt.wantUnit) {
				t.Errorf("formatFileSize(%d) = %q, want unit %q", tt.bytes, got, tt.wantUnit)
			}
		})
	}
}
