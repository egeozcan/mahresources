package application_context

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// ---- fixtures -------------------------------------------------------------------

// applyReduction is a settled table plan whose selection is the whole library.
func applyReduction(t *testing.T, ctx *MahresourcesContext) *models.ResourceReduction {
	t.Helper()
	r := newPlanReduction(t, ctx.db)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET extent_all = ? WHERE id = ?", true, r.ID).Error)
	return loadReductionRow(t, ctx.db, r.ID)
}

// insertAt writes a Cluster at a plan position.
func insertAt(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key, tier string, position int64, winner uint, members ...models.ReductionMemberRow) {
	t.Helper()
	c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: tier, Position: position, State: models.ReductionClusterOpen, WinnerID: winner, Checked: true}, Members: members}
	require.NoError(t, ctx.reductionStore().insertCluster(r, opsGen, c))
}

// pairAt is an approved Identical Cluster of two existing Resources at a position.
func pairAt(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key string, position int64, owner *uint) (winner, loser uint) {
	t.Helper()
	winner = makeResource(t, ctx.db, key+"-w", opsRes{Hash: "h", Owner: owner})
	loser = makeResource(t, ctx.db, key+"-l", opsRes{Hash: "h", Owner: owner})
	insertAt(t, ctx, r, key, models.ReductionTierIdentical, position, winner, opsMember(winner, "h"), opsMember(loser, "h"))
	return winner, loser
}

func applyRequest(r *models.ResourceReduction) *query_models.ReductionApply {
	return &query_models.ReductionApply{ID: r.ID, Version: r.Version, RequestID: "req-1"}
}

func lastApplyOf(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) models.ReductionLastApply {
	t.Helper()
	var rec models.ReductionLastApply
	require.NoError(t, json.Unmarshal(loadReductionRow(t, ctx.db, r.ID).LastApply, &rec))
	return rec
}

func storeResourceExists(t *testing.T, ctx *MahresourcesContext, id uint) bool {
	t.Helper()
	var n int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id = ?", id).Count(&n).Error)
	return n == 1
}

func appliedKeys(out []contracts.ReductionApplyOutcome) []string {
	keys := make([]string, len(out))
	for i, o := range out {
		keys[i] = o.ClusterID
	}
	return keys
}

// ---- end to end ---------------------------------------------------------------------

