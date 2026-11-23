package api_tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// planReduction stores a Reduction whose plan is exactly the Clusters given, so the
// list counts can be checked against a plan built by hand.
func planReduction(t *testing.T, tc *TestContext, name string, clusters []*models.ReductionCluster) *models.ResourceReduction {
	t.Helper()
	encoded, err := json.Marshal(models.ResourceReductionPlan{Clusters: clusters})
	require.NoError(t, err)
	now := time.Now()
	row := &models.ResourceReduction{
		Name: name, Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeBothTiers,
		Plan: types.JSON(encoded), ComputedAt: &now,
	}
	require.NoError(t, tc.DB.Create(row).Error)
	return row
}

func pairCluster(id string, state string, checked bool, a, b uint) *models.ReductionCluster {
	return &models.ReductionCluster{
		ID: id, Tier: models.ReductionTierIdentical, State: state, Checked: checked, WinnerID: a,
		Members: []*models.ReductionMember{{ResourceID: a}, {ResourceID: b}},
	}
}

func progressOf(t *testing.T, c *application_context.MahresourcesContext, owner *uint, restricted bool) map[uint]contracts.ReductionProgress {
	t.Helper()
	rows, err := c.GetResourceReductions(0, 50, &query_models.ResourceReductionQuery{OwnerUserID: owner, OwnerRestricted: restricted})
	require.NoError(t, err)
	progress, err := c.GetReductionProgress(rows)
	require.NoError(t, err)
	return progress
}

func TestListProgressCountsEachStatus(t *testing.T) {
	tc := SetupTestEnv(t)
	// The members have to exist: for an unrestricted caller a Cluster naming a
	// deleted Resource is counted as stale.
	for i := 1; i <= 16; i++ {
		r := addResourceWithBody(t, tc, fmt.Sprintf("m%d.txt", i), fmt.Sprintf("body %d", i))
		require.EqualValues(t, i, r.ID)
	}
	red := planReduction(t, tc, "Counted", []*models.ReductionCluster{
		pairCluster("a", models.ReductionClusterOpen, true, 1, 2),
		pairCluster("b", models.ReductionClusterOpen, true, 3, 4),
		pairCluster("c", models.ReductionClusterOpen, false, 5, 6),
		pairCluster("d", models.ReductionClusterSkipped, false, 7, 8),
		pairCluster("e", models.ReductionClusterApplied, false, 9, 10),
		pairCluster("f", models.ReductionClusterApplied, false, 11, 12),
		pairCluster("g", models.ReductionClusterApplied, false, 13, 14),
		pairCluster("h", models.ReductionClusterStale, false, 15, 16),
	})
	draft := &models.ResourceReduction{Name: "Never computed", Status: models.ReductionStatusDraft, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, tc.DB.Create(draft).Error)

	owner, restricted := asAdmin()
	progress := progressOf(t, tc.AppCtx, owner, restricted)
	assert.Equal(t, contracts.ReductionProgress{Clusters: 8, Open: 3, Approved: 2, Skipped: 1, Stale: 1, Applied: 3}, progress[red.ID])
	assert.Equal(t, contracts.ReductionProgress{}, progress[draft.ID], "an uncomputed Reduction has nothing to count")
}

// For a subtree-confined principal a Cluster reaching a Resource they may not see
// is in none of the counts, and the union of the page's members is read once.
func TestListProgressLeavesOutWithheldClusters(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)
	var inside, outside []uint
	for i := 0; i < 4; i++ {
		inside = append(inside, addResourceWithBody(t, tc, fmt.Sprintf("in-%d.txt", i), fmt.Sprintf("in %d", i)).ID)
		outside = append(outside, addResourceWithBody(t, tc, fmt.Sprintf("out-%d.txt", i), fmt.Sprintf("out %d", i)).ID)
	}
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", inside).Update("owner_id", mine.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", outside).Update("owner_id", theirs.ID).Error)

	red := planReduction(t, tc, "Mixed", []*models.ReductionCluster{
		pairCluster("visible-open", models.ReductionClusterOpen, true, inside[0], inside[1]),
		pairCluster("visible-skipped", models.ReductionClusterSkipped, false, inside[2], inside[3]),
		pairCluster("hidden-open", models.ReductionClusterOpen, true, outside[0], outside[1]),
		pairCluster("half-hidden", models.ReductionClusterApplied, false, inside[0], outside[2]),
	})

	scoped := scopedTo(t, tc, mine.ID)
	got := progressOf(t, scoped, owner, restricted)[red.ID]
	assert.Equal(t, contracts.ReductionProgress{Clusters: 2, Open: 1, Approved: 1, Skipped: 1}, got,
		"the Cluster outside the subtree and the one half outside it are not counted at all")

	admin := progressOf(t, tc.AppCtx, owner, restricted)[red.ID]
	assert.Equal(t, 4, admin.Clusters)
}

