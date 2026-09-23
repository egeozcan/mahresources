package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This file is the executor-ownership contract of the queue-backed Kinds: which
// process owns a Job whose executor is an in-memory queue entry, for how long, and
// what happens when that executor starts, runs and ends.
//
// The thing it is about is the window a submission used to leave open. A submission
// accepts the Job and starts the specialized executor — a queue entry — and until
// that Job is *owned* it is ordinary queued work of a registered Kind. Another
// process's dispatch loop claims queued work; finding no entry in its own queue, it
// starts a second executor for the same work: a second transfer of one URL, a second
// export of one tree, a second import applied twice. One claim, committed in the same
// transaction as the acceptance, is what closes it.
//
// Every property below is asserted through the seams production uses — the
// submission methods, the queue, the control plane's own reads — and the two-process
// tests are two application contexts over one database and one filesystem, which is
// the only shape in which a duplicate executor is observable at all: the queue is
// memory, and one process cannot see another's.

// newSecondProcessJobContext builds the *other* process of a two-process deployment:
// a second application context over the first one's database and filesystem, with its
// own queue and its own control plane. Only the rows and the stored bytes are shared.
//
// The dispatch loop it hands back is not started: a test drives its passes itself,
// because "the other process looked for work and found none of this" is a statement
// about one pass and not about a cadence.
func newSecondProcessJobContext(t *testing.T, first *MahresourcesContext, key jobs.ReplayKey) (*MahresourcesContext, *JobRuntime) {
	t.Helper()

	dialector, ok := first.db.Dialector.(*sqlite.Dialector)
	if !ok {
		t.Fatalf("the first context is not on SQLite: %T", first.db.Dialector)
	}
	db, err := gorm.Open(sqlite.Open(dialector.DSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open the second process's database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	config := *first.Config
	other := NewMahresourcesContext(first.GetDefaultFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &config)
	holdJobReplayKey(t, other, key)
	service := jobs.NewService()
	other.SetJobService(service)
	return other, NewJobRuntime(other, service, JobRuntimeConfig{Claimant: "second-process", Interval: time.Hour})
}

// sharedReplayKey builds the one replay key both processes of a two-process
// deployment hold: the second one has to be able to open what the first one sealed,
// or it could not dispatch a Job at all.
func sharedReplayKey(t *testing.T) jobs.ReplayKey {
	t.Helper()
	key, err := jobs.GenerateReplayKey()
	if err != nil {
		t.Fatalf("generate a replay key: %v", err)
	}
	return key
}

// holdJobReplayKey installs one keyring on one context.
func holdJobReplayKey(t *testing.T, ctx *MahresourcesContext, key jobs.ReplayKey) {
	t.Helper()
	ring, err := jobs.NewKeyring(key)
	if err != nil {
		t.Fatalf("build the keyring: %v", err)
	}
	ctx.SetJobReplayKeyring(ring)
}

// heldTransferServer serves one file per request and holds every response until the
// test releases them, which is how a transfer is made to be still running while the
// assertions are made. It counts requests, because the duplicate this file is about is
// a second request for one URL.
//
// Each path answers with its own bytes: two transfers of one body are two submissions
// of identical content, which the library refuses as a duplicate rather than storing
// twice — a fact about the resource layer that has nothing to do with what these tests
// are asking.
func heldTransferServer(t *testing.T) (*httptest.Server, *atomic.Int64, func()) {
	t.Helper()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-release
		_, _ = w.Write([]byte("held body for " + r.URL.Path))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(unblock)
	return server, &requests, unblock
}

// holdTheDeploymentBudgetIn occupies one context's whole deployment budget with a claim
// of the runtime test Kind: a deployment at its ceiling, seen from a submission.
func holdTheDeploymentBudgetIn(t *testing.T, ctx *MahresourcesContext) jobs.Execution {
	t.Helper()
	service := ctx.JobService()
	if _, registered := service.AdapterFor(runtimeTestKind, 1); !registered {
		if err := service.RegisterAdapter(newRuntimeTestAdapter()); err != nil {
			t.Fatalf("register the budget holder's kind: %v", err)
		}
	}
	accepted, err := service.Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "test",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept the budget holder: %v", err)
	}
	execution, claimed, err := service.Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, JobID: accepted.ID, Claimant: "budget-holder",
		Capacity: ctx.hostClaimCapacityBudget(),
	})
	if err != nil || !claimed {
		t.Fatalf("claim the budget holder: claimed=%v err=%v", claimed, err)
	}
	return execution
}

