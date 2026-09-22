package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mahresources/archive"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/spf13/afero"
)

// This file drives the export Kind's behaviour beyond the happy path: what
// reconciliation does with the evidence a crash left behind, that a cancellation is the
// executor's to confirm, that Retry and Repeat are new Jobs rather than a re-run, and
// that an artifact's expiry is not its Job's outcome.
//
// The tests that are about work *nobody* is running use a harness with no dispatch loop
// (see newJobHarnessContext): a claim taken by the test itself is what reconciliation and
// the command surface are about, and a running loop would adopt the Job a tick later and
// finish it first.

// newClaimableExportContext builds a harness whose Jobs stay exactly where a test puts
// them.
func newClaimableExportContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	return newJobHarnessContext(t, false)
}

// createExportGroupForTest stores one top-level group for an export to name.
func createExportGroupForTest(t *testing.T, ctx *MahresourcesContext, name string) uint {
	t.Helper()
	group := &models.Group{Name: name}
	if err := ctx.db.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return group.ID
}

// exportRequestForTest is the smallest request the export executor accepts: one group
// and its subtree.
func exportRequestForTest(groupID uint) *ExportRequest {
	return &ExportRequest{
		RootGroupIDs: []uint{groupID},
		Scope:        archive.ExportScope{Subtree: true},
	}
}

// acceptAndClaimExportForTest accepts one export Job and claims it, leaving the claim
// for a test to expire or to publish through.
func acceptAndClaimExportForTest(t *testing.T, ctx *MahresourcesContext, handle string, groupID uint, lease time.Duration) (jobs.Snapshot, jobs.Execution) {
	t.Helper()
	service := ctx.JobService()
	input, err := json.Marshal(exportJobInput{Request: *exportRequestForTest(groupID)})
	if err != nil {
		t.Fatalf("encode the export input: %v", err)
	}
	accepted, err := service.Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindGroupExport,
		KindVersion: jobExportKindVersion,
		State:       jobs.StateQueued,
		Origin:      "test",
		Title:       "Group export",
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: GroupExportHandleNamespace, Handle: handle}},
	})
	if err != nil {
		t.Fatalf("accept the export: %v", err)
	}
	execution, claimed, err := service.Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind:        JobKindGroupExport,
		KindVersion: jobExportKindVersion,
		Claimant:    "test-execution",
		Lease:       lease,
	})
	if err != nil {
		t.Fatalf("claim the export: %v", err)
	}
	if !claimed || execution.JobID != accepted.ID {
		t.Fatalf("the claim is %v for %s, want a claim of %s", claimed, execution.JobID, accepted.ID)
	}
	return accepted, execution
}

// finishForTest ends one claimed Job through its own report, which is how a real
// executor publishes an outcome.
func finishForTest(t *testing.T, execution jobs.Execution, outcome jobs.State) {
	t.Helper()
	request := jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version,
		Outcome:         outcome,
	}
	if outcome == jobs.StateFailed {
		request.Failure = &jobs.Failure{Code: "test-failure", Class: jobs.FailureClassInternal}
	}
	if _, err := execution.Finish(request); err != nil {
		t.Fatalf("finish the job as %s: %v", outcome, err)
	}
}

// currentVersionOf reads the version a command has to decide from.
func currentVersionOf(t *testing.T, ctx *MahresourcesContext, jobID string) uint64 {
	t.Helper()
	snap, err := ctx.GetJob(jobID)
	if err != nil {
		t.Fatalf("read %s: %v", jobID, err)
	}
	return snap.Version
}

// reconcileOnce runs one reconciliation pass and returns the decision applied to one
// Job, or an empty string when the pass did not reach it.
func reconcileOnce(t *testing.T, ctx *MahresourcesContext, jobID string) jobs.ReconcileDecision {
	t.Helper()
	report, err := ctx.JobService().ReconcileExpired(context.Background(), ctx.jobDeps(), "reconciler", 32)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.JobID == jobID {
			return outcome.Decision
		}
	}
	return ""
}

