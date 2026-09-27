package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// claimNamed claims one named Job of the test Kind through ClaimJob.
func claimNamed(svc *Service, deps Deps, jobID string, capacity ...CapacityRef) (Execution, error) {
	return svc.ClaimJob(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, JobID: jobID, Claimant: "named-runtime", Capacity: capacity,
	})
}

// TestClaimJobTellsAFullBudgetFromAJobThatIsNotWaiting pins the distinction an
// executor holding one Job in memory has to act on: a full budget means "ask
// again later" and leaves the Job exactly as it was, while a Job that is no
// longer waiting will never be this executor's to run.
func TestClaimJobTellsAFullBudgetFromAJobThatIsNotWaiting(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-job.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	budget := CapacityRef{Group: CapacityGroupGlobal, Limit: 1}

	holder := acceptQueued(t, svc, deps, nil)
	waiting := acceptQueued(t, svc, deps, nil)
	held, err := claimNamed(svc, deps, holder.ID, budget)
	if err != nil {
		t.Fatalf("claim the first job with a free budget: %v", err)
	}

	_, err = claimNamed(svc, deps, waiting.ID, budget)
	if !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("a claim refused by a full budget answered %v, want ErrCapacityExhausted", err)
	}
	if errors.Is(err, ErrJobNotWaiting) {
		t.Fatalf("a full budget was reported as a job that is not waiting: %v", err)
	}
	if stored := jobRow(t, deps, waiting.ID); stored.State != string(StateQueued) || stored.ExecutionToken != "" {
		t.Fatalf("the refused job is %s with token %q, want queued and unowned", stored.State, stored.ExecutionToken)
	}
	if events := jobEvents(t, deps, waiting.ID); len(events) != 1 {
		t.Fatalf("the refused job has %d events, want only its acceptance", len(events))
	}

	// The job that holds the slot is running, so it is not waiting any more.
	if _, err := claimNamed(svc, deps, holder.ID, budget); !errors.Is(err, ErrJobNotWaiting) {
		t.Fatalf("claiming a running job answered %v, want ErrJobNotWaiting", err)
	}
	if _, err := claimNamed(svc, deps, "01900000-0000-7000-8000-000000000000", budget); !errors.Is(err, ErrJobNotWaiting) {
		t.Fatalf("claiming a job that does not exist answered %v, want ErrJobNotWaiting", err)
	}

	// Once the slot frees, the same claim succeeds.
	finishCancelled(t, svc, deps, holder.ID, held.ExecutionToken)
	if _, err := claimNamed(svc, deps, waiting.ID, budget); err != nil {
		t.Fatalf("the claim failed after the budget freed: %v", err)
	}
	if count := capacityCount(t, deps, CapacityGroupGlobal); count != 1 {
		t.Fatalf("global capacity rows = %d, want 1", count)
	}

	if _, err := svc.ClaimJob(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "named-runtime",
	}); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("a ClaimJob naming no job answered %v, want ErrInvalidClaim", err)
	}
}

// TestWaitingJobsListsOnlyUnownedWaitingWorkOldestFirst pins the listing an
// executor chooses its candidates from: it must agree with the claim about
// which Jobs are waiting, and page from a keyset position.
func TestWaitingJobsListsOnlyUnownedWaitingWorkOldestFirst(t *testing.T) {
	_, deps := newDispatchDatabase(t, "waiting-jobs.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	accept := func(offset time.Duration) Snapshot {
		deps.Now = func() time.Time { return base.Add(offset) }
		return acceptQueued(t, svc, deps, nil)
	}
	first := accept(0)
	second := accept(time.Second)
	third := accept(2 * time.Second)
	fourth := accept(3 * time.Second)
	deps.Now = func() time.Time { return base.Add(time.Minute) }

	// One is running and one has ended: neither is waiting.
	running, err := claimNamed(svc, deps, second.ID)
	if err != nil {
		t.Fatalf("claim the second job: %v", err)
	}
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: fourth.ID},
		ExpectedVersion: jobRow(t, deps, fourth.ID).Version,
		Outcome:         StateCancelled,
	}); err != nil {
		t.Fatalf("cancel the fourth job: %v", err)
	}

	page, err := svc.WaitingJobs(deps, testKind, 1, Cursor{}, 10)
	if err != nil {
		t.Fatalf("WaitingJobs: %v", err)
	}
	if got := snapshotIDs(page); len(got) != 2 || got[0] != first.ID || got[1] != third.ID {
		t.Fatalf("waiting jobs = %v, want [%s %s] (the running %s and the ended %s are not waiting)",
			got, first.ID, third.ID, running.JobID, fourth.ID)
	}

	next, err := svc.WaitingJobs(deps, testKind, 1, Cursor{AcceptedAt: page[0].AcceptedAt, ID: page[0].ID}, 10)
	if err != nil {
		t.Fatalf("WaitingJobs after a cursor: %v", err)
	}
	if got := snapshotIDs(next); len(got) != 1 || got[0] != third.ID {
		t.Fatalf("waiting jobs after the first = %v, want [%s]", got, third.ID)
	}

	if other, err := svc.WaitingJobs(deps, "some-other-kind", 1, Cursor{}, 10); err != nil || len(other) != 0 {
		t.Fatalf("another kind's waiting jobs = %v (err %v), want none", snapshotIDs(other), err)
	}
	var stored int64
	if err := deps.DB.Model(&models.Job{}).Count(&stored).Error; err != nil || stored != 4 {
		t.Fatalf("the listing changed the jobs table: %d rows, err %v", stored, err)
	}
}

