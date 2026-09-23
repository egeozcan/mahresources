package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
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

	advertise func(context.Context, CommandContext) ([]Command, error)
	execute   func(context.Context, CommandExecution) (CommandOutcome, error)

	mu         sync.Mutex
	dispatched []Execution
	reconciled []ReconcileRequest
	cleanups   []ArtifactCleanupRequest
	advertised []CommandContext
	commands   []CommandExecution
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

func (a *testAdapter) Commands(ctx context.Context, commandContext CommandContext) ([]Command, error) {
	a.mu.Lock()
	a.advertised = append(a.advertised, commandContext)
	a.mu.Unlock()
	if a.advertise != nil {
		return a.advertise(ctx, commandContext)
	}
	return nil, nil
}

func (a *testAdapter) ExecuteCommand(ctx context.Context, execution CommandExecution) (CommandOutcome, error) {
	a.mu.Lock()
	a.commands = append(a.commands, execution)
	a.mu.Unlock()
	if a.execute != nil {
		return a.execute(ctx, execution)
	}
	return CommandOutcome{Status: CommandStatusSucceeded}, nil
}

// commandCount is how many commands the adapter was asked to run, which is what
// an idempotency assertion counts: a repeat must not reach the adapter at all.
func (a *testAdapter) commandCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.commands)
}

// lastCommand is the execution the adapter was last handed.
func (a *testAdapter) lastCommand() CommandExecution {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.commands) == 0 {
		return CommandExecution{}
	}
	return a.commands[len(a.commands)-1]
}

// advertisementCount is how many times the adapter was asked what a Job offers.
func (a *testAdapter) advertisementCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.advertised)
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
		&models.JobPreference{}, &models.JobPinGuard{}, &models.JobLegacyHandle{},
		&models.JobCommandRequest{},
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

// capacitySlots lists the slot numbers one capacity group's leases occupy,
// ascending: which slots a budget holds is what a limit is enforced against.
func capacitySlots(t *testing.T, deps Deps, group string) []int {
	t.Helper()
	var slots []int
	if err := deps.DB.Model(&models.JobCapacityLease{}).Where("capacity_group = ?", group).
		Order("slot").Pluck("slot", &slots).Error; err != nil {
		t.Fatalf("load capacity slots for %s: %v", group, err)
	}
	return slots
}

// finishCancelled ends a claimed Job as cancelled, which is how a test gives
// back the capacity an execution held without leaving the Job claimable again.
func finishCancelled(t *testing.T, svc *Service, deps Deps, jobID, token string) {
	t.Helper()
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: jobID, ExecutionToken: token},
		ExpectedVersion: jobRow(t, deps, jobID).Version,
		Outcome:         StateCancelled,
	}); err != nil {
		t.Fatalf("Finish(cancelled) %s: %v", jobID, err)
	}
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

// TestClaimCountsTheSlotsOutsideAReducedBudget is this correction's regression:
// admission asked only whether a slot below the *current* limit was free, so an
// execution admitted under a wider limit and still holding a higher-numbered
// slot was invisible to it. Lowering the deployment's limit then admitted one
// execution more than the new limit allows.
//
// The budget here is the whole of it: the Kind declares none of its own, so the
// occupancy that matters is the deployment-wide group's.
func TestClaimCountsTheSlotsOutsideAReducedBudget(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-reduced-budget.db")
	svc := NewService()
	registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})

	// The two Jobs are accepted a second apart so the candidate order — oldest
	// accepted, id breaking the tie — hands them over in the order they were made.
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	wide := CapacityRef{Group: CapacityGroupGlobal, Limit: 2}
	first := acceptQueued(t, svc, deps, nil)
	clock = clock.Add(time.Second)
	second := acceptQueued(t, svc, deps, nil)
	firstExecution, ok := claimOnce(t, svc, deps, "runtime-a", wide)
	if !ok {
		t.Fatal("the first Job was refused a budget of two with nothing in it")
	}
	if _, ok := claimOnce(t, svc, deps, "runtime-a", wide); !ok {
		t.Fatal("the second Job was refused the budget's second slot")
	}
	if firstExecution.JobID != first.ID {
		t.Fatalf("the first claim took %s, want the oldest accepted Job %s", firstExecution.JobID, first.ID)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{0, 1}) {
		t.Fatalf("slots occupied in %s = %v, want 0 and 1", CapacityGroupGlobal, slots)
	}

	// The execution holding the lower slot ends, so the budget holds one
	// execution — in slot 1, which is outside the range a limit of one admits.
	finishCancelled(t, svc, deps, first.ID, firstExecution.ExecutionToken)
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after one execution ended = %v, want only slot 1",
			CapacityGroupGlobal, slots)
	}

	third := acceptQueued(t, svc, deps, nil)
	if execution, claimed := claimOnce(t, svc, deps, "runtime-b",
		CapacityRef{Group: CapacityGroupGlobal, Limit: 1}); claimed {
		t.Fatalf("Job %s was admitted while %s already held its one execution in slot 1: an occupied "+
			"slot outside the reduced range must count against the limit", execution.JobID, CapacityGroupGlobal)
	}
	if stored := jobRow(t, deps, third.ID); stored.State != string(StateQueued) || stored.ExecutionToken != "" {
		t.Fatalf("the refused Job is %s with token %q, want queued and unowned", stored.State, stored.ExecutionToken)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after the refusal = %v, want only the held slot 1",
			CapacityGroupGlobal, slots)
	}
	if stored := jobRow(t, deps, second.ID); stored.State != string(StateRunning) {
		t.Fatalf("the held Job is %s, want still running", stored.State)
	}
}

