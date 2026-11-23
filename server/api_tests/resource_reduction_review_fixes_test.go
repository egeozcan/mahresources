package api_tests

import (
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
