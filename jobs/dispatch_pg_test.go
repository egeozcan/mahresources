//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// These tests run the dialect-sensitive halves of dispatch against PostgreSQL:
// the conditional claim upsert, the slot-based capacity admission, and the race
// two connections run for one Job. SQLite serializes writers, so a race there
// cannot show whether the database is what admits one winner; PostgreSQL can.

// TestClaimAdmitsOneWinnerAcrossTwoConnectionsPG races two claims for one queued
// Job on two connections. The guarded update and the conditional claim upsert are
// what make exactly one of them win even when both read the same candidate.
func TestClaimAdmitsOneWinnerAcrossTwoConnectionsPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})

	for iteration := 0; iteration < 5; iteration++ {
		accepted := acceptQueued(t, svc, deps, nil)

		results := make([]bool, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				_, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
					Kind: testKind, KindVersion: 1, Claimant: "runtime-pg",
				})
				results[idx], errs[idx] = ok, err
			}(i)
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
		if started := jobEvents(t, deps, accepted.ID); len(started) != 2 {
			t.Fatalf("iteration %d: the losing racer recorded %d events", iteration, len(started))
		}
	}
}

// TestClaimTakesOverAReleasedClaimRowPG pins the conditional upsert on the
// dialect whose ON CONFLICT clause this depends on: a Job claimed again after a
// release keeps one claim row, and a row that is still held is never overwritten.
func TestClaimTakesOverAReleasedClaimRowPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})

	accepted := acceptQueued(t, svc, deps, nil)
	first, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	// A quiesced release: the runtime hands the claim back and leaves the Job in
	// the state it decided, in one write, which is the only way a running Job stops
	// running under an execution that is giving it up.
	if _, err := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ExecutionRef{JobID: accepted.ID, ExecutionToken: first.ExecutionToken},
		Reason:       ReleaseReasonExecutionEnded,
		To:           StateQueued,
	}); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}

	second, ok := claimOnce(t, svc, deps, "runtime-b")
	if !ok {
		t.Fatal("a released and requeued Job was not claimable again")
	}
	if second.ExecutionToken == first.ExecutionToken {
		t.Fatal("the second claim reused the first claim's token")
	}

	var claims []models.JobClaim
	if err := deps.DB.Where("job_id = ?", accepted.ID).Find(&claims).Error; err != nil {
		t.Fatalf("load claims: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("claim rows = %d, want one per Job", len(claims))
	}
	if claims[0].State != models.JobClaimStateHeld || claims[0].Claimant != "runtime-b" {
		t.Fatalf("claim row = %+v, want runtime-b's held claim", claims[0])
	}
}

// TestCapacityIsObservedAcrossServiceInstancesPG is the database-backed half of
// concurrency: a budget one runtime fills is full for another, because the
// occupancy is rows rather than a counter either of them holds.
func TestCapacityIsObservedAcrossServiceInstancesPG(t *testing.T) {
	deps := newPGDeps(t)
	definition := Definition{Kind: testKind, KindVersion: 1, Restorable: true, MaxConcurrent: 1}
	first := NewService()
	registerTestAdapter(t, first, definition)
	second := NewService()
	registerTestAdapter(t, second, definition)

	acceptQueued(t, first, deps, nil)
	if _, ok := claimOnce(t, first, deps, "runtime-a"); !ok {
		t.Fatal("the first runtime did not claim the waiting Job")
	}

	acceptQueued(t, first, deps, nil)
	if _, ok := claimOnce(t, second, deps, "runtime-b"); ok {
		t.Fatal("a second runtime ran work while the Kind's only budget slot was full")
	}

	// Freeing the slot frees it for everybody: the budget is the rows, not a
	// counter one process owns.
	var claim models.JobClaim
	if err := deps.DB.Where("state = ?", models.JobClaimStateHeld).First(&claim).Error; err != nil {
		t.Fatalf("load the held claim: %v", err)
	}
	var held models.Job
	if err := deps.DB.Where("id = ?", claim.JobID).First(&held).Error; err != nil {
		t.Fatalf("load the held job: %v", err)
	}
	if _, err := first.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ExecutionRef{JobID: claim.JobID, ExecutionToken: claim.ExecutionToken},
		Reason:       ReleaseReasonExecutionEnded,
		To:           StateQueued,
	}); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	if _, ok := claimOnce(t, second, deps, "runtime-b"); !ok {
		t.Fatal("the freed budget was not available to the other runtime")
	}
}

