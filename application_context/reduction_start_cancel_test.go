package application_context

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/jobs"
	"mahresources/models"
)

// A cancel recorded after a start's last check and before its queue entry exists
// has no entry to stop. The start asks again once it holds its claim, so the run
// never lands a plan and the Job ends cancelled.
func TestACancelRecordedBetweenTheLastCheckAndTheClaimStopsTheCompute(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 1
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, otherRuntime := newSecondProcessJobContext(t, first, key)
	reduction := createReductionRowForTest(t, first, models.ReductionStatusFailed)

	// The deployment's one slot, held, so the run is admitted for later and started
	// by the adapter's own start rather than by the request.
	holder := holdTheDeploymentBudgetIn(t, first)
	_, err := first.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil)
	require.NoError(t, err)
	queued := jobOfKindForTest(t, first, JobKindReductionCompute)
	require.Equal(t, jobs.StateQueued, queued.State)

	reductionStartAfterClaimHook = func(execution jobs.Execution) {
		current, err := first.JobService().Get(first.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
		require.NoError(t, err)
		result, err := first.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
			JobID: execution.JobID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-in-the-window", ExpectedVersion: current.Version,
		})
		require.NoError(t, err)
		require.Equal(t, jobs.CommandStatusSucceeded, result.Status)
	}
	t.Cleanup(func() { reductionStartAfterClaimHook = nil })

	_, err = first.JobService().ReleaseClaim(first.jobDeps(), jobs.ReleaseRequest{
		ExecutionRef: jobs.ExecutionRef{JobID: holder.JobID, ExecutionToken: holder.ExecutionToken},
		To:           jobs.StateQueued,
		Reason:       "test released the budget",
	})
	require.NoError(t, err)
	otherRuntime.tick(context.Background())

	snap := waitForSnapshot(t, other, queued.ID, "the cancelled run to end", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	assert.Equal(t, jobs.StateCancelled, snap.State)

	row := loadReductionRow(t, first.db, reduction.ID)
	assert.NotEqual(t, models.ReductionStatusReady, row.Status, "a cancelled compute must not land its plan")
	assert.Nil(t, row.PlanGeneration)
	assert.NotEqual(t, models.ReductionStatusComputing, EffectiveReductionStatus(row), "the claim is released")
}

// A migration that has not published after the wait leaves the Job held with a
// reason, not failed: design D12 defers the compute, and Resume runs it once the
// plan is published.
func TestAComputeThatOutwaitsTheMigrationIsRefusedAsPendingNotFailed(t *testing.T) {
	ctx := newCutoverContext(t)
	r := legacyRow(t, ctx, models.ReductionStatusFailed)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}
	prevPoll, prevWait := reductionUpgradePoll, reductionUpgradeMaxWait
	reductionUpgradePoll, reductionUpgradeMaxWait = 5*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { reductionUpgradePoll, reductionUpgradeMaxWait = prevPoll, prevWait })

	_, err := adapter.awaitUpgrade(context.Background(), jobs.Execution{}, r.ID)
	var refusal *reductionStartRefusal
	require.True(t, errors.As(err, &refusal), "a timeout is a held Job (a start refusal), got %v", err)
	assert.Equal(t, "migration-pending", refusal.reason)
}
