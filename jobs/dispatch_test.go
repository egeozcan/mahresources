package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"
)

// This file drives the dispatch seam the same way a production runtime does:
// register a Kind adapter, claim its waiting work, hand the execution to the
// adapter, and reconcile what a dead claimant left behind. It asserts the
// durable rows each step leaves, because "a claim happened" is a claim about a
// transaction.

// testKind is the Kind these tests register. One Kind is enough to drive every
// dispatch behavior; the interesting variation is in the Definition the test
// registers it with.
const testKind = "test-work"

// testAdapter is a Kind adapter with one hook per behavior a test needs to
// observe: what it was handed, and what it answers.
type testAdapter struct {
	def       Definition
	dispatch  func(context.Context, Execution) error
	reconcile func(context.Context, ReconcileRequest) (ReconcileDecision, error)

	cleanup func(context.Context, ArtifactCleanupRequest) (ArtifactCleanupResult, error)

	mu         sync.Mutex
	dispatched []Execution
	reconciled []ReconcileRequest
	cleanups   []ArtifactCleanupRequest
}

func newTestAdapter(def Definition) *testAdapter { return &testAdapter{def: def} }

func (a *testAdapter) Definition() Definition { return a.def }

func (a *testAdapter) Dispatch(ctx context.Context, execution Execution) error {
	a.mu.Lock()
	a.dispatched = append(a.dispatched, execution)
	a.mu.Unlock()
	if a.dispatch != nil {
		return a.dispatch(ctx, execution)
	}
	return nil
}

func (a *testAdapter) Reconcile(ctx context.Context, request ReconcileRequest) (ReconcileDecision, error) {
	a.mu.Lock()
	a.reconciled = append(a.reconciled, request)
	a.mu.Unlock()
	if a.reconcile != nil {
		return a.reconcile(ctx, request)
	}
	return ReconcileRemainRunning, nil
}

func (a *testAdapter) CleanupArtifacts(ctx context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
	a.mu.Lock()
	a.cleanups = append(a.cleanups, request)
	a.mu.Unlock()
	if a.cleanup != nil {
		return a.cleanup(ctx, request)
	}
	// The default is the honest answer for a Kind whose artifacts the host owns:
	// every one of them is gone.
	removed := make([]string, 0, len(request.Artifacts))
	for _, artifact := range request.Artifacts {
		removed = append(removed, artifact.Key)
	}
	return ArtifactCleanupResult{Removed: removed}, nil
}

func (a *testAdapter) Commands(context.Context, CommandContext) ([]Command, error) { return nil, nil }

func (a *testAdapter) ExecuteCommand(context.Context, CommandExecution) (CommandOutcome, error) {
	return CommandOutcome{}, nil
}

func (a *testAdapter) cleanupCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.cleanups)
}

func (a *testAdapter) dispatchedCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.dispatched)
}

func (a *testAdapter) lastDispatch() Execution {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.dispatched) == 0 {
		return Execution{}
	}
	return a.dispatched[len(a.dispatched)-1]
}

func (a *testAdapter) reconciledCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.reconciled)
}

func (a *testAdapter) lastReconcile() ReconcileRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.reconciled) == 0 {
		return ReconcileRequest{}
	}
	return a.reconciled[len(a.reconciled)-1]
}

// testDefinition is a Kind that can be restarted after runtime loss, with a
// one-minute lease and a per-Kind budget of four concurrent executions.
func testDefinition() Definition {
	return Definition{
		Kind: testKind, KindVersion: 1, Restorable: true,
		MaxConcurrent: 4, Lease: time.Minute,
	}
}

// registerTestAdapter registers a Kind whose fixed definition the test chooses.
func registerTestAdapter(t *testing.T, svc *Service, def Definition) *testAdapter {
	t.Helper()
	adapter := newTestAdapter(def)
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter(%s v%d): %v", def.Kind, def.KindVersion, err)
	}
	return adapter
}

// newDispatchDatabase opens a file-backed SQLite database with the durable job
// core and the claim tables migrated, and returns its DSN so a test can open a
// second handle on the same database — two connections to one file is what
// makes a claim race a race rather than a sequence.
func newDispatchDatabase(t *testing.T, name string) (string, Deps) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), name)
	return dsn, openDispatchDatabase(t, dsn)
}

func openDispatchDatabase(t *testing.T, dsn string) Deps {
	t.Helper()
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
		&models.JobPreference{}, &models.JobPinGuard{},
	); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}
	return Deps{DB: db}
}

// acceptQueued accepts one queued Job of the test Kind.
func acceptQueued(t *testing.T, svc *Service, deps Deps, owner *uint) Snapshot {
	t.Helper()
	snap, err := svc.Accept(deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: owner, ActorUserID: owner,
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept queued job: %v", err)
	}
	return snap
}

// claimOnce claims one Job for a runtime, failing the test on an unexpected
// error.
func claimOnce(t *testing.T, svc *Service, deps Deps, claimant string, capacity ...CapacityRef) (Execution, bool) {
	t.Helper()
	execution, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: claimant, Capacity: capacity,
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return execution, ok
}

func claimRow(t *testing.T, deps Deps, jobID string) models.JobClaim {
	t.Helper()
	var claim models.JobClaim
	if err := deps.DB.Where("job_id = ?", jobID).First(&claim).Error; err != nil {
		t.Fatalf("load claim for %s: %v", jobID, err)
	}
	return claim
}

func capacityRows(t *testing.T, deps Deps, jobID string) []models.JobCapacityLease {
	t.Helper()
	var leases []models.JobCapacityLease
	if err := deps.DB.Where("job_id = ?", jobID).Order("capacity_group, slot").Find(&leases).Error; err != nil {
		t.Fatalf("load capacity leases for %s: %v", jobID, err)
	}
	return leases
}

func capacityCount(t *testing.T, deps Deps, group string) int64 {
	t.Helper()
	var count int64
	if err := deps.DB.Model(&models.JobCapacityLease{}).Where("capacity_group = ?", group).Count(&count).Error; err != nil {
		t.Fatalf("count capacity leases for %s: %v", group, err)
	}
	return count
}

