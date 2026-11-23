package api_tests

import (
	"encoding/json"
	"mahresources/contracts"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// reductionScopeFixture is two nested Groups and a Reduction computed by a reviewer who
// could see both, so a test can then move a Resource out of the narrow one and read
// the Reduction as the narrow reviewer.
type reductionScopeFixture struct {
	tc           *TestContext
	wide, narrow *models.Group
	wideCtx      *application_context.MahresourcesContext
	narrowCtx    *application_context.MahresourcesContext
	red          *models.ResourceReduction
}

func newReductionScopeFixture(t *testing.T, tc *TestContext) *reductionScopeFixture {
	t.Helper()
	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)
	return &reductionScopeFixture{tc: tc, wide: wide, narrow: narrow,
		wideCtx: scopedTo(t, tc, wide.ID), narrowCtx: scopedTo(t, tc, narrow.ID)}
}

// compute creates and computes a Reduction over ids as the wide reviewer.
func (f *reductionScopeFixture) compute(t *testing.T, ids []uint) models.ResourceReductionPlan {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := f.wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Scoped fixture", ResourceIds: ids,
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := f.wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = f.wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	f.red = red
	return awaitReduction(t, f.tc, red.ID)
}

// place puts Resources into the narrow or the wide-only Group.
func (f *reductionScopeFixture) place(t *testing.T, group *models.Group, ids ...uint) {
	t.Helper()
	require.NoError(t, f.tc.DB.Model(&models.Resource{}).Where("id IN ?", ids).Update("owner_id", group.ID).Error)
}

func (f *reductionScopeFixture) narrowApply(t *testing.T, expectedClusters, expectedLosers *uint) (resultErr error, applied, stale int) {
	t.Helper()
	owner, restricted := asAdmin()
	current, err := f.narrowCtx.GetResourceReduction(f.red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := f.narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: f.red.ID, Version: current.Version, ExpectedClusters: expectedClusters, ExpectedLosers: expectedLosers,
	}, owner, restricted)
	if err != nil {
		return err, 0, 0
	}
	return nil, len(result.Applied), len(result.Stale)
}

// P0-3. A Cluster with a hidden *ejected* member is withheld from the review
// tabs and from bulk approval, so it must be left out of the apply figures and out
// of what Apply merges as well: one full-Cluster visibility rule.
func TestAClusterWithAHiddenEjectedMemberIsNotCountedOrApplied(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	owner, restricted := asAdmin()

	ids := clusterOf(t, tc, "p03-hash", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	setDimensions(t, tc, ids[1], 400, 400)
	setDimensions(t, tc, ids[2], 100, 100)
	f.place(t, f.narrow, ids[0], ids[1])
	f.place(t, f.wide, ids[2])
	plan := f.compute(t, ids)
	cluster := clusterFor(plan, ids[0])
	require.Equal(t, ids[0], cluster.WinnerID)

	// The wide reviewer ejects the member that is about to leave the narrow one.
	current, err := f.wideCtx.GetResourceReduction(f.red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = f.wideCtx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID: f.red.ID, Version: current.Version, ClusterID: cluster.ID,
		Action: application_context.ReductionActionEject, ResourceID: ids[2],
	}, owner, restricted)
	require.NoError(t, err)

	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, review.TabCounts.Open, "the tabs treat it as withheld")
	assert.Equal(t, 0, review.ApplyFigures.Identical.Clusters, "so must the apply figures")
	assert.Equal(t, 0, review.ApplyFigures.Identical.Losers)
	assert.Equal(t, 0, review.CheckedCount)
	assert.Equal(t, 0, review.CheckedLoserCount)

	err, applied, stale := f.narrowApply(t, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, applied)
	assert.Zero(t, stale)
	assert.True(t, resourceExists(t, tc, ids[1]), "its other Loser was not merged")
	after := findClusterByID(planOf(t, tc, f.red.ID), cluster.ID)
	assert.Equal(t, models.ReductionClusterOpen, after.State)
	assert.True(t, after.Checked)
}

// P0-4. A withheld Cluster is left completely untouched and omitted from the
// response, whatever order its members are checked in: an earlier visible member
// whose hash changed must not make the apply name it, and a hidden active member
// must not produce a stale entry or a persisted state change.
func TestApplyLeavesAWithheldClusterUntouchedAndUnreported(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)

	ids := clusterOf(t, tc, "p04-hash", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	setDimensions(t, tc, ids[1], 400, 400)
	setDimensions(t, tc, ids[2], 100, 100)
	f.place(t, f.narrow, ids[0], ids[1])
	f.place(t, f.wide, ids[2])
	plan := f.compute(t, ids)
	cluster := clusterFor(plan, ids[0])

	// A visible member's bytes changed: the hash check would trip first.
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", ids[1]).Update("hash", "changed").Error)

	err, applied, stale := f.narrowApply(t, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, applied)
	assert.Zero(t, stale, "a withheld Cluster is omitted from the response, not reported stale")

	after := findClusterByID(planOf(t, tc, f.red.ID), cluster.ID)
	assert.Equal(t, models.ReductionClusterOpen, after.State, "no persisted state change")
	assert.True(t, after.Checked)
	assert.Empty(t, after.StaleReason)
}

