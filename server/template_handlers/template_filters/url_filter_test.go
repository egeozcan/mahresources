package template_filters

import "testing"

func TestIsSafeURL(t *testing.T) {
	tests := []struct {
		input string
		safe  bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"ftp://files.example.com/doc.pdf", true},
		{"/relative/path", true},
		{"", true},
		{"javascript:alert(1)", false},
		{"JavaScript:alert(1)", false},
		{"JAVASCRIPT:alert(1)", false},
		{"  javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"vbscript:msgbox", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isSafeURL(tt.input); got != tt.safe {
				t.Errorf("isSafeURL(%q) = %v, want %v", tt.input, got, tt.safe)
			}
		})
	}
}
