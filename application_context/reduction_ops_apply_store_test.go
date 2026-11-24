package application_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// startBatch makes a batch the live owner of the Reduction: the last_apply record
// names it and the plan generation it pinned, and the apply lease runs for d (a
// negative d is an expired one).
func startBatch(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, batch string, d time.Duration) *models.ResourceReduction {
	t.Helper()
	record := fmt.Sprintf(`{"batchId":%q,"generation":%q,"startedAt":"2026-11-24T00:00:00Z","applied":0,"removed":0,"stale":0}`, batch, opsGen)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET last_apply = ?, apply_lease_until = ? WHERE id = ?",
		record, time.Now().Add(d).UnixNano(), r.ID).Error)
	return loadReductionRow(t, ctx.db, r.ID)
}

// stamp is what the batch claim does to the Clusters it takes in.
func stamp(t *testing.T, ctx *MahresourcesContext, batch string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET apply_batch = ?, approved_winner = winner_id, approved_seq = restore_seq WHERE "key" = ?`, batch, key).Error)
	}
}

func rowOf(t *testing.T, ctx *MahresourcesContext, key string) models.ReductionClusterRow {
	t.Helper()
	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.Where(`"key" = ?`, key).First(&row).Error)
	return row
}

func versionOf(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction) uint {
	return loadReductionRow(t, ctx.db, r.ID).Version
}

// identicalPair is an approved Identical Cluster of two existing Resources.
func identicalPair(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key string) (winner, loser uint) {
	t.Helper()
	winner = makeResource(t, ctx.db, key+"-w", opsRes{Hash: "h"})
	loser = makeResource(t, ctx.db, key+"-l", opsRes{Hash: "h"})
	insertOps(t, ctx, r, key, models.ReductionTierIdentical, winner, opsMember(winner, "h"), opsMember(loser, "h"))
	return winner, loser
}

func testOpsClaim(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	w, l := identicalPair(t, ctx, r, "ok")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "ok")
	before := versionOf(t, ctx, r)

	claimed, gate, err := ctx.storeClaimCluster(r.ID, "b1", "ok")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	require.Equal(t, models.ReductionClusterApplied, claimed.Row.State)
	require.NotEmpty(t, claimed.Row.ClaimID)
	require.Equal(t, []uint{l}, claimed.loserIDs())
	require.Equal(t, w, claimed.Row.WinnerID)
	row := rowOf(t, ctx, "ok")
	require.Equal(t, models.ReductionClusterApplied, row.State)
	require.False(t, row.Checked)
	require.True(t, row.Reviewed)
	require.False(t, row.Merged)
	require.Equal(t, claimed.Row.ClaimID, row.ClaimID)
	require.NotNil(t, row.AppliedAt)
	require.Equal(t, before+1, versionOf(t, ctx, r))

	_, gate, err = ctx.storeClaimCluster(r.ID, "b1", "ok")
	require.NoError(t, err)
	require.Equal(t, claimChanged, gate, "a claimed Cluster cannot be claimed again")

	// each thing that makes a Cluster not claimable, one Cluster apiece; the
	// mutation is a real one (members and flags), the derived columns follow it
	type tc struct{ name, sql string }
	for _, c := range []tc{
		{"unchecked", `UPDATE reduction_clusters SET checked = false WHERE "key" = ?`},
		{"skipped", `UPDATE reduction_clusters SET state = 'skipped' WHERE "key" = ?`},
		{"gone member", `UPDATE reduction_members SET gone = true WHERE cluster_id = (SELECT id FROM reduction_clusters WHERE "key" = ?) AND position = 1`},
		{"read only", `UPDATE reduction_clusters SET read_only = true WHERE "key" = ?`},
		{"nothing to remove", `UPDATE reduction_members SET ejected = true WHERE cluster_id = (SELECT id FROM reduction_clusters WHERE "key" = ?) AND position = 1`},
		{"other batch", `UPDATE reduction_clusters SET apply_batch = 'other' WHERE "key" = ?`},
		{"winner changed since approval", `UPDATE reduction_clusters SET approved_winner = 12345 WHERE "key" = ?`},
		{"a Loser was restored", `UPDATE reduction_clusters SET restore_seq = restore_seq + 1 WHERE "key" = ?`},
		{"full star lost its seed", `UPDATE reduction_clusters SET fixed_seed = true, seed_resource_id = 999 WHERE "key" = ?`},
	} {
		key := "x-" + c.name
		identicalPair(t, ctx, r, key)
		stamp(t, ctx, "b1", key)
		require.NoError(t, ctx.db.Exec(c.sql, key).Error)
		mutateOps(t, ctx, r, key, func(*storedCluster) {})
		v := versionOf(t, ctx, r)
		_, gate, err := ctx.storeClaimCluster(r.ID, "b1", key)
		require.NoError(t, err, c.name)
		require.Equal(t, claimChanged, gate, c.name)
		require.Equal(t, v, versionOf(t, ctx, r), "%s: nothing written", c.name)
		require.NotEqual(t, models.ReductionClusterApplied, rowOf(t, ctx, key).State, c.name)
	}
	_, gate, err = ctx.storeClaimCluster(r.ID, "b1", "no-such-key")
	require.NoError(t, err)
	require.Equal(t, claimChanged, gate)

	// an ejection during the batch is honoured: the claim snapshot is what is left
	w3 := makeResource(t, ctx.db, "e-w", opsRes{Hash: "h"})
	l3 := makeResource(t, ctx.db, "e-l", opsRes{Hash: "h"})
	l4 := makeResource(t, ctx.db, "e-m", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "eject", models.ReductionTierIdentical, w3, opsMember(w3, "h"), opsMember(l3, "h"), opsMember(l4, "h"))
	stamp(t, ctx, "b1", "eject")
	r = loadReductionRow(t, ctx.db, r.ID)
	next, err := ctx.storeOverrideCluster(r, override(r, "eject", ReductionActionEject, l3))
	require.NoError(t, err)
	claimed, gate, err = ctx.storeClaimCluster(r.ID, "b1", "eject")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	require.Equal(t, []uint{l4}, claimed.loserIDs())
	_ = next
	// a promotion during the batch is refused
	w5 := makeResource(t, ctx.db, "p-w", opsRes{Hash: "h"})
	l5 := makeResource(t, ctx.db, "p-l", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "promoted", models.ReductionTierIdentical, w5, opsMember(w5, "h"), opsMember(l5, "h"))
	stamp(t, ctx, "b1", "promoted")
	r = loadReductionRow(t, ctx.db, r.ID)
	_, err = ctx.storeOverrideCluster(r, override(r, "promoted", ReductionActionPromote, l5))
	require.NoError(t, err)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = true WHERE "key" = 'promoted'`).Error)
	_, gate, err = ctx.storeClaimCluster(r.ID, "b1", "promoted")
	require.NoError(t, err)
	require.Equal(t, claimChanged, gate)

	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsClaimSQLite(t *testing.T) { testOpsClaim(t, withSettings(newReductionSQLiteContext(t))) }