// P0-2. The apply dialog confirms figures over what the reviewer could see when it
// opened. A Cluster that was withheld then and became visible before Apply was not
// counted, so an apply carrying those figures is refused and merges nothing.
func TestApplyRefusesWhenTheConfirmedFiguresNoLongerMatch(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)

	mine := clusterOf(t, tc, "p02-mine", 2)
	late := clusterOf(t, tc, "p02-late", 2)
	f.place(t, f.narrow, mine...)
	f.place(t, f.narrow, late[0])
	f.place(t, f.wide, late[1])
	f.compute(t, append(append([]uint{}, mine...), late...))

	// The dialog counted one Cluster and one copy: `late` was withheld.
	owner, restricted := asAdmin()
	seen, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, seen.ApplyFigures.Identical.Clusters)
	require.Equal(t, 1, seen.ApplyFigures.Identical.Losers)

	// Before Apply, `late` comes into their subtree.
	f.place(t, f.narrow, late[1])

	err, applied, _ := f.narrowApply(t, uintPtr(1), uintPtr(1))
	assert.ErrorIs(t, err, application_context.ErrReductionApplyFiguresChanged)
	assert.Zero(t, applied)
	assert.True(t, resourceExists(t, tc, mine[0]) && resourceExists(t, tc, mine[1]), "nothing was merged")
	assert.True(t, resourceExists(t, tc, late[0]) && resourceExists(t, tc, late[1]))
	for _, cluster := range planOf(t, tc, f.red.ID).Clusters {
		assert.Equal(t, models.ReductionClusterOpen, cluster.State)
	}
	assert.Nil(t, planOf(t, tc, f.red.ID).LastApply, "a refused apply does not even take the claim")

	// With the figures it now sees, it goes through.
	err, applied, _ = f.narrowApply(t, uintPtr(2), uintPtr(2))
	require.NoError(t, err)
	assert.Equal(t, 2, applied)

	// And the HTTP route answers 409 for a mismatch.
	red2 := createReduction(t, tc, "Route figures", clusterOf(t, tc, "p02-route", 2))
	computeReduction(t, tc, red2.ID)
	rec := tc.MakeRequest("POST", "/v1/reduction/apply", map[string]any{
		"id": red2.ID, "version": versionOf(t, tc, red2.ID), "expectedClusters": 5, "expectedLosers": 5,
	})
	assert.Equal(t, 409, rec.Code)
}

// P1-7. A second apply that read a version after the first one's claim must not
// overwrite a running record or run beside it. Past the ceiling the record is an
// interrupted one and a new apply proceeds.
func TestASecondApplyIsRefusedWhileOneIsRunning(t *testing.T) {
	tc := SetupTestEnv(t)
	red := createReduction(t, tc, "Running", clusterOf(t, tc, "p17-hash", 2))
	plan := computeReduction(t, tc, red.ID)
	owner, restricted := asAdmin()

	running := plan
	running.LastApply = &models.ReductionLastApply{StartedAt: time.Now()}
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, running))

	// A fresh version, read after the "first" claim.
	current, err = tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	assert.True(t, resourceExists(t, tc, plan.Clusters[0].Members[0].ResourceID) && resourceExists(t, tc, plan.Clusters[0].Members[1].ResourceID))
	assert.True(t, planOf(t, tc, red.ID).LastApply.StartedAt.Equal(running.LastApply.StartedAt), "the running record was not overwritten")

	// Interrupted: running for longer than the ceiling.
	stale := plan
	stale.LastApply = &models.ReductionLastApply{StartedAt: time.Now().Add(-2 * application_context.ReductionApplyCeiling)}
	current, err = tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, stale))
	result := applyReduction(t, tc, red.ID)
	assert.Len(t, result.Applied, 1)
}

// P1-8. The ceiling stops the batch starting new Clusters, and the Cluster in
// flight when it passes is settled rather than stranded as applied over Losers
// that still exist. Clusters not reached stay approved and open, not stale.
func TestTheCeilingSettlesTheInFlightClusterAndLeavesTheRestApproved(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "p18-a", 2)
	b := clusterOf(t, tc, "p18-b", 2)
	red := createReduction(t, tc, "Ceiling", append(append([]uint{}, a...), b...))
	computeReduction(t, tc, red.ID)

	saved := application_context.ReductionApplyCeiling
	application_context.ReductionApplyCeiling = time.Second
	defer func() { application_context.ReductionApplyCeiling = saved }()
	calls := 0
	restore := application_context.SetReductionApplyAfterClaimHookForTest(func() {
		calls++
		if calls == 1 {
			time.Sleep(1300 * time.Millisecond)
		}
	})
	defer restore()

	result := applyReduction(t, tc, red.ID)
	assert.Equal(t, 1, calls, "no second Cluster was started past the deadline")
	assert.Empty(t, result.Applied)

	after := planOf(t, tc, red.ID)
	states := map[string]int{}
	for _, cluster := range after.Clusters {
		states[cluster.State]++
		if cluster.State == models.ReductionClusterOpen {
			assert.True(t, cluster.Checked, "the Cluster not reached is still approved")
		}
		assert.NotEqual(t, models.ReductionClusterApplied, cluster.State, "nothing is left claimed over live Losers")
	}
	assert.Equal(t, 1, states[models.ReductionClusterOpen])
	assert.Equal(t, 1, states[models.ReductionClusterStale], "the in-flight Cluster was settled")
	require.NotNil(t, after.LastApply.FinishedAt, "and the record says the batch finished")
	assert.True(t, resourceExists(t, tc, a[0]) && resourceExists(t, tc, a[1]) && resourceExists(t, tc, b[0]) && resourceExists(t, tc, b[1]))
}