// TestAQueueBackedSubmissionsClaimKeepsEveryOtherProcessOut is the finding this
// correction exists for.
//
// The submitting process starts the executor and owns the Job from the moment it
// exists; a second process's dispatch loop, running the same Kinds over the same
// database, finds nothing of it to claim. Before the claim was committed with the
// acceptance, that loop claimed a Job whose entry lived in another process's memory
// and started a second transfer of the same URL.
func TestAQueueBackedSubmissionsClaimKeepsEveryOtherProcessOut(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 2
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, otherRuntime := newSecondProcessJobContext(t, first, key)

	server, requests, unblock := heldTransferServer(t)
	submissions := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/held.bin",
	}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	if submissions[0].Job == nil {
		t.Fatalf("the submission started no executor in the process that accepted it")
	}
	jobID := submissions[0].CanonicalJobID

	waitForSnapshot(t, first, jobID, "the transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})
	claim := storedClaim(t, first, jobID)
	if claim.State != models.JobClaimStateHeld || claim.ExecutionToken == "" {
		t.Fatalf("the transfer's job is %s under token %q, want it held by the submitter",
			claim.State, claim.ExecutionToken)
	}
	if held := storedCapacity(t, first, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the deployment budget holds %d slots for one running execution, want one", held)
	}

	// The other process looks for work of every Kind it can run.
	otherRuntime.tick(context.Background())
	// A dispatch that happened would reach the server in milliseconds: the whole path
	// is this process's own memory. This bound is what makes "nothing happened" a
	// measurement rather than an assumption.
	time.Sleep(300 * time.Millisecond)

	if entries := other.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("the other process started %d executors for work this process owns", len(entries))
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("the server was asked for the file %d times: two processes transferred one URL", got)
	}
	if now := storedClaim(t, first, jobID); now.ExecutionToken != claim.ExecutionToken {
		t.Fatalf("the claim moved from %q to %q while the work was running", claim.ExecutionToken, now.ExecutionToken)
	}

	// And the work ends where it began: the submitting process publishes the outcome
	// and hands the claim and its capacity back, because it is the only owner there is.
	unblock()
	succeeded := waitForSnapshot(t, first, jobID, "the transfer to succeed", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if succeeded.State != jobs.StateSucceeded {
		t.Fatalf("the transfer ended %s (%+v)", succeeded.State, succeeded.Failure)
	}
	if claim := storedClaim(t, first, jobID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the work ended, want released", claim.State)
	}
	if held := storedCapacity(t, first, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the work ended", held)
	}
}

