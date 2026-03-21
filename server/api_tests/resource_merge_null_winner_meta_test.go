package api_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models/query_models"
)

// TestMergeResources_NullWinnerMeta verifies that MergeResources correctly
// merges the loser's meta when the winner has NULL meta.
//
// Root cause: The meta-merge SQL uses json_patch(loser_meta, winner_meta).
// SQLite's json_patch returns NULL when the second argument is NULL, so
// the entire result becomes NULL — the loser's meta is silently discarded.
//
// This is the same bug class as MergeGroups/MergeTags NULL meta, but in
// MergeResources. The meta merge at resource_bulk_context.go line 547:
//
//	json_patch(coalesce((SELECT meta FROM resources WHERE id = loser), '{}'), meta)
//	                                                                         ^^^^
//	                                                                 NOT coalesced!
//
// And the backup save at line 578:
//
//	json_patch(meta, ?) where meta is NULL → returns NULL
func TestMergeResources_NullWinnerMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create winner resource with real content (AddResource sets meta to '{}')
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-content-data")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner"},
	})
	require.NoError(t, err)
	require.NotZero(t, winner.ID)

	// Force winner's meta to NULL (simulates imported/legacy data)
	tc.DB.Exec("UPDATE resources SET meta = NULL WHERE id = ?", winner.ID)

	// Verify winner meta is actually NULL
	var rawMeta *string
	tc.DB.Raw("SELECT meta FROM resources WHERE id = ?", winner.ID).Scan(&rawMeta)
	require.Nil(t, rawMeta, "winner meta should be NULL after forced update")

	// Create loser resource with real meta
	file2 := io.NopCloser(bytes.NewReader([]byte("loser-content-data-different")))
	loser, err := tc.AppCtx.AddResource(file2, "loser.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "Loser",
			Meta: `{"artist":"monet","year":"1899"}`,
		},
	})
	require.NoError(t, err)
	require.NotZero(t, loser.ID)

	// Merge loser into winner
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false)
	require.NoError(t, err, "MergeResources should succeed")

	// Reload winner's meta directly from DB
	var metaAfter *string
	tc.DB.Raw("SELECT meta FROM resources WHERE id = ?", winner.ID).Scan(&metaAfter)

	require.NotNil(t, metaAfter,
		"BUG: winner meta is NULL after merging a loser with meta — "+
			"json_patch(loser_meta, NULL) returns NULL, losing loser's meta")

	var metaMap map[string]any
	err = json.Unmarshal([]byte(*metaAfter), &metaMap)
	require.NoError(t, err, "winner meta should be valid JSON after merge, got: %s", *metaAfter)

	// The loser's meta keys should have been merged into the winner.
	assert.Equal(t, "monet", metaMap["artist"],
		"loser's 'artist' key should be merged into winner")
	assert.Equal(t, "1899", metaMap["year"],
		"loser's 'year' key should be merged into winner")

	// The backups key should also be present.
	assert.NotNil(t, metaMap["backups"],
		"backups key should be present in winner's meta after merge")
}
