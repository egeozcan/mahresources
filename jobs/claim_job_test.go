package jobs

import (
	"context"
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

// TestAClaimsDeadlineDoesNotReachTheReadsAfterItsCommit pins where a claim's
// deadline stops. A caller may bound the claim's own queries; once the claim has
// committed, the execution is loaded on the caller's context, because a read
// that ran into the deadline there would leave the Job running with no
// execution to run it or settle it.
func TestAClaimsDeadlineDoesNotReachTheReadsAfterItsCommit(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-deadline.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	waiting := acceptQueued(t, svc, deps, nil)

	deadline := time.Now().Add(300 * time.Millisecond)
	var committed atomic.Bool
	// The started event is the claim transaction's last write, so every read
	// after it is a read after the commit.
	if err := deps.DB.Callback().Create().After("gorm:create").Register("mark-claim-committed", func(db *gorm.DB) {
		if db.Statement.Table == "job_events" {
			committed.Store(true)
		}
	}); err != nil {
		t.Fatalf("register the commit marker: %v", err)
	}
	if err := deps.DB.Callback().Query().Before("gorm:query").Register("outlast-the-deadline", func(db *gorm.DB) {
		if committed.Load() {
			time.Sleep(time.Until(deadline) + 50*time.Millisecond)
		}
	}); err != nil {
		t.Fatalf("register the slow read: %v", err)
	}

	bounded := deps
	claimCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	bounded.DB = deps.DB.WithContext(claimCtx)
	execution, err := svc.ClaimJob(context.Background(), bounded, ClaimRequest{
		Kind: testKind, KindVersion: 1, JobID: waiting.ID, Claimant: "bounded-runtime",
	})
	if err != nil {
		t.Fatalf("a claim that committed before its deadline failed loading its execution: %v", err)
	}
	if execution.ExecutionToken == "" || jobRow(t, deps, waiting.ID).ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the execution does not own the claimed job")
	}
}
