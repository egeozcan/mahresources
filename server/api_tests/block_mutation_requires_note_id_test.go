package api_tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block mutation that omits noteId used to skip the ownership check, so the
// guard could be bypassed by leaving the parameter out. noteId is now required.
func TestBlockMutation_RequiresNoteId(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Owner")
	block := tc.CreateDummyBlock(note.ID, "text", `{"text": "keep"}`, "n")

	cases := []struct {
		name, method, path string
		body               any
	}{
		{"update content", http.MethodPut, "/v1/note/block", map[string]any{"content": map[string]string{"text": "hijacked"}}},
		{"update state", http.MethodPatch, "/v1/note/block/state", map[string]any{"state": map[string]any{}}},
		{"delete", http.MethodDelete, "/v1/note/block", nil},
		{"delete via POST", http.MethodPost, "/v1/note/block/delete", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := tc.MakeRequest(c.method, fmt.Sprintf("%s?id=%d", c.path, block.ID), c.body)
			assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
			assert.Contains(t, resp.Body.String(), "noteId is required")
		})
	}

	// None of the refused requests touched the block.
	got := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/block?id=%d", block.ID), nil)
	require.Equal(t, http.StatusOK, got.Code)
	assert.Contains(t, got.Body.String(), "keep")
}

// A mismatched noteId is refused on the state and POST-delete routes too, and
// leaves the block in place.
func TestBlockMutation_RejectsWrongNoteId(t *testing.T) {
	tc := SetupTestEnv(t)
	owner := tc.CreateDummyNote("Owner")
	other := tc.CreateDummyNote("Other")
	block := tc.CreateDummyBlock(owner.ID, "text", `{"text": "keep"}`, "n")

	q := fmt.Sprintf("id=%d&noteId=%d", block.ID, other.ID)
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, "/v1/note/block/state", map[string]any{"state": map[string]any{}}},
		{http.MethodPost, "/v1/note/block/delete", nil},
	} {
		resp := tc.MakeRequest(c.method, c.path+"?"+q, c.body)
		assert.Equal(t, http.StatusBadRequest, resp.Code, c.method+" "+c.path)
	}

	got := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/block?id=%d", block.ID), nil)
	assert.Equal(t, http.StatusOK, got.Code)
}
