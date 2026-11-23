//go:build postgres

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
