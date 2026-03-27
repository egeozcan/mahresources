package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bug 1: ReorderBlocksHandler returns 500 for validation errors instead of 400.
func TestReorderBlocks_ValidationError_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Reorder Status Code Test")

	// Create two blocks
	respA := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId": note.ID, "type": "text", "position": "a",
		"content": map[string]string{"text": "A"},
	})
	require.Equal(t, http.StatusCreated, respA.Code)
	var blockA models.NoteBlock
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &blockA))

	respB := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId": note.ID, "type": "text", "position": "b",
		"content": map[string]string{"text": "B"},
	})
	require.Equal(t, http.StatusCreated, respB.Code)
	var blockB models.NoteBlock
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &blockB))

	t.Run("duplicate positions should return 400", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", map[string]any{
			"noteId": note.ID,
			"positions": map[string]string{
				fmt.Sprintf("%d", blockA.ID): "m",
				fmt.Sprintf("%d", blockB.ID): "m",
			},
		})
		assert.Equal(t, http.StatusBadRequest, resp.Code,
			"duplicate position values should return 400, not 500")
	})

	t.Run("block IDs from wrong note should return 400", func(t *testing.T) {
		otherNote := tc.CreateDummyNote("Other Note")
		otherBlock := tc.CreateDummyBlock(otherNote.ID, "text", `{"text":"x"}`, "a")

		resp := tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", map[string]any{
			"noteId": note.ID,
			"positions": map[string]string{
				fmt.Sprintf("%d", otherBlock.ID): "z",
			},
		})
		assert.Equal(t, http.StatusBadRequest, resp.Code,
			"block IDs not belonging to note should return 400, not 500")
	})
}

// Bug 2: RemoveResourceFromSeries returns 500 for "resource is not in a series".
func TestRemoveResourceFromSeries_NotInSeries_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource that is NOT in any series
	res := &models.Resource{Name: "Standalone Resource", Meta: []byte(`{}`)}
	tc.DB.Create(res)
	require.NotZero(t, res.ID)

	resp := tc.MakeRequest(http.MethodPost,
		fmt.Sprintf("/v1/resource/removeSeries?id=%d", res.ID), nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"removing a resource not in a series should return 400, not 500")
}

// Bug 4: AddRelation with both IDs = 0 gives "cannot relate to self" instead of "required IDs".
func TestAddRelation_BothIDsZero_ReturnsMissingIDsError(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/relation", map[string]any{
		"FromGroupId":         0,
		"ToGroupId":           0,
		"GroupRelationTypeId": 1,
	})

	// Should get a 400 error about missing required IDs, not "cannot relate to self"
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	json.Unmarshal(resp.Body.Bytes(), &body)

	// The error message should mention missing/required IDs, not "self"
	if errMsg, ok := body["error"].(string); ok {
		assert.NotContains(t, errMsg, "cannot relate to self",
			"error for both IDs=0 should not be 'cannot relate to self'")
		assert.Contains(t, errMsg, "required",
			"error for both IDs=0 should mention that IDs are required")
	}
}