// TestClaimCountsAQuarantinedSlotOutsideAReducedBudget is the other half of the
// same rule: a claim nobody could prove anything about keeps its capacity while
// it stays unresolved (§3), and that capacity is occupancy like any other, so it
// counts against a limit lowered underneath it.
func TestClaimCountsAQuarantinedSlotOutsideAReducedBudget(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-reduced-budget-quarantined.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileExternalWorkUnproven, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// Accepted a second apart, so the candidate order hands them over in the order
	// they were made and the quarantined claim is the one in the higher slot.
	wide := CapacityRef{Group: CapacityGroupGlobal, Limit: 2}
	first := acceptQueued(t, svc, deps, nil)
	clock = clock.Add(time.Second)
	quarantined := acceptQueued(t, svc, deps, nil)
	firstExecution, ok := claimOnce(t, svc, deps, "runtime-a", wide)
	if !ok {
		t.Fatal("the first Job was refused a budget of two with nothing in it")
	}
	if firstExecution.JobID != first.ID {
		t.Fatalf("the first claim took %s, want the oldest accepted Job %s", firstExecution.JobID, first.ID)
	}
	if _, ok := claimOnce(t, svc, deps, "runtime-a", wide); !ok {
		t.Fatal("the second Job was refused the budget's second slot")
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{0, 1}) {
		t.Fatalf("slots occupied in %s = %v, want 0 and 1", CapacityGroupGlobal, slots)
	}

	// The second execution's runtime disappears without proving anything, so its
	// claim is quarantined and its slot stays occupied.
	expireClaim(t, deps, quarantined.ID, clock)
	reconcileOnce(t, svc, deps, "runtime-b")
	if claim := claimRow(t, deps, quarantined.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}

	// The first execution ends, so the budget holds one execution — the
	// quarantined one, in slot 1.
	finishCancelled(t, svc, deps, first.ID, firstExecution.ExecutionToken)
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after one execution ended = %v, want only the quarantined slot 1",
			CapacityGroupGlobal, slots)
	}

	third := acceptQueued(t, svc, deps, nil)
	if execution, claimed := claimOnce(t, svc, deps, "runtime-c",
		CapacityRef{Group: CapacityGroupGlobal, Limit: 1}); claimed {
		t.Fatalf("Job %s was admitted while a quarantined claim held %s's only execution in slot 1",
			execution.JobID, CapacityGroupGlobal)
	}
	if stored := jobRow(t, deps, third.ID); stored.State != string(StateQueued) {
		t.Fatalf("the refused Job is %s, want queued", stored.State)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after the refusal = %v, want only the quarantined slot 1",
			CapacityGroupGlobal, slots)
	}
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
	want := []string{EventAccepted, EventStarted, "checkpoint", EventOutputPublished, EventSucceeded}
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

