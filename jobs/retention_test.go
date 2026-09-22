package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// expiredHistory is a policy with windows short enough to drive with an injected
// clock: succeeded/cancelled work ages out after an hour, failed/interrupted work
// after two.
func expiredHistory(history time.Duration) RetentionPolicy {
	return RetentionPolicy{History: history, Attention: 2 * history}
}

// sweepFor runs one bounded sweep and fails the test on an unexpected error.
func sweepFor(t *testing.T, svc *Service, deps Deps, policy RetentionPolicy, cursor SweepCursor, limit int) SweepResult {
	t.Helper()
	result, err := svc.Sweep(deps, policy, cursor, limit)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	return result
}

// jobExists reports whether a Job row is still there. Sweep assertions are about
// rows that survive and rows that do not, so they read the table directly rather
// than through the visibility predicate.
func jobExists(t *testing.T, deps Deps, id string) bool {
	t.Helper()
	var count int64
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count job %s: %v", id, err)
	}
	return count > 0
}

func countRows(t *testing.T, deps Deps, model any, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := deps.DB.Model(model).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

// TestRetentionSweepStartsAtFinishedAtAndLeavesNonterminalWorkAlone is §9's
// retention contract: the window opens when a Job reached its terminal state,
// the two windows are separate, and nonterminal work — including blocked work,
// which is waiting for a person — is never ordinary history.
func TestRetentionSweepStartsAtFinishedAtAndLeavesNonterminalWorkAlone(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy

	clock := time.Date(2031, 7, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	// finishedAt settles a Job and reports the instant it ended.
	finishedAt := func(job Snapshot, outcome State) (Snapshot, time.Time) {
		clock = clock.Add(time.Minute)
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		clock = clock.Add(time.Minute)
		transition := Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: outcome,
			ExecutionToken: executionTokenOf(t, deps, job.ID),
		}
		if outcome == StateFailed {
			transition.Failure = &Failure{Code: "boom", Class: FailureClassInternal, Message: "it broke"}
		}
		next, err := svc.Transition(deps, transition)
		if err != nil {
			t.Fatalf("finish as %s: %v", outcome, err)
		}
		return next, clock
	}

	// Aged past their windows: pruned.
	oldSuccess, _ := finishedAt(accept("an old success"), StateSucceeded)
	oldFailure, _ := finishedAt(accept("an old failure"), StateFailed)
	// Queued before the gap, so it is older than every window: still not
	// ordinary history, because its window never started.
	queued := accept("still queued")

	// The clock moves on: everything above is now past its deadline.
	clock = clock.Add(24 * time.Hour)

	// Finished inside its window: kept.
	recentSuccess, _ := finishedAt(accept("a recent success"), StateSucceeded)
	// Failed and inside the longer attention window: kept.
	recentFailure, _ := finishedAt(accept("a recent failure"), StateFailed)
	// Nonterminal work of every nonterminal kind: kept.
	running := advanceReplayJob(t, svc, deps, accept("still running"), StateRunning)
	paused := advanceReplayJob(t, svc, deps, running, StatePaused)
	blocked := advanceReplayJob(t, svc, deps, paused, StateBlocked)
	clock = clock.Add(5 * time.Minute)

	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	for _, gone := range []Snapshot{oldSuccess, oldFailure} {
		if jobExists(t, deps, gone.ID) {
			t.Errorf("an expired job survived: %s", gone.Title)
		}
		if rows := countRows(t, deps, &models.JobEvent{}, "job_id = ?", gone.ID); rows != 0 {
			t.Errorf("events of a pruned job survived: %d", rows)
		}
	}
	for _, kept := range []Snapshot{recentSuccess, recentFailure, queued, running, paused, blocked} {
		if !jobExists(t, deps, kept.ID) {
			t.Errorf("a job that is not due was pruned: %s (%s)", kept.Title, kept.State)
		}
	}
	if result.Pruned != 2 {
		t.Fatalf("pruned %d jobs, want 2", result.Pruned)
	}
	// The deadline the sweep obeyed is the Job's own, stamped when it finished.
	old := jobRow(t, deps, recentSuccess.ID)
	if old.ExpiresAt == nil {
		t.Fatal("a finished job carries no expiry deadline")
	}
	if !old.ExpiresAt.Equal(old.FinishedAt.Add(time.Hour)) {
		t.Fatalf("deadline = %v, want finished_at + the history window", old.ExpiresAt)
	}
}

// TestRetentionSweepNeverPrunesAJobWithAnUnresolvedClaim covers §9's protection
// for recovery records: a claim nobody could resolve — a quarantined one, whose
// process group may still be alive — keeps its Job, its lease and its capacity,
// and neither expiry nor a sweep may go around it.
func TestRetentionSweepNeverPrunesAJobWithAnUnresolvedClaim(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 2, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "plugin-command", KindVersion: 1, State: StateQueued, Origin: "api", Title: "a command run",
		Visibility: VisibilityAdmin,
		Replay:     ReplayInput{NonReplayable: true},
	})
	// An output of that Job whose own expiry has passed. It is not recorded by the
	// sweep: a claim nothing could prove dead is the one thing no expiry may write
	// through, and leaving a Job alone has to mean leaving its rows alone.
	artifactExpiry := clock.Add(time.Minute)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: job.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the command's output",
		Reference: json.RawMessage(`{"name":"out.log"}`), ExpiresAt: &artifactExpiry,
	}); err != nil {
		t.Fatalf("publish output: %v", err)
	}
	job = advanceReplayJob(t, svc, deps, job, StateRunning)
	job = advanceReplayJob(t, svc, deps, job, StateSucceeded)

	// A claim that reached a terminal state while still unresolved. The lifecycle
	// releases a claim its own execution still owns, so this row is constructed
	// directly: it is the state a quarantined execution leaves behind when the
	// process that held it never came back.
	if err := deps.DB.Create(&models.JobClaim{
		JobID: job.ID, Kind: job.Kind, KindVersion: job.KindVersion,
		Claimant: "host:gone", ExecutionToken: "00000000-0000-7000-8000-000000000001",
		State:     models.JobClaimStateQuarantined,
		ClaimedAt: clock, HeartbeatAt: clock, LeaseExpiresAt: clock,
	}).Error; err != nil {
		t.Fatalf("seed quarantined claim: %v", err)
	}

	// A second Job that is genuinely due, so the sweep is proved to make progress
	// rather than to stop at the first obstacle.
	other := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: "a due job",
		Replay: ReplayInput{NonReplayable: true},
	})
	other = advanceReplayJob(t, svc, deps, other, StateRunning)
	other = advanceReplayJob(t, svc, deps, other, StateSucceeded)

	clock = clock.Add(24 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)
	if !jobExists(t, deps, job.ID) {
		t.Fatal("a job with an unresolved claim was pruned")
	}
	if claim := claimRow(t, deps, job.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("the quarantined claim was changed: %+v", claim)
	}
	if jobExists(t, deps, other.ID) {
		t.Fatal("the sweep stopped at the first unresolvable job")
	}
	if result.Skipped == 0 {
		t.Error("the sweep did not report the job it left alone")
	}
	// What the protection covers is the execution record: the Job, its history
	// and the claim all stay exactly as they were, and nothing is deleted. The
	// artifact's own deadline is not the Job's retention — §7 makes availability
	// independent of the outcome — so the one thing the sweep does write is that
	// promised expiry, which is a fact about the file rather than about the
	// quarantined process.
	outputs, err := svc.Outputs(deps, Access{UserID: 1, Administrator: true}, job.ID)
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("the sweep deleted a claimed job's output row: %+v", outputs)
	}
	if outputs[0].Availability != OutputExpired {
		t.Fatalf("artifact availability = %s, want the deadline it was published with recorded",
			outputs[0].Availability)
	}
}

