package jobs

import (
	"context"
	"encoding/json"
	"errors"
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
	// its way past — including the artifact its history points at.
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

	// The republication runs from the pass's own update statement: after the
	// selection has been read and before the update executes, which is the window
	// the defect lived in. It goes through a second connection, because two writes
	// on one handle would be a sequence rather than an interleaving.
	other := Deps{DB: openSecondHandle(t, dsn), Now: deps.Now}
	fresh := clock.Add(24 * time.Hour)
	var once sync.Once
	const hook = "test:republish_output"
	if err := deps.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
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
	t.Cleanup(func() { _ = deps.DB.Callback().Update().Remove(hook) })

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
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2031, 7, 9, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	adapter := registerTestAdapter(t, svc, testDefinition())
	pinDuringCleanup := ""
	adapter.cleanup = func(_ context.Context, request ArtifactCleanupRequest) (ArtifactCleanupResult, error) {
		if pinDuringCleanup == request.JobID {
			if err := svc.SetPreference(deps, Access{UserID: 7},
				PreferenceRequest{JobID: request.JobID, Pinned: boolPtr(true)}); err != nil {
				return ArtifactCleanupResult{}, err
			}
		}
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
	pinDuringCleanup = pinnedDuring.ID

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