// TestQuiescedReleaseMovesTheJobAndClaimsItBack is the restart contract: a runtime
// that stops running an execution hands the Job back in a state the next process can
// pick up, and the durable evidence of what it held is freed rather than left
// occupying a budget.
//
// The state is the adapter's decision and it is written with the release, in one
// transaction. Two writes would leave the instant in between — a Job that is running
// with no token — and a Job in that state is neither claimable (Claim takes queued
// or scheduled work) nor reconciled (the expiry scan looks for held claims): work
// stranded by a graceful stop, which is the one thing a graceful stop must not do.
func TestQuiescedReleaseMovesTheJobAndClaimsItBack(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}

	released, err := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		Reason:       ReleaseReasonExecutionEnded,
		To:           StateQueued,
	})
	if err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if released.State != StateQueued {
		t.Fatalf("released Job is %s, want the state the adapter decided: queued", released.State)
	}

	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released", claim.State)
	}
	if claim.ReleasedAt == nil || !claim.ReleasedAt.Equal(clock) {
		t.Fatalf("released_at = %v, want %v", claim.ReleasedAt, clock)
	}
	if claim.ReleaseReason != ReleaseReasonExecutionEnded {
		t.Fatalf("release reason = %q, want the reason the releasing runtime gave", claim.ReleaseReason)
	}
	if leases := capacityRows(t, deps, accepted.ID); len(leases) != 0 {
		t.Fatalf("capacity leases after release = %d, want none", len(leases))
	}
	if stored := jobRow(t, deps, accepted.ID); stored.ExecutionToken != "" {
		t.Fatalf("job token after release = %q, want cleared", stored.ExecutionToken)
	}

	// The next process is a new Service over the same database: it reconciles what
	// nobody owns and claims what is waiting. Nothing here needs the abandoned
	// runtime to answer, which is the property a graceful handoff has to have.
	reconciled, err := svc.ReconcileExpired(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileExpired: %v", err)
	}
	if len(reconciled.Resume) != 0 || reconciled.Examined != 0 {
		t.Fatalf("a released claim was reconciled: %+v", reconciled)
	}

	clock = clock.Add(time.Minute)
	second := NewService()
	registerTestAdapter(t, second, testDefinition())
	resumed, ok := claimOnce(t, second, deps, "runtime-b")
	if !ok {
		t.Fatal("the Job a runtime handed back was not claimable by the next one")
	}
	if resumed.JobID != accepted.ID || resumed.ExecutionToken == execution.ExecutionToken {
		t.Fatalf("the second claim = %+v, want the same Job under a fresh token", resumed)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateRunning) {
		t.Fatalf("re-claimed Job is %s, want running", stored.State)
	}

	// A release under a token that owns nothing stays what it always was: a no-op,
	// because releasing is idempotent and a runtime must be able to report what it
	// already reported. The token the abandoned runtime held is not the one that
	// owns the Job any more, so it cannot take the Job away from its successor.
	if _, err := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		Reason:       ReleaseReasonExecutionEnded,
	}); err != nil {
		t.Fatalf("second ReleaseClaim: %v", err)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateRunning) ||
		stored.ExecutionToken != resumed.ExecutionToken {
		t.Fatalf("a release under a token that owns nothing moved the Job to %s token %q",
			stored.State, stored.ExecutionToken)
	}
}

// TestReleaseClaimRefusesToStrandARunningJob is the other half of the same rule.
//
// A release that names no state leaves a running Job running with no token, no claim
// and no capacity: nothing can claim it, no reconciliation reaches it, and the work
// it was doing is invisible to every path that could resolve it. So the release is
// refused and nothing is written — the Job keeps its execution until the runtime
// says where it goes.
func TestReleaseClaimRefusesToStrandARunningJob(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 7, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	decidedFrom := jobRow(t, deps, accepted.ID)
	admitted := capacityRows(t, deps, accepted.ID)
	if len(admitted) == 0 {
		t.Fatal("the claim admitted the Job without occupying any capacity")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

	_, err := svc.ReleaseClaim(deps, ReleaseRequest{ExecutionRef: ref, Reason: ReleaseReasonExecutionEnded})
	if !errors.Is(err, ErrReleaseNeedsState) {
		t.Fatalf("release of a running execution = %v, want ErrReleaseNeedsState", err)
	}

	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateRunning) || stored.Version != decidedFrom.Version || stored.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("a refused release wrote %s v%d token %q", stored.State, stored.Version, stored.ExecutionToken)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateHeld || claim.ReleasedAt != nil {
		t.Fatalf("a refused release changed the claim: %+v", claim)
	}
	if leases := capacityRows(t, deps, accepted.ID); len(leases) != len(admitted) {
		t.Fatalf("a refused release freed the capacity that admitted the Job: %d leases, want %d",
			len(leases), len(admitted))
	}
}

