package application_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/auth"
	"mahresources/models"
)

func testReductionSelections(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	s := ctx.reductionStore()

	require.NoError(t, s.addExtentResources(r, []uint{3, 1, 2, 2}))
	require.NoError(t, s.addExtentResources(r, []uint{2, 4}), "widening twice with an overlap is idempotent")
	n, names, err := s.extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 4, n)
	require.Empty(t, names, "ids with no Resource row have no names")

	tooMany := make([]uint, reductionSelectionCap+1)
	for i := range tooMany {
		tooMany[i] = uint(i + 1)
	}
	require.ErrorIs(t, s.addExtentResources(r, tooMany), ErrReductionSelectionTooLarge)
	require.ErrorIs(t, s.addExtentGroups(r, tooMany), ErrReductionSelectionTooLarge)
	n, _, err = s.extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 4, n, "a refused request writes nothing")

	require.NoError(t, s.addExtentGroups(r, []uint{9, 9, 8}))
	var groups int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentGroup{}).Where("reduction_id = ?", r.ID).Count(&groups).Error)
	require.EqualValues(t, 2, groups)
}

func TestReductionSelectionsSQLite(t *testing.T) {
	testReductionSelections(t, newReductionSQLiteContext(t))
}

func testReductionSelectFromGroup(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	for i := 0; i < 1203; i++ {
		res := &models.Resource{Name: fmt.Sprintf("r%04d", i), OwnerId: &mine.ID}
		require.NoError(t, ctx.db.Create(res).Error)
	}
	twin := &models.Resource{Name: "twin", OwnerId: &other.ID}
	require.NoError(t, ctx.db.Create(twin).Error)
	r := newReductionRow(t, ctx.db)

	added, err := ctx.reductionStore().addExtentFromGroupResources(r, mine.ID)
	require.NoError(t, err)
	require.Equal(t, 1203, added)
	n, names, err := ctx.reductionStore().extentPreview(r)
	require.NoError(t, err)
	require.EqualValues(t, 1203, n)
	require.Len(t, names, reductionSelectionPreview)
	require.Equal(t, "r0000", names[0])
	var hasTwin int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentResource{}).Where("reduction_id = ? AND resource_id = ?", r.ID, twin.ID).Count(&hasTwin).Error)
	require.EqualValues(t, 0, hasTwin)

	// a principal confined to the other Group cannot take Resources of mine
	r2 := newReductionRow(t, ctx.db)
	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &other.ID})
	added, err = p.reductionStore().addExtentFromGroupResources(r2, mine.ID)
	require.NoError(t, err)
	require.Equal(t, 0, added)
}

func TestReductionSelectFromGroupSQLite(t *testing.T) {
	testReductionSelectFromGroup(t, newReductionSQLiteContext(t))
}

func testReductionLeases(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	s := ctx.reductionStore()
	gen := "gen-1"
	started := time.Now()
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'computing', compute_generation = ?, compute_lease_until = ? WHERE id = ?",
		gen, started.Add(time.Minute).UnixNano(), r.ID).Error)

	// renewal extends a live lease to now + 10 minutes, and touches neither version nor updated_at
	var before models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&before, r.ID).Error)
	ok, err := s.renewComputeLease(r.ID, gen, started, started.Add(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	var after models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&after, r.ID).Error)
	require.Equal(t, started.Add(time.Second+leaseWindow).UnixNano(), after.ComputeLeaseUntil)
	require.Equal(t, before.Version, after.Version)
	require.True(t, before.UpdatedAt.Equal(after.UpdatedAt))

	// the 24 hour ceiling: no lease runs past started + 24h
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_lease_until = ? WHERE id = ?", started.Add(24*time.Hour).UnixNano(), r.ID).Error)
	ok, err = s.renewComputeLease(r.ID, gen, started, started.Add(24*time.Hour-time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, ctx.db.Select(reductionColumns).First(&after, r.ID).Error)
	require.Equal(t, started.Add(24*time.Hour).UnixNano(), after.ComputeLeaseUntil)

	// an expired lease is never renewed
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_lease_until = ? WHERE id = ?", started.Add(-time.Hour).UnixNano(), r.ID).Error)
	ok, err = s.renewComputeLease(r.ID, gen, started, started)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, ctx.db.Select(reductionColumns).First(&after, r.ID).Error)
	require.Equal(t, started.Add(-time.Hour).UnixNano(), after.ComputeLeaseUntil)

	// a superseded generation is not renewed either
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET compute_lease_until = ? WHERE id = ?", started.Add(time.Hour).UnixNano(), r.ID).Error)
	ok, err = s.renewComputeLease(r.ID, "gen-2", started, started)
	require.NoError(t, err)
	require.False(t, ok)

	// the fence: live lease and matching generation hold; after expiry it fails
	require.NoError(t, s.fenceCompute(r.ID, gen, started))
	require.ErrorIs(t, s.fenceCompute(r.ID, gen, started.Add(2*time.Hour)), ErrReductionLeaseLost)
	require.ErrorIs(t, s.fenceCompute(r.ID, "gen-2", started), ErrReductionLeaseLost)
}

func TestReductionLeasesSQLite(t *testing.T) { testReductionLeases(t, newReductionSQLiteContext(t)) }