// TestACrashedExportIsSettledFromItsArchiveRatherThanRerun is §3's reconciliation
// contract for a staged artifact: an execution that finished the tar and died before
// recording it leaves a complete archive — the executor renames it into place precisely so
// a file there is a whole one — and the reconciling process publishes it and succeeds the
// Job. Running the export again would be the blind rerun the rule forbids, and it would
// replace a finished archive with a second one.
func TestACrashedExportIsSettledFromItsArchiveRatherThanRerun(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "crashed-export")
	accepted, _ := acceptAndClaimExportForTest(t, ctx, "export-crash-1", groupID, 20*time.Millisecond)

	// The crash: the export streamed its archive to the published path and died before
	// publishing the output. No queue entry exists — the process is gone.
	archivePath := exportArchivePath(accepted.ID, false)
	if err := ctx.GetDefaultFs().MkdirAll("_exports", 0755); err != nil {
		t.Fatalf("mkdir _exports: %v", err)
	}
	archive := []byte("a finished archive")
	if err := afero.WriteFile(ctx.GetDefaultFs(), archivePath, archive, 0644); err != nil {
		t.Fatalf("stage the archive: %v", err)
	}

	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcileSucceed {
		t.Fatalf("the reconciler decided %q, want %q", decision, jobs.ReconcileSucceed)
	}

	snap := waitForSnapshot(t, ctx, accepted.ID, "the reconciled export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the reconciled export ended %s (%+v)", snap.State, snap.Failure)
	}
	outputs, err := ctx.GetJobOutputs(accepted.ID)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	if _, published := findJobOutput(outputs, jobExportArtifactOutput); !published {
		t.Fatalf("the reconciled export published no artifact: %+v", outputs)
	}
	// The bytes are untouched: the archive was published, not produced again.
	got, err := afero.ReadFile(ctx.GetDefaultFs(), archivePath)
	if err != nil {
		t.Fatalf("read the archive back: %v", err)
	}
	if string(got) != string(archive) {
		t.Fatalf("the archive was rewritten: %q", got)
	}
}

// TestACrashedExportWithNothingStagedIsQueuedAgain is the other half of the same rule:
// with no archive on disk nothing was produced, so running the export again is the only
// way it can succeed and nothing is overwritten by doing so.
func TestACrashedExportWithNothingStagedIsQueuedAgain(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "crashed-export-empty")
	accepted, _ := acceptAndClaimExportForTest(t, ctx, "export-crash-2", groupID, 20*time.Millisecond)

	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcileQueue {
		t.Fatalf("the reconciler decided %q, want %q", decision, jobs.ReconcileQueue)
	}
}

// TestAnExportWhosePublishedArchiveIsGoneIsFailedNotSucceeded covers the third state a
// crashed export can be in: the output says the archive is there and it is not. Reporting
// that as a success would hand a client a reference to nothing — which is why the
// executor publishes only after stat-ing the file — so reconciliation ends the Job as
// failed rather than claiming otherwise.
func TestAnExportWhosePublishedArchiveIsGoneIsFailedNotSucceeded(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "export-gone")
	accepted, execution := acceptAndClaimExportForTest(t, ctx, "export-crash-3", groupID, 20*time.Millisecond)

	// The output is published with a reference to where the archive was, and the archive
	// is not there. It is written through the same call the executor makes, with the file
	// staged only long enough to publish: what is under test is the reconciliation, not
	// the publication.
	archivePath := exportArchivePath(accepted.ID, false)
	if err := ctx.GetDefaultFs().MkdirAll("_exports", 0755); err != nil {
		t.Fatalf("mkdir _exports: %v", err)
	}
	if err := afero.WriteFile(ctx.GetDefaultFs(), archivePath, []byte("gone soon"), 0644); err != nil {
		t.Fatalf("stage the archive: %v", err)
	}
	if err := ctx.publishQueueArtifact(execution, jobExportArtifactOutput, "Exported archive",
		archivePath, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("publish the artifact: %v", err)
	}
	if err := ctx.GetDefaultFs().Remove(archivePath); err != nil {
		t.Fatalf("remove the archive: %v", err)
	}

	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcileFail {
		t.Fatalf("the reconciler decided %q, want %q", decision, jobs.ReconcileFail)
	}
	snap := waitForSnapshot(t, ctx, accepted.ID, "the export to be ended", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateFailed {
		t.Fatalf("an export whose archive vanished ended %s, want failed", snap.State)
	}
}

