package application_context

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
)

// This file drives the application-owned runtime loop: the piece that decides
// what this process should be running right now. It is tested through the same
// seam main.go uses — register an adapter, tick, stop — because the properties
// worth pinning are about what the loop does to durable state, not about its
// internals.

// runtimeTestKind is the Kind these tests register.
const runtimeTestKind = "runtime-test-work"

// runtimeTestAdapter is a Kind adapter whose dispatch and reconciliation the test
// supplies, recording every execution and request it was given.
type runtimeTestAdapter struct {
	def       jobs.Definition
	dispatch  func(context.Context, jobs.Execution) error
	reconcile func(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error)

	mu         sync.Mutex
	executions []jobs.Execution
	requests   []jobs.ReconcileRequest
}

func newRuntimeTestAdapter() *runtimeTestAdapter {
	return &runtimeTestAdapter{def: jobs.Definition{
		Kind: runtimeTestKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	}}
}

func (a *runtimeTestAdapter) Definition() jobs.Definition { return a.def }

func (a *runtimeTestAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	a.mu.Lock()
	a.executions = append(a.executions, execution)
	a.mu.Unlock()
	if a.dispatch != nil {
		return a.dispatch(ctx, execution)
	}
	return nil
}

func (a *runtimeTestAdapter) Reconcile(ctx context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	a.mu.Lock()
	a.requests = append(a.requests, request)
	a.mu.Unlock()
	if a.reconcile != nil {
		return a.reconcile(ctx, request)
	}
	return jobs.ReconcileRemainRunning, nil
}

func (a *runtimeTestAdapter) Commands(context.Context, jobs.CommandContext) ([]jobs.Command, error) {
	return nil, nil
}

func (a *runtimeTestAdapter) ExecuteCommand(context.Context, jobs.CommandExecution) (jobs.CommandOutcome, error) {
	return jobs.CommandOutcome{}, nil
}

func (a *runtimeTestAdapter) dispatched() []jobs.Execution {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]jobs.Execution(nil), a.executions...)
}

func (a *runtimeTestAdapter) reconciled() []jobs.ReconcileRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]jobs.ReconcileRequest(nil), a.requests...)
}

// newJobRuntimeContext builds a context whose database holds the durable job
// core and whose process holds a replay keyring — the minimum a runtime needs to
// claim and dispatch work.
func newJobRuntimeContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "job-runtime.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{},
		&models.JobClaim{}, &models.JobCapacityLease{},
	); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}

	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("build replay keyring: %v", err)
	}
	ctx.SetJobReplayKeyring(ring)
	return ctx
}

// acceptRuntimeJob accepts one queued Job of the runtime test Kind.
func acceptRuntimeJob(t *testing.T, svc *jobs.Service, app *MahresourcesContext) jobs.Snapshot {
	t.Helper()
	snap, err := svc.Accept(app.jobDeps(), jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
	})
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}
	return snap
}

