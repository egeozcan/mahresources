package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func hookCluster(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, gen, key string, winner uint, members ...models.ReductionMemberRow) *storedCluster {
	t.Helper()
	c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: "identical", State: "open", WinnerID: winner}, Members: members}
	require.NoError(t, ctx.reductionStore().insertCluster(r, gen, c))
	return c
}

func goneOf(t *testing.T, ctx *MahresourcesContext, clusterID uint) (map[uint]bool, int) {
	t.Helper()
	var ms []models.ReductionMemberRow
	require.NoError(t, ctx.db.Where("cluster_id = ?", clusterID).Find(&ms).Error)
	gone := map[uint]bool{}
	for _, m := range ms {
		gone[m.ResourceID] = m.Gone
	}
	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.First(&row, clusterID).Error)
	return gone, row.GoneMembers
}

// The hook marks a staged Resource gone in the live, computing and migrating
// generations only, ejected members included, and counts only non-ejected ones.
func testDeletionHookMarksMembersGone(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	gen, comp, mig := "g-live", "g-compute", "g-migrate"
	require.NoError(t, ctx.db.Model(r).Updates(map[string]any{"plan_generation": gen, "compute_generation": comp}).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionMigration{ReductionID: r.ID, Generation: mig}).Error)

	ejected := member(5, "h", 1)
	ejected.Ejected = true
	c1 := hookCluster(t, ctx, r, gen, "c1", 1, member(1, "h", 1), member(2, "h", 1), member(3, "h", 1))
	c2 := hookCluster(t, ctx, r, comp, "c2", 2, member(2, "h", 1), member(5, "h", 1))
	c3 := hookCluster(t, ctx, r, mig, "c3", 7, member(2, "h", 1), member(7, "h", 1))
	c4 := hookCluster(t, ctx, r, "g-stale", "c4", 9, member(2, "h", 1), member(9, "h", 1))
	c5 := hookCluster(t, ctx, r, gen, "c5", 4, member(4, "h", 1), ejected)

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{2, 5}))
	require.NoError(t, reductionDeletionHook(ctx.db, op))
	// Idempotent.
	require.NoError(t, reductionDeletionHook(ctx.db, op))

	gone, n := goneOf(t, ctx, c1.Row.ID)
	assert.Equal(t, map[uint]bool{1: false, 2: true, 3: false}, gone)
	assert.Equal(t, 1, n)
	gone, n = goneOf(t, ctx, c2.Row.ID)
	assert.Equal(t, map[uint]bool{2: true, 5: true}, gone)
	assert.Equal(t, 2, n, "a Winner that is gone counts, as does a Loser")
	gone, n = goneOf(t, ctx, c3.Row.ID)
	assert.Equal(t, map[uint]bool{2: true, 7: false}, gone, "the migrating generation is covered")
	assert.Equal(t, 1, n)
	gone, n = goneOf(t, ctx, c4.Row.ID)
	assert.Equal(t, map[uint]bool{2: false, 9: false}, gone, "a generation nothing reads is left alone")
	assert.Equal(t, 0, n)
	gone, n = goneOf(t, ctx, c5.Row.ID)
	assert.Equal(t, map[uint]bool{4: false, 5: true}, gone, "an ejected member is gone too")
	assert.Equal(t, 0, n, "but gone_members counts non-ejected members only")

	var staged int64
	require.NoError(t, ctx.db.Model(&models.ReductionDeletedID{}).Where("op = ?", op).Count(&staged).Error)
	assert.EqualValues(t, 2, staged, "the hook never deletes the staged ids")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

// One walk over the affected Clusters in ascending id order, whatever the order
// the ids were staged in: a bulk delete never locks Cluster 20 before Cluster 10.
func testDeletionHookWalksClustersAscending(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Model(r).Update("plan_generation", "g").Error)
	var clusters []*storedCluster
	for i := 0; i < 7; i++ {
		// Resource 100+i is in Cluster i, Resource 200 is in all of them.
		clusters = append(clusters, hookCluster(t, ctx, r, "g", fmt.Sprintf("k%d", i), uint(100+i), member(uint(100+i), "h", 1), member(200, "h", 1)))
	}

	old := reductionHookStep
	reductionHookStep = 2
	t.Cleanup(func() { reductionHookStep = old })

	var mu sync.Mutex
	var order [][]any
	const cb = "test:hook_cluster_order"
	require.NoError(t, ctx.db.Callback().Raw().Before("gorm:raw").Register(cb, func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.HasPrefix(sql, "UPDATE reduction_clusters SET gone_members") {
			mu.Lock()
			order = append(order, tx.Statement.Vars)
			mu.Unlock()
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(cb) })

	op := newDeleteOp()
	// Staged in descending order on purpose.
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{106, 104, 102, 200, 101}))
	require.NoError(t, reductionDeletionHook(ctx.db, op))

	var seen []uint
	mu.Lock()
	for _, vars := range order {
		for _, v := range vars {
			switch x := v.(type) {
			case uint:
				seen = append(seen, x)
			case []uint:
				seen = append(seen, x...)
			}
		}
	}
	mu.Unlock()
	require.NotEmpty(t, seen)
	for i := 1; i < len(seen); i++ {
		assert.Less(t, seen[i-1], seen[i], "Clusters are updated in ascending id order across steps: %v", seen)
	}
	assert.Len(t, seen, 7, "every Cluster holding a staged Resource is visited exactly once")
	want := map[int]int{0: 1, 1: 2, 2: 2, 3: 1, 4: 2, 5: 1, 6: 2}
	for i, c := range clusters {
		_, n := goneOf(t, ctx, c.Row.ID)
		assert.Equal(t, want[i], n, "Cluster %d", i)
	}
}

func TestDeletionHookMarksMembersGoneSQLite(t *testing.T) {
	testDeletionHookMarksMembersGone(t, newReductionSQLiteContext(t))
}

func TestDeletionHookWalksClustersAscendingSQLite(t *testing.T) {
	testDeletionHookWalksClustersAscending(t, newReductionSQLiteContext(t))
}