// TestLifecycleReleasAndQuarantineTakeTheJobLockFirstPG is a lock-order
// regression, and only PostgreSQL can show it: SQLite serializes writers, so two
// transactions can never hold the rows the other needs.
//
// Every lifecycle, release and reconciliation transaction has to take the Job's
// row before the claim's, because a release and a quarantine used to take them
// in the opposite order from the terminal transition. A terminal transition holds
// the Job row and then wants the claim row; a release or a quarantine held the
// claim row and then wanted the Job row — so a release racing a finish deadlocked
// with 40P01, and PostgreSQL resolved it by killing one of them.
//
// The competing finish is started the moment the release takes its first row
// lock, which is the only way to place two transactions inside each other's
// window deterministically.
func TestLifecycleReleaseAndQuarantineTakeTheJobLockFirstPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	clock := time.Date(2034, 2, 3, 4, 5, 6, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}
	decidedFrom := jobRow(t, deps, accepted.ID)

	finish := make(chan error, 1)
	var once sync.Once
	deps.DB.Callback().Update().After("gorm:update").Register("test:competing-finish", func(tx *gorm.DB) {
		once.Do(func() {
			go func() {
				_, err := svc.Finish(deps, FinishRequest{
					ExecutionRef:    ref,
					ExpectedVersion: decidedFrom.Version,
					Outcome:         StateFailed,
					Failure:         &Failure{Code: "gave-up", Class: FailureClassInternal},
				})
				finish <- err
			}()
			// Give it enough time to reach whatever row it needs and block there.
			time.Sleep(200 * time.Millisecond)
		})
	})

	_, releaseErr := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ref, Reason: ReleaseReasonExecutionEnded, To: StateQueued,
	})
	var finishErr error
	select {
	case finishErr = <-finish:
	case <-time.After(10 * time.Second):
		t.Fatal("the competing finish never returned; the two transactions are waiting on each other")
	}

	for _, result := range []struct {
		name string
		err  error
	}{{"release", releaseErr}, {"finish", finishErr}} {
		if result.err != nil && strings.Contains(result.err.Error(), "deadlock") {
			t.Fatalf("the %s transaction deadlocked: %v", result.name, result.err)
		}
	}
}

// pgSecondKind is a second Kind the capacity admission tests register: one
// claim's candidate query is taken by each claim's own transaction, so two claims
// of two Kinds reach the deployment-wide budget they share at the same time. Two
// claims of one Kind cannot: the second reads the first's Job as still queued
// (its claim is uncommitted) and is refused by the Job's own fence before
// capacity is ever asked.
const pgSecondKind = "test-work-b"

// acceptQueuedKind accepts one queued Job of the named Kind.
func acceptQueuedKind(t *testing.T, svc *Service, deps Deps, kind string) Snapshot {
	t.Helper()
	snap, err := svc.Accept(deps, Acceptance{
		Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept queued job of %s: %v", kind, err)
	}
	return snap
}

// claimKindOnce claims one Job of the named Kind, failing the test on an
// unexpected error.
func claimKindOnce(t *testing.T, svc *Service, deps Deps, kind, claimant string, capacity ...CapacityRef) (Execution, bool) {
	t.Helper()
	execution, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: kind, KindVersion: 1, Claimant: claimant, Capacity: capacity,
	})
	if err != nil {
		t.Fatalf("Claim(%s): %v", kind, err)
	}
	return execution, ok
}

// TestClaimCountsTheSlotsOutsideAReducedBudgetPG is the PostgreSQL half of the
// reduced-budget regression: the deployment-wide group holds one execution in a
// slot outside the range a limit of one admits, and further work must be refused
// there too. It is the same decision as the SQLite test's, asked of the engine
// whose writers run concurrently.
func TestClaimCountsTheSlotsOutsideAReducedBudgetPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})
	clock := time.Date(2034, 6, 7, 8, 9, 10, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	wide := CapacityRef{Group: CapacityGroupGlobal, Limit: 2}
	first := acceptQueued(t, svc, deps, nil)
	clock = clock.Add(time.Second)
	acceptQueued(t, svc, deps, nil)
	firstExecution, ok := claimKindOnce(t, svc, deps, testKind, "runtime-a", wide)
	if !ok {
		t.Fatal("the first Job was refused a budget of two with nothing in it")
	}
	if firstExecution.JobID != first.ID {
		t.Fatalf("the first claim took %s, want the oldest accepted Job %s", firstExecution.JobID, first.ID)
	}
	if _, ok := claimKindOnce(t, svc, deps, testKind, "runtime-a", wide); !ok {
		t.Fatal("the second Job was refused the budget's second slot")
	}
	finishCancelled(t, svc, deps, firstExecution.JobID, firstExecution.ExecutionToken)
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after one execution ended = %v, want only slot 1",
			CapacityGroupGlobal, slots)
	}

	third := acceptQueued(t, svc, deps, nil)
	if execution, claimed := claimKindOnce(t, svc, deps, testKind, "runtime-b",
		CapacityRef{Group: CapacityGroupGlobal, Limit: 1}); claimed {
		t.Fatalf("Job %s was admitted while %s already held its one execution in slot 1",
			execution.JobID, CapacityGroupGlobal)
	}
	if stored := jobRow(t, deps, third.ID); stored.State != string(StateQueued) {
		t.Fatalf("the refused Job is %s, want queued", stored.State)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{1}) {
		t.Fatalf("slots occupied in %s after the refusal = %v, want only the held slot 1",
			CapacityGroupGlobal, slots)
	}
}

