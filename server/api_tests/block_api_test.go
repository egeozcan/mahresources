// server/api_tests/block_api_test.go
package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note for testing
	note := tc.CreateDummyNote("Block Test Note")

	var createdBlockID uint

	t.Run("Create Block", func(t *testing.T) {
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "n",
			"content":  map[string]string{"text": "Hello World"},
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		assert.Equal(t, http.StatusCreated, resp.Code)

		var block models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &block)
		assert.NoError(t, err)
		assert.Equal(t, "text", block.Type)
		assert.Equal(t, "n", block.Position)
		createdBlockID = block.ID
	})

	t.Run("Get Blocks for Note", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var blocks []models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &blocks)
		assert.NoError(t, err)
		assert.Len(t, blocks, 1)
	})

	t.Run("Get Single Block", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var block models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &block)
		assert.NoError(t, err)
		assert.Equal(t, createdBlockID, block.ID)
	})

	t.Run("Update Block Content", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		payload := map[string]interface{}{
			"content": map[string]string{"text": "Updated content"},
		}

		resp := tc.MakeRequest(http.MethodPut, url, payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var block models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &block)
		assert.Contains(t, string(block.Content), "Updated content")
	})

	t.Run("Update Block State", func(t *testing.T) {
		// Create a todos block for state testing
		todosPayload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "todos",
			"position": "o",
			"content":  map[string]interface{}{"items": []map[string]string{{"id": "x1", "label": "Task 1"}}},
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", todosPayload)
		assert.Equal(t, http.StatusCreated, resp.Code)

		var todosBlock models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &todosBlock)

		// Update state
		stateURL := fmt.Sprintf("/v1/note/block/state?id=%d", todosBlock.ID)
		statePayload := map[string]interface{}{
			"state": map[string][]string{"checked": {"x1"}},
		}

		resp = tc.MakeRequest(http.MethodPatch, stateURL, statePayload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var updated models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &updated)
		assert.Contains(t, string(updated.State), "x1")
	})

	t.Run("Reorder Blocks", func(t *testing.T) {
		// Create another block
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "a",
			"content":  map[string]string{"text": "First block"},
		}
		tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)

		// Get all blocks
		url := fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		var blocks []models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &blocks)

		// Reorder - swap positions
		reorderPayload := map[string]interface{}{
			"noteId": note.ID,
			"positions": map[uint]string{
				blocks[0].ID: "z",
				blocks[1].ID: "a",
			},
		}

		resp = tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", reorderPayload)
		assert.Equal(t, http.StatusNoContent, resp.Code)
	})

	t.Run("Delete Block", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		resp := tc.MakeRequest(http.MethodDelete, url, nil)
		assert.Equal(t, http.StatusNoContent, resp.Code)

		// Verify deletion
		var check models.NoteBlock
		result := tc.DB.First(&check, createdBlockID)
		assert.Error(t, result.Error)
	})

	t.Run("Delete Block via POST", func(t *testing.T) {
		// Create a block to delete
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "p",
			"content":  map[string]string{"text": "To be deleted"},
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		var block models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &block)

		url := fmt.Sprintf("/v1/note/block/delete?id=%d", block.ID)
		resp = tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusNoContent, resp.Code)
	})

	t.Run("Create Block Invalid Type", func(t *testing.T) {
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "invalid_type",
			"position": "q",
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Get Blocks Missing NoteId", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/blocks", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}
