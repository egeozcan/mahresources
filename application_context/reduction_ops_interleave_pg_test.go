//go:build postgres && json1 && fts5

package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

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
	select {
	case err := <-deleted:
		t.Fatalf("the delete did not wait for the approval's share lock (err %v)", err)
	case <-time.After(400 * time.Millisecond):
	}
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
	moved := make(chan error, 1)
	go func() {
		moved <- ctx.db.Exec(`UPDATE resources SET owner_id = ? WHERE id = ?`, other.ID, b).Error
	}()
	select {
	case err := <-moved:
		t.Fatalf("the owner move did not wait for the approval (err %v)", err)
	case <-time.After(400 * time.Millisecond):
	}
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
