package api_tests

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Closing the tab, or a proxy dropping the connection, cancels the request's
// context. Apply claims a Cluster as applied *before* it merges, so a cancellation
// landing between the two used to strand it: marked applied over Losers that still
// exist, and the failure never demoted to stale because that write was cancelled
// too. The batch now runs on a context detached from the request.
//
// The cancellation is injected rather than raced for: a GORM callback cancels the
// request context on the write that claims the first Cluster, which is the one
// place the window is microseconds wide.
func TestApplySurvivesTheRequestBeingCancelledAfterTheClaim(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Cancelled request", []uint{winner.ID, loser.ID})
	computeReduction(t, tc, red.ID)
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)

	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := tc.AppCtx.WithParentContext(requestCtx)

	// The first write to the row is the batch's claim, the second is the Cluster's.
	var writes atomic.Int32
	var cancelled atomic.Bool
	require.NoError(t, tc.DB.Callback().Update().After("gorm:update").Register("test:cancel_after_cluster_claim", func(db *gorm.DB) {
		if db.Statement.Table != "resource_reductions" || cancelled.Load() {
			return
		}
		if writes.Add(1) == 2 && cancelled.CompareAndSwap(false, true) {
			cancel()
		}
	}))

	_, _ = request.ApplyResourceReduction(&query_models.ReductionApply{
		ID: red.ID, Version: current.Version,
	}, owner, restricted)
	require.True(t, cancelled.Load(), "the injected cancellation never ran, so this proves nothing")

	plan := planOf(t, tc, red.ID)
	cluster := plan.Clusters[0]
	assert.Equal(t, models.ReductionClusterApplied, cluster.State)
	assert.True(t, cluster.Merged, "a claimed Cluster is merged and confirmed, not stranded applied over live Losers")
	assert.False(t, resourceExists(t, tc, loser.ID), "the merge ran to completion")
	require.NotNil(t, plan.LastApply)
	assert.NotNil(t, plan.LastApply.FinishedAt, "and the batch's record was completed")
}

// The batch records itself: running at the claim, finished at the end, with counts
// only. The record survives every other writer of the plan.
func TestApplyRecordsTheLastApplyAndTheOtherWritersKeepIt(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	a := clusterOf(t, tc, "last-a", 2)
	b := clusterOf(t, tc, "last-b", 3)
	c := clusterOf(t, tc, "last-c", 2)
	red := createReduction(t, tc, "Last apply", append(append(append([]uint{}, a...), b...), c...))
	plan := computeReduction(t, tc, red.ID)
	assert.Nil(t, plan.LastApply, "nothing has been applied yet")
	// Left open, so the writers below have a Cluster to act on after the apply.
	openID := clusterFor(plan, c[0]).ID
	override(t, tc, red.ID, openID, application_context.ReductionActionUncheck, 0)
	// A stale Cluster: bytes changed after review.
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", a[1]).Update("hash", "changed").Error)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)

	var observed atomic.Pointer[models.ReductionLastApply]
	var reads atomic.Int32
	var inside atomic.Bool
	require.NoError(t, tc.DB.Callback().Query().After("gorm:query").Register("test:observe_last_apply", func(db *gorm.DB) {
		// Reading the row from inside the callback is itself a read of the row.
		if db.Statement.Table != "resource_reductions" || observed.Load() != nil || !inside.CompareAndSwap(false, true) {
			return
		}
		defer inside.Store(false)
		if reads.Add(1) < 3 {
			return
		}
		row, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
		if err != nil {
			return
		}
		if p, err := application_context.DecodeReductionPlan(row.Plan); err == nil && p.LastApply != nil {
			cp := *p.LastApply
			observed.Store(&cp)
		}
	}))

	result, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version}, owner, restricted)
	require.NoError(t, err)
	require.Len(t, result.Applied, 1)
	require.Len(t, result.Stale, 1)

	seen := observed.Load()
	require.NotNil(t, seen, "the record was written before the first Cluster merged")
	assert.False(t, seen.StartedAt.IsZero())
	assert.Nil(t, seen.FinishedAt, "and read as running while the batch worked")

	plan = planOf(t, tc, red.ID)
	last := plan.LastApply
	require.NotNil(t, last)
	require.NotNil(t, last.FinishedAt)
	assert.Equal(t, 1, last.Applied)
	assert.Equal(t, 2, last.Removed)
	assert.Equal(t, 1, last.Stale)

	// An override, a bulk act and a recompute all carry it over untouched.
	override(t, tc, red.ID, openID, application_context.ReductionActionCheck, 0)
	assert.Equal(t, last.Applied, planOf(t, tc, red.ID).LastApply.Applied)
	require.NoError(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "uncheck", ClusterIDs: []string{openID},
	}))
	assert.NotNil(t, planOf(t, tc, red.ID).LastApply)
	recomputed := computeReduction(t, tc, red.ID)
	require.NotNil(t, recomputed.LastApply, "a recompute rebuilds the Clusters, not the record of the last apply")
	assert.Equal(t, last.Removed, recomputed.LastApply.Removed)

	view := reviewOf(t, tc, red.ID, 1, nil).LastApply
	require.NotNil(t, view)
	assert.False(t, view.Running)
	assert.False(t, view.Interrupted)
}

// The stale count excludes Clusters that were withheld from whoever applied, so a
// scope-restricted owner cannot learn how many there were.
func TestLastApplyDoesNotCountWithheldClusters(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	hidden := clusterOf(t, tc, "last-hidden", 2)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[0]).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[1]).Update("owner_id", wide.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Withheld apply", ResourceIds: hidden,
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	awaitReduction(t, tc, red.ID)

	narrowCtx := scopedTo(t, tc, narrow.ID)
	current, err := narrowCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version}, owner, restricted)
	require.NoError(t, err)
	require.Len(t, result.Stale, 1)
	require.True(t, result.Stale[0].Withheld)

	last := planOf(t, tc, red.ID).LastApply
	require.NotNil(t, last)
	assert.Zero(t, last.Stale, "a withheld outcome is not counted, stored or displayed")
	assert.Zero(t, last.Applied)
}