// workerFinishes is the ordinary adapter behavior: run the work, then report the
// outcome through the execution's own report.
func workerFinishes(work func(context.Context, jobs.Execution) error) func(context.Context, jobs.Execution) error {
	return func(ctx context.Context, execution jobs.Execution) error {
		if work != nil {
			if err := work(ctx, execution); err != nil {
				return err
			}
		}
		_, err := execution.Finish(jobs.FinishRequest{
			ExpectedVersion: execution.Version, Outcome: jobs.StateSucceeded,
		})
		return err
	}
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func jobSnapshot(t *testing.T, svc *jobs.Service, app *MahresourcesContext, jobID string) jobs.Snapshot {
	t.Helper()
	snap, err := svc.Get(app.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		t.Fatalf("Get %s: %v", jobID, err)
	}
	return snap
}

func storedJob(t *testing.T, app *MahresourcesContext, jobID string) models.Job {
	t.Helper()
	var job models.Job
	if err := app.db.Where("id = ?", jobID).First(&job).Error; err != nil {
		t.Fatalf("load job %s: %v", jobID, err)
	}
	return job
}

func storedClaim(t *testing.T, app *MahresourcesContext, jobID string) models.JobClaim {
	t.Helper()
	var claim models.JobClaim
	if err := app.db.Where("job_id = ?", jobID).First(&claim).Error; err != nil {
		t.Fatalf("load claim for %s: %v", jobID, err)
	}
	return claim
}

func storedCapacity(t *testing.T, app *MahresourcesContext, group string) int64 {
	t.Helper()
	var count int64
	if err := app.db.Model(&models.JobCapacityLease{}).Where("capacity_group = ?", group).Count(&count).Error; err != nil {
		t.Fatalf("count capacity in %s: %v", group, err)
	}
	return count
}

// TestJobRuntimeClaimsAndDispatchesWhatTheAdapterReports is the loop's ordinary
// path: one pass finds the queued Job, claims it under this runtime's identity,
// hands it to the Kind's adapter, and the outcome the adapter reports is the one
// the Job keeps — with the claim and the capacity released by it.
func TestJobRuntimeClaimsAndDispatchesWhatTheAdapterReports(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	adapter := newRuntimeTestAdapter()
	adapter.dispatch = workerFinishes(nil)
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	accepted := acceptRuntimeJob(t, svc, app)
	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{
		Claimant: "runtime-test", Interval: time.Hour, GlobalCapacity: 2,
	})
	runtime.tick(context.Background())

	waitFor(t, "the Job to be dispatched and finished", func() bool {
		return jobSnapshot(t, svc, app, accepted.ID).State == jobs.StateSucceeded
	})

	dispatched := adapter.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("dispatched %d executions, want one", len(dispatched))
	}
	if dispatched[0].Claimant != "runtime-test" || dispatched[0].ExecutionToken == "" {
		t.Fatalf("execution = %+v, want this runtime's claimant and a token", dispatched[0])
	}
	claim := storedClaim(t, app, accepted.ID)
	if claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released once the Job finished", claim.State)
	}
	if held := storedCapacity(t, app, jobs.CapacityGroupGlobal) + storedCapacity(t, app, runtimeTestKind); held != 0 {
		t.Fatalf("capacity rows = %d, want none after the Job finished", held)
	}
}

// TestJobRuntimeBoundsDispatchByTheDeploymentCapacity proves the budget is what
// limits the runtime's own goroutines: one pass claims no more than the deployed
// budget allows, and the next pass picks up the rest.
func TestJobRuntimeBoundsDispatchByTheDeploymentCapacity(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	release := make(chan struct{})
	adapter := newRuntimeTestAdapter()
	adapter.dispatch = workerFinishes(func(context.Context, jobs.Execution) error {
		<-release
		return nil
	})
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	first := acceptRuntimeJob(t, svc, app)
	second := acceptRuntimeJob(t, svc, app)

	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{
		Claimant: "runtime-test", Interval: time.Hour, GlobalCapacity: 1,
	})
	runtime.tick(context.Background())

	waitFor(t, "the first Job to be dispatched", func() bool {
		return len(adapter.dispatched()) == 1
	})
	if state := jobSnapshot(t, svc, app, second.ID).State; state != jobs.StateQueued {
		t.Fatalf("the second Job is %s while the deployment budget admits one execution", state)
	}
	if held := storedCapacity(t, app, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("global capacity rows = %d, want the one running execution", held)
	}

	close(release)
	waitFor(t, "the first Job to finish", func() bool {
		return jobSnapshot(t, svc, app, first.ID).State == jobs.StateSucceeded
	})
	runtime.tick(context.Background())
	waitFor(t, "the second Job to finish", func() bool {
		return jobSnapshot(t, svc, app, second.ID).State == jobs.StateSucceeded
	})
}

