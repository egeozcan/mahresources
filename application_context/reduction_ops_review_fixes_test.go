package application_context

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Regression tests for the review of the table-backed apply writers. Each body
// runs on SQLite here and on Postgres in reduction_ops_interleave_pg_test.go.

// pinLeaseClock makes the clock fences read jump two hours ahead the moment a
// writer has taken its locks, so a lease that was live at the fence is expired by
// the time the writer would act: the wait for a member's row lock, injected.
func pinLeaseClock(t *testing.T) {
	t.Helper()
	orig := leaseNow
	hook := func() {
		reductionOpsAfterLocksHook.Store(nil)
		leaseNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	}
	reductionOpsAfterLocksHook.Store(&hook)
	t.Cleanup(func() {
		reductionOpsAfterLocksHook.Store(nil)
		leaseNow = orig
	})
}

// ---- 1: the account is re-read in every write transaction ------------------------

func testOpsAccountNarrowedMidRequest(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	account := accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID)
	p := ctx.WithPrincipal(account)

	r := newPlanReduction(t, ctx.db)
	res := func(name string) uint { return makeResource(t, ctx.db, name, opsRes{Hash: "h", Owner: &mine.ID}) }
	a, b := res("a"), res("b")
	insertOps(t, ctx, r, "claim", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	c, d := res("c"), res("d")
	insertOps(t, ctx, r, "stale", models.ReductionTierIdentical, c, opsMember(c, "h"), opsMember(d, "h"))
	e, f := res("e"), res("f")
	insertOps(t, ctx, r, "demote", models.ReductionTierIdentical, e, opsMember(e, "h"), opsMember(f, "h"))
	g, h := res("g"), res("h")
	insertOps(t, ctx, r, "strand", models.ReductionTierIdentical, g, opsMember(g, "h"), opsMember(h, "h"))
	strand(t, ctx, "strand", false)
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "claim", "stale", "demote")

	// the demote's claim is taken while the account still holds
	claimed, gate, err := p.storeClaimCluster(r.ID, "b1", "demote")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)

	// an admin narrows the account to another Group
	require.NoError(t, ctx.db.Exec("UPDATE users SET scope_group_id = ? WHERE id = ?", other.ID, account.UserID).Error)

	_, _, err = p.storeClaimCluster(r.ID, "b1", "claim")
	require.ErrorIs(t, err, errAccountChanged)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "claim").State, "nothing was claimed")

	require.Equal(t, staleHidden, p.storeRecordStale(r.ID, "b1", "stale", rowOf(t, ctx, "stale").Fingerprint, staleReasonHashChanged, nil))
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "stale").State, "nothing was recorded")

	require.False(t, p.storeDemoteToStale(r.ID, "b1", "demote", claimed.Row.ClaimID, "r", nil))
	dm := rowOf(t, ctx, "demote")
	require.Equal(t, models.ReductionClusterOpen, dm.State, "the claim was released, not reported as stale")
	require.True(t, dm.Checked)
	require.Empty(t, dm.ClaimID)

	_, _, err = p.storeReconcileClaims(r.ID, batchClaimFence(r.ID, "b1"))
	require.ErrorIs(t, err, errAccountChanged)
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "strand").State, "no claim was reconciled")

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0 WHERE id = ?", r.ID).Error)
	fresh := loadReductionRow(t, ctx.db, r.ID)
	_, err = p.storeSettleClaims(fresh, &query_models.ReductionApply{ID: r.ID, Version: fresh.Version, SettleOnly: true})
	require.ErrorIs(t, err, errAccountChanged)
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "strand").State)

	_, err = p.storeClaimApplyBatch(fresh, applyRequest(fresh))
	require.ErrorIs(t, err, errAccountChanged)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsAccountNarrowedMidRequestSQLite(t *testing.T) {
	testOpsAccountNarrowedMidRequest(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 2: settle-only compares ownership under the lock ---------------------------

func testOpsSettleOwnerNulledSinceLoad(t *testing.T, ctx *MahresourcesContext) {
	owner := accountPrincipal(t, ctx, "owner", models.RoleUser, nil)
	r := newPlanReduction(t, ctx.db)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = ? WHERE id = ?", owner.UserID, r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	require.NotNil(t, r.CreatedByUserId)
	identicalPair(t, ctx, r, "s")
	strand(t, ctx, "s", false)

	// the owner is deleted after the caller loaded the pointer: no version bump
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = NULL WHERE id = ?", r.ID).Error)
	_, err := ctx.storeSettleClaims(r, &query_models.ReductionApply{ID: r.ID, Version: r.Version, SettleOnly: true})
	require.ErrorIs(t, err, ErrReductionConflict)
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "s").State, "a stale pointer did not reopen the claim")
}

