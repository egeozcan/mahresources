package application_context

import (
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// (6) Choosing an owner Group as a confined principal asks whether the Group is in
// the principal's subtree. That is one bind (the subtree CTE), not the subtree's
// ids expanded into the statement: a subtree past 32,766 Groups exceeded SQLite's
// variable limit and answered "not visible".
func testReductionOwnerSelectionInAHugeScopedSubtree(t *testing.T, ctx *MahresourcesContext) {
	root := &models.Group{Name: "root"}
	require.NoError(t, ctx.db.Create(root).Error)
	kids := make([]models.Group, 33000)
	for i := range kids {
		kids[i] = models.Group{Name: "kid", OwnerId: &root.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(kids, 500).Error)
	res := &models.Resource{Name: "owned", OwnerId: &root.ID}
	require.NoError(t, ctx.db.Create(res).Error)

	w := &scopedSelectionWorld{}
	p := w.principal(ctx, root.ID)
	got, err := p.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: root.ID, IncludeDescendants: true}, nil, false)
	require.NoError(t, err)
	require.Equal(t, []uint{res.ID}, selectedResourceIDs(t, ctx.db, got.ID))

	// The same check still refuses a Group outside the subtree.
	other := &models.Group{Name: "outside"}
	require.NoError(t, ctx.db.Create(other).Error)
	_, err = p.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: other.ID}, nil, false)
	require.ErrorIs(t, err, ErrReductionGroupNotVisible)
}

// (7) The 10,000 cap is on the request's Resource ids and Group ids together.
func testReductionSelectionCapCountsBothLists(t *testing.T, ctx *MahresourcesContext) {
	resources := make([]uint, 5001)
	groups := make([]uint, 5000)
	for i := range resources {
		resources[i] = uint(i + 1)
	}
	for i := range groups {
		groups[i] = uint(i + 1)
	}
	_, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ResourceIds: resources, GroupIds: groups}, nil, false)
	require.ErrorIs(t, err, ErrReductionSelectionTooLarge)
	var n int64
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Count(&n).Error)
	require.Zero(t, n, "a refused request leaves nothing behind")
}

func TestReductionOwnerSelectionInAHugeScopedSubtreeSQLite(t *testing.T) {
	testReductionOwnerSelectionInAHugeScopedSubtree(t, newReductionSQLiteContext(t))
}

func TestReductionSelectionCapCountsBothListsSQLite(t *testing.T) {
	testReductionSelectionCapCountsBothLists(t, newReductionSQLiteContext(t))
}
