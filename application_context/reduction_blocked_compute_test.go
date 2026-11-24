package application_context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/jobs"
	"mahresources/models"
)

// blockedComputeOf queues a clustering run of a fresh Reduction behind a held
// deployment budget and blocks it as a migration wait that outlasted its limit does.
func blockedComputeOf(t *testing.T, ctx *MahresourcesContext) (*models.ResourceReduction, jobs.Snapshot) {
	t.Helper()
	ctx.Config.MaxJobConcurrency = 1
	reduction := createReductionRowForTest(t, ctx, models.ReductionStatusFailed)
	holdTheDeploymentBudgetIn(t, ctx)
	_, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil)
	require.NoError(t, err)
	queued := jobOfKindForTest(t, ctx, JobKindReductionCompute)
	require.Equal(t, jobs.StateQueued, queued.State)
	require.NoError(t, ctx.blockQueueJob(queued.ID, "", "migration-pending"))
	blocked, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, queued.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateBlocked, blocked.State)
	return reduction, blocked
}

// A compute held because the migration had not published stays held for good unless
// something returns it to the queue: the held Job offers Resume, and Resume works.
func TestABlockedReductionComputeOffersAndHonoursResume(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	_, blocked := blockedComputeOf(t, ctx)

	require.True(t, offersCommand(advertisedForTest(t, ctx, blocked.ID), jobs.CommandResume), "a held compute offers Resume")
	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: blocked.ID, Key: jobs.CommandResume, IdempotencyKey: "resume-1", ExpectedVersion: blocked.Version,
	})
	require.NoError(t, err)
	require.Equal(t, jobs.CommandStatusSucceeded, result.Status)
	after, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, blocked.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, after.State)
}

// The migration publishing a Reduction returns the computes held for it to the queue.
func TestMigrationPublicationRequeuesTheBlockedComputes(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	reduction, blocked := blockedComputeOf(t, ctx)

	ctx.resumeBlockedReductionComputes(reduction.ID + 1000)
	still, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, blocked.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateBlocked, still.State, "another Reduction's publication leaves it held")

	ctx.resumeBlockedReductionComputes(reduction.ID)
	after, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, blocked.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, after.State)
}