func TestOpsSettleOwnerNulledSinceLoadSQLite(t *testing.T) {
	testOpsSettleOwnerNulledSinceLoad(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 3: every outcome of a batch carries its id --------------------------------

func testStoreApplyEvidenceForEveryOutcomeKind(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	pairAt(t, ctx, r, "applied", 1, nil)
	_, lRefused := pairAt(t, ctx, r, "refused", 2, nil)
	_, lStale := pairAt(t, ctx, r, "stale", 3, nil)
	require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'changed' WHERE id = ?", lStale).Error)
	_, lStranded := pairAt(t, ctx, r, "stranded", 4, nil)
	strand(t, ctx, "stranded", false)
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", lStranded).Error)

	// the bytes of one Loser change after its claim: the merge refuses, the claim is demoted
	restore := SetReductionApplyAfterClaimHookForTest(func() {
		require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'late' WHERE id = ?", lRefused).Error)
	})
	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	restore()
	require.NoError(t, err)
	require.Equal(t, 1, result.AppliedCount)
	require.Equal(t, 3, result.StaleCount, "revalidation, a refused merge and a reconciled claim")

	rec := lastApplyOf(t, ctx, r)
	require.Equal(t, 1, rec.Applied)
	require.Equal(t, 3, rec.Stale)
	for _, key := range []string{"applied", "refused", "stale", "stranded"} {
		require.Equal(t, rec.BatchID, rowOf(t, ctx, key).OutcomeBatch, key)
	}
	visible, err := ctx.reductionStore().lastApplyCountsVisible(r, opsGen, rec.BatchID, 4)
	require.NoError(t, err)
	require.True(t, visible)

	// a later settle-only changes no Cluster's outcome batch
	fresh := loadReductionRow(t, ctx.db, r.ID)
	identicalPair(t, ctx, fresh, "later")
	strand(t, ctx, "later", false)
	_, err = ctx.storeSettleClaims(fresh, &query_models.ReductionApply{ID: r.ID, Version: fresh.Version, SettleOnly: true})
	require.NoError(t, err)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "later").State)
	require.Empty(t, rowOf(t, ctx, "later").OutcomeBatch)
	visible, err = ctx.reductionStore().lastApplyCountsVisible(fresh, opsGen, rec.BatchID, 4)
	require.NoError(t, err)
	require.True(t, visible, "the earlier evidence survived the settle")
}

func TestStoreApplyEvidenceForEveryOutcomeKindSQLite(t *testing.T) {
	testStoreApplyEvidenceForEveryOutcomeKind(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 4: the lease is judged again after the locks --------------------------------

func testOpsLeaseRecheckedAfterLocks(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	identicalPair(t, ctx, r, "c")
	identicalPair(t, ctx, r, "s")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "c", "s")

	pinLeaseClock(t)
	_, _, err := ctx.storeClaimCluster(r.ID, "b1", "c")
	require.ErrorIs(t, err, ErrReductionLeaseLost, "the lease ran out while the claim waited for the member locks")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "c").State)

	pinLeaseClock(t)
	require.Equal(t, staleSuperseded, ctx.storeRecordStale(r.ID, "b1", "s", rowOf(t, ctx, "s").Fingerprint, staleReasonHashChanged, nil))
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "s").State, "an expired worker marked nothing stale")
}

func TestOpsLeaseRecheckedAfterLocksSQLite(t *testing.T) {
	testOpsLeaseRecheckedAfterLocks(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 5: bulk marking judges visibility in the marking statement ------------------

func testOpsBulkMarkRechecksVisibility(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h", Owner: &mine.ID})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h", Owner: &mine.ID})
	insertOps(t, ctx, r, "c1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	id := rowOf(t, ctx, "c1").ID
	p := ctx.WithPrincipal(accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID))

	// the Group is re-parented out of the subtree between selection and marking
	require.NoError(t, ctx.db.Exec("UPDATE resources SET owner_id = ? WHERE id = ?", other.ID, b).Error)
	marked, err := p.reductionStore().markClusters([]uint{id}, "op")
	require.NoError(t, err)
	require.Zero(t, marked, "a Cluster the principal can no longer see is not marked")
	requireNoBulkMarks(t, ctx, r)

	marked, err = ctx.reductionStore().markClusters([]uint{id}, "op")
	require.NoError(t, err)
	require.EqualValues(t, 1, marked, "an unconfined caller still marks it")
}

