package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
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

	cleanup func(context.Context, jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error)

	advertise func(context.Context, jobs.CommandContext) ([]jobs.Command, error)
	command   func(context.Context, jobs.CommandExecution) (jobs.CommandOutcome, error)

	mu         sync.Mutex
	executions []jobs.Execution
	requests   []jobs.ReconcileRequest
	cleanups   []jobs.ArtifactCleanupRequest
	commands   []jobs.CommandExecution
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

func (a *runtimeTestAdapter) CleanupArtifacts(ctx context.Context, request jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	a.mu.Lock()
	a.cleanups = append(a.cleanups, request)
	a.mu.Unlock()
	if a.cleanup != nil {
		return a.cleanup(ctx, request)
	}
	removed := make([]string, 0, len(request.Artifacts))
	for _, artifact := range request.Artifacts {
		removed = append(removed, artifact.Key)
	}
	return jobs.ArtifactCleanupResult{Removed: removed}, nil
}

func (a *runtimeTestAdapter) Commands(ctx context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	if a.advertise != nil {
		return a.advertise(ctx, commandContext)
	}
	return nil, nil
}

func (a *runtimeTestAdapter) ExecuteCommand(ctx context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	a.mu.Lock()
	a.commands = append(a.commands, execution)
	a.mu.Unlock()
	if a.command != nil {
		return a.command(ctx, execution)
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded}, nil
}

// commandExecutions is the commands this adapter was asked to run.
func (a *runtimeTestAdapter) commandExecutions() []jobs.CommandExecution {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]jobs.CommandExecution(nil), a.commands...)
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
		&models.JobClaim{}, &models.JobCapacityLease{}, &models.RuntimeSetting{},
	); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}

	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)
	// A live settings service, so a runtime here reads the deployment's Job
	// configuration the way a deployment does — including after an operator
	// changes one while work is running.
	settings := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), BuildDefaultsFromConfig(cfg))
	if err := settings.Load(); err != nil {
		t.Fatalf("load settings: %v", err)
	}
	ctx.SetSettings(settings)
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
		Replay: jobs.ReplayInput{NonReplayable: true},
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

// TestTerminalDeadlinesFollowTheSettingsInEffectAtCompletion is §9's "metadata
// retention starts at terminal completion", read as the setting the deployment
// had at that instant rather than the one it had when the work was handed out.
//
// The handle an execution publishes through is built when the Job is claimed and
// is the handle it finishes through — an execution may run for hours — so a window
// captured there is the operator's answer from before the change. Both deadlines
// the terminal write stamps are asserted, because they come from two different
// settings and are stamped by two different statements: the Job's own
// expires_at, and the sealed input's.
func TestTerminalDeadlinesFollowTheSettingsInEffectAtCompletion(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome jobs.State
		window  time.Duration
	}{
		{"a success follows the history window", jobs.StateSucceeded, time.Hour},
		{"a failure follows the attention window", jobs.StateFailed, 3 * time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			app := newJobRuntimeContext(t)
			svc := jobs.NewService()
			if err := svc.RegisterReplayCodec(runtimeTestKind, 1, runtimeTestCodec()); err != nil {
				t.Fatalf("RegisterReplayCodec: %v", err)
			}

			claimed := make(chan jobs.Execution, 1)
			release := make(chan struct{})
			adapter := newRuntimeTestAdapter()
			adapter.dispatch = func(_ context.Context, execution jobs.Execution) error {
				claimed <- execution
				<-release
				request := jobs.FinishRequest{ExpectedVersion: execution.Version, Outcome: testCase.outcome}
				if testCase.outcome == jobs.StateFailed {
					request.Failure = &jobs.Failure{Code: "boom", Class: jobs.FailureClassInternal, Message: "it broke"}
				}
				_, err := execution.Finish(request)
				return err
			}
			if err := svc.RegisterAdapter(adapter); err != nil {
				t.Fatalf("RegisterAdapter: %v", err)
			}

			accepted, err := svc.Accept(app.jobDeps(), jobs.Acceptance{
				Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
				Title:  "work in flight",
				Replay: jobs.ReplayInput{Input: json.RawMessage(`{"url":"https://files.example.test/clip.mp4"}`)},
			})
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}

			runtime := NewJobRuntime(app, svc, JobRuntimeConfig{Claimant: "runtime-test", Interval: time.Hour})
			runtime.Start()
			defer runtime.Stop()

			// The execution is claimed and paused inside its adapter: this is the
			// window an operator's change lands in.
			<-claimed
			for key, value := range map[string]string{
				KeyJobHistoryRetention:   "1h",
				KeyJobAttentionRetention: "3h",
				KeyJobReplayRetention:    "2h",
			} {
				if err := app.settings.Set(key, value, "test", "127.0.0.1"); err != nil {
					t.Fatalf("set %s: %v", key, err)
				}
			}
			close(release)

			waitFor(t, "the Job to finish", func() bool {
				return jobSnapshot(t, svc, app, accepted.ID).State == testCase.outcome
			})

			snap := jobSnapshot(t, svc, app, accepted.ID)
			if snap.FinishedAt == nil || snap.ExpiresAt == nil {
				t.Fatalf("the finished Job carries no deadline: %+v", snap)
			}
			if want := snap.FinishedAt.Add(testCase.window); !snap.ExpiresAt.Equal(want) {
				t.Fatalf("the Job's deadline = %v, want finished_at + the window in effect while it ran (%v)",
					snap.ExpiresAt, want)
			}

			var envelope models.JobReplayEnvelope
			if err := app.db.Where("job_id = ?", accepted.ID).First(&envelope).Error; err != nil {
				t.Fatalf("read the replay envelope: %v", err)
			}
			if envelope.ExpiresAt == nil {
				t.Fatal("the finished Job's sealed input carries no deadline")
			}
			if want := snap.FinishedAt.Add(2 * time.Hour); !envelope.ExpiresAt.Equal(want) {
				t.Fatalf("the envelope's deadline = %v, want finished_at + the replay window in effect while it ran (%v)",
					envelope.ExpiresAt, want)
			}
		})
	}
}