// TestClaimSerializesAdmissionOnTheCapacityGroupPG is the race the group's
// admission lock exists for, and only PostgreSQL can show it: SQLite has one
// writer, so two claims can never be inside capacity admission at the same
// moment.
//
// The budget holds one execution in slot 2 — a slot the limit of two now in
// force does not admit, left behind by the wider limit that admitted it — so one
// more execution fits and only one. Two claims of two Kinds read that count at
// the same moment; without a serialization of the group both see room for one,
// and the budget ends up holding three executions where two are allowed.
func TestClaimSerializesAdmissionOnTheCapacityGroupPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	shared := Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
		CapacityGroup: CapacityGroupGlobal,
	}
	registerTestAdapter(t, svc, shared)
	registerTestAdapter(t, svc, Definition{
		Kind: pgSecondKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
		CapacityGroup: CapacityGroupGlobal,
	})
	clock := time.Date(2034, 6, 7, 8, 9, 10, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// Three executions under a budget of three fill slots 0, 1 and 2; the first
	// two end, leaving the budget one execution away from its ceiling in a slot
	// outside the range a limit of two admits.
	wide := CapacityRef{Group: CapacityGroupGlobal, Limit: 3}
	var executions []Execution
	for i := 0; i < 3; i++ {
		acceptQueued(t, svc, deps, nil)
		clock = clock.Add(time.Second)
		execution, ok := claimKindOnce(t, svc, deps, testKind, "runtime-a", wide)
		if !ok {
			t.Fatalf("execution %d was refused a budget of three with room in it", i)
		}
		executions = append(executions, execution)
	}
	for _, execution := range executions[:2] {
		finishCancelled(t, svc, deps, execution.JobID, execution.ExecutionToken)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{2}) {
		t.Fatalf("slots occupied in %s = %v, want the one execution in slot 2", CapacityGroupGlobal, slots)
	}

	// Two Jobs wait, one per Kind, and the second Kind's claim is started from
	// inside the first claim's transaction: the window the two counts would
	// otherwise pass through at the same moment.
	acceptQueued(t, svc, deps, nil)
	acceptQueuedKind(t, svc, deps, pgSecondKind)

	narrowed := []CapacityRef{{Group: CapacityGroupGlobal, Limit: 2}}
	type claimResult struct {
		execution Execution
		claimed   bool
		err       error
	}
	competing := make(chan claimResult, 1)
	var once sync.Once
	deps.DB.Callback().Create().After("gorm:create").Register("test:competing-capacity-claim", func(tx *gorm.DB) {
		if tx.Statement.Table != "job_capacity_leases" {
			return
		}
		once.Do(func() {
			go func() {
				execution, claimed, err := svc.Claim(context.Background(), deps, ClaimRequest{
					Kind: pgSecondKind, KindVersion: 1, Claimant: "runtime-b", Capacity: narrowed,
				})
				competing <- claimResult{execution: execution, claimed: claimed, err: err}
			}()
			// Long enough for the competing claim to read the occupancy and take
			// the free slot if nothing serializes the group.
			time.Sleep(500 * time.Millisecond)
		})
	})

	first, ok := claimKindOnce(t, svc, deps, testKind, "runtime-a", narrowed...)
	if !ok {
		t.Fatal("the first claim was refused while the budget had room for one execution")
	}

	var result claimResult
	select {
	case result = <-competing:
	case <-time.After(10 * time.Second):
		t.Fatal("the competing claim never returned")
	}
	if result.err != nil {
		t.Fatalf("the competing claim: %v", result.err)
	}
	if result.claimed {
		t.Fatalf("two claims were admitted into a budget with room for one: %s and %s",
			first.JobID, result.execution.JobID)
	}
	if slots := capacitySlots(t, deps, CapacityGroupGlobal); !slices.Equal(slots, []int{0, 2}) {
		t.Fatalf("slots occupied in %s = %v, want the admitted slot 0 and the leftover slot 2",
			CapacityGroupGlobal, slots)
	}
}