// TestAnOwnedQueueSubmissionsClaimOutlivesItsLease is the second half of owning an
// execution: a claim that is not renewed is reconciled out from under the work that
// holds it.
//
// A transfer, an export or an import outlives a lease as a matter of course. The
// reconciler that takes an expired claim then decides a Job whose executor is still
// running — and an execution that publishes into a Job the reconciler has since
// blocked has its outcome refused outright, because `blocked -> succeeded` is not a
// transition. The heartbeat is what keeps the claim live for as long as its executor
// is.
func TestAnOwnedQueueSubmissionsClaimOutlivesItsLease(t *testing.T) {
	first := newJobHarnessContext(t, false)
	// A lease short enough to outlive three times over inside a test. Only tests set
	// this; a deployment's Kind declares two minutes.
	first.queueClaimLease = 400 * time.Millisecond

	server, _, unblock := heldTransferServer(t)
	submissions := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/outliving.bin",
	}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	waitForSnapshot(t, first, jobID, "the transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})
	before := storedClaim(t, first, jobID)

	time.Sleep(1200 * time.Millisecond)

	after := storedClaim(t, first, jobID)
	if after.State != models.JobClaimStateHeld {
		t.Fatalf("the claim is %s after three leases of running work, want it held", after.State)
	}
	if !after.LeaseExpiresAt.After(before.LeaseExpiresAt) {
		t.Fatalf("the claim's lease never moved: %s -> %s, so nothing renewed it",
			before.LeaseExpiresAt, after.LeaseExpiresAt)
	}
	if snap := jobSnapshot(t, first.JobService(), first, jobID); snap.State != jobs.StateRunning {
		t.Fatalf("the job is %s while its executor runs", snap.State)
	}

	// Nothing for a reconciler to decide: a claim that is being renewed is not expired,
	// so the Job is not asked about and no replacement is dispatched.
	report, err := first.JobService().ReconcileExpired(context.Background(), first.jobDeps(), "a-reconciler", 32)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, outcome := range report.Outcomes {
		if outcome.JobID == jobID {
			t.Fatalf("a live claim was reconciled as %q", outcome.Decision)
		}
	}

	unblock()
	finished := waitForSnapshot(t, first, jobID, "the transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the transfer ended %s (%+v)", finished.State, finished.Failure)
	}
	if claim := storedClaim(t, first, jobID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the work ended, want released", claim.State)
	}
}

