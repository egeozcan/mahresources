package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// override runs one review decision and hands back the fresh plan.
func override(t *testing.T, tc *TestContext, reductionID uint, clusterID, action string, resourceID uint) models.ResourceReductionPlan {
	t.Helper()
	owner, restricted := asAdmin()
	current, err := tc.AppCtx.GetResourceReduction(reductionID, owner, restricted)
	require.NoError(t, err)

	updated, err := tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID:         reductionID,
		Version:    current.Version,
		ClusterID:  clusterID,
		Action:     action,
		ResourceID: resourceID,
	}, owner, restricted)
	require.NoError(t, err)
	plan, err := application_context.DecodeReductionPlan(updated.Plan)
	require.NoError(t, err)
	return plan
}

func overrideErr(t *testing.T, tc *TestContext, reductionID uint, clusterID, action string, resourceID uint) error {
	t.Helper()
	owner, restricted := asAdmin()
	current, err := tc.AppCtx.GetResourceReduction(reductionID, owner, restricted)
	require.NoError(t, err)

	_, err = tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID:         reductionID,
		Version:    current.Version,
		ClusterID:  clusterID,
		Action:     action,
		ResourceID: resourceID,
	}, owner, restricted)
	return err
}

func memberOf(cluster *models.ReductionCluster, resourceID uint) *models.ReductionMember {
	for _, m := range cluster.Members {
		if m.ResourceID == resourceID {
			return m
		}
	}
	return nil
}

// The reviewer's judgement beats the rule when the rule is wrong.
func TestPromotingAMemberMakesItTheWinner(t *testing.T) {
	tc := SetupTestEnv(t)

	big := addWithHash(t, tc, "big.txt", "big body", "shared-hash")
	small := addWithHash(t, tc, "small.txt", "small body", "shared-hash")
	setDimensions(t, tc, big.ID, 400, 400)
	setDimensions(t, tc, small.ID, 100, 100)

	red := createReduction(t, tc, "Promote", []uint{big.ID, small.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Equal(t, big.ID, plan.Clusters[0].WinnerID)

	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, small.ID)

	cluster := plan.Clusters[0]
	assert.Equal(t, small.ID, cluster.WinnerID)
	assert.Equal(t, []uint{big.ID}, cluster.LoserIDs())
	assert.True(t, cluster.Reviewed, "an explicit decision freezes the Cluster against re-clustering")
}

// Promotion must never quietly widen what gets deleted. ADR 0002's guarantee —
// every proposed deletion rests on a stored pair between that exact Loser and that
// exact Winner — holds only while the Winner is the seed, so a Loser with no pair
// to the new Winner is ejected and shown as ejected.
func TestPromotingEjectsALoserWithNoPairToTheNewWinner(t *testing.T) {
	tc := SetupTestEnv(t)

	hub := addImage(t, tc, "hub.jpg", 800, 800)
	paired := addImage(t, tc, "paired.jpg", 400, 400)
	unpaired := addImage(t, tc, "unpaired.jpg", 200, 200)
	pairThem(t, tc, hub, paired, 3)
	pairThem(t, tc, hub, unpaired, 3)
	// paired and unpaired have no stored pair to each other.

	red := createReduction(t, tc, "Rejustify", []uint{hub.ID, paired.ID, unpaired.ID})
	plan := computeReduction(t, tc, red.ID)
	cluster := plan.Clusters[0]
	require.Equal(t, hub.ID, cluster.WinnerID)
	require.Len(t, cluster.LoserIDs(), 2)

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionPromote, paired.ID)

	cluster = plan.Clusters[0]
	assert.Equal(t, paired.ID, cluster.WinnerID)

	ejected := memberOf(cluster, unpaired.ID)
	require.NotNil(t, ejected)
	assert.True(t, ejected.Ejected, "no stored pair to the new Winner")
	assert.Equal(t, models.EjectReasonNoPairToWinner, ejected.EjectedReason)
	assert.NotContains(t, cluster.LoserIDs(), unpaired.ID)

	stillALoser := memberOf(cluster, hub.ID)
	require.NotNil(t, stillALoser)
	require.NotNil(t, stillALoser.Distance, "and the one that keeps its place records its distance")
}

// Hash equality is transitive, so an Identical Cluster is a true equivalence class
// and any member is as good a Winner as any other. Nothing is ejected.
func TestPromotingInAnIdenticalClusterEjectsNothing(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	c := addWithHash(t, tc, "c.txt", "c body", "shared-hash")
	setDimensions(t, tc, a.ID, 400, 400)

	red := createReduction(t, tc, "Equivalence", []uint{a.ID, b.ID, c.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Equal(t, a.ID, plan.Clusters[0].WinnerID)

	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, c.ID)

	cluster := plan.Clusters[0]
	assert.Equal(t, c.ID, cluster.WinnerID)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, cluster.LoserIDs())
}

