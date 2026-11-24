package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/jobs"
	"mahresources/models"
)

// claimedReductionExecution accepts and claims one clustering Job whose actor is
// userID, the way the runtime would hand it to the adapter.
func claimedReductionExecution(t *testing.T, ctx *MahresourcesContext, reduction *models.ResourceReduction, userID uint) (jobs.Execution, string) {
	t.Helper()
	input, err := json.Marshal(reductionComputeJobInput{ReductionID: reduction.ID, Version: reduction.Version})
	require.NoError(t, err)
	actor := userID
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindReductionCompute, KindVersion: jobReductionKindVersion,
		State: jobs.StateQueued, Origin: "api", OwnerUserID: &actor, ActorUserID: &actor,
		Replay: jobs.ReplayInput{Input: input},
	})
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindReductionCompute, KindVersion: jobReductionKindVersion,
		JobID: accepted.ID, Claimant: "test-runtime",
	})
	require.NoError(t, err)
	require.True(t, claimed)
	return execution, accepted.ID
}

func fastReductionWaits(t *testing.T) {
	t.Helper()
	prev := reductionUpgradePoll
	reductionUpgradePoll = 5 * time.Millisecond
	t.Cleanup(func() { reductionUpgradePoll = prev })
}

// A run authorized before it waits for the migration is authorized again after:
// an account re-scoped during the wait must not start the clustering.
func TestAQueuedComputeIsReauthorizedAfterItsWait(t *testing.T) {
	fastReductionWaits(t)
	ctx := newJobHarnessContext(t, false)
	owner, mine, _ := scopedReviewerForTest(t, ctx, "waiting-owner")
	hashedResourceForTest(t, ctx, "a.txt", "h", mine)
	hashedResourceForTest(t, ctx, "b.txt", "h", mine)

	reduction := &models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusFailed, CreatedByUserId: &owner.ID}
	require.NoError(t, ctx.db.Create(reduction).Error)
	require.NoError(t, ctx.db.Model(reduction).Update("plan_schema", 0).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: reduction.ID, GroupID: mine}).Error)

	execution, jobID := claimedReductionExecution(t, ctx, reduction, owner.ID)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}
	done := make(chan error, 1)
	go func() { done <- adapter.Dispatch(context.Background(), execution) }()

	time.Sleep(50 * time.Millisecond)
	// The administrator confines the account elsewhere, then the migration lands.
	other := createExportGroupForTest(t, ctx, "elsewhere")
	require.NoError(t, ctx.db.Model(&models.User{}).Where("id = ?", owner.ID).Update("scope_group_id", other).Error)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", reduction.ID).Update("plan_schema", 1).Error)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("dispatch never returned")
	}
	snap := waitForSnapshot(t, ctx, jobID, "the job to be settled", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateBlocked || s.State.Terminal()
	})
	require.Equal(t, jobs.StateBlocked, snap.State, "the run computed under the scope the account had before the wait")
	var row models.ResourceReduction
	require.NoError(t, ctx.db.First(&row, reduction.ID).Error)
	require.NotEqual(t, models.ReductionStatusReady, row.Status)
}

// A cancel recorded while the run waits for the migration ends the Job; the wait
// does not outlive it.
func TestACancelDuringTheMigrationWaitStopsTheCompute(t *testing.T) {
	fastReductionWaits(t)
	ctx := newJobHarnessContext(t, false)
	owner, _, _ := scopedReviewerForTest(t, ctx, "cancel-owner")
	reduction := &models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusFailed, CreatedByUserId: &owner.ID}
	require.NoError(t, ctx.db.Create(reduction).Error)
	require.NoError(t, ctx.db.Model(reduction).Update("plan_schema", 0).Error)

	execution, jobID := claimedReductionExecution(t, ctx, reduction, owner.ID)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}
	done := make(chan error, 1)
	go func() { done <- adapter.Dispatch(context.Background(), execution) }()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
		Update("control_intent", jobs.ControlIntentCancel).Error)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the wait ignored the cancel intent")
	}
	snap := waitForSnapshot(t, ctx, jobID, "the job to end cancelled", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	require.Equal(t, jobs.StateCancelled, snap.State)
}