// TestJobRuntimeReconcilesAnExpiredClaimBeforeItDispatches is the pass order:
// what nobody owns is resolved first, because the capacity it holds is capacity
// the dispatch behind it may need.
func TestJobRuntimeReconcilesAnExpiredClaimBeforeItDispatches(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	adapter := newRuntimeTestAdapter()
	adapter.reconcile = func(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
		return jobs.ReconcileRemainRunning, nil
	}
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	accepted := acceptRuntimeJob(t, svc, app)
	claimed, ok, err := svc.Claim(context.Background(), app.jobDeps(), jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, Claimant: "runtime-a",
	})
	if err != nil || !ok {
		t.Fatalf("Claim: ok=%v err=%v", ok, err)
	}
	if err := app.db.Model(&models.JobClaim{}).Where("job_id = ?", accepted.ID).
		Update("lease_expires_at", time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatalf("expire claim: %v", err)
	}

	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{Claimant: "runtime-b", Interval: time.Hour})
	runtime.tick(context.Background())

	requests := adapter.reconciled()
	if len(requests) != 1 {
		t.Fatalf("the adapter was asked %d times, want once", len(requests))
	}
	if requests[0].Snapshot.ID != accepted.ID || requests[0].Claimant != "runtime-a" {
		t.Fatalf("reconcile request = %+v, want the expired claim's job and claimant", requests[0])
	}
	if len(adapter.dispatched()) != 0 {
		t.Fatalf("the runtime dispatched %d executions during reconciliation", len(adapter.dispatched()))
	}

	// remain-running keeps everything: the Job, its token and its capacity.
	stored := storedJob(t, app, accepted.ID)
	if stored.State != string(jobs.StateRunning) || stored.ExecutionToken != claimed.ExecutionToken {
		t.Fatalf("job = %s token %q, want running under the original token", stored.State, stored.ExecutionToken)
	}
	claim := storedClaim(t, app, accepted.ID)
	if claim.State != models.JobClaimStateHeld || !claim.LeaseExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("claim = %+v, want held with a lease in the future", claim)
	}
}

// TestJobRuntimeDispatchesWhatAReconciliationResumes covers the one decision the
// caller has to act on: a resume keeps the Job running under a fresh token, and
// only the runtime can run it.
func TestJobRuntimeDispatchesWhatAReconciliationResumes(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	adapter := newRuntimeTestAdapter()
	adapter.reconcile = func(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
		return jobs.ReconcileResume, nil
	}
	adapter.dispatch = workerFinishes(nil)
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	accepted := acceptRuntimeJob(t, svc, app)
	expired, ok, err := svc.Claim(context.Background(), app.jobDeps(), jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, Claimant: "runtime-a",
	})
	if err != nil || !ok {
		t.Fatalf("Claim: ok=%v err=%v", ok, err)
	}
	if err := app.db.Model(&models.JobClaim{}).Where("job_id = ?", accepted.ID).
		Update("lease_expires_at", time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatalf("expire claim: %v", err)
	}

	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{Claimant: "runtime-b", Interval: time.Hour})
	runtime.tick(context.Background())

	waitFor(t, "the resumed execution to finish", func() bool {
		return jobSnapshot(t, svc, app, accepted.ID).State == jobs.StateSucceeded
	})
	dispatched := adapter.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("dispatched %d executions, want the resumed one", len(dispatched))
	}
	if dispatched[0].ExecutionToken == expired.ExecutionToken {
		t.Fatal("the resumed execution kept the expired execution's token")
	}
	if dispatched[0].Claimant != "runtime-b" {
		t.Fatalf("resumed claimant = %q, want the reconciling runtime", dispatched[0].Claimant)
	}
}

