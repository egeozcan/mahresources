package application_context

import (
	"errors"
	"testing"

	"mahresources/jobs"
)

// This file drives the maintenance Kind: the similarity recompute the admin surface
// offers as a background job, through the same context method the admin endpoint calls.
// Two properties are its own and not shared with the transfer Kinds — it is admin-only
// durably, and it advertises no Repeat — so both are pinned here rather than left to
// the adapter's comments.

// TestASimilarityRecomputeAcceptsAnAdminOnlyJob is the maintenance Kind's contract: the
// admin submission accepts a durable Job, that Job is admin-visible only — whatever
// account submitted it — and it is what the recompute's outcome is recorded on.
func TestASimilarityRecomputeAcceptsAnAdminOnlyJob(t *testing.T) {
	ctx := newWorkflowJobContext(t)

	legacyID, err := ctx.RecomputeSimilarities()
	if err != nil {
		t.Fatalf("submit the recompute: %v", err)
	}
	if legacyID == "" {
		t.Fatalf("the recompute answered no legacy id")
	}

	snap := jobOfKindForTest(t, ctx, JobKindSimilarityRecompute)
	if snap.Visibility != jobs.VisibilityAdmin {
		t.Fatalf("the maintenance job is %q-visible, want admin-only", snap.Visibility)
	}

	// A plain user cannot see it at all: visibility is a property of the Kind rather
	// than of who submitted it, so a Job accepted by an administrator is still not a
	// plain user's to read.
	if _, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{UserID: 4242}, snap.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("a plain user read the maintenance job: %v", err)
	}

	snap = waitForSnapshot(t, ctx, snap.ID, "the recompute to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the recompute ended %s (%+v)", snap.State, snap.Failure)
	}
}

// TestAMaintenanceJobAdvertisesNoRepeat pins the Kind's command set. A rebuild that just
// succeeded is the same work twice over the same rows, which is what the executor's own
// process-wide guard refuses — advertising a control the executor would refuse is the
// button-that-lies the advertised-command surface exists to prevent.
func TestAMaintenanceJobAdvertisesNoRepeat(t *testing.T) {
	ctx := newWorkflowJobContext(t)

	if _, err := ctx.RecomputeSimilarities(); err != nil {
		t.Fatalf("submit the recompute: %v", err)
	}
	snap := jobOfKindForTest(t, ctx, JobKindSimilarityRecompute)
	snap = waitForSnapshot(t, ctx, snap.ID, "the recompute to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the recompute ended %s (%+v)", snap.State, snap.Failure)
	}

	commands := advertisedForTest(t, ctx, snap.ID)
	if offersCommand(commands, jobs.CommandRepeat) {
		t.Fatalf("a succeeded maintenance job advertised a Repeat: %+v", commands)
	}
	if offersCommand(commands, jobs.CommandRetry) {
		t.Fatalf("a succeeded maintenance job advertised a Retry: %+v", commands)
	}
}