// TestReleaseClaimNeverEndsAJob is the other half of the same rule, and it is
// §7's contract read at this seam.
//
// A release that could name an end state would be a terminal transition reached
// around everything that makes one honest: the failure taxonomy a failed Job must
// record, and the required outputs a success is verified against. An execution
// holding a required artifact it cannot serve — its deadline had already passed
// when it was published — could release its way to succeeded, which is exactly the
// invariant Finish and Transition both enforce and the one entry point that could
// not. Ending a Job is Finish's decision, and a release names a nonterminal state
// the Job is left in for whoever comes next.
func TestReleaseClaimNeverEndsAJob(t *testing.T) {
	for _, terminal := range []State{StateSucceeded, StateFailed, StateCancelled, StateInterrupted} {
		t.Run(string(terminal), func(t *testing.T) {
			deps := newTestDeps(t)
			svc := NewService()
			registerTestAdapter(t, svc, testDefinition())
			clock := time.Date(2033, 5, 8, 7, 8, 9, 0, time.UTC)
			deps.Now = func() time.Time { return clock }

			accepted := acceptQueued(t, svc, deps, nil)
			execution, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 1})
			if !ok {
				t.Fatal("the queued Job was not claimed")
			}
			ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}

			// A required artifact whose deadline had already passed when it was
			// published: the shape §7 refuses to call a success.
			past := clock.Add(-time.Hour)
			if _, err := svc.PublishOutput(deps, ref, OutputInput{
				Key: "artifact", Type: OutputTypeArtifact, Required: true,
				Reference: json.RawMessage(`{"path":"exports/late.tar"}`), ExpiresAt: &past,
			}); err != nil {
				t.Fatalf("PublishOutput: %v", err)
			}

			decidedFrom := jobRow(t, deps, accepted.ID)
			events := len(jobEvents(t, deps, accepted.ID))
			admitted := capacityRows(t, deps, accepted.ID)

			_, err := svc.ReleaseClaim(deps, ReleaseRequest{
				ExecutionRef: ref, Reason: ReleaseReasonExecutionEnded, To: terminal,
			})
			if !errors.Is(err, ErrReleaseTerminalState) {
				t.Fatalf("release naming %s = %v, want ErrReleaseTerminalState", terminal, err)
			}

			// Nothing moved: not the state, not the timeline, not the claim, not the
			// capacity — which is what makes the refusal a refusal rather than an ending
			// that happened to be reported.
			stored := jobRow(t, deps, accepted.ID)
			if stored.State != string(StateRunning) || stored.Version != decidedFrom.Version ||
				stored.ExecutionToken != execution.ExecutionToken {
				t.Fatalf("a refused release left the Job %s v%d token %q", stored.State, stored.Version, stored.ExecutionToken)
			}
			if got := len(jobEvents(t, deps, accepted.ID)); got != events {
				t.Fatalf("a refused release recorded %d events, want the %d that were there", got, events)
			}
			if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateHeld || claim.ReleasedAt != nil {
				t.Fatalf("a refused release changed the claim: %+v", claim)
			}
			if leases := capacityRows(t, deps, accepted.ID); len(leases) != len(admitted) {
				t.Fatalf("a refused release freed the capacity that admitted the Job: %d leases, want %d",
					len(leases), len(admitted))
			}

			// The decision is still available where it belongs, with the contract this
			// Job cannot satisfy: the same success is refused, and the failure that does
			// end it carries a taxonomy. Only the artifact's bytes survive either way —
			// nothing here deleted what the output still promises.
			if _, err := svc.Finish(deps, FinishRequest{
				ExecutionRef: ref, ExpectedVersion: decidedFrom.Version, Outcome: StateSucceeded,
			}); !errors.Is(err, ErrRequiredOutputUnavailable) {
				t.Fatalf("Finish as succeeded over an exhausted required artifact = %v, want ErrRequiredOutputUnavailable", err)
			}
			failed, err := svc.Finish(deps, FinishRequest{
				ExecutionRef: ref, ExpectedVersion: decidedFrom.Version, Outcome: StateFailed,
				Failure: &Failure{Code: "gave-up", Class: FailureClassInternal},
			})
			if err != nil {
				t.Fatalf("Finish as failed: %v", err)
			}
			if failed.State != StateFailed {
				t.Fatalf("finished Job is %s, want failed", failed.State)
			}
			if stored := jobRow(t, deps, accepted.ID); stored.FailureCode != "gave-up" {
				t.Fatalf("failure code = %q, want the taxonomy the outcome records", stored.FailureCode)
			}
			var artifact models.JobOutput
			if err := deps.DB.Where("job_id = ? AND key = ?", accepted.ID, "artifact").First(&artifact).Error; err != nil {
				t.Fatalf("read the artifact: %v", err)
			}
			if artifact.RemovedAt != nil {
				t.Fatal("ending the Job removed the artifact it published")
			}
		})
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

