package application_context

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"

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
		// A runtime that is provably gone, because that is the state these fixtures
		// stand for: an execution whose process died with the export. Reconciliation
		// may only dispatch a replacement once the claim's own runtime identity
		// proves the original cannot still be running (see the download Kind's
		// TestAQueueBackedReconcileNeedsProofTheExecutorIsGone), so a claim taken in
		// the name of an unknown process makes "queued again" unreachable.
		Claimant: goneRuntimeIdentityForTest(),
		Lease:    lease,
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

// goneRuntimeIdentityForTest names a process that cannot exist any more: this host,
// a boot session it is not in, and a pid. Fixtures that stand for "the execution
// died" claim in this name, because an identity a reconciler cannot read has to
// leave the Job blocked rather than dispatching a replacement over work that might
// be live.
func goneRuntimeIdentityForTest() string {
	return plugin_system.CurrentRuntimeIdentity().Host + "/boot-that-ended-for-tests/4242"
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
	if gotPath := writeGroupExportArchiveForTest(t, ctx, accepted.ID, []uint{groupID}, nil, nil, nil); gotPath != archivePath {
		t.Fatalf("test archive path = %q, want %q", gotPath, archivePath)
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

func TestLargeGroupExportScopeManifestKeepsOutputReferenceBounded(t *testing.T) {
	ctx := newClaimableExportContext(t)
	groupID := createExportGroupForTest(t, ctx, "export-large-manifest")
	accepted, execution := acceptAndClaimExportForTest(t, ctx, "export-large-manifest", groupID, time.Minute)
	groupIDs := make([]uint, 2048)
	groupIDs[0] = groupID
	for i := 1; i < len(groupIDs); i++ {
		groupIDs[i] = uint(100000 + i)
	}
	path := writeGroupExportArchiveForTest(t, ctx, accepted.ID, groupIDs, nil, nil, nil)
	if err := ctx.publishQueueArtifact(execution, jobExportArtifactOutput, "Exported archive", path, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("publish large export artifact: %v", err)
	}
	outputs, err := ctx.jobOutputsFor(accepted.ID)
	if err != nil || len(outputs) != 1 {
		t.Fatalf("published outputs = %#v, err=%v", outputs, err)
	}
	if len(outputs[0].Reference) >= jobs.MaxOutputReferenceBytes {
		t.Fatalf("output reference is %d bytes, want below %d; source IDs belong in the archive manifest", len(outputs[0].Reference), jobs.MaxOutputReferenceBytes)
	}
	var reference queueArtifactReference
	if err := json.Unmarshal(outputs[0].Reference, &reference); err != nil || reference.ScopeManifestVersion != jobExportScopeManifestVersion {
		t.Fatalf("published scope proof reference = %+v, err=%v", reference, err)
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

// scopedExportFixture builds a tree one scoped principal is entitled to export
// only part of: their own root group, and a group outside it that the root is
// related to. Without the relation the scoped assertion would hold even if the
// traversal never ran.
//
// It returns the root and the outside group.
func scopedExportFixture(t *testing.T, ctx *MahresourcesContext) (uint, uint) {
	t.Helper()
	root := &models.Group{Name: "scoped-export-root"}
	if err := ctx.db.Create(root).Error; err != nil {
		t.Fatalf("create root group: %v", err)
	}
	outside := &models.Group{Name: "scoped-export-outside"}
	if err := ctx.db.Create(outside).Error; err != nil {
		t.Fatalf("create outside group: %v", err)
	}
	if err := ctx.db.Model(root).Association("RelatedGroups").Append(outside); err != nil {
		t.Fatalf("relate the outside group: %v", err)
	}
	return root.ID, outside.ID
}

// exportedGroupIDs reads one finished export's archive and returns the source ids
// of the groups it contains.
func exportedGroupIDs(t *testing.T, ctx *MahresourcesContext, jobID string) map[uint]bool {
	t.Helper()
	path := exportArchivePath(jobID, false)
	content, err := afero.ReadFile(ctx.GetDefaultFs(), path)
	if err != nil {
		t.Fatalf("read the export archive %s: %v", path, err)
	}
	var manifest archive.Manifest
	tr := tar.NewReader(bytes.NewReader(content))
	found := false
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read the archive %s: %v", path, err)
		}
		if header.Name != "manifest.json" {
			continue
		}
		payload, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read the manifest: %v", err)
		}
		if err := json.Unmarshal(payload, &manifest); err != nil {
			t.Fatalf("parse the manifest: %v", err)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("the archive %s has no manifest", path)
	}
	ids := make(map[uint]bool, len(manifest.Entries.Groups))
	for _, entry := range manifest.Entries.Groups {
		ids[entry.SourceID] = true
	}
	return ids
}

// TestAScopedExportsRepeatKeepsTheExportsSubtree is the export's confinement
// contract across a second execution.
//
// A group-limited principal's export is filtered by *their* subtree, and the
// filtering lives in the groupio dependencies the run reads — the context's db and
// its scope resolver — so an execution that builds its run function from the
// process singleton exports the related tree the request-scoped run deliberately
// left out. Repeat is where that shows: the successor is a new Job dispatched by
// the runtime, with no request behind it at all.
func TestAScopedExportsRepeatKeepsTheExportsSubtree(t *testing.T) {
	ctx := newJobHarnessContext(t, true)
	rootID, outsideID := scopedExportFixture(t, ctx)

	// The fixture's control: an unscoped export does follow the relation.
	unscoped := ctx.SubmitGroupExport(fullBFSRequest(rootID), "api")
	if unscoped.Err != nil {
		t.Fatalf("submit the control export: %v", unscoped.Err)
	}
	control := waitForSnapshot(t, ctx, unscoped.CanonicalJobID, "the control export to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if control.State != jobs.StateSucceeded {
		t.Fatalf("the control export ended %s (%+v)", control.State, control.Failure)
	}
	if !exportedGroupIDs(t, ctx, control.ID)[outsideID] {
		t.Fatalf("the unscoped control export did not follow the relation: the scoped assertions would prove nothing")
	}

	// The scoped principal's own export, submitted through the request-bound path.
	// The account has to exist: a dispatch resolves the Job's actor to the stored
	// user, and an actor nobody can read is this tree's deny-all identity.
	user := &models.User{
		Username: "scoped-exporter", Role: models.RoleUser,
		ScopeGroupId: &rootID, PasswordHash: "not-a-real-hash",
	}
	if err := ctx.db.Create(user).Error; err != nil {
		t.Fatalf("create the scoped user: %v", err)
	}
	scoped := ctx.WithPrincipal(&auth.Principal{
		UserID: user.ID, Role: models.RoleUser, ScopeGroupID: &rootID,
	})
	submitted := scoped.SubmitGroupExport(fullBFSRequest(rootID), "api")
	if submitted.Err != nil {
		t.Fatalf("submit the scoped export: %v", submitted.Err)
	}
	first := waitForSnapshot(t, ctx, submitted.CanonicalJobID, "the scoped export to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if first.State != jobs.StateSucceeded {
		t.Fatalf("the scoped export ended %s (%+v)", first.State, first.Failure)
	}
	if exportedGroupIDs(t, ctx, first.ID)[outsideID] {
		t.Fatalf("the request-bound export followed a relation outside the principal's subtree")
	}

	// Repeat it. The successor has no request behind it: the runtime claims it and
	// builds the executor from whatever the adapter binds at dispatch.
	result, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID:           first.ID,
		Key:             jobs.CommandRepeat,
		IdempotencyKey:  "repeat-scoped-export",
		ExpectedVersion: first.Version,
		Actor:           jobs.Access{UserID: user.ID},
	})
	if err != nil {
		t.Fatalf("repeat the scoped export: %v", err)
	}
	repeated := waitForSnapshot(t, ctx, result.SuccessorID, "the repeated export to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if repeated.State != jobs.StateSucceeded {
		t.Fatalf("the repeated export ended %s (%+v)", repeated.State, repeated.Failure)
	}
	if exportedGroupIDs(t, ctx, repeated.ID)[outsideID] {
		t.Fatalf("the repeated export followed a relation outside the principal's subtree")
	}
}

