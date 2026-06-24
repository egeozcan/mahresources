package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models/query_models"
)

// assertGalleryDoesNotReference fetches the note's first block and asserts the
// given resource ID is absent from gallery.resourceIds.
func assertGalleryDoesNotReference(t *testing.T, tc *TestContext, noteID, resID uint) {
	t.Helper()
	blocksRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/blocks?noteId=%d", noteID), nil)
	require.Equal(t, http.StatusOK, blocksRR.Code)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(blocksRR.Body.Bytes(), &blocks))
	require.NotEmpty(t, blocks)

	content, _ := blocks[0]["content"].(map[string]any)
	ids, _ := content["resourceIds"].([]any)
	for _, id := range ids {
		assert.NotEqual(t, float64(resID), id, "resource ID must be scrubbed from gallery.resourceIds")
	}
}

// B6: bulk resource delete must scrub dangling gallery references, matching the
// single-delete behavior (BH-020).
func TestBulkDeleteResources_ScrubsGalleryBlockReferences(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.CreateDummyResource(t, "bulk-del-res")
	note := tc.CreateDummyNote("bulk-del-note")
	tc.createBlock(t, note.ID, "gallery", fmt.Sprintf(`{"resourceIds":[%d]}`, res.ID))

	require.NoError(t, tc.AppCtx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{res.ID}}))

	assertGalleryDoesNotReference(t, tc, note.ID, res.ID)
}

// B6 (review follow-up): the batched scrub must remove MULTIPLE deleted ids
// from a single block in one traversal.
func TestBulkDeleteResources_ScrubsMultipleIdsInOneBlock(t *testing.T) {
	tc := SetupTestEnv(t)
	r1 := tc.CreateDummyResource(t, "bulk-multi-1")
	r2 := tc.CreateDummyResource(t, "bulk-multi-2")
	note := tc.CreateDummyNote("bulk-multi-note")
	tc.createBlock(t, note.ID, "gallery", fmt.Sprintf(`{"resourceIds":[%d,%d]}`, r1.ID, r2.ID))

	require.NoError(t, tc.AppCtx.BulkDeleteResources(&query_models.BulkQuery{ID: []uint{r1.ID, r2.ID}}))

	assertGalleryDoesNotReference(t, tc, note.ID, r1.ID)
	assertGalleryDoesNotReference(t, tc, note.ID, r2.ID)
}

// B6: resource merge must scrub dangling gallery references to the loser.
func TestMergeResources_ScrubsGalleryBlockReferences(t *testing.T) {
	tc := SetupTestEnv(t)
	winner := tc.CreateDummyResource(t, "merge-winner")
	loser := tc.CreateDummyResource(t, "merge-loser")
	note := tc.CreateDummyNote("merge-note")
	tc.createBlock(t, note.ID, "gallery", fmt.Sprintf(`{"resourceIds":[%d]}`, loser.ID))

	require.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false))

	assertGalleryDoesNotReference(t, tc, note.ID, loser.ID)
}
