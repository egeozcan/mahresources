package jobs

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

// A runtime can pass over waiting Jobs its Kind has said cannot start yet, and
// claim the next one instead; the passed-over Jobs stay waiting, unclaimed and
// holding no capacity.
func TestClaimPassesOverExcludedJobs(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-exclusion.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	first := acceptQueued(t, svc, deps, nil)
	second := acceptQueued(t, svc, deps, nil)

	execution, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
		ExcludeJobIDs: []string{first.ID},
	})
	if err != nil || !ok {
		t.Fatalf("claim = %v, %v", ok, err)
	}
	if execution.JobID != second.ID {
		t.Fatalf("claimed %s, want the Job after the excluded one (%s)", execution.JobID, second.ID)
	}
	if _, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a",
		ExcludeJobIDs: []string{first.ID},
	}); err != nil || ok {
		t.Fatalf("a claim with only excluded work waiting answered %v, %v", ok, err)
	}
	waiting, err := svc.Get(deps, Access{Administrator: true}, first.ID)
	if err != nil || waiting.State != StateQueued {
		t.Fatalf("the excluded Job is %s (%v), want still queued", waiting.State, err)
	}
}

// However many Jobs a Kind passes over, the next waiting one is still claimed:
// passing over is not bounded by how many ids fit in one query.
func TestClaimPassesOverAnyNumberOfExcludedJobs(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-exclusion-many.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	var excluded []string
	for i := 0; i < 1200; i++ {
		excluded = append(excluded, acceptQueued(t, svc, deps, nil).ID)
	}
	next := acceptQueued(t, svc, deps, nil)

	execution, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a", ExcludeJobIDs: excluded,
	})
	if err != nil || !ok || execution.JobID != next.ID {
		t.Fatalf("claim = %v, %v, %s; want the Job behind 1200 passed-over ones (%s)", ok, err, execution.JobID, next.ID)
	}
}

// PassOver is read after the waiting Jobs, so a Job named only by then is still
// passed over; and it is read once per claim, not once per page of passed-over
// Jobs, since a long list rebuilt per page made one claim quadratic in it.
func TestClaimAsksPassOverAfterTheReadAndOnce(t *testing.T) {
	_, deps := newDispatchDatabase(t, "claim-pass-over.db")
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())

	var excluded []string
	for i := 0; i < 450; i++ {
		excluded = append(excluded, acceptQueued(t, svc, deps, nil).ID)
	}
	// Handed back between the caller's list and the claim's read: only PassOver names it.
	late := acceptQueued(t, svc, deps, nil)
	next := acceptQueued(t, svc, deps, nil)

	// The late Job is named only once the claim has read the waiting Jobs, as a
	// hand-back that lands between the caller's list and that read would be.
	read := false
	if err := deps.DB.Callback().Query().After("gorm:query").Register("test:claim_read", func(*gorm.DB) { read = true }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	execution, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: testKind, KindVersion: 1, Claimant: "runtime-a", ExcludeJobIDs: excluded,
		PassOver: func() []string {
			calls++
			if !read {
				return excluded
			}
			return append(append([]string{}, excluded...), late.ID)
		},
	})
	if err != nil || !ok || execution.JobID != next.ID {
		t.Fatalf("claim = %v, %v, %s; want the Job after the late one (%s)", ok, err, execution.JobID, next.ID)
	}
	if calls != 1 {
		t.Fatalf("PassOver was asked %d times for one claim, want 1", calls)
	}
}
