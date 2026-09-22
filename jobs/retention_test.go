package jobs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mahresources/models"
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
		})
	}
	// finishedAt settles a Job and reports the instant it ended.
	finishedAt := func(job Snapshot, outcome State) (Snapshot, time.Time) {
		clock = clock.Add(time.Minute)
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		clock = clock.Add(time.Minute)
		transition := Transition{JobID: job.ID, ExpectedVersion: job.Version, To: outcome}
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
	outputs, err := svc.Outputs(deps, Access{UserID: 1, Administrator: true}, job.ID)
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Availability != OutputAvailable {
		t.Fatalf("the sweep wrote to a claimed job's outputs: %+v", outputs)
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
