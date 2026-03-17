package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeTagsLosesLoserMeta demonstrates that MergeTags does NOT merge the
// losers' meta into the winner's meta, unlike MergeGroups and MergeResources
// which do perform this merge.
//
// The design document for MergeTags states:
//   "Follows the exact merge pattern established by MergeGroups"
//
// MergeGroups merges each loser's meta into the winner (winner keys win on
// conflict) via:
//
//   UPDATE groups SET meta = json_patch(
//       coalesce((SELECT meta FROM groups WHERE id = <loser>), '{}'), meta
//   ) WHERE id = <winner>
//
// MergeTags is missing this step entirely. The loser's meta is serialised into
// the backup structure but its keys are never actively merged into the winner.
//
// Steps to reproduce:
//  1. Create a winner tag  with meta {"winner_key":"winner_val"}
//  2. Create a loser tag   with meta {"loser_key":"loser_val"}
//  3. MergeTags(winner, [loser])
//  4. Reload winner — expect meta to contain BOTH "winner_key" AND "loser_key"
//
// Expected: winner.Meta contains {"winner_key":"winner_val","loser_key":"loser_val",
//           "backups":{...}}
// Actual:   winner.Meta only contains {"backups":{...}} — loser_key is missing.
func TestMergeTagsLosesLoserMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// --- Setup ---
	winner := &models.Tag{
		Name: "winner-tag",
		Meta: types.JSON(`{"winner_key":"winner_val"}`),
	}
	tc.DB.Create(winner)

	loser := &models.Tag{
		Name: "loser-tag",
		Meta: types.JSON(`{"loser_key":"loser_val"}`),
	}
	tc.DB.Create(loser)

	// Sanity: both tags exist
	require.NotZero(t, winner.ID)
	require.NotZero(t, loser.ID)

	// --- Act ---
	err := tc.AppCtx.MergeTags(winner.ID, []uint{loser.ID})
	require.NoError(t, err, "MergeTags should succeed")

	// --- Assert ---
	var updated models.Tag
	err = tc.DB.First(&updated, winner.ID).Error
	require.NoError(t, err, "winner tag should still exist after merge")

	var metaMap map[string]interface{}
	err = json.Unmarshal(updated.Meta, &metaMap)
	require.NoError(t, err, "winner meta should be valid JSON")

	// The winner's original key must survive.
	assert.Equal(t, "winner_val", metaMap["winner_key"],
		"winner's own key should be preserved after merge")

	// The loser's key should have been merged in (winner wins on conflict).
	// This is the assertion that exposes the bug: MergeTags does NOT merge
	// loser meta, so "loser_key" is absent from the winner's meta.
	assert.Equal(t, "loser_val", metaMap["loser_key"],
		"loser's meta key should be merged into winner — "+
			"MergeTags is missing the meta-merge step that MergeGroups/MergeResources have")
}