// TestClaimMovesTheJobAndItsLeaseCapacityAndStartedEventInOneTransaction is
// Task 4's central fact: one transaction moves the Job to running, installs the
// fencing token, records the lease, occupies every capacity budget the runtime
// asked for, and appends the started event.
func TestClaimMovesTheJobAndItsLeaseCapacityAndStartedEventInOneTransaction(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, uintPtr(4))
	execution, ok := claimOnce(t, svc, deps, "runtime-a",
		CapacityRef{Group: CapacityGroupGlobal, Limit: 4},
	)
	if !ok {
		t.Fatal("a queued Job with free capacity was not claimed")
	}
	if execution.JobID != accepted.ID {
		t.Fatalf("claimed job %s, want %s", execution.JobID, accepted.ID)
	}
	if execution.ExecutionToken == "" {
		t.Fatal("the claim has no execution token, so no executor can be fenced")
	}
	if execution.Version != accepted.Version+1 {
		t.Fatalf("execution version = %d, want the version the claim created (%d)",
			execution.Version, accepted.Version+1)
	}
	if execution.Kind != testKind || execution.KindVersion != 1 {
		t.Fatalf("execution names %s v%d", execution.Kind, execution.KindVersion)
	}
	if execution.Claimant != "runtime-a" {
		t.Fatalf("execution claimant = %q", execution.Claimant)
	}
	if execution.Access.UserID != 4 {
		t.Fatalf("execution acts as user %d, want the Job's owner 4", execution.Access.UserID)
	}

	job := jobRow(t, deps, accepted.ID)
	if job.State != string(StateRunning) {
		t.Fatalf("job state = %s, want running", job.State)
	}
	if job.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("job token = %q, claim token = %q", job.ExecutionToken, execution.ExecutionToken)
	}
	if job.Version != accepted.Version+1 {
		t.Fatalf("job version = %d, want %d", job.Version, accepted.Version+1)
	}
	if job.StartedAt == nil || !job.StartedAt.Equal(clock) {
		t.Fatalf("job started_at = %v, want %v", job.StartedAt, clock)
	}

	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateHeld {
		t.Fatalf("claim state = %s, want held", claim.State)
	}
	if claim.ExecutionToken != execution.ExecutionToken || claim.Claimant != "runtime-a" {
		t.Fatalf("claim row = %+v", claim)
	}
	if !claim.LeaseExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("lease expires at %v, want %v", claim.LeaseExpiresAt, clock.Add(time.Minute))
	}
	if !claim.HeartbeatAt.Equal(clock) {
		t.Fatalf("claim heartbeat = %v, want the claim instant %v", claim.HeartbeatAt, clock)
	}
	if claim.Kind != testKind || claim.KindVersion != 1 {
		t.Fatalf("claim names %s v%d", claim.Kind, claim.KindVersion)
	}

	leases := capacityRows(t, deps, accepted.ID)
	if len(leases) != 2 {
		t.Fatalf("capacity leases = %d, want one per budget (global and %s)", len(leases), testKind)
	}
	for _, lease := range leases {
		if lease.ExecutionToken != execution.ExecutionToken {
			t.Fatalf("capacity lease for %s carries token %q", lease.CapacityGroup, lease.ExecutionToken)
		}
	}

	events := jobEvents(t, deps, accepted.ID)
	if len(events) != 2 {
		t.Fatalf("timeline = %d events, want accepted and started", len(events))
	}
	started := events[1]
	if started.Type != EventStarted || !started.ReservedHost {
		t.Fatalf("second event = %+v, want a reserved started event", started)
	}
}

// TestClaimAdmitsOneWinnerWhenTwoConnectionsRace is the admission contract: two
// connections contending for one queued Job admit exactly one execution, and
// the loser leaves nothing behind.
func TestClaimAdmitsOneWinnerWhenTwoConnectionsRace(t *testing.T) {
	dsn, deps := newDispatchDatabase(t, "claim-race.db")
	svc := NewService()
	// No budget: this test is about admission, and a filled budget would refuse
	// later iterations for an unrelated reason.
	registerTestAdapter(t, svc, Definition{Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute})

	for iteration := 0; iteration < 5; iteration++ {
		accepted := acceptQueued(t, svc, deps, nil)

		// A second handle on the same file: the racers are two connections,
		// which is what makes this a database race rather than a mutex test.
		other := openDispatchDatabase(t, dsn)
		start := make(chan struct{})
		results := make([]bool, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, handle := range []Deps{deps, other} {
			wg.Add(1)
			go func(idx int, claimDeps Deps) {
				defer wg.Done()
				<-start
				_, ok, err := svc.Claim(context.Background(), claimDeps, ClaimRequest{
					Kind: testKind, KindVersion: 1, Claimant: "runtime-" + string(rune('a'+idx)),
				})
				results[idx], errs[idx] = ok, err
			}(i, handle)
		}
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("iteration %d: racer %d: %v", iteration, i, err)
			}
		}
		winners := 0
		for _, ok := range results {
			if ok {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("iteration %d: %d racers claimed one Job, want exactly one", iteration, winners)
		}

		var claims int64
		if err := deps.DB.Model(&models.JobClaim{}).Where("job_id = ?", accepted.ID).Count(&claims).Error; err != nil {
			t.Fatalf("count claims: %v", err)
		}
		if claims != 1 {
			t.Fatalf("iteration %d: %d claim rows for one Job", iteration, claims)
		}
		if started := jobEvents(t, deps, accepted.ID); len(started) != 2 {
			t.Fatalf("iteration %d: the losing racer recorded %d events", iteration, len(started))
		}
	}
}

// TestClaimRefusesAFullBudgetWithoutClaimingAnything is the other half of the
// admission contract: a refused capacity budget rolls the whole claim back, so
// a Job is never left running without the capacity that admitted it.
func TestClaimRefusesAFullBudgetWithoutClaimingAnything(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-capacity.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	first := acceptQueued(t, svc, deps, nil)
	second := acceptQueued(t, svc, deps, nil)

	budget := []CapacityRef{{Group: CapacityGroupGlobal, Limit: 1}, {Group: testKind, Limit: 5}}
	if _, ok := claimOnce(t, svc, deps, "runtime-a", budget...); !ok {
		t.Fatal("the first claim was refused with an empty budget")
	}
	if _, ok := claimOnce(t, svc, deps, "runtime-a", budget...); ok {
		t.Fatal("a second Job was claimed while the deployment-wide budget was full")
	}

	if stored := jobRow(t, deps, second.ID); stored.State != string(StateQueued) {
		t.Fatalf("the refused Job is %s, want queued: a refused claim must write nothing", stored.State)
	}
	if stored := jobRow(t, deps, second.ID); stored.ExecutionToken != "" {
		t.Fatalf("the refused Job carries token %q", stored.ExecutionToken)
	}
	var claims int64
	if err := deps.DB.Model(&models.JobClaim{}).Count(&claims).Error; err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 1 {
		t.Fatalf("claim rows = %d, want only the winner's", claims)
	}
	if events := jobEvents(t, deps, second.ID); len(events) != 1 {
		t.Fatalf("the refused Job has %d events, want only its acceptance", len(events))
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal); count != 1 {
		t.Fatalf("global capacity rows = %d, want 1", count)
	}
	if count := capacityCount(t, deps, testKind); count != 1 {
		t.Fatalf("kind capacity rows = %d, want 1: a rolled back claim must free every budget it took", count)
	}
	_ = first
}

