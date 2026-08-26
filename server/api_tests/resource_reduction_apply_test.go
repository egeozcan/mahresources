package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

func applyReduction(t *testing.T, tc *TestContext, id uint) *contracts.ReductionApplyResult {
	t.Helper()
	owner, restricted := asAdmin()
	current, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
	require.NoError(t, err)
	result, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: id, Version: current.Version,
	}, owner, restricted)
	require.NoError(t, err)
	return result
}

func planOf(t *testing.T, tc *TestContext, id uint) models.ResourceReductionPlan {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
	require.NoError(t, err)
	plan, err := application_context.DecodeReductionPlan(red.Plan)
	require.NoError(t, err)
	return plan
}

func resourceExists(t *testing.T, tc *TestContext, id uint) bool {
	t.Helper()
	var count int64
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Count(&count).Error)
	return count > 0
}

// Applying merges every checked Cluster's Losers into its Winner and deletes
// them. The Winner survives, and the Cluster is marked applied.
func TestApplyMergesTheCheckedClusters(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Apply", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)
	require.True(t, plan.Clusters[0].Checked, "an Identical Cluster arrives checked")

	result := applyReduction(t, tc, red.ID)

	require.Len(t, result.Applied, 1)
	assert.Empty(t, result.Stale)
	assert.Equal(t, []uint{loser.ID}, result.Applied[0].LoserIDs)

	assert.True(t, resourceExists(t, tc, winner.ID))
	assert.False(t, resourceExists(t, tc, loser.ID))

	after := planOf(t, tc, red.ID)
	assert.Equal(t, models.ReductionClusterApplied, after.Clusters[0].State)
	assert.False(t, after.Clusters[0].Checked)
	require.NotNil(t, after.Clusters[0].AppliedAt)
}

// An unchecked Cluster stays open and untouched, so applying is a deliberate
// selection rather than all-or-nothing and partial progress is progress.
func TestApplyLeavesUncheckedClustersAlone(t *testing.T) {
	tc := SetupTestEnv(t)

	winnerA := addWithHash(t, tc, "a1.txt", "a1 body", "hash-a")
	loserA := addWithHash(t, tc, "a2.txt", "a2 body", "hash-a")
	winnerB := addWithHash(t, tc, "b1.txt", "b1 body", "hash-b")
	loserB := addWithHash(t, tc, "b2.txt", "b2 body", "hash-b")
	setDimensions(t, tc, winnerA.ID, 400, 400)
	setDimensions(t, tc, winnerB.ID, 400, 400)

	red := createReduction(t, tc, "Partial", []uint{winnerA.ID, loserA.ID, winnerB.ID, loserB.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 2)

	clusterB := findClusterByID(plan, clusterFor(plan, winnerB.ID).ID)
	override(t, tc, red.ID, clusterB.ID, application_context.ReductionActionUncheck, 0)

	result := applyReduction(t, tc, red.ID)

	assert.Len(t, result.Applied, 1)
	assert.False(t, resourceExists(t, tc, loserA.ID))
	assert.True(t, resourceExists(t, tc, loserB.ID), "the Cluster left unchecked is untouched")

	after := planOf(t, tc, red.ID)
	remaining := findClusterByID(after, clusterB.ID)
	require.NotNil(t, remaining, "and it stays in the Reduction")
	assert.Equal(t, models.ReductionClusterOpen, remaining.State)
}

// A Cluster whose Resources changed since it was computed is refused at apply and
// marked stale rather than merged — never delete bytes nobody reviewed.
func TestApplyRefusesAClusterWhoseBytesChanged(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Stale", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID

	// A version upload rewrites resources.hash and leaves the similarity pairs
	// untouched, which is exactly why the plan snapshots each member's hash.
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", loser.ID).
		Update("hash", "different-bytes-now").Error)

	result := applyReduction(t, tc, red.ID)

	assert.Empty(t, result.Applied)
	require.Len(t, result.Stale, 1, "and the stale Cluster is named in the result")
	assert.Equal(t, clusterID, result.Stale[0].ClusterID)
	assert.Contains(t, result.Stale[0].Reason, "bytes")

	assert.True(t, resourceExists(t, tc, loser.ID), "nothing was destroyed")

	after := planOf(t, tc, red.ID)
	stale := findClusterByID(after, clusterID)
	require.NotNil(t, stale, "and it stays in the Reduction to be looked at")
	assert.Equal(t, models.ReductionClusterStale, stale.State)
	assert.NotEmpty(t, stale.StaleReason)
}

