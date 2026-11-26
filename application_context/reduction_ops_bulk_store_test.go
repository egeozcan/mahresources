package application_context

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// bulkInsert writes many two-member Clusters (a Winner and one Loser over fresh
// Resource ids that need not exist) in a few statements, so a test can cross the
// 500-row chunk boundaries without a transaction per Cluster.
func bulkInsert(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, n int, mutate func(i int, c *storedCluster)) []*storedCluster {
	t.Helper()
	out := make([]*storedCluster, n)
	rows := make([]models.ReductionClusterRow, n)
	for i := 0; i < n; i++ {
		w, l := uint(2_000_000+2*i), uint(2_000_001+2*i)
		c := &storedCluster{
			Row:     models.ReductionClusterRow{ReductionID: r.ID, Generation: opsGen, Key: fmt.Sprintf("b%05d", i), Position: int64(i), Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen, WinnerID: w},
			Members: []models.ReductionMemberRow{{ResourceID: w, Hash: "h", FileSize: 5, InExtent: true}, {ResourceID: l, Hash: "h", FileSize: 5, InExtent: true}},
		}
		if mutate != nil {
			mutate(i, c)
		}
		deriveClusterColumns(c)
		rows[i] = c.Row
		out[i] = c
	}
	require.NoError(t, ctx.db.CreateInBatches(&rows, 200).Error)
	var members []models.ReductionMemberRow
	for i := range out {
		out[i].Row = rows[i]
		for j := range out[i].Members {
			out[i].Members[j].ClusterID = rows[i].ID
			out[i].Members[j].ReductionID = r.ID
			out[i].Members[j].Generation = opsGen
			out[i].Members[j].Position = j
		}
		members = append(members, out[i].Members...)
	}
	require.NoError(t, ctx.db.CreateInBatches(&members, 200).Error)
	return out
}

func bulk(r *models.ResourceReduction, action string, ids []string, all bool, expected *uint) *query_models.ReductionBulkOverride {
	return &query_models.ReductionBulkOverride{ID: r.ID, Version: r.Version, Action: action, ClusterIDs: ids, AllIdentical: all, ExpectedCount: expected}
}

func requireNoBulkMarks(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) {
	t.Helper()
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND bulk_op <> ''", r.ID).Count(&n).Error)
	require.Zero(t, n, "no Cluster keeps a bulk mark")
}

