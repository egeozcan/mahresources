package template_filters

import (
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
)

func TestHumanReadableSizeNegativeValue(t *testing.T) {
	// SizeDelta can be negative when a newer version is smaller.
	// The filter must not wrap to a huge unsigned number like "16.0 EB".
	result, err := humanReadableSize(pongo2.AsValue(int64(-500)), pongo2.AsValue(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.String()
	if strings.Contains(got, "EB") || strings.Contains(got, "PB") || strings.Contains(got, "TB") {
		t.Errorf("humanReadableSize(-500) = %q — negative value wrapped to huge unsigned number", got)
	}
	if !strings.Contains(got, "500") {
		t.Errorf("humanReadableSize(-500) = %q — should contain '500'", got)
	}
	if !strings.HasPrefix(got, "-") {
		t.Errorf("humanReadableSize(-500) = %q — should start with '-' for negative values", got)
	}
}

func TestHumanReadableSizePositiveValue(t *testing.T) {
	result, _ := humanReadableSize(pongo2.AsValue(int64(1024)), pongo2.AsValue(""))
	got := result.String()
	if strings.Contains(got, "EB") {
		t.Errorf("humanReadableSize(1024) = %q — unexpected", got)
	}
}

func TestHumanReadableSizeZero(t *testing.T) {
	result, _ := humanReadableSize(pongo2.AsValue(int64(0)), pongo2.AsValue(""))
	got := result.String()
	if strings.Contains(got, "EB") {
		t.Errorf("humanReadableSize(0) = %q — unexpected", got)
	}
}