// TestAnExportCancelAsksTheExecutorRatherThanEndingTheJob pins what "cooperative" means
// here: the command reaches the queue entry and stops *it*, and the Job keeps running
// until the executor publishes that it stopped. §4 makes the completed state the
// executor's confirmation — a cancellation that won lifecycle ownership is not the same
// fact as work that is no longer running.
func TestAnExportCancelAsksTheExecutorRatherThanEndingTheJob(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	groupID := createExportGroupForTest(t, ctx, "cancel-me")
	accepted, execution := acceptAndClaimExportForTest(t, ctx, "export-cancel-1", groupID, time.Minute)

	// This Job's own queue entry, submitted the way a dispatch submits one: the adapter's
	// start path is the only other writer of an entry, and the cancel has to reach
	// whichever one is there.
	entry, err := ctx.submitQueueJob(
		download_queue.JobOptions{Source: download_queue.JobSourceGroupExport, InitialPhase: "queued"},
		"export-cancel-1",
		jobs.ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		func(jobCtx context.Context, _ *download_queue.DownloadJob, _ download_queue.ProgressSink) error {
			<-jobCtx.Done()
			return jobCtx.Err()
		},
	)
	if err != nil {
		t.Fatalf("submit the export's queue entry: %v", err)
	}

	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: accepted.ID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-1",
		ExpectedVersion: currentVersionOf(t, ctx, accepted.ID),
	})
	if err != nil {
		t.Fatalf("cancel the export: %v", err)
	}
	if result.Code != jobs.CommandCodeRequested && result.Code != jobs.CommandCodeApplied {
		t.Fatalf("the cancellation answered %q (%s)", result.Code, result.Message)
	}

	// The executor was actually asked to stop...
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !queueJobTerminal(entry.GetStatus()) {
		time.Sleep(10 * time.Millisecond)
	}
	if entry.GetStatus() != download_queue.JobStatusCancelled {
		t.Fatalf("the queue entry is %s, want cancelled", entry.GetStatus())
	}
	// ...and the Job is still the executor's to end: the command did not declare it
	// cancelled on the executor's behalf.
	running, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatalf("re-read the export: %v", err)
	}
	if running.State.Terminal() && running.State != jobs.StateCancelled {
		t.Fatalf("the cancellation ended the Job as %s without the executor", running.State)
	}

	// The executor's own publish is what ends it, and it ends cancelled.
	adapter, registered := ctx.JobService().AdapterFor(JobKindGroupExport, jobExportKindVersion)
	if !registered {
		t.Fatalf("the export kind is not registered")
	}
	if err := adapter.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("dispatch the cancelled execution: %v", err)
	}
	snap := waitForSnapshot(t, ctx, accepted.ID, "the cancelled export to end", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateCancelled {
		t.Fatalf("the cancelled export ended %s, want cancelled", snap.State)
	}
}

// TestAnExportRetryIsANewJobAndMovesTheHandle pins ADR 0007 at the command seam: a Retry
// creates a new Job linked to the unsuccessful one, moves the legacy handle onto it, and
// leaves the ancestor's own outcome exactly as it was. The legacy id therefore keeps
// naming one current execution while every attempt keeps its own identity.
func TestAnExportRetryIsANewJobAndMovesTheHandle(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "retry-source")
	accepted, execution := acceptAndClaimExportForTest(t, ctx, "export-retry-1", groupID, time.Minute)
	finishForTest(t, execution, jobs.StateFailed)

	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: accepted.ID, Key: jobs.CommandRetry, IdempotencyKey: "retry-1",
		ExpectedVersion: currentVersionOf(t, ctx, accepted.ID),
	})
	if err != nil {
		t.Fatalf("retry the export: %v", err)
	}
	if result.SuccessorID == "" || result.SuccessorID == accepted.ID {
		t.Fatalf("the retry produced successor %q", result.SuccessorID)
	}

	resolved, err := ctx.ResolveJobHandle(GroupExportHandleNamespace, "export-retry-1")
	if err != nil {
		t.Fatalf("resolve the export handle: %v", err)
	}
	if resolved.ID != result.SuccessorID {
		t.Fatalf("the handle names %s, want the retry successor %s", resolved.ID, result.SuccessorID)
	}
	ancestor, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatalf("re-read the ancestor: %v", err)
	}
	if ancestor.State != jobs.StateFailed {
		t.Fatalf("the ancestor's own outcome changed to %s", ancestor.State)
	}
}

