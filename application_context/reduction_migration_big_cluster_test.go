package application_context

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// A legacy plan with one very large Cluster spools in time linear in its members.
// Found by the D14 scale proof: the SQLite spool statement re-read the whole
// Cluster's JSON once per member for every Cluster-level field (the Cluster's id,
// tier, state, loser owners, changed members), so a 50,000-member Cluster took
// time quadratic in its size. The bound is generous (the fix runs in well under a
// second) and the quadratic form misses it by a factor of ten or more.
func TestMigrationOfALargeClusterIsLinear(t *testing.T) {
	testMigrationOfALargeClusterIsLinear(t, newReductionSQLiteContext(t))
}

func testMigrationOfALargeClusterIsLinear(t *testing.T, ctx *MahresourcesContext) {
	const members = 8000
	first := seedResources(t, ctx, members, `'big' || n`, `'bh'`, `NULL`)
	type m = map[string]any
	var ms []any
	owners := m{}
	var changed []uint
	for i := 0; i < members; i++ {
		id := first + uint(i)
		ms = append(ms, m{"resourceId": id, "hash": "bh", "inExtent": true})
		if i%100 == 0 {
			owners[idText(id)] = 7
			changed = append(changed, id)
		}
	}
	cluster := m{"id": "c-big", "tier": "near", "winnerId": first, "state": "applied", "merged": true, "claimId": "c1", "members": ms,
		"loserOwners": owners, "changedMembers": changed}
	red := seedLegacyPlan(t, ctx, []any{cluster})

	start := time.Now()
	require.NoError(t, ctx.MigrateReductionPlans())
	took := time.Since(start)
	t.Logf("migration of a %d-member Cluster took %s", members, took.Round(time.Millisecond))
	require.Less(t, took, 15*time.Second, "the spool's cost must not grow with the square of a Cluster's members")

	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ?", red.ID).First(&row).Error)
	require.Equal(t, members, row.MemberCount)
	var recorded, flagged int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("cluster_id = ? AND owner_at_merge = 7", row.ID).Count(&recorded).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("cluster_id = ? AND changed", row.ID).Count(&flagged).Error)
	require.EqualValues(t, members/100, recorded, "each recorded owner reaches its member")
	require.EqualValues(t, members/100, flagged, "each changed member is flagged")
}

func idText(id uint) string { return strconv.FormatUint(uint64(id), 10) }

// seedLegacyPlan stores a legacy Reduction whose plan holds the given Clusters.
func seedLegacyPlan(t *testing.T, ctx *MahresourcesContext, clusters []any) *models.ResourceReduction {
	t.Helper()
	plan, err := json.Marshal(map[string]any{"clusters": clusters,
		"settings": map[string]any{"matchingMode": "both", "winnerRule": []string{"pixels"}}})
	require.NoError(t, err)
	extent, _ := json.Marshal(map[string]any{"all": true})
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy big", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: plan, Extent: extent}
	require.NoError(t, ctx.db.Create(legacy).Error)
	return &legacy.ResourceReduction
}