func testStoreApplyEndToEnd(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	// Inserted out of key order so plan order (position) is what the loop follows.
	wa, la := pairAt(t, ctx, r, "ka", 30, &home.ID)
	wb, lb := pairAt(t, ctx, r, "kb", 10, nil)
	wc, lc := pairAt(t, ctx, r, "kc", 20, nil)
	// not approved: stays exactly as it was
	wd := makeResource(t, ctx.db, "kd-w", opsRes{Hash: "h"})
	ld := makeResource(t, ctx.db, "kd-l", opsRes{Hash: "h"})
	insertAt(t, ctx, r, "kd", models.ReductionTierIdentical, 5, wd, opsMember(wd, "h"), opsMember(ld, "h"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = false WHERE "key" = 'kd'`).Error)

	token, clusters, losers, err := ctx.reductionStore().applyEligibility(r, opsGen)
	require.NoError(t, err)
	require.Equal(t, 3, clusters)
	expectedClusters, expectedLosers := uint(clusters), uint(losers)
	req := applyRequest(r)
	req.ApplyToken, req.ExpectedClusters, req.ExpectedLosers = token, &expectedClusters, &expectedLosers

	result, err := ctx.storeApplyReduction(r, req)
	require.NoError(t, err)
	require.Equal(t, []string{"kb", "kc", "ka"}, appliedKeys(result.Applied), "plan order, not key order")
	require.Equal(t, 3, result.AppliedCount)
	require.Equal(t, 3, result.Destroyed)
	require.Equal(t, 3, result.DestroyedCount())
	require.Zero(t, result.StaleCount+result.Changed+result.Errors)
	require.False(t, result.More)
	for _, o := range result.Applied {
		require.NotEmpty(t, o.WinnerName, "each entry names its Winner")
	}
	for _, id := range []uint{wa, wb, wc, wd, ld} {
		require.True(t, storeResourceExists(t, ctx, id))
	}
	for _, id := range []uint{la, lb, lc} {
		require.False(t, storeResourceExists(t, ctx, id))
	}

	rec := lastApplyOf(t, ctx, r)
	require.NotEmpty(t, rec.BatchID)
	require.Equal(t, opsGen, rec.Generation)
	require.Equal(t, "req-1", rec.RequestID)
	require.Equal(t, 3, rec.ExpectedClusters)
	require.NotNil(t, rec.FinishedAt)
	require.Equal(t, 3, rec.Applied)
	require.Equal(t, 3, rec.Removed)
	require.Zero(t, loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil, "the lease is given back")

	ka := rowOf(t, ctx, "ka")
	require.True(t, ka.Merged)
	require.Equal(t, models.ReductionClusterApplied, ka.State)
	require.Equal(t, rec.BatchID, ka.OutcomeBatch)
	require.Equal(t, rec.BatchID, ka.ApplyBatch)
	require.Equal(t, wa, ka.ApprovedWinner)
	var owner uint
	require.NoError(t, ctx.db.Raw(`SELECT owner_at_merge FROM reduction_members WHERE cluster_id = ? AND resource_id = ?`, ka.ID, la).Scan(&owner).Error)
	require.Equal(t, home.ID, owner, "the Loser's owning Group is recorded in the merge's transaction")
	kd := rowOf(t, ctx, "kd")
	require.Equal(t, models.ReductionClusterOpen, kd.State)
	require.Empty(t, kd.ApplyBatch)

	visible, err := ctx.reductionStore().lastApplyCountsVisible(r, opsGen, rec.BatchID, 3)
	require.NoError(t, err)
	require.True(t, visible)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreApplyTokenMismatchRollsBack(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)
	pairAt(t, ctx, r, "k2", 2, nil)
	before := loadReductionRow(t, ctx.db, r.ID)

	req := applyRequest(r)
	req.ApplyToken = "00000000000000000000000000000000"
	_, err := ctx.storeApplyReduction(r, req)
	require.ErrorIs(t, err, ErrReductionApplyFiguresChanged)

	three := uint(3)
	req = applyRequest(r)
	req.ExpectedClusters = &three
	_, err = ctx.storeApplyReduction(r, req)
	require.ErrorIs(t, err, ErrReductionApplyFiguresChanged)
	req = applyRequest(r)
	req.ExpectedLosers = &three
	_, err = ctx.storeApplyReduction(r, req)
	require.ErrorIs(t, err, ErrReductionApplyFiguresChanged)

	var stamped int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("apply_batch <> ''").Count(&stamped).Error)
	require.Zero(t, stamped, "the stamp rolled back with the refusal")
	after := loadReductionRow(t, ctx.db, r.ID)
	require.Equal(t, before.Version, after.Version, "so did the version bump")
	require.Equal(t, before.LastApply, after.LastApply)
	require.Zero(t, after.ApplyLeaseUntil)
	require.True(t, storeResourceExists(t, ctx, l1))
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreApplyRefusals(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l := pairAt(t, ctx, r, "k1", 1, nil)

	stale := applyRequest(r)
	stale.Version = r.Version + 7
	_, err := ctx.storeApplyReduction(r, stale)
	require.ErrorIs(t, err, ErrReductionConflict)

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", time.Now().Add(time.Minute).UnixNano(), r.ID).Error)
	live := loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeApplyReduction(live, applyRequest(live))
	require.ErrorIs(t, err, ErrReductionConflict, "a live apply owns the Reduction")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0 WHERE id = ?", r.ID).Error)

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_op = 'op', settle_lease_until = ? WHERE id = ?", time.Now().Add(time.Minute).UnixNano(), r.ID).Error)
	live = loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeApplyReduction(live, applyRequest(live))
	require.ErrorIs(t, err, ErrReductionConflict, "a live settle excludes an apply")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_op = NULL, settle_lease_until = 0 WHERE id = ?", r.ID).Error)

	// and an apply in flight excludes a settle
	restore := SetReductionApplyAfterClaimHookForTest(func() {
		cur := loadReductionRow(t, ctx.db, r.ID)
		_, settleErr := ctx.storeSettleClaims(cur, &query_models.ReductionApply{ID: cur.ID, Version: cur.Version, SettleOnly: true})
		require.ErrorIs(t, settleErr, ErrReductionConflict)
	})
	fresh := loadReductionRow(t, ctx.db, r.ID)
	result, err := ctx.storeApplyReduction(fresh, applyRequest(fresh))
	restore()
	require.NoError(t, err)
	require.Equal(t, 1, result.AppliedCount)
	require.False(t, storeResourceExists(t, ctx, l))

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 0 WHERE id = ?", r.ID).Error)
	legacy := loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeApplyReduction(legacy, applyRequest(legacy))
	require.ErrorIs(t, err, ErrReductionBusy)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1, status = 'computing', compute_lease_until = ? WHERE id = ?", time.Now().Add(time.Hour).UnixNano(), r.ID).Error)
	busy := loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeApplyReduction(busy, applyRequest(busy))
	require.ErrorIs(t, err, ErrReductionBusy)
}

func testStoreApplyStamp(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	pairAt(t, ctx, r, "eligible", 1, nil)
	type tc struct{ name, sql string }
	for i, c := range []tc{
		{"unchecked", `UPDATE reduction_clusters SET checked = false WHERE "key" = ?`},
		{"skipped", `UPDATE reduction_clusters SET state = 'skipped' WHERE "key" = ?`},
		{"gone member", `UPDATE reduction_members SET gone = true WHERE cluster_id = (SELECT id FROM reduction_clusters WHERE "key" = ?) AND position = 1`},
		{"read only", `UPDATE reduction_clusters SET read_only = true WHERE "key" = ?`},
		{"claimed", `UPDATE reduction_clusters SET claim_id = 'old' WHERE "key" = ?`},
		{"nothing to remove", `UPDATE reduction_members SET ejected = true WHERE cluster_id = (SELECT id FROM reduction_clusters WHERE "key" = ?) AND position = 1`},
		{"full star lost its seed", `UPDATE reduction_clusters SET fixed_seed = true, seed_resource_id = 999 WHERE "key" = ?`},
	} {
		key := "x-" + c.name
		pairAt(t, ctx, r, key, int64(10+i), nil)
		require.NoError(t, ctx.db.Exec(c.sql, key).Error)
		mutateOps(t, ctx, r, key, func(*storedCluster) {})
	}
	// withheld from a scoped caller: stamped for nobody who cannot see all of it
	mine := &models.Group{Name: "mine"}
	require.NoError(t, ctx.db.Create(mine).Error)
	pairAt(t, ctx, r, "outside", 50, nil)

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	run, err := scoped.storeClaimApplyBatch(r, applyRequest(r))
	require.NoError(t, err)
	var stamped int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("apply_batch = ?", run.batch).Count(&stamped).Error)
	require.Zero(t, stamped, "a scoped caller stamps nothing it cannot fully see")

	r = loadReductionRow(t, ctx.db, r.ID)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0 WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	run, err = ctx.storeClaimApplyBatch(r, applyRequest(r))
	require.NoError(t, err)
	var keys []string
	require.NoError(t, ctx.db.Raw(`SELECT "key" FROM reduction_clusters WHERE apply_batch = ? ORDER BY "key"`, run.batch).Scan(&keys).Error)
	require.Equal(t, []string{"eligible", "outside"}, keys)
	row := rowOf(t, ctx, "eligible")
	require.Equal(t, row.WinnerID, row.ApprovedWinner)
	require.Equal(t, row.RestoreSeq, row.ApprovedSeq)
}

func testStoreApplyExpiredLeaseDestroysNothing(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)
	_, l2 := pairAt(t, ctx, r, "k2", 2, nil)

	// the lease runs out after the claim and before the merge
	claims := 0
	restore := SetReductionApplyAfterClaimHookForTest(func() {
		claims++
		require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", time.Now().Add(-time.Second).UnixNano(), r.ID).Error)
	})
	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	restore()
	require.NoError(t, err)
	require.Equal(t, 1, claims, "the first Cluster was claimed and no second one started")
	require.Zero(t, result.AppliedCount)
	require.True(t, storeResourceExists(t, ctx, l1))
	require.True(t, storeResourceExists(t, ctx, l2))
	k1 := rowOf(t, ctx, "k1")
	require.Equal(t, models.ReductionClusterOpen, k1.State, "the claim was handed back")
	require.True(t, k1.Checked)
	require.False(t, k1.Merged)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "k2").State, "and the loop stopped there")
	rec := lastApplyOf(t, ctx, r)
	require.NotNil(t, rec.FinishedAt, "the finish record is bookkeeping, allowed after expiry")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreApplyParkedWorkerCannotMerge(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)

	// the worker passes every fence, then parks until its lease has run out: the
	// clock read by InTx, after the locks and the revalidation, is past it
	orig := leaseNow
	t.Cleanup(func() { leaseNow = orig })
	reached := 0
	restoreHook := SetReductionPreconditionHookForTest(func() {
		reached++
		leaseNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	})
	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	restoreHook()
	leaseNow = orig
	require.NoError(t, err)
	require.Equal(t, 1, reached, "the merge got as far as its precondition")
	require.Zero(t, result.AppliedCount)
	require.True(t, storeResourceExists(t, ctx, l1), "the merge rolled back whole")
	k1 := rowOf(t, ctx, "k1")
	require.False(t, k1.Merged)
	require.Equal(t, models.ReductionClusterOpen, k1.State)
	require.NotNil(t, lastApplyOf(t, ctx, r).FinishedAt)
}

func testStoreApplyStaleOutcomes(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	// the bytes of a Loser changed after review
	wh, lh := pairAt(t, ctx, r, "hash", 1, nil)
	require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'changed' WHERE id = ?", lh).Error)
	// a Loser is gone
	_, lg := pairAt(t, ctx, r, "deleted", 2, nil)
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", lg).Error)
	// a Near Loser lost its pair
	wn := makeResource(t, ctx.db, "near-w", opsRes{Hash: "n1"})
	ln := makeResource(t, ctx.db, "near-l", opsRes{Hash: "n2"})
	insertAt(t, ctx, r, "nopair", models.ReductionTierNear, 3, wn, opsMember(wn, "n1"), opsMember(ln, "n2"))
	// ... and one that kept it
	wk := makeResource(t, ctx.db, "kept-w", opsRes{Hash: "n3"})
	lk := makeResource(t, ctx.db, "kept-l", opsRes{Hash: "n4"})
	insertAt(t, ctx, r, "paired", models.ReductionTierNear, 4, wk, opsMember(wk, "n3"), opsMember(lk, "n4"))
	makePair(t, ctx.db, wk, lk, 2)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = true WHERE tier = 'near'`).Error)

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, []string{"paired"}, appliedKeys(result.Applied))
	require.Equal(t, 3, result.StaleCount)
	reasons := map[string]string{}
	for _, o := range result.Stale {
		reasons[o.ClusterID] = o.Reason
	}
	require.Equal(t, staleReasonHashChanged, reasons["hash"])
	require.Equal(t, staleReasonDeleted, reasons["deleted"])
	require.Equal(t, staleReasonPairGone, reasons["nopair"])
	require.True(t, storeResourceExists(t, ctx, lh))
	require.True(t, storeResourceExists(t, ctx, wh))
	require.True(t, storeResourceExists(t, ctx, ln))
	require.False(t, storeResourceExists(t, ctx, lk))

	rec := lastApplyOf(t, ctx, r)
	require.Equal(t, 1, rec.Applied)
	require.Equal(t, 3, rec.Stale)
	for _, key := range []string{"hash", "deleted", "nopair", "paired"} {
		require.Equal(t, rec.BatchID, rowOf(t, ctx, key).OutcomeBatch, key)
	}
	visible, err := ctx.reductionStore().lastApplyCountsVisible(r, opsGen, rec.BatchID, 4)
	require.NoError(t, err)
	require.True(t, visible, "four outcomes carry the batch, as counted")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreApplyLeftTheSelection(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET extent_all = ? WHERE id = ?", false, r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	selected := &models.Group{Name: "selected"}
	require.NoError(t, ctx.db.Create(selected).Error)
	child := &models.Group{Name: "child", OwnerId: &selected.ID}
	require.NoError(t, ctx.db.Create(child).Error)
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(other).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: r.ID, GroupID: selected.ID}).Error)

	// owned below a selected Group, selected by id, related to a selected Group: inside
	wo, lo := pairAt(t, ctx, r, "owned", 1, &child.ID)
	_ = wo
	we, le := pairAt(t, ctx, r, "explicit", 2, nil)
	_ = we
	require.NoError(t, ctx.db.Create(&models.ReductionExtentResource{ReductionID: r.ID, ResourceID: le}).Error)
	wr, lr := pairAt(t, ctx, r, "related", 3, nil)
	_ = wr
	require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", selected.ID, lr).Error)
	// owned by a Group outside the selection: the Loser left it
	_, lx := pairAt(t, ctx, r, "outside", 4, &other.ID)

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"owned", "explicit", "related"}, appliedKeys(result.Applied))
	require.Equal(t, []string{"outside"}, appliedKeys(result.Stale))
	require.Equal(t, staleReasonLeftSelection, result.Stale[0].Reason)
	require.True(t, storeResourceExists(t, ctx, lx))
	for _, id := range []uint{lo, le, lr} {
		require.False(t, storeResourceExists(t, ctx, id))
	}
}