// Every outcome write needs the live batch and the pinned generation; bookkeeping on
// a claim the batch itself took survives the lease's expiry.
func testOpsFences(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	identicalPair(t, ctx, r, "k")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "k")

	// no batch, a different batch, a moved generation, an expired lease: all lost
	_, _, err := ctx.storeClaimCluster(r.ID, "", "k")
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	_, _, err = ctx.storeClaimCluster(r.ID, "someone-else", "k")
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_generation = 'g2' WHERE id = ?", r.ID).Error)
	_, _, err = ctx.storeClaimCluster(r.ID, "b1", "k")
	require.ErrorIs(t, err, ErrReductionLeaseLost, "the plan moved under the batch")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_generation = ? WHERE id = ?", opsGen, r.ID).Error)
	startBatch(t, ctx, r, "b1", -time.Second)
	_, _, err = ctx.storeClaimCluster(r.ID, "b1", "k")
	require.ErrorIs(t, err, ErrReductionLeaseLost, "an expired lease cannot claim")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "k").State)

	// a worker whose lease expired cannot mark a successor's proposal stale
	fp := rowOf(t, ctx, "k").Fingerprint
	require.Equal(t, staleSuperseded, ctx.storeRecordStale(r.ID, "b1", "k", fp, staleReasonHashChanged, nil))
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "k").State)
	startBatch(t, ctx, r, "b1", time.Hour)

	// claim, then expire: demote and release still settle the batch's own claim
	claimed, gate, err := ctx.storeClaimCluster(r.ID, "b1", "k")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	startBatch(t, ctx, r, "b1", -time.Minute)
	require.False(t, ctx.storeDemoteToStale(r.ID, "b2", "k", claimed.Row.ClaimID, "r", nil), "a batch that no longer owns the Reduction settles nothing")
	require.False(t, ctx.storeDemoteToStale(r.ID, "b1", "k", "wrong-claim", "r", nil))
	require.False(t, ctx.storeReleaseClaim(r.ID, "b1", "k", "wrong-claim"))
	require.True(t, ctx.storeReleaseClaim(r.ID, "b1", "k", claimed.Row.ClaimID), "release is bookkeeping, allowed after expiry")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "k").State)
	require.False(t, ctx.storeReleaseClaim(r.ID, "b1", "k", claimed.Row.ClaimID), "a released claim is not released twice")
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsFencesSQLite(t *testing.T) { testOpsFences(t, withSettings(newReductionSQLiteContext(t))) }

