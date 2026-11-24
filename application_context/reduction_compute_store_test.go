package application_context

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// ---- fixtures -----------------------------------------------------------------------

// seedAt writes a Cluster of the given generation with the given Row fields set.
func seedAt(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, generation string, row models.ReductionClusterRow, members ...models.ReductionMemberRow) {
	t.Helper()
	c := &storedCluster{Row: row, Members: members}
	require.NoError(t, ctx.reductionStore().insertCluster(r, generation, c))
}

func clusterRowsOf(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, generation string) map[string]models.ReductionClusterRow {
	t.Helper()
	var rows []models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ? AND generation = ?", r.ID, generation).Find(&rows).Error)
	out := map[string]models.ReductionClusterRow{}
	for _, row := range rows {
		out[row.Key] = row
	}
	return out
}

func planRows(t *testing.T, ctx *MahresourcesContext, table string, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, ctx.db.Raw("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n).Error)
	return n
}

// computing puts the Reduction through beginReductionComputeStore and admits the lease,
// as a queued job's start does.
func computing(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) (jobID, generation string, version uint) {
	t.Helper()
	nonce, gen, err := ctx.beginReductionComputeStore(r.ID, r.Version)
	require.NoError(t, err)
	require.NoError(t, ctx.storeAdmitComputeLease(r.ID, gen))
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_job_id = ? WHERE id = ?", "job-1", r.ID).Error)
	_ = nonce
	return "job-1", gen, loadReductionRow(t, ctx.db, r.ID).Version
}

func freshCluster(key string, winner uint, losers ...uint) *models.ReductionCluster {
	c := &models.ReductionCluster{ID: key, Tier: models.ReductionTierIdentical, WinnerID: winner, State: models.ReductionClusterOpen, Checked: true}
	c.Members = append(c.Members, &models.ReductionMember{ResourceID: winner, Hash: "h", InExtent: true})
	for _, id := range losers {
		c.Members = append(c.Members, &models.ReductionMember{ResourceID: id, Hash: "h", InExtent: true})
	}
	return c
}

// ---- landing ---------------------------------------------------------------------------

func testStoreLandCarriesEverythingAndKeepsDecisions(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	res := func(name string) uint { return makeResource(t, ctx.db, name, opsRes{Hash: "h"}) }

	// the plan being replaced: one Cluster per carried reason, and one that is not carried
	merged := models.ReductionClusterRow{Key: "applied", Tier: models.ReductionTierIdentical, State: models.ReductionClusterApplied, Merged: true, Reviewed: true, Position: 1,
		Lossy: []byte(`["description"]`), FixedSeed: true, SeedResourceID: 9}
	aw, al := res("a-w"), res("a-l")
	seedAt(t, ctx, r, opsGen, merged, opsMember(aw, "h"), opsMember(al, "h"))
	rw, rl := res("r-w"), res("r-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "reviewed", Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen, Reviewed: true, Checked: true, Position: 2}, opsMember(rw, "h"), opsMember(rl, "h"))
	sw, sl := res("s-w"), res("s-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "skipped", Tier: models.ReductionTierNear, State: models.ReductionClusterSkipped, Position: 3}, opsMember(sw, "h"), opsMember(sl, "h"))
	tw, tl := res("t-w"), res("t-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "stale", Tier: models.ReductionTierIdentical, State: models.ReductionClusterStale, StaleReason: "x", Position: 4}, opsMember(tw, "h"), opsMember(tl, "h"))
	hw, hl := res("h-w"), res("h-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "history", Tier: models.ReductionTierNear, State: models.ReductionClusterOpen, ReadOnly: true, Position: 5}, opsMember(hw, "h"), opsMember(hl, "h"))
	cw, cl := res("c-w"), res("c-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "claimed", Tier: models.ReductionTierIdentical, State: models.ReductionClusterApplied, ClaimID: "claim", Position: 6}, opsMember(cw, "h"), opsMember(cl, "h"))
	ow, ol := res("o-w"), res("o-l")
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "unreviewed", Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen, Checked: true, Position: 7}, opsMember(ow, "h"), opsMember(ol, "h"))
	// decisions only a carry can keep, on every column the write paths touch
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET outcome_batch = 'ob', apply_batch = 'ab', approved_winner = ?, approved_seq = 3, restore_seq = 3,
		bulk_op = 'op', truncated = 4, decided_by = 'pixels', margin = '2x' WHERE "key" = 'applied'`, aw).Error)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_members SET owner_at_merge = 77, changed = true WHERE resource_id = ?`, al).Error)
	// a deletion since the plan was written
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", rl).Error)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET reduction_id = reduction_id WHERE 1 = 0`).Error)
	before := clusterRowsOf(t, ctx, r, opsGen)
	// staging rows of this and of a dead generation
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentRow{ReductionID: r.ID, Generation: gen, ResourceID: aw, Hash: "h"}).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionSeed{ReductionID: r.ID, Generation: gen, ResourceID: aw, KeyPrefix: []byte{1}, SortKey: []byte{1}}).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentRow{ReductionID: r.ID, Generation: "dead", ResourceID: aw, Hash: "h"}).Error)

	// the unreviewed Cluster is rebuilt (same members, so the same key); a fresh one
	// collides with a carried key; one holds a member of read-only history
	fresh := []*models.ReductionCluster{
		freshCluster("unreviewed", ow, ol),
		freshCluster("applied", res("n1-w"), res("n1-l")),
		freshCluster("over-history", res("n2-w"), hl),
	}
	require.NoError(t, ctx.storeLandComputedPlan(r, jobID, gen, version, fresh, models.ReductionCoverage{ExtentSize: 14, ContentHashed: 14},
		&models.ReductionSettings{MatchingMode: "both", WinnerRule: []string{"pixels_desc"}}))

	row := loadReductionRow(t, ctx.db, r.ID)
	require.NotNil(t, row.PlanGeneration)
	require.Equal(t, gen, *row.PlanGeneration)
	require.Nil(t, row.ComputeGeneration)
	require.Equal(t, models.ReductionStatusReady, row.Status)
	require.Equal(t, version+1, row.Version)
	var coverage models.ReductionCoverage
	require.NoError(t, json.Unmarshal(row.Coverage, &coverage))
	require.Equal(t, 14, coverage.ExtentSize)
	require.NotEmpty(t, row.PlanSettings)

	after := clusterRowsOf(t, ctx, r, gen)
	require.Len(t, after, 9, "six carried and three fresh")
	for _, key := range []string{"applied", "reviewed", "skipped", "stale", "history", "claimed"} {
		want := before[key]
		got := after[key]
		require.NotEqual(t, want.ID, got.ID, key)
		want.ID, got.ID, want.Generation, got.Generation = 0, 0, "", ""
		if key == "reviewed" {
			// the deleted member is counted at the copy, from the locked read
			require.Equal(t, 1, got.GoneMembers)
			want.GoneMembers = 1
		}
		require.Equal(t, want, got, "every column of %s survives a compute", key)
	}
	require.Equal(t, "ob", after["applied"].OutcomeBatch, "the LastApply evidence survives a compute")
	var owner uint
	require.NoError(t, ctx.db.Raw(`SELECT owner_at_merge FROM reduction_members WHERE cluster_id = ? AND resource_id = ?`, after["applied"].ID, al).Scan(&owner).Error)
	require.Equal(t, uint(77), owner)
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_members", "cluster_id = ? AND resource_id = ? AND gone", after["reviewed"].ID, rl))
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_members", "cluster_id = ? AND resource_id = ? AND changed", after["applied"].ID, al))

	// the unreviewed Cluster arrives as rebuilt, under its own key
	require.Equal(t, 1, after["unreviewed"].LoserCount)
	// a key that collides with a carried one takes a numbered suffix
	require.Contains(t, after, "applied-2")
	require.True(t, after["applied"].Merged)
	require.False(t, after["applied-2"].Merged)
	// arriving next to read-only history, it needs fresh approval
	require.True(t, after["unreviewed"].Checked)
	require.False(t, after["over-history"].Checked)
	for _, key := range []string{"unreviewed", "applied-2", "over-history"} {
		require.Greater(t, after[key].Position, int64(6), "fresh Clusters follow the carried ones")
	}

	// the old generation, the dead staging and this generation's staging are gone
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, opsGen))
	require.Zero(t, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation = ?", r.ID, opsGen))
	require.Zero(t, planRows(t, ctx, "reduction_extent", "reduction_id = ?", r.ID))
	require.Zero(t, planRows(t, ctx, "reduction_seeds", "reduction_id = ?", r.ID))
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreLandCarryIsChunked(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	const n = reductionRowChunk + 20
	owners := map[string][2]uint{}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("k%04d", i)
		w, l := makeResource(t, ctx.db, key+"w", opsRes{Hash: "h"}), makeResource(t, ctx.db, key+"l", opsRes{Hash: "h"})
		owners[key] = [2]uint{w, l}
		seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterSkipped, Position: int64(i + 1), WinnerID: w},
			opsMember(w, "h"), opsMember(l, "h"))
	}
	// one deleted late member in the second chunk
	gone := owners["k0510"][1]
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", gone).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)
	require.NoError(t, ctx.storeLandComputedPlan(r, jobID, gen, version, nil, models.ReductionCoverage{}, nil))

	after := clusterRowsOf(t, ctx, r, gen)
	require.Len(t, after, n)
	require.EqualValues(t, n*2, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation = ?", r.ID, gen))
	for key, ids := range owners {
		var got []uint
		require.NoError(t, ctx.db.Raw(`SELECT resource_id FROM reduction_members WHERE cluster_id = ? ORDER BY position`, after[key].ID).Scan(&got).Error)
		require.Equal(t, []uint{ids[0], ids[1]}, got, "members of %s follow their Cluster, in order", key)
	}
	require.Equal(t, 1, after["k0510"].GoneMembers)
	require.Equal(t, 0, after["k0001"].GoneMembers)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreLandRefusals(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	w, l := pairAt(t, ctx, r, "old", 1, nil)
	_ = w
	_ = l

	// the version moved under the compute: refused, not rebased, the old plan stands
	jobID, gen, version := computing(t, ctx, r)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET version = version + 1 WHERE id = ?", r.ID).Error)
	nw, nl := makeResource(t, ctx.db, "n-w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "n-l", opsRes{Hash: "h"})
	err := ctx.storeLandComputedPlan(r, jobID, gen, version, []*models.ReductionCluster{freshCluster("new", nw, nl)}, models.ReductionCoverage{}, nil)
	require.ErrorIs(t, err, ErrReductionStaleCompute)
	row := loadReductionRow(t, ctx.db, r.ID)
	require.Equal(t, opsGen, *row.PlanGeneration, "the plan the compute was about stands")
	require.Equal(t, models.ReductionStatusComputing, row.Status)
	// the run records its failure; what it staged is swept, the old plan is untouched
	require.NoError(t, ctx.storeFailCompute(r.ID, jobID, gen, err))
	require.NoError(t, ctx.sweepReductionGenerations())
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, gen))
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, opsGen))
	require.Equal(t, models.ReductionStatusFailed, loadReductionRow(t, ctx.db, r.ID).Status)

	// a job that is not the one the row names
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version = computing(t, ctx, r)
	err = ctx.storeLandComputedPlan(r, "some-other-job", gen, version, nil, models.ReductionCoverage{}, nil)
	require.ErrorIs(t, err, ErrReductionComputeSuperseded)
	require.Equal(t, opsGen, *loadReductionRow(t, ctx.db, r.ID).PlanGeneration)

	// a newer compute replaced this one (its lease ran out, a second begin took over)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_lease_until = 0 WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	_, gen2, err2 := ctx.beginReductionComputeStore(r.ID, r.Version)
	require.NoError(t, err2)
	err = ctx.storeLandComputedPlan(r, jobID, gen, version, []*models.ReductionCluster{freshCluster("late", nw, nl)}, models.ReductionCoverage{}, nil)
	require.ErrorIs(t, err, ErrReductionLeaseLost, "the superseded run's fence refuses it before it writes")
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, gen))
	require.NotEqual(t, gen, gen2)
	row = loadReductionRow(t, ctx.db, r.ID)
	require.Equal(t, gen2, *row.ComputeGeneration)

	// an expired lease writes nothing
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_lease_until = ? WHERE id = ?", time.Now().Add(-time.Second).UnixNano(), r.ID).Error)
	err = ctx.storeLandComputedPlan(r, "", gen2, row.Version, []*models.ReductionCluster{freshCluster("expired", nw, nl)}, models.ReductionCoverage{}, nil)
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, gen2))
	require.Equal(t, opsGen, *loadReductionRow(t, ctx.db, r.ID).PlanGeneration)
}