// TestJobRuntimeLeavesUnresolvedWorkDurableAcrossShutdown is §3's shutdown
// contract, end to end: stopping the runtime stops new claims and asks the
// running execution to quiesce, and work that was still unresolved stays exactly
// where it is — running, claimed and occupying its capacity — for the next
// process to reconcile rather than for this one to guess about.
func TestJobRuntimeLeavesUnresolvedWorkDurableAcrossShutdown(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()

	entered := make(chan jobs.Execution, 1)
	stopped := newRuntimeTestAdapter()
	stopped.dispatch = func(ctx context.Context, execution jobs.Execution) error {
		select {
		case entered <- execution:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	if err := svc.RegisterAdapter(stopped); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	first := acceptRuntimeJob(t, svc, app)
	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{
		Claimant: "runtime-a", Interval: 5 * time.Millisecond, GlobalCapacity: 1,
		QuiesceTimeout: 2 * time.Second,
	})
	runtime.Start()

	var execution jobs.Execution
	select {
	case execution = <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the runtime never dispatched the queued job")
	}
	runtime.Stop()

	// The work is unresolved and durable: the Job is still running under the
	// claim the stopped runtime held, nothing wrote a terminal state on the way
	// out, and the capacity it occupies is still occupied.
	stored := storedJob(t, app, first.ID)
	if stored.State != string(jobs.StateRunning) {
		t.Fatalf("job state after shutdown = %s, want running and unresolved", stored.State)
	}
	if stored.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("job token after shutdown = %q, want the stopped runtime's %q", stored.ExecutionToken, execution.ExecutionToken)
	}
	claim := storedClaim(t, app, first.ID)
	if claim.State != models.JobClaimStateHeld {
		t.Fatalf("claim state after shutdown = %s, want held for the next process", claim.State)
	}
	if held := storedCapacity(t, app, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("global capacity after shutdown = %d, want the unresolved execution's slot kept", held)
	}

	// And the loop is gone: nothing claims new work after Stop.
	second := acceptRuntimeJob(t, svc, app)
	time.Sleep(100 * time.Millisecond)
	if state := jobSnapshot(t, svc, app, second.ID).State; state != jobs.StateQueued {
		t.Fatalf("a Job accepted after shutdown is %s, want queued", state)
	}
	if len(stopped.dispatched()) != 1 {
		t.Fatalf("the stopped runtime dispatched %d executions, want only the one it had", len(stopped.dispatched()))
	}

	// A later process picks the work up through reconciliation, which is the
	// point of leaving it unresolved rather than inventing an outcome for it.
	//
	// Recovery waits for the lease to run out, and that wait is deliberate: a
	// shutdown proves nothing about external work, so the next process may not
	// take a claim over while it is still trusted. Aging the lease is the test
	// standing in for the clock.
	if err := app.db.Model(&models.JobClaim{}).Where("job_id = ?", first.ID).
		Update("lease_expires_at", time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatalf("expire the abandoned claim: %v", err)
	}

	recovery := newRuntimeTestAdapter()
	recovery.reconcile = func(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
		return jobs.ReconcileQueue, nil
	}
	recovery.dispatch = workerFinishes(nil)
	// A restart is a new process: its own Service instance, holding its own
	// registry, against the same durable state.
	recoveryService := jobs.NewService()
	if err := recoveryService.RegisterAdapter(recovery); err != nil {
		t.Fatalf("RegisterAdapter(recovery): %v", err)
	}

	restarted := NewJobRuntime(app, recoveryService, JobRuntimeConfig{Claimant: "runtime-b", Interval: time.Hour})
	restarted.tick(context.Background())

	waitFor(t, "the recovery runtime to resolve the abandoned Job", func() bool {
		return jobSnapshot(t, svc, app, first.ID).State == jobs.StateSucceeded
	})
	recovered := recovery.dispatched()
	if len(recovered) == 0 {
		t.Fatal("the recovered Job was never dispatched")
	}
	if recovered[0].ExecutionToken == execution.ExecutionToken {
		t.Fatal("the recovered execution reused the abandoned execution's token")
	}
	if recovered[0].Claimant != "runtime-b" {
		t.Fatalf("recovered claimant = %q, want the recovery runtime", recovered[0].Claimant)
	}
}