// runtimeTestCodec is the codec a runtime test's replayable Kind registers: it
// seals and opens the input unchanged, so the test is about what the runtime does
// with a Job rather than about a migration.
func runtimeTestCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(value json.RawMessage) (json.RawMessage, error) { return value, nil },
		Encode:   func(value json.RawMessage) (json.RawMessage, error) { return value, nil },
		Decode:   func(payload json.RawMessage, _ uint) (json.RawMessage, error) { return payload, nil },
		Migrate:  func(payload json.RawMessage, _, _ uint) (json.RawMessage, error) { return payload, nil },
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

// TestJobRuntimeStopsWhileReconciliationIsWaitingOnItsContext is the shutdown
// contract against an adapter that does what the interface says it may: it
// honors its context and waits for cancellation.
//
// Stop closed the loop's stop channel and then waited for the loop to return
// before cancelling the lifecycle context, so a reconciliation waiting on that
// context kept the wait open forever — the request to stop was never delivered
// because delivering it came after the wait.
func TestJobRuntimeStopsWhileReconciliationIsWaitingOnItsContext(t *testing.T) {
	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	adapter := newRuntimeTestAdapter()
	adapter.def.Lease = 20 * time.Millisecond
	entered := make(chan struct{})
	adapter.reconcile = func(ctx context.Context, _ jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
		close(entered)
		<-ctx.Done()
		return jobs.ReconcileQueue, nil
	}
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	// A Job claimed with a lease that has already run out, so the loop's next
	// pass finds an expired claim and asks the adapter what to do with it.
	acceptRuntimeJob(t, svc, app)
	if _, ok, err := svc.Claim(context.Background(), app.jobDeps(), jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, Claimant: "runtime-a", Lease: time.Millisecond,
	}); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	time.Sleep(5 * time.Millisecond)

	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{
		Interval: 5 * time.Millisecond, QuiesceTimeout: 250 * time.Millisecond,
	})
	runtime.Start()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the runtime never asked the adapter to reconcile the expired claim")
	}

	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop waited for the reconciliation it was supposed to cancel")
	}
}

// TestJobRuntimeBlocksQueuedWorkWhoseAdapterIsGone is the restart the design's
// §3 answers: work accepted under one Service, and a process that comes back with
// no adapter for its Kind.
//
// Reconciliation only visits held claims, so a Job that was accepted and never
// claimed is invisible to it — no claim to expire, no registration to be reached
// through — and it would stay queued forever while the process reports itself
// healthy. §2/§3 make a missing adapter block nonterminal work rather than fall
// back to another executor, and the blocked state is what puts it in front of the
// person who can decide what happens to it.
func TestJobRuntimeBlocksQueuedWorkWhoseAdapterIsGone(t *testing.T) {
	app := newJobRuntimeContext(t)

	// The process that accepted the work: one Kind, one adapter.
	original := jobs.NewService()
	if err := original.RegisterAdapter(newRuntimeTestAdapter()); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}
	accepted := acceptRuntimeJob(t, original, app)
	due := time.Now().Add(24 * time.Hour).UTC()
	scheduled, err := original.Accept(app.jobDeps(), jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateScheduled, Origin: "api",
		ScheduledFor: &due, Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept scheduled job: %v", err)
	}

	// The restart: a fresh control plane with no registration for that Kind, and
	// the runtime that owns the loop.
	restarted := jobs.NewService()
	runtime := NewJobRuntime(app, restarted, JobRuntimeConfig{Claimant: "runtime-b", Interval: time.Hour})
	runtime.Start()
	defer runtime.Stop()

	for _, job := range []jobs.Snapshot{accepted, scheduled} {
		waitFor(t, "the Job to be blocked", func() bool {
			var stored models.Job
			if err := app.db.Where("id = ?", job.ID).First(&stored).Error; err != nil {
				return false
			}
			return stored.State == string(jobs.StateBlocked)
		})
		if held := storedCapacity(t, app, jobs.CapacityGroupGlobal) + storedCapacity(t, app, runtimeTestKind); held != 0 {
			t.Fatalf("blocking pending work took %d capacity slots: nothing is executing", held)
		}
		var claims int64
		if err := app.db.Model(&models.JobClaim{}).Where("job_id = ?", job.ID).Count(&claims).Error; err != nil {
			t.Fatalf("count claims for %s: %v", job.ID, err)
		}
		if claims != 0 {
			t.Fatalf("blocking pending work took a claim: %d rows", claims)
		}
	}

	// The Kind this process *can* run is unaffected: registering the adapter again
	// is all it takes for new work of that Kind to run.
	if err := restarted.RegisterAdapter(newRuntimeTestAdapter()); err != nil {
		t.Fatalf("re-register the adapter: %v", err)
	}
	later := acceptRuntimeJob(t, restarted, app)
	runtime.tick(context.Background())
	waitFor(t, "the Job accepted after the adapter came back to be dispatched", func() bool {
		stored := jobSnapshot(t, restarted, app, later.ID)
		return stored.State != jobs.StateQueued && stored.State != jobs.StateRunning
	})
}

