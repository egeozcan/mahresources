//go:build postgres

package api_tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// P0-6. A review decision's visibility check must hold at the write. A Resource
// moving out of the reviewer's subtree between the visibility read and the plan's
// compare-and-set would otherwise let them change a Cluster they can no longer see.
// The write now locks the member Resources (through the scoped handle) inside its
// transaction, so a mover that has not committed makes the decision wait, and one
// that commits makes it fail as an unknown Cluster. Single and bulk are the same
// path, and both are proved here.
func TestAReviewDecisionWaitsForAndRechecksAMemberLeavingTheSubtreePG(t *testing.T) {
	for _, name := range []string{"bulk", "single"} {
		t.Run(name, func(t *testing.T) {
			tc := SetupPostgresTestEnv(t)
			f := newReductionScopeFixture(t, tc)
			owner, restricted := asAdmin()

			ids := clusterOf(t, tc, "p06-"+name, 2)
			f.place(t, f.narrow, ids...)
			plan := f.compute(t, ids)
			cluster := plan.Clusters[0]
			require.True(t, cluster.Checked)

			// The mover: has taken the row lock on a member and not committed.
			mover := tc.DB.Begin()
			require.NoError(t, mover.Error)
			require.NoError(t, mover.Model(&models.Resource{}).Where("id = ?", ids[1]).Update("owner_id", f.wide.ID).Error)

			current, err := f.narrowCtx.GetResourceReduction(f.red.ID, owner, restricted)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				if name == "bulk" {
					_, err := f.narrowCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
						ID: f.red.ID, Version: current.Version, Action: "uncheck", ClusterIDs: []string{cluster.ID},
					}, owner, restricted)
					done <- err
					return
				}
				_, err := f.narrowCtx.OverrideReductionCluster(&query_models.ReductionOverride{
					ID: f.red.ID, Version: current.Version, ClusterID: cluster.ID, Action: application_context.ReductionActionUncheck,
				}, owner, restricted)
				done <- err
			}()

			select {
			case err := <-done:
				_ = mover.Rollback()
				t.Fatalf("the decision did not wait for the uncommitted move (returned %v)", err)
			case <-time.After(700 * time.Millisecond):
			}
			require.NoError(t, mover.Commit().Error)

			select {
			case err := <-done:
				assert.ErrorIs(t, err, application_context.ErrReductionClusterNotFound,
					"the Cluster is outside their subtree now, so it is an unknown one")
			case <-time.After(20 * time.Second):
				t.Fatal("the decision never finished")
			}
			assert.True(t, findClusterByID(planOf(t, tc, f.red.ID), cluster.ID).Checked, "nothing was written")
		})
	}
}

// Round 3. Two Reductions whose active and ejected members cross must not
// deadlock: each merge locks its complete member set in one ascending pass, not its
// active pair first and the ejected members later.
func TestTwoReductionsWithCrossingEjectedMembersDoNotDeadlockPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	owner, restricted := asAdmin()

	ids := clusterOf(t, tc, "r3-cross", 4) // a, b, c, d
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]
	setDimensions(t, tc, a, 900, 900)
	setDimensions(t, tc, b, 100, 100)
	setDimensions(t, tc, c, 950, 950)
	setDimensions(t, tc, d, 50, 50)

	r1 := createReduction(t, tc, "Cross one", []uint{a, b, d})
	p1 := computeReduction(t, tc, r1.ID)
	override(t, tc, r1.ID, p1.Clusters[0].ID, application_context.ReductionActionEject, d)
	r2 := createReduction(t, tc, "Cross two", []uint{a, c, d})
	p2 := computeReduction(t, tc, r2.ID)
	require.Equal(t, c, p2.Clusters[0].WinnerID)
	override(t, tc, r2.ID, p2.Clusters[0].ID, application_context.ReductionActionEject, a)

	// Both merges hold their rows before either reaches its precondition.
	var arrived sync.WaitGroup
	arrived.Add(2)
	var once1, once2 sync.Once
	_ = once1
	_ = once2
	restore := application_context.SetReductionPreconditionHookForTest(func() {
		arrived.Done()
		done := make(chan struct{})
		go func() { arrived.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2500 * time.Millisecond):
		}
	})
	defer restore()

	type res struct {
		applied, stale int
		err            error
	}
	run := func(id uint, out *res) {
		current, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
		if err != nil {
			out.err = err
			return
		}
		result, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: id, Version: current.Version}, owner, restricted)
		if err != nil {
			out.err = err
			return
		}
		out.applied, out.stale = len(result.Applied), len(result.Stale)
		for _, st := range result.Stale {
			t.Logf("stale: %s", st.Reason)
		}
	}
	var out1, out2 res
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run(r1.ID, &out1) }()
	go func() { defer wg.Done(); run(r2.ID, &out2) }()
	wg.Wait()

	require.NoError(t, out1.err)
	require.NoError(t, out2.err)
	assert.Equal(t, 1, out1.applied)
	assert.Equal(t, 1, out2.applied)
	assert.Zero(t, out1.stale+out2.stale, "neither merge was refused as a deadlock victim")
}

// A second merge into the same Winner must keep the first merge's backup record.
// Postgres's `||` is shallow and used to replace the whole backups object; SQLite's
// json_patch already merged recursively.
func TestASecondMergeKeepsTheEarlierMergesBackupRecordPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	winner := addResourceWithBody(t, tc, "w.txt", "winner body")
	first := addResourceWithBody(t, tc, "a.txt", "first body")
	second := addResourceWithBody(t, tc, "b.txt", "second body")

	require.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{first.ID}, false))
	require.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{second.ID}, false))

	var meta string
	require.NoError(t, tc.DB.Raw("SELECT meta::text FROM resources WHERE id = ?", winner.ID).Scan(&meta).Error)
	assert.Contains(t, meta, fmt.Sprintf("resource_%d", first.ID), "the first merge's record survived")
	assert.Contains(t, meta, fmt.Sprintf("resource_%d", second.ID))
}