// TestAWriteDecidedBeforeTheReleaseCannotCommitAfterIt is the atomic half of the
// execution fence, under the interleaving a release leaves: an execution decides how
// its Job ends, and its runtime quiesces that execution — releasing the claim and
// leaving the Job in the state the adapter decided — before the write lands.
//
// The write may not commit. The Job it was decided from is not the Job that is there
// any more, and an execution that no longer owns a Job does not get to publish an
// outcome into it: the guard carries the state, the version and the token the caller
// read, and the check has to be in the same statement as the write, or the gap
// between the read and the write is exactly where a released claim lands.
func TestAWriteDecidedBeforeTheReleaseCannotCommitAfterIt(t *testing.T) {
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

	// The interleaving: the decision is taken, and the runtime releases the
	// execution — handing the Job back into the queue for the next process —
	// before the write lands.
	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		t.Fatalf("prepareTransition: %v", err)
	}
	released, err := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ref, Reason: ReleaseReasonExecutionEnded, To: StateQueued,
	})
	if err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if released.State != StateQueued {
		t.Fatalf("released Job is %s, want queued", released.State)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateQueued) || stored.ExecutionToken != "" {
		t.Fatalf("released Job = %s token %q, want the decided state with no execution owning it",
			stored.State, stored.ExecutionToken)
	}

	if _, err := svc.commitTransition(deps, prepared, nil); err == nil {
		t.Fatal("a transition decided before the release committed after it")
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateQueued) {
		t.Fatalf("state = %s, want the Job left where the release put it", stored.State)
	}

	// The same write through the public seam, in the shape an executor that re-read
	// the Job would send it: the version it names is the current one, so what
	// refuses it is the token — the execution that was released does not own the Job
	// any more, whatever it thinks it knows about it.
	_, err = svc.Finish(deps, FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: released.Version,
		Outcome:         StateFailed,
		Failure:         &Failure{Code: "gave-up", Class: FailureClassInternal},
	})
	if !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("Finish after the claim was released = %v, want ErrStaleExecution", err)
	}
	if events := jobEvents(t, deps, accepted.ID); len(events) != 3 {
		t.Fatalf("timeline has %d events, want acceptance, the started event and the release's queueing", len(events))
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

// TestReconcileExpiredReachesClaimsBehindAFullBatchOfFailures is the fairness
// contract of a bounded pass: a batch of claims whose adapters cannot answer must
// not be the only claims any pass ever looks at.
//
// The scan reads the oldest expired claims up to its limit and nothing else, and a
// claim nothing was applied to stays exactly where it was — so a Kind whose
// reconciler keeps failing monopolized every pass: the claims behind it, including
// one whose adapter could decide it at once, were never examined, however long the
// deployment ran. A claim the pass could not decide is now deferred to a later
// instant and the scan is ordered by when each claim is next due, so the work
// behind a full batch of failures is reached on the very next pass — while the
// deferred claims keep their claim and the capacity that goes with it.
func TestReconcileExpiredReachesClaimsBehindAFullBatchOfFailures(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-fairness.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// One Kind, one adapter: the three oldest claims are ones it cannot answer
	// about, and the newest is one it can decide at once.
	var resolvable Snapshot
	adapter.reconcile = func(_ context.Context, request ReconcileRequest) (ReconcileDecision, error) {
		if request.Snapshot.ID == resolvable.ID {
			return ReconcileBlock, nil
		}
		return "", errors.New("the reconciler could not reach its own database")
	}

	jobs := make([]Snapshot, 0, 4)
	for i := 0; i < 4; i++ {
		accepted := acceptQueued(t, svc, deps, nil)
		execution, ok := claimOnce(t, svc, deps, "runtime-a")
		if !ok {
			t.Fatalf("queued Job %d was not claimed", i)
		}
		if execution.JobID != accepted.ID {
			t.Fatalf("claimed %s while accepting %s", execution.JobID, accepted.ID)
		}
		jobs = append(jobs, accepted)
	}
	resolvable = jobs[3]

	// Every claim has expired, with the three the adapter cannot answer about
	// oldest — the order the scan reads them in.
	for i, job := range jobs {
		expiry := clock.Add(-time.Minute)
		if i < 3 {
			expiry = clock.Add(-time.Duration(30-i) * time.Minute)
		}
		if err := deps.DB.Model(&models.JobClaim{}).Where("job_id = ?", job.ID).
			Update("lease_expires_at", expiry).Error; err != nil {
			t.Fatalf("expire the claim of %s: %v", job.ID, err)
		}
	}

	const batch = 3
	first, err := svc.ReconcileExpired(context.Background(), deps, "runtime-b", batch)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.Examined != batch || len(first.Outcomes) != 0 {
		t.Fatalf("first pass = %+v, want the three claims the adapter cannot answer about and nothing applied", first)
	}
	if first.Deferred != batch {
		t.Fatalf("first pass deferred %d of %d undecided claims", first.Deferred, batch)
	}
	// Nothing was applied, so nothing was released: a claim an adapter could not
	// decide keeps the claim and the capacity it holds, and its next attempt is
	// scheduled rather than immediate.
	for i, job := range jobs[:3] {
		claim := claimRow(t, deps, job.ID)
		if claim.State != models.JobClaimStateHeld {
			t.Fatalf("undecided claim %d is %s, want held", i, claim.State)
		}
		if leases := countRows(t, deps, &models.JobCapacityLease{}, "job_id = ?", job.ID); leases == 0 {
			t.Errorf("undecided claim %d lost the capacity it holds", i)
		}
		if claim.ReconcileAttempts != 1 {
			t.Errorf("undecided claim %d recorded %d attempts, want one", i, claim.ReconcileAttempts)
		}
		if claim.NextReconcileAt == nil || !claim.NextReconcileAt.After(clock) {
			t.Fatalf("undecided claim %d is due again at %v, want a later attempt than now",
				i, claim.NextReconcileAt)
		}
	}

	// The claim behind them is reached by the next pass, rather than after the
	// failures have been retried for however long the deployment runs.
	second, err := svc.ReconcileExpired(context.Background(), deps, "runtime-b", batch)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.Examined != 1 || len(second.Outcomes) != 1 || second.Outcomes[0].JobID != resolvable.ID {
		t.Fatalf("second pass = %+v, want the one claim behind the failures", second)
	}
	if stored := jobRow(t, deps, resolvable.ID); stored.State != string(StateBlocked) {
		t.Fatalf("the claim behind a full batch of failures was never reconciled: state = %s", stored.State)
	}
}

// TestReconcileRetryScheduleWidensAndStops pins the schedule an undecided claim is
// put on: the first retry is short, so a transient failure recovers quickly without
// the claim being asked about in every pass, and the wait widens to a bounded
// ceiling rather than growing without limit or abandoning the claim.
func TestReconcileRetryScheduleWidensAndStops(t *testing.T) {
	cases := []struct {
		attempts uint
		want     time.Duration
	}{
		{attempts: 1, want: DefaultReconcileRetry},
		{attempts: 2, want: 2 * DefaultReconcileRetry},
		{attempts: 3, want: 4 * DefaultReconcileRetry},
		{attempts: 500, want: MaxReconcileRetry},
	}
	for _, tc := range cases {
		if got := reconcileRetryDelay(tc.attempts); got != tc.want {
			t.Errorf("reconcileRetryDelay(%d) = %v, want %v", tc.attempts, got, tc.want)
		}
	}
	// No attempt count reaches a wait that is not a wait, and none grows past the
	// ceiling however long a reconciler keeps failing.
	for _, attempts := range []uint{0, 1, 7, 1000} {
		got := reconcileRetryDelay(attempts)
		if got <= 0 || got > MaxReconcileRetry {
			t.Errorf("reconcileRetryDelay(%d) = %v, want a positive wait within the ceiling", attempts, got)
		}
	}
}

// TestAQuarantinedClaimIsNotAFenceForItsOwnExecution is the other half of the
// quarantine rule, and the half a live executor depends on.
//
// A quarantine says nobody could prove what became of the work, so no *other*
// executor may take it. What it must not say is that the execution holding the token
// has been fenced: a runtime that reads that refusal as a fence stops an executor
// whose worker is still running, and the Job it owns is then settled by nobody. The
// heartbeat therefore succeeds for the owning token — there is nothing to extend, and
// nothing to report as lost — and is refused exactly as before for any other one.
func TestAQuarantinedClaimIsNotAFenceForItsOwnExecution(t *testing.T) {
	_, deps := newDispatchDatabase(t, "quarantine-heartbeat.db")
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

	own := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}
	if err := svc.Heartbeat(deps, own, time.Minute); err != nil {
		t.Fatalf("the owning execution's heartbeat was refused: %v", err)
	}
	// Still quarantined, and nobody else's: the heartbeat extends nothing and
	// releases nothing.
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s after the owner's heartbeat, want quarantined", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal); count != 1 {
		t.Fatalf("capacity rows = %d while the quarantine stands, want its one slot", count)
	}

	other := ExecutionRef{JobID: accepted.ID, ExecutionToken: "0192f0aa-0000-7000-8000-00000000dead"}
	if err := svc.Heartbeat(deps, other, time.Minute); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("a foreign token's heartbeat = %v, want ErrStaleExecution", err)
	}
}

