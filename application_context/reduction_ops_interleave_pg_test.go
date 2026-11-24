//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

// requireShareLocked proves the parked transaction really holds a lock on the
// Resource: another transaction cannot take it for update without waiting. A test
// that only watched for a delay could pass with no contention at all.
func requireShareLocked(t *testing.T, db *gorm.DB, resourceID uint) {
	t.Helper()
	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`SELECT id FROM resources WHERE id = ? FOR UPDATE NOWAIT`, resourceID).Error
	})
	require.Error(t, err, "the parked writer holds no lock on the Resource")
	require.Contains(t, err.Error(), "55P03", "the failure is a lock conflict")
}

// requireWaitsOnLock waits until a backend running a statement that starts with
// prefix is blocked on a lock, and fails if done reports the statement finished
// first: the competing writer really waited.
func requireWaitsOnLock(t *testing.T, db *gorm.DB, prefix string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("%s finished instead of waiting on the lock (err %v)", prefix, err)
		default:
		}
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE ?`, prefix+"%").Scan(&n).Error)
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no backend running %q was ever blocked on a lock", prefix)
}

// park runs the writer's transaction until it holds its locks, then holds it there
// until release is closed. The hook is process-wide, so the tests using it do not
// run in parallel.
func park(t *testing.T) (parked chan struct{}, release chan struct{}) {
	t.Helper()
	parked, release = make(chan struct{}), make(chan struct{})
	hook := func() {
		reductionOpsAfterLocksHook.Store(nil) // only the first transaction parks
		close(parked)
		<-release
	}
	reductionOpsAfterLocksHook.Store(&hook)
	t.Cleanup(func() { reductionOpsAfterLocksHook.Store(nil) })
	return parked, release
}

// An injected interleave, not a race: a bulk approval is parked holding its share
// locks on a Cluster's Resources, and a deleting transaction (Resources first, then
// the Cluster rows and members, as the deletion hook does) is started. The delete
// must wait for the approval, and once it has the approval stands and the Cluster
// reads stale, with the derived columns still true. No deadlock, whichever way the
// two orders would have crossed.
func TestOpsDeleteVsBulkApprovePG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h"})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "c1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = false WHERE "key" = 'c1'`).Error)

	parked, release := park(t)
	approved := make(chan error, 1)
	go func() {
		_, err := ctx.storeBulkOverride(r, bulk(r, ReductionActionCheck, []string{"c1"}, false, nil))
		approved <- err
	}()
	<-parked
	requireShareLocked(t, ctx.db, b)

	deleted := make(chan error, 1)
	go func() {
		deleted <- ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`DELETE FROM resources WHERE id = ?`, b).Error; err != nil {
				return err
			}
			if err := tx.Exec(`SELECT id FROM reduction_clusters WHERE id IN (SELECT cluster_id FROM reduction_members WHERE resource_id = ?) ORDER BY id FOR UPDATE`, b).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE reduction_members SET gone = true WHERE resource_id = ?`, b).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE reduction_clusters SET gone_members = (SELECT COUNT(*) FROM reduction_members m WHERE m.cluster_id = reduction_clusters.id AND NOT m.ejected AND m.gone)
				WHERE id IN (SELECT cluster_id FROM reduction_members WHERE resource_id = ?)`, b).Error
		})
	}()
	requireWaitsOnLock(t, ctx.db, "DELETE FROM resources", deleted)
	close(release)
	require.NoError(t, <-approved)
	require.NoError(t, <-deleted)

	row := rowOf(t, ctx, "c1")
	require.True(t, row.Checked, "the approval landed")
	require.True(t, row.Reviewed)
	require.Equal(t, 1, row.GoneMembers)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

// A Resource moving out of the principal's subtree cannot slip between the
// visibility decision and the approval: the share lock holds the move until the
// approval commits.
func TestOpsOwnerMoveVsScopedBulkApprovePG(t *testing.T) {
	ctx := newOpsPGContext(t)
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h", Owner: &mine.ID})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h", Owner: &mine.ID})
	insertOps(t, ctx, r, "c1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = false WHERE "key" = 'c1'`).Error)

	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &mine.ID})
	parked, release := park(t)
	approved := make(chan error, 1)
	go func() {
		_, err := p.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, uintPtr(1)))
		approved <- err
	}()
	<-parked
	requireShareLocked(t, ctx.db, b)
	moved := make(chan error, 1)
	go func() {
		moved <- ctx.db.Exec(`UPDATE resources SET owner_id = ? WHERE id = ?`, other.ID, b).Error
	}()
	requireWaitsOnLock(t, ctx.db, "UPDATE resources SET owner_id", moved)
	close(release)
	require.NoError(t, <-approved, "visibility was decided with the members locked, so the count the button showed still held")
	require.NoError(t, <-moved)
	require.True(t, rowOf(t, ctx, "c1").Checked)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

