package deferredtoken

import (
	"strings"
	"testing"
)

var testKey = []byte("test-signing-key-0123456789abcdef")

func TestSealOpenRoundTrip(t *testing.T) {
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
			token := Seal(testKey, tc.entityType, tc.entityID, tc.body)
			if token == "" {
				t.Fatal("Seal returned empty token")
			}
			gotType, gotID, gotBody, ok := Open(testKey, token)
			if !ok {
				t.Fatal("Open returned ok=false for a freshly sealed token")
			}
			if gotType != tc.entityType || gotID != tc.entityID || gotBody != tc.body {
				t.Fatalf("round-trip mismatch: got (%q,%d,%q) want (%q,%d,%q)",
					gotType, gotID, gotBody, tc.entityType, tc.entityID, tc.body)
			}
		})
	}
}

// TestTokenIsOpaque is the point of using authenticated encryption over a bare
// signature: the raw template body must not be recoverable from the token that is
// emitted into the page.
func TestTokenIsOpaque(t *testing.T) {
	secretBody := `[conditional field="secret" eq="42"][mrql query='type = "resource"'][/conditional]`
	token := Seal(testKey, "group", 1, secretBody)
	if strings.Contains(token, "conditional") || strings.Contains(token, "mrql") || strings.Contains(token, "secret") {
		t.Fatalf("token leaks template source verbatim: %q", token)
	}
	// Even after base64-decoding the token, the plaintext must not be present.
	decoded, err := b64.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not valid base64url: %v", err)
	}
	if strings.Contains(string(decoded), "conditional") || strings.Contains(string(decoded), secretBody) {
		t.Fatalf("decoded token exposes the template body")
	}
}

func TestOpenWrongKeyFails(t *testing.T) {
	token := Seal(testKey, "group", 1, "body")
	if _, _, _, ok := Open([]byte("a-different-key"), token); ok {
		t.Fatal("Open accepted a token sealed with a different key")
	}
}

func TestOpenTamperedTokenFails(t *testing.T) {
	token := Seal(testKey, "group", 1, `[mrql query='type = "resource"']`)

	// Flip a byte anywhere in the token: AES-GCM authentication must reject it.
	for _, at := range []int{0, len(token) / 2, len(token) - 1} {
		b := []byte(token)
		if b[at] == 'A' {
			b[at] = 'B'
		} else {
			b[at] = 'A'
		}
		if _, _, _, ok := Open(testKey, string(b)); ok {
			t.Fatalf("Open accepted a token tampered at index %d", at)
		}
	}
}

func TestOpenMalformedTokenFails(t *testing.T) {
	for _, bad := range []string{
		"",
		".",
		"short",
		"not!base64!",
	} {
		if _, _, _, ok := Open(testKey, bad); ok {
			t.Fatalf("Open accepted malformed token %q", bad)
		}
	}
}