// TestClaimRefusesAKindNoAdapterIsRegisteredFor keeps dispatch from inventing an
// executor: work whose Kind this process cannot run is refused, never handed to
// another Kind.
func TestClaimRefusesAKindNoAdapterIsRegisteredFor(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-unregistered.db")
	svc := NewService()

	_, _, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
	})
	if !errors.Is(err, ErrAdapterUnregistered) {
		t.Fatalf("Claim for an unregistered Kind = %v, want ErrAdapterUnregistered", err)
	}
}

// TestExecutionHandsTheAdapterTheInputActorAndReport is the seam itself: an
// adapter receives the Job identity, its token, the decoded input, the
// principal the work acts as, and the report it publishes through — and nothing
// else, in particular no database handle.
func TestExecutionHandsTheAdapterTheInputActorAndReport(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-execution.db")
	svc := NewService()
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "dispatch-key-material")}
	registerTestCodec(t, svc)

	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted, err := svc.Accept(deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(4), ActorUserID: uintPtr(9),
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"the input only the executor sees"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}

	var seen Execution
	var progressed Snapshot
	adapter := newTestAdapter(testDefinition())
	adapter.dispatch = func(_ context.Context, execution Execution) error {
		seen = execution
		snap, err := execution.Progress(Progress{Phase: "working", Completed: int64Ptr(3), Total: int64Ptr(9)})
		if err != nil {
			return err
		}
		progressed = snap
		if err := execution.Event(EventInput{Type: "checkpoint"}); err != nil {
			return err
		}
		if _, err := execution.Output(OutputInput{
			Key: "summary", Type: OutputTypeSummary, Reference: json.RawMessage(`{"count":3}`),
		}); err != nil {
			return err
		}
		_, err = execution.Finish(FinishRequest{ExpectedVersion: execution.Version, Outcome: StateSucceeded})
		return err
	}
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}

	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the accepted Job was not claimed")
	}
	if err := adapter.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if seen.JobID != accepted.ID || seen.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the adapter was handed %+v, want the claimed execution", seen)
	}
	if string(seen.Input) != `{"secret":"the input only the executor sees"}` {
		t.Fatalf("the adapter's input = %s, want the decoded replay input", seen.Input)
	}
	if seen.Access.UserID != 9 {
		t.Fatalf("the adapter acts as user %d, want the Job's actor 9", seen.Access.UserID)
	}

	if progressed.Progress.Phase != "working" || progressed.Progress.Completed == nil || *progressed.Progress.Completed != 3 {
		t.Fatalf("progress = %+v, want the snapshot the adapter published", progressed.Progress)
	}

	snap, err := svc.Get(deps, Access{Administrator: true}, accepted.ID)
	if err != nil {
		t.Fatalf("Get after finish: %v", err)
	}
	if snap.State != StateSucceeded {
		t.Fatalf("job state = %s, want succeeded through the execution's own report", snap.State)
	}
	types := []string{}
	for _, event := range jobEvents(t, deps, accepted.ID) {
		types = append(types, event.Type)
	}
	want := []string{EventAccepted, EventStarted, "checkpoint", EventSucceeded}
	if len(types) != len(want) {
		t.Fatalf("timeline = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("timeline = %v, want %v", types, want)
		}
	}

	var outputs []models.JobOutput
	if err := deps.DB.Where("job_id = ?", accepted.ID).Find(&outputs).Error; err != nil {
		t.Fatalf("load outputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Key != "summary" {
		t.Fatalf("outputs = %+v, want the summary the execution published", outputs)
	}
}

