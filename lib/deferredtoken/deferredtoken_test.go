package deferredtoken

import "testing"

var testKey = []byte("test-signing-key-0123456789abcdef")

func TestSignVerifyRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		entityType string
		entityID   uint
		body       string
	}{
		{"simple", "group", 42, `<div>[property path="Name"]</div>`},
		{"empty body", "note", 1, ""},
		{"unicode + quotes", "resource", 9999, `héllo "world" [mrql query='type = "note"']`},
		{"multiline", "group", 7, "line1\nline2\n  [meta path=\"x\"]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := Sign(testKey, tc.entityType, tc.entityID, tc.body)
			if token == "" {
				t.Fatal("Sign returned empty token")
			}
			gotType, gotID, gotBody, ok := Verify(testKey, token)
			if !ok {
				t.Fatal("Verify returned ok=false for a freshly signed token")
			}
			if gotType != tc.entityType || gotID != tc.entityID || gotBody != tc.body {
				t.Fatalf("round-trip mismatch: got (%q,%d,%q) want (%q,%d,%q)",
					gotType, gotID, gotBody, tc.entityType, tc.entityID, tc.body)
			}
		})
	}
}

func TestVerifyWrongKeyFails(t *testing.T) {
	token := Sign(testKey, "group", 1, "body")
	if _, _, _, ok := Verify([]byte("a-different-key-that-is-long-enough!!"), token); ok {
		t.Fatal("Verify accepted a token signed with a different key")
	}
}

func TestVerifyTamperedTokenFails(t *testing.T) {
	token := Sign(testKey, "group", 1, `[mrql query='type = "resource"']`)

	// Flip the last character of the payload segment (before the '.').
	dot := len(token) - 1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	tamperAt := dot - 1
	tampered := []byte(token)
	if tampered[tamperAt] == 'A' {
		tampered[tamperAt] = 'B'
	} else {
		tampered[tamperAt] = 'A'
	}
	if _, _, _, ok := Verify(testKey, string(tampered)); ok {
		t.Fatal("Verify accepted a token with a tampered payload")
	}

	// Tamper the signature segment.
	tampered = []byte(token)
	last := len(tampered) - 1
	if tampered[last] == 'A' {
		tampered[last] = 'B'
	} else {
		tampered[last] = 'A'
	}
	if _, _, _, ok := Verify(testKey, string(tampered)); ok {
		t.Fatal("Verify accepted a token with a tampered signature")
	}
}

func TestVerifyMalformedTokenFails(t *testing.T) {
	for _, bad := range []string{
		"",
		".",
		"nodot",
		"onlypayload.",
		".onlysig",
		"not!base64.also!bad",
	} {
		if _, _, _, ok := Verify(testKey, bad); ok {
			t.Fatalf("Verify accepted malformed token %q", bad)
		}
	}
}