// TestRetentionSweepIsBoundedAndResumesFromItsCursor covers the batching
// contract: a pass deletes at most what it was asked to, reports where to
// continue, and terminating is a property of the cursor rather than of luck.
func TestRetentionSweepIsBoundedAndResumesFromItsCursor(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 3, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	var jobs []Snapshot
	for i := 0; i < 5; i++ {
		job := acceptFor(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: "export",
			Replay: ReplayInput{NonReplayable: true},
		})
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		job = advanceReplayJob(t, svc, deps, job, StateSucceeded)
		jobs = append(jobs, job)
	}
	clock = clock.Add(24 * time.Hour)

	pruned := 0
	cursor := SweepCursor{}
	passes := 0
	for {
		result := sweepFor(t, svc, deps, policy, cursor, 2)
		passes++
		if result.Pruned > 2 {
			t.Fatalf("a pass pruned %d jobs, over the bound it was given", result.Pruned)
		}
		pruned += result.Pruned
		if result.Next == nil {
			break
		}
		cursor = *result.Next
		if passes > 10 {
			t.Fatal("the sweep never reached the end of the expired range")
		}
	}
	if pruned != len(jobs) {
		t.Fatalf("pruned %d jobs over %d passes, want %d", pruned, passes, len(jobs))
	}
	for _, job := range jobs {
		if jobExists(t, deps, job.ID) {
			t.Errorf("a job survived the sweep: %s", job.ID)
		}
	}
	// A further pass finds nothing to do rather than failing.
	if result := sweepFor(t, svc, deps, policy, SweepCursor{}, 2); result.Pruned != 0 || result.Next != nil {
		t.Fatalf("a sweep after the range ended = %+v", result)
	}
	// The bound is bounded: a caller may not ask for an unbounded delete.
	if _, err := svc.Sweep(deps, policy, SweepCursor{}, MaxSweepBatch+1); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("an oversized sweep batch = %v, want ErrInvalidPage", err)
	}
	// Nor may it walk from a position with no cycle: the boundary is what says
	// where the walk ends, and a caller that drops it would be asking for the
	// open-ended walk this cursor shape exists to remove.
	if _, err := svc.Sweep(deps, policy, SweepCursor{
		FinishedAt: time.Date(2031, 7, 3, 9, 0, 0, 0, time.UTC), ID: "a-job-id",
	}, 10); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("a sweep position without its cycle = %v, want ErrInvalidCursor", err)
	}
}

// TestRetentionSweepRevisitsWorkItsFirstCycleLeftBehind covers the property a
// keyset position cannot have on its own: a bounded walk ends.
//
// A Job the sweep may not take — a pinned one here — stays where it is and the
// cursor moves past it. Under sustained expiry there is always another due Job
// ahead of that cursor, so a pass that reports "there is more" whenever its batch
// was full reports it for ever, the walk never returns to the beginning, and the
// Job an earlier pass left alone is never asked about again however long the
// deployment keeps finishing work. The regression drives exactly that: a Job the
// first pass may not take, and as many arrivals between two passes as one batch
// can hold, so a walk with no end always has somewhere further to go.
func TestRetentionSweepRevisitsWorkItsFirstCycleLeftBehind(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 7, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// finish accepts one Job and settles it as a success at the current instant.
	finish := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		job := acceptFor(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7), Replay: ReplayInput{NonReplayable: true},
		})
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		return advanceReplayJob(t, svc, deps, job, StateSucceeded)
	}

	// A batch of two, and three Jobs due when the walk starts. The oldest is
	// pinned, so the pass that has room for two takes the second and leaves the
	// first behind its cursor.
	const batch = 2
	behind := finish("left behind")
	finish("the batch's second")
	finish("the range's end")
	if err := svc.SetPreference(deps, Access{UserID: 7},
		PreferenceRequest{JobID: behind.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}
	clock = clock.Add(24 * time.Hour)

	cursor := SweepCursor{}
	pruned := 0
	cycleEnded := false
	for pass := 0; pass < 20 && jobExists(t, deps, behind.ID); pass++ {
		result := sweepFor(t, svc, deps, policy, cursor, batch)
		pruned += result.Pruned
		if result.Next == nil {
			cycleEnded = true
			// The pass had nothing left inside its range, which is where a caller
			// starts again from the oldest expired work.
			cursor = SweepCursor{}
		} else {
			// A continued pass carries the cycle it belongs to, or the position it
			// returns is one the next call refuses: the boundary is the only thing
			// that says where the walk ends.
			if result.Next.Bound == nil {
				t.Fatalf("a continued pass carries no cycle boundary: %+v", result.Next)
			}
			cursor = *result.Next
		}
		if pass == 0 {
			// The pin is lifted between batches, so the Job the first pass could not
			// take is due from the second pass onwards.
			if err := svc.SetPreference(deps, Access{UserID: 7},
				PreferenceRequest{JobID: behind.ID, Pinned: boolPtr(false)}); err != nil {
				t.Fatalf("unpin: %v", err)
			}
		}
		// Two later Jobs become due before every pass after this one — a whole batch
		// of them — so a walk that never reaches the end of its range always has
		// somewhere further to go.
		finish(fmt.Sprintf("arrival %d", pass))
		finish(fmt.Sprintf("arrival %d again", pass))
		clock = clock.Add(2 * time.Hour)
	}

	if jobExists(t, deps, behind.ID) {
		t.Fatalf("a Job an earlier pass left behind was never revisited: %d pruned over 20 passes", pruned)
	}
	if !cycleEnded {
		t.Fatal("no pass ever reached the end of its range, so the walk never restarted from the oldest work")
	}
	if pruned < 3 {
		t.Fatalf("the sweep pruned %d jobs, want the due work it walked past", pruned)
	}
}