// One stray edit must not waste a review of four hundred, so the rest of the batch
// applies around a Cluster that goes stale.
func TestApplyContinuesPastAStaleCluster(t *testing.T) {
	tc := SetupTestEnv(t)

	goodWinner := addWithHash(t, tc, "good1.txt", "good1 body", "hash-good")
	goodLoser := addWithHash(t, tc, "good2.txt", "good2 body", "hash-good")
	staleWinner := addWithHash(t, tc, "stale1.txt", "stale1 body", "hash-stale")
	staleLoser := addWithHash(t, tc, "stale2.txt", "stale2 body", "hash-stale")
	setDimensions(t, tc, goodWinner.ID, 400, 400)
	setDimensions(t, tc, staleWinner.ID, 400, 400)

	red := createReduction(t, tc, "Mixed", []uint{goodWinner.ID, goodLoser.ID, staleWinner.ID, staleLoser.ID})
	computeReduction(t, tc, red.ID)

	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", staleLoser.ID).
		Update("hash", "moved-on").Error)

	result := applyReduction(t, tc, red.ID)

	assert.Len(t, result.Applied, 1)
	assert.Len(t, result.Stale, 1)
	assert.False(t, resourceExists(t, tc, goodLoser.ID))
	assert.True(t, resourceExists(t, tc, staleLoser.ID))
}

// An ejected Resource is left completely untouched, which is what makes ejection
// a safe action — including at apply, where a change to one must not stale a
// Cluster it is no longer part of.
func TestApplyIgnoresAnEjectedMemberEntirely(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	ejected := addWithHash(t, tc, "ejected.txt", "ejected body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Ejected", []uint{winner.ID, loser.ID, ejected.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID

	override(t, tc, red.ID, clusterID, application_context.ReductionActionEject, ejected.ID)
	// The ejected Resource's bytes change. It is out of the Cluster, so this must
	// not refuse the merge of the rest.
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", ejected.ID).
		Update("hash", "changed-after-ejection").Error)

	result := applyReduction(t, tc, red.ID)

	require.Len(t, result.Applied, 1)
	assert.Equal(t, []uint{loser.ID}, result.Applied[0].LoserIDs)
	assert.True(t, resourceExists(t, tc, ejected.ID), "an ejected Resource is never destroyed")
	assert.False(t, resourceExists(t, tc, loser.ID))
}

// A Near-Identical Cluster whose Winner-to-Loser pair no longer holds is refused:
// the reviewer approved a match that is not there any more.
func TestApplyRefusesANearClusterWhosePairIsGone(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addImage(t, tc, "winner.jpg", 800, 800)
	loser := addImage(t, tc, "loser.jpg", 400, 400)
	pairThem(t, tc, winner, loser, 3)

	red := createReduction(t, tc, "Pair gone", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID
	override(t, tc, red.ID, clusterID, application_context.ReductionActionCheck, 0)

	require.NoError(t, tc.DB.Where("resource_id1 = ? OR resource_id2 = ?", winner.ID, winner.ID).
		Delete(&models.ResourceSimilarity{}).Error)

	result := applyReduction(t, tc, red.ID)

	assert.Empty(t, result.Applied)
	require.Len(t, result.Stale, 1)
	assert.Contains(t, result.Stale[0].Reason, "perceptual match")
	assert.True(t, resourceExists(t, tc, loser.ID))
}

// keepAsVersion is per tier, and applying has to read the right one. A
// Near-Identical Loser's file becomes a version of the Winner so there is a way
// back to pixels the reviewer decided against; a byte-identical one has nothing to
// preserve.
func TestApplyUsesThePerTierKeepAsVersionFlag(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addImage(t, tc, "winner.jpg", 800, 800)
	loser := addImage(t, tc, "loser.jpg", 400, 400)
	pairThem(t, tc, winner, loser, 3)

	red := createReduction(t, tc, "Versions", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Equal(t, models.ReductionTierNear, plan.Clusters[0].Tier)

	before := versionCount(t, tc, winner.ID)
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionCheck, 0)
	applyReduction(t, tc, red.ID)

	after := versionCount(t, tc, winner.ID)
	assert.Greater(t, after, before+1,
		"the Loser's own versions transfer, and keepAsVersion adds a further one from its resource-level file")
}

// A merge writes no backup of a file it is not removing. Storage is
// content-addressed, so for the Identical tier the Winner and the Loser are the
// same file — copying it into a directory with no readers and no retention sweep
// would be pure waste. This is the property the merge fix landed for, tied to the
// feature that found it.
func TestApplyingAnIdenticalClusterWritesNoBackup(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)
	require.Empty(t, deletedFiles(t, tc.Fs), "precondition: nothing in /deleted yet")

	red := createReduction(t, tc, "No backup", []uint{winner.ID, loser.ID})
	computeReduction(t, tc, red.ID)
	applyReduction(t, tc, red.ID)

	assert.Empty(t, deletedFiles(t, tc.Fs),
		"the Loser's hash is still referenced, so its file stays and needs no backup")
}

// A Winner that survived an applied Cluster goes back into the pool as an ordinary
// candidate. What freezing protects is the judgement, not the Winner's future
// eligibility — a duplicate arriving next month has to be catchable.
func TestAnAppliedClustersWinnerCanBeClusteredAgain(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Reusable", []uint{winner.ID, loser.ID})
	computeReduction(t, tc, red.ID)
	applyReduction(t, tc, red.ID)

	// Next month, another copy of the same content arrives.
	newcomer := addWithHash(t, tc, "newcomer.txt", "newcomer body", "shared-hash")
	_, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID:          red.ID,
		ResourceIds: []uint{newcomer.ID},
	}, owner, restricted)
	require.NoError(t, err)

	plan := computeReduction(t, tc, red.ID)

	fresh := clusterFor(plan, newcomer.ID)
	require.NotNil(t, fresh, "the newcomer clusters")
	assert.Contains(t, memberIDs(fresh), winner.ID, "with the surviving Winner of the applied Cluster")
	assert.Equal(t, models.ReductionClusterOpen, fresh.State)
}

