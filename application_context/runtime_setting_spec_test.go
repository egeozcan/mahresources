package application_context

import (
	"testing"
	"time"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   any
		typ  string
	}{
		{"int64", int64(1 << 31), "int64"},
		{"int", int(500), "int"},
		{"uint64", uint64(42), "uint64"},
		{"duration", 2 * time.Hour, "duration"},
		{"string_empty", "", "string"},
		{"string_url", "https://example.com", "string"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := encodeSettingValue(tc.typ, tc.in)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := decodeSettingValue(tc.typ, enc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got != tc.in {
				t.Fatalf("round-trip: got %v want %v", got, tc.in)
			}
		})
	}
}

func TestEnvelopeTypeMismatch(t *testing.T) {
	enc, _ := encodeSettingValue("int64", int64(1))
	if _, err := decodeSettingValue("string", enc); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestEnvelopeDurationEncodedAsNanos(t *testing.T) {
	enc, err := encodeSettingValue("duration", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Envelope payload should be the nanosecond count, i.e. 500_000_000.
	wantSubstr := `"value":500000000`
	if !contains(string(enc), wantSubstr) {
		t.Fatalf("duration envelope %q should contain %q", string(enc), wantSubstr)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