// TestAnExportRepeatIsANewJobThatLeavesTheHandleAlone pins the other half of the lineage
// contract: a Repeat of successful work is an independent execution that branches, so it
// creates a new Job and does *not* take the handle with it — a handle projects the linear
// Retry chain, and a repeat is off that chain.
func TestAnExportRepeatIsANewJobThatLeavesTheHandleAlone(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "repeat-source")
	accepted, execution := acceptAndClaimExportForTest(t, ctx, "export-repeat-1", groupID, time.Minute)
	finishForTest(t, execution, jobs.StateSucceeded)

	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: accepted.ID, Key: jobs.CommandRepeat, IdempotencyKey: "repeat-1",
		ExpectedVersion: currentVersionOf(t, ctx, accepted.ID),
	})
	if err != nil {
		t.Fatalf("repeat the export: %v", err)
	}
	if result.SuccessorID == "" || result.SuccessorID == accepted.ID {
		t.Fatalf("the repeat produced successor %q", result.SuccessorID)
	}

	resolved, err := ctx.ResolveJobHandle(GroupExportHandleNamespace, "export-repeat-1")
	if err != nil {
		t.Fatalf("resolve the export handle: %v", err)
	}
	if resolved.ID != accepted.ID {
		t.Fatalf("a repeat moved the handle to %s; a repeat branches and moves nothing", resolved.ID)
	}
	original, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatalf("re-read the original: %v", err)
	}
	if original.State != jobs.StateSucceeded {
		t.Fatalf("the repeated export's own outcome changed to %s", original.State)
	}
}

// TestAnExportJobsArtifactExpiryDoesNotChangeItsOutcome is §7's independence of an
// outcome from an artifact's retention: the artifact's own deadline takes the bytes and
// marks the output unavailable, and the Job stays exactly as it finished.
func TestAnExportJobsArtifactExpiryDoesNotChangeItsOutcome(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	groupID := createExportGroupForTest(t, ctx, "expiring-export")
	submission := ctx.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if submission.Err != nil {
		t.Fatalf("submit the export: %v", submission.Err)
	}
	snap := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the export ended %s (%+v)", snap.State, snap.Failure)
	}

	// The deadline arrives. A sweep is what a deployment runs on its cleanup ticker; this
	// test moves the artifact's own deadline rather than waiting out its retention, which
	// is the same fact seen from the sweep's side.
	if err := ctx.db.Exec("UPDATE job_outputs SET expires_at = ? WHERE job_id = ?",
		time.Now().Add(-time.Minute).UTC(), snap.ID).Error; err != nil {
		t.Fatalf("age the artifact: %v", err)
	}
	if _, err := ctx.SweepJobHistory(jobs.SweepCursor{}, 64); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	outputs, err := ctx.GetJobOutputs(snap.ID)
	if err != nil {
		t.Fatalf("read the outputs: %v", err)
	}
	artifact, published := findJobOutput(outputs, jobExportArtifactOutput)
	if !published {
		t.Fatalf("the artifact output disappeared: %+v", outputs)
	}
	if artifact.Availability == jobs.OutputAvailable {
		t.Fatalf("the artifact is still available after its deadline: %+v", artifact)
	}
	after, err := ctx.GetJob(snap.ID)
	if err != nil {
		t.Fatalf("re-read the export: %v", err)
	}
	if after.State != jobs.StateSucceeded {
		t.Fatalf("the artifact's expiry changed the outcome to %s", after.State)
	}
}