// TestARefusedQueueSubmissionEndsTheJobAndHandsItsClaimBack is the start-failure half:
// a claim taken before the executor exists must not outlive an executor that never
// started.
//
// The queue refuses the submission, and the Job is ended — terminally, with the
// refusal's own class — in the same write that clears its token and frees the
// capacity it was admitted against. Leaving it running would be a Job nothing can
// claim and nothing reconciles, holding a slot for work that was never admitted.
func TestARefusedQueueSubmissionEndsTheJobAndHandsItsClaimBack(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 2

	// A header net/http owns. The queue refuses it at submission, which is the
	// refusal the durable acceptance already exists to record.
	refused := first.submitRemoteDownload(&query_models.ResourceFromRemoteCreator{
		URL:     "https://example.invalid/refused.bin",
		Headers: map[string]string{"Content-Length": "12"},
	}, nil, "", "api")
	if refused.Err == nil {
		t.Fatalf("a header net/http owns was accepted")
	}
	if refused.CanonicalJobID == "" {
		t.Fatalf("the refusal left no durable record")
	}

	failed := waitForSnapshot(t, first, refused.CanonicalJobID, "the refusal to be recorded", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if failed.State != jobs.StateFailed || failed.Failure == nil || failed.Failure.Code != "submission-refused" {
		t.Fatalf("the refused submission was left as %s (%+v)", failed.State, failed.Failure)
	}
	if claim := storedClaim(t, first, failed.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("a submission whose executor could not start left its claim %s", claim.State)
	}
	if held := storedCapacity(t, first, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("a submission whose executor could not start holds %d capacity slots", held)
	}
}

// TestACapacityBoundSubmissionIsQueuedAndRunsWhenTheSlotFrees is the deployment
// budget's case, and it is deliberately not a refusal.
//
// Capacity is a deployment-wide invariant: work that runs must have been admitted
// against it, so an execution never runs unbudgeted. But a budget is a statement
// about what may run *now*, not about what may be queued for later — a submission is
// accepted durably, answers the same legacy row it always has, and waits, unclaimed
// and with no executor anywhere, until a runtime with a free slot claims it. What it
// must never do is start an executor it has no capacity to admit, which is exactly
// the unbudgeted execution this test's second half looks for.
func TestACapacityBoundSubmissionIsQueuedAndRunsWhenTheSlotFrees(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 1
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, otherRuntime := newSecondProcessJobContext(t, first, key)

	server, requests, unblock := heldTransferServer(t)

	// The one slot the deployment has.
	holding := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/holding.bin",
	}, nil, "", "api")
	if len(holding) != 1 || holding[0].Err != nil || holding[0].Job == nil {
		t.Fatalf("the first submission: %+v", holding)
	}
	waitForSnapshot(t, first, holding[0].CanonicalJobID, "the first transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})

	waiting := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/waiting.bin",
	}, nil, "", "api")
	if len(waiting) != 1 {
		t.Fatalf("%d submissions, want one", len(waiting))
	}
	submission := waiting[0]
	if submission.Err != nil {
		t.Fatalf("a submission was refused for the deployment's budget: %v", submission.Err)
	}
	if submission.CanonicalJobID == "" {
		t.Fatalf("the accepted submission created no durable job")
	}
	if submission.Job != nil {
		t.Fatalf("the deployment's budget was full and this process started an executor anyway")
	}
	if submission.Row == nil {
		t.Fatalf("the accepted submission reported no row at all")
	}
	handle := submission.Row.ID
	if handle == "" {
		t.Fatalf("the accepted submission reported no stable id: %+v", submission.Row)
	}
	if submission.Row.CanonicalJobID != submission.CanonicalJobID {
		t.Fatalf("the reported row names job %q, want %q", submission.Row.CanonicalJobID, submission.CanonicalJobID)
	}

	// Accepted, visible and controllable under the id the client was handed.
	queued, err := first.GetJob(submission.CanonicalJobID)
	if err != nil {
		t.Fatalf("read the accepted job: %v", err)
	}
	if queued.State != jobs.StateQueued {
		t.Fatalf("a submission with no capacity to run it is %s, want queued", queued.State)
	}
	resolved, err := first.ResolveJobHandle(DownloadHandleNamespace, handle)
	if err != nil {
		t.Fatalf("the id the client was handed resolves to no job: %v", err)
	}
	if resolved.ID != queued.ID {
		t.Fatalf("the answered id resolves to %s, want %s", resolved.ID, queued.ID)
	}
	if _, ok := first.DownloadManager().GetJobByCanonicalJobID(queued.ID); ok {
		t.Fatalf("the accepted submission started a queue entry it had no capacity to run")
	}

	// No runtime in the deployment can take it either: every claim asks for the same
	// budget, and the budget is what is full.
	otherRuntime.tick(context.Background())
	time.Sleep(300 * time.Millisecond)
	if entries := other.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("work with no capacity to admit it started %d executors", len(entries))
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("the server was asked %d times while the deployment's one slot was taken", got)
	}

	// The slot frees. The next process with room claims the Job and starts its
	// executor from the durable input alone — the restart case, and the reason a Job
	// admitted for later is worth accepting: nothing about it lived in the process
	// that accepted it.
	unblock()
	waitForSnapshot(t, first, holding[0].CanonicalJobID, "the first transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	otherRuntime.tick(context.Background())

	waitFor(t, "the queued submission's executor to appear", func() bool {
		entry, ok := other.DownloadManager().GetJob(handle)
		return ok && entry.CanonicalJobID == queued.ID
	})
	finished := waitForSnapshot(t, other, queued.ID, "the queued transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the queued transfer ended %s (%+v)", finished.State, finished.Failure)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("the server was asked %d times for two URLs", got)
	}
}