// replayLogSecrets are the shapes a decrypted replay input carries that must never
// reach a log line: a URL's query string, a Cookie, an Authorization header and a
// value a plugin supplied. They are deliberately not secret-shaped, so an
// assertion that scans for them cannot pass by accident.
const (
	replayLogQuerySecret  = "runtime-url-query-secret-4d1a"
	replayLogCookieSecret = "runtime-cookie-secret-7e2b"
	replayLogAuthSecret   = "runtime-bearer-secret-1c9d"
	replayLogPluginSecret = "runtime-plugin-secret-6f30"
)

// lockedBuffer is a log destination a test goroutine and the runtime's goroutines
// can share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestJobRuntimeNeverLogsTheDecryptedInputItCouldNotDecode is §5's redaction rule
// where it was actually broken: the runtime's own log.
//
// A codec's Decode runs on the decrypted envelope, so the errors a real one
// produces quote their input — a URL with its query string, a Cookie, an
// Authorization header, a plugin value. Claim propagated that error, and the loop
// logged it verbatim, so a secret that had been stored encrypted and never rendered
// appeared in plaintext in the deployment's log. The failure classification is
// unchanged: the Job is still blocked rather than run with input nobody can decode.
func TestJobRuntimeNeverLogsTheDecryptedInputItCouldNotDecode(t *testing.T) {
	secrets := []string{replayLogQuerySecret, replayLogCookieSecret, replayLogAuthSecret, replayLogPluginSecret}
	input := jobs.ReplayInput{Input: json.RawMessage(`{
  "url": "https://files.example.test/media/clip.mp4?token=` + replayLogQuerySecret + `",
  "headers": {"Cookie": "sid=` + replayLogCookieSecret + `", "Authorization": "Bearer ` + replayLogAuthSecret + `"},
  "pluginSecret": "` + replayLogPluginSecret + `"
}`)}

	app := newJobRuntimeContext(t)
	svc := jobs.NewService()
	codec := jobs.ReplayCodec{
		Sanitize: func(json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"host":"files.example.test"}`), nil
		},
		Encode: func(value json.RawMessage) (json.RawMessage, error) { return value, nil },
		Decode: func(payload json.RawMessage, _ uint) (json.RawMessage, error) {
			// The shape a parser's own error takes: it quotes what it could not read.
			return nil, fmt.Errorf("cannot decode payload %s", payload)
		},
		Migrate: func(payload json.RawMessage, _, _ uint) (json.RawMessage, error) { return payload, nil },
	}
	if err := svc.RegisterReplayCodec(runtimeTestKind, 1, codec); err != nil {
		t.Fatalf("RegisterReplayCodec: %v", err)
	}
	if err := svc.RegisterAdapter(newRuntimeTestAdapter()); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}
	accepted, err := svc.Accept(app.jobDeps(), jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		Replay: input,
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	captured := &lockedBuffer{}
	previous := log.Writer()
	log.SetOutput(captured)
	defer log.SetOutput(previous)

	runtime := NewJobRuntime(app, svc, JobRuntimeConfig{Claimant: "runtime-test", Interval: time.Hour})
	runtime.Start()
	defer runtime.Stop()

	waitFor(t, "the Job to be blocked", func() bool {
		return jobSnapshot(t, svc, app, accepted.ID).State == jobs.StateBlocked
	})
	// The claim failure was logged at all: without this, the absence of the
	// secrets below could be the absence of a log line rather than redaction.
	if logged := captured.String(); !strings.Contains(logged, "job runtime: claiming") {
		t.Fatalf("the runtime logged nothing about the refused claim: %q", logged)
	}
	if logged := captured.String(); !strings.Contains(logged, "replay") {
		t.Fatalf("the runtime logged something other than the replay refusal: %q", logged)
	}
	for _, secret := range secrets {
		if logged := captured.String(); strings.Contains(logged, secret) {
			t.Fatalf("the runtime logged a decrypted secret (%s): %q", secret, logged)
		}
	}
	if held := storedCapacity(t, app, jobs.CapacityGroupGlobal) + storedCapacity(t, app, runtimeTestKind); held != 0 {
		t.Fatalf("the blocked Job still holds %d capacity slots", held)
	}
}