// TestTheExecutionThatOwnsAQuarantinedJobMayEndIt is §3's quiescence rule at the
// state machine: a quarantined Job's owner is the only thing that can prove what
// became of the work, and it proves it by reporting the outcome it reached — which
// may be a success, or a Job whose work in fact succeeded could never be settled and
// would hold a deployment-wide capacity slot for ever.
//
// The fence is the token, and the negative cases below are what keep it one: a hold
// nobody owns has no execution that may end it, and a foreign token cannot write.
func TestTheExecutionThatOwnsAQuarantinedJobMayEndIt(t *testing.T) {
	_, deps := newDispatchDatabase(t, "quarantine-settled.db")
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
	blocked := jobRow(t, deps, accepted.ID)
	if State(blocked.State) != StateBlocked || blocked.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the quarantined job is %s under token %q, want blocked under the owner's",
			blocked.State, blocked.ExecutionToken)
	}

	// A foreign execution cannot settle it, and says so rather than writing.
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: accepted.ID, ExecutionToken: "0192f0aa-0000-7000-8000-00000000dead"},
		ExpectedVersion: blocked.Version,
		Outcome:         StateSucceeded,
	}); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("a foreign token's success = %v, want ErrStaleExecution", err)
	}

	// The owner may, and its outcome is kept verbatim: succeeded, not blocked, not a
	// person's guess. The claim and the capacity go in the same transaction.
	settled, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: blocked.Version,
		Outcome:         StateSucceeded,
	})
	if err != nil {
		t.Fatalf("the owning execution's success: %v", err)
	}
	if settled.State != StateSucceeded {
		t.Fatalf("the settled job is %s, want succeeded", settled.State)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s after the owner settled it, want released", claim.State)
	}
	if token := jobRow(t, deps, accepted.ID).ExecutionToken; token != "" {
		t.Fatalf("the settled job still carries token %q", token)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows = %d after the quarantine was settled, want none", count)
	}

	// A Job blocked with no token is an ordinary hold, and nothing may end it by
	// declaring an outcome: this is the edge that is *not* open.
	held := acceptQueued(t, svc, deps, nil)
	heldExecution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the second queued Job was not claimed")
	}
	heldBlocked, err := svc.Transition(deps, Transition{
		JobID: held.ID, ExpectedVersion: heldExecution.Version,
		ExecutionToken: heldExecution.ExecutionToken, To: StateBlocked,
	})
	if err != nil {
		t.Fatalf("block the second job: %v", err)
	}
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: held.ID},
		ExpectedVersion: heldBlocked.Version,
		Outcome:         StateSucceeded,
	}); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("a held job's host-side success = %v, want ErrIllegalTransition", err)
	}
}