// TestAnOwnedExportPublishesItsOwnOutcomeAndHandsItsClaimBack is the same contract for
// a Kind whose work is not a transfer, and with no dispatch loop anywhere in the
// test: the submission path is the whole executor.
//
// It is the assertion that the ownership wrapper really is the executor's owner rather
// than a heartbeat bolted onto one. With the Job owned from acceptance, a polling
// runtime never sees it, so if the submitting process did not publish the outcome the
// export would sit at `running` forever — with its claim held and its capacity
// occupied.
func TestAnOwnedExportPublishesItsOwnOutcomeAndHandsItsClaimBack(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	ctx.Config.MaxJobConcurrency = 2
	groupID := createExportGroupForTest(t, ctx, "owned-export")

	submission := ctx.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if submission.Err != nil {
		t.Fatalf("submit the export: %v", submission.Err)
	}
	if submission.CanonicalJobID == "" {
		t.Fatalf("the export created no durable job")
	}

	finished := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the export ended %s (%+v)", finished.State, finished.Failure)
	}
	outputs, err := ctx.GetJobOutputs(finished.ID)
	if err != nil {
		t.Fatalf("read the outputs: %v", err)
	}
	if _, published := findJobOutput(outputs, jobExportArtifactOutput); !published {
		t.Fatalf("a succeeded export published no archive to hand over: %+v", outputs)
	}
	if claim := storedClaim(t, ctx, finished.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the export ended, want released", claim.State)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the export ended", held)
	}
}

// TestAQueuedClusteringRunLeavesTheReductionFreeForTheRuntimeThatRunsIt is the
// reduction path's own half of the capacity contract, and the one place a submission
// has a *domain* claim of its own to reconcile with the durable one.
//
// A clustering run takes the Reduction row's compute claim before it dispatches, and
// that claim is what a second request is refused by. A run whose Job was admitted to
// wait for a slot has no worker at all, so the claim it took has to be handed back:
// the runtime that eventually dispatches the Job takes the row itself, and a row left
// `computing` would refuse it as busy and leave the Job blocked for a person.
func TestAQueuedClusteringRunLeavesTheReductionFreeForTheRuntimeThatRunsIt(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 1
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, otherRuntime := newSecondProcessJobContext(t, first, key)

	// The one slot the deployment has, held by a claim of the runtime test Kind —
	// which the second process can run, so a free slot really does mean it is free.
	holder := holdTheDeploymentBudgetIn(t, first)

	reduction := createReductionRowForTest(t, first, `{"clusters":[]}`, models.ReductionStatusFailed)
	running, err := first.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil)
	if err != nil {
		t.Fatalf("request the compute: %v", err)
	}
	if EffectiveReductionStatus(running) == models.ReductionStatusComputing {
		t.Fatalf("a clustering run with no capacity to start left its reduction computing")
	}

	// Accepted, and owned by nobody: no executor exists in either process.
	queued := jobOfKindForTest(t, first, JobKindReductionCompute)
	if queued.State != jobs.StateQueued {
		t.Fatalf("the clustering job is %s while the deployment has no room, want queued", queued.State)
	}
	if _, found := first.DownloadManager().GetJobByCanonicalJobID(queued.ID); found {
		t.Fatalf("the queued clustering run started an executor it had no capacity to admit")
	}

	otherRuntime.tick(context.Background())
	if entries := other.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("a full budget admitted %d executors", len(entries))
	}

	// The slot frees, and the process with room takes both claims — the Job's and the
	// row's — and runs the clustering.
	if _, err := first.JobService().ReleaseClaim(first.jobDeps(), jobs.ReleaseRequest{
		ExecutionRef: jobs.ExecutionRef{JobID: holder.JobID, ExecutionToken: holder.ExecutionToken},
		To:           jobs.StateQueued,
		Reason:       "test released the budget",
	}); err != nil {
		t.Fatalf("release the budget holder: %v", err)
	}
	otherRuntime.tick(context.Background())

	finished := waitForSnapshot(t, other, queued.ID, "the queued clustering run to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the queued clustering run ended %s (%+v)", finished.State, finished.Failure)
	}
	var stored models.ResourceReduction
	if err := other.db.First(&stored, reduction.ID).Error; err != nil {
		t.Fatalf("reload the Reduction: %v", err)
	}
	if stored.Status != models.ReductionStatusReady {
		t.Fatalf("the Reduction is %q after the queued run, want ready", stored.Status)
	}
}
