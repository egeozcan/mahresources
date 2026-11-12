package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// The share token is a bearer credential for the share server, so no generic
// JSON encoding of a Note may carry it: every surface that serializes a note,
// top-level or nested in a group, resource, tag or MRQL result, inherits this
// (#107). The one writer-only disclosure, GET /v1/note, adds it back itself.
func TestNoteJSONNeverCarriesTheShareToken(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	out, err := json.Marshal(Note{Name: "n", ShareToken: &token, HasShare: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), token) || strings.Contains(string(out), `"shareToken"`) {
		t.Fatalf("note JSON carries the share token: %s", out)
	}
	if !strings.Contains(string(out), `"hasShare":true`) {
		t.Fatalf("note JSON lacks hasShare: %s", out)
	}
}

// AfterFind derives the non-secret hasShare flag on every load, so read-only
// callers learn a note is shared without being handed the token.
func TestNoteAfterFindSetsHasShare(t *testing.T) {
	token, empty := "0123456789abcdef0123456789abcdef", ""
	for _, c := range []struct {
		name  string
		token *string
		want  bool
	}{
		{"shared", &token, true},
		{"nil token", nil, false},
		{"empty token", &empty, false},
	} {
		n := Note{ShareToken: c.token}
		if err := n.AfterFind(nil); err != nil {
			t.Fatalf("%s: AfterFind: %v", c.name, err)
		}
		if n.HasShare != c.want {
			t.Errorf("%s: HasShare = %v, want %v", c.name, n.HasShare, c.want)
		}
	}
}