// TestReconcileLeavesAClaimItCannotDecideToAnUnreadableInput is the quiescence half
// of the replay contract, and the one the expired-claim path got wrong.
//
// A reconciler that holds no key for the Job's input can decide nothing about the
// work: the execution that sealed that input may be running perfectly well in the
// process that holds the key. Handing the adapter a nil input and applying whatever
// it answers released the live worker's claim and its deployment-wide capacity on a
// decision nobody could make — and the adapter's own "I could not decode the input"
// branch answered exactly that release.
func TestReconcileLeavesAClaimItCannotDecideToAnUnreadableInput(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-input-unavailable.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	registerTestCodec(t, svc)
	// The answer every queue-backed Kind gives for input it cannot read: the Job
	// cannot be run with what could not be decoded, so it is blocked.
	adapter.reconcile = func(_ context.Context, request ReconcileRequest) (ReconcileDecision, error) {
		if len(request.Input) == 0 {
			return ReconcileBlock, nil
		}
		return ReconcileRemainRunning, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	sealing := Deps{DB: deps.DB, Now: deps.Now, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "the-key-that-sealed-it")}}
	accepted, err := svc.Accept(sealing, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"sealed"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}
	execution, ok, err := svc.Claim(context.Background(), sealing, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
		Capacity: []CapacityRef{{Group: CapacityGroupGlobal, Limit: 2}},
	})
	if err != nil || !ok {
		t.Fatalf("claim as the runtime that owns the key: claimed=%v err=%v", ok, err)
	}
	expireClaim(t, deps, accepted.ID, clock)

	// A second runtime — a rotated key not yet rolled out, a key file lost with the
	// data root — reconciles the expired claim and can read none of the input.
	blind := Deps{DB: deps.DB, Now: deps.Now}
	report := reconcileOnce(t, svc, blind, "runtime-b")
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != ReconcileExternalWorkUnproven {
		t.Fatalf("report = %+v, want the undecidable claim left unresolved", report.Outcomes)
	}
	if adapter.reconciledCount() != 0 {
		t.Fatalf("the adapter was asked %d times with input it could not be given", adapter.reconciledCount())
	}

	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want blocked", stored.State)
	}
	if stored.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("job token = %q, want the live execution's %q kept", stored.ExecutionToken, execution.ExecutionToken)
	}
	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined: a live worker may still own this work", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 2 {
		t.Fatalf("capacity rows = %d, want the live execution's two slots kept", count)
	}
}

