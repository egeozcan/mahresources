package application_context

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// legacyPlanFixture is a legacy JSON plan over real Resources: an open Identical
// Cluster with a recorded owner and a changed member, an applied and merged one,
// an over-cap Near one (not applied), an empty-cluster-list apply record.
func seedLegacyReduction(t *testing.T, ctx *MahresourcesContext, withOverCap bool) (*models.ResourceReduction, []uint) {
	t.Helper()
	var ids []uint
	for i := 0; i < 6; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", FileSize: int64(10 * (i + 1))}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	member := func(id uint, extra map[string]any) map[string]any {
		m := map[string]any{"resourceId": id, "hash": "h", "inExtent": true}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	clusters := []any{
		map[string]any{"id": "c-open", "tier": "identical", "winnerId": ids[0], "state": "open", "checked": true, "reviewed": true,
			"decidedBy": "pixels", "margin": "2x", "lossy": []string{"description"}, "changedMembers": []uint{ids[1]},
			"loserOwners": map[string]uint{fmt.Sprint(ids[1]): 7},
			"members":     []any{member(ids[0], nil), member(ids[1], nil), member(ids[2], map[string]any{"ejected": true, "ejectedReason": "manual"})}},
		map[string]any{"id": "c-applied", "tier": "near", "winnerId": ids[3], "state": "applied", "merged": true, "claimId": "claim1",
			"loserOwners": map[string]uint{fmt.Sprint(ids[4]): 9}, "appliedAt": "2026-01-02T03:04:05Z",
			"members": []any{member(ids[3], nil), member(ids[4], map[string]any{"distance": 3})}},
	}
	if withOverCap {
		var big []any
		big = append(big, member(ids[5], nil))
		for i := 0; i < 205; i++ {
			big = append(big, member(100000+uint(i), nil))
		}
		clusters = append(clusters, map[string]any{"id": "c-big", "tier": "near", "winnerId": ids[5], "state": "open", "checked": true, "members": big})
	}
	plan := map[string]any{"clusters": clusters,
		"coverage":  map[string]any{"extentSize": 6, "contentHashed": 6},
		"settings":  map[string]any{"matchingMode": "both", "winnerRule": []string{"pixels"}},
		"lastApply": map[string]any{"startedAt": "2026-01-02T03:04:05Z", "finishedAt": "2026-01-02T03:05:05Z", "applied": 1, "removed": 1, "stale": 0, "clusterIds": []string{"c-applied"}, "requestId": "req"}}
	planJSON, err := json.Marshal(plan)
	require.NoError(t, err)
	extent, _ := json.Marshal(map[string]any{"resourceIds": ids[:3], "groupIds": []uint{4, 5}, "all": true})
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: planJSON, Extent: extent}
	require.NoError(t, ctx.db.Create(legacy).Error)
	return &legacy.ResourceReduction, ids
}

func TestReductionMigrationSpoolsAndPublishes(t *testing.T) {
	testReductionMigrationSpoolsAndPublishes(t, newReductionSQLiteContext(t))
}

func testReductionMigrationSpoolsAndPublishes(t *testing.T, ctx *MahresourcesContext) {
	red, ids := seedLegacyReduction(t, ctx, true)

	require.NoError(t, ctx.MigrateReductionPlans())

	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	require.NotNil(t, got.PlanGeneration)
	require.True(t, got.ExtentAll)
	var legacy struct{ Plan, Extent *string }
	require.NoError(t, ctx.db.Raw("SELECT plan, extent FROM resource_reductions WHERE id = ?", red.ID).Scan(&legacy).Error)
	require.Nil(t, legacy.Plan, "publication nulls the legacy plan")
	require.Nil(t, legacy.Extent)

	var rows []models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND generation = ?", red.ID, *got.PlanGeneration).Order("position").Find(&rows).Error)
	require.Len(t, rows, 3)
	open, applied, big := rows[0], rows[1], rows[2]
	require.Equal(t, "c-open", open.Key)
	require.Equal(t, 3, open.MemberCount)
	require.Equal(t, 1, open.LoserCount, "the ejected member is not a Loser")
	require.True(t, open.NeedsAttention, "lossy")
	require.NotEmpty(t, open.Fingerprint)
	require.False(t, open.ReadOnly)
	require.Equal(t, int64(10+20+30), open.TotalSize)
	require.Equal(t, models.ReductionClusterApplied, applied.State)
	require.True(t, applied.Merged)
	require.Equal(t, "claim1", applied.ClaimID)
	require.NotNil(t, applied.AppliedAt)
	require.Equal(t, "2026-01-02T03:04:05Z", applied.AppliedAt.UTC().Format("2006-01-02T15:04:05Z"))
	require.NotEmpty(t, applied.OutcomeBatch, "lastApply.clusterIds becomes outcome_batch")
	require.Empty(t, open.OutcomeBatch)

	require.True(t, big.ReadOnly, "an over-cap Cluster that is not applied and merged is read only")
	require.Equal(t, models.ReductionClusterStale, big.State)
	require.False(t, big.Checked)
	require.Empty(t, big.Fingerprint)
	require.Equal(t, 206, big.MemberCount)
	require.Equal(t, 205, big.GoneMembers, "205 members name no Resource")

	var members []models.ReductionMemberRow
	require.NoError(t, ctx.db.Where("cluster_id = ?", open.ID).Order("position").Find(&members).Error)
	require.Len(t, members, 3)
	require.Equal(t, uint(7), members[1].OwnerAtMerge)
	require.True(t, members[1].Changed)
	require.False(t, members[0].Changed)
	require.True(t, members[2].Ejected)
	require.Equal(t, int64(20), members[1].FileSize)
	require.Equal(t, ids[1], members[1].ResourceID)
	var am []models.ReductionMemberRow
	require.NoError(t, ctx.db.Where("cluster_id = ?", applied.ID).Order("position").Find(&am).Error)
	require.Equal(t, uint(9), am[1].OwnerAtMerge)
	require.NotNil(t, am[1].Distance)
	require.Equal(t, 3, *am[1].Distance)

	var nExtentRes, nExtentGroups, nSpool, nMig int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ?", red.ID).Count(&nExtentRes).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionExtentGroup{}).Where("reduction_id = ?", red.ID).Count(&nExtentGroups).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", red.ID).Count(&nSpool).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionMigration{}).Where("reduction_id = ?", red.ID).Count(&nMig).Error)
	require.EqualValues(t, 3, nExtentRes)
	require.EqualValues(t, 2, nExtentGroups)
	require.EqualValues(t, 0, nMig, "publication deletes the migration row")
	require.EqualValues(t, 0, nSpool, "the spool is deleted after publication")

	// idempotent: a second run touches nothing
	require.NoError(t, ctx.MigrateReductionPlans())
}

