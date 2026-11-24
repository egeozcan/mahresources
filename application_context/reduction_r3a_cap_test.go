package application_context

import (
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

func TestOwnerExpansionCountsAgainstTheRequestCapSQLite(t *testing.T) {
	testOwnerExpansionCountsAgainstTheRequestCap(t, newReductionSQLiteContext(t))
}

// The per-request cap is on everything one request selects. Ten thousand explicit
// ids and a Group's own Resources on top of them is more than that, and the whole
// request is refused with nothing written.
func testOwnerExpansionCountsAgainstTheRequestCap(t *testing.T, ctx *MahresourcesContext) {
	g := &models.Group{Name: "owner"}
	require.NoError(t, ctx.db.Create(g).Error)
	for i := 0; i < 5; i++ {
		require.NoError(t, ctx.db.Create(&models.Resource{Name: "owned", OwnerId: &g.ID}).Error)
	}
	explicit := make([]uint, reductionSelectionCap)
	for i := range explicit {
		explicit[i] = uint(1_000_000 + i)
	}

	_, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "capped", ResourceIds: explicit, OwnerId: g.ID}, nil, false)
	require.ErrorIs(t, err, ErrReductionSelectionTooLarge)

	var reductions, selected int64
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Count(&reductions).Error)
	require.NoError(t, ctx.db.Model(&models.ReductionExtentResource{}).Count(&selected).Error)
	require.Zero(t, reductions, "the refused request created nothing")
	require.Zero(t, selected, "the refused request selected nothing")

	// Within the cap the same shape is accepted.
	_, err = ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "fits", ResourceIds: explicit[:reductionSelectionCap-5], OwnerId: g.ID}, nil, false)
	require.NoError(t, err)
}