// TestAReconciledResumeThatCannotOpenItsInputKeepsTheClaimItTook is the resumed half
// of the same rule.
//
// A resume replaces an expired claim with a fresh one and hands the Job to this
// runtime. When the execution cannot then be built — the input is gone by the time
// it is opened — releasing that replacement handed the Job back to whoever asks next,
// which is a Resume dispatched over work that may still be running.
func TestAReconciledResumeThatCannotOpenItsInputKeepsTheClaimItTook(t *testing.T) {
	_, deps := newDispatchDatabase(t, "reconcile-resume-input.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	registerTestCodec(t, svc)

	sealing := Deps{DB: deps.DB, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "the-key-that-sealed-it")}}
	accepted, err := svc.Accept(sealing, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"sealed"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}
	if _, ok, err := svc.Claim(context.Background(), sealing, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
		Capacity: []CapacityRef{{Group: CapacityGroupGlobal, Limit: 2}},
	}); err != nil || !ok {
		t.Fatalf("claim the job: claimed=%v err=%v", ok, err)
	}

	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	reconciling := Deps{DB: deps.DB, Now: func() time.Time { return clock }, Replay: sealing.Replay}
	// The input disappears between the decision and the execution the resume builds:
	// a purge, a key rotation landing mid-pass. Whatever the cause, the replacement
	// claim was taken over an expired one and may not be handed back.
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		if err := deps.DB.Where("job_id = ?", accepted.ID).Delete(&models.JobReplayEnvelope{}).Error; err != nil {
			t.Fatalf("purge the envelope: %v", err)
		}
		return ReconcileResume, nil
	}
	expireClaim(t, deps, accepted.ID, clock)

	report := reconcileOnce(t, svc, reconciling, "runtime-b")
	if len(report.Resume) != 0 {
		t.Fatalf("a resume whose execution could not be built was dispatched anyway: %+v", report.Resume)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want blocked", stored.State)
	}
	if stored.ExecutionToken == "" {
		t.Fatalf("the replacement claim's token was cleared: a Resume would now be admitted over work nobody proved stopped")
	}
	claim := claimRow(t, deps, accepted.ID)
	if claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}
	if claim.ExecutionToken != stored.ExecutionToken {
		t.Fatalf("claim token %q and job token %q disagree", claim.ExecutionToken, stored.ExecutionToken)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 2 {
		t.Fatalf("capacity rows = %d, want the replacement claim's two slots kept", count)
	}
}

// TestAQuarantinedClaimIsReaskedAndReleasedOnNewEvidence is Task 4's recovery edge
// for the state a quarantine leaves behind: a Job nobody could prove anything about
// keeps its claim and its capacity, and the proof may arrive later — this host
// rebooted, the process no longer exists, the Kind's own durable evidence turned up.
// Only a decision that resolves it is applied, and a pass in between asks again
// rather than applying the same silence.
func TestAQuarantinedClaimIsReaskedAndReleasedOnNewEvidence(t *testing.T) {
	_, deps := newDispatchDatabase(t, "quarantine-recovered.db")
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileExternalWorkUnproven, nil
	}
	clock := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	if _, ok := claimOnce(t, svc, deps, "runtime-a", CapacityRef{Group: CapacityGroupGlobal, Limit: 2}); !ok {
		t.Fatal("the queued Job was not claimed")
	}
	expireClaim(t, deps, accepted.ID, clock)
	reconcileOnce(t, svc, deps, "runtime-b")
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want quarantined", claim.State)
	}
	asked := adapter.reconciledCount()

	// The same silence again: nothing is applied, and the claim is left exactly where
	// it is while the pass is deferred.
	report, err := svc.ReconcileQuarantined(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileQuarantined: %v", err)
	}
	if report.Examined != 1 || report.Deferred != 1 || len(report.Outcomes) != 0 {
		t.Fatalf("report = %+v, want the claim examined, deferred and unchanged", report)
	}
	if adapter.reconciledCount() != asked+1 {
		t.Fatalf("the adapter was asked %d times in the quarantine pass", adapter.reconciledCount()-asked)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateBlocked) {
		t.Fatalf("job state = %s, want it left blocked", stored.State)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("claim state = %s, want it left quarantined", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 2 {
		t.Fatalf("capacity rows = %d, want them still held", count)
	}
	// The pass is deferred, not retried on the next tick: a quarantine is not a
	// reconciliation loop over work nobody could decide.
	deferred, err := svc.ReconcileQuarantined(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("second ReconcileQuarantined: %v", err)
	}
	if deferred.Examined != 0 {
		t.Fatalf("a deferred quarantine was examined again immediately: %+v", deferred)
	}

	// New evidence: the runtime that owned the work is proved gone, and the adapter
	// answers with the release that proof permits.
	adapter.reconcile = func(context.Context, ReconcileRequest) (ReconcileDecision, error) {
		return ReconcileQueue, nil
	}
	clock = clock.Add(MaxReconcileRetry + time.Minute)
	recovered, err := svc.ReconcileQuarantined(context.Background(), deps, "runtime-b", DefaultReconcileBatch)
	if err != nil {
		t.Fatalf("ReconcileQuarantined after the proof: %v", err)
	}
	if len(recovered.Outcomes) != 1 || recovered.Outcomes[0].Decision != ReconcileQueue {
		t.Fatalf("report = %+v, want the proved-gone claim queued again", recovered)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateQueued) {
		t.Fatalf("job state = %s, want queued", stored.State)
	}
	if stored.ExecutionToken != "" {
		t.Fatalf("the released job still carries token %q", stored.ExecutionToken)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("claim state = %s, want released", claim.State)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal) + capacityCount(t, deps, testKind); count != 0 {
		t.Fatalf("capacity rows = %d after the quarantine was resolved, want none", count)
	}
}