func TestOpsBulkMarkRechecksVisibilitySQLite(t *testing.T) {
	testOpsBulkMarkRechecksVisibility(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 6: demote needs a live lease -----------------------------------------------

func testOpsDemoteNeedsLiveLease(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	identicalPair(t, ctx, r, "d")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "d")
	claimed, gate, err := ctx.storeClaimCluster(r.ID, "b1", "d")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)

	startBatch(t, ctx, r, "b1", -time.Minute)
	require.False(t, ctx.storeDemoteToStale(r.ID, "b1", "d", claimed.Row.ClaimID, "r", nil), "an expired batch records no stale outcome")
	row := rowOf(t, ctx, "d")
	require.Equal(t, models.ReductionClusterOpen, row.State, "the claim was released instead: the next apply judges it afresh")
	require.True(t, row.Checked)
	require.Empty(t, row.ClaimID)
	require.Empty(t, row.OutcomeBatch)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsDemoteNeedsLiveLeaseSQLite(t *testing.T) {
	testOpsDemoteNeedsLiveLease(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 7: a failed stale write is an error, not a reviewer's change ----------------

func testStoreApplyFailedStaleWriteIsAnError(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l := pairAt(t, ctx, r, "k", 1, nil)
	require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'changed' WHERE id = ?", l).Error)

	const name = "fail-stale-write"
	require.NoError(t, ctx.db.Callback().Update().Before("gorm:update").Register(name, func(d *gorm.DB) {
		if m, ok := d.Statement.Dest.(map[string]any); ok {
			if _, has := m["stale_reason"]; has {
				_ = d.AddError(fmt.Errorf("injected write failure"))
			}
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(name) })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Zero(t, result.Changed, "nothing was changed by a reviewer")
	require.Equal(t, 1, result.Errors)
	require.Zero(t, result.StaleCount)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "k").State)
}

func TestStoreApplyFailedStaleWriteIsAnErrorSQLite(t *testing.T) {
	testStoreApplyFailedStaleWriteIsAnError(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 8: reconcile walks an index -----------------------------------------------

func testOpsReconcileHasAnIndex(t *testing.T, ctx *MahresourcesContext) {
	require.True(t, ctx.db.Migrator().HasIndex(&models.ReductionClusterRow{}, "idx_rc_claims"), "state = 'applied' AND NOT merged has an index")
	if ctx.db.Dialector.Name() != "sqlite" {
		return
	}
	var plan []struct{ Detail string }
	require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+reconcileKeysetSQL, 1, "g", 0, reconcileChunk).Scan(&plan).Error)
	var all []string
	for _, p := range plan {
		all = append(all, p.Detail)
	}
	require.Contains(t, strings.Join(all, "\n"), "idx_rc_claims")
}

func TestOpsReconcileHasAnIndexSQLite(t *testing.T) {
	testOpsReconcileHasAnIndex(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 10: bookkeeping after expiry leaves a running compute's version alone -------

func testStoreApplyBookkeepingDoesNotBreakACompute(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	identicalPair(t, ctx, r, "k")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "k")
	claimed, gate, err := ctx.storeClaimCluster(r.ID, "b1", "k")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)

	// the lease runs out and a compute begins before the worker settles
	startBatch(t, ctx, r, "b1", -time.Minute)
	fresh := loadReductionRow(t, ctx.db, r.ID)
	_, generation, err := ctx.beginReductionComputeStore(fresh.ID, fresh.Version)
	require.NoError(t, err)
	computing := loadReductionRow(t, ctx.db, r.ID)

	require.True(t, ctx.storeReleaseClaim(r.ID, "b1", "k", claimed.Row.ClaimID))
	ctx.storeFinishApply(r.ID, &applyRun{reductionID: r.ID, batch: "b1", generation: opsGen, startedAt: time.Now()}, &contracts.ReductionApplyResult{})
	require.Equal(t, computing.Version, versionOf(t, ctx, r), "bookkeeping did not move the version under the compute")

	require.NoError(t, ctx.landGeneration(r.ID, "", generation, computing.Version, models.ReductionCoverage{}, nil), "the compute still lands")
}

func TestStoreApplyBookkeepingDoesNotBreakAComputeSQLite(t *testing.T) {
	testStoreApplyBookkeepingDoesNotBreakACompute(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 11: the orphan sweep deletes a bounded number of rows per statement ---------

func testSweepOrphansDeletesBoundedChunks(t *testing.T, ctx *MahresourcesContext) {
	live := newPlanReduction(t, ctx.db)
	const perReduction = 400
	for _, orphan := range []int{9001, 9002, 9003} {
		for i := 1; i <= perReduction; i++ {
			require.NoError(t, ctx.db.Exec("INSERT INTO reduction_extent_resources (reduction_id, resource_id) VALUES (?, ?)", orphan, i).Error)
		}
	}
	require.NoError(t, ctx.db.Exec("INSERT INTO reduction_extent_resources (reduction_id, resource_id) VALUES (?, ?)", live.ID, 1).Error)

	var perStatement []int64
	const name = "capture-deletes"
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(name, func(d *gorm.DB) {
		if strings.HasPrefix(strings.TrimSpace(d.Statement.SQL.String()), "DELETE FROM reduction_extent_resources") {
			perStatement = append(perStatement, d.RowsAffected)
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(name) })

	require.NoError(t, ctx.reductionStore().sweepOrphans("reduction_extent_resources", "resource_id"))
	var total int64
	for _, n := range perStatement {
		require.LessOrEqual(t, n, int64(reductionRowChunk), "one statement deleted an unbounded number of rows")
		total += n
	}
	require.EqualValues(t, 3*perReduction, total)
	var left int64
	require.NoError(t, ctx.db.Table("reduction_extent_resources").Count(&left).Error)
	require.EqualValues(t, 1, left, "the live Reduction's row stays")
}

func TestSweepOrphansDeletesBoundedChunksSQLite(t *testing.T) {
	testSweepOrphansDeletesBoundedChunks(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 12: the merge renews the lease between its work units ---------------------

func testMergeRenewsBetweenWorkUnits(t *testing.T, ctx *MahresourcesContext) {
	w := makeResource(t, ctx.db, "w", opsRes{Hash: "h"})
	l1 := makeResource(t, ctx.db, "l1", opsRes{Hash: "h"})
	l2 := makeResource(t, ctx.db, "l2", opsRes{Hash: "h"})
	renewals := 0
	err := ctx.MergeResourcesLocking(w, []uint{l1, l2}, false, nil, false, MergeHooks{
		Renew: func(*MahresourcesContext) error { renewals++; return nil },
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, renewals, 4, "the merge calls Renew between its work units")

	// a Renew that refuses stops the merge before it deletes anything
	w2 := makeResource(t, ctx.db, "w2", opsRes{Hash: "h"})
	l3 := makeResource(t, ctx.db, "l3", opsRes{Hash: "h"})
	err = ctx.MergeResourcesLocking(w2, []uint{l3}, false, nil, false, MergeHooks{
		Renew: func(*MahresourcesContext) error { return ErrReductionLeaseLost },
	})
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	require.True(t, storeResourceExists(t, ctx, l3))
}

func TestMergeRenewsBetweenWorkUnitsSQLite(t *testing.T) {
	testMergeRenewsBetweenWorkUnits(t, withSettings(newReductionSQLiteContext(t)))
}

// the store's renew writes a longer lease only when less than half the window is
// left, never revives an expired one, and never touches the version
func testStoreMergeRenewHook(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	identicalPair(t, ctx, r, "k")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	run := &applyRun{reductionID: r.ID, batch: "b1", generation: opsGen, startedAt: time.Now()}
	hooks := ctx.storeMergeHooks(run, &storedCluster{Row: models.ReductionClusterRow{ID: 1}})
	require.NotNil(t, hooks.Renew)
	leaseOf := func() int64 { return loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil }
	renew := func() error {
		return ctx.WithTransaction(func(txCtx *MahresourcesContext) error { return hooks.Renew(txCtx) })
	}
	v := versionOf(t, ctx, r)

	before := leaseOf()
	require.NoError(t, renew())
	require.Equal(t, before, leaseOf(), "plenty of window left: nothing written")

	short := time.Now().Add(time.Minute).UnixNano()
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", short, r.ID).Error)
	require.NoError(t, renew())
	require.Greater(t, leaseOf(), short, "less than half the window left: renewed")
	require.Equal(t, v, versionOf(t, ctx, r))

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", time.Now().Add(-time.Second).UnixNano(), r.ID).Error)
	require.ErrorIs(t, renew(), ErrReductionLeaseLost, "an expired lease is never revived")
}

func TestStoreMergeRenewHookSQLite(t *testing.T) {
	testStoreMergeRenewHook(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 13: a reconcile error abandons the batch instead of finishing it ------------

func testStoreApplyReconcileErrorDoesNotFinish(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	pairAt(t, ctx, r, "stranded", 1, nil)
	strand(t, ctx, "stranded", false)
	pairAt(t, ctx, r, "ok", 2, nil)

	const name = "fail-reconcile-members"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, func(d *gorm.DB) {
		if strings.Contains(d.Statement.SQL.String(), "reduction_members") {
			_ = d.AddError(fmt.Errorf("injected read failure"))
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.Error(t, err)
	require.Nil(t, result)
	rec := lastApplyOf(t, ctx, r)
	require.Nil(t, rec.FinishedAt, "a batch that errored is interrupted, not finished")
	require.Zero(t, loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil, "the lease is given back so the next apply is not blocked for ten minutes")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "ok").State, "nothing was merged")
}

func TestStoreApplyReconcileErrorDoesNotFinishSQLite(t *testing.T) {
	testStoreApplyReconcileErrorDoesNotFinish(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- 8: an account change stops the apply loop ----------------------------------
//
// After a claim, a merge that fails because the caller's account changed (or a refused
// merge whose demote then finds the account changed) says the same to every later
// Cluster, so the loop stops (lost=true) as it does when the claim itself notices.

func testStoreApplyAccountChangeStopsTheLoop(t *testing.T, ctx *MahresourcesContext, alsoRefuseMerge bool) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	account := accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID)
	p := ctx.WithPrincipal(account)

	r := applyReduction(t, ctx)
	res := func(name string) uint { return makeResource(t, ctx.db, name, opsRes{Hash: "h", Owner: &mine.ID}) }
	a, b := res("a"), res("b")
	insertOps(t, ctx, r, "k", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "k")
	run := &applyRun{reductionID: r.ID, batch: "b1", generation: opsGen, startedAt: time.Now()}

	reductionApplyAfterClaimHook = func() {
		require.NoError(t, ctx.db.Exec("UPDATE users SET scope_group_id = ? WHERE id = ?", other.ID, account.UserID).Error)
		if alsoRefuseMerge {
			require.NoError(t, ctx.db.Exec("UPDATE resources SET hash = 'changed' WHERE id = ?", b).Error)
		}
	}
	t.Cleanup(func() { reductionApplyAfterClaimHook = nil })

	outcome, lost := p.storeApplyOneCluster(r, run, "k")
	require.True(t, outcome.Omit, "nothing about the Cluster may be reported")
	require.True(t, lost, "the account changed: every later Cluster would say the same, so the loop stops")
}

func TestStoreApplyAccountChangeStopsTheLoopSQLite(t *testing.T) {
	t.Run("merge", func(t *testing.T) {
		testStoreApplyAccountChangeStopsTheLoop(t, withSettings(newReductionSQLiteContext(t)), false)
	})
	t.Run("refused merge", func(t *testing.T) {
		testStoreApplyAccountChangeStopsTheLoop(t, withSettings(newReductionSQLiteContext(t)), true)
	})
}

// ---- 9: a withheld Cluster is hidden even when the lease ran out ----------------

func testOpsWithheldBeatsExpiredLease(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	account := accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID)
	p := ctx.WithPrincipal(account)

	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "wa", opsRes{Hash: "h", Owner: &mine.ID})
	b := makeResource(t, ctx.db, "wb", opsRes{Hash: "h", Owner: &other.ID})
	insertOps(t, ctx, r, "w", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "w")

	pinLeaseClock(t)
	require.Equal(t, staleHidden, p.storeRecordStale(r.ID, "b1", "w", rowOf(t, ctx, "w").Fingerprint, staleReasonHashChanged, nil),
		"a Cluster the caller may not see is hidden, not counted as changed, whatever the lease says")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "w").State)
}

func TestOpsWithheldBeatsExpiredLeaseSQLite(t *testing.T) {
	testOpsWithheldBeatsExpiredLease(t, withSettings(newReductionSQLiteContext(t)))
}