func testOpsDemoteAndRelease(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	w, l := identicalPair(t, ctx, r, "d")
	identicalPair(t, ctx, r, "rel")
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "d", "rel")

	claimed, gate, err := ctx.storeClaimCluster(r.ID, "b1", "d")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	v := versionOf(t, ctx, r)
	require.True(t, ctx.storeDemoteToStale(r.ID, "b1", "d", claimed.Row.ClaimID, staleReasonPairGone, []uint{l}))
	row := rowOf(t, ctx, "d")
	require.Equal(t, models.ReductionClusterStale, row.State)
	require.Equal(t, staleReasonPairGone, row.StaleReason)
	require.Empty(t, row.ClaimID, "a demoted claim does not keep its id: the next stamp would refuse it")
	require.Nil(t, row.AppliedAt)
	require.False(t, row.Checked)
	require.Equal(t, v+1, versionOf(t, ctx, r))
	got := loadOps(t, ctx, r, "d")
	require.True(t, memberOf(got, l).Changed)
	require.False(t, memberOf(got, w).Changed)
	require.False(t, ctx.storeDemoteToStale(r.ID, "b1", "d", claimed.Row.ClaimID, "again", nil), "no longer applied")

	// release recounts gone: a Loser deleted while the claim was held
	claimed, gate, err = ctx.storeClaimCluster(r.ID, "b1", "rel")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	relLoser := claimed.loserIDs()[0]
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", relLoser).Error)
	require.True(t, ctx.storeReleaseClaim(r.ID, "b1", "rel", claimed.Row.ClaimID))
	row = rowOf(t, ctx, "rel")
	require.Equal(t, models.ReductionClusterOpen, row.State)
	require.True(t, row.Checked)
	require.Empty(t, row.ClaimID)
	require.Equal(t, 1, row.GoneMembers, "the deletion that landed while claimed is not lost")
	require.True(t, memberOf(loadOps(t, ctx, r, "rel"), relLoser).Gone)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsDemoteAndReleaseSQLite(t *testing.T) {
	testOpsDemoteAndRelease(t, withSettings(newReductionSQLiteContext(t)))
}

func testOpsRecordStale(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	w, l := identicalPair(t, ctx, r, "s")
	m3 := makeResource(t, ctx.db, "m3", opsRes{Hash: "h"})
	mutateOps(t, ctx, r, "s", func(*storedCluster) {})
	// a third copy, so ejecting one changes the proposal without emptying it
	var cid uint
	require.NoError(t, ctx.db.Raw(`SELECT id FROM reduction_clusters WHERE "key" = 's'`).Scan(&cid).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionMemberRow{ClusterID: cid, ReductionID: r.ID, Generation: opsGen, ResourceID: m3, Position: 2, Hash: "h", FileSize: 10, InExtent: true}).Error)
	mutateOps(t, ctx, r, "s", func(*storedCluster) {})
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stale := rowOf(t, ctx, "s").Fingerprint

	// a decision made after the failing read: the proposal is no longer the one that failed
	o, err := ctx.storeOverrideCluster(r, override(r, "s", ReductionActionEject, m3))
	require.NoError(t, err)
	require.Equal(t, staleSuperseded, ctx.storeRecordStale(r.ID, "b1", "s", stale, staleReasonHashChanged, []uint{l}), "the reviewer already removed the reason")
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "s").State)
	require.True(t, rowOf(t, ctx, "s").Checked)

	v := versionOf(t, ctx, o)
	fresh := rowOf(t, ctx, "s").Fingerprint
	require.Equal(t, staleRecorded, ctx.storeRecordStale(r.ID, "b1", "s", fresh, staleReasonHashChanged, []uint{l}))
	row := rowOf(t, ctx, "s")
	require.Equal(t, models.ReductionClusterStale, row.State)
	require.Equal(t, staleReasonHashChanged, row.StaleReason)
	require.False(t, row.Checked)
	require.True(t, row.Reviewed)
	require.Equal(t, v+1, versionOf(t, ctx, o))
	got := loadOps(t, ctx, o, "s")
	require.True(t, memberOf(got, l).Changed)
	require.False(t, memberOf(got, w).Changed)
	require.Equal(t, staleSuperseded, ctx.storeRecordStale(r.ID, "b1", "s", fresh, "again", nil), "no longer open")

	// a Cluster that left the caller's scope is neither recorded nor reported
	a := makeResource(t, ctx.db, "ha", opsRes{Hash: "h", Owner: &mine.ID})
	b := makeResource(t, ctx.db, "hb", opsRes{Hash: "h", Owner: &other.ID})
	insertOps(t, ctx, r, "hidden", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	require.Equal(t, staleHidden, p.storeRecordStale(r.ID, "b1", "hidden", rowOf(t, ctx, "hidden").Fingerprint, "r", nil))
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "hidden").State)
	_, gate, err := p.storeClaimCluster(r.ID, "b1", "hidden")
	require.NoError(t, err)
	require.Equal(t, claimHidden, gate)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsRecordStaleSQLite(t *testing.T) {
	testOpsRecordStale(t, withSettings(newReductionSQLiteContext(t)))
}

