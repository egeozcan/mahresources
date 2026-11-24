package application_context

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// ---- 1: a file replaced between the star's load and its insertion ---------------------

// A Loser whose file is replaced after the star loaded it is not stored under the
// replacement's hash on the strength of the old pair. The replacement drops the
// perceptual hash first (OnResourceFileChanged), and the old pair row outlives it
// until the hash worker removes it.
func testR6ReplacedFileIsNotStoredUnderTheOldPair(t *testing.T, ctx *MahresourcesContext) {
	a := mkStreamRes(t, ctx.db, streamRes{Hash: "ha", W: 90, H: 90, Size: 10, Image: true})
	b := mkStreamRes(t, ctx.db, streamRes{Hash: "hb", W: 10, H: 10, Size: 10, Image: true})
	mkPair(t, ctx.db, a, b, 1)
	fired := 0
	computeBeforeInsertHook = func(tier string, attempt int) {
		if tier == models.ReductionTierNear && fired == 0 {
			fired++
			require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'replaced' WHERE id = ?", b).Error)
			require.NoError(t, ctx.db.Exec("DELETE FROM image_hashes WHERE resource_id = ?", b).Error)
		}
	}
	t.Cleanup(func() { computeBeforeInsertHook = nil })
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	streamCompute(t, ctx, r)
	require.Equal(t, 1, fired)
	for key, p := range planOf(t, ctx, r) {
		for _, m := range p.Members {
			require.NotEqual(t, b, m.ResourceID, "Cluster %s holds the replaced file on the old pair", key)
			require.NotEqual(t, "replaced", m.Hash)
		}
	}
}

// A hash that changed alone is rebuilt, never stored from the stale load: the
// rebuild reads the row as it is, so the stored hash is the one that was loaded with
// it and the insertion saw it unchanged.
func testR6ChangedHashIsRebuiltNotCarried(t *testing.T, ctx *MahresourcesContext) {
	a := mkStreamRes(t, ctx.db, streamRes{Hash: "ha", W: 90, H: 90, Size: 10, Image: true})
	b := mkStreamRes(t, ctx.db, streamRes{Hash: "hb", W: 10, H: 10, Size: 10, Image: true})
	mkPair(t, ctx.db, a, b, 1)
	var attempts []int
	computeBeforeInsertHook = func(tier string, attempt int) {
		if tier != models.ReductionTierNear {
			return
		}
		attempts = append(attempts, attempt)
		if len(attempts) == 1 {
			require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'replaced' WHERE id = ?", b).Error)
		}
	}
	t.Cleanup(func() { computeBeforeInsertHook = nil })
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	streamCompute(t, ctx, r)
	require.Equal(t, []int{0, 1}, attempts, "the first insertion was refused and the Cluster rebuilt")
	for _, p := range planOf(t, ctx, r) {
		for _, m := range p.Members {
			var live string
			require.NoError(t, ctx.db.Raw("SELECT hash FROM resources WHERE id = ?", m.ResourceID).Scan(&live).Error)
			require.Equal(t, live, m.Hash)
		}
	}
}

// A pair removed between the build and the insertion refuses the Cluster too.
func testR6VanishedPairRefusesTheCluster(t *testing.T, ctx *MahresourcesContext) {
	a := mkStreamRes(t, ctx.db, streamRes{Hash: "ha", W: 90, H: 90, Size: 10, Image: true})
	b := mkStreamRes(t, ctx.db, streamRes{Hash: "hb", W: 10, H: 10, Size: 10, Image: true})
	mkPair(t, ctx.db, a, b, 1)
	fired := 0
	computeBeforeInsertHook = func(tier string, attempt int) {
		if tier == models.ReductionTierNear && fired == 0 {
			fired++
			require.NoError(t, ctx.db.Exec("DELETE FROM resource_similarities").Error)
		}
	}
	t.Cleanup(func() { computeBeforeInsertHook = nil })
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	streamCompute(t, ctx, r)
	require.Empty(t, planOf(t, ctx, r), "no Cluster rests on a pair that no longer exists")
}

