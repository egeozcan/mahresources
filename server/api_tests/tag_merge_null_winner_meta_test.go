package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeTagsNullWinnerMeta demonstrates that MergeTags loses the loser's
// meta when the winner tag has NULL meta (the default for tags created via
// CreateTag, which does not set Meta).
//
// Root cause: The meta-merge SQL uses json_patch(loser_meta, winner_meta).
// SQLite's json_patch returns NULL when the second argument is NULL, so
// the entire result becomes NULL — the loser's meta is silently discarded.
//
// The correct fix is to coalesce the winner's meta to '{}' as well:
//   json_patch(coalesce(loser_meta, '{}'), coalesce(meta, '{}'))
//
// Steps to reproduce:
//  1. Create a winner tag with NO meta (NULL — the default from CreateTag)
//  2. Create a loser tag with meta {"color":"red"}
//  3. MergeTags(winner, [loser])
//  4. Reload winner — expect meta to contain "color":"red"
//
// Expected: winner.Meta contains {"color":"red", "backups":{...}}
// Actual:   winner.Meta is NULL — loser's meta is lost because
//           json_patch('{"color":"red"}', NULL) returns NULL.
func TestMergeTagsNullWinnerMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// --- Setup ---
	// Create winner with NULL meta (the default — CreateTag does not set Meta)
	winner := &models.Tag{
		Name: "winner-null-meta",
		// Meta intentionally NOT set — this is the default from CreateTag
	}
	tc.DB.Create(winner)
	require.NotZero(t, winner.ID)

	// Verify winner meta is NULL or the JSON literal "null"
	// (GORM may serialize SQL NULL as the JSON token "null" for types.JSON fields)
	var checkWinner models.Tag
	tc.DB.First(&checkWinner, winner.ID)
	isNullMeta := checkWinner.Meta == nil || string(checkWinner.Meta) == "null"
	assert.True(t, isNullMeta, "winner meta should be NULL/null (the default for CreateTag), got: %s", string(checkWinner.Meta))

	// Create loser with real meta
	loser := &models.Tag{
		Name: "loser-with-meta",
		Meta: types.JSON(`{"color":"red","priority":"high"}`),
	}
	tc.DB.Create(loser)
	require.NotZero(t, loser.ID)

	// --- Act ---
	err := tc.AppCtx.MergeTags(winner.ID, []uint{loser.ID})
	require.NoError(t, err, "MergeTags should succeed")

	// --- Assert ---
	var updated models.Tag
	err = tc.DB.First(&updated, winner.ID).Error
	require.NoError(t, err, "winner tag should still exist after merge")

	// The winner's meta must not be NULL/null after merging a loser that had meta.
	isNullAfterMerge := updated.Meta == nil || string(updated.Meta) == "null"
	require.False(t, isNullAfterMerge,
		"winner meta should NOT be NULL/null after merging a loser with meta — "+
			"json_patch(loser_meta, NULL) returns NULL, losing loser's meta; got: %s", string(updated.Meta))

	var metaMap map[string]interface{}
	err = json.Unmarshal(updated.Meta, &metaMap)
	require.NoError(t, err, "winner meta should be valid JSON after merge")

	// The loser's meta keys should have been merged into the winner.
	assert.Equal(t, "red", metaMap["color"],
		"loser's 'color' key should be merged into winner")
	assert.Equal(t, "high", metaMap["priority"],
		"loser's 'priority' key should be merged into winner")
}