// TestRetentionPinExemptsMetadataButNotArtifacts covers both halves of pinning:
// a pinned Job keeps its history past its deadline, and the pin exempts nothing
// else — the artifact's own expiry is still recorded, and a relative's Job is a
// different Job.
func TestRetentionPinExemptsMetadataButNotArtifacts(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 4, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	pinned := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "a pinned export",
		Replay: ReplayInput{NonReplayable: true},
	})
	// The artifact expires before the metadata does, which is the case the
	// ordering in the sweep exists for.
	artifactExpiry := clock.Add(30 * time.Minute)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: pinned.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"name":"export.tar"}`), ExpiresAt: &artifactExpiry,
	}); err != nil {
		t.Fatalf("publish artifact: %v", err)
	}
	pinned = advanceReplayJob(t, svc, deps, pinned, StateRunning)
	pinned = advanceReplayJob(t, svc, deps, pinned, StateSucceeded)

	// Its retry is a different Job, and pinning the ancestor does not exempt it.
	relative := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "its retry",
		Replay: ReplayInput{NonReplayable: true},
	})
	relative = advanceReplayJob(t, svc, deps, relative, StateRunning)
	relative = advanceReplayJob(t, svc, deps, relative, StateSucceeded)
	if err := svc.Link(deps, LinkRequest{Type: LinkRetryOf, FromJobID: relative.ID, ToJobID: pinned.ID}); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: pinned.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	clock = clock.Add(24 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	if !jobExists(t, deps, pinned.ID) {
		t.Fatal("a pinned job's metadata was pruned")
	}
	if rows := countRows(t, deps, &models.JobEvent{}, "job_id = ?", pinned.ID); rows == 0 {
		t.Error("a pinned job's events were pruned")
	}
	if result.Skipped == 0 {
		t.Error("the sweep did not report the pinned job it left alone")
	}
	// The artifact's own expiry was still recorded: a pin is not a retention
	// policy for what a Job produced.
	outputs, err := svc.Outputs(deps, Access{UserID: 7}, pinned.ID)
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Availability != OutputExpired {
		t.Fatalf("a pinned job's expired artifact = %+v", outputs)
	}
	// And the relative is gone: pinning one Job does not pin a lineage.
	if jobExists(t, deps, relative.ID) {
		t.Fatal("pinning one job exempted its relative")
	}
}

// TestRetentionSweepPurgesExpiredReplayEnvelopes puts the replay window on the
// sweep's clock: a finished Job's sealed input is purged once its own retention
// passes, whether or not the Job's metadata is due yet.
func TestRetentionSweepPurgesExpiredReplayEnvelopes(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	policy := expiredHistory(24 * time.Hour)
	deps.Retention = &policy
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material"), Retention: time.Hour}
	clock := time.Date(2031, 7, 5, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		Title: "a download", Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	job = advanceReplayJob(t, svc, deps, job, StateRunning)
	job = advanceReplayJob(t, svc, deps, job, StateSucceeded)

	// Two hours on: past the replay window, nowhere near the metadata window.
	clock = clock.Add(2 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	envelope := replayEnvelopeRow(t, deps, job.ID)
	if envelope.PurgedAt == nil || envelope.PurgeReason != models.JobReplayPurgeExpired {
		t.Fatalf("the expired envelope was not purged: %+v", envelope)
	}
	if len(envelope.Ciphertext) != 0 {
		t.Error("a purged envelope still holds ciphertext")
	}
	if result.Envelopes != 1 {
		t.Fatalf("the sweep reported %d purged envelopes, want 1", result.Envelopes)
	}
	if !jobExists(t, deps, job.ID) {
		t.Fatal("the sweep pruned a job whose metadata window has not passed")
	}
}

// TestRetentionDeadlinesAreComputedOnceForRowsThatPredateThem covers the one
// shape that has no deadline: a Job that finished before deadlines existed. It
// gets the policy's window from its own finish time — exactly what migration
// does for backfilled history — rather than becoming immortal.
func TestRetentionDeadlinesAreComputedOnceForRowsThatPredateThem(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	clock := time.Date(2031, 7, 6, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// A legacy terminal row: finished long ago, no deadline recorded.
	finished := clock.Add(-48 * time.Hour)
	legacy := seedJob(t, deps, StateSucceeded, finished.Add(-time.Minute), 3)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", legacy.ID).
		Updates(map[string]any{"finished_at": finished}).Error; err != nil {
		t.Fatalf("stamp finished_at: %v", err)
	}
	// A legacy terminal row that finished recently.
	recentFinished := clock.Add(-10 * time.Minute)
	recent := seedJob(t, deps, StateFailed, recentFinished.Add(-time.Minute), 3)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", recent.ID).
		Updates(map[string]any{"finished_at": recentFinished}).Error; err != nil {
		t.Fatalf("stamp finished_at: %v", err)
	}

	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)
	if jobExists(t, deps, legacy.ID) {
		t.Fatal("a legacy job past the policy window was never swept")
	}
	if !jobExists(t, deps, recent.ID) {
		t.Fatal("a legacy job inside the policy window was pruned")
	}
	stamped := jobRow(t, deps, recent.ID)
	if stamped.ExpiresAt == nil || !stamped.ExpiresAt.Equal(recentFinished.Add(2*time.Hour)) {
		t.Fatalf("legacy job deadline = %v, want finished_at + the attention window", stamped.ExpiresAt)
	}
	if result.Pruned != 1 {
		t.Fatalf("pruned %d jobs, want 1", result.Pruned)
	}
}

// TestRetentionSweepExpiresOutputsOnTheirOwnDeadline is §10's "artifact expiry is
// independent of the Job's outcome and retention".
//
// Output availability used to be recorded only for the Jobs the sweep was
// already pruning, so an artifact that expired an hour after it was published
// kept advertising itself as available until its Job's history came due — a
// month later for a success, three months later for a failure, and never for a
// Job that was still running.
func TestRetentionSweepExpiresOutputsOnTheirOwnDeadline(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 5, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// A still-running Job with an artifact that expires in an hour, and a setted
	// Job whose metadata window is a month out.
	running := seededExecution(t, deps, StateRunning, "claim-a")
	settled := seededExecution(t, deps, StateRunning, "claim-b")

	expires := clock.Add(time.Hour)
	for _, job := range []models.Job{running, settled} {
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: job.ExecutionToken}
		if _, err := svc.PublishOutput(deps, ref, OutputInput{
			Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
			Reference: json.RawMessage(`{"path":"exports/9.tar"}`), ExpiresAt: &expires,
		}); err != nil {
			t.Fatalf("PublishOutput for %s: %v", job.ID, err)
		}
	}
	// The second Job reaches an outcome of its own, which starts its metadata
	// window; the artifact it published keeps the deadline it was given.
	if _, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: settled.ID, ExecutionToken: "claim-b"},
		ExpectedVersion: settled.Version,
		Outcome:         StateSucceeded,
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// The artifacts' own deadline passes; neither Job's metadata is anywhere near
	// its own.
	clock = clock.Add(2 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)
	if result.Outputs != 2 {
		t.Fatalf("recorded %d output availabilities, want both expired artifacts", result.Outputs)
	}

	for _, job := range []models.Job{running, settled} {
		var output models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", job.ID, "artifact").First(&output).Error; err != nil {
			t.Fatalf("read output of %s: %v", job.ID, err)
		}
		if output.Availability != string(OutputExpired) {
			t.Fatalf("artifact of %s is %s, want expired", job.ID, output.Availability)
		}
	}

	// Recording the expiry records the Job Event with it (§7), so the timeline and
	// the resumable stream both say what became of the artifact rather than leaving
	// a reader to compare a clock against a stored instant.
	admin := Access{UserID: 1, Administrator: true}
	for _, job := range []models.Job{running, settled} {
		timeline, err := svc.Timeline(deps, admin, job.ID, 0, 0)
		if err != nil {
			t.Fatalf("Timeline of %s: %v", job.ID, err)
		}
		expired := eventsOfType(timeline, EventOutputExpired)
		if len(expired) != 1 {
			t.Fatalf("job %s recorded %d expiry events, want one", job.ID, len(expired))
		}
		if detail := string(expired[0].Detail); !strings.Contains(detail, `"key":"artifact"`) {
			t.Errorf("expiry detail = %s, want it to name the artifact", detail)
		}
		if expired[0].ReservedHost {
			t.Error("an output expiry is not a lifecycle fact and must not claim the reserved capacity")
		}
	}
	if _, err := svc.PublishPendingEvents(deps, DefaultPublishBatch); err != nil {
		t.Fatalf("publish: %v", err)
	}
	delivered, err := svc.PublishedEvents(deps, admin, 0, 0)
	if err != nil {
		t.Fatalf("PublishedEvents: %v", err)
	}
	if len(eventsOfType(delivered, EventOutputExpired)) != 2 {
		t.Fatalf("delivered %+v, want both committed expiries", delivered)
	}

	// A repeated pass records nothing more: an expiry is a durable fact about one
	// crossing of one deadline, not one event per sweep.
	second := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)
	if second.Outputs != 0 {
		t.Fatalf("the repeated pass recorded %d more output availabilities, want none", second.Outputs)
	}
	for _, job := range []models.Job{running, settled} {
		timeline, err := svc.Timeline(deps, admin, job.ID, 0, 0)
		if err != nil {
			t.Fatalf("Timeline of %s: %v", job.ID, err)
		}
		if len(eventsOfType(timeline, EventOutputExpired)) != 1 {
			t.Fatalf("job %s recorded the same expiry twice", job.ID)
		}
	}

	// The Job itself is untouched: its own window has not started, or has not
	// passed, and an expired artifact never rewrites an outcome.
	if !jobExists(t, deps, running.ID) || !jobExists(t, deps, settled.ID) {
		t.Fatal("expiring an artifact pruned the Job that published it")
	}
	if stored := jobRow(t, deps, settled.ID); stored.State != string(StateSucceeded) {
		t.Fatalf("state = %s, want succeeded", stored.State)
	}
}

// TestRetentionSweepConfirmsArtifactCleanupBeforePruningHistory is §9's "sweep
// work records output removal before pruning the relevant history", read as the
// requirement that the removal be *established* rather than assumed.
//
// Marking an output removed says what the database believes; only the Kind knows
// whether the bytes are really gone. So the sweep asks, and a Job whose artifacts
// nobody could account for keeps its history — and the reference to what was left
// behind.
func TestRetentionSweepConfirmsArtifactCleanupBeforePruningHistory(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 6, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, testDefinition())
	retain := map[string]bool{}
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		if retain[request.JobID] {
			return ArtifactCleanupResult{Retained: []string{"artifact"}}, nil
		}
		return ArtifactCleanupResult{Removed: []string{"artifact"}}, nil
	}

	// A Job that published an artifact and finished cleanly.
	settle := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		accepted := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7),
			Replay:      ReplayInput{NonReplayable: true},
		})
		execution, ok := claimOnce(t, svc, deps, "runtime-a")
		if !ok {
			t.Fatalf("claim for %s: nothing was claimed", title)
		}
		if execution.JobID != accepted.ID {
			t.Fatalf("claimed %s while settling %s", execution.JobID, title)
		}
		if _, err := execution.Output(OutputInput{
			Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
			Reference: json.RawMessage(`{"path":"exports/keep.tar"}`),
		}); err != nil {
			t.Fatalf("publish artifact of %s: %v", title, err)
		}
		finished, err := execution.Finish(FinishRequest{ExpectedVersion: execution.Version, Outcome: StateSucceeded})
		if err != nil {
			t.Fatalf("finish %s: %v", title, err)
		}
		return finished
	}

	cleaned := settle("the artifacts go")
	retained := settle("the artifacts stay")
	retain[retained.ID] = true
	// A pinned Job is not due at all, so nothing of its own may be destroyed on
	// its way past — including the artifact its history points at. That artifact
	// carries no deadline of its own; one that did would be removed on its own
	// schedule, which is what the pin does not exempt (see
	// TestRetentionExpiredArtifactIsRemovedWhateverTheJobsMetadataSays).
	pinned := settle("the pinned one")
	if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: pinned.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// A Job of a Kind this process has no adapter for: no cleanup authority at
	// all, which is the same answer as a refusal.
	orphan := seedJob(t, deps, StateSucceeded, clock.Add(-2*time.Hour), 1)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", orphan.ID).
		Updates(map[string]any{"finished_at": clock.Add(-2 * time.Hour), "expires_at": clock.Add(-time.Hour)}).Error; err != nil {
		t.Fatalf("settle the adapter-less job: %v", err)
	}
	if err := deps.DB.Create(&models.JobOutput{
		ID: types.NewUUIDv7(), JobID: orphan.ID, Key: "artifact", Type: OutputTypeArtifact,
		Label: "group-export.tar", Reference: types.JSON(`{"path":"exports/orphan.tar"}`),
		Availability: string(OutputAvailable), Version: 1, CreatedAt: clock, UpdatedAt: clock,
	}).Error; err != nil {
		t.Fatalf("publish the adapter-less job's artifact: %v", err)
	}

	clock = clock.Add(48 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	if adapter.cleanupCount() != 2 {
		t.Fatalf("the sweep asked for cleanup %d times, want once per Job with artifacts it can reach",
			adapter.cleanupCount())
	}
	if jobExists(t, deps, cleaned.ID) {
		t.Fatal("a Job whose artifacts were confirmed removed survived the sweep")
	}
	if rows := countRows(t, deps, &models.JobOutput{}, "job_id = ?", cleaned.ID); rows != 0 {
		t.Fatalf("the pruned Job's output rows survived: %d", rows)
	}

	if !jobExists(t, deps, retained.ID) {
		t.Fatal("a Job whose artifact could not be removed was pruned anyway")
	}
	var output models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", retained.ID, "artifact").First(&output).Error; err != nil {
		t.Fatalf("the retained Job lost its artifact reference: %v", err)
	}
	if output.Availability != string(OutputAvailable) {
		t.Fatalf("retained artifact availability = %s, want it still advertised", output.Availability)
	}

	if !jobExists(t, deps, orphan.ID) {
		t.Fatal("a Job whose Kind this process cannot run was pruned without any cleanup authority")
	}
	if rows := countRows(t, deps, &models.JobOutput{}, "job_id = ?", orphan.ID); rows != 1 {
		t.Fatalf("the adapter-less Job's artifact reference is gone: %d rows", rows)
	}

	// The pinned Job was never asked about: a Job retention may not take keeps
	// everything it points at.
	if !jobExists(t, deps, pinned.ID) {
		t.Fatal("a pinned Job was pruned")
	}
	if rows := countRows(t, deps, &models.JobOutput{}, "job_id = ?", pinned.ID); rows != 1 {
		t.Fatalf("the pinned Job's artifact reference is gone: %d rows", rows)
	}
	if adapter.cleanupCount() != 2 {
		t.Fatalf("the sweep asked for cleanup %d times, want neither the pinned nor the adapter-less Job",
			adapter.cleanupCount())
	}

	if result.Pruned != 1 {
		t.Fatalf("pruned %d Jobs, want only the one whose artifacts were confirmed gone", result.Pruned)
	}
	if result.Skipped < 3 {
		t.Fatalf("skipped %d Jobs, want the retained artifact, the pinned one and the adapter-less one",
			result.Skipped)
	}
}

// TestRetentionOutputExpiryRechecksTheRowItSelected is §7's "planned expiry is
// visible in advance" read as a property of the pass that records it: the update
// that records an expiry has to be about the row the pass decided about.
//
// The deadline pass selected the outputs past their deadline and then recorded
// them expired by id and availability alone. An at-least-once executor publishing
// the same key again — a fresh reference and a fresh deadline, which is exactly
// what a repeated execution does — replaces that row in the window between the
// two statements, and the update then marked the new, still-valid output expired.
// A Job would refuse a required output it can serve, and an optional one would be
// reported away to its reader.
func TestRetentionOutputExpiryRechecksTheRowItSelected(t *testing.T) {
	deps, dsn := newFileDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 8, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	running := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: running.ID, ExecutionToken: "claim-a"}
	expires := clock.Add(time.Minute)
	if _, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
		Reference: json.RawMessage(`{"path":"exports/first.tar"}`), ExpiresAt: &expires,
	}); err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}

	// The artifact's own deadline passes, so this pass selects its row.
	clock = clock.Add(time.Hour)

	// The republication runs from the pass's own selection: after the rows past
	// their deadline have been read and before the transaction that records them
	// opens, which is the window the defect lived in. It goes through a second
	// connection, because two writes on one handle would be a sequence rather
	// than an interleaving.
	other := Deps{DB: openSecondHandle(t, dsn), Now: deps.Now}
	fresh := clock.Add(24 * time.Hour)
	var once sync.Once
	const hook = "test:republish_output"
	if err := deps.DB.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "job_outputs" {
			return
		}
		once.Do(func() {
			if _, err := svc.PublishOutput(other, ref, OutputInput{
				Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
				Reference: json.RawMessage(`{"path":"exports/second.tar"}`), ExpiresAt: &fresh,
			}); err != nil {
				t.Errorf("republish the output while the pass was running: %v", err)
			}
		})
	}); err != nil {
		t.Fatalf("register the interleaving hook: %v", err)
	}
	t.Cleanup(func() { _ = deps.DB.Callback().Query().Remove(hook) })

	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	var output models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", running.ID, "artifact").First(&output).Error; err != nil {
		t.Fatalf("read the artifact output: %v", err)
	}
	if output.Availability != string(OutputAvailable) {
		t.Fatalf("the republished artifact is %s, want it still available: the pass recorded an expiry for a row it did not select",
			output.Availability)
	}
	if string(output.Reference) != `{"path":"exports/second.tar"}` {
		t.Fatalf("artifact reference = %s, want the republished one", output.Reference)
	}
	if result.Outputs != 0 {
		t.Fatalf("the pass recorded %d output availabilities, want none: its selection was replaced", result.Outputs)
	}
}

// TestRetentionSweepRecordsEachAcknowledgedArtifactRemoval is §9's "records output
// removal before pruning the relevant history" read as an obligation per artifact
// rather than per Job.
//
// A cleanup acknowledges what it removed and what it retained, and the sweep uses
// that answer for two different decisions: only a complete accounting may prune
// the history, and every acknowledged removal is a fact about an artifact whatever
// the history that names it does next. Reducing the answer to one boolean lost the
// removals in between — an artifact whose bytes were deleted stayed advertised as
// available for as long as its Job kept its history, which for an artifact with no
// expiry of its own is forever. The same loss happens when every artifact goes but
// the metadata cannot: a pin landing while the cleanup runs is enough.
func TestRetentionSweepRecordsEachAcknowledgedArtifactRemoval(t *testing.T) {
	deps, dsn := newFileDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 9, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		// One artifact of every Job is left where it is; the rest are gone.
		result := ArtifactCleanupResult{}
		for _, artifact := range request.Artifacts {
			if artifact.Key == "artifact-kept" {
				result.Retained = append(result.Retained, artifact.Key)
				continue
			}
			result.Removed = append(result.Removed, artifact.Key)
		}
		return result, nil
	}

	settle := func(title string, artifactKeys ...string) Snapshot {
		t.Helper()
		clock = clock.Add(time.Minute)
		accepted := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7), Replay: ReplayInput{NonReplayable: true},
		})
		execution, ok := claimOnce(t, svc, deps, "runtime-a")
		if !ok {
			t.Fatalf("claim for %s: nothing was claimed", title)
		}
		if execution.JobID != accepted.ID {
			t.Fatalf("claimed %s while settling %s", execution.JobID, title)
		}
		for _, key := range artifactKeys {
			if _, err := execution.Output(OutputInput{
				Key: key, Type: OutputTypeArtifact, Label: "group-export.tar",
				Reference: json.RawMessage(`{"path":"exports/` + key + `.tar"}`),
			}); err != nil {
				t.Fatalf("publish %s of %s: %v", key, title, err)
			}
		}
		finished, err := execution.Finish(FinishRequest{ExpectedVersion: execution.Version, Outcome: StateSucceeded})
		if err != nil {
			t.Fatalf("finish %s: %v", title, err)
		}
		return finished
	}

	partial := settle("one of two artifacts is gone", "artifact-gone", "artifact-kept")
	pinnedDuring := settle("pinned while its artifacts were being cleaned up", "artifact")

	// The pin lands between the sweep's own protection read and the cleanup that
	// records the removals: after the Job has been decided about as unpinned and
	// before anything of its own is written. It is driven from the accounting read
	// of that Job rather than from inside the Kind's cleanup, because the cleanup
	// now runs under the Job's own row — a write from there is the cleanup waiting
	// for itself rather than an interleaving — and it goes through a second
	// connection, because two writes on one handle would be a sequence.
	pinner := Deps{DB: openSecondHandle(t, dsn), Now: deps.Now}
	var once sync.Once
	const hook = "test:pin-before-cleanup"
	if err := deps.DB.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "job_outputs" {
			return
		}
		for _, variable := range tx.Statement.Vars {
			if named, ok := variable.(string); ok && named == pinnedDuring.ID {
				once.Do(func() {
					if err := svc.SetPreference(pinner, Access{UserID: 7},
						PreferenceRequest{JobID: pinnedDuring.ID, Pinned: boolPtr(true)}); err != nil {
						t.Errorf("pin the Job while the sweep was reading its artifacts: %v", err)
					}
				})
				return
			}
		}
	}); err != nil {
		t.Fatalf("register the interleaving hook: %v", err)
	}
	t.Cleanup(func() { _ = deps.DB.Callback().Query().Remove(hook) })

	clock = clock.Add(48 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	// Neither Job's history may go: one still has an artifact, and the other was
	// pinned while its cleanup was running.
	if !jobExists(t, deps, partial.ID) || !jobExists(t, deps, pinnedDuring.ID) {
		t.Fatal("a Job retention may not take was pruned")
	}

	removedIsRecorded := func(job Snapshot, key string) {
		t.Helper()
		var output models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", job.ID, key).First(&output).Error; err != nil {
			t.Fatalf("%s lost its %s reference: %v", job.Title, key, err)
		}
		if output.Availability != string(OutputRemoved) {
			t.Fatalf("%s/%s is %s, want removed: the cleanup acknowledged it gone", job.Title, key, output.Availability)
		}
		if output.RemovedAt == nil {
			t.Fatalf("%s/%s carries no removal instant", job.Title, key)
		}
		if events := countRows(t, deps, &models.JobEvent{}, "job_id = ? AND type = ?", job.ID, EventOutputRemoved); events != 1 {
			t.Fatalf("%s recorded %d artifact-removal events, want one", job.Title, events)
		}
	}
	removedIsRecorded(partial, "artifact-gone")
	removedIsRecorded(pinnedDuring, "artifact")

	// A retained artifact is untouched: a removal is a fact about one artifact,
	// not about the Job that published it.
	var kept models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", partial.ID, "artifact-kept").First(&kept).Error; err != nil {
		t.Fatalf("read the retained artifact: %v", err)
	}
	if kept.Availability != string(OutputAvailable) {
		t.Fatalf("the retained artifact is %s, want it still advertised", kept.Availability)
	}
	if result.Outputs != 2 {
		t.Fatalf("recorded %d output availabilities, want the two acknowledged removals", result.Outputs)
	}
	if result.Pruned != 0 {
		t.Fatalf("pruned %d Jobs, want none", result.Pruned)
	}

	// A second pass records nothing more: a recorded removal is a durable fact,
	// not one event per sweep.
	second := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)
	if second.Outputs != 0 {
		t.Fatalf("the second pass recorded %d more output availabilities, want none", second.Outputs)
	}
	if events := countRows(t, deps, &models.JobEvent{}, "job_id = ? AND type = ?", partial.ID, EventOutputRemoved); events != 1 {
		t.Fatalf("the second pass recorded the same removal again: %d events", events)
	}
}

// TestRetentionExpiryRollsBackItsRowWhenTheEventCannotBeRecorded is the atomic
// half of §7's "confirmed expiry records a Job Event": the availability change and
// the fact that says so are one write, so a failure while the event is stored must
// leave the artifact advertised exactly as it was — never an output that silently
// changed state with nothing on the timeline saying why.
func TestRetentionExpiryRollsBackItsRowWhenTheEventCannotBeRecorded(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 10, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	running := seededExecution(t, deps, StateRunning, "claim-a")
	expires := clock.Add(time.Minute)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: running.ID, ExecutionToken: "claim-a"}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
		Reference: json.RawMessage(`{"path":"exports/17.tar"}`), ExpiresAt: &expires,
	}); err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}
	clock = clock.Add(time.Hour)

	// The event store fails while the pass is recording the expiry.
	var once sync.Once
	const hook = "test:fail_event_store"
	if err := deps.DB.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "job_events" {
			return
		}
		once.Do(func() { tx.AddError(errors.New("the event store is unavailable")) })
	}); err != nil {
		t.Fatalf("register the failing event store: %v", err)
	}
	t.Cleanup(func() { _ = deps.DB.Callback().Create().Remove(hook) })

	if _, err := svc.Sweep(deps, policy, SweepCursor{}, 100); err == nil {
		t.Fatal("a sweep that could not record an expiry reported success")
	}

	var output models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", running.ID, "artifact").First(&output).Error; err != nil {
		t.Fatalf("read the artifact output: %v", err)
	}
	if output.Availability != string(OutputAvailable) {
		t.Fatalf("artifact availability = %s, want the write rolled back with the event it belongs to",
			output.Availability)
	}
	if rows := countRows(t, deps, &models.JobOutput{}, "job_id = ? AND availability = ?", running.ID, string(OutputExpired)); rows != 0 {
		t.Fatalf("an unrecorded expiry left %d expired rows", rows)
	}
}

// TestRetentionExpiredArtifactIsRemovedWhateverTheJobsMetadataSays is §9's
// "sweep work records output removal before pruning the relevant history" read
// from the artifact's own side: an artifact has its own deadline and its own
// retention, so the bytes must go when that deadline passes — whether the Job's
// metadata window is a month out, or the Job is pinned and about to be kept
// forever.
//
// Artifact cleanup used to be reachable only from the metadata pass: it ran for a
// Job that was already due to be pruned, after pins and unresolved claims had been
// filtered out. So a one-hour artifact of a Job with thirty days of history kept
// its bytes for thirty days, and a pinned Job kept them indefinitely — a pin is not
// a retention policy for what a Job produced (§7).
func TestRetentionExpiredArtifactIsRemovedWhateverTheJobsMetadataSays(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 11, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, testDefinition())
	var askedMu sync.Mutex
	var askedJobs []string
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		askedMu.Lock()
		askedJobs = append(askedJobs, request.JobID)
		askedMu.Unlock()
		result := ArtifactCleanupResult{}
		for _, artifact := range request.Artifacts {
			result.Removed = append(result.Removed, artifact.Key)
		}
		return result, nil
	}

	settle := func(title string) Snapshot {
		t.Helper()
		clock = clock.Add(time.Minute)
		accepted := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7), Replay: ReplayInput{NonReplayable: true},
		})
		execution, ok := claimOnce(t, svc, deps, "runtime-a")
		if !ok {
			t.Fatalf("claim for %s: nothing was claimed", title)
		}
		if execution.JobID != accepted.ID {
			t.Fatalf("claimed %s while settling %s", execution.JobID, title)
		}
		// The artifact promises an hour, which is far inside the Job's thirty days.
		expires := clock.Add(time.Hour)
		if _, err := execution.Output(OutputInput{
			Key: "artifact", Type: OutputTypeArtifact, Label: "group-export.tar",
			Reference: json.RawMessage(`{"path":"exports/17.tar"}`), ExpiresAt: &expires,
		}); err != nil {
			t.Fatalf("publish artifact of %s: %v", title, err)
		}
		finished, err := execution.Finish(FinishRequest{ExpectedVersion: execution.Version, Outcome: StateSucceeded})
		if err != nil {
			t.Fatalf("finish %s: %v", title, err)
		}
		return finished
	}

	ordinary := settle("an artifact past its own deadline")
	pinned := settle("a pinned Job's artifact past its own deadline")
	if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: pinned.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// An artifact whose deadline has passed on a Job an unresolved claim still
	// protects: the one thing §9 says no expiry may write through, and the bytes
	// stay where they are with everything else about that execution.
	protected := settle("a claim-protected Job's artifact past its own deadline")
	// The claim its own execution released becomes one nobody could resolve: the
	// state a quarantined execution leaves behind when the process that held it
	// never came back.
	if err := deps.DB.Model(&models.JobClaim{}).Where("job_id = ?", protected.ID).
		Updates(map[string]any{
			"state":            models.JobClaimStateQuarantined,
			"lease_expires_at": clock,
		}).Error; err != nil {
		t.Fatalf("quarantine the claim: %v", err)
	}

	// The artifacts' deadlines pass; no Job's metadata window is anywhere near its
	// own.
	clock = clock.Add(2 * time.Hour)
	result := sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	removedIsRecorded := func(job Snapshot, title string) {
		t.Helper()
		var output models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", job.ID, "artifact").First(&output).Error; err != nil {
			t.Fatalf("%s lost its artifact reference: %v", title, err)
		}
		if output.Availability != string(OutputRemoved) || output.RemovedAt == nil {
			t.Fatalf("%s/artifact is %s (removed at %v), want the bytes gone and recorded",
				title, output.Availability, output.RemovedAt)
		}
		if events := countRows(t, deps, &models.JobEvent{}, "job_id = ? AND type = ?", job.ID, EventOutputRemoved); events != 1 {
			t.Fatalf("%s recorded %d artifact-removal events, want one", title, events)
		}
	}
	removedIsRecorded(ordinary, "an artifact past its own deadline")
	removedIsRecorded(pinned, "a pinned Job's artifact")

	// The pinned Job's history is exactly what the pin exempts, and it is still
	// there: the artifact went on its own schedule, the metadata stayed.
	if !jobExists(t, deps, pinned.ID) {
		t.Fatal("removing a pinned Job's expired artifact pruned the Job the pin protects")
	}
	if rows := countRows(t, deps, &models.JobEvent{}, "job_id = ?", pinned.ID); rows == 0 {
		t.Error("a pinned Job's events were pruned")
	}

	// The claim-protected Job was not asked about at all, and keeps its artifact
	// reference: a claim nothing could prove dead is not a license to delete what
	// the execution may still be producing.
	var protectedOutput models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", protected.ID, "artifact").First(&protectedOutput).Error; err != nil {
		t.Fatalf("the claim-protected Job lost its artifact reference: %v", err)
	}
	if protectedOutput.Availability != string(OutputExpired) {
		t.Fatalf("claim-protected artifact availability = %s, want the deadline it was published with recorded",
			protectedOutput.Availability)
	}
	if protectedOutput.RemovedAt != nil {
		t.Fatal("a Job an unresolved claim still protects had its artifact removed")
	}
	if result.Pruned != 0 {
		t.Fatalf("the sweep pruned %d Jobs, want none: no Job's metadata window has passed", result.Pruned)
	}

	// The cleanup was asked about exactly the two artifacts whose own deadline had
	// passed — never the claim-protected Job's, which is still in the hands of an
	// execution nobody could prove had stopped.
	askedMu.Lock()
	defer askedMu.Unlock()
	if len(askedJobs) != 2 {
		t.Fatalf("the sweep asked for cleanup %d times, want the two artifacts past their deadline: %v",
			len(askedJobs), askedJobs)
	}
	for _, jobID := range askedJobs {
		if jobID == protected.ID {
			t.Fatal("the sweep asked about a Job an unresolved claim still protects")
		}
	}
}

// TestRetentionArtifactCleanupDoesNotDeleteARepublishedArtifact is the window the
// deletion fence does not cover: a candidate selected, then replaced, before the
// pass reaches the Job's row.
//
// The pass selects the artifacts whose own deadline has passed outside any
// transaction, and a queued Job carrying one is claimable at any instant. An
// execution that claims it publishes the same key again — the same bytes, since a
// Kind that produced the export once produces the same file — with a deadline of
// its own, and finishes, which releases the claim the fence looks for. The Job the
// fence admits is therefore no longer the Job the artifacts were selected from,
// and the reference the pass is holding names bytes an output still promises.
// Deleting them is the defect the version guard on the recording cannot undo: it
// can only decline to record a deletion that already happened.
func TestRetentionArtifactCleanupDoesNotDeleteARepublishedArtifact(t *testing.T) {
	deps, dsn := newFileDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 16, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	// The runtime's writes run on their own connection, so the republication is an
	// interleaving rather than a step of the pass.
	other := Deps{DB: openSecondHandle(t, dsn), Now: deps.Now, Retention: deps.Retention}

	runArtifactCleanupRepublication(t, svc, deps, other, policy)
}

// runArtifactCleanupRepublication drives the selection → claim/republication/finish
// → cleanup interleaving for one engine. deps is the connection the pass reads on
// and other is where the runtime's writes run: a second handle on SQLite, the
// engine's own pool on PostgreSQL.
func runArtifactCleanupRepublication(t *testing.T, svc *Service, deps Deps, other Deps, policy RetentionPolicy) {
	t.Helper()
	now := deps.Now()

	// The artifact's bytes. The Kind's cleanup deletes the file it is asked about,
	// so "the artifact was deleted" is a fact about the bytes rather than about a
	// column — the column says what a stale decision left behind either way.
	bytes := filepath.Join(t.TempDir(), "export.tar")
	if err := os.WriteFile(bytes, []byte("the export"), 0o600); err != nil {
		t.Fatalf("write the artifact: %v", err)
	}
	reference := json.RawMessage(fmt.Sprintf(`{"path":%q}`, bytes))

	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		result := ArtifactCleanupResult{}
		for _, artifact := range request.Artifacts {
			// The Kind deletes the file its reference names, so what the bytes
			// assertion above reads is whether anything that still promises them asked
			// for them to go.
			var published struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(artifact.Reference, &published); err == nil && published.Path == bytes {
				if err := os.Remove(bytes); err != nil {
					t.Errorf("remove the artifact the Kind was asked about: %v", err)
				}
			}
			result.Removed = append(result.Removed, artifact.Key)
		}
		return result, nil
	}

	// A queued Job — claimable, by definition — whose artifact's own deadline has
	// already passed.
	accepted := acceptQueued(t, svc, deps, nil)
	deadline := now.Add(-time.Hour)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: accepted.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: reference, ExpiresAt: &deadline,
	}); err != nil {
		t.Fatalf("publish the artifact: %v", err)
	}

	// The window: the pass has read the output rows past their deadline and has not
	// yet reached the Job's row, so the claim this runtime takes is admitted and the
	// publication replaces the row the pass is holding.
	fresh := now.Add(24 * time.Hour)
	var once sync.Once
	republished := false
	const hook = "test:republish-artifact-under-cleanup"
	if err := deps.DB.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		// The pass's artifact selection is the only query here that names the
		// deferral column, which is what places this write after it and before the
		// transaction that acts on it.
		if tx.Statement == nil || tx.Statement.Table != "job_outputs" ||
			!strings.Contains(tx.Statement.SQL.String(), "next_cleanup_at") {
			return
		}
		once.Do(func() {
			execution, ok, err := svc.Claim(context.Background(), other, ClaimRequest{
				Kind: testKind, KindVersion: 1, Claimant: "runtime-b",
			})
			if err != nil || !ok {
				t.Errorf("claim the Job the pass was about to clean up: claimed %v, %v", ok, err)
				return
			}
			ref := ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken}
			if _, err := svc.PublishOutput(other, ref, OutputInput{
				Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
				Reference: reference, ExpiresAt: &fresh,
			}); err != nil {
				t.Errorf("republish the artifact while the pass was running: %v", err)
				return
			}
			if _, err := svc.Finish(other, FinishRequest{
				ExecutionRef: ref, ExpectedVersion: execution.Version, Outcome: StateSucceeded,
			}); err != nil {
				t.Errorf("finish the Job the pass was about to clean up: %v", err)
				return
			}
			republished = true
		})
	}); err != nil {
		t.Fatalf("register the interleaving hook: %v", err)
	}
	t.Cleanup(func() { _ = deps.DB.Callback().Query().Remove(hook) })

	sweepFor(t, svc, deps, policy, SweepCursor{}, 100)

	if !republished {
		t.Fatal("the interleaving never ran: the pass did not select an artifact past its deadline")
	}
	if _, err := os.Stat(bytes); err != nil {
		t.Fatalf("the artifact republished while the pass ran was deleted: %v", err)
	}
	var artifact models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", accepted.ID, "artifact").First(&artifact).Error; err != nil {
		t.Fatalf("read the artifact: %v", err)
	}
	if artifact.Availability != string(OutputAvailable) || artifact.RemovedAt != nil {
		t.Fatalf("the artifact the pass skipped is %s (removed at %v), want it still available",
			artifact.Availability, artifact.RemovedAt)
	}
	if artifact.ExpiresAt == nil || !artifact.ExpiresAt.Equal(fresh) {
		t.Fatalf("artifact deadline = %v, want the republication's %v", artifact.ExpiresAt, fresh)
	}
	if asked := adapter.cleanupCount(); asked != 0 {
		t.Fatalf("the Kind was asked to clean up %d artifact sets, want none: nothing the pass decided about is left", asked)
	}
}

// TestRetentionArtifactCleanupReachesTheArtifactsBehindOneItCannotRemove is §9's
// "cleanup is bounded and resumable" read as fairness rather than as a single
// batch.
//
// The artifact pass takes the oldest expired candidates, one batch at a time, and
// two kinds of candidate it cannot act on do not move: a Job an unresolved claim
// protects, whose bytes no expiry may go around, and an artifact no adapter could
// account for, whose Kind this process cannot run at all. Either of them sat at the
// head of every pass, so with a batch of one the artifacts behind it were never
// reached at all — not late, never.
func TestRetentionArtifactCleanupReachesTheArtifactsBehindOneItCannotRemove(t *testing.T) {
	t.Run("a Job an unresolved claim protects", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		policy := expiredHistory(30 * 24 * time.Hour)
		deps.Retention = &policy
		clock := time.Date(2031, 7, 13, 9, 0, 0, 0, time.UTC)
		deps.Now = func() time.Time { return clock }

		adapter := registerTestAdapter(t, svc, testDefinition())
		var askedMu sync.Mutex
		var asked []string
		adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
			askedMu.Lock()
			asked = append(asked, request.JobID)
			askedMu.Unlock()
			result := ArtifactCleanupResult{}
			for _, artifact := range request.Artifacts {
				result.Removed = append(result.Removed, artifact.Key)
			}
			return result, nil
		}

		expireArtifact := func(t *testing.T, job Snapshot, at time.Time) {
			t.Helper()
			if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: job.ID}, OutputInput{
				Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
				Reference: json.RawMessage(`{"name":"export.tar"}`), ExpiresAt: &at,
			}); err != nil {
				t.Fatalf("publish artifact of %s: %v", job.Title, err)
			}
		}

		// The older artifact belongs to a Job an unresolved claim protects: the one
		// thing §9 says no expiry may write through.
		protected := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: "a claim-protected Job", Replay: ReplayInput{NonReplayable: true},
		})
		expireArtifact(t, protected, clock.Add(-2*time.Hour))
		// The claim its own execution would have released becomes one nobody could
		// resolve: the state a quarantined execution leaves behind when the process
		// that held it never came back.
		if err := deps.DB.Create(&models.JobClaim{
			JobID: protected.ID, Kind: testKind, KindVersion: 1,
			Claimant: "host:gone", ExecutionToken: "00000000-0000-7000-8000-000000000002",
			State:     models.JobClaimStateQuarantined,
			ClaimedAt: clock, HeartbeatAt: clock, LeaseExpiresAt: clock,
			CreatedAt: clock, UpdatedAt: clock,
		}).Error; err != nil {
			t.Fatalf("seed the quarantined claim: %v", err)
		}

		// The later artifact belongs to a Job that is pinned — which §9 exempts
		// metadata and events for, and nothing else — so its bytes are due.
		pinned := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: "a pinned Job", Replay: ReplayInput{NonReplayable: true},
		})
		expireArtifact(t, pinned, clock.Add(-time.Hour))
		if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: pinned.ID, Pinned: boolPtr(true)}); err != nil {
			t.Fatalf("pin: %v", err)
		}

		// Both artifacts are past their own deadline, and the pass is allowed to act
		// on one: the protected Job's is older, so it is the head of the batch.
		result := sweepFor(t, svc, deps, policy, SweepCursor{}, 1)

		var cleanable models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", pinned.ID, "artifact").First(&cleanable).Error; err != nil {
			t.Fatalf("read the pinned Job's artifact: %v", err)
		}
		if cleanable.Availability != string(OutputRemoved) || cleanable.RemovedAt == nil {
			t.Fatalf("the artifact behind the protected one is %s (removed at %v), want the bytes gone and recorded",
				cleanable.Availability, cleanable.RemovedAt)
		}
		if result.Outputs != 1 {
			t.Fatalf("the pass recorded %d output availabilities, want the one removal", result.Outputs)
		}

		// And the protected Job was never asked about: it keeps its artifact
		// reference, its bytes and the claim nothing could prove dead.
		var kept models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", protected.ID, "artifact").First(&kept).Error; err != nil {
			t.Fatalf("the protected Job lost its artifact reference: %v", err)
		}
		if kept.RemovedAt != nil {
			t.Fatal("a Job an unresolved claim still protects had its artifact removed")
		}
		askedMu.Lock()
		defer askedMu.Unlock()
		for _, jobID := range asked {
			if jobID == protected.ID {
				t.Fatal("the pass asked about a Job an unresolved claim still protects")
			}
		}
	})

	t.Run("an artifact no adapter can account for", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		policy := expiredHistory(30 * 24 * time.Hour)
		deps.Retention = &policy
		clock := time.Date(2031, 7, 14, 9, 0, 0, 0, time.UTC)
		deps.Now = func() time.Time { return clock }

		// One Kind this process can run, and one it has no adapter for at all: an
		// artifact of an unregistered Kind can never be established as gone here.
		adapter := registerTestAdapter(t, svc, testDefinition())

		expireArtifact := func(t *testing.T, job Snapshot, at time.Time) {
			t.Helper()
			if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: job.ID}, OutputInput{
				Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
				Reference: json.RawMessage(`{"name":"export.tar"}`), ExpiresAt: &at,
			}); err != nil {
				t.Fatalf("publish artifact of %s: %v", job.Title, err)
			}
		}

		unaccountable := acceptFor(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: "a Kind this process cannot run",
			Replay: ReplayInput{NonReplayable: true},
		})
		expireArtifact(t, unaccountable, clock.Add(-2*time.Hour))
		cleanable := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: "a Kind this process can run",
			Replay: ReplayInput{NonReplayable: true},
		})
		expireArtifact(t, cleanable, clock.Add(-time.Hour))

		// The pass that can only ask about the unaccountable artifact gets no
		// further than it: nothing is removed and nothing is pruned.
		first := sweepFor(t, svc, deps, policy, SweepCursor{}, 1)
		if first.Outputs != 0 {
			t.Fatalf("the first pass recorded %d output availabilities, want none", first.Outputs)
		}
		if adapter.cleanupCount() != 0 {
			t.Fatalf("the unaccountable Kind's artifact was handed to another Kind's adapter")
		}

		// The next pass reaches past it: a candidate that could not be cleaned
		// waits its turn instead of holding the head of every batch.
		sweepFor(t, svc, deps, policy, SweepCursor{}, 1)

		var removed models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", cleanable.ID, "artifact").First(&removed).Error; err != nil {
			t.Fatalf("read the cleanable Job's artifact: %v", err)
		}
		if removed.Availability != string(OutputRemoved) || removed.RemovedAt == nil {
			t.Fatalf("the artifact behind the unaccountable one is %s (removed at %v), want the bytes gone and recorded",
				removed.Availability, removed.RemovedAt)
		}

		// The artifact nobody could account for is untouched and still advertised
		// as expired rather than gone.
		var kept models.JobOutput
		if err := deps.DB.Where("job_id = ? AND key = ?", unaccountable.ID, "artifact").First(&kept).Error; err != nil {
			t.Fatalf("read the unaccountable Job's artifact: %v", err)
		}
		if kept.RemovedAt != nil {
			t.Fatal("an artifact no adapter accounted for was recorded as removed")
		}
	})
}

// TestRetentionArtifactCleanupIsFencedAgainstANewClaim is the ordering half of the
// same contract, and it is the one a check before the deletion cannot provide.
//
// An artifact's own deadline is not the Job's: a queued Job can carry an expired
// artifact from an earlier attempt, and a runtime may claim it at any instant. A
// claim is a new execution, and a new execution publishes — the same key again, if
// the Kind does. So the decision to delete and the deletion itself have to be
// admitted under the same row a claim is admitted under, or the bytes are deleted
// underneath an execution that has just taken the Job.
func TestRetentionArtifactCleanupIsFencedAgainstANewClaim(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(30 * 24 * time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 15, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, testDefinition())

	// A queued Job — claimable, by definition — whose artifact's own deadline has
	// already passed.
	accepted := acceptQueued(t, svc, deps, nil)
	expires := clock.Add(-time.Hour)
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: accepted.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"name":"export.tar"}`), ExpiresAt: &expires,
	}); err != nil {
		t.Fatalf("publish artifact: %v", err)
	}

	// The cleanup is where the pass is inside its deletion: while it runs, the
	// runtime claims the Job.
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

	var orderMu sync.Mutex
	var order []string
	record := func(what string) {
		orderMu.Lock()
		order = append(order, what)
		orderMu.Unlock()
	}

	swept := make(chan error, 1)
	go func() {
		_, err := svc.Sweep(deps, policy, SweepCursor{}, 100)
		swept <- err
	}()
	<-inCleanup

	claimed := make(chan error, 1)
	go func() {
		_, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
			Kind: testKind, KindVersion: 1, Claimant: "runtime-b",
		})
		if err == nil && !ok {
			err = fmt.Errorf("the queued Job was not claimable")
		}
		if err == nil {
			record("claimed")
		}
		claimed <- err
	}()

	select {
	case err := <-claimed:
		t.Fatalf("a claim committed while the artifact's bytes were still being deleted: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	record("cleanup")
	close(releaseCleanup)

	if err := <-swept; err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if err := <-claimed; err != nil {
		t.Fatalf("Claim: %v", err)
	}

	orderMu.Lock()
	defer orderMu.Unlock()
	if len(order) != 2 || order[0] != "cleanup" || order[1] != "claimed" {
		t.Fatalf("the pass and the claim ordered as %v, want the deletion finished before the claim was admitted", order)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.State != string(StateRunning) {
		t.Fatalf("state = %s, want the claim's running", stored.State)
	}
	var artifact models.JobOutput
	if err := deps.DB.Where("job_id = ? AND key = ?", accepted.ID, "artifact").First(&artifact).Error; err != nil {
		t.Fatalf("read the artifact: %v", err)
	}
	if artifact.Availability != string(OutputRemoved) {
		t.Fatalf("artifact availability = %s, want the removal the adapter acknowledged recorded", artifact.Availability)
	}
}
