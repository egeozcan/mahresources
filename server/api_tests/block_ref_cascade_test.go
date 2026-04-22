package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// CreateDummyResource creates a resource directly in the DB for testing.
func (tc *TestContext) CreateDummyResource(t *testing.T, name string) *models.Resource {
	t.Helper()
	r := &models.Resource{Name: name, ContentType: "application/octet-stream", FileSize: 1}
	require.NoError(t, tc.DB.Create(r).Error)
	return r
}

// deleteResource deletes a resource via POST /v1/resource/delete.
func (tc *TestContext) deleteResource(t *testing.T, id uint) {
	t.Helper()
	form := url.Values{}
	form.Set("ID", fmt.Sprintf("%d", id))
	rr := tc.MakeFormRequest(http.MethodPost, "/v1/resource/delete", form)
	require.Equal(t, http.StatusOK, rr.Code, "resource delete failed: %s", rr.Body.String())
}

// deleteGroup deletes a group via POST /v1/group/delete.
func (tc *TestContext) deleteGroup(t *testing.T, id uint) {
	t.Helper()
	form := url.Values{}
	form.Set("ID", fmt.Sprintf("%d", id))
	rr := tc.MakeFormRequest(http.MethodPost, "/v1/group/delete", form)
	require.Equal(t, http.StatusOK, rr.Code, "group delete failed: %s", rr.Body.String())
}

// createBlock creates a note block via POST /v1/note/block with a JSON body.
// Content must be a valid JSON string for the block type.
func (tc *TestContext) createBlock(t *testing.T, noteID uint, blockType, content string) *models.NoteBlock {
	t.Helper()
	body := map[string]any{
		"noteId":  noteID,
		"type":    blockType,
		"content": json.RawMessage(content),
	}
	rr := tc.MakeRequest(http.MethodPost, "/v1/note/block", body)
	require.Equal(t, http.StatusCreated, rr.Code, "%s block create failed: %s", blockType, rr.Body.String())

	var block models.NoteBlock
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &block), "unmarshal block response: %s", rr.Body.String())
	require.NotZero(t, block.ID, "block ID must be non-zero")
	return &block
}

// TestResourceDelete_ScrubsGalleryBlockReferences verifies BH-020:
// deleting a resource removes its ID from gallery block resourceIds arrays.
func TestResourceDelete_ScrubsGalleryBlockReferences(t *testing.T) {
	tc := SetupTestEnv(t)

	res := tc.CreateDummyResource(t, "bh020-res")
	note := tc.CreateDummyNote("bh020-note")

	blockContent := fmt.Sprintf(`{"resourceIds":[%d]}`, res.ID)
	tc.createBlock(t, note.ID, "gallery", blockContent)

	// Delete the resource via the API
	tc.deleteResource(t, res.ID)

	// Fetch the block and verify resourceIds no longer contains the deleted ID
	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	require.Equal(t, http.StatusOK, blocksRR.Code)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(blocksRR.Body.Bytes(), &blocks))
	require.NotEmpty(t, blocks)

	content, ok := blocks[0]["content"].(map[string]any)
	require.True(t, ok, "block content must be a JSON object, got: %T", blocks[0]["content"])

	ids, _ := content["resourceIds"].([]any)
	for _, id := range ids {
		assert.NotEqual(t, float64(res.ID), id, "deleted resource ID must not remain in gallery.resourceIds")
	}
}

// TestGroupDelete_ScrubsReferencesBlockGroupIds verifies BH-020:
// deleting a group removes its ID from references block groupIds arrays.
func TestGroupDelete_ScrubsReferencesBlockGroupIds(t *testing.T) {
	tc := SetupTestEnv(t)
	grp := tc.CreateDummyGroup("bh020-grp")
	note := tc.CreateDummyNote("bh020-refnote")

	blockContent := fmt.Sprintf(`{"groupIds":[%d]}`, grp.ID)
	tc.createBlock(t, note.ID, "references", blockContent)

	tc.deleteGroup(t, grp.ID)

	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	require.Equal(t, http.StatusOK, blocksRR.Code)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(blocksRR.Body.Bytes(), &blocks))
	require.NotEmpty(t, blocks)

	content, _ := blocks[0]["content"].(map[string]any)
	ids, _ := content["groupIds"].([]any)
	for _, id := range ids {
		assert.NotEqual(t, float64(grp.ID), id, "deleted group ID must not remain in references.groupIds")
	}
}

// TestCalendarBlock_ScrubsResourceSourceOnResourceDelete verifies BH-020:
// deleting a resource removes its resourceId from calendar block source configurations.
func TestCalendarBlock_ScrubsResourceSourceOnResourceDelete(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.CreateDummyResource(t, "bh020-calres")
	note := tc.CreateDummyNote("bh020-calnote")

	blockContent := fmt.Sprintf(`{"calendars":[{"id":"cal1","name":"cal1","color":"#ff0000","source":{"type":"resource","resourceId":%d}}]}`, res.ID)
	tc.createBlock(t, note.ID, "calendar", blockContent)

	tc.deleteResource(t, res.ID)

	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID), nil)
	require.Equal(t, http.StatusOK, blocksRR.Code)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(blocksRR.Body.Bytes(), &blocks))
	require.NotEmpty(t, blocks)

	content, _ := blocks[0]["content"].(map[string]any)
	cals, _ := content["calendars"].([]any)
	require.NotEmpty(t, cals, "calendars array must be present after scrub")
	cal0 := cals[0].(map[string]any)
	source := cal0["source"].(map[string]any)
	// After scrub, resourceId should be absent
	if rid, ok := source["resourceId"]; ok {
		switch v := rid.(type) {
		case float64:
			assert.NotEqual(t, float64(res.ID), v, "calendar source resourceId must be scrubbed after resource delete")
		}
	}
}