func snapshotIDs(snaps []Snapshot) []string {
	ids := make([]string, 0, len(snaps))
	for _, snap := range snaps {
		ids = append(ids, snap.ID)
	}
	return ids
}

// TestAClaimAgainstAFullBudgetWritesNothingAtAll pins the read that spares a
// waiting caller the writer's lock: an executor asking again and again while the
// budget is full must not open a write — here, the guarded update that moves the
// Job to running — only to roll it back.
func TestAClaimAgainstAFullBudgetWritesNothingAtAll(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-no-write.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	budget := CapacityRef{Group: CapacityGroupGlobal, Limit: 1}

	holder := acceptQueued(t, svc, deps, nil)
	waiting := acceptQueued(t, svc, deps, nil)
	if _, err := claimNamed(svc, deps, holder.ID, budget); err != nil {
		t.Fatalf("claim the holder: %v", err)
	}

	var updates atomic.Int64
	if err := deps.DB.Callback().Update().Before("gorm:update").Register("count-claim-updates", func(*gorm.DB) {
		updates.Add(1)
	}); err != nil {
		t.Fatalf("register the update counter: %v", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := claimNamed(svc, deps, waiting.ID, budget); !errors.Is(err, ErrCapacityExhausted) {
			t.Fatalf("attempt %d answered %v, want ErrCapacityExhausted", attempt, err)
		}
	}
	if got := updates.Load(); got != 0 {
		t.Fatalf("three refused claims issued %d updates, want none", got)
	}
}

// TestAClaimJobDeadlineBoundsTheLoadAfterItsCommit pins where a claim's deadline
// stops. ClaimJob's caller bounds the claim, and the read of the execution's input
// after its commit, through the handle it passes: a read that runs into the
// deadline answers ErrExecutionNotLoaded with the claimed execution rather than
// leaving the Job running with a token nobody holds. That execution publishes
// through the context ClaimJob was given, so it can still write, and hand the
// claim back, after the deadline has passed.
func TestAClaimJobDeadlineBoundsTheLoadAfterItsCommit(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-deadline.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	waiting := acceptQueued(t, svc, deps, nil)
	budget := CapacityRef{Group: CapacityGroupGlobal, Limit: 1}

	deadline := time.Now().Add(300 * time.Millisecond)
	var committed, stalled atomic.Bool
	// The started event is the claim transaction's last write, so every read
	// after it is a read after the commit.
	if err := deps.DB.Callback().Create().After("gorm:create").Register("mark-claim-committed", func(db *gorm.DB) {
		if db.Statement.Table == "job_events" {
			committed.Store(true)
		}
	}); err != nil {
		t.Fatalf("register the commit marker: %v", err)
	}
	// The first read after the commit waits for its own statement's context,
	// so it returns when the deadline does if the deadline reaches it, and only
	// after five seconds if it does not.
	if err := deps.DB.Callback().Query().Before("gorm:query").Register("stall-the-load", func(db *gorm.DB) {
		if committed.Load() && stalled.CompareAndSwap(false, true) {
			select {
			case <-db.Statement.Context.Done():
			case <-time.After(5 * time.Second):
			}
		}
	}); err != nil {
		t.Fatalf("register the stalled read: %v", err)
	}

	bounded := deps
	claimCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	bounded.DB = deps.DB.WithContext(claimCtx)
	started := time.Now()
	execution, err := svc.ClaimJob(context.Background(), bounded, ClaimRequest{
		Kind: testKind, KindVersion: 1, JobID: waiting.ID, Claimant: "bounded-runtime",
		Capacity: []CapacityRef{budget},
	})
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("the load after the commit took %v: the claim's deadline did not reach it", elapsed)
	}
	if !stalled.Load() {
		t.Fatal("no read after the commit was stalled: the test did not reach the load")
	}
	if !errors.Is(err, ErrExecutionNotLoaded) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a load cut short by the deadline answered %v, want ErrExecutionNotLoaded wrapping the deadline", err)
	}
	row := jobRow(t, deps, waiting.ID)
	if execution.ExecutionToken == "" || row.ExecutionToken != execution.ExecutionToken || State(row.State) != StateRunning {
		t.Fatalf("the execution handed back does not own the claimed job (token %q, row %q in %s)",
			execution.ExecutionToken, row.ExecutionToken, row.State)
	}

	// The deadline has passed; the execution still writes.
	if _, err := execution.Progress(Progress{Phase: "admitting"}); err != nil {
		t.Fatalf("the execution publishes through the claim's deadline: %v", err)
	}
	if _, err := svc.ReleaseClaim(deps, ReleaseRequest{
		ExecutionRef: ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		Reason:       "admission-unfinished", To: StateQueued,
	}); err != nil {
		t.Fatalf("hand the claim back: %v", err)
	}
	if State(jobRow(t, deps, waiting.ID).State) != StateQueued {
		t.Fatal("the handed-back job is not waiting again")
	}
	if held := capacityRows(t, deps, waiting.ID); len(held) != 0 {
		t.Fatalf("the handed-back job still holds %d capacity rows", len(held))
	}
	if _, err := claimNamed(svc, deps, waiting.ID, budget); err != nil {
		t.Fatalf("the handed-back job could not be claimed again: %v", err)
	}
}