func TestR6ReplacedFileIsNotStoredUnderTheOldPair(t *testing.T) {
	testR6ReplacedFileIsNotStoredUnderTheOldPair(t, withSettings(newReductionSQLiteContext(t)))
}
func TestR6ChangedHashIsRebuiltNotCarried(t *testing.T) {
	testR6ChangedHashIsRebuiltNotCarried(t, withSettings(newReductionSQLiteContext(t)))
}
func TestR6VanishedPairRefusesTheCluster(t *testing.T) {
	testR6VanishedPairRefusesTheCluster(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 2: a retry recomputes membership from the live Extent -----------------------------

func testR6UnlinkedMemberLeavesTheRestAsACluster(t *testing.T, ctx *MahresourcesContext) {
	a := mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 30, H: 30})
	b := mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 20, H: 20})
	c := mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 10, H: 10})
	fired := 0
	computeBeforeInsertHook = func(tier string, attempt int) {
		if fired == 0 {
			fired++
			require.NoError(t, ctx.db.Exec("DELETE FROM reduction_extent_resources WHERE resource_id = ?", c).Error)
		}
	}
	t.Cleanup(func() { computeBeforeInsertHook = nil })
	r := mkStreamReduction(t, ctx, streamSel{Resources: []uint{a, b, c}, ExcludeExt: true, Identical: true})
	streamCompute(t, ctx, r)
	rows := streamRows(t, ctx, r, models.ReductionTierIdentical)
	require.Len(t, rows, 1, "the two files still in the Extent form a Cluster")
	require.ElementsMatch(t, []uint{a, b}, rows[0].ids())
}

func TestR6UnlinkedMemberLeavesTheRestAsACluster(t *testing.T) {
	testR6UnlinkedMemberLeavesTheRestAsACluster(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 3: only a reviewer's ejection reserves a member --------------------------------------

func testR6OnlyManualEjectionReserves(t *testing.T, ctx *MahresourcesContext) {
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	const gen = "r6-gen"
	cluster := &models.ReductionClusterRow{ReductionID: r.ID, Generation: gen, Key: "k", State: models.ReductionClusterOpen}
	require.NoError(t, ctx.db.Create(cluster).Error)
	reasons := map[string]string{"manual": models.EjectReasonManual, "legacy": "", "nopair": models.EjectReasonNoPairToWinner, "demoted": models.EjectReasonOutsiderDemoted}
	ids := map[string]uint{}
	for name, reason := range reasons {
		id := mkStreamRes(t, ctx.db, streamRes{Hash: name})
		ids[name] = id
		require.NoError(t, ctx.db.Create(&models.ReductionMemberRow{ClusterID: cluster.ID, ReductionID: r.ID, Generation: gen, ResourceID: id, Ejected: true, EjectedReason: reason}).Error)
	}
	// A cluster that is not carried history reserves its members only while they are
	// ejected by someone; an open one is rebuilt, so only the ejection can hold them.
	require.NoError(t, ctx.db.Exec("UPDATE reduction_clusters SET read_only = ? WHERE id = ?", true, cluster.ID).Error)
	run := &reductionStream{ctx: ctx, s: ctx.reductionStore(), r: r, generation: gen, jobCtx: context.Background()}
	for name, want := range map[string]bool{"manual": false, "legacy": false, "nopair": true, "demoted": true} {
		got, err := run.isAvailable(ids[name])
		require.NoError(t, err)
		require.Equal(t, want, got, "%s ejection: available", name)
	}
}

func TestR6OnlyManualEjectionReserves(t *testing.T) {
	testR6OnlyManualEjectionReserves(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 4: the carry-over's member page seeks by cluster id ---------------------------------

func TestR6CarryMemberPageDoesNotRebuildTheClusterSet(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	var plan []string
	var rows []struct {
		ID, Parent, Notused int
		Detail              string
	}
	require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+carryMemberPageSQL, []uint{1, 2, 3}, 0, 0, -1, reductionRowChunk).Scan(&rows).Error)
	for _, row := range rows {
		plan = append(plan, row.Detail)
	}
	text := strings.Join(plan, "\n")
	require.Contains(t, text, "idx_rm_page", "the page is read off the (cluster, position) index:\n%s", text)
	require.NotContains(t, text, "SUBQUERY", "no statement rebuilds the Cluster set per page:\n%s", text)
	require.NotContains(t, text, "SCAN", "no table scan:\n%s", text)
	_ = r
}

// ---- 5: tie discovery walks the seeds in chunks ------------------------------------------

func testR6TieDiscoveryIsIncremental(t *testing.T, ctx *MahresourcesContext) {
	prev := computeChunk
	computeChunk = 2
	t.Cleanup(func() { computeChunk = prev })
	// Prefixes: three seeds sharing one (spanning a page boundary at chunk 2), one
	// alone, two sharing another. Names equal so the key prefix is identical.
	r := mkStreamReduction(t, ctx, streamSel{All: true})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	var ids []uint
	mk := func(prefix string, n int) {
		for i := 0; i < n; i++ {
			id := mkStreamRes(t, ctx.db, streamRes{Hash: fmt.Sprintf("%s%d", prefix, i)})
			ids = append(ids, id)
			require.NoError(t, ctx.db.Create(&models.ReductionSeed{ReductionID: cur.ID, Generation: gen, ResourceID: id,
				KeyPrefix: []byte(prefix), SortKey: []byte(fmt.Sprintf("%s-%03d", prefix, 100-i))}).Error)
		}
	}
	mk("a", 3)
	mk("b", 1)
	mk("c", 2)
	var statements []string
	record := func(db *gorm.DB) { statements = append(statements, db.Statement.SQL.String()) }
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register("test:r6_ties_raw", record))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register("test:r6_ties_row", record))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Raw().Remove("test:r6_ties_raw")
		_ = ctx.db.Callback().Row().Remove("test:r6_ties_row")
	})
	run := &reductionStream{ctx: ctx, s: ctx.reductionStore(), r: cur, generation: gen, jobCtx: context.Background()}
	require.NoError(t, run.rankTiedSeeds())
	require.NotEmpty(t, statements)
	for _, s := range statements {
		require.NotContains(t, strings.ToUpper(s), "HAVING", "no statement groups the whole seed generation: %s", s)
	}
	rank := map[uint]int64{}
	var rows []struct {
		ResourceID uint
		TieRank    int64
	}
	require.NoError(t, ctx.db.Raw("SELECT resource_id, tie_rank FROM reduction_seeds WHERE reduction_id = ? AND generation = ?", cur.ID, gen).Scan(&rows).Error)
	for _, row := range rows {
		rank[row.ResourceID] = row.TieRank
	}
	// Within a group the rank follows sort_key: the highest i has the lowest key.
	require.Equal(t, int64(3), rank[ids[0]])
	require.Equal(t, int64(2), rank[ids[1]])
	require.Equal(t, int64(1), rank[ids[2]])
	require.Equal(t, int64(0), rank[ids[3]], "a seed with a prefix of its own keeps rank 0")
	require.Equal(t, int64(2), rank[ids[4]])
	require.Equal(t, int64(1), rank[ids[5]])
}