// P0-5. lastApply holds counts of what an apply did. A viewer whose subtree has
// narrowed since must not read counts of Clusters now outside their access; a viewer
// who can still see every Cluster counted keeps them (they are how a lost response
// is answered on reload).
func TestLastApplyCountsAreHiddenFromAViewerWhoLostAccess(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	owner, restricted := asAdmin()

	kept := clusterOf(t, tc, "p05-kept", 2)
	left := clusterOf(t, tc, "p05-left", 2)
	setDimensions(t, tc, kept[0], 900, 900)
	setDimensions(t, tc, left[0], 900, 900)
	f.place(t, f.narrow, kept...)
	f.place(t, f.narrow, left...)
	f.compute(t, append(append([]uint{}, kept...), left...))

	current, err := f.narrowCtx.GetResourceReduction(f.red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = f.narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: f.red.ID, Version: current.Version}, owner, restricted)
	require.NoError(t, err)

	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, review.LastApply)
	assert.Equal(t, 2, review.LastApply.Applied, "everything counted is still visible, so the numbers show")
	assert.Equal(t, 2, review.LastApply.Removed)
	assert.False(t, review.LastApply.CountsHidden)

	// Their access narrows: one applied Cluster's Winner is now outside it.
	f.place(t, f.wide, left[0])
	review, err = f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	require.NotNil(t, review.LastApply, "the record itself still shows")
	assert.False(t, review.LastApply.StartedAt.IsZero())
	assert.Zero(t, review.LastApply.Applied)
	assert.Zero(t, review.LastApply.Removed)
	assert.Zero(t, review.LastApply.Stale)
	assert.True(t, review.LastApply.CountsHidden)

	// An unrestricted viewer is always shown the numbers.
	full, err := tc.AppCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, full.LastApply.Applied)
	assert.False(t, full.LastApply.CountsHidden)
}

// P1-11a. The keeps-smaller suppression follows the rule the Cluster's proposal was
// computed under, not whatever rule the row holds now: changing the rule and then
// ejecting an unrelated member must not turn a warning on for a proposal that was
// asked for.
func TestKeepsSmallerSuppressionFollowsTheRuleThePlanWasComputedUnder(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	small := addWithHash(t, tc, "small.txt", "small body", "p111-hash")
	big := addWithHash(t, tc, "big.txt", "big body", "p111-hash")
	mid := addWithHash(t, tc, "mid.txt", "mid body", "p111-hash")
	setDimensions(t, tc, small.ID, 400, 400)
	setDimensions(t, tc, big.ID, 1200, 1200)
	setDimensions(t, tc, mid.ID, 800, 800)

	red := createReductionWithRule(t, tc, "Asked for the smallest", []uint{small.ID, big.ID, mid.ID},
		[]string{models.WinnerCriterionPixelsAsc})
	plan := computeReduction(t, tc, red.ID)
	require.Equal(t, small.ID, plan.Clusters[0].WinnerID)
	require.False(t, plan.Clusters[0].KeepsSmaller, "the rule asked for the lowest resolution")

	// The rule is changed afterwards; the plan is not recomputed.
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: red.ID, Version: current.Version, WinnerRule: []string{models.WinnerCriterionPixelsDesc},
	}, owner, restricted)
	require.NoError(t, err)

	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, mid.ID)
	assert.False(t, plan.Clusters[0].KeepsSmaller,
		"the proposal is still the one the old rule computed, so its warning stays suppressed")
}

// P1-11b. A plan stored before the flag existed has no KeepsSmaller on a Cluster
// whose frozen manual promotion keeps a smaller copy. The review derives it at read
// time, so the card, the figures, the filter and bulk approval all see it.
func TestKeepsSmallerIsDerivedForAPlanStoredBeforeTheFlag(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	big := addWithHash(t, tc, "big.txt", "big body", "p112-hash")
	small := addWithHash(t, tc, "small.txt", "small body", "p112-hash")
	setDimensions(t, tc, big.ID, 1600, 1066)
	setDimensions(t, tc, small.ID, 400, 266)

	red := createReduction(t, tc, "Legacy", []uint{big.ID, small.ID})
	plan := computeReduction(t, tc, red.ID)
	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, small.ID)
	require.True(t, plan.Clusters[0].KeepsSmaller)
	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionCheck, 0)

	// Rewrite the stored plan as an older release left it: no flag, no settings.
	legacy := plan
	legacy.Settings = nil
	legacy.Clusters[0].KeepsSmaller = false
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, legacy))

	review := reviewOf(t, tc, red.ID, 1, nil)
	require.Len(t, review.Clusters, 1)
	assert.True(t, review.Clusters[0].KeepsSmaller, "derived at read time")
	assert.True(t, review.Clusters[0].NeedsAttention)
	assert.Equal(t, 1, review.ApplyFigures.KeepsSmaller)
	assert.Equal(t, 0, review.Bulk.IdenticalApprovable, "it needs attention, so the bulk button skips it")
	filtered := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{NeedsAttention: true})
	assert.Len(t, filtered.Clusters, 1)

	// Bulk approval skips it too.
	require.NoError(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "uncheck", ClusterIDs: []string{legacy.Clusters[0].ID},
	}))
	result, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "check", ClusterIDs: []string{legacy.Clusters[0].ID},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, 1, result.NeedsReview)
	assert.False(t, findClusterByID(planOf(t, tc, red.ID), legacy.Clusters[0].ID).Checked)
}

