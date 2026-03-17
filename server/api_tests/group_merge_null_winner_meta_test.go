package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeGroups_NullWinnerMeta verifies that MergeGroups correctly merges
// the loser's meta when the winner has NULL meta.
//
// Root cause: The meta-merge SQL uses json_patch(loser_meta, winner_meta).
// SQLite's json_patch returns NULL when the second argument is NULL, so
// the entire result becomes NULL — the loser's meta is silently discarded.
//
// MergeTags was already fixed for this (coalesces both sides), but MergeGroups
// only coalesces the loser's meta, not the winner's:
//
//	json_patch(coalesce((SELECT meta FROM groups WHERE id = loser), '{}'), meta)
//	                                                                      ^^^^
//	                                                              NOT coalesced!
//
// The fix should match MergeTags:
//
//	json_patch(coalesce(loser_meta, '{}'), coalesce(nullif(meta, 'null'), '{}'))
//
// Steps to reproduce:
//  1. Create a winner group with NULL meta (bypassing API's default)
//  2. Create a loser group with meta {"color":"blue","size":"large"}
//  3. MergeGroups(winner, [loser])
//  4. Reload winner — expect meta to contain loser's keys + backups
//
// Expected: winner.Meta contains {"color":"blue","size":"large","backups":{...}}
// Actual:   winner.Meta is NULL — loser's meta AND backups are both lost.
func TestMergeGroups_NullWinnerMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create winner with NULL meta (simulates imported/legacy data)
	winner := &models.Group{Name: "winner-null-meta"}
	tc.DB.Create(winner)
	require.NotZero(t, winner.ID)

	// Force meta to NULL (bypassing any GORM hooks)
	tc.DB.Exec("UPDATE groups SET meta = NULL WHERE id = ?", winner.ID)

	// Verify winner meta is actually NULL
	var checkWinner models.Group
	tc.DB.First(&checkWinner, winner.ID)
	isNullMeta := checkWinner.Meta == nil || string(checkWinner.Meta) == "null"
	require.True(t, isNullMeta, "winner meta should be NULL, got: %s", string(checkWinner.Meta))

	// Create loser with real meta
	loser := &models.Group{
		Name: "loser-with-meta",
		Meta: types.JSON(`{"color":"blue","size":"large"}`),
	}
	tc.DB.Create(loser)
	require.NotZero(t, loser.ID)

	// Merge loser into winner
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loser.ID})
	require.NoError(t, err, "MergeGroups should succeed")

	// Reload winner
	var updated models.Group
	err = tc.DB.First(&updated, winner.ID).Error
	require.NoError(t, err, "winner group should still exist after merge")

	// The winner's meta must not be NULL after merging a loser that had meta.
	isNullAfterMerge := updated.Meta == nil || string(updated.Meta) == "null"
	require.False(t, isNullAfterMerge,
		"BUG: winner meta is NULL after merging a loser with meta — "+
			"json_patch(loser_meta, NULL) returns NULL, losing loser's meta; got: %s",
		string(updated.Meta))

	var metaMap map[string]any
	err = json.Unmarshal(updated.Meta, &metaMap)
	require.NoError(t, err, "winner meta should be valid JSON after merge")

	// The loser's meta keys should have been merged into the winner.
	assert.Equal(t, "blue", metaMap["color"],
		"loser's 'color' key should be merged into winner")
	assert.Equal(t, "large", metaMap["size"],
		"loser's 'size' key should be merged into winner")

	// The backups key should also be present (saved after meta merge).
	assert.NotNil(t, metaMap["backups"],
		"backups key should be present in winner's meta after merge")
}
