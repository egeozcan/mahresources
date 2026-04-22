package application_context

import (
	"strings"
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
	if !strings.Contains(string(enc), wantSubstr) {
		t.Fatalf("duration envelope %q should contain %q", string(enc), wantSubstr)
	}
}

func TestBuildSpecs_ElevenKeys(t *testing.T) {
	specs := buildSpecs()
	if len(specs) != 11 {
		t.Fatalf("want 11 specs, got %d", len(specs))
	}
	expected := []string{
		KeyMaxUploadSize, KeyMaxImportSize, KeyMRQLDefaultLimit, KeyMRQLQueryTimeout,
		KeyExportRetention, KeyRemoteConnectTimeout, KeyRemoteIdleTimeout, KeyRemoteOverallTimeout,
		KeySharePublicURL, KeyHashSimilarityThreshold, KeyHashAHashThreshold,
	}
	for _, k := range expected {
		if _, ok := specs[k]; !ok {
			t.Errorf("missing spec for key %q", k)
		}
	}
}

func TestValidateSharePublicURL(t *testing.T) {
	ok := []string{"", "https://example.com", "http://example.com:8080/base"}
	bad := []string{"/relative", "no-scheme.example.com", "ftp://example.com", "http://", "https:///nohost"}
	for _, s := range ok {
		if err := validateSharePublicURL(s); err != nil {
			t.Errorf("want accept %q, got %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validateSharePublicURL(s); err == nil {
			t.Errorf("want reject %q, got nil error", s)
		}
	}
}