func testStoreApplyStrandedClaimIsNotInTheBatch(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, ls := pairAt(t, ctx, r, "stranded", 1, nil)
	_, lf := pairAt(t, ctx, r, "fine", 2, nil)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET state = 'applied', checked = false, merged = false, claim_id = 'old' WHERE "key" = 'stranded'`).Error)

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, []string{"fine"}, appliedKeys(result.Applied))
	s := rowOf(t, ctx, "stranded")
	require.Equal(t, models.ReductionClusterOpen, s.State, "reconciled back to approved")
	require.True(t, s.Checked)
	require.Empty(t, s.ClaimID)
	require.Empty(t, s.ApplyBatch, "a Cluster the reconciliation reopened is not in this batch")
	require.True(t, storeResourceExists(t, ctx, ls))
	require.False(t, storeResourceExists(t, ctx, lf))

	again := loadReductionRow(t, ctx.db, r.ID)
	second, err := ctx.storeApplyReduction(again, applyRequest(again))
	require.NoError(t, err)
	require.Equal(t, []string{"stranded"}, appliedKeys(second.Applied))
}

func testStoreApplyCappedResponse(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	const n = reductionApplyOutcomeCap + 5
	var want []string
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("c%03d", i)
		// plan order is the reverse of key order
		pairAt(t, ctx, r, key, int64(n-i), nil)
		want = append([]string{key}, want...)
	}
	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, n, result.AppliedCount)
	require.Equal(t, n, result.Destroyed)
	require.Len(t, result.Applied, reductionApplyOutcomeCap)
	require.True(t, result.More)
	require.Equal(t, want[:reductionApplyOutcomeCap], appliedKeys(result.Applied), "in plan order across the 50-per-read chunks")
	rec := lastApplyOf(t, ctx, r)
	require.Equal(t, n, rec.Applied)
	visible, err := ctx.reductionStore().lastApplyCountsVisible(r, opsGen, rec.BatchID, n)
	require.NoError(t, err)
	require.True(t, visible)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func testStoreApplyEvidenceHiddenForScopedViewer(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	mine := &models.Group{Name: "mine"}
	theirs := &models.Group{Name: "theirs"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(theirs).Error)
	pairAt(t, ctx, r, "inside", 1, &mine.ID)
	// one Loser lives outside the viewer's subtree
	w := makeResource(t, ctx.db, "x-w", opsRes{Hash: "h", Owner: &mine.ID})
	l := makeResource(t, ctx.db, "x-l", opsRes{Hash: "h", Owner: &theirs.ID})
	insertAt(t, ctx, r, "outcome", models.ReductionTierIdentical, 2, w, opsMember(w, "h"), opsMember(l, "h"))

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, 2, result.AppliedCount)
	batch := lastApplyOf(t, ctx, r).BatchID

	admin, err := ctx.reductionStore().lastApplyCountsVisible(r, opsGen, batch, 2)
	require.NoError(t, err)
	require.True(t, admin)
	viewer := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	scoped, err := viewer.reductionStore().lastApplyCountsVisible(r, opsGen, batch, 2)
	require.NoError(t, err)
	require.False(t, scoped, "an outcome Cluster withheld from the viewer hides the whole count")
}

func TestStoreApplyEndToEndSQLite(t *testing.T) {
	testStoreApplyEndToEnd(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyTokenMismatchRollsBackSQLite(t *testing.T) {
	testStoreApplyTokenMismatchRollsBack(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyRefusalsSQLite(t *testing.T) {
	testStoreApplyRefusals(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyStampSQLite(t *testing.T) {
	testStoreApplyStamp(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyExpiredLeaseDestroysNothingSQLite(t *testing.T) {
	testStoreApplyExpiredLeaseDestroysNothing(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyParkedWorkerCannotMergeSQLite(t *testing.T) {
	testStoreApplyParkedWorkerCannotMerge(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyStaleOutcomesSQLite(t *testing.T) {
	testStoreApplyStaleOutcomes(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyLeftTheSelectionSQLite(t *testing.T) {
	testStoreApplyLeftTheSelection(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyStrandedClaimIsNotInTheBatchSQLite(t *testing.T) {
	testStoreApplyStrandedClaimIsNotInTheBatch(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyCappedResponseSQLite(t *testing.T) {
	testStoreApplyCappedResponse(t, withSettings(newReductionSQLiteContext(t)))
}
func TestStoreApplyEvidenceHiddenForScopedViewerSQLite(t *testing.T) {
	testStoreApplyEvidenceHiddenForScopedViewer(t, withSettings(newReductionSQLiteContext(t)))
}