func testStoreLandStopsWhenTheReductionIsDeleted(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "frozen", Tier: models.ReductionTierIdentical, State: models.ReductionClusterSkipped, Position: 1},
		opsMember(makeResource(t, ctx.db, "f1", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, "f2", opsRes{Hash: "h"}), "h"))
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)
	w, l := makeResource(t, ctx.db, "n-w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "n-l", opsRes{Hash: "h"})
	// deleted after the carry and before the first fresh Cluster
	reductionLandHook = func() { require.NoError(t, ctx.storeDeleteReduction(r)) }
	t.Cleanup(func() { reductionLandHook = nil })
	err := ctx.storeLandComputedPlan(r, jobID, gen, version, []*models.ReductionCluster{freshCluster("new", w, l)}, models.ReductionCoverage{}, nil)
	reductionLandHook = nil
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ?", r.ID), "no insert lands after the delete")
	require.Zero(t, planRows(t, ctx, "reduction_members", "reduction_id = ?", r.ID))
}

func testStoreBeginComputeRefusals(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	now := time.Now()
	_, _, err := ctx.beginReductionComputeStore(r.ID, r.Version+9)
	require.ErrorIs(t, err, ErrReductionConflict)

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", now.Add(time.Minute).UnixNano(), r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	_, _, err = ctx.beginReductionComputeStore(r.ID, r.Version)
	require.ErrorIs(t, err, ErrReductionConflict, "a live apply refuses a compute")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0, settle_op = 'op', settle_lease_until = ? WHERE id = ?", now.Add(time.Minute).UnixNano(), r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	_, _, err = ctx.beginReductionComputeStore(r.ID, r.Version)
	require.ErrorIs(t, err, ErrReductionConflict, "a live settle refuses a compute")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_op = NULL, settle_lease_until = 0, plan_schema = 0 WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	_, _, err = ctx.beginReductionComputeStore(r.ID, r.Version)
	require.ErrorIs(t, err, ErrReductionBusy, "a legacy JSON plan is migrated first")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1 WHERE id = ?", r.ID).Error)

	r = loadReductionRow(t, ctx.db, r.ID)
	nonce, gen, err := ctx.beginReductionComputeStore(r.ID, r.Version)
	require.NoError(t, err)
	require.Contains(t, nonce, "pending:")
	row := loadReductionRow(t, ctx.db, r.ID)
	require.Equal(t, models.ReductionStatusComputing, row.Status)
	require.Equal(t, gen, *row.ComputeGeneration)
	require.Greater(t, row.ComputeLeaseUntil, now.UnixNano(), "a pending reservation holds until a worker admits the job")
	_, _, err = ctx.beginReductionComputeStore(r.ID, row.Version)
	require.ErrorIs(t, err, ErrReductionBusy, "a second start while one is live")

	// the same fence refuses a settle and an apply while it computes
	require.ErrorIs(t, ctx.storeAdmitComputeLease(r.ID, "another-generation"), ErrReductionLeaseLost)
	require.NoError(t, ctx.storeAdmitComputeLease(r.ID, gen))
}

