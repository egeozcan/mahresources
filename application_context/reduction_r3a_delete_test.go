package application_context

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func TestReductionDeleteChunksAreExactSQLite(t *testing.T) {
	testReductionDeleteChunksAreExact(t, newReductionSQLiteContext(t))
}

// A staging table is keyed (reduction, generation, resource). The chunk read returns
// reductionRowChunk keys, and a delete that named them by key alone also took the
// same key out of every other generation: 167 keys across 3 generations is 501 rows,
// and the first 500 end inside a key, so one statement removed 501.
func testReductionDeleteChunksAreExact(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	const keys = 167
	var rows []models.ReductionExtentRow
	for _, gen := range []string{"g1", "g2", "g3"} {
		for k := 1; k <= keys; k++ {
			rows = append(rows, models.ReductionExtentRow{ReductionID: r.ID, Generation: gen, ResourceID: uint(k)})
		}
	}
	require.NoError(t, ctx.db.CreateInBatches(rows, 200).Error)

	var widest atomic.Int64
	name := "r3a:widest-extent-delete"
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(name, func(d *gorm.DB) {
		sql := strings.ToUpper(d.Statement.SQL.String())
		if strings.HasPrefix(sql, "DELETE FROM REDUCTION_EXTENT ") && d.RowsAffected > widest.Load() {
			widest.Store(d.RowsAffected)
		}
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(name) })

	require.NoError(t, ctx.DeleteResourceReduction(r.ID, nil, false))

	var left int64
	require.NoError(t, ctx.db.Model(&models.ReductionExtentRow{}).Where("reduction_id = ?", r.ID).Count(&left).Error)
	require.EqualValues(t, 0, left)
	require.LessOrEqual(t, widest.Load(), int64(reductionRowChunk), "a delete statement removed more rows than the chunk it read")
}