// TestAClaimJobDeadlineBoundsBlockingAnInputItCannotOpen pins the other write a
// claim makes before its execution is handed over. An input that cannot be opened
// blocks the Job, and that block is written inside the claim's bound: a caller
// holding a plugin's VM must not wait on it past its budget. A block that could
// not be written in the bound answers an UnrunnableClaimError with the claimed
// execution, so the caller holds the token and can record the block itself.
func TestAClaimJobDeadlineBoundsBlockingAnInputItCannotOpen(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-block-deadline.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	registerTestCodec(t, svc)

	sealing := Deps{DB: deps.DB, Now: deps.Now, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "the-key-that-sealed-it")}}
	accepted, err := svc.Accept(sealing, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"sealed"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}

	var stalled atomic.Bool
	if err := deps.DB.Callback().Update().Before("gorm:update").Register("stall-the-block", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(StateBlocked) {
			return
		}
		stalled.Store(true)
		select {
		case <-db.Statement.Context.Done():
		case <-time.After(5 * time.Second):
		}
	}); err != nil {
		t.Fatalf("register the stalled block: %v", err)
	}

	// A runtime holding no key for the input: it cannot be opened here.
	blind := Deps{DB: deps.DB, Now: deps.Now}
	claimCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	blind.DB = deps.DB.WithContext(claimCtx)
	started := time.Now()
	execution, err := svc.ClaimJob(context.Background(), blind, ClaimRequest{
		Kind: testKind, KindVersion: 1, JobID: accepted.ID, Claimant: "blind-runtime",
	})
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("blocking the unopenable input took %v: the claim's deadline did not reach it", elapsed)
	}
	if !stalled.Load() {
		t.Fatal("the block was never written: the test did not reach it")
	}
	var unrunnable *UnrunnableClaimError
	if !errors.As(err, &unrunnable) || unrunnable.Reason != blockedReasonInputUnavailable || !ReplayBlocked(StateRunning, err) {
		t.Fatalf("a block that could not be written answered %v, want an UnrunnableClaimError for the input", err)
	}
	row := jobRow(t, deps, accepted.ID)
	if State(row.State) != StateRunning || row.ExecutionToken == "" || row.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the claim handed back does not own the running job (row %s token %q, execution %q)",
			row.State, row.ExecutionToken, execution.ExecutionToken)
	}
}