func TestR6TieDiscoveryIsIncremental(t *testing.T) {
	testR6TieDiscoveryIsIncremental(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 6: an expired compute generation is failed and swept --------------------------------

func testR6ExpiredComputeGenerationIsSwept(t *testing.T, ctx *MahresourcesContext) {
	seed := func(r *models.ResourceReduction, gen string) {
		res := mkStreamRes(t, ctx.db, streamRes{Hash: "x" + gen})
		require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_extent (reduction_id, generation, resource_id, hash) VALUES (?, ?, ?, 'h')`, r.ID, gen, res).Error)
		c := &models.ReductionClusterRow{ReductionID: r.ID, Generation: gen, Key: "k", State: models.ReductionClusterOpen}
		require.NoError(t, ctx.db.Create(c).Error)
		require.NoError(t, ctx.db.Create(&models.ReductionMemberRow{ClusterID: c.ID, ReductionID: r.ID, Generation: gen, ResourceID: res}).Error)
	}
	dead := mkStreamReduction(t, ctx, streamSel{All: true})
	live := mkStreamReduction(t, ctx, streamSel{All: true})
	seed(dead, "dead-gen")
	seed(live, "live-gen")
	require.NoError(t, ctx.db.Exec(`UPDATE resource_reductions SET status = 'computing', compute_generation = 'dead-gen', compute_lease_until = 1 WHERE id = ?`, dead.ID).Error)
	require.NoError(t, ctx.db.Exec(`UPDATE resource_reductions SET status = 'computing', compute_generation = 'live-gen', compute_lease_until = ? WHERE id = ?`,
		int64(1)<<62, live.ID).Error)

	require.NoError(t, ctx.sweepReductionGenerations())

	got := loadReductionRow(t, ctx.db, dead.ID)
	require.Equal(t, models.ReductionStatusFailed, got.Status)
	require.Nil(t, got.ComputeGeneration)
	var n int64
	for _, table := range []string{"reduction_extent", "reduction_clusters", "reduction_members"} {
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM `+table+` WHERE reduction_id = ? AND generation = 'dead-gen'`, dead.ID).Scan(&n).Error)
		require.Zero(t, n, "%s of the expired generation", table)
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM `+table+` WHERE reduction_id = ? AND generation = 'live-gen'`, live.ID).Scan(&n).Error)
		require.NotZero(t, n, "%s of a live compute stays", table)
	}
	alive := loadReductionRow(t, ctx.db, live.ID)
	require.Equal(t, models.ReductionStatusComputing, alive.Status)
}

func TestR6ExpiredComputeGenerationIsSwept(t *testing.T) {
	testR6ExpiredComputeGenerationIsSwept(t, withSettings(newReductionSQLiteContext(t)))
}
