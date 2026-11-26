package application_context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// (8) A settings-less legacy Near Cluster derives keeps_smaller from its members
// when it migrates, and an over-cap one does too: the SQL aggregate compares the
// Winner's pixels with the largest non-ejected Loser's.
func testReductionOverCapMigrationDerivesKeepsSmaller(t *testing.T, ctx *MahresourcesContext) {
	mk := func(name string, w, h uint) uint {
		r := &models.Resource{Name: name, Hash: "h", Width: w, Height: h}
		require.NoError(t, ctx.db.Create(r).Error)
		return r.ID
	}
	smallWinner, big := mk("small", 10, 10), mk("big", 100, 100)
	bigWinner, small := mk("bigw", 100, 100), mk("smalll", 10, 10)
	bigEjected := mk("ejected", 500, 500)

	build := func(winner uint, others []uint, extra int, ejected uint) map[string]any {
		members := []any{map[string]any{"resourceId": winner, "hash": "h", "inExtent": true}}
		for _, o := range others {
			members = append(members, map[string]any{"resourceId": o, "hash": "h", "inExtent": true})
		}
		if ejected != 0 {
			members = append(members, map[string]any{"resourceId": ejected, "hash": "h", "inExtent": true, "ejected": true, "ejectedReason": "manual"})
		}
		for i := 0; i < extra; i++ { // Resources that no longer exist
			members = append(members, map[string]any{"resourceId": 900000 + i, "hash": "h", "inExtent": true})
		}
		return map[string]any{"tier": "near", "winnerId": winner, "state": "open", "decidedBy": "manual", "members": members}
	}
	clusters := []any{}
	for i, c := range []map[string]any{
		build(smallWinner, []uint{big}, 250, 0),          // promoted a smaller copy: flagged
		build(bigWinner, []uint{small}, 250, 0),          // winner is the larger: not flagged
		build(bigWinner, []uint{small}, 250, bigEjected), // the larger copy is ejected: not flagged
		build(smallWinner, []uint{big}, 3, 0),            // under the cap, the control
	} {
		c["id"] = []string{"flagged", "larger", "ejected", "control"}[i]
		clusters = append(clusters, c)
	}
	plan, err := json.Marshal(map[string]any{"clusters": clusters}) // no settings
	require.NoError(t, err)
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: plan, Extent: []byte(`{"all":true}`)}
	require.NoError(t, ctx.db.Create(legacy).Error)

	require.NoError(t, ctx.MigrateReductionPlans())
	require.Equal(t, 1, planSchemaOf(t, ctx, legacy.ID))

	got := map[string]models.ReductionClusterRow{}
	var rows []models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ?", legacy.ID).Find(&rows).Error)
	for _, r := range rows {
		got[r.Key] = r
	}
	require.True(t, got["control"].KeepsSmaller, "the under-cap path derives it")
	require.True(t, got["flagged"].ReadOnly, "the cluster is over the cap")
	require.True(t, got["flagged"].KeepsSmaller, "an over-cap Cluster derives it too")
	require.True(t, got["flagged"].NeedsAttention)
	require.False(t, got["larger"].KeepsSmaller)
	require.False(t, got["ejected"].KeepsSmaller, "an ejected copy is not a candidate")
}

func TestReductionOverCapMigrationDerivesKeepsSmallerSQLite(t *testing.T) {
	testReductionOverCapMigrationDerivesKeepsSmaller(t, newReductionSQLiteContext(t))
}
