package validation_test

import (
	"strings"
	"testing"

	"mahresources/application_context/validation"
)

func TestSanitizeEntityName_RejectsNullByte(t *testing.T) {
	_, err := validation.SanitizeEntityName("foo\x00bar")
	if err == nil {
		t.Fatal("expected error for NUL byte, got nil")
	}
	if !strings.Contains(err.Error(), "NUL") && !strings.Contains(err.Error(), "control") {
		t.Fatalf("expected error to mention NUL/control, got: %v", err)
	}
}

func TestSanitizeEntityName_RejectsDirectionalOverrides(t *testing.T) {
	// U+202A..U+202E (embedding + override) and U+2066..U+2069 (isolates)
	for _, ch := range []string{
		"‪", "‫", "‬", "‭", "‮",
		"⁦", "⁧", "⁨", "⁩",
	} {
		_, err := validation.SanitizeEntityName("foo" + ch + "bar")
		if err == nil {
			t.Fatalf("expected error for directional override U+%04X, got nil", []rune(ch)[0])
		}
	}
}

func TestSanitizeEntityName_RejectsEmbeddedNewlines(t *testing.T) {
	for _, raw := range []string{"foo\nbar", "foo\rbar", "foo\r\nbar"} {
		_, err := validation.SanitizeEntityName(raw)
		if err == nil {
			t.Fatalf("expected error for embedded newline in %q, got nil", raw)
		}
	}
}

func TestSanitizeEntityName_RejectsC0Controls(t *testing.T) {
	// A handful of representative C0 control characters other than TAB/CR/LF.
	for _, r := range []rune{'\x01', '\x07', '\x1B', '\x7F'} {
		_, err := validation.SanitizeEntityName("foo" + string(r) + "bar")
		if err == nil {
			t.Fatalf("expected error for C0 control U+%04X, got nil", r)
		}
	}
}

func TestSanitizeEntityName_AllowsTabAndNormalUnicode(t *testing.T) {
	cases := []string{"hello world", "café", "日本語", "name\twith\ttab", "emoji \U0001F600"}
	for _, raw := range cases {
		got, err := validation.SanitizeEntityName(raw)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", raw, err)
		}
		if got != raw {
			t.Fatalf("expected passthrough for %q, got %q", raw, got)
		}
	}
}

func TestSanitizeEntityName_TrimsSurroundingWhitespace(t *testing.T) {
	got, err := validation.SanitizeEntityName("  hello  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("expected trimmed 'hello', got %q", got)
	}
}

func TestSanitizeEntityName_RejectsEmptyAfterTrim(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\t"} {
		_, err := validation.SanitizeEntityName(raw)
		if err == nil {
			t.Fatalf("expected error for whitespace-only name %q, got nil", raw)
		}
	}
}