// Round 2, P0. Equal totals can stand for a different merge: one approved pair
// leaves the reviewer's subtree and another enters. The token binds the apply to the
// exact proposals the dialog showed.
func TestApplyTokenBindsTheExactProposalsNotTheTotals(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	owner, restricted := asAdmin()

	p1 := clusterOf(t, tc, "r2-token-1", 2)
	p2 := clusterOf(t, tc, "r2-token-2", 2)
	f.place(t, f.narrow, p1...)
	f.place(t, f.narrow, p2[0])
	f.place(t, f.wide, p2[1])
	f.compute(t, append(append([]uint{}, p1...), p2...))

	seen, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	require.Len(t, seen.ApplyToken, 32)
	require.Equal(t, 1, seen.ApplyFigures.Identical.Clusters)

	// p1 leaves, p2 arrives: still one Cluster and one copy.
	f.place(t, f.wide, p1[1])
	f.place(t, f.narrow, p2[1])
	now, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, now.ApplyFigures.Identical.Clusters)
	require.Equal(t, 1, now.ApplyFigures.Identical.Losers)
	assert.NotEqual(t, seen.ApplyToken, now.ApplyToken)

	current, err := f.narrowCtx.GetResourceReduction(f.red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = f.narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: f.red.ID, Version: current.Version, ApplyToken: seen.ApplyToken,
		ExpectedClusters: uintPtr(1), ExpectedLosers: uintPtr(1),
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionApplyFiguresChanged)
	assert.True(t, resourceExists(t, tc, p2[0]) && resourceExists(t, tc, p2[1]), "nothing was merged")
	assert.Nil(t, planOf(t, tc, f.red.ID).LastApply, "and nothing was claimed")

	result, err := f.narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: f.red.ID, Version: current.Version, ApplyToken: now.ApplyToken, RequestID: "req-123",
	}, owner, restricted)
	require.NoError(t, err)
	assert.Len(t, result.Applied, 1)
	assert.Equal(t, "req-123", planOf(t, tc, f.red.ID).LastApply.RequestID, "the client's id is recorded")
	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, "req-123", review.LastApply.RequestID)

	_, err = f.narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: f.red.ID, Version: versionOf(t, tc, f.red.ID), RequestID: string(make([]byte, 65)),
	}, owner, restricted)
	assert.Error(t, err, "the id is bounded")
}

// Round 2, P0. A member (an ejected one included) leaving the caller's access after
// the claim stops the merge, hands the claim back, and reports nothing.
func TestAMemberLeavingAccessAfterTheClaimReleasesTheClaimSilently(t *testing.T) {
	for _, name := range []string{"ejected", "active"} {
		t.Run(name, func(t *testing.T) {
			tc := SetupTestEnv(t)
			f := newReductionScopeFixture(t, tc)
			owner, restricted := asAdmin()

			ids := clusterOf(t, tc, "r2-claim-"+name, 3)
			setDimensions(t, tc, ids[0], 900, 900)
			setDimensions(t, tc, ids[1], 400, 400)
			setDimensions(t, tc, ids[2], 100, 100)
			f.place(t, f.narrow, ids...)
			plan := f.compute(t, ids)
			cluster := plan.Clusters[0]
			current, err := f.wideCtx.GetResourceReduction(f.red.ID, owner, restricted)
			require.NoError(t, err)
			_, err = f.wideCtx.OverrideReductionCluster(&query_models.ReductionOverride{
				ID: f.red.ID, Version: current.Version, ClusterID: cluster.ID,
				Action: application_context.ReductionActionEject, ResourceID: ids[2],
			}, owner, restricted)
			require.NoError(t, err)

			leaving := ids[2]
			if name == "active" {
				leaving = ids[1]
			}
			restore := application_context.SetReductionApplyAfterClaimHookForTest(func() { f.place(t, f.wide, leaving) })
			defer restore()

			err, applied, stale := f.narrowApply(t, nil, nil)
			require.NoError(t, err)
			assert.Zero(t, applied)
			assert.Zero(t, stale, "no outcome, so no ids")
			assert.True(t, resourceExists(t, tc, ids[1]))
			after := findClusterByID(planOf(t, tc, f.red.ID), cluster.ID)
			assert.Equal(t, models.ReductionClusterOpen, after.State, "the claim was released")
			assert.True(t, after.Checked)
			assert.False(t, after.Merged)
		})
	}
}

