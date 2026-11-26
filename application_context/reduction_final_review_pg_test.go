//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func TestReductionMigrationRunnerRetriesPastALiveLeasePG(t *testing.T) {
	testReductionMigrationRunnerRetriesPastALiveLease(t, newReductionPGContext(t))
}
func TestReductionMigrationRefusesNonIntegerIDsPG(t *testing.T) {
	testReductionMigrationRefusesNonIntegerIDs(t, newReductionPGContext(t))
}
func TestReductionSpoolChangedMembersLinearPG(t *testing.T) {
	testReductionSpoolChangedMembersLinear(t, newReductionPGContext(t))
}
func TestReductionListChunksSeekOnTheWholeKeyPG(t *testing.T) {
	testReductionListChunksSeekOnTheWholeKey(t, newReductionPGContext(t))
}
func TestReductionSelectSubtreeIsSetBasedPG(t *testing.T) {
	testReductionSelectSubtreeIsSetBased(t, newReductionPGContext(t))
}
func TestReductionMigrationScansInChunksPG(t *testing.T) {
	testReductionMigrationScansInChunks(t, newReductionPGContext(t))
}
func TestReductionSweepsOrphanSpoolPG(t *testing.T) {
	testReductionSweepsOrphanSpool(t, newReductionPGContext(t))
}
func TestSettleableClaimsHonoursTheSettleLeasePG(t *testing.T) {
	testSettleableClaimsHonoursTheSettleLease(t, newReductionPGContext(t))
}

// (9) The over-cap Cluster's aggregate is computed under the Cluster's row lock, so a
// deletion hook that commits between the aggregate and the update cannot have its
// newer gone_members overwritten. The hook is started inside the window; with the
// lock it waits for the migration's transaction, without it it commits first and
// the update then writes the stale figure.
func TestReductionMigrationOverCapAggregateHoldsTheClusterLockPG(t *testing.T) {
	ctx := newReductionPGContext(t)
	ids := makeResources(t, ctx, 1)
	var members []string
	members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, ids[0]))
	for i := 0; i < 250; i++ {
		members = append(members, fmt.Sprintf(`{"resourceId":%d,"hash":"h"}`, 400000+i))
	}
	red := createLegacyReduction(t, ctx, fmt.Sprintf(
		`{"clusters":[{"id":"big","tier":"identical","winnerId":%d,"state":"open","members":[%s]}],"coverage":{},"settings":{}}`,
		ids[0], joinStrings(members)), "")
	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{ids[0]}))

	hookDone := make(chan error, 1)
	hookFinishedInsideWindow := false
	migrationAfterAggregate = func() {
		go func() {
			hookDone <- ctx.db.Transaction(func(tx *gorm.DB) error { return reductionDeletionHook(tx, op) })
		}()
		select {
		case err := <-hookDone:
			hookDone <- err
			hookFinishedInsideWindow = true
		case <-time.After(700 * time.Millisecond):
		}
	}
	t.Cleanup(func() { migrationAfterAggregate = nil })
	require.NoError(t, ctx.MigrateReductionPlans())
	require.NoError(t, <-hookDone)
	require.False(t, hookFinishedInsideWindow, "the hook waited for the Cluster lock the aggregate holds")
	var goneRows int64
	require.NoError(t, ctx.db.Model(&models.ReductionMemberRow{}).Where("reduction_id = ? AND gone = ? AND ejected = ?", red.ID, true, false).Count(&goneRows).Error)
	require.EqualValues(t, 251, goneRows)
	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ?", red.ID).First(&row).Error)
	require.Equal(t, 251, row.GoneMembers, "250 missing Resources and the one the hook marked")
}
