package application_context

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// countFullResourceRows counts Resource rows hydrated with every column. A whole-
// library Reduction on a library of millions died of exactly this: the
// Near-Identical tier loaded the full row of every hashed image before looking at
// a single pair, and a row there averages 16 kB.
func countFullResourceRows(t *testing.T, ctx *MahresourcesContext) *atomic.Int64 {
	t.Helper()
	var rows atomic.Int64
	name := "test:count_full_resource_rows:" + t.Name()
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, func(db *gorm.DB) {
		if db.Statement.Table == "resources" && len(db.Statement.Selects) == 0 {
			if _, ok := db.Statement.Dest.(*[]*models.Resource); ok {
				rows.Add(db.Statement.RowsAffected)
			}
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })
	return &rows
}

func newReductionComputeContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := createTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.RuntimeSetting{}))
	settings := NewRuntimeSettings(ctx.db, &stubLogger{}, buildSpecs(), BuildDefaultsFromConfig(ctx.Config))
	require.NoError(t, settings.Load())
	ctx.SetSettings(settings)
	return ctx
}

func seedHashedImages(t *testing.T, ctx *MahresourcesContext, n int) []uint {
	t.Helper()
	ids := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		r := &models.Resource{
			Name:        fmt.Sprintf("image-%d.png", i),
			Hash:        fmt.Sprintf("hash-%04d", i),
			ContentType: "image/png",
			Width:       100, Height: 100, FileSize: int64(1000 + i),
		}
		require.NoError(t, ctx.db.Create(r).Error)
		p := int64(i)
		require.NoError(t, ctx.db.Create(&models.ImageHash{ResourceId: &r.ID, PHashInt: &p}).Error)
		ids = append(ids, r.ID)
	}
	return ids
}

func TestNearIdenticalComputeHydratesOnlyPairedImages(t *testing.T) {
	ctx := newReductionComputeContext(t)
	ids := seedHashedImages(t, ctx, 40)
	d := uint8(2)
	require.NoError(t, ctx.db.Create(&models.ResourceSimilarity{
		ResourceID1: ids[0], ResourceID2: ids[1], HammingDistance: d, PDistance: &d, ADistance: &d,
	}).Error)

	full := countFullResourceRows(t, ctx)
	extent := &ReductionExtent{All: true, ResourceIDs: map[uint]bool{}, GroupIDs: map[uint]bool{}}
	clusters, err := ctx.clusterNearIdentical(t.Context(), extent, models.DefaultWinnerRule(), map[uint]bool{}, nil)
	require.NoError(t, err)

	require.Len(t, clusters, 1)
	var members []uint
	for _, m := range clusters[0].Members {
		members = append(members, m.ResourceID)
	}
	assert.ElementsMatch(t, []uint{ids[0], ids[1]}, members)
	assert.LessOrEqual(t, full.Load(), int64(2),
		"only the members of a Cluster are hydrated in full; 38 images with no stored pair are not")
}

func TestIdenticalComputeCollectsOnlyRepeatedHashes(t *testing.T) {
	ctx := newReductionComputeContext(t)
	ids := seedHashedImages(t, ctx, 6)
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id = ?", ids[1]).Update("hash", "hash-0000").Error)

	got, err := ctx.repeatedHashesIn(ids)
	require.NoError(t, err)
	assert.Equal(t, []string{"hash-0000"}, got, "a hash only one Resource holds cannot form a Cluster and is not kept")

	extent := &ReductionExtent{All: true, ResourceIDs: map[uint]bool{}, GroupIDs: map[uint]bool{}}
	clusters, err := ctx.clusterIdentical(extent, models.DefaultWinnerRule(), map[uint]bool{}, nil)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Len(t, clusters[0].Members, 2)
}