// Round 2, P0. A withheld Cluster is out before anything is counted or paginated,
// whatever the query, nil included.
func TestAReviewWithNoQueryHasNoWithheldStubs(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r2-nostub", 2)
	f.place(t, f.narrow, ids[0])
	f.place(t, f.wide, ids[1])
	f.compute(t, ids)
	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, review.Clusters)
	assert.Zero(t, review.ClusterCount)
}

// Round 2, P1. A Cluster with nothing left to remove cannot be approved again.
func TestAClusterWithNothingToRemoveCannotBeApproved(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r2-empty", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Empty", ids)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID
	override(t, tc, red.ID, id, application_context.ReductionActionEject, ids[1])

	assert.ErrorIs(t, overrideErr(t, tc, red.ID, id, application_context.ReductionActionCheck, 0), application_context.ErrReductionNothingToRemove)
	result, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "check", ClusterIDs: []string{id},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Zero(t, result.Changed)
	assert.False(t, findClusterByID(planOf(t, tc, red.ID), id).Checked)

	// Even handed to apply already approved, it is not eligible and not reported.
	forced := planOf(t, tc, red.ID)
	forced.Clusters[0].Checked = true
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, forced))
	review := reviewOf(t, tc, red.ID, 1, nil)
	assert.Zero(t, review.ApplyFigures.Identical.Clusters)
	applied := applyReduction(t, tc, red.ID)
	assert.Empty(t, applied.Applied)
	assert.Empty(t, applied.Stale)
}

// Round 2, P1. A deleted ejected copy does not block unrelated decisions for an
// unrestricted reviewer.
func TestADeletedEjectedCopyDoesNotBlockADecision(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r2-ejdel", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	other := clusterOf(t, tc, "r2-ejdel-other", 2)
	red := createReduction(t, tc, "Ejected deleted", append(append([]uint{}, ids...), other...))
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, clusterFor(plan, ids[0]).ID, application_context.ReductionActionEject, ids[2])
	require.NoError(t, tc.DB.Where("id = ?", ids[2]).Delete(&models.Resource{}).Error)

	result, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "uncheck",
		ClusterIDs: []string{clusterFor(plan, ids[0]).ID, clusterFor(plan, other[0]).ID},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Changed)
}

// Round 2, P1. A recompute that carries a legacy frozen Cluster over keeps its
// keeps-smaller warning.
func TestRecomputeKeepsTheLegacyKeepsSmallerWarning(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	big := addWithHash(t, tc, "big.txt", "big body", "r2-legacy")
	small := addWithHash(t, tc, "small.txt", "small body", "r2-legacy")
	setDimensions(t, tc, big.ID, 1600, 1066)
	setDimensions(t, tc, small.ID, 400, 266)
	red := createReduction(t, tc, "Legacy recompute", []uint{big.ID, small.ID})
	plan := computeReduction(t, tc, red.ID)
	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, small.ID)
	legacy := plan
	legacy.Settings = nil
	legacy.Clusters[0].KeepsSmaller = false
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, legacy))

	recomputed := computeReduction(t, tc, red.ID)
	require.NotNil(t, recomputed.Settings)
	assert.True(t, recomputed.Clusters[0].KeepsSmaller, "frozen and carried, with its warning")
}

// Round 3. A claim is not a completed merge until confirmed. An abandoned one is
// settled only inside the next apply's batch claim, from evidence the merge itself
// committed, and a page read never writes.
func TestAbandonedClaimsAreSettledByTheNextApply(t *testing.T) {
	type outcome struct {
		state           string
		merged, checked bool
	}
	cases := map[string]struct {
		prepare func(t *testing.T, tc *TestContext, winner, loser uint)
		want    outcome
	}{
		"losers still exist": {
			prepare: func(t *testing.T, tc *TestContext, winner, loser uint) {},
			want:    outcome{models.ReductionClusterOpen, false, true},
		},
		"merged by something else, not by this claim": {
			prepare: func(t *testing.T, tc *TestContext, winner, loser uint) {
				require.NoError(t, tc.AppCtx.MergeResources(winner, []uint{loser}, false))
			},
			want: outcome{models.ReductionClusterStale, false, false},
		},
		"loser deleted by something else": {
			prepare: func(t *testing.T, tc *TestContext, winner, loser uint) {
				require.NoError(t, tc.DB.Where("id = ?", loser).Delete(&models.Resource{}).Error)
			},
			want: outcome{models.ReductionClusterStale, false, false},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tc := SetupTestEnv(t)
			owner, restricted := asAdmin()
			ids := clusterOf(t, tc, "r3-claim", 2)
			setDimensions(t, tc, ids[0], 900, 900)
			red := createReduction(t, tc, "Claimed", ids)
			plan := computeReduction(t, tc, red.ID)
			cluster := plan.Clusters[0]
			claimed := tc.AppCtx.ClaimClusterForApplyForTest(red.ID, cluster.ID, cluster.WinnerID, map[uint]string{ids[0]: "r3-claim", ids[1]: "r3-claim"})
			require.NotNil(t, claimed)
			require.NotEmpty(t, claimed.ClaimID)

			review := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"all"}})
			assert.Equal(t, 0, review.TabCounts.Applied)
			assert.Equal(t, 1, review.TabCounts.Merging)
			assert.Equal(t, "Merging", review.Clusters[0].StateLabel)
			assert.Zero(t, listProgressOf(t, tc.AppCtx, red.ID).Applied)

			c.prepare(t, tc, ids[0], ids[1])

			// A page read, however late, writes nothing.
			before := versionOf(t, tc, red.ID)
			_, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1, nil)
			require.NoError(t, err)
			assert.Equal(t, before, versionOf(t, tc, red.ID))
			assert.Equal(t, models.ReductionClusterApplied, planOf(t, tc, red.ID).Clusters[0].State)

			applyReduction(t, tc, red.ID)
			after := planOf(t, tc, red.ID).Clusters[0]
			assert.Equal(t, c.want, outcome{after.State, after.Merged, after.Checked})
			if c.want.state == models.ReductionClusterStale {
				assert.Contains(t, after.StaleReason, "no longer exists")
			}
		})
	}
}

