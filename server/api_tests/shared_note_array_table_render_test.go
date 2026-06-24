package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// T10: a shared note containing a legacy array-format manual table (string
// columns + array rows) must render, not 500. Before normalization, pongo2
// errored on `col.label` against a string and aborted the whole page.
func TestSharedNote_ArrayFormatTable_RendersWithoutCrash(t *testing.T) {
	tc := SetupTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	note := tc.CreateDummyNote("array-table-note")
	token := shareNote(t, tc, note.ID)

	content, _ := json.Marshal(map[string]any{
		"columns": []string{"Name", "Value", "Status"},
		"rows":    [][]string{{"Alice", "100", "Active"}},
	})
	tb := &models.NoteBlock{NoteID: note.ID, Type: "table", Position: "a", Content: content, State: []byte("{}")}
	require.NoError(t, tc.DB.Create(tb).Error)

	req, _ := http.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code,
		"shared note with an array-format table must render (got %d): %s", rr.Code, rr.Body.String())
	body := rr.Body.String()
	assert.Contains(t, body, "Name", "column header should render")
	assert.Contains(t, body, "Alice", "row cell should render")
}