// A Resource outside the Extent may win and may never lose, so a promotion that
// would demote it is refused rather than silently allowed.
func TestPromotingPastAnOutOfExtentWinnerIsRefused(t *testing.T) {
	tc := SetupTestEnv(t)

	inside := addWithHash(t, tc, "inside.txt", "inside body", "shared-hash")
	outside := addWithHash(t, tc, "outside.txt", "outside body", "shared-hash")
	setDimensions(t, tc, inside.ID, 100, 100)
	setDimensions(t, tc, outside.ID, 800, 800)

	red := createReduction(t, tc, "Refused promote", []uint{inside.ID})
	plan := computeReduction(t, tc, red.ID)
	cluster := plan.Clusters[0]
	require.Equal(t, outside.ID, cluster.WinnerID)

	err := overrideErr(t, tc, red.ID, cluster.ID, application_context.ReductionActionPromote, inside.ID)
	assert.ErrorIs(t, err, application_context.ErrReductionWouldDemoteOutsider)
}

// Ejection is a safe action, which is what makes it usable freely: the Resource is
// left completely untouched and simply leaves the Cluster.
func TestEjectingAMemberLeavesItOutOfTheLosers(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	c := addWithHash(t, tc, "c.txt", "c body", "shared-hash")
	setDimensions(t, tc, a.ID, 400, 400)

	red := createReduction(t, tc, "Eject", []uint{a.ID, b.ID, c.ID})
	plan := computeReduction(t, tc, red.ID)
	cluster := plan.Clusters[0]

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionEject, c.ID)
	cluster = plan.Clusters[0]

	assert.Equal(t, []uint{b.ID}, cluster.LoserIDs())
	member := memberOf(cluster, c.ID)
	require.NotNil(t, member)
	assert.True(t, member.Ejected)
	assert.Equal(t, models.EjectReasonManual, member.EjectedReason)

	// And it goes back.
	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionRestore, c.ID)
	assert.ElementsMatch(t, []uint{b.ID, c.ID}, plan.Clusters[0].LoserIDs())
}

// Ejecting the Winner would leave the Cluster with nothing to merge into, so it is
// refused with the instruction that resolves it.
func TestEjectingTheWinnerIsRefused(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	setDimensions(t, tc, a.ID, 400, 400)

	red := createReduction(t, tc, "Eject winner", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	err := overrideErr(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, a.ID)
	assert.ErrorIs(t, err, application_context.ErrReductionEjectWinner)
}

// Skipping moves past a Cluster without deciding about it, and unchecks it.
func TestSkippingAClusterUnchecksAndFreezesIt(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red := createReduction(t, tc, "Skip", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)
	require.True(t, plan.Clusters[0].Checked)

	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionSkip, 0)

	cluster := plan.Clusters[0]
	assert.Equal(t, models.ReductionClusterSkipped, cluster.State)
	assert.False(t, cluster.Checked)
	assert.True(t, cluster.Frozen())
}

// A judgement already made is never rearranged by a later compute. Growing the
// Extent can add Clusters; it cannot move a Resource out of one the reviewer has
// acted on.
func TestARecomputeCarriesForwardAJudgementAlreadyMade(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	setDimensions(t, tc, a.ID, 100, 100)
	setDimensions(t, tc, b.ID, 200, 200)

	red := createReduction(t, tc, "Frozen", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID
	require.Equal(t, b.ID, plan.Clusters[0].WinnerID)

	override(t, tc, red.ID, clusterID, application_context.ReductionActionPromote, a.ID)

	// A third copy, bigger than both, arrives and the Extent is widened. Without
	// the freeze it would take the Cluster's Winner.
	c := addWithHash(t, tc, "c.txt", "c body", "shared-hash")
	setDimensions(t, tc, c.ID, 900, 900)
	owner, restricted := asAdmin()
	_, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID:          red.ID,
		ResourceIds: []uint{c.ID},
	}, owner, restricted)
	require.NoError(t, err)

	plan = computeReduction(t, tc, red.ID)

	frozen := findClusterByID(plan, clusterID)
	require.NotNil(t, frozen, "the reviewed Cluster survives the recompute")
	assert.Equal(t, a.ID, frozen.WinnerID, "and keeps the Winner the reviewer chose")
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, memberIDs(frozen))

	assert.Nil(t, clusterFor(plan, c.ID), "a frozen Cluster's members are held out of the pool, so nothing re-forms around them")
}

// An explicit check is a decision too, so it freezes — but a Cluster that merely
// arrived checked by default has not been acted on and stays re-clusterable.
func TestArrivingCheckedIsNotAJudgement(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red := createReduction(t, tc, "Default checked", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	cluster := plan.Clusters[0]
	assert.True(t, cluster.Checked)
	assert.False(t, cluster.Reviewed)
	assert.False(t, cluster.Frozen())

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionUncheck, 0)
	assert.False(t, plan.Clusters[0].Checked)
	assert.True(t, plan.Clusters[0].Reviewed, "unchecking is an act")
	assert.True(t, plan.Clusters[0].Frozen())
}

// An override taken from a page loaded before something else wrote the plan is
// refused rather than merged.
func TestAStaleOverrideIsRefused(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red := createReduction(t, tc, "Conflict", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)
	clusterID := plan.Clusters[0].ID

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	staleVersion := current.Version

	override(t, tc, red.ID, clusterID, application_context.ReductionActionUncheck, 0)

	_, err = tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID:        red.ID,
		Version:   staleVersion,
		ClusterID: clusterID,
		Action:    application_context.ReductionActionSkip,
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
}

func findClusterByID(plan models.ResourceReductionPlan, id string) *models.ReductionCluster {
	for _, cluster := range plan.Clusters {
		if cluster.ID == id {
			return cluster
		}
	}
	return nil
}