// ---- sweep -------------------------------------------------------------------------------

func testStoreSweep(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx) // plan generation opsGen
	seed := func(red *models.ResourceReduction, gen, key string) {
		seedAt(t, ctx, red, gen, models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
			opsMember(makeResource(t, ctx.db, key+"-a", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, key+"-b", opsRes{Hash: "h"}), "h"))
	}
	seed(r, opsGen, "plan")
	seed(r, "running", "running")
	seed(r, "migrating", "migrating")
	seed(r, "stale", "stale")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'computing', compute_generation = 'running', compute_lease_until = ? WHERE id = ?", time.Now().Add(time.Hour).UnixNano(), r.ID).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionMigration{ReductionID: r.ID, Generation: "migrating"}).Error)
	// a Reduction that never landed a plan (plan_generation NULL) and whose first compute failed
	first := &models.ResourceReduction{Name: "first", Status: models.ReductionStatusFailed, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, ctx.db.Create(first).Error)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1 WHERE id = ?", first.ID).Error)
	seed(first, "failed-first", "failed")
	// rows of a Reduction that no longer exists
	ghost := &models.ResourceReduction{ID: 424242}
	seed(ghost, "ghost", "ghost")
	require.NoError(t, ctx.db.Create(&models.ReductionExtentResource{ReductionID: ghost.ID, ResourceID: 1}).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: ghost.ID, GroupID: 1}).Error)
	// staging: the running compute's survives, a dead one's and the plan generation's do not
	for _, gen := range []string{"running", "dead", opsGen} {
		require.NoError(t, ctx.db.Create(&models.ReductionExtentRow{ReductionID: r.ID, Generation: gen, ResourceID: 1, Hash: "h"}).Error)
		require.NoError(t, ctx.db.Create(&models.ReductionSeed{ReductionID: r.ID, Generation: gen, ResourceID: 1, KeyPrefix: []byte{1}, SortKey: []byte{1}}).Error)
	}

	require.NoError(t, ctx.sweepReductionGenerations())
	exists := func(red uint, gen string) bool {
		return planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", red, gen) > 0
	}
	require.True(t, exists(r.ID, opsGen))
	require.True(t, exists(r.ID, "running"), "a running compute's generation is protected by its column")
	require.True(t, exists(r.ID, "migrating"), "a migration's by its row")
	require.False(t, exists(r.ID, "stale"))
	require.False(t, exists(first.ID, "failed-first"), "a NULL plan_generation protects nothing")
	require.False(t, exists(ghost.ID, "ghost"))
	require.Zero(t, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation IN ('stale', 'ghost')", r.ID))
	require.Zero(t, planRows(t, ctx, "reduction_members", "reduction_id = ?", first.ID))
	require.EqualValues(t, 2, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation = ?", r.ID, opsGen))
	require.Zero(t, planRows(t, ctx, "reduction_extent_resources", "reduction_id = ?", ghost.ID))
	require.Zero(t, planRows(t, ctx, "reduction_extent_groups", "reduction_id = ?", ghost.ID))
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_extent", "generation = 'running'"))
	require.Zero(t, planRows(t, ctx, "reduction_extent", "generation IN ('dead', ?)", opsGen),
		"staging is protected only by a running compute, so a crash right after landing leaves rows the next sweep removes")
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_seeds", "generation = 'running'"))
	require.Zero(t, planRows(t, ctx, "reduction_seeds", "generation IN ('dead', ?)", opsGen))
}

func testStoreSweepVersusComputeStart(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	seedAt(t, ctx, r, "replayed", models.ReductionClusterRow{Key: "mine", Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
		opsMember(makeResource(t, ctx.db, "m-a", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, "m-b", opsRes{Hash: "h"}), "h"))
	// the sweep has read the stale generation's ids when a compute starts under that
	// generation (a replayed job keeps its generation); the delete must find it protected
	calls := 0
	sweepBetweenReadAndDeleteHook = func() {
		calls++
		require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'computing', compute_generation = 'replayed' WHERE id = ?", r.ID).Error)
	}
	t.Cleanup(func() { sweepBetweenReadAndDeleteHook = nil })
	require.NoError(t, ctx.sweepReductionGenerations())
	sweepBetweenReadAndDeleteHook = nil
	require.Greater(t, calls, 0, "the sweep did read the generation as stale")
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = 'replayed'", r.ID))
	require.EqualValues(t, 2, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation = 'replayed'", r.ID))
}

func testStoreSweepFirstComputeFails(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1, status = 'draft' WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, _ := computing(t, ctx, r)
	w, l := makeResource(t, ctx.db, "w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "l", opsRes{Hash: "h"})
	// what the failed run had staged
	require.NoError(t, ctx.reductionStore().insertCluster(r, gen, &storedCluster{
		Row: models.ReductionClusterRow{Key: "half", Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen}, Members: []models.ReductionMemberRow{opsMember(w, "h"), opsMember(l, "h")}}))
	require.NoError(t, ctx.sweepReductionGenerations())
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_clusters", "reduction_id = ?", r.ID), "protected while it computes")

	require.NoError(t, ctx.storeFailCompute(r.ID, jobID, gen, fmt.Errorf("boom")))
	row := loadReductionRow(t, ctx.db, r.ID)
	require.Equal(t, models.ReductionStatusFailed, row.Status)
	require.Nil(t, row.ComputeGeneration)
	require.Nil(t, row.PlanGeneration)
	require.Equal(t, "boom", row.ComputeError)
	require.NoError(t, ctx.sweepReductionGenerations())
	require.Zero(t, planRows(t, ctx, "reduction_clusters", "reduction_id = ?", r.ID), "a first compute that fails leaves nothing behind")
	require.Zero(t, planRows(t, ctx, "reduction_members", "reduction_id = ?", r.ID))
	// a superseded run records nothing over the newer one
	r = loadReductionRow(t, ctx.db, r.ID)
	_, gen2, err := ctx.beginReductionComputeStore(r.ID, r.Version)
	require.NoError(t, err)
	require.NoError(t, ctx.storeFailCompute(r.ID, "", gen, fmt.Errorf("late")))
	require.Equal(t, models.ReductionStatusComputing, loadReductionRow(t, ctx.db, r.ID).Status)
	require.Equal(t, gen2, *loadReductionRow(t, ctx.db, r.ID).ComputeGeneration)
}

func testStoreDeleteReduction(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	for i := 0; i < reductionRowChunk+5; i++ {
		key := fmt.Sprintf("k%04d", i)
		seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
			opsMember(makeResource(t, ctx.db, key+"a", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, key+"b", opsRes{Hash: "h"}), "h"))
	}
	other := newPlanReduction(t, ctx.db)
	seedAt(t, ctx, other, opsGen, models.ReductionClusterRow{Key: "keep", Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
		opsMember(makeResource(t, ctx.db, "ka", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, "kb", opsRes{Hash: "h"}), "h"))
	require.NoError(t, ctx.db.Create(&models.ReductionExtentResource{ReductionID: r.ID, ResourceID: 1}).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentRow{ReductionID: r.ID, Generation: "g", ResourceID: 1}).Error)

	require.NoError(t, ctx.storeDeleteReduction(r))
	require.Zero(t, planRows(t, ctx, "resource_reductions", "id = ?", r.ID))
	for _, table := range []string{"reduction_clusters", "reduction_members", "reduction_extent_resources", "reduction_extent"} {
		require.Zero(t, planRows(t, ctx, table, "reduction_id = ?", r.ID), table)
	}
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_clusters", "reduction_id = ?", other.ID), "another Reduction's rows are untouched")
	require.ErrorIs(t, ctx.storeDeleteReduction(r), ErrReductionNotFound)
}

func testStoreComputeSeams(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	w, l := makeResource(t, ctx.db, "w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "l", opsRes{Hash: "h"})
	plan := models.ResourceReductionPlan{
		Clusters: []*models.ReductionCluster{freshCluster("one", w, l)},
		Coverage: models.ReductionCoverage{ExtentSize: 2},
		Settings: &models.ReductionSettings{MatchingMode: "identical"},
	}
	require.NoError(t, ctx.OverwritePlanForTest(r.ID, r.Version, plan))
	row := loadReductionRow(t, ctx.db, r.ID)
	require.NotNil(t, row.PlanGeneration)
	require.Equal(t, uint(1), countUint(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, *row.PlanGeneration))
	require.ErrorIs(t, ctx.OverwritePlanForTest(r.ID, r.Version, plan), ErrReductionConflict)

	// the claim and release halves, under a batch the test makes live
	rec := fmt.Sprintf(`{"batchId":"b1","generation":%q,"startedAt":"2026-11-24T00:00:00Z"}`, *row.PlanGeneration)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET last_apply = ?, apply_lease_until = ? WHERE id = ?", rec, time.Now().Add(time.Hour).UnixNano(), r.ID).Error)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET apply_batch = 'b1', approved_winner = winner_id, approved_seq = restore_seq WHERE "key" = 'one'`).Error)
	claimed := ctx.ClaimClusterForApplyForTest(r.ID, "b1", "one")
	require.NotNil(t, claimed)
	require.Equal(t, models.ReductionClusterApplied, claimed.State)
	require.Equal(t, []uint{l}, claimed.LoserIDs())
	require.Nil(t, ctx.ClaimClusterForApplyForTest(r.ID, "b1", "one"), "claimed once")
	ctx.ReleaseClaimForTest(r.ID, "b1", "one", "not-the-claim")
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "one").State, "a stale claim id changes nothing")
	ctx.ReleaseClaimForTest(r.ID, "b1", "one", claimed.ClaimID)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "one").State)

	// landing a plan wholesale needs a compute underway
	r = loadReductionRow(t, ctx.db, r.ID)
	require.ErrorIs(t, ctx.StoreReductionPlanForTest(r.ID, "job-1", r.Version, plan), ErrReductionComputeSuperseded)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0 WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)
	replacement := models.ResourceReductionPlan{Clusters: []*models.ReductionCluster{freshCluster("two", w, l)}}
	require.NoError(t, ctx.StoreReductionPlanForTest(r.ID, jobID, version, replacement))
	require.Equal(t, gen, *loadReductionRow(t, ctx.db, r.ID).PlanGeneration)
	require.EqualValues(t, 1, planRows(t, ctx, "reduction_clusters", `reduction_id = ? AND "key" = 'two'`, r.ID))
	require.Zero(t, planRows(t, ctx, "reduction_clusters", `reduction_id = ? AND "key" = 'one'`, r.ID), "the replaced plan is swept")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func countUint(t *testing.T, ctx *MahresourcesContext, table, where string, args ...any) uint {
	return uint(planRows(t, ctx, table, where, args...))
}

func TestStoreLandCarriesEverythingSQLite(t *testing.T) {
	testStoreLandCarriesEverythingAndKeepsDecisions(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreLandCarryIsChunkedSQLite(t *testing.T) {
	testStoreLandCarryIsChunked(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreLandRefusalsSQLite(t *testing.T) {
	testStoreLandRefusals(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreLandStopsWhenTheReductionIsDeletedSQLite(t *testing.T) {
	testStoreLandStopsWhenTheReductionIsDeleted(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreBeginComputeRefusalsSQLite(t *testing.T) {
	testStoreBeginComputeRefusals(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreSweepSQLite(t *testing.T) {
	testStoreSweep(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreSweepVersusComputeStartSQLite(t *testing.T) {
	testStoreSweepVersusComputeStart(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreSweepFirstComputeFailsSQLite(t *testing.T) {
	testStoreSweepFirstComputeFails(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreDeleteReductionSQLite(t *testing.T) {
	testStoreDeleteReduction(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreComputeSeamsSQLite(t *testing.T) {
	testStoreComputeSeams(t, withSettings(newReductionSQLiteContext(t)))
}
