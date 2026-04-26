//go:build json1 && fts5

package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// TestNoteList_FilterByNoteTypeIds verifies that repeated NoteTypeIds query
// params are bound by gorilla/schema and passed through to the database IN-list
// filter, returning only notes whose note_type_id is in the requested set.
func TestNoteList_FilterByNoteTypeIds(t *testing.T) {
	tc := SetupTestEnv(t)

	nt1 := tc.CreateNoteType(t, "Type 1")
	nt2 := tc.CreateNoteType(t, "Type 2")
	nt3 := tc.CreateNoteType(t, "Type 3")
	tc.CreateNoteWithType(t, "n1", nt1.ID)
	tc.CreateNoteWithType(t, "n2", nt2.ID)
	tc.CreateNoteWithType(t, "n3", nt3.ID)

	url := fmt.Sprintf("/v1/notes?NoteTypeIds=%d&NoteTypeIds=%d", nt1.ID, nt2.ID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Note
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	if len(got) != 2 {
		t.Fatalf("expected 2 notes (nt1 + nt2), got %d", len(got))
	}

	// Per-note identity assertion: each returned note must carry one of the
	// requested NoteTypeIds (mirrors the per-resource check in resource_filter_test.go).
	for _, n := range got {
		if n.NoteTypeId == nil {
			t.Errorf("note %d has nil NoteTypeId; expected %d or %d", n.ID, nt1.ID, nt2.ID)
			continue
		}
		if *n.NoteTypeId != nt1.ID && *n.NoteTypeId != nt2.ID {
			t.Errorf("note %d has unexpected NoteTypeId %d (expected %d or %d)",
				n.ID, *n.NoteTypeId, nt1.ID, nt2.ID)
		}
	}
}