// An apply from a page loaded before something else wrote the plan is refused
// before anything is destroyed.
func TestAStaleApplyIsRefusedBeforeAnythingIsDestroyed(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Stale apply", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	staleVersion := current.Version

	// Something else writes the plan.
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionUncheck, 0)

	_, err = tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: red.ID, Version: staleVersion,
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	assert.True(t, resourceExists(t, tc, loser.ID))
}

func versionCount(t *testing.T, tc *TestContext, resourceID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", resourceID).Count(&count).Error)
	return count
}

// The merge itself refuses content that changed, not just the check before it.
//
// revalidateCluster runs outside the merge's transaction, so a version upload
// landing between the two would have the merge delete bytes nobody reviewed. The
// expected hashes travel into the transaction that does the deleting.
func TestTheMergeRefusesContentThatChangedUnderIt(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")

	err := tc.AppCtx.MergeResourcesExpecting(winner.ID, []uint{loser.ID}, false, map[uint]string{
		winner.ID: "shared-hash",
		loser.ID:  "what-it-used-to-be",
	})
	require.ErrorIs(t, err, application_context.ErrMergeContentChanged)
	assert.True(t, resourceExists(t, tc, loser.ID), "the transaction rolled back before deleting anything")

	// The same call with the hashes the rows actually hold goes through.
	require.NoError(t, tc.AppCtx.MergeResourcesExpecting(winner.ID, []uint{loser.ID}, false, map[uint]string{
		winner.ID: "shared-hash",
		loser.ID:  "shared-hash",
	}))
	assert.False(t, resourceExists(t, tc, loser.ID))
}

// A Cluster is claimed before it is merged, so an override landing between the
// batch reading it and the merge running cannot be missed. The claim is what makes
// every override on that Cluster refuse for the duration.
func TestApplyClaimsAClusterBeforeMergingIt(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)

	red := createReduction(t, tc, "Claimed", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID

	applyReduction(t, tc, red.ID)

	// The Cluster is settled, and an override on it is refused rather than
	// rewriting a judgement whose Losers no longer exist.
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID:        red.ID,
		Version:   current.Version,
		ClusterID: clusterID,
		Action:    application_context.ReductionActionUncheck,
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionClusterSettled)
}
