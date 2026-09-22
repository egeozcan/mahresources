//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm/clause"
)

// These tests run the dialect-sensitive half of retention against PostgreSQL: the
// row locks a sweep's transactions take, and the concurrent writers those locks are
// there to serialize. SQLite serializes writers by itself, so an interleaving two
// connections run for one Job cannot form there.

// TestArtifactRemovalSerializesOnTheJobTimelinePG is the interleaving one
// connection cannot express: a sweep recording an artifact removal while another
// connection appends an event to the same Job.
//
// A timeline position is read as the Job's own maximum sequence plus one, and
// store.go's rule is that the parent Job's row must be held for that read to mean
// anything. The removal recorder wrote its output row and then its event without
// taking it, so both writers read the same maximum: on PostgreSQL the removal's
// insert waited on the unique index the other transaction was about to commit
// into, and the one that lost was rolled back — an artifact the Kind had confirmed
// gone, lost together with the record of it.
func TestArtifactRemovalSerializesOnTheJobTimelinePG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.cleanup = func(context.Context, ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		return ArtifactCleanupResult{Removed: []string{"artifact"}}, nil
	}
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2034, 2, 3, 4, 5, 6, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	// An artifact whose deadline had already passed when it was published, on a Job
	// that then ended: the deadline pass has nothing left to record for this Job, so
	// the removal recorder is the first thing in the sweep that writes on its
	// timeline. The Job's own history is a month out, so nothing prunes what the
	// assertions read.
	past := clock.Add(-time.Hour)
	if _, err := execution.Output(OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"path":"exports/17.tar"}`), ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("publish the artifact: %v", err)
	}
	if _, err := execution.Finish(FinishRequest{ExpectedVersion: execution.Version, Outcome: StateSucceeded}); err != nil {
		t.Fatalf("finish the Job: %v", err)
	}

	// The competing writer: it takes the Job's own row, which is the order every
	// writer of that timeline uses, and appends an event it has not committed yet.
	// Its position is therefore invisible to the sweep's own read of the maximum.
	// The Job has ended, so no execution owns it any more and the reference it is
	// addressed by carries no token.
	other := deps.DB.Begin()
	if other.Error != nil {
		t.Fatalf("begin the other connection: %v", other.Error)
	}
	var locked models.Job
	if err := other.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", accepted.ID).
		First(&locked).Error; err != nil {
		t.Fatalf("take the Job's row on the other connection: %v", err)
	}
	if err := svc.AppendEvent(Deps{DB: other, Now: deps.Now},
		ExecutionRef{JobID: accepted.ID},
		EventInput{Type: "checkpoint", Detail: json.RawMessage(`{"step":"the other connection got there first"}`)},
	); err != nil {
		t.Fatalf("append on the other connection: %v", err)
	}

	swept := make(chan SweepResult, 1)
	sweepFailed := make(chan error, 1)
	go func() {
		result, err := svc.Sweep(deps, policy, SweepCursor{}, 100)
		swept <- result
		sweepFailed <- err
	}()

	// The sweep has reached the row the append holds — the Job's own row, which
	// every writer of its timeline takes first — and can go no further.
	waitForABlockedQuery(t, deps.DB)
	if err := other.Commit().Error; err != nil {
		t.Fatalf("commit the append: %v", err)
	}

	result := <-swept
	if err := <-sweepFailed; err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	// Both facts are on the timeline, and neither took the other's position.
	events := jobEvents(t, deps, accepted.ID)
	sequences := make(map[uint64]string, len(events))
	checkpoints, removals := 0, 0
	for _, event := range events {
		if previous, seen := sequences[event.Sequence]; seen {
			t.Fatalf("timeline position %d holds both %s and %s", event.Sequence, previous, event.Type)
		}
		sequences[event.Sequence] = event.Type
		switch event.Type {
		case "checkpoint":
			checkpoints++
		case EventOutputRemoved:
			removals++
		}
	}
	if checkpoints != 1 {
		t.Fatalf("the other connection's append is not on the timeline: %+v", events)
	}
	if removals != 1 {
		t.Fatalf("recorded %d artifact removals, want the one the Kind confirmed", removals)
	}

	var output models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", accepted.ID, "artifact").First(&output).Error; err != nil {
		t.Fatalf("read the artifact output: %v", err)
	}
	if output.Availability != string(OutputRemoved) || output.RemovedAt == nil {
		t.Fatalf("artifact is %s (removed at %v), want the confirmed removal recorded",
			output.Availability, output.RemovedAt)
	}
	if result.Outputs == 0 {
		t.Error("the sweep reported no output record for the removal it performed")
	}
}

// TestRetentionArtifactCleanupIsFencedAgainstANewClaimPG is the PostgreSQL half of
// the cleanup fence. On SQLite the writer lock is the exclusion, which is what the
// untagged test proves; here the exclusion is the Job's own row, and the ordering
// the dialect takes it in is different: the row is locked first and the protection
// is re-read under it as its own statement, so a claim already waiting on that row
// commits before the read and the pass stands down, while a claim that arrives later
// waits for the deletion instead of racing it.
//
// Without the fence the claim commits in the gap between the check and the call, and
// the bytes go while a new execution holds the Job — which is the whole reason a
// check before the deletion is not enough.
func TestRetentionArtifactCleanupIsFencedAgainstANewClaimPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2034, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Lease: time.Minute,
	})

	// A queued Job — claimable, by definition — whose artifact's own deadline has
	// already passed.
	accepted := acceptQueued(t, svc, deps, nil)
	expires := clock.Add(-time.Hour)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: accepted.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"path":"exports/18.tar"}`), ExpiresAt: &expires,
	}); err != nil {
		t.Fatalf("publish the artifact: %v", err)
	}

	inCleanup := make(chan struct{})
	releaseCleanup := make(chan struct{})
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		close(inCleanup)
		<-releaseCleanup
		result := ArtifactCleanupResult{}
		for _, artifact := range request.Artifacts {
			result.Removed = append(result.Removed, artifact.Key)
		}
		return result, nil
	}

	swept := make(chan error, 1)
	go func() {
		_, err := svc.Sweep(deps, policy, SweepCursor{}, 100)
		swept <- err
	}()
	<-inCleanup

	// A second connection claims the Job while the pass is inside its deletion. The
	// guarded update it is admitted by and the pass's own row lock are the same row,
	// so it cannot commit until the deletion has.
	claimed := make(chan error, 1)
	go func() {
		_, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
			Kind: testKind, KindVersion: 1, Claimant: "runtime-b",
		})
		if err == nil && !ok {
			err = errors.New("the queued Job was not claimable")
		}
		claimed <- err
	}()

	select {
	case err := <-claimed:
		t.Fatalf("a claim committed while the artifact's bytes were still being deleted: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(releaseCleanup)

	if err := <-swept; err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if err := <-claimed; err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// The removal landed before the claim did, and the Job the claim admitted is the
	// one the pass had finished with.
	var artifact models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", accepted.ID, "artifact").First(&artifact).Error; err != nil {
		t.Fatalf("read the artifact: %v", err)
	}
	if artifact.Availability != string(OutputRemoved) {
		t.Fatalf("artifact is %s, want the acknowledged removal recorded", artifact.Availability)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateRunning) {
		t.Fatalf("state = %s, want the claim's running", stored.State)
	}
}