// By ids: today's per-Cluster rules. A check leaves a Cluster that needs attention
// exactly as it was and counts it; one with nothing to remove is skipped silently;
// everything else is set and frozen as reviewed. Reachability is decided for every
// id before state, and nothing is written on a refusal.
func testOpsBulkByIDs(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	clusters := bulkInsert(t, ctx, r, 6, func(i int, c *storedCluster) {
		switch i {
		case 0: // plain, unchecked
		case 1: // plain, already checked
			c.Row.Checked = true
		case 2: // needs attention
			c.Row.Lossy = []byte(`["description"]`)
		case 3: // nothing to remove
			c.Members[1].Ejected = true
		case 4: // skipped
			c.Row.State = models.ReductionClusterSkipped
		case 5: // near tier, plain
			c.Row.Tier = models.ReductionTierNear
		}
	})
	keys := func(idx ...int) []string {
		var out []string
		for _, i := range idx {
			out = append(out, clusters[i].Row.Key)
		}
		return out
	}

	_, err := ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, keys(0, 1, 4), false, nil))
	require.ErrorIs(t, err, ErrReductionClusterNotOpen)
	_, err = ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, append(keys(0), "ghost", clusters[4].Row.Key), false, nil))
	require.ErrorIs(t, err, ErrReductionClusterNotFound, "unknown is answered before not-open")
	require.Equal(t, r.Version, loadReductionRow(t, ctx.db, r.ID).Version)
	require.False(t, loadOps(t, ctx, r, clusters[0].Row.Key).Row.Checked, "a refused request writes nothing")
	requireNoBulkMarks(t, ctx, r)

	res, err := ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, keys(0, 1, 2, 3, 5, 0), false, nil))
	require.NoError(t, err)
	require.Equal(t, 2, res.Changed, "clusters 0 and 5 flipped; 1 was already checked")
	require.Equal(t, 1, res.NeedsReview)
	require.Equal(t, r.Version+1, res.Version)
	r = loadReductionRow(t, ctx.db, r.ID)
	for i, wantChecked := range []bool{true, true, false, false, false, true} {
		got := loadOps(t, ctx, r, clusters[i].Row.Key)
		require.Equal(t, wantChecked, got.Row.Checked, "cluster %d", i)
	}
	require.True(t, loadOps(t, ctx, r, clusters[1].Row.Key).Row.Reviewed, "an unchanged approval is still a review")
	require.False(t, loadOps(t, ctx, r, clusters[2].Row.Key).Row.Reviewed, "needs-review is left exactly as it was")
	require.False(t, loadOps(t, ctx, r, clusters[3].Row.Key).Row.Reviewed)

	res, err = ctx.storeBulkOverride(r, bulk(r, ReductionActionUncheck, keys(0, 1, 2), false, nil))
	require.NoError(t, err)
	require.Equal(t, 2, res.Changed)
	require.Zero(t, res.NeedsReview)
	require.True(t, loadOps(t, ctx, r, clusters[2].Row.Key).Row.Reviewed, "an uncheck always freezes")

	many := make([]string, reductionBulkIDCap+1)
	for i := range many {
		many[i] = fmt.Sprintf("x%d", i)
	}
	r = loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, many, false, nil))
	require.ErrorIs(t, err, ErrReductionBulkTooMany)
	_, err = ctx.storeBulkOverride(r, bulk(r, "frob", keys(0), false, nil))
	require.ErrorIs(t, err, ErrReductionBulkTarget)
	_, err = ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, nil))
	require.ErrorIs(t, err, ErrReductionBulkNeedsCount)

	requireNoBulkMarks(t, ctx, r)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsBulkByIDsSQLite(t *testing.T) {
	testOpsBulkByIDs(t, withSettings(newReductionSQLiteContext(t)))
}

// allIdentical across several chunks: the counted set is the written set, never
// reaches the Near tier, and a stale expectedCount refuses before anything is written.
func testOpsBulkAllIdentical(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	n := reductionRowChunk*2 + 77
	clusters := bulkInsert(t, ctx, r, n, func(i int, c *storedCluster) {
		switch {
		case i%11 == 0:
			c.Row.Tier = models.ReductionTierNear
		case i%7 == 0:
			c.Row.Oversized = true
		case i%5 == 0:
			c.Row.Checked = true
		case i%13 == 0:
			c.Row.State = models.ReductionClusterSkipped
		case i%17 == 0:
			c.Members[1].Gone = true
		}
	})
	approvable, checkedNow := 0, 0
	for _, c := range clusters {
		if c.Row.Tier != models.ReductionTierIdentical || c.Row.State != models.ReductionClusterOpen || c.Row.GoneMembers > 0 {
			continue
		}
		if c.Row.Checked {
			checkedNow++
		} else if !c.Row.NeedsAttention {
			approvable++
		}
	}
	needsReview := 0
	for _, c := range clusters {
		if c.Row.Tier == models.ReductionTierIdentical && c.Row.State == models.ReductionClusterOpen && c.Row.GoneMembers == 0 && c.Row.NeedsAttention {
			needsReview++
		}
	}

	_, err := ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, ptrUint(uint(approvable+1))))
	require.ErrorIs(t, err, ErrReductionCountMismatch)
	require.Equal(t, r.Version, loadReductionRow(t, ctx.db, r.ID).Version, "a refused request does not move the version")
	var checked int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND checked", r.ID).Count(&checked).Error)
	var checkedBefore int64
	for _, c := range clusters {
		if c.Row.Checked {
			checkedBefore++
		}
	}
	require.Equal(t, checkedBefore, checked, "nothing was written on the mismatch")
	requireNoBulkMarks(t, ctx, r)

	res, err := ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, ptrUint(uint(approvable))))
	require.NoError(t, err)
	require.Equal(t, approvable, res.Changed)
	require.Equal(t, needsReview, res.NeedsReview)
	requireNoBulkMarks(t, ctx, r)
	var nearChecked int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND tier = 'near' AND checked", r.ID).Count(&nearChecked).Error)
	require.Zero(t, nearChecked, "allIdentical never reaches the Near tier")

	r = loadReductionRow(t, ctx.db, r.ID)
	res, err = ctx.storeBulkOverride(r, bulk(r, ReductionActionUncheck, nil, true, ptrUint(uint(approvable+checkedNow+needsReview))))
	require.NoError(t, err)
	require.Equal(t, approvable+checkedNow, res.Changed)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsBulkAllIdenticalSQLite(t *testing.T) {
	testOpsBulkAllIdentical(t, withSettings(newReductionSQLiteContext(t)))
}

