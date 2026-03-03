package template_filters

import (
	"testing"
	"time"

	"github.com/flosch/pongo2/v4"
)

func TestTimeagoFilter(t *testing.T) {
	noParam := pongo2.AsValue("")

	tests := []struct {
		name     string
		offset   time.Duration
		expected string
	}{
		{"just now", 10 * time.Second, "just now"},
		{"1 minute ago", 1 * time.Minute, "1 minute ago"},
		{"45 minutes ago", 45 * time.Minute, "45 minutes ago"},
		{"1 hour ago", 1 * time.Hour, "1 hour ago"},
		{"5 hours ago", 5 * time.Hour, "5 hours ago"},
		{"1 day ago", 24 * time.Hour, "1 day ago"},
		{"15 days ago", 15 * 24 * time.Hour, "15 days ago"},
		{"falls back to date", 60 * 24 * time.Hour, time.Now().Add(-60 * 24 * time.Hour).Format("2006-01-02")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := pongo2.AsValue(time.Now().Add(-tt.offset))
			result, err := timeagoFilter(input, noParam)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.String() != tt.expected {
				t.Errorf("got %q, want %q", result.String(), tt.expected)
			}
		})
	}
}

func TestTimeagoFilter_NonTimeInput(t *testing.T) {
	result, err := timeagoFilter(pongo2.AsValue("not a time"), pongo2.AsValue(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.String() != "" {
		t.Errorf("expected empty string for non-time input, got %q", result.String())
	}
}