// A delayed settlement of an old claim must not touch a newer one.
func TestASettlementOfAnOldClaimDoesNotTouchANewerClaim(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r3-gen", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Generations", ids)
	cluster := computeReduction(t, tc, red.ID).Clusters[0]
	approved := map[uint]string{ids[0]: "r3-gen", ids[1]: "r3-gen"}

	first := tc.AppCtx.ClaimClusterForApplyForTest(red.ID, cluster.ID, cluster.WinnerID, approved)
	require.NotNil(t, first)
	applyReduction(t, tc, red.ID) // settles the abandoned first claim back to open, then applies nothing
	second := tc.AppCtx.ClaimClusterForApplyForTest(red.ID, cluster.ID, cluster.WinnerID, approved)
	require.NotNil(t, second)
	require.NotEqual(t, first.ClaimID, second.ClaimID)

	tc.AppCtx.ReleaseClaimForTest(red.ID, cluster.ID, first.ClaimID)
	after := planOf(t, tc, red.ID).Clusters[0]
	assert.Equal(t, models.ReductionClusterApplied, after.State, "the old claim's settlement found a different claim and did nothing")
	assert.Equal(t, second.ClaimID, after.ClaimID)
	assert.True(t, resourceExists(t, tc, ids[1]))
}

// A claimed Cluster with a Loser the viewer cannot see stays withheld, whatever
// happened to that Loser since.
func TestAClaimedClusterWithAHiddenLoserStaysWithheld(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r3-hidden-claim", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	f.place(t, f.narrow, ids[0])
	f.place(t, f.wide, ids[1])
	plan := f.compute(t, ids)
	cluster := plan.Clusters[0]
	require.NotNil(t, tc.AppCtx.ClaimClusterForApplyForTest(f.red.ID, cluster.ID, cluster.WinnerID, map[uint]string{ids[0]: "r3-hidden-claim", ids[1]: "r3-hidden-claim"}))
	require.NoError(t, tc.DB.Where("id = ?", ids[1]).Delete(&models.Resource{}).Error)

	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, review.Clusters)
	assert.Equal(t, contracts.ReductionTabCounts{}, review.TabCounts)
	body, err := json.Marshal(review.Clusters)
	require.NoError(t, err)
	assert.NotContains(t, string(body), cluster.ID)
	assert.Zero(t, listProgressOf(t, f.narrowCtx, f.red.ID).Merging)
}

// A checked Cluster with every Loser ejected is not an approval anywhere.
func TestTheListDoesNotCountAnApprovalTheDialogExcludes(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r3-list", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "List", ids)
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, ids[1])
	forced := planOf(t, tc, red.ID)
	forced.Clusters[0].Checked = true
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, forced))
	assert.Zero(t, listProgressOf(t, tc.AppCtx, red.ID).Approved)
	assert.Zero(t, reviewOf(t, tc, red.ID, 1, nil).ApplyFigures.Identical.Clusters)
}

// Recovery from a stale Cluster: "Don't remove" on the copy that changed reopens
// the rest, and the apply then merges what still holds.
func TestEjectingTheChangedCopyRecoversAStaleCluster(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r3-recover", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Recover", ids)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", ids[2]).Update("hash", "changed-bytes").Error)

	first := applyReduction(t, tc, red.ID)
	require.Len(t, first.Stale, 1)
	assert.Equal(t, models.ReductionClusterStale, findClusterByID(planOf(t, tc, red.ID), id).State)

	plan = override(t, tc, red.ID, id, application_context.ReductionActionEject, ids[2])
	assert.Equal(t, models.ReductionClusterOpen, findClusterByID(plan, id).State, "ejecting the changed copy reopens the Cluster")
	override(t, tc, red.ID, id, application_context.ReductionActionCheck, 0)
	second := applyReduction(t, tc, red.ID)
	require.Len(t, second.Applied, 1)
	assert.False(t, resourceExists(t, tc, ids[1]))
	assert.True(t, resourceExists(t, tc, ids[2]), "the changed copy was left alone")
}

