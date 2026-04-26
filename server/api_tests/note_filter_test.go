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

// createNoteType inserts a NoteType with the given name directly into the test
// database and returns the created record (with its auto-assigned ID).
func createNoteType(t *testing.T, tc *TestContext, name string) *models.NoteType {
	t.Helper()
	nt := &models.NoteType{Name: name}
	require.NoError(t, tc.DB.Create(nt).Error)
	return nt
}

// createNoteWithType inserts a Note with the given name and the supplied
// NoteTypeId directly into the test database.
func createNoteWithType(t *testing.T, tc *TestContext, name string, noteTypeId uint) *models.Note {
	t.Helper()
	n := &models.Note{Name: name, NoteTypeId: &noteTypeId}
	require.NoError(t, tc.DB.Create(n).Error)
	return n
}

// TestNoteList_FilterByNoteTypeIds verifies that repeated NoteTypeIds query
// params are bound by gorilla/schema and passed through to the database IN-list
// filter, returning only notes whose note_type_id is in the requested set.
func TestNoteList_FilterByNoteTypeIds(t *testing.T) {
	tc := SetupTestEnv(t)

	nt1 := createNoteType(t, tc, "Type 1")
	nt2 := createNoteType(t, tc, "Type 2")
	nt3 := createNoteType(t, tc, "Type 3")
	createNoteWithType(t, tc, "n1", nt1.ID)
	createNoteWithType(t, tc, "n2", nt2.ID)
	createNoteWithType(t, tc, "n3", nt3.ID)

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
