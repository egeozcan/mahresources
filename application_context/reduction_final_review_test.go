package application_context

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// Final-review fixes to the plan-table migration and the selections. Every body
// takes a context so the SQLite and Postgres wrappers run the same assertions.

func planSchemaOf(t *testing.T, ctx *MahresourcesContext, id uint) int {
	t.Helper()
	var got struct{ PlanSchema int }
	require.NoError(t, ctx.db.Raw("SELECT plan_schema FROM resource_reductions WHERE id = ?", id).Scan(&got).Error)
	return got.PlanSchema
}

// (1) A run that died mid-migration leaves a live lease; the one-shot startup run
// skips it. The periodic runner comes back, takes the lease over once it lapses,
// and stops when no legacy Reduction is left.
func testReductionMigrationRunnerRetriesPastALiveLease(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	_, err := ctx.reductionStore().takeOverMigration(red.ID) // the crashed run's lease
	require.NoError(t, err)

	require.NoError(t, ctx.MigrateReductionPlans(), "a live lease held by another run is not an error")
	require.Equal(t, 0, planSchemaOf(t, ctx, red.ID), "the one-shot run skipped the leased Reduction")

	require.NoError(t, ctx.db.Exec("UPDATE reduction_migrations SET lease_until = ? WHERE reduction_id = ?",
		time.Now().Add(400*time.Millisecond).UnixNano(), red.ID).Error)
	stop, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := ctx.StartReductionMigration(stop, 40*time.Millisecond)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the runner never finished")
	}
	require.Equal(t, 1, planSchemaOf(t, ctx, red.ID), "the lapsed lease was taken over and the Reduction published")
}