// Round 2, UX. A Cluster the page shows as stale for a deleted copy is refused as
// stale, with a message that says so, and the label does not blame the apply.
func TestApprovingAClusterStaleForADeletedCopySaysSo(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r2-stale-msg", 2)
	red := createReduction(t, tc, "Stale msg", ids)
	plan := computeReduction(t, tc, red.ID)
	require.NoError(t, tc.DB.Where("id = ?", ids[1]).Delete(&models.Resource{}).Error)

	err := overrideErr(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionCheck, 0)
	assert.ErrorIs(t, err, application_context.ErrReductionClusterStale)
	assert.Contains(t, err.Error(), "stale")

	review := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"stale"}})
	require.Len(t, review.Clusters, 1)
	assert.Equal(t, "Stale", review.Clusters[0].StateLabel)
}

func TestClusterViewCarriesTheLoserCount(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r2-losercount", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Loser count", ids)
	plan := computeReduction(t, tc, red.ID)
	assert.Equal(t, 2, reviewOf(t, tc, red.ID, 1, nil).Clusters[0].LoserCount)
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, ids[1])
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, ids[2])
	view := reviewOf(t, tc, red.ID, 1, nil).Clusters[0]
	assert.Zero(t, view.LoserCount)
	for _, m := range view.Members {
		if m.ResourceID != ids[0] {
			assert.True(t, m.Ejected, "the ejected members carry Ejected")
		}
	}
}

// Round 4 A. The merge and its confirmation are one transaction, and the merge only
// goes ahead while the claim it was started for is still current.
func TestAMergeWhoseClaimWasReplacedDeletesNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r4-claimlost", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Claim lost", ids)
	computeReduction(t, tc, red.ID)

	restore := application_context.SetReductionApplyAfterClaimHookForTest(func() {
		forced := planOf(t, tc, red.ID)
		forced.Clusters[0].ClaimID = "someone-else"
		current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
		require.NoError(t, err)
		require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, forced))
	})
	defer restore()

	result := applyReduction(t, tc, red.ID)
	assert.Empty(t, result.Applied)
	assert.Empty(t, result.Stale)
	assert.Equal(t, 1, result.Changed)
	assert.True(t, resourceExists(t, tc, ids[0]) && resourceExists(t, tc, ids[1]), "nothing was deleted")
	assert.Equal(t, "someone-else", planOf(t, tc, red.ID).Clusters[0].ClaimID)
}

func TestASuccessfulApplyCommitsTheMergedMarkWithTheMerge(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r4-atomic", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Atomic", ids)
	computeReduction(t, tc, red.ID)
	result := applyReduction(t, tc, red.ID)
	require.Len(t, result.Applied, 1)
	cluster := planOf(t, tc, red.ID).Clusters[0]
	assert.True(t, cluster.Merged)
	assert.Equal(t, models.ReductionClusterApplied, cluster.State)
	assert.False(t, resourceExists(t, tc, ids[1]))
}

// Round 4 B. A recompute leaves the members of an unsettled claim alone.
func TestRecomputeKeepsAnUnsettledClaimsMembersOutOfThePool(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r4-recompute", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Recompute claim", ids)
	plan := computeReduction(t, tc, red.ID)
	require.NotNil(t, tc.AppCtx.ClaimClusterForApplyForTest(red.ID, plan.Clusters[0].ID, plan.Clusters[0].WinnerID,
		map[uint]string{ids[0]: "r4-recompute", ids[1]: "r4-recompute"}))
	after := computeReduction(t, tc, red.ID)
	require.Len(t, after.Clusters, 1, "no second Cluster was made over the same Resources")
	assert.Equal(t, models.ReductionClusterApplied, after.Clusters[0].State)
}

// Round 4 C. A merge that cannot see a member on the subtree as it is now releases
// the claim and says nothing, even though the request's own binding still sees it.
func TestAMergeRefusedOnFreshScopeReleasesTheClaimWithoutNamingIt(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)
	child, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Child", OwnerId: f.narrow.ID})
	require.NoError(t, err)
	// The allow-list is a snapshot at bind time, so rebind now the tree is complete.
	f.wideCtx, f.narrowCtx = scopedTo(t, tc, f.wide.ID), scopedTo(t, tc, f.narrow.ID)
	ids := clusterOf(t, tc, "r4-fresh", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	f.place(t, child, ids...)
	plan := f.compute(t, ids)

	restore := application_context.SetReductionApplyAfterClaimHookForTest(func() {
		require.NoError(t, tc.DB.Model(&models.Group{}).Where("id = ?", child.ID).Update("owner_id", f.wide.ID).Error)
	})
	defer restore()

	err, applied, stale := f.narrowApply(t, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, applied)
	assert.Zero(t, stale, "nothing is reported")
	assert.True(t, resourceExists(t, tc, ids[1]))
	after := findClusterByID(planOf(t, tc, f.red.ID), plan.Clusters[0].ID)
	assert.Equal(t, models.ReductionClusterOpen, after.State)
	assert.True(t, after.Checked)
}