// A claim parked holding the Reduction row: an override of the same Cluster waits
// for it, and is refused once the Cluster is claimed.
func TestOpsClaimVsOverrideEjectPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r := newPlanReduction(t, ctx.db)
	w := makeResource(t, ctx.db, "w", opsRes{Hash: "h"})
	l1 := makeResource(t, ctx.db, "l1", opsRes{Hash: "h"})
	l2 := makeResource(t, ctx.db, "l2", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "c1", models.ReductionTierIdentical, w, opsMember(w, "h"), opsMember(l1, "h"), opsMember(l2, "h"))
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "c1")

	parked, release := park(t)
	claimed := make(chan *storedCluster, 1)
	go func() {
		c, _, _ := ctx.storeClaimCluster(r.ID, "b1", "c1")
		claimed <- c
	}()
	<-parked
	ejected := make(chan error, 1)
	go func() {
		_, err := ctx.storeOverrideCluster(r, override(r, "c1", ReductionActionEject, l2))
		ejected <- err
	}()
	select {
	case err := <-ejected:
		t.Fatalf("the eject did not wait for the claim (err %v)", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	c := <-claimed
	require.NotNil(t, c)
	require.ErrorIs(t, <-ejected, ErrReductionConflict, "the claim moved the version while the eject waited on the Reduction row")
	fresh := loadReductionRow(t, ctx.db, r.ID)
	_, err := ctx.storeOverrideCluster(fresh, override(fresh, "c1", ReductionActionEject, l2))
	require.ErrorIs(t, err, ErrReductionClusterSettled, "once claimed, an override refuses: the ids about to be merged are frozen")
	require.Equal(t, models.ReductionClusterApplied, rowOf(t, ctx, "c1").State)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

// A Group re-parent is not blocked by the Resource share locks: it can take a
// Cluster out of the principal's subtree after the bulk write selected it. The
// marking statement judges visibility itself, so the approval is refused instead of
// landing on a Cluster the principal can no longer see.
func TestOpsReparentVsScopedBulkMarkPG(t *testing.T) {
	for _, byIDs := range []bool{true, false} {
		ctx := newOpsPGContext(t)
		mine := &models.Group{Name: "mine"}
		require.NoError(t, ctx.db.Create(mine).Error)
		sub := &models.Group{Name: "sub", OwnerId: &mine.ID}
		require.NoError(t, ctx.db.Create(sub).Error)
		r := newPlanReduction(t, ctx.db)
		a := makeResource(t, ctx.db, "a", opsRes{Hash: "h", Owner: &sub.ID})
		b := makeResource(t, ctx.db, "b", opsRes{Hash: "h", Owner: &sub.ID})
		insertOps(t, ctx, r, "c1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
		require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = false WHERE "key" = 'c1'`).Error)
		p := ctx.WithPrincipal(accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID))

		parked, release := park(t)
		done := make(chan error, 1)
		go func() {
			var err error
			if byIDs {
				_, err = p.storeBulkOverride(r, bulk(r, ReductionActionCheck, []string{"c1"}, false, nil))
			} else {
				_, err = p.storeBulkOverride(r, bulk(r, ReductionActionCheck, nil, true, uintPtr(1)))
			}
			done <- err
		}()
		<-parked
		require.NoError(t, ctx.db.Exec(`UPDATE groups SET owner_id = NULL WHERE id = ?`, sub.ID).Error, "a re-parent is not held off by the share locks")
		close(release)
		err := <-done
		if byIDs {
			require.ErrorIs(t, err, ErrReductionClusterNotFound, "the Cluster is withheld by the time it is marked")
		} else {
			require.ErrorIs(t, err, ErrReductionCountMismatch, "the count the button showed no longer holds")
		}
		require.False(t, rowOf(t, ctx, "c1").Checked, "nothing was approved")
		requireNoBulkMarks(t, ctx, r)
	}
}

// Reconciliation share-locks the members of 50 Clusters in keyset chunks: no single
// statement returns more than reductionRowChunk of them.
func TestOpsReconcileMemberReadIsBoundedPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r := newPlanReduction(t, ctx.db)
	const clusters, perCluster = 50, 20 // 1,000 members
	for i := 0; i < clusters; i++ {
		key := fmt.Sprintf("c%02d", i)
		first := makeResource(t, ctx.db, key+"-0", opsRes{Hash: "h"})
		members := []models.ReductionMemberRow{opsMember(first, "h")}
		for j := 1; j < perCluster; j++ {
			members = append(members, opsMember(makeResource(t, ctx.db, fmt.Sprintf("%s-%d", key, j), opsRes{Hash: "h"}), "h"))
		}
		insertOps(t, ctx, r, key, models.ReductionTierIdentical, first, members...)
		strand(t, ctx, key, false)
	}
	r = startBatch(t, ctx, r, "b1", time.Hour)

	// A raw scan runs the Row chain; the statement's last bind is its LIMIT.
	var widest int64
	reads := 0
	const name = "capture-member-reads"
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name, func(d *gorm.DB) {
		sql := d.Statement.SQL.String()
		if strings.Contains(sql, "reduction_members") && strings.Contains(sql, "GROUP BY") {
			reads++
			if limit, ok := d.Statement.Vars[len(d.Statement.Vars)-1].(int); ok && int64(limit) > widest {
				widest = int64(limit)
			}
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Row().Remove(name) })

	_, _, err := ctx.storeReconcileClaims(r.ID, batchClaimFence(r.ID, "b1"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, reads, 2, "1,000 members take more than one chunk")
	require.LessOrEqual(t, widest, int64(reductionRowChunk), "no member read returns more than a chunk")
	var open int64
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("reduction_id = ? AND state = 'open'", r.ID).Count(&open).Error)
	require.EqualValues(t, clusters, open)
}

func TestOpsAccountNarrowedMidRequestPG(t *testing.T) {
	testOpsAccountNarrowedMidRequest(t, newOpsPGContext(t))
}
func TestOpsSettleOwnerNulledSinceLoadPG(t *testing.T) {
	testOpsSettleOwnerNulledSinceLoad(t, newOpsPGContext(t))
}
func TestStoreApplyEvidenceForEveryOutcomeKindPG(t *testing.T) {
	testStoreApplyEvidenceForEveryOutcomeKind(t, newOpsPGContext(t))
}
func TestOpsLeaseRecheckedAfterLocksPG(t *testing.T) {
	testOpsLeaseRecheckedAfterLocks(t, newOpsPGContext(t))
}
func TestOpsBulkMarkRechecksVisibilityPG(t *testing.T) {
	testOpsBulkMarkRechecksVisibility(t, newOpsPGContext(t))
}
func TestOpsDemoteNeedsLiveLeasePG(t *testing.T) { testOpsDemoteNeedsLiveLease(t, newOpsPGContext(t)) }
func TestStoreApplyFailedStaleWriteIsAnErrorPG(t *testing.T) {
	testStoreApplyFailedStaleWriteIsAnError(t, newOpsPGContext(t))
}
func TestOpsReconcileHasAnIndexPG(t *testing.T) { testOpsReconcileHasAnIndex(t, newOpsPGContext(t)) }
func TestStoreApplyBookkeepingDoesNotBreakAComputePG(t *testing.T) {
	testStoreApplyBookkeepingDoesNotBreakACompute(t, newOpsPGContext(t))
}
func TestSweepOrphansDeletesBoundedChunksPG(t *testing.T) {
	testSweepOrphansDeletesBoundedChunks(t, newOpsPGContext(t))
}
func TestMergeRenewsBetweenWorkUnitsPG(t *testing.T) {
	testMergeRenewsBetweenWorkUnits(t, newOpsPGContext(t))
}
func TestStoreMergeRenewHookPG(t *testing.T) { testStoreMergeRenewHook(t, newOpsPGContext(t)) }
func TestStoreApplyReconcileErrorDoesNotFinishPG(t *testing.T) {
	testStoreApplyReconcileErrorDoesNotFinish(t, newOpsPGContext(t))
}
