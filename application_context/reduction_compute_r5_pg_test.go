//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

func TestCarryOverRetriesTheFailedMemberChunkPG(t *testing.T) {
	testCarryOverRetriesTheFailedChunk(t, newOpsPGContext(t), "members")
}
func TestCarryOverRetriesTheFailedClusterChunkPG(t *testing.T) {
	testCarryOverRetriesTheFailedChunk(t, newOpsPGContext(t), "clusters")
}
func TestCarryVerificationRefusesAnIncompleteCopyPG(t *testing.T) {
	testCarryVerificationRefusesAnIncompleteCopy(t, newOpsPGContext(t))
}

func TestLandingRechecksCancelUnderTheRowLockPG(t *testing.T) {
	testLandingRechecksCancelUnderTheRowLock(t, newOpsPGContext(t))
}
func TestInsertionRefusesAMemberMovedOutOfTheSubtreePG(t *testing.T) {
	testInsertionRefusesAMemberMovedOutOfTheSubtree(t, newOpsPGContext(t))
}
func TestAssociationsRuleOverAHugeSubtreePG(t *testing.T) {
	testAssociationsRuleOverAHugeSubtree(t, newOpsPGContext(t))
}
func TestEjectedSurvivorOfAMergedClusterStaysOutPG(t *testing.T) {
	testEjectedSurvivorOfAMergedClusterStaysOut(t, newOpsPGContext(t))
}
func TestRelatedGroupStagingProbesPerResourcePG(t *testing.T) {
	testRelatedGroupStagingProbesPerResource(t, newOpsPGContext(t))
}
func TestHolderCursorIsServedByTheHashIDIndexPG(t *testing.T) {
	testHolderCursorIsServedByTheHashIDIndex(t, newOpsPGContext(t))
}

func TestOrphanSweepContinuesFromTheLastDeletedKeyPG(t *testing.T) {
	testOrphanSweepContinuesFromTheLastDeletedKey(t, newOpsPGContext(t))
}
func TestSpoolHonoursACancelledRunnerPG(t *testing.T) {
	testSpoolHonoursACancelledRunner(t, newOpsPGContext(t))
}

// A spool waiting for a Reduction's row lock ends when the runner is cancelled.
func TestSpoolWaitingForTheRowLockEndsOnCancelPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	red := legacyRow(t, ctx, models.ReductionStatusReady)
	blocker := ctx.db.Begin()
	require.NoError(t, blocker.Error)
	t.Cleanup(func() { _ = blocker.Rollback().Error })
	require.NoError(t, blocker.Exec(`SELECT id FROM resource_reductions WHERE id = ? FOR UPDATE`, red.ID).Error)

	stop, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ctx.spoolReductionMigrationsUntil(stop) }()
	require.Eventually(t, func() bool {
		var waiting int
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting).Error)
		return waiting > 0
	}, 20*time.Second, 50*time.Millisecond, "the spool never blocked on the held Reduction row")
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled spool stayed blocked behind the held row")
	}
	require.EqualValues(t, 0, planRows(t, ctx, "reduction_migrations", "reduction_id = ?", red.ID))
}

// The real Postgres failure: a lock_timeout (55P03) on a member's Resource row held
// by another transaction. The retry, after the blocker is gone, copies the chunk.
func TestCarryOverRetriesARealLockTimeoutPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r := applyReduction(t, ctx)
	w, l := makeResource(t, ctx.db, "w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "l", opsRes{Hash: "h"})
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "k", Tier: models.ReductionTierIdentical, State: models.ReductionClusterSkipped, Position: 1, WinnerID: w},
		opsMember(w, "h"), opsMember(l, "h"))
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)

	spare := makeResource(t, ctx.db, "spare", opsRes{Hash: "x"})
	blocker := ctx.db.Begin()
	require.NoError(t, blocker.Error)
	released := false
	release := func() {
		if !released {
			released = true
			_ = blocker.Rollback().Error
		}
	}
	t.Cleanup(release)
	require.NoError(t, blocker.Exec(`SELECT id FROM resources WHERE id = ? FOR UPDATE`, spare).Error)

	calls := 0
	carryOverChunkHook = func(tx *gorm.DB, phase string) error {
		if phase != "members" {
			return nil
		}
		calls++
		if calls > 1 {
			return nil
		}
		defer release()
		if err := tx.Exec(`SET LOCAL lock_timeout = '100ms'`).Error; err != nil {
			return err
		}
		return tx.Exec(`SELECT id FROM resources WHERE id = ? FOR UPDATE`, spare).Error
	}
	t.Cleanup(func() { carryOverChunkHook = nil })

	require.NoError(t, ctx.storeLandComputedPlan(r, jobID, gen, version, nil, models.ReductionCoverage{}, nil))
	require.Equal(t, 2, calls)
	require.EqualValues(t, 2, planRows(t, ctx, "reduction_members", "reduction_id = ? AND generation = ?", r.ID, gen))
}