// A confined principal: a Cluster reaching a Resource outside the subtree is
// withheld, so by id it is not found (not "not open"), and allIdentical neither
// counts nor writes it.
func testOpsBulkScoped(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	res := func(name string, owner uint) uint {
		return makeResource(t, ctx.db, name, opsRes{Hash: "h", Owner: &owner})
	}
	a, b := res("a", mine.ID), res("b", mine.ID)
	c, d := res("c", mine.ID), res("d", other.ID)
	e, f := res("e", other.ID), res("f", other.ID)
	insertOps(t, ctx, r, "visible", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	insertOps(t, ctx, r, "mixed", models.ReductionTierIdentical, c, opsMember(c, "h"), opsMember(d, "h"))
	insertOps(t, ctx, r, "theirs", models.ReductionTierIdentical, e, opsMember(e, "h"), opsMember(f, "h"))
	for _, key := range []string{"visible", "mixed", "theirs"} {
		require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = ? WHERE "key" = ?`, false, key).Error)
	}

	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	_, err := p.storeBulkOverride(r, bulk(r, ReductionActionCheck, []string{"visible", "mixed"}, false, nil))
	require.ErrorIs(t, err, ErrReductionClusterNotFound)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET state = 'skipped' WHERE "key" = 'mixed'`).Error)
	_, err = p.storeBulkOverride(r, bulk(r, ReductionActionCheck, []string{"visible", "mixed"}, false, nil))
	require.ErrorIs(t, err, ErrReductionClusterNotFound, "withheld is answered before the state of a Cluster the caller may not see")
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET state = 'open' WHERE "key" = 'mixed'`).Error)

	got, err := p.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, ptrUint(1)))
	require.NoError(t, err)
	require.Equal(t, 1, got.Changed)
	for key, want := range map[string]bool{"visible": true, "mixed": false, "theirs": false} {
		require.Equal(t, want, loadOps(t, ctx, loadReductionRow(t, ctx.db, r.ID), key).Row.Checked, key)
	}
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsBulkScopedSQLite(t *testing.T) {
	testOpsBulkScoped(t, withSettings(newReductionSQLiteContext(t)))
}

// The oracle: today's loop over randomized Clusters, copied from
// bulkOverrideReductionClustersOnce, against the table writer.
func testOpsBulkMatchesLegacyLoop(t *testing.T, ctx *MahresourcesContext, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for trial := 0; trial < 20; trial++ {
		r := newPlanReduction(t, ctx.db)
		n := 5 + rng.Intn(20)
		clusters := bulkInsert(t, ctx, r, n, func(i int, c *storedCluster) {
			c.Row.Checked = rng.Intn(2) == 0
			c.Row.Oversized = rng.Intn(5) == 0
			c.Row.KeepsSmaller = rng.Intn(6) == 0
			if rng.Intn(5) == 0 {
				c.Row.Lossy = []byte(`["series"]`)
			}
			if rng.Intn(5) == 0 {
				c.Members[1].Ejected = true
			}
			if rng.Intn(4) == 0 {
				c.Row.Tier = models.ReductionTierNear
			}
		})
		var targets []*models.ReductionCluster
		var keys []string
		for _, c := range clusters {
			if c.Row.Tier == models.ReductionTierIdentical {
				targets = append(targets, toLegacy(c))
				keys = append(keys, c.Row.Key)
			}
		}
		checking := rng.Intn(2) == 0
		action := ReductionActionUncheck
		if checking {
			action = ReductionActionCheck
		}
		counted := len(targets)
		if checking {
			counted = 0
			for _, cluster := range targets {
				if !cluster.Checked && !clusterNeedsAttention(cluster) && len(cluster.LoserIDs()) > 0 {
					counted++
				}
			}
		}
		wantChanged, wantNeeds := 0, 0
		for _, cluster := range targets {
			if checking && len(cluster.LoserIDs()) == 0 {
				continue
			}
			if checking && clusterNeedsAttention(cluster) {
				wantNeeds++
				continue
			}
			if cluster.Checked != checking {
				wantChanged++
			}
			cluster.Checked = checking
			cluster.Reviewed = true
		}
		label := fmt.Sprintf("seed %d trial %d %s", seed, trial, action)
		got, err := ctx.storeBulkOverride(r, bulk(r, action, nil, true, ptrUint(uint(counted))))
		require.NoError(t, err, label)
		require.Equal(t, wantChanged, got.Changed, label)
		require.Equal(t, wantNeeds, got.NeedsReview, label)
		r = loadReductionRow(t, ctx.db, r.ID)
		for _, cluster := range targets {
			stored := loadOps(t, ctx, r, cluster.ID)
			require.Equal(t, cluster.Checked, stored.Row.Checked, "%s %s checked", label, cluster.ID)
			require.Equal(t, cluster.Reviewed, stored.Row.Reviewed, "%s %s reviewed", label, cluster.ID)
		}
		_ = keys
		assertDerivedColumnsConsistent(t, ctx.db, r.ID)
	}
}

func TestOpsBulkMatchesLegacyLoopSQLite(t *testing.T) {
	for _, seed := range []int64{1, 2, 3} {
		testOpsBulkMatchesLegacyLoop(t, withSettings(newReductionSQLiteContext(t)), seed)
	}
}

// Every marking, counting and writing chunk is a keyset on idx_rc_bulk: its plan
// searches that index and neither sorts nor scans the table.
func testOpsBulkChunksAreKeysets(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	bulkInsert(t, ctx, r, 600, nil)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET bulk_op = 'op1' WHERE id % 3 = 0`).Error)
	if ctx.db.Dialector.Name() == "postgres" {
		require.NoError(t, ctx.db.Exec(`ANALYZE reduction_clusters`).Error)
	} else {
		require.NoError(t, ctx.db.Exec(`ANALYZE`).Error)
	}
	store := ctx.reductionStore()
	candidates, cargs := store.identicalCandidates(r, opsGen, 0)
	statements := map[string]struct {
		sql  string
		args []any
	}{
		"marking":  {candidates, cargs},
		"counting": {markedChunkSQL, []any{r.ID, opsGen, "op1", 0, reductionRowChunk}},
		"writing":  {writeMarkedSQL, []any{true, true, true, r.ID, opsGen, "op1", 0, 1000}},
	}
	for name, st := range statements {
		var plan []string
		if ctx.db.Dialector.Name() == "postgres" {
			require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
				// A test table this small is planned for cheap sorts and scans; what is
				// asserted is that the index can serve the keyset order, so the planner
				// is denied the alternatives.
				for _, off := range []string{"enable_seqscan", "enable_sort", "enable_bitmapscan"} {
					if err := tx.Exec(`SET LOCAL ` + off + ` = off`).Error; err != nil {
						return err
					}
				}
				return tx.Raw("EXPLAIN "+st.sql, st.args...).Scan(&plan).Error
			}), name)
		} else {
			var rows []struct{ Detail string }
			require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+st.sql, st.args...).Scan(&rows).Error, name)
			for _, row := range rows {
				plan = append(plan, row.Detail)
			}
		}
		text := strings.Join(plan, "\n")
		require.Contains(t, text, "idx_rc_bulk", "%s: %s", name, text)
		for _, bad := range []string{"TEMP B-TREE", "Sort", "Seq Scan", "SCAN reduction_clusters", "SCAN c\n", "SCAN c "} {
			require.NotContains(t, text, bad, "%s: %s", name, text)
		}
	}
}

func TestOpsBulkChunksAreKeysetsSQLite(t *testing.T) {
	testOpsBulkChunksAreKeysets(t, withSettings(newReductionSQLiteContext(t)))
}