// strand makes a Cluster look like an earlier crashed batch claimed it.
func strand(t *testing.T, ctx *MahresourcesContext, key string, merged bool) {
	t.Helper()
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET state = 'applied', checked = false, claim_id = ?, merged = ?, applied_at = ? WHERE "key" = ?`,
		"claim-"+key, merged, time.Now(), key).Error)
}

func testOpsReconcile(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	n := reconcileChunk*2 + 9
	var deleted []string
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("r%03d", i)
		_, l := identicalPair(t, ctx, r, key)
		strand(t, ctx, key, i%10 == 3)
		if i%7 == 0 && i%10 != 3 {
			require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", l).Error)
			deleted = append(deleted, key)
		}
	}
	r = startBatch(t, ctx, r, "b2", time.Hour)
	v := versionOf(t, ctx, r)

	// a chunk fence that is lost after the first chunk leaves the rest untouched
	calls := 0
	lossy := func(tx *gorm.DB) (string, error) {
		calls++
		if calls > 1 {
			return "", ErrReductionLeaseLost
		}
		return fenceBatch(tx, r.ID, "b2", true)
	}
	_, _, err := ctx.storeReconcileClaims(r.ID, lossy)
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	var open int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state IN ('open','stale')", r.ID).Count(&open).Error)
	require.EqualValues(t, reconcileChunk, open, "exactly the first chunk was settled before the fence was lost")
	require.Equal(t, v+1, versionOf(t, ctx, r), "one chunk reconciled, one bump")

	outcomes, total, err := ctx.storeReconcileClaims(r.ID, batchClaimFence(r.ID, "b2"))
	require.NoError(t, err)
	_ = outcomes
	// merged claims are untouched; everything else is open (checked) or stale (deleted)
	var stillApplied, merged, staleN, checkedOpen int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state = 'applied' AND NOT merged", r.ID).Count(&stillApplied).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state = 'applied' AND merged", r.ID).Count(&merged).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state = 'stale' AND stale_reason = ? AND claim_id = ''", r.ID, staleReasonDeleted).Count(&staleN).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state = 'open' AND checked AND claim_id = '' AND applied_at IS NULL", r.ID).Count(&checkedOpen).Error)
	require.Zero(t, stillApplied)
	require.EqualValues(t, len(deleted), staleN)
	require.EqualValues(t, n, int(merged+staleN+checkedOpen))
	require.LessOrEqual(t, total, len(deleted))
	require.LessOrEqual(t, len(outcomes), reductionApplyOutcomeCap)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)

	// a confined principal leaves a claim it cannot fully see exactly as it is, and
	// a settle that changes nothing leaves the version alone
	r2 := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "ca", opsRes{Hash: "h", Owner: &mine.ID})
	b := makeResource(t, ctx.db, "cb", opsRes{Hash: "h", Owner: &other.ID})
	insertOps(t, ctx, r2, "theirs", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	strand(t, ctx, "theirs", false)
	r2 = startBatch(t, ctx, r2, "b3", time.Hour)
	before := versionOf(t, ctx, r2)
	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	_, _, err = p.storeReconcileClaims(r2.ID, batchClaimFence(r2.ID, "b3"))
	require.NoError(t, err)
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "theirs").State)
	require.Equal(t, before, versionOf(t, ctx, r2), "nothing reconciled, nothing bumped")
}

func TestOpsReconcileSQLite(t *testing.T) {
	testOpsReconcile(t, withSettings(newReductionSQLiteContext(t)))
}

func testOpsSettle(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	_, l := identicalPair(t, ctx, r, "s1")
	identicalPair(t, ctx, r, "s2")
	strand(t, ctx, "s1", false)
	strand(t, ctx, "s2", false)
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", l).Error)
	// an earlier apply's evidence, which settle must not touch
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET last_apply = ? WHERE id = ?", `{"batchId":"old","generation":"g","applied":3}`, r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	lastApply := string(r.LastApply)

	// refusals
	req := &query_models.ReductionApply{ID: r.ID, Version: r.Version, SettleOnly: true}
	stale := &query_models.ReductionApply{ID: r.ID, Version: r.Version - 1, SettleOnly: true}
	_, err := ctx.storeSettleClaims(r, stale)
	require.ErrorIs(t, err, ErrReductionConflict)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", time.Now().Add(time.Hour).UnixNano(), r.ID).Error)
	_, err = ctx.storeSettleClaims(r, req)
	require.ErrorIs(t, err, ErrReductionConflict, "a live apply owns the claims")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = 0, settle_op = 'other', settle_lease_until = ? WHERE id = ?", time.Now().Add(time.Hour).UnixNano(), r.ID).Error)
	_, err = ctx.storeSettleClaims(r, req)
	require.ErrorIs(t, err, ErrReductionConflict, "another settle is running")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_op = NULL, settle_lease_until = 0, status = 'computing', compute_deadline = ? WHERE id = ?", time.Now().Add(time.Hour), r.ID).Error)
	_, err = ctx.storeSettleClaims(r, req)
	require.ErrorIs(t, err, ErrReductionBusy)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'ready' WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	req.Version = r.Version

	// a confined principal that can see none of them changes nothing and bumps nothing
	hiddenA := makeResource(t, ctx.db, "ha", opsRes{Hash: "h", Owner: &mine.ID})
	hiddenB := makeResource(t, ctx.db, "hb", opsRes{Hash: "h", Owner: &other.ID})
	insertOps(t, ctx, r, "hid", models.ReductionTierIdentical, hiddenA, opsMember(hiddenA, "h"), opsMember(hiddenB, "h"))
	strand(t, ctx, "hid", false)
	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &other.ID})
	v := versionOf(t, ctx, r)
	_, err = p.storeSettleClaims(r, req)
	require.NoError(t, err)
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "hid").State)
	require.Equal(t, v, versionOf(t, ctx, r), "nothing reconciled, nothing bumped")
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "s2").State, "s2 is not visible to a principal confined to other")

	// the unconfined settle
	res, err := ctx.storeSettleClaims(r, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, models.ReductionClusterStale, rowOf(t, ctx, "s1").State)
	require.Equal(t, staleReasonDeleted, rowOf(t, ctx, "s1").StaleReason)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "s2").State)
	require.True(t, rowOf(t, ctx, "s2").Checked)
	require.Equal(t, models.ReductionClusterOpen, rowOf(t, ctx, "hid").State)
	after := loadReductionRow(t, ctx.db, r.ID)
	require.Greater(t, after.Version, v)
	require.Equal(t, lastApply, string(after.LastApply), "settle never writes last_apply")
	require.Nil(t, after.SettleOp, "the settle fence is released")
	require.Zero(t, after.SettleLeaseUntil)

	// settleFence: the right op with a live lease, and nothing else
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_op = 'mine', settle_lease_until = ? WHERE id = ?", time.Now().Add(time.Minute).UnixNano(), r.ID).Error)
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error { _, err := settleFence(r.ID, "mine")(tx); return err }))
	require.ErrorIs(t, ctx.db.Transaction(func(tx *gorm.DB) error { _, err := settleFence(r.ID, "not-mine")(tx); return err }), ErrReductionLeaseLost)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET settle_lease_until = ? WHERE id = ?", time.Now().Add(-time.Second).UnixNano(), r.ID).Error)
	require.ErrorIs(t, ctx.db.Transaction(func(tx *gorm.DB) error { _, err := settleFence(r.ID, "mine")(tx); return err }), ErrReductionLeaseLost)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsSettleSQLite(t *testing.T) { testOpsSettle(t, withSettings(newReductionSQLiteContext(t))) }
