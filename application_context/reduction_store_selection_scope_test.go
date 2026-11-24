package application_context

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

// scopedSelectionWorld is two sibling Groups, a child of the first, and a
// Resource in each plus one with no owner.
type scopedSelectionWorld struct {
	a, b, child                *models.Group
	inA, inB, inChild, noOwner *models.Resource
}

func newScopedSelectionWorld(t *testing.T, ctx *MahresourcesContext) *scopedSelectionWorld {
	t.Helper()
	w := &scopedSelectionWorld{a: &models.Group{Name: "A"}, b: &models.Group{Name: "B"}}
	require.NoError(t, ctx.db.Create(w.a).Error)
	require.NoError(t, ctx.db.Create(w.b).Error)
	w.child = &models.Group{Name: "A child", OwnerId: &w.a.ID}
	require.NoError(t, ctx.db.Create(w.child).Error)
	mk := func(name string, owner *uint) *models.Resource {
		r := &models.Resource{Name: name, OwnerId: owner}
		require.NoError(t, ctx.db.Create(r).Error)
		return r
	}
	w.inA, w.inB, w.inChild, w.noOwner = mk("in-a", &w.a.ID), mk("in-b", &w.b.ID), mk("in-child", &w.child.ID), mk("no-owner", nil)
	return w
}

func (w *scopedSelectionWorld) principal(ctx *MahresourcesContext, group uint) *MahresourcesContext {
	return ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &group})
}

func selectedResourceIDs(t *testing.T, db *gorm.DB, reductionID uint) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ?", reductionID).Order("resource_id").Pluck("resource_id", &ids).Error)
	return ids
}

// A principal confined to Group A cannot select Resources outside A's subtree:
// they are skipped, as the scoped Resource query skips them on today's create.
func testReductionSelectionSkipsResourcesOutsideScope(t *testing.T, ctx *MahresourcesContext) {
	w := newScopedSelectionWorld(t, ctx)
	r := newReductionRow(t, ctx.db)
	p := w.principal(ctx, w.a.ID)
	require.NoError(t, p.reductionStore().addExtentResources(r, []uint{w.inA.ID, w.inB.ID, w.inChild.ID, w.noOwner.ID, 987654}))
	require.Equal(t, []uint{w.inA.ID, w.inChild.ID}, selectedResourceIDs(t, ctx.db, r.ID))

	deny := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, UserID: 6})
	r2 := newReductionRow(t, ctx.db)
	require.NoError(t, deny.reductionStore().addExtentResources(r2, []uint{w.inA.ID}))
	require.Empty(t, selectedResourceIDs(t, ctx.db, r2.ID), "a principal with no scope Group selects nothing")
}

// Every Group a confined principal names must be in its subtree: the request is
// refused whole, as an invisible owner Group is refused ("no such Group") today.
func testReductionSelectionRefusesGroupsOutsideScope(t *testing.T, ctx *MahresourcesContext) {
	w := newScopedSelectionWorld(t, ctx)
	r := newReductionRow(t, ctx.db)
	p := w.principal(ctx, w.a.ID)
	require.ErrorIs(t, p.reductionStore().addExtentGroups(r, []uint{w.a.ID, w.b.ID}), ErrReductionGroupNotVisible)
	require.ErrorIs(t, p.reductionStore().addExtentGroups(r, []uint{987654}), ErrReductionGroupNotVisible)
	var n int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentGroup{}).Where("reduction_id = ?", r.ID).Count(&n).Error)
	require.EqualValues(t, 0, n, "a refused request writes nothing")
	require.NoError(t, p.reductionStore().addExtentGroups(r, []uint{w.a.ID, w.child.ID}))
	require.NoError(t, ctx.db.Model(&models.ReductionExtentGroup{}).Where("reduction_id = ?", r.ID).Count(&n).Error)
	require.EqualValues(t, 2, n)
	deny := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, UserID: 6})
	require.ErrorIs(t, deny.reductionStore().addExtentGroups(r, []uint{w.a.ID}), ErrReductionGroupNotVisible)
}

// The preview counts only Resources the caller can see: a Resource moved out of
// the subtree after it was selected is neither counted nor named.
func testReductionPreviewCountsOnlyVisible(t *testing.T, ctx *MahresourcesContext) {
	w := newScopedSelectionWorld(t, ctx)
	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.reductionStore().addExtentResources(r, []uint{w.inA.ID, w.inB.ID, w.noOwner.ID}))
	n, names, err := ctx.reductionStore().extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
	require.Len(t, names, 3)

	p := w.principal(ctx, w.a.ID)
	n, names, err = p.reductionStore().extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "the two Resources outside the subtree are not counted")
	require.Equal(t, []string{"in-a"}, names)

	deny := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, UserID: 6})
	n, names, err = deny.reductionStore().extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
	require.Empty(t, names)
}

// The cap holds inside the insert transaction: Resources that arrive in the Group
// after the preflight count are counted by the chunk that sees them, and the
// whole request rolls back.
func testReductionSelectFromGroupCapHoldsUnderConcurrentInsert(t *testing.T, ctx *MahresourcesContext) {
	g := &models.Group{Name: "big"}
	require.NoError(t, ctx.db.Create(g).Error)
	batch := make([]models.Resource, reductionSelectionCap-1)
	for i := range batch {
		batch[i] = models.Resource{Name: fmt.Sprintf("b%05d", i), OwnerId: &g.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(batch, 1000).Error)
	r := newReductionRow(t, ctx.db)

	orig := selectionAfterPreflight
	t.Cleanup(func() { selectionAfterPreflight = orig })
	selectionAfterPreflight = func(tx *gorm.DB) {
		for i := 0; i < 5; i++ {
			require.NoError(t, tx.Create(&models.Resource{Name: fmt.Sprintf("late%d", i), OwnerId: &g.ID}).Error)
		}
	}
	_, err := ctx.reductionStore().addExtentFromGroupResources(r, g.ID)
	require.ErrorIs(t, err, ErrReductionSelectionTooLarge)
	require.Empty(t, selectedResourceIDs(t, ctx.db, r.ID), "the refused request writes nothing")
}

func TestReductionSelectionSkipsResourcesOutsideScopeSQLite(t *testing.T) {
	testReductionSelectionSkipsResourcesOutsideScope(t, newReductionSQLiteContext(t))
}

func TestReductionSelectionRefusesGroupsOutsideScopeSQLite(t *testing.T) {
	testReductionSelectionRefusesGroupsOutsideScope(t, newReductionSQLiteContext(t))
}

func TestReductionPreviewCountsOnlyVisibleSQLite(t *testing.T) {
	testReductionPreviewCountsOnlyVisible(t, newReductionSQLiteContext(t))
}

func TestReductionSelectFromGroupCapHoldsUnderConcurrentInsertSQLite(t *testing.T) {
	testReductionSelectFromGroupCapHoldsUnderConcurrentInsert(t, newReductionSQLiteContext(t))
}
