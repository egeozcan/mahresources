//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"mahresources/models"
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
	if _, err := svc.ReleaseClaim(deps, ExecutionRef{
		JobID: accepted.ID, ExecutionToken: first.ExecutionToken,
	}, ReleaseReasonExecutionEnded); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}

	// The Job is still running, so it is not claimable until it is back in the
	// queue — which is what the release plus a transition models.
	if _, err := svc.Transition(deps, Transition{
		JobID: accepted.ID, ExpectedVersion: first.Version, To: StateQueued,
	}); err != nil {
		t.Fatalf("Transition to queued: %v", err)
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
	if _, err := first.ReleaseClaim(deps, ExecutionRef{
		JobID: claim.JobID, ExecutionToken: claim.ExecutionToken,
	}, ReleaseReasonExecutionEnded); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	var held models.Job
	if err := deps.DB.Where("id = ?", claim.JobID).First(&held).Error; err != nil {
		t.Fatalf("load the held job: %v", err)
	}
	if _, err := first.Transition(deps, Transition{
		JobID: claim.JobID, ExpectedVersion: held.Version, To: StateQueued,
	}); err != nil {
		t.Fatalf("Transition to queued: %v", err)
	}
	if _, ok := claimOnce(t, second, deps, "runtime-b"); !ok {
		t.Fatal("the freed budget was not available to the other runtime")
	}
}