// A page of Reductions with large plans completes quickly, with right counts, for an
// unrestricted principal and a confined one. Measured here: one such plan on the
// review page takes about 10ms and the counts for all twenty about 140ms, so the
// 5s budget is for slow CI machines; what it guards is the shape (one pass over each plan, one
// union visibility read), which a per-Cluster or per-Reduction query would blow
// through by orders of magnitude.
func TestListProgressOverAPageOfLargePlans(t *testing.T) {
	if testing.Short() {
		t.Skip("builds 100,000 Clusters")
	}
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)

	// Real Resources the plans point at: three quarters inside the subtree.
	const pool = 1200
	rows := make([]models.Resource, pool)
	for i := range rows {
		ownerID := mine.ID
		if i%4 == 3 {
			ownerID = theirs.ID
		}
		rows[i] = models.Resource{Name: fmt.Sprintf("pool-%d", i), Hash: fmt.Sprintf("pool-hash-%d", i), OwnerId: &ownerID}
	}
	require.NoError(t, tc.DB.CreateInBatches(&rows, 200).Error)

	const reductions, clustersEach = 20, 5000
	var wantAll, wantVisible contracts.ReductionProgress
	tally := func(p *contracts.ReductionProgress, state string, checked bool) {
		p.Clusters++
		switch state {
		case models.ReductionClusterOpen:
			p.Open++
			if checked {
				p.Approved++
			}
		case models.ReductionClusterApplied:
			p.Applied++
		case models.ReductionClusterSkipped:
			p.Skipped++
		}
	}
	inSubtree := func(r models.Resource) bool { return *r.OwnerId == mine.ID }
	build := func() []*models.ReductionCluster {
		clusters := make([]*models.ReductionCluster, clustersEach)
		wantAll, wantVisible = contracts.ReductionProgress{}, contracts.ReductionProgress{}
		for i := range clusters {
			first, second := rows[(i*2)%pool], rows[(i*2+1)%pool]
			state, checked := models.ReductionClusterOpen, i%2 == 0
			switch i % 5 {
			case 3:
				state, checked = models.ReductionClusterApplied, false
			case 4:
				state, checked = models.ReductionClusterSkipped, false
			}
			clusters[i] = pairCluster(fmt.Sprintf("c-%d", i), state, checked, first.ID, second.ID)
			tally(&wantAll, state, checked)
			if inSubtree(first) && inSubtree(second) {
				tally(&wantVisible, state, checked)
			}
		}
		return clusters
	}
	ids := make([]uint, 0, reductions)
	for i := 0; i < reductions; i++ {
		ids = append(ids, planReduction(t, tc, fmt.Sprintf("Large %d", i), build()).ID)
	}

	// The same plan on the review page, as the yardstick.
	started := time.Now()
	review := reviewOf(t, tc, ids[0], 1, nil)
	reviewTook := time.Since(started)
	require.Equal(t, clustersEach, review.ClusterCount)

	started = time.Now()
	admin := progressOf(t, tc.AppCtx, owner, restricted)
	adminTook := time.Since(started)
	for _, id := range ids {
		assert.Equal(t, wantAll, admin[id])
	}

	scoped := scopedTo(t, tc, mine.ID)
	started = time.Now()
	confined := progressOf(t, scoped, owner, restricted)
	confinedTook := time.Since(started)
	for _, id := range ids {
		assert.Equal(t, wantVisible, confined[id])
	}
	assert.Less(t, wantVisible.Clusters, clustersEach, "the fixture hides some Clusters")

	t.Logf("review page of one plan: %v; list counts for %d plans: admin %v, confined %v",
		reviewTook, reductions, adminTook, confinedTook)
	assert.Less(t, adminTook, 5*time.Second)
	assert.Less(t, confinedTook, 5*time.Second)
}
