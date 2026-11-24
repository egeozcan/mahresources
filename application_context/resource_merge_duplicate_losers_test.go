package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// Naming a Loser twice is refused with a clear error and changes nothing: it used to
// fail and roll back, and with the set-based delete it would otherwise commit two
// versions for one delete.
func testMergeRefusesDuplicateLosers(t *testing.T, ctx *MahresourcesContext) {
	winner := mkResource(t, ctx, models.Resource{Name: "dup-winner"})
	loser := mkResource(t, ctx, models.Resource{Name: "dup-loser"})

	err := ctx.MergeResources(winner.ID, []uint{loser.ID, loser.ID}, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")

	var n int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id = ?", loser.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "the loser is still there")
	require.NoError(t, ctx.db.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&n).Error)
	assert.Zero(t, n, "no version was created")
}

func TestMergeRefusesDuplicateLosersSQLite(t *testing.T) {
	testMergeRefusesDuplicateLosers(t, newReductionSQLiteContext(t))
}
