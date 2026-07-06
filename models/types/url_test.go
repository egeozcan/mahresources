package types

import "testing"

func TestURLString(t *testing.T) {
	var u URL
	if err := u.Scan("https://example.com/profile?tab=social#links"); err != nil {
		t.Fatalf("scan url: %v", err)
	}

	if got := u.String(); got != "https://example.com/profile?tab=social#links" {
		t.Fatalf("String() = %q", got)
	}
}