// registerTestCodec registers the one Kind codec these tests accept input with.
func registerTestCodec(t *testing.T, svc *Service) {
	t.Helper()
	err := svc.RegisterReplayCodec(testKind, 1, ReplayCodec{
		Sanitize: func(json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"safe":"summary"}`), nil
		},
		Encode:  func(input json.RawMessage) (json.RawMessage, error) { return input, nil },
		Decode:  func(payload json.RawMessage, _ uint) (json.RawMessage, error) { return payload, nil },
		Migrate: func(payload json.RawMessage, _, _ uint) (json.RawMessage, error) { return payload, nil },
	})
	if err != nil {
		t.Fatalf("RegisterReplayCodec: %v", err)
	}
}

// --- heartbeats and release -------------------------------------------------

// TestHeartbeatExtendsOnlyTheClaimItsTokenOwns is the lease half of the fence:
// the runtime that owns the execution keeps it alive, and a runtime whose token
// was replaced cannot extend anything.
func TestHeartbeatExtendsOnlyTheClaimItsTokenOwns(t *testing.T) {
	_, deps := newDispatchDatabase(t, "heartbeat.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

	// A heartbeat inside the lease keeps the claim alive: the point is to have a
	// usable lease ahead of the last proof of life, not to shorten it and not to
	// carry the old expiry forward as well.
	clock = clock.Add(10 * time.Second)
	if err := svc.Heartbeat(deps, ref, time.Minute); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	claim := claimRow(t, deps, accepted.ID)
	if !claim.LeaseExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("lease = %v, want one lease past the heartbeat (%v)",
			claim.LeaseExpiresAt, clock.Add(time.Minute))
	}
	if !claim.HeartbeatAt.Equal(clock) {
		t.Fatalf("heartbeat = %v, want %v", claim.HeartbeatAt, clock)
	}

	// Past the expiry, the extension is measured from now: a claim that was
	// already stale must not be handed a lease in the past.
	clock = clock.Add(2 * time.Hour)
	if err := svc.Heartbeat(deps, ref, time.Minute); err != nil {
		t.Fatalf("Heartbeat after expiry: %v", err)
	}
	claim = claimRow(t, deps, accepted.ID)
	if !claim.LeaseExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("lease after a late heartbeat = %v, want %v", claim.LeaseExpiresAt, clock.Add(time.Minute))
	}

	// A token that does not own the claim extends nothing.
	stale := ExecutionRef{JobID: accepted.ID, ExecutionToken: "some-other-execution"}
	if err := svc.Heartbeat(deps, stale, time.Hour); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("Heartbeat with a foreign token = %v, want ErrStaleExecution", err)
	}
	if arrived := claimRow(t, deps, accepted.ID); !arrived.LeaseExpiresAt.Equal(clock.Add(time.Minute)) {
		t.Fatalf("a foreign token moved the lease to %v, want %v", arrived.LeaseExpiresAt, clock.Add(time.Minute))
	}
}

// TestReleaseClaimFreesTheCapacityAndTheTokenWithoutMovingTheJob covers the
// half of ownership the state machine does not own: a runtime that stops
// running an execution hands the claim back, and the durable evidence of what it
// held is freed rather than left occupying a budget.
func TestReleaseClaimFreesTheCapacityAndTheTokenWithoutMovingTheJob(t *testing.T) {
	_, deps := newDispatchDatabase(t, "release.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

	released, err := svc.ReleaseClaim(deps, ref, ReleaseReasonExecutionEnded)
	if err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if released.State != StateRunning {
		t.Fatalf("release moved the Job to %s; releasing ownership is not a state change", released.State)
	}

	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released", claim.State)
	}
	if claim.ReleasedAt == nil || !claim.ReleasedAt.Equal(clock) {
		t.Fatalf("released_at = %v, want %v", claim.ReleasedAt, clock)
	}
	if claim.ReleaseReason != ReleaseReasonExecutionEnded {
		t.Fatalf("release reason = %q", claim.ReleaseReason)
	}
	if leases := capacityRows(t, deps, accepted.ID); len(leases) != 0 {
		t.Fatalf("capacity leases after release = %d, want none", len(leases))
	}
	if stored := jobRow(t, deps, accepted.ID); stored.ExecutionToken != "" {
		t.Fatalf("job token after release = %q, want cleared", stored.ExecutionToken)
	}

	// A second release under the same token is a no-op rather than an error, and
	// a foreign token cannot release somebody else's claim.
	if _, err := svc.ReleaseClaim(deps, ref, ReleaseReasonExecutionEnded); err != nil {
		t.Fatalf("second ReleaseClaim: %v", err)
	}
	if err := func() error {
		_, err := svc.ReleaseClaim(deps, ExecutionRef{JobID: accepted.ID, ExecutionToken: "foreign"}, ReleaseReasonExecutionEnded)
		return err
	}(); err != nil {
		t.Fatalf("ReleaseClaim with a foreign token: %v", err)
	}
}

// TestLeavingTheRunningStateReleasesTheClaimAndItsCapacity ties the two halves
// together: a Job that is no longer running is not owned by an execution, so its
// claim and capacity are freed in the same transaction that moved it, and no
// budget is leaked by ordinary completion.
func TestLeavingTheRunningStateReleasesTheClaimAndItsCapacity(t *testing.T) {
	_, deps := newDispatchDatabase(t, "release-on-transition.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal); count != 1 {
		t.Fatalf("global capacity rows = %d, want 1", count)
	}

	_, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version,
		Outcome:         StateSucceeded,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state after the terminal transition = %s, want released", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal); count != 0 {
		t.Fatalf("global capacity rows after completion = %d, want 0", count)
	}
	if count := capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("kind capacity rows after completion = %d, want 0", count)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.ExecutionToken != "" {
		t.Fatalf("job token after completion = %q, want cleared", stored.ExecutionToken)
	}
}

// TestRegisterAdapterRefusesDuplicatesAndMalformedDefinitions pins registration:
// one adapter per (Kind, version), and a definition the control plane cannot
// enforce is refused rather than registered.
func TestRegisterAdapterRefusesDuplicatesAndMalformedDefinitions(t *testing.T) {
	svc := NewService()

	if err := svc.RegisterAdapter(newTestAdapter(testDefinition())); err != nil {
		t.Fatalf("RegisterAdapter: %v", err)
	}
	if err := svc.RegisterAdapter(newTestAdapter(testDefinition())); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate registration = %v, want ErrInvalidDefinition", err)
	}
	// A second version of the same Kind is a different adapter, not a duplicate.
	if err := svc.RegisterAdapter(newTestAdapter(Definition{Kind: testKind, KindVersion: 2})); err != nil {
		t.Fatalf("RegisterAdapter for v2: %v", err)
	}

	cases := map[string]Definition{
		"no kind":                {KindVersion: 1},
		"no version":             {Kind: testKind},
		"negative budget":        {Kind: testKind, KindVersion: 1, MaxConcurrent: -1},
		"negative lease":         {Kind: testKind, KindVersion: 1, Lease: -time.Second},
		"oversized kind":         {Kind: string(make([]byte, MaxKindBytes+1)), KindVersion: 1},
		"oversized budget group": {Kind: testKind, KindVersion: 1, CapacityGroup: string(make([]byte, MaxCapacityGroupBytes+1))},
	}
	for name, definition := range cases {
		if err := svc.RegisterAdapter(newTestAdapter(definition)); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%s = %v, want ErrInvalidDefinition", name, err)
		}
	}

	registered := svc.Registrations()
	if len(registered) != 2 {
		t.Fatalf("registrations = %d, want the two versions of %s", len(registered), testKind)
	}
	if registered[0].Definition.KindVersion != 1 || registered[1].Definition.KindVersion != 2 {
		t.Fatalf("registrations are not ordered by version: %+v", registered)
	}
}

// --- reconciliation of expired claims ---------------------------------------

// expireClaim ages a claim past its lease without touching the Job, which is
// exactly the state a crashed runtime leaves behind. A claim that is not there
// is a test bug rather than an inert step, so it fails rather than continuing
// with a lease that never expired.
func expireClaim(t *testing.T, deps Deps, jobID string, now time.Time) {
	t.Helper()
	result := deps.DB.Model(&models.JobClaim{}).Where("job_id = ?", jobID).
		Update("lease_expires_at", now.Add(-time.Minute))
	if result.Error != nil {
		t.Fatalf("expire claim: %v", result.Error)
	}
	if result.RowsAffected != 1 {
		t.Fatalf("expiring the claim on job %s matched %d rows", jobID, result.RowsAffected)
	}
}

// reconcileOnce runs one reconciliation pass for the adjudicating runtime.
func reconcileOnce(t *testing.T, svc *Service, deps Deps, claimant string) ReconcileReport {
	t.Helper()
	report, err := svc.ReconcileExpired(context.Background(), deps, claimant, DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileExpired: %v", err)
	}
	return report
}

// TestReconcileAsksTheAdapterAndAppliesOnlyWhatItAnswers drives the decision
// vocabulary: expiry permits reconciliation, and the adapter's answer — not the
// control plane's guess — is what moves the Job.
func TestReconcileAsksTheAdapterAndAppliesOnlyWhatItAnswers(t *testing.T) {
	cases := []struct {
		name          string
		decision      ReconcileDecision
		wantState     State
		wantClaim     string
		wantCapacity  int64
		wantTokenKept bool
		wantResumed   int
		wantEvent     string
		wantFailure   string
	}{
		{
			name: "remain-running", decision: ReconcileRemainRunning,
			wantState: StateRunning, wantClaim: models.JobClaimStateHeld,
			wantCapacity: 2, wantTokenKept: true,
		},
		{
			name: "resume", decision: ReconcileResume,
			wantState: StateRunning, wantClaim: models.JobClaimStateHeld,
			wantCapacity: 2, wantTokenKept: false, wantResumed: 1, wantEvent: EventResumed,
		},
		{
			name: "queue", decision: ReconcileQueue,
			wantState: StateQueued, wantClaim: models.JobClaimStateReleased,
			wantCapacity: 0, wantTokenKept: false, wantEvent: EventQueued,
		},
		{
			name: "block", decision: ReconcileBlock,
			wantState: StateBlocked, wantClaim: models.JobClaimStateReleased,
			wantCapacity: 0, wantTokenKept: false, wantEvent: EventBlocked,
		},
		{
			name: "interrupt", decision: ReconcileInterrupt,
			wantState: StateInterrupted, wantClaim: models.JobClaimStateReleased,
			wantCapacity: 0, wantTokenKept: false, wantEvent: EventInterrupted,
		},
		{
			name: "fail", decision: ReconcileFail,
			wantState: StateFailed, wantClaim: models.JobClaimStateReleased,
			wantCapacity: 0, wantTokenKept: false, wantEvent: EventFailed,
			wantFailure: ReconcileFailureCode,
		},
		{
			name: "succeed", decision: ReconcileSucceed,
			wantState: StateSucceeded, wantClaim: models.JobClaimStateReleased,
			wantCapacity: 0, wantTokenKept: false, wantEvent: EventSucceeded,
		},
		{
			name: "unproven external work", decision: ReconcileExternalWorkUnproven,
			wantState: StateBlocked, wantClaim: models.JobClaimStateQuarantined,
			wantCapacity: 2, wantTokenKept: true, wantEvent: EventBlocked,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, deps := newDispatchDatabase(t, "reconcile.db")
			svc := NewService()
			adapter := registerTestAdapter(t, svc, testDefinition())
			adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
				return tt.decision, nil
			}
			clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
			deps.Now = func() time.Time { return clock }

			accepted := acceptQueued(t, svc, deps, nil)
			execution, ok := claimOnce(t, svc, deps, "runtime-a",
				CapacityRef{Group: CapacityGroupGlobal, Limit: 2})
			if !ok {
				t.Fatal("the queued Job was not claimed")
			}
			expiredToken := execution.ExecutionToken
			expireClaim(t, deps, accepted.ID, clock)

			report := reconcileOnce(t, svc, deps, "runtime-b")
			if report.Examined != 1 {
				t.Fatalf("examined %d expired claims, want 1", report.Examined)
			}
			if len(report.Outcomes) != 1 {
				t.Fatalf("applied %d decisions, want one", len(report.Outcomes))
			}
			if report.Outcomes[0].Decision != tt.decision {
				t.Fatalf("recorded decision = %q, want %q", report.Outcomes[0].Decision, tt.decision)
			}
			if len(report.Resume) != tt.wantResumed {
				t.Fatalf("resume executions = %d, want %d", len(report.Resume), tt.wantResumed)
			}
			if len(adapter.reconciled) != 1 {
				t.Fatalf("the adapter was asked %d times, want once", adapter.reconciledCount())
			}
			if got := adapter.lastReconcile(); got.Claimant != "runtime-a" || !got.LeaseExpiredAt.Equal(clock.Add(-time.Minute)) {
				t.Fatalf("reconcile request = %+v, want the expired claim's claimant and lease", got)
			}

			stored := jobRow(t, deps, accepted.ID)
			if stored.State != string(tt.wantState) {
				t.Fatalf("job state = %s, want %s", stored.State, tt.wantState)
			}
			claim := claimRow(t, deps, accepted.ID)
			if claim.State != tt.wantClaim {
				t.Fatalf("claim state = %s, want %s", claim.State, tt.wantClaim)
			}
			if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != tt.wantCapacity {
				t.Fatalf("capacity rows = %d, want %d", count, tt.wantCapacity)
			}
			if tt.wantTokenKept && stored.ExecutionToken != expiredToken {
				t.Fatalf("job token = %q, want the expired claim's %q kept", stored.ExecutionToken, expiredToken)
			}
			if !tt.wantTokenKept && stored.ExecutionToken == expiredToken {
				t.Fatalf("job token is still the expired claim's %q", expiredToken)
			}
			if tt.wantFailure != "" && stored.FailureCode != tt.wantFailure {
				t.Fatalf("failure code = %q, want %q", stored.FailureCode, tt.wantFailure)
			}
			if tt.wantEvent != "" {
				events := jobEvents(t, deps, accepted.ID)
				last := events[len(events)-1].Type
				if last != tt.wantEvent {
					t.Fatalf("last event = %q, want %q", last, tt.wantEvent)
				}
			}
		})
	}
}

// TestReconcileRefusesTheTokenItReplaced is the fence after a replacement: once
// a resume installs a fresh token, the expired execution's token can publish
// nothing at all, and the replacement can.
func TestReconcileRefusesTheTokenItReplaced(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-token.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileResume, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, deps, "runtime-b")
	if len(report.Resume) != 1 {
		t.Fatalf("resume executions = %d, want 1", len(report.Resume))
	}
	resumed := report.Resume[0]
	if resumed.ExecutionToken == execution.ExecutionToken {
		t.Fatal("the resumed execution kept the expired claim's token")
	}
	if resumed.Claimant != "runtime-b" {
		t.Fatalf("resumed claimant = %q, want the reconciling runtime", resumed.Claimant)
	}
	if resumed.Version <= execution.Version {
		t.Fatalf("resumed version = %d, want more than the expired execution's %d", resumed.Version, execution.Version)
	}

	stale := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}
	if _, err := svc.UpdateProgress(deps, stale, Progress{Phase: "still running"}); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("the replaced token published progress: %v", err)
	}
	if err := svc.AppendEvent(deps, stale, EventInput{Type: "checkpoint"}); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("the replaced token appended an event: %v", err)
	}
	live := ExecutionRef{JobID: accepted.ID, ExecutionToken: resumed.ExecutionToken}
	if _, err := svc.UpdateProgress(deps, live, Progress{Phase: "resumed"}); err != nil {
		t.Fatalf("the replacement token was refused: %v", err)
	}

	// The capacity the expired execution held belongs to the replacement now: it
	// is not re-admitted, and the replacement's own completion must free it
	// rather than leaving rows stamped with a token nobody owns.
	for _, lease := range capacityRows(t, deps, accepted.ID) {
		if lease.ExecutionToken != resumed.ExecutionToken {
			t.Fatalf("capacity lease for %s still carries token %q", lease.CapacityGroup, lease.ExecutionToken)
		}
	}
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef: live, ExpectedVersion: resumed.Version, Outcome: StateSucceeded,
	}); err != nil {
		t.Fatalf("Finish through the resumed execution: %v", err)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows after the resumed execution finished = %d, want none", count)
	}
}

// TestReconcileKeepsAClaimItCannotProveDead is §3's fail-safe: an adapter that
// cannot prove the external work stopped leaves the Job, its claim and its
// capacity exactly where they are, blocked and out of the expiry scan, so no
// replacement is ever dispatched over work that may still be running.
func TestReconcileKeepsAClaimItCannotProveDead(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-unproven.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileExternalWorkUnproven, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, deps, "runtime-b")
	if len(report.Outcomes) != 1 || report.Outcomes[0].Snapshot.State != StateBlocked {
		t.Fatalf("outcomes = %+v, want one blocked Job", report.Outcomes)
	}
	quarantined := jobRow(t, deps, accepted.ID)
	if State(quarantined.State).Terminal() {
		// Nonterminal is what keeps the Job out of ordinary retention and out of
		// every path that would drop its record; unresolved external work stays
		// visible until somebody resolves it.
		t.Fatalf("job state = %s, and quarantined work must stay nonterminal", quarantined.State)
	}

	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 2 {
		t.Fatalf("capacity rows = %d, want the quarantined claim's two budgets kept", count)
	}
	// A second pass must not ask the adapter about it again: the claim is out of
	// the expiry scan, so the blocked Job cannot become a reconciliation loop.
	second := reconcileOnce(t, svc, deps, "runtime-b")
	if second.Examined != 0 {
		t.Fatalf("a second pass examined %d claims, want none", second.Examined)
	}
	if adapter.reconciledCount() != 1 {
		t.Fatalf("the adapter was asked %d times, want once", adapter.reconciledCount())
	}
	if execution.ExecutionToken == "" {
		t.Fatal("no token was recorded for the quarantined claim")
	}
	// Nothing may be dispatched over it either.
	if _, ok := claimOnce(t, svc, deps, "runtime-c"); ok {
		t.Fatal("a blocked Job with an unresolved claim was dispatched")
	}
}

// TestReconcileBlocksAJobWhoseAdapterIsGone is the other fail-safe: a Kind this
// process cannot run has nobody to reconcile it, so the Job is blocked with its
// claim held rather than handed to another executor or assumed stopped.
func TestReconcileBlocksAJobWhoseAdapterIsGone(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-no-adapter.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	if _, ok := claimOnce(t, svc, deps, "runtime-a"); !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)

	// A process that no longer registers the Kind — a disabled plugin, a Kind
	// removed from this release — still has to reconcile what it finds.
	gone := NewService()
	report := reconcileOnce(t, gone, deps, "runtime-b")
	if report.Examined != 1 || len(report.Outcomes) != 1 {
		t.Fatalf("report = %+v, want one examined and applied claim", report)
	}
	if report.Outcomes[0].Decision != ReconcileExternalWorkUnproven {
		t.Fatalf("decision = %q, want the unproven answer for a missing adapter", report.Outcomes[0].Decision)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want blocked", stored.State)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}
}

// TestReconcileNeverRerunsNonRestorableWorkOnExpiryAlone holds ADR 0006's
// closure-backed rule at the control plane: even an adapter that asks for a
// rerun cannot get one for work whose in-memory state died with its process.
func TestReconcileNeverRerunsNonRestorableWorkOnExpiryAlone(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-non-restorable.db")
	svc := NewService()
	definition := testDefinition()
	definition.Restorable = false
	adapter := registerTestAdapter(t, svc, definition)
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileQueue, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	if _, ok := claimOnce(t, svc, deps, "runtime-a"); !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, deps, "runtime-b")
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != ReconcileExternalWorkUnproven {
		t.Fatalf("report = %+v, want the unproven answer", report.Outcomes)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s: closure-backed work must not be re-queued on expiry alone", stored.State)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}
}

// TestReconcileAppliesNothingWhenTheAdapterCannotAnswer keeps an adapter that
// failed out of the Job record: nothing is applied on its behalf, and the claim
// stays reconcileable for the next pass.
func TestReconcileAppliesNothingWhenTheAdapterCannotAnswer(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-failed-adapter.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return "", errors.New("the reconciler could not reach its own database")
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	if _, ok := claimOnce(t, svc, deps, "runtime-a"); !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, deps, "runtime-b")
	if len(report.Outcomes) != 0 {
		t.Fatalf("applied %+v from an adapter that answered nothing", report.Outcomes)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateRunning) {
		t.Fatalf("job state = %s, want running", stored.State)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateHeld {
		t.Fatalf("claim state = %s, want held for the next pass", claim.State)
	}
}

// TestReconcileBlocksADecisionItCannotApply is the refusal half of the decision
// vocabulary: a success asserted without its required output is not a success,
// and the Job becomes visibly blocked rather than finishing untruthfully or
// being retried forever.
func TestReconcileBlocksADecisionItCannotApply(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-refused.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileSucceed, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	// The execution promised a required artifact and never published it.
	if _, err := svc.PublishOutput(deps, ExecutionRef{
		JobID: accepted.ID, ExecutionToken: execution.ExecutionToken,
	}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Required: true,
		Reference: json.RawMessage(`{"name":"export.tar"}`),
	}); err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}
	if err := deps.DB.Model(&models.JobOutput{}).Where("job_id = ?", accepted.ID).
		Update("availability", "removed").Error; err != nil {
		t.Fatalf("remove the promised artifact: %v", err)
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, deps, "runtime-b")
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != ReconcileBlock {
		t.Fatalf("outcomes = %+v, want a single block", report.Outcomes)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want blocked: a success without its required output is not a success", stored.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows = %d, want the refused claim released", count)
	}
}

// TestClaimBlocksAJobWhoseInputCannotBeOpened is the dispatch half of the replay
// contract: work accepted with sealed input that this process cannot open cannot
// be run, so the Job is blocked — under the claim's own token — rather than left
// running under an execution that never started.
func TestClaimBlocksAJobWhoseInputCannotBeOpened(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-input.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	registerTestCodec(t, svc)

	sealing := Deps{DB: deps.DB, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "the-key-that-sealed-it")}}
	accepted, err := svc.Accept(sealing, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"sealed"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}

	// A process holding a different key — a rotation not yet rolled out, a key
	// file lost with the data root — cannot open it, and must not run the Job
	// with half of its input.
	claiming := Deps{DB: deps.DB, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "a-different-key")}}
	_, ok, err := svc.Claim(context.Background(), claiming, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
	})
	if ok {
		t.Fatal("a Job whose input cannot be opened was handed to an execution")
	}
	if !errors.Is(err, ErrReplayKeyUnavailable) {
		t.Fatalf("Claim = %v, want ErrReplayKeyUnavailable", err)
	}

	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want blocked", stored.State)
	}
	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released with the Job it could not run", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows = %d, want the unrunnable claim released", count)
	}
	events := jobEvents(t, deps, accepted.ID)
	if last := events[len(events)-1]; last.Type != EventBlocked {
		t.Fatalf("last event = %q, want the block", last.Type)
	}
}

// TestAQuarantinedClaimIsReleasedWhenItsOwnerFinishesTheJob is the exception
// that proves the quarantine rule: a claim nobody could prove anything about is
// kept, and the one thing that does prove something is its own execution — which
// can still publish, because the Job kept its token — reaching an outcome. Then
// the claim and the capacity go.
func TestAQuarantinedClaimIsReleasedWhenItsOwnerFinishesTheJob(t *testing.T) {
	_, deps := newDispatchDatabase(t, "quarantine-released.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileExternalWorkUnproven, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)
	reconcileOnce(t, svc, deps, "runtime-b")
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}

	// The execution it was quarantined for was alive after all, and reports an
	// outcome. A quarantined Job is blocked, and the state machine admits no
	// blocked -> succeeded edge, so the outcome an owner can record here is a
	// failure (or a re-queue): either way it is the execution saying what became
	// of its own work, which is the proof the quarantine was missing.
	blocked := jobRow(t, deps, accepted.ID)
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: blocked.Version,
		Outcome:         StateFailed,
		Failure:         &Failure{Code: "work-abandoned", Class: FailureClassCancellation},
	}); err != nil {
		t.Fatalf("Finish by the quarantined claim's own execution: %v", err)
	}

	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released once its execution proved itself", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows = %d, want the finished Job's capacity freed", count)
	}
}

// TestReconcileRefusesADecisionOutsideTheVocabulary keeps a buggy adapter from
// deciding anything: an answer this release does not know leaves its Job alone,
// is reported to the caller, and does not stop the pass from reconciling the
// claims behind it.
func TestReconcileRefusesADecisionOutsideTheVocabulary(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-unknown.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(_ context.Context, request ReconcileRequest) (ReconcileDecision, error) {
		if request.Snapshot.ID == "" {
			t.Error("the adapter was asked about no job")
		}
		return ReconcileDecision("merge"), nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	first := acceptQueued(t, svc, deps, nil)
	second := acceptQueued(t, svc, deps, nil)
	// Claiming is oldest-first by acceptance, and two Jobs accepted inside one
	// test clock tick are ordered by their UUIDv7 tiebreak — which is not the
	// order they were accepted in. The claim's own Job id is the one to age.
	for range []Snapshot{first, second} {
		execution, ok := claimOnce(t, svc, deps, "runtime-a")
		if !ok {
			t.Fatal("a queued Job was not claimed")
		}
		expireClaim(t, deps, execution.JobID, clock)
	}

	report, err := svc.ReconcileExpired(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if !errors.Is(err, ErrInvalidReconcileDecision) {
		t.Fatalf("ReconcileExpired = %v, want ErrInvalidReconcileDecision", err)
	}
	if report.Examined != 2 {
		t.Fatalf("examined %d claims, want both", report.Examined)
	}
	if len(report.Outcomes) != 0 {
		t.Fatalf("applied %+v from an unknown decision", report.Outcomes)
	}
	for _, job := range []Snapshot{first, second} {
		if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) {
			t.Fatalf("job %s is %s, want running and untouched", job.ID, stored.State)
		}
		if claim := claimRow(t, deps, job.ID); claim.State != models.JobClaimStateHeld {
			t.Fatalf("job %s's claim is %s, want held for the next pass", job.ID, claim.State)
		}
	}
	if adapter.reconciledCount() != 2 {
		t.Fatalf("the adapter was asked %d times, want both claims", adapter.reconciledCount())
	}
}

// TestFinishingIsFencedByTheClaimTheExecutionStillHolds is the atomic half of
// the execution fence. The token was checked before the lifecycle transaction
// opened but not inside its UPDATE predicate, and ReleaseClaim clears the token
// without touching the state or the version — so a transition decided while the
// claim was still held could commit after that claim had been handed back.
//
// The check has to be in the same statement as the write, or the gap between the
// read and the write is exactly where a released claim lands.
func TestFinishingIsFencedByTheClaimTheExecutionStillHolds(t *testing.T) {
	_, deps := newDispatchDatabase(t, "finish-after-release.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

	// What an executor has in hand when it decides to finish: the version and
	// state it read after its own claim.
	decidedFrom := jobRow(t, deps, accepted.ID)
	if decidedFrom.State != string(StateRunning) {
		t.Fatalf("state after the claim = %s, want running", decidedFrom.State)
	}
	transition := Transition{
		JobID: accepted.ID, ExpectedVersion: decidedFrom.Version, ExecutionToken: execution.ExecutionToken,
		To: StateFailed, Failure: &Failure{Code: "gave-up", Class: FailureClassInternal},
	}

	// The interleaving: the decision is taken, and the claim is handed back
	// before the write lands. A release moves neither the version nor the state,
	// so the guarded update alone cannot tell the difference.
	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		t.Fatalf("prepareTransition: %v", err)
	}
	if _, err := svc.ReleaseClaim(deps, ref, ReleaseReasonExecutionEnded); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if released := jobRow(t, deps, accepted.ID); released.Version != decidedFrom.Version || released.State != decidedFrom.State {
		t.Fatalf("release changed the row to v%d %s; the fence is the whole point of it not doing that",
			released.Version, released.State)
	}

	if _, err := svc.commitTransition(deps, prepared, nil); err == nil {
		t.Fatal("a transition decided before the claim was released committed after it")
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateRunning) {
		t.Fatalf("state = %s, want the Job left where the released execution found it", stored.State)
	}

	// The same write through the public seam, where the token check before the
	// transaction is what refuses it: nothing is written and nothing is appended.
	_, err = svc.Finish(deps, FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: decidedFrom.Version,
		Outcome:         StateFailed,
		Failure:         &Failure{Code: "gave-up", Class: FailureClassInternal},
	})
	if !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("Finish after the claim was released = %v, want ErrStaleExecution", err)
	}
	if events := jobEvents(t, deps, accepted.ID); len(events) != 2 {
		t.Fatalf("timeline has %d events, want only acceptance and the started event", len(events))
	}
}

// TestHeartbeatKeepsTheLeaseNearTheLastHeartbeatRatherThanAccumulatingIt covers
// the recovery delay a long-running execution accumulates.
//
// The extension used to be measured from the *later of* now and the stored
// expiry and then added to it, so every healthy heartbeat banked another whole
// lease: an hour-long execution with two-minute leases ended up with an expiry
// about three hours out. A claim whose runtime then died was not reconcilable at
// its lease — it waited for all of the banked time, and the capacity it held was
// frozen for just as long.
func TestHeartbeatKeepsTheLeaseNearTheLastHeartbeatRatherThanAccumulatingIt(t *testing.T) {
	_, deps := newDispatchDatabase(t, "heartbeat-drift.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileQueue, nil
	}
	clock := time.Date(2033, 9, 10, 11, 12, 13, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

	// An hour of healthy heartbeats at the cadence the runtime uses: one every
	// third of the lease.
	lease := testDefinition().EffectiveLease()
	for heartbeats := 0; heartbeats < 180; heartbeats++ {
		clock = clock.Add(lease / 3)
		if err := svc.Heartbeat(deps, ref, lease); err != nil {
			t.Fatalf("Heartbeat %d: %v", heartbeats, err)
		}
	}

	// The runtime dies here. What it leaves behind must become reconcilable one
	// lease after its last heartbeat, however long it had been running.
	abandoned := clock
	claim := claimRow(t, deps, accepted.ID)
	if claim.LeaseExpiresAt.After(abandoned.Add(lease)) {
		t.Fatalf("after an hour of heartbeats the lease runs to %v, %v past the last heartbeat: "+
			"every heartbeat banked another lease", claim.LeaseExpiresAt, claim.LeaseExpiresAt.Sub(abandoned))
	}

	clock = claim.LeaseExpiresAt.Add(time.Second)
	report, err := svc.ReconcileExpired(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileExpired: %v", err)
	}
	if report.Examined != 1 {
		t.Fatalf("examined %d expired claims one lease after the last heartbeat, want the abandoned one",
			report.Examined)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateQueued) {
		t.Fatalf("state after reconciliation = %s, want queued", stored.State)
	}
}

// TestKindCapacityGroupCollisionCannotDiscardTheDeploymentBudget is §3's
// "deployment-wide global or per-Kind concurrency limits use database-backed
// accounting" read as a property that survives a Kind naming the deployment's own
// group.
//
// A runtime asks every claim for the deployment-wide budget on top of the Kind's
// own, and the two are the same group whenever a Kind declares
// `CapacityGroup: "global"`. Install-first deduplication then silently discarded
// the runtime's budget: a Kind declaring the group with no limit of its own left
// the global budget unenforced altogether, and one declaring a larger limit
// replaced it. The merge is the strictest positive limit instead, because a limit
// of zero means "not enforced" rather than "unlimited".
func TestKindCapacityGroupCollisionCannotDiscardTheDeploymentBudget(t *testing.T) {
	cases := []struct {
		name        string
		kindLimit   int
		deployLimit int
		wantHeld    int64
	}{
		{
			name: "a Kind declaring the global group with no limit of its own",
			// The Kind's own budget says nothing; the deployment's one slot is what
			// applies.
			kindLimit: 0, deployLimit: 1, wantHeld: 1,
		},
		{
			name:        "a Kind declaring a larger limit in the global group",
			kindLimit:   5,
			deployLimit: 1,
			wantHeld:    1,
		},
		{
			name:        "a Kind declaring a stricter limit in the global group",
			kindLimit:   1,
			deployLimit: 5,
			wantHeld:    1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, deps := newDispatchDatabase(t, "capacity-collision.db")
			deps.Now = func() time.Time { return time.Date(2033, 8, 9, 10, 11, 12, 0, time.UTC) }
			definition := Definition{
				Kind: testKind, KindVersion: 1, Restorable: true,
				CapacityGroup: CapacityGroupGlobal, MaxConcurrent: testCase.kindLimit,
			}
			first, second := NewService(), NewService()
			registerTestAdapter(t, first, definition)
			registerTestAdapter(t, second, definition)
			acceptQueued(t, first, deps, uintPtr(4))
			acceptQueued(t, first, deps, uintPtr(4))

			budget := CapacityRef{Group: CapacityGroupGlobal, Limit: testCase.deployLimit}
			if _, ok := claimOnce(t, first, deps, "runtime-a", budget); !ok {
				t.Fatal("the first claim of a queued Job was refused")
			}
			if _, ok := claimOnce(t, second, deps, "runtime-b", budget); ok {
				t.Fatalf("a second execution was admitted into the %s budget: the Kind's declaration "+
					"discarded the deployment-wide limit", CapacityGroupGlobal)
			}
			if held := capacityCount(t, deps, CapacityGroupGlobal); held != testCase.wantHeld {
				t.Fatalf("capacity rows in %s = %d, want %d", CapacityGroupGlobal, held, testCase.wantHeld)
			}
		})
	}
}

// TestReconcileUnrunnableBlocksPendingWorkNoAdapterCanRun is §2/§3's "if an
// adapter disappears after a deploy or plugin disablement ... nonterminal work
// becomes `blocked`. The system never falls back to a different executor."
//
// Reconciliation scans held claims, so work that is *waiting* — queued, or
// scheduled for a time that has come — has no claim to expire and no registration
// to be visited through. Without a pass of its own it stays queued forever, which
// is the one state nobody is asked about: it is not running, nothing owns it, and
// no executor here can ever run it. Blocking it is the honest answer, and it takes
// no execution capacity, because nothing is executing.
func TestReconcileUnrunnableBlocksPendingWorkNoAdapterCanRun(t *testing.T) {
	_, deps := newDispatchDatabase(t, "unrunnable.db")
	svc := NewService()
	clock := time.Date(2033, 9, 10, 11, 12, 13, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	registerTestAdapter(t, svc, Definition{Kind: "runnable-kind", KindVersion: 1, Restorable: true})

	acceptKind := func(kind string, state State, scheduledFor *time.Time) Snapshot {
		t.Helper()
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: state, Origin: "api",
			ScheduledFor: scheduledFor, OwnerUserID: uintPtr(4),
			Replay: ReplayInput{NonReplayable: true},
		})
	}

	queued := acceptKind("gone-kind", StateQueued, nil)
	due := clock.Add(time.Hour)
	scheduled := acceptKind("gone-kind", StateScheduled, &due)
	// The Kind this process *can* run is none of this pass's business, whatever
	// state its work is in.
	runnable := acceptKind("runnable-kind", StateQueued, nil)

	report, err := svc.ReconcileUnrunnable(deps, DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileUnrunnable: %v", err)
	}
	if report.Examined != 2 {
		t.Fatalf("examined %d Jobs, want the two of the Kind this process cannot run", report.Examined)
	}

	for _, job := range []Snapshot{queued, scheduled} {
		stored := jobRow(t, deps, job.ID)
		if stored.State != string(StateBlocked) {
			t.Errorf("job %s of a Kind this process cannot run is %s, want blocked", job.ID, stored.State)
		}
		if events := jobEvents(t, deps, job.ID); len(events) == 0 || events[len(events)-1].Type != EventBlocked {
			t.Errorf("job %s has no blocked event on its timeline", job.ID)
		}
		if claims := countRows(t, deps, &models.JobClaim{}, "job_id = ?", job.ID); claims != 0 {
			t.Errorf("blocking job %s took a claim", job.ID)
		}
		if leases := countRows(t, deps, &models.JobCapacityLease{}, "job_id = ?", job.ID); leases != 0 {
			t.Errorf("blocking job %s took %d capacity slots: nothing is executing", job.ID, leases)
		}
	}
	if stored := jobRow(t, deps, runnable.ID); stored.State != string(StateQueued) {
		t.Fatalf("a queued Job of a Kind this process can run became %s", stored.State)
	}

	// The pass is idempotent: once those Jobs are blocked there is nothing left
	// for it to look at, and a repeated pass cannot grow timelines.
	second, err := svc.ReconcileUnrunnable(deps, DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("second ReconcileUnrunnable: %v", err)
	}
	if second.Examined != 0 || len(second.Outcomes) != 0 {
		t.Fatalf("the second pass examined %d Jobs and applied %d decisions, want none",
			second.Examined, len(second.Outcomes))
	}
}
