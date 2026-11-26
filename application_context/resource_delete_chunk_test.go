package application_context

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// Design rule 2 for the bulk Resource delete: no statement writes more than one
// chunk of rows, however many ids the request names (up to its cap of 10,000).
// Found by the D14 scale proof (reduction_scale_test.go): the purge of the
// deletion's staged ids and the manifest drain's claim renewal each ran as one
// statement over every row of the operation.
func TestBulkDeleteStatementsStayWithinAChunk(t *testing.T) {
	testBulkDeleteStatementsStayWithinAChunk(t, newReductionSQLiteContext(t))
}

func testBulkDeleteStatementsStayWithinAChunk(t *testing.T, ctx *MahresourcesContext) {
	n := 3*deleteStageChunk + 123
	rows := make([]models.Resource, n)
	for i := range rows {
		rows[i] = models.Resource{Name: fmt.Sprintf("chunk-%d", i), Hash: fmt.Sprintf("chunk-hash-%d", i), ResourceCategoryId: 1}
	}
	require.NoError(t, ctx.db.CreateInBatches(&rows, 200).Error)
	ids := make([]uint, n)
	for i := range rows {
		ids[i] = rows[i].ID
	}

	rec := installScaleRecorder(t, ctx)
	rec.arm(true)
	require.NoError(t, ctx.BulkDeleteResources(&query_models.BulkQuery{ID: ids}))
	rec.arm(false)
	rec.requireWithinChunk(t, "bulk delete")

	var left int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id IN ?", ids).Count(&left).Error)
	require.Zero(t, left)
	var staged, manifest int64
	require.NoError(t, ctx.db.Model(&models.ReductionDeletedID{}).Count(&staged).Error)
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&manifest).Error)
	require.Zero(t, staged, "the staged ids are purged")
	require.Zero(t, manifest, "the manifest is drained")
}
