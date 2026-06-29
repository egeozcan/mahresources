package api_handlers

import "testing"

// TestParseUintStrict covers the strict base-10 uint parser used by the
// bulk-unshare handler. The original hand-rolled loop wrapped silently on
// overflow (e.g. 2^64 parsed to 0/1), which could target the wrong note ID.
func TestParseUintStrict(t *testing.T) {
	ok := []struct {
		in   string
		want uint
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"4294967295", 4294967295},
	}
	for _, c := range ok {
		got, err := parseUintStrict(c.in)
		if err != nil {
			t.Errorf("parseUintStrict(%q) unexpected err: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseUintStrict(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	bad := []string{
		"",                      // empty
		" 1",                    // leading space
		"1 ",                    // trailing space
		"+1",                    // sign
		"-1",                    // sign
		"1.0",                   // non-digit
		"abc",                   // non-digit
		"18446744073709551616",  // 2^64 — must error, not wrap to 0
		"18446744073709551617",  // 2^64+1 — must error, not wrap to 1
		"99999999999999999999",  // far past 2^64
	}
	for _, s := range bad {
		if got, err := parseUintStrict(s); err == nil {
			t.Errorf("parseUintStrict(%q) = %d, want error", s, got)
		}
	}
}