// (2) A JSON true is not Resource 1 in a legacy plan: only integers are ids, on
// both databases. The refused member is counted and a checked Cluster that lost a
// Loser to it migrates unchecked, never as approved.
func testReductionMigrationRefusesNonIntegerIDs(t *testing.T, ctx *MahresourcesContext) {
	filler := makeResources(t, ctx, 1) // Resource 1, the id a JSON true would read as
	ids := makeResources(t, ctx, 4)
	require.EqualValues(t, 1, filler[0])
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[`+
			`{"id":"bad","tier":"identical","winnerId":%[1]d,"state":"open","checked":true,"members":[{"resourceId":%[1]d,"hash":"h"},{"resourceId":%[2]d,"hash":"h"},{"resourceId":true,"hash":"h"}]},`+
			`{"id":"good","tier":"identical","winnerId":%[3]d,"state":"open","checked":true,"changedMembers":[true],`+
			`"members":[{"resourceId":%[3]d,"hash":"h"},{"resourceId":%[4]d,"hash":"h"},{"resourceId":1,"hash":"h"}]}],"coverage":{},"settings":{}}`,
		ids[0], ids[1], ids[2], ids[3]),
		fmt.Sprintf(`{"resourceIds":[%d,true],"groupIds":[true,3]}`, ids[0]))

	require.NoError(t, ctx.SpoolReductionMigrations())
	var rejected int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ? AND rejected = ?", red.ID, true).Count(&rejected).Error)
	require.EqualValues(t, 3, rejected, "the true member, the true Resource id and the true Group id")

	require.NoError(t, ctx.MigrateReductionPlans())
	require.Equal(t, 1, planSchemaOf(t, ctx, red.ID))

	var extent []uint
	require.NoError(t, ctx.db.Raw("SELECT resource_id FROM reduction_extent_resources WHERE reduction_id = ?", red.ID).Scan(&extent).Error)
	require.Equal(t, []uint{ids[0]}, extent, "true is not Resource 1")
	var groups []uint
	require.NoError(t, ctx.db.Raw("SELECT group_id FROM reduction_extent_groups WHERE reduction_id = ?", red.ID).Scan(&groups).Error)
	require.Equal(t, []uint{3}, groups, "true is not Group 1")

	var bad, good models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "bad").First(&bad).Error)
	require.NoError(t, ctx.db.Where("reduction_id = ? AND \"key\" = ?", red.ID, "good").First(&good).Error)
	require.False(t, bad.Checked, "a Cluster that lost a member to a refused id is not approved")
	require.True(t, good.Checked, "a Cluster with only valid ids keeps its approval")
	var badMembers int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("cluster_id = ? AND resource_id = 1", bad.ID).Count(&badMembers).Error)
	require.EqualValues(t, 0, badMembers, "true did not become a Resource 1 member")
	var changed int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("cluster_id = ? AND changed = ?", good.ID, true).Count(&changed).Error)
	require.EqualValues(t, 0, changed, "a true in changedMembers marks nothing")
}

// legacyBigClusterPlan is one Cluster of n members over ids that need not exist,
// every other one listed as changed.
func legacyBigClusterPlan(n int) string {
	var members, changed []string
	for i := 0; i < n; i++ {
		id := 5000000 + i
		members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, id))
		if i%2 == 0 {
			changed = append(changed, fmt.Sprint(id))
		}
	}
	return fmt.Sprintf(`{"clusters":[{"id":"big","tier":"identical","winnerId":5000000,"state":"open","changedMembers":[%s],"members":[%s]}],"coverage":{},"settings":{}}`,
		strings.Join(changed, ","), strings.Join(members, ","))
}

// (3) Spooling a Cluster with many changed members is linear in them: Postgres
// searched the whole changedMembers array for every member.
func testReductionSpoolChangedMembersLinear(t *testing.T, ctx *MahresourcesContext) {
	timed := func(n int) time.Duration {
		red := createLegacyReduction(t, ctx, legacyBigClusterPlan(n), "")
		start := time.Now()
		require.NoError(t, ctx.SpoolReductionMigrations())
		took := time.Since(start)
		var changed int64
		require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ? AND kind = 'member' AND changed = ?", red.ID, true).Count(&changed).Error)
		require.EqualValues(t, n/2, changed, "every other member was listed as changed")
		return took
	}
	small := timed(4000)
	large := timed(16000)
	require.Less(t, large, 7*small+200*time.Millisecond, "4x the changed members took %v against %v: quadratic", large, small)
}

// (4) The legacy list chunks seek on the whole primary key, cluster_ord included.
func testReductionListChunksSeekOnTheWholeKey(t *testing.T, ctx *MahresourcesContext) {
	for _, step := range spooledListSteps(1, "gen", 0) {
		var details []string
		if ctx.db.Dialector.Name() == "postgres" {
			require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, tx.Exec("SET LOCAL enable_seqscan = off").Error)
				return tx.Raw("EXPLAIN "+step.sql, step.args...).Scan(&details).Error
			}))
			plan := strings.Join(details, "\n")
			require.Regexp(t, `Index Cond:.*cluster_ord.*member_ord`, strings.ReplaceAll(plan, "\n", " "), "%s: %s", step.kind, plan)
			continue
		}
		var rows []struct {
			Detail string `gorm:"column:detail"`
		}
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+step.sql, step.args...).Scan(&rows).Error)
		var scan []string
		for _, r := range rows {
			if strings.Contains(r.Detail, "reduction_migration_rows") {
				scan = append(scan, r.Detail)
			}
		}
		require.NotEmpty(t, scan, step.kind)
		joined := strings.Join(scan, "\n")
		require.Contains(t, joined, "cluster_ord=?", "%s: %s", step.kind, joined)
		require.Contains(t, joined, "member_ord>", "%s: the range seeks past cluster_ord: %s", step.kind, joined)
	}
}

// (5) Selecting a Group with its descendants is a handful of statements, not one
// per descendant Group, and takes exactly the Resources of the subtree.
func testReductionSelectSubtreeIsSetBased(t *testing.T, ctx *MahresourcesContext) {
	root := &models.Group{Name: "root"}
	require.NoError(t, ctx.db.Create(root).Error)
	kids := make([]models.Group, 20000)
	for i := range kids {
		kids[i] = models.Group{Name: fmt.Sprintf("k%d", i), OwnerId: &root.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(kids, 1000).Error)
	grand := &models.Group{Name: "grand", OwnerId: &kids[7].ID}
	require.NoError(t, ctx.db.Create(grand).Error)
	away := &models.Group{Name: "away"}
	require.NoError(t, ctx.db.Create(away).Error)
	own := func(owner uint, n int) []uint {
		var out []uint
		for i := 0; i < n; i++ {
			res := &models.Resource{Name: fmt.Sprintf("r%d-%d", owner, i), OwnerId: &owner}
			require.NoError(t, ctx.db.Create(res).Error)
			out = append(out, res.ID)
		}
		return out
	}
	var want []uint
	want = append(want, own(root.ID, 2)...)
	want = append(want, own(kids[3].ID, 3)...)
	want = append(want, own(grand.ID, 2)...)
	own(away.ID, 4)
	r := newReductionRow(t, ctx.db)

	var mu sync.Mutex
	statements := 0
	count := func(*gorm.DB) { mu.Lock(); statements++; mu.Unlock() }
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register("test:ffa_row", count))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register("test:ffa_query", count))
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register("test:ffa_raw", count))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Row().Remove("test:ffa_row")
		_ = ctx.db.Callback().Query().Remove("test:ffa_query")
		_ = ctx.db.Callback().Raw().Remove("test:ffa_raw")
	})

	added, err := ctx.addOwnedResourcesToExtent(ctx.reductionStore(), r, root.ID, true)
	require.NoError(t, err)
	require.Equal(t, len(want), added)
	mu.Lock()
	used := statements
	mu.Unlock()
	require.Less(t, used, 40, "one statement set for the subtree, not one per Group")

	var got []uint
	require.NoError(t, ctx.db.Raw("SELECT resource_id FROM reduction_extent_resources WHERE reduction_id = ? ORDER BY resource_id", r.ID).Scan(&got).Error)
	require.ElementsMatch(t, want, got)

	r2 := newReductionRow(t, ctx.db)
	added, err = ctx.addOwnedResourcesToExtent(ctx.reductionStore(), r2, root.ID, false)
	require.NoError(t, err)
	require.Equal(t, 2, added, "without descendants only the Group's own Resources")
}

// (6) The startup scan reads candidates in keyset chunks and keeps one
// Reduction's settings probe.
func testReductionMigrationScansInChunks(t *testing.T, ctx *MahresourcesContext) {
	ids := makeResources(t, ctx, 2)
	var reds []*models.ResourceReduction
	for i := 0; i < 5; i++ {
		reds = append(reds, createLegacyReduction(t, ctx, fmt.Sprintf(
			`{"clusters":[{"id":"c","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h"}]}],"coverage":{}}`,
			ids[0], ids[0], ids[1]), ""))
	}
	old := migrationScanChunk
	migrationScanChunk = 2
	t.Cleanup(func() { migrationScanChunk = old })

	var mu sync.Mutex
	scans := 0
	scan := func(db *gorm.DB) {
		sql := db.Statement.SQL.String()
		if strings.Contains(sql, "FROM resource_reductions WHERE plan_schema = 0") || strings.Contains(sql, "FROM reduction_migrations WHERE reduction_id >") {
			mu.Lock()
			scans++
			mu.Unlock()
		}
	}
	require.NoError(t, ctx.db.Callback().Row().Before("gorm:row").Register("test:ffa_scan_row", scan))
	require.NoError(t, ctx.db.Callback().Query().Before("gorm:query").Register("test:ffa_scan_query", scan))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Row().Remove("test:ffa_scan_row")
		_ = ctx.db.Callback().Query().Remove("test:ffa_scan_query")
	})

	require.NoError(t, ctx.MigrateReductionPlans())
	for _, r := range reds {
		require.Equal(t, 1, planSchemaOf(t, ctx, r.ID))
	}
	mu.Lock()
	require.GreaterOrEqual(t, scans, 6, "5 candidates in chunks of 2 are three reads for each of the two steps")
	mu.Unlock()

	s := ctx.reductionStore()
	for i := 0; i < 3; i++ {
		red := createLegacyReduction(t, ctx, fmt.Sprintf(
			`{"clusters":[{"id":"c","tier":"identical","winnerId":%d,"state":"open","members":[{"resourceId":%d,"hash":"h"},{"resourceId":%d,"hash":"h"}]}],"coverage":{}}`,
			ids[0], ids[0], ids[1]), "")
		require.NoError(t, ctx.SpoolReductionMigrations())
		require.NoError(t, s.migrateReduction(red.ID))
	}
	require.LessOrEqual(t, s.settingsProbeCount(), 1, "only the current Reduction's probe is kept")
}

// (7) Spool rows whose Reduction is published and whose migration record is gone
// (a crash between publication and the cleanup) are swept; a live migration's are not.
func testReductionSweepsOrphanSpool(t *testing.T, ctx *MahresourcesContext) {
	published := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1 WHERE id = ?", published.ID).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, ctx.db.Create(&models.ReductionMigrationRow{ReductionID: published.ID, Generation: "dead", Kind: "member", ClusterOrd: 0, MemberOrd: i, ResourceID: uint(i + 1)}).Error)
	}
	live, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	var liveBefore int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", live.ID).Count(&liveBefore).Error)
	require.NotZero(t, liveBefore)

	require.NoError(t, ctx.SweepOrphanReductionSpool())
	var dead, liveAfter int64
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", published.ID).Count(&dead).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionMigrationRow{}).Where("reduction_id = ?", live.ID).Count(&liveAfter).Error)
	require.Zero(t, dead, "the published Reduction's spool was swept")
	require.Equal(t, liveBefore, liveAfter, "a migration in flight keeps its spool")
}

// (10) Review reports no settleable claims while a settle lease is live, as the
// write path would refuse them.
func testSettleableClaimsHonoursTheSettleLease(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	a := &models.Resource{Name: "a"}
	b := &models.Resource{Name: "b"}
	require.NoError(t, ctx.db.Create(a).Error)
	require.NoError(t, ctx.db.Create(b).Error)
	c := &storedCluster{Row: models.ReductionClusterRow{Key: "claimed", Tier: "identical", State: "applied", WinnerID: a.ID, ClaimID: "x", Position: 0},
		Members: []models.ReductionMemberRow{{ResourceID: a.ID, Hash: "h"}, {ResourceID: b.ID, Hash: "h"}}}
	require.NoError(t, ctx.reductionStore().insertCluster(r, "g", c))
	s := ctx.reductionStore()
	now := time.Now()

	n, err := s.settleableClaims(r, "g", now)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	op := "settle-op"
	r.SettleOp = &op
	r.SettleLeaseUntil = now.Add(time.Minute).UnixNano()
	n, err = s.settleableClaims(r, "g", now)
	require.NoError(t, err)
	require.Equal(t, 0, n, "a live settle lease owns the claims")

	r.SettleLeaseUntil = now.Add(-time.Minute).UnixNano()
	n, err = s.settleableClaims(r, "g", now)
	require.NoError(t, err)
	require.Equal(t, 1, n, "a lapsed lease owns nothing")
}

func TestReductionMigrationRunnerRetriesPastALiveLeaseSQLite(t *testing.T) {
	testReductionMigrationRunnerRetriesPastALiveLease(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationRefusesNonIntegerIDsSQLite(t *testing.T) {
	testReductionMigrationRefusesNonIntegerIDs(t, newReductionSQLiteContext(t))
}
func TestReductionSpoolChangedMembersLinearSQLite(t *testing.T) {
	testReductionSpoolChangedMembersLinear(t, newReductionSQLiteContext(t))
}
func TestReductionListChunksSeekOnTheWholeKeySQLite(t *testing.T) {
	testReductionListChunksSeekOnTheWholeKey(t, newReductionSQLiteContext(t))
}
func TestReductionSelectSubtreeIsSetBasedSQLite(t *testing.T) {
	testReductionSelectSubtreeIsSetBased(t, newReductionSQLiteContext(t))
}
func TestReductionMigrationScansInChunksSQLite(t *testing.T) {
	testReductionMigrationScansInChunks(t, newReductionSQLiteContext(t))
}
func TestReductionSweepsOrphanSpoolSQLite(t *testing.T) {
	testReductionSweepsOrphanSpool(t, newReductionSQLiteContext(t))
}
func TestSettleableClaimsHonoursTheSettleLeaseSQLite(t *testing.T) {
	testSettleableClaimsHonoursTheSettleLease(t, newReductionSQLiteContext(t))
}

func joinStrings(s []string) string { return strings.Join(s, ",") }
