package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
)

func blockedReasonOf(t *testing.T, ctx *MahresourcesContext, id string) string {
	t.Helper()
	snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, id)
	require.NoError(t, err)
	reasons, err := ctx.JobService().BlockedReasons(ctx.jobDeps(), []jobs.Snapshot{snap})
	require.NoError(t, err)
	return reasons[id]
}

func jobStateOf(t *testing.T, ctx *MahresourcesContext, id string) jobs.State {
	t.Helper()
	snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, id)
	require.NoError(t, err)
	return snap.State
}

// heldComputesFor holds one queued clustering run per fresh Reduction, each blocked
// with its reason, behind one held deployment budget so none starts.
func heldComputesFor(t *testing.T, ctx *MahresourcesContext, reasons ...string) ([]*models.ResourceReduction, []string) {
	t.Helper()
	ctx.Config.MaxJobConcurrency = 1
	holdTheDeploymentBudgetIn(t, ctx)
	var reductions []*models.ResourceReduction
	var ids []string
	for _, reason := range reasons {
		reduction := createReductionRowForTest(t, ctx, models.ReductionStatusFailed)
		_, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil)
		require.NoError(t, err)
		var id string
		require.NoError(t, ctx.db.Raw(`SELECT id FROM jobs WHERE kind = ? AND CAST(`+jobSummaryTextExpr(ctx.db, "reductionId")+` AS TEXT) = ?`,
			JobKindReductionCompute, fmt.Sprint(reduction.ID)).Scan(&id).Error)
		require.NotEmpty(t, id)
		require.NoError(t, ctx.blockQueueJob(id, "", reason))
		require.Equal(t, jobs.StateBlocked, jobStateOf(t, ctx, id))
		reductions = append(reductions, reduction)
		ids = append(ids, id)
	}
	return reductions, ids
}

// ---- 7: a start waiting for the migration or the indexes holds no slot ----------------

// A run whose Reduction is still a legacy plan is held at once (blocked, its
// execution released), not slept on inside the worker for up to half an hour.
func TestAMigrationPendingComputeIsHeldAtOnceNotWaitedOn(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	owner, _, _ := scopedReviewerForTest(t, ctx, "hold-owner")
	reduction := &models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusFailed, CreatedByUserId: &owner.ID}
	require.NoError(t, ctx.db.Create(reduction).Error)
	require.NoError(t, ctx.db.Model(reduction).Update("plan_schema", 0).Error)
	execution, jobID := claimedReductionExecution(t, ctx, reduction, owner.ID)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}

	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- adapter.Dispatch(run, execution) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the dispatch is sleeping in the worker for the migration")
	}
	require.Equal(t, jobs.StateBlocked, jobStateOf(t, ctx, jobID))
	require.Equal(t, reductionHoldMigrationPending, blockedReasonOf(t, ctx, jobID))
}

// The similarity-index wait is a hold too, with a reason and a resumer of its own.
func TestAnIndexWaitIsAHoldNotAWorkerWait(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.Resource{}, &models.ResourceSimilarity{}))
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}
	start := time.Now()
	err := adapter.awaitSimilarityIndexes(context.Background(), jobs.Execution{})
	var refusal *reductionStartRefusal
	require.True(t, errors.As(err, &refusal), "got %v", err)
	require.Equal(t, reductionHoldIndexesBuilding, refusal.reason)
	require.ErrorIs(t, err, ErrReductionIndexesBuilding)
	require.Less(t, time.Since(start), time.Second)
}

// ---- 8: publication reads only that Reduction's held runs, in chunks ----------------------

func TestPublicationReadsOnlyThatReductionsHeldComputes(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	reductions, held := heldComputesFor(t, ctx, reductionHoldMigrationPending, reductionHoldMigrationPending)

	var statements []string
	record := func(db *gorm.DB) { statements = append(statements, db.Statement.SQL.String()) }
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register("test:r6_held_row", record))
	t.Cleanup(func() { _ = ctx.db.Callback().Row().Remove("test:r6_held_row") })

	ctx.resumeBlockedReductionComputes(reductions[0].ID)

	var read []string
	for _, s := range statements {
		if strings.Contains(s, "FROM jobs WHERE") && strings.Contains(s, "kind = ?") {
			read = append(read, s)
		}
	}
	require.NotEmpty(t, read)
	for _, s := range read {
		require.Contains(t, s, "reductionId", "the database narrows to the Reduction: %s", s)
		require.Contains(t, strings.ToUpper(s), "LIMIT", "a bounded read: %s", s)
	}
	require.Equal(t, jobs.StateQueued, jobStateOf(t, ctx, held[0]))
	require.Equal(t, jobs.StateBlocked, jobStateOf(t, ctx, held[1]), "another Reduction's held run stays held")
}

// The index build's completion walks every held run in keyset chunks, and resumes
// only the ones held for the indexes.
func TestIndexCompletionResumesHeldComputesInChunks(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	prev := blockedComputeChunk
	blockedComputeChunk = 1
	t.Cleanup(func() { blockedComputeChunk = prev })

	_, held := heldComputesFor(t, ctx, reductionHoldIndexesBuilding, reductionHoldIndexesBuilding, reductionHoldMigrationPending, reductionHoldIndexesBuilding)

	ctx.ResumeIndexBlockedReductionComputes()

	for i, id := range held {
		want := jobs.StateQueued
		if i == 2 {
			want = jobs.StateBlocked
		}
		require.Equal(t, want, jobStateOf(t, ctx, id), "held run %d", i)
	}
}

// ---- 9: a publish between the last read and the hold is not missed ---------------------

func TestAPublishBetweenTheLastReadAndTheHoldResumesTheRun(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	owner, _, _ := scopedReviewerForTest(t, ctx, "race-owner")
	reduction := &models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusFailed, CreatedByUserId: &owner.ID}
	require.NoError(t, ctx.db.Create(reduction).Error)
	require.NoError(t, ctx.db.Model(reduction).Update("plan_schema", 0).Error)
	execution, jobID := claimedReductionExecution(t, ctx, reduction, owner.ID)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}

	reductionBeforeHoldHook = func(reason string) {
		require.Equal(t, reductionHoldMigrationPending, reason)
		// The migration publishes now, and its resumer finds no held run yet.
		require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", reduction.ID).Update("plan_schema", 1).Error)
		ctx.resumeBlockedReductionComputes(reduction.ID)
		require.Equal(t, jobs.StateRunning, jobStateOf(t, ctx, jobID), "nothing was held to resume")
	}
	t.Cleanup(func() { reductionBeforeHoldHook = nil })

	require.NoError(t, adapter.Dispatch(context.Background(), execution))
	require.Equal(t, jobs.StateQueued, jobStateOf(t, ctx, jobID), "the hold looked again and resumed the run")
}