// Round 4 D. A request that names a version other than the loaded one is refused.
func TestAWriteNamingAnotherVersionIsRefused(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r4-version", 2)
	red := createReduction(t, tc, "Versions", ids)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID
	next := versionOf(t, tc, red.ID) + 1

	_, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: next}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	_, err = tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{ID: red.ID, Version: next, ClusterID: id, Action: application_context.ReductionActionUncheck}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	_, err = tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{ID: red.ID, Version: next, Action: "uncheck", ClusterIDs: []string{id}}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{ID: red.ID, Version: next, Name: "x"}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)
	assert.True(t, resourceExists(t, tc, ids[0]) && resourceExists(t, tc, ids[1]))
}

// Round 4 E. A Cluster another decision moved before its turn is "changed", not
// stale: the Stale tab does not gain it.
func TestAClusterUncheckedMidBatchIsChangedNotStale(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "r4-changed-a", 2)
	b := clusterOf(t, tc, "r4-changed-b", 2)
	red := createReduction(t, tc, "Changed", append(append([]uint{}, a...), b...))
	plan := computeReduction(t, tc, red.ID)
	first := plan.Clusters[0].ID
	second := plan.Clusters[1].ID

	calls := 0
	restore := application_context.SetReductionApplyAfterClaimHookForTest(func() {
		calls++
		if calls == 1 {
			override(t, tc, red.ID, second, application_context.ReductionActionUncheck, 0)
		}
	})
	defer restore()
	result := applyReduction(t, tc, red.ID)
	_ = first
	assert.Len(t, result.Applied, 1)
	assert.Empty(t, result.Stale)
	assert.Equal(t, 1, result.Changed)
	assert.Zero(t, planOf(t, tc, red.ID).LastApply.Stale, "lastApply equals what the Stale tab gains")
	assert.Zero(t, reviewOf(t, tc, red.ID, 1, nil).TabCounts.Stale)
}

// Round 4 F. A legacy checked Cluster with nothing to remove is not "Approved".
func TestALegacyEmptyProposalIsNotApprovedAnywhere(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	ids := clusterOf(t, tc, "r4-legacy-empty", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Legacy empty", ids)
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, ids[1])
	forced := planOf(t, tc, red.ID)
	forced.Clusters[0].Checked = true
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, forced))

	approved := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Approval: "approved"})
	assert.Empty(t, approved.Clusters)
	unapproved := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Approval: "unapproved"})
	require.Len(t, unapproved.Clusters, 1)
	assert.Equal(t, "Not approved", unapproved.Clusters[0].StateLabel)
}

// Round 4 G. A failed revalidation records which copies changed and whether the
// kept one is among them.
func TestStaleRecordsWhichCopiesChanged(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "r4-which", 3)
	setDimensions(t, tc, ids[0], 900, 900)
	red := createReduction(t, tc, "Which", ids)
	computeReduction(t, tc, red.ID)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", ids[2]).Update("hash", "changed-2").Error)
	applyReduction(t, tc, red.ID)

	review := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"stale"}})
	require.Len(t, review.Clusters, 1)
	view := review.Clusters[0]
	assert.False(t, view.WinnerChanged)
	for _, m := range view.Members {
		assert.Equal(t, m.ResourceID == ids[2], m.Changed, "only the copy whose bytes changed is marked")
	}

	// And when the kept copy itself changed.
	tc2 := SetupTestEnv(t)
	ids2 := clusterOf(t, tc2, "r4-which-winner", 2)
	setDimensions(t, tc2, ids2[0], 900, 900)
	red2 := createReduction(t, tc2, "Which winner", ids2)
	computeReduction(t, tc2, red2.ID)
	require.NoError(t, tc2.DB.Model(&models.Resource{}).Where("id = ?", ids2[0]).Update("hash", "changed-w").Error)
	applyReduction(t, tc2, red2.ID)
	assert.True(t, reviewOf(t, tc2, red2.ID, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"stale"}}).Clusters[0].WinnerChanged)
}

// Round 4 H. settleOnly releases a stranded claim and merges nothing.
func TestSettleOnlyReleasesAStrandedClaimAndMergesNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	a := clusterOf(t, tc, "r4-settle-a", 2)
	b := clusterOf(t, tc, "r4-settle-b", 2)
	setDimensions(t, tc, a[0], 900, 900)
	red := createReduction(t, tc, "Settle", append(append([]uint{}, a...), b...))
	plan := computeReduction(t, tc, red.ID)
	cluster := clusterFor(plan, a[0])
	require.NotNil(t, tc.AppCtx.ClaimClusterForApplyForTest(red.ID, cluster.ID, cluster.WinnerID, map[uint]string{a[0]: "r4-settle-a", a[1]: "r4-settle-a"}))

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version, SettleOnly: true}, owner, restricted)
	require.NoError(t, err)
	assert.Empty(t, result.Applied)
	after := planOf(t, tc, red.ID)
	assert.Equal(t, models.ReductionClusterOpen, findClusterByID(after, cluster.ID).State)
	assert.True(t, findClusterByID(after, cluster.ID).Checked)
	assert.Nil(t, after.LastApply, "no apply was recorded")
	for _, id := range append(append([]uint{}, a...), b...) {
		assert.True(t, resourceExists(t, tc, id), "nothing was merged, the other approved Cluster included")
	}
}