// TestAClaimJobHandsBackAClaimWhosePrincipalIsGone pins the same rule for the
// other Job that cannot run: one whose principal has been deleted. When its
// failure cannot be written within the claim's bound, the claim is not dropped:
// the execution comes back with an UnrunnableClaimError naming the reason, so its
// holder can record it, and a later claim does not find a running Job that
// nobody owns.
func TestAClaimJobHandsBackAClaimWhosePrincipalIsGone(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-principal-gone.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	actor := uint(41)
	accepted := acceptQueued(t, svc, deps, &actor)
	// The account deletion sweep nulls the actor a Job was accepted to act as.
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", accepted.ID).
		Updates(map[string]any{"actor_user_id": nil, "owner_user_id": nil}).Error; err != nil {
		t.Fatalf("delete the actor: %v", err)
	}

	var stalled atomic.Bool
	if err := deps.DB.Callback().Update().Before("gorm:update").Register("stall-the-block", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(StateFailed) {
			return
		}
		stalled.Store(true)
		select {
		case <-db.Statement.Context.Done():
		case <-time.After(5 * time.Second):
		}
	}); err != nil {
		t.Fatalf("register the stalled block: %v", err)
	}

	bounded := deps
	claimCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	bounded.DB = deps.DB.WithContext(claimCtx)
	execution, err := svc.ClaimJob(context.Background(), bounded, ClaimRequest{
		Kind: testKind, KindVersion: 1, JobID: accepted.ID, Claimant: "bounded-runtime",
	})
	if !stalled.Load() {
		t.Fatal("the failure was never written: the test did not reach it")
	}
	var unrunnable *UnrunnableClaimError
	if !errors.As(err, &unrunnable) || unrunnable.Reason != blockedReasonPrincipalMissing {
		t.Fatalf("a failure that could not be written answered %v, want an UnrunnableClaimError for the principal", err)
	}
	if unrunnable.Failure == nil || unrunnable.Failure.Code != blockedReasonPrincipalMissing {
		t.Fatalf("the unrunnable claim carries failure %+v, want the one its holder must record", unrunnable.Failure)
	}
	row := jobRow(t, deps, accepted.ID)
	if State(row.State) != StateRunning || row.ExecutionToken == "" || row.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the claim handed back does not own the running job (row %s token %q, execution %q)",
			row.State, row.ExecutionToken, execution.ExecutionToken)
	}
	if execution.Access != (Access{}) {
		t.Fatalf("an execution whose principal is gone carries access %+v, want none", execution.Access)
	}
}

// TestAClaimIsNamedBeforeAnythingFollowsItsCommit pins where a caller learns its
// token. Everything ClaimJob does after the commit (reading the input, blocking a
// Job that cannot run) can fail without returning, and a caller that learns the
// token only from the return value then cannot settle a Job that is running under
// it. The token is handed over first.
func TestAClaimIsNamedBeforeAnythingFollowsItsCommit(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-named-first.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	registerTestCodec(t, svc)
	sealing := Deps{DB: deps.DB, Now: deps.Now, Replay: &ReplayConfig{Keys: replayKeyringFromSeeds(t, "the-key")}}
	accepted, err := svc.Accept(sealing, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: json.RawMessage(`{"secret":"sealed"}`)},
	})
	if err != nil {
		t.Fatalf("accept with replay input: %v", err)
	}
	if err := deps.DB.Callback().Query().Before("gorm:query").Register("panic-on-the-load", func(db *gorm.DB) {
		if db.Statement.Table == "job_replay_envelopes" {
			panic("the load failed without returning")
		}
	}); err != nil {
		t.Fatalf("register the panicking read: %v", err)
	}

	var named ExecutionRef
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the load did not panic: the test did not reach it")
			}
		}()
		_, _ = svc.ClaimJob(context.Background(), sealing, ClaimRequest{
			Kind: testKind, KindVersion: 1, JobID: accepted.ID, Claimant: "panicking-runtime",
			Claimed: func(ref ExecutionRef) { named = ref },
		})
	}()
	row := jobRow(t, deps, accepted.ID)
	if named.ExecutionToken == "" || row.ExecutionToken != named.ExecutionToken || State(row.State) != StateRunning {
		t.Fatalf("the caller was told %+v, and the Job is %s under %q", named, row.State, row.ExecutionToken)
	}
}