// gatedExportFs is the export harness's filesystem with one gate in it: creation of
// the tar block until the test releases it. It is how a worker is made to be *still
// running* while the assertions about its Job are made — the export's own staged
// file, rather than a sleep in the test, is what holds it.
type gatedExportFs struct {
	afero.Fs
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
	released    sync.Once
}

func newGatedExportFs(inner afero.Fs) *gatedExportFs {
	return &gatedExportFs{Fs: inner, entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedExportFs) Create(name string) (afero.File, error) {
	g.enteredOnce.Do(func() { close(g.entered) })
	<-g.release
	return g.Fs.Create(name)
}

// release lets every staged write through, once.
func (g *gatedExportFs) unblock() { g.released.Do(func() { close(g.release) }) }

// TestARefusedProgressWriteDoesNotEndAnExportThatIsStillRunning is the other half of
// owning an execution: a Job's outcome belongs to its executor, and nothing that
// happens *around* the executor may classify the work.
//
// The queue bridge mirrors the running entry's progress into the Job, and a mirror
// can be refused — a transient write failure, a locked database, a pool briefly
// exhausted. Reading that refusal as the executor's answer ended the Job as
// `dispatch-failed` while the queue's own worker was still producing the archive, so
// the Job Center offered a Retry of work in flight and the claim and the capacity
// that owned it went with the wrong outcome. A progress snapshot is telemetry:
// ownership lasts until the worker returns, which is what the terminal status below
// records.
//
// The runtime dispatched this Job, so the harm the finding names — `finishOwnedExecution`
// ending a running Job and freeing its claim — is what the test observes, not merely a
// refused dispatch call.
func TestARefusedProgressWriteDoesNotEndAnExportThatIsStillRunning(t *testing.T) {
	ctx := newClaimableExportContext(t)
	ctx.Config.MaxJobConcurrency = 1
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{Claimant: "export-dispatch-test", Interval: time.Hour})
	t.Cleanup(runtime.Stop)

	groupID := createExportGroupForTest(t, ctx, "gated-export")

	// The deployment's one slot, taken by a transfer that is held open by its server:
	// the export is therefore accepted durably and dispatched by the runtime rather
	// than by the request that accepted it.
	server, _, unblock := heldTransferServer(t)
	transfer := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/holding.bin"}, nil, "", "api")
	if len(transfer) != 1 || transfer[0].Err != nil || transfer[0].Job == nil {
		t.Fatalf("the holding transfer: %+v", transfer)
	}
	waitForSnapshot(t, ctx, transfer[0].CanonicalJobID, "the holding transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})

	// One refused write, injected: the failure under test is transient, so the
	// executor runs on and the next tick has to record where it got to.
	ctx.jobFaults = &jobDurabilityFaults{}
	ctx.jobFaults.failProgressWrite.Store(true)

	accepted := ctx.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if accepted.Err != nil {
		t.Fatalf("submit the export: %v", accepted.Err)
	}
	if accepted.CanonicalJobID == "" {
		t.Fatalf("the export created no durable job")
	}
	if _, found := ctx.queueEntryFor(accepted.CanonicalJobID); found {
		t.Fatalf("the export started an executor while the deployment's only slot was taken")
	}

	// The slot frees, and the runtime claims the export and dispatches it.
	unblock()
	waitForSnapshot(t, ctx, transfer[0].CanonicalJobID, "the holding transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	// The worker is gated on its own staged write, so "still running" is a fact about
	// the export rather than about a sleep in this test. The gate goes on after the
	// holding transfer is done with the filesystem, and before the export reads it.
	gate := newGatedExportFs(ctx.GetDefaultFs())
	t.Cleanup(gate.unblock)
	ctx.fs = gate
	waitFor(t, "the runtime to claim and dispatch the export", func() bool {
		runtime.tick(context.Background())
		select {
		case <-gate.entered:
			return true
		default:
			return false
		}
	})

	// The loop mirrors the entry's progress on its first tick, which is well inside
	// this window: what it does with the refusal is the whole question.
	time.Sleep(300 * time.Millisecond)
	snap := jobSnapshot(t, ctx.JobService(), ctx, accepted.CanonicalJobID)
	if snap.State != jobs.StateRunning {
		t.Fatalf("the export is %s while its worker is still producing the archive: a refused progress write was read as its executor's answer (%+v)",
			snap.State, snap.Failure)
	}
	if claimed := storedClaim(t, ctx, accepted.CanonicalJobID); claimed.State != models.JobClaimStateHeld {
		t.Fatalf("the export's claim is %s while its worker is still running", claimed.State)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the export's worker is running with %d capacity slots held, want the one that admitted it", held)
	}
	if ctx.jobFaults.failProgressWrite.Load() {
		t.Fatal("the injected write refusal was never consumed: the progress mirror never ran")
	}

	// The worker returns, and its own outcome is the one the Job keeps.
	gate.unblock()
	finished := waitForSnapshot(t, ctx, accepted.CanonicalJobID, "the gated export to end", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the export ended %s (%+v)", finished.State, finished.Failure)
	}
	outputs, err := ctx.GetJobOutputs(accepted.CanonicalJobID)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	if _, published := findJobOutput(outputs, jobExportArtifactOutput); !published {
		t.Fatalf("a succeeded export published no artifact: %+v", outputs)
	}
	if claimed := storedClaim(t, ctx, accepted.CanonicalJobID); claimed.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the export ended, want released", claimed.State)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the export ended", held)
	}
}