func TestReductionMigrationNullClustersKeepsApplyRecord(t *testing.T) {
	testReductionMigrationNullClustersKeepsApplyRecord(t, newReductionSQLiteContext(t))
}

func testReductionMigrationNullClustersKeepsApplyRecord(t *testing.T, ctx *MahresourcesContext) {
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "empty", Status: models.ReductionStatusReady, MatchingMode: "both"},
		Plan: []byte(`{"clusters":null,"coverage":{},"lastApply":{"startedAt":"2026-01-02T03:04:05Z","applied":0,"removed":0,"stale":0,"requestId":"r"}}`)}
	require.NoError(t, ctx.db.Create(legacy).Error)
	red := &legacy.ResourceReduction
	require.NoError(t, ctx.MigrateReductionPlans())
	var got models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&got, red.ID).Error)
	require.Equal(t, 1, got.PlanSchema)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(got.LastApply, &rec))
	require.Equal(t, "r", rec["requestId"])
}

func TestReductionMigrationFailsComputingAndMigratesDrafts(t *testing.T) {
	testReductionMigrationFailsComputingAndMigratesDrafts(t, newReductionSQLiteContext(t))
}

func testReductionMigrationFailsComputingAndMigratesDrafts(t *testing.T, ctx *MahresourcesContext) {
	computingRow := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "c", Status: models.ReductionStatusComputing, MatchingMode: "both"}}
	draftRow := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "d", Status: models.ReductionStatusDraft, MatchingMode: "both"}, Extent: []byte(`{"resourceIds":[1,2]}`)}
	require.NoError(t, ctx.db.Create(computingRow).Error)
	require.NoError(t, ctx.db.Create(draftRow).Error)
	computing, draft := &computingRow.ResourceReduction, &draftRow.ResourceReduction
	require.NoError(t, ctx.MigrateReductionPlans())
	var c, d models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&c, computing.ID).Error)
	require.NoError(t, ctx.db.Select(reductionColumns).First(&d, draft.ID).Error)
	require.Equal(t, models.ReductionStatusFailed, c.Status)
	require.Contains(t, c.ComputeError, "interrupted by an upgrade")
	require.Equal(t, 1, d.PlanSchema)
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ?", draft.ID).Count(&n).Error)
	require.EqualValues(t, 2, n)
}

func TestReductionMigrationResumesAfterTakeover(t *testing.T) {
	testReductionMigrationResumesAfterTakeover(t, newReductionSQLiteContext(t))
}

// A runner that stops after some Clusters leaves a checkpoint; the next run takes
// over with a new writer token, resumes there, and the stalled token can write
// nothing (the fence refuses it).
func testReductionMigrationResumesAfterTakeover(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	s := ctx.reductionStore()
	mig, err := s.takeOverMigration(red.ID)
	require.NoError(t, err)
	_, err = s.takeOverMigration(red.ID)
	require.ErrorIs(t, err, errReductionMigrationBusy, "a live lease is not taken over")

	require.NoError(t, s.migrateCluster(mig, spoolHead{ClusterOrd: 0, N: 3}))
	// the lease lapses (a stalled runner); a second runner takes over
	require.NoError(t, ctx.db.Exec("UPDATE reduction_migrations SET lease_until = 1 WHERE reduction_id = ?", red.ID).Error)
	second, err := s.takeOverMigration(red.ID)
	require.NoError(t, err)
	require.NotEqual(t, mig.WriterToken, second.WriterToken)
	require.EqualValues(t, 1, second.Checkpoint)

	err = s.migrateCluster(mig, spoolHead{ClusterOrd: 1, N: 2})
	require.ErrorIs(t, err, errReductionMigrationBusy, "the stalled token writes nothing")
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ?", red.ID).Count(&n).Error)
	require.EqualValues(t, 1, n)

	require.NoError(t, ctx.db.Exec("UPDATE reduction_migrations SET lease_until = 1 WHERE reduction_id = ?", red.ID).Error)
	require.NoError(t, ctx.MigrateReductionPlans())
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ?", red.ID).Count(&n).Error)
	require.EqualValues(t, 2, n, "the resumed run wrote only the missing Cluster")
}
