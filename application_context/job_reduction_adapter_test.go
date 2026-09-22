package application_context

import (
	"context"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
)

// This file drives the queue-backed Kind whose work is a domain operation rather than
// a transferred file: the Resource Reduction clustering run. It is exercised through
// the same seam its caller uses — the context method the handler calls, a real control
// plane and a running runtime — because what is worth pinning is that the *deployed*
// call accepts a durable Job at all, not that a helper exists that could.

// newWorkflowJobContext builds the context these Kinds' tests run on.
//
// It is the download adapter's harness, deliberately rather than a second one: one
// file-backed SQLite database with every table these Kinds touch, a replay keyring, a
// control plane with this context's Kinds registered and a running runtime is exactly
// what all of them need, and a copy of it would be a second place for the wiring to
// drift.
func newWorkflowJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	return newDownloadJobContext(t)
}

// jobOfKindForTest answers the one Job of a Kind this context knows about, waiting
// briefly for it to appear.
func jobOfKindForTest(t *testing.T, ctx *MahresourcesContext, kind string) jobs.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
			jobs.Filter{Kinds: []string{kind}}, jobs.Cursor{}, 10)
		if err == nil && len(page.Jobs) > 0 {
			return page.Jobs[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no durable %s job was ever accepted", kind)
	return jobs.Snapshot{}
}

// createReductionRowForTest stores one Resource Reduction. The Extent is empty so a
// clustering run over it is real — it resolves, measures, clusters and stores a plan —
// and fast; the plan document is the caller's, so a test can make the run fail by
// handing it one nothing can read.
func createReductionRowForTest(t *testing.T, ctx *MahresourcesContext, plan, status string) *models.ResourceReduction {
	t.Helper()
	reduction := &models.ResourceReduction{
		Name:   "workflow-test",
		Status: status,
		Extent: []byte(`{"resourceIds":[]}`),
		Plan:   []byte(plan),
	}
	if err := ctx.db.Create(reduction).Error; err != nil {
		t.Fatalf("create Reduction: %v", err)
	}
	return reduction
}

// offersCommand reports whether one advertisement contains a key. A client decides from
// this list and nothing else, so the list is the contract.
func offersCommand(commands []jobs.Command, key string) bool {
	for _, command := range commands {
		if command.Key == key {
			return true
		}
	}
	return false
}

// advertisedForTest answers the commands one Job offers an administrator.
func advertisedForTest(t *testing.T, ctx *MahresourcesContext, jobID string) []jobs.Command {
	t.Helper()
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(),
		jobs.Access{Administrator: true}, jobID)
	if err != nil {
		t.Fatalf("advertise the commands of %s: %v", jobID, err)
	}
	return commands
}

// TestAReductionComputeAcceptsADurableJobAndPublishesItsReduction is the whole
// dual-publication contract for a clustering run: the request accepts a durable Job
// before the queue is asked to run anything, the run's outcome is the Job's outcome,
// and what it computed is published as a typed output pointing at the Reduction.
func TestAReductionComputeAcceptsADurableJobAndPublishesItsReduction(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	reduction := createReductionRowForTest(t, ctx, `{"clusters":[]}`, models.ReductionStatusFailed)

	if _, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil); err != nil {
		t.Fatalf("request the compute: %v", err)
	}

	snap := jobOfKindForTest(t, ctx, JobKindReductionCompute)
	if snap.Kind != JobKindReductionCompute {
		t.Fatalf("the clustering job is %q, want %s", snap.Kind, JobKindReductionCompute)
	}
	snap = waitForSnapshot(t, ctx, snap.ID, "the clustering run to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the clustering run ended %s (%+v)", snap.State, snap.Failure)
	}

	outputs, err := ctx.GetJobOutputs(snap.ID)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	if _, published := findJobOutput(outputs, jobReductionOutput); !published {
		t.Fatalf("the succeeded clustering run published no Reduction output: %+v", outputs)
	}

	var stored models.ResourceReduction
	if err := ctx.db.First(&stored, reduction.ID).Error; err != nil {
		t.Fatalf("reload the Reduction: %v", err)
	}
	if stored.Status != models.ReductionStatusReady {
		t.Fatalf("the Reduction is %q after a successful run, want ready", stored.Status)
	}
}

// TestAReductionJobsRetryIsGovernedByTheRowsOwnState pins the Kind's own policy at the
// advertisement seam: a failed run offers Retry while the Reduction can be clustered
// again, and offers none once it is ready (somebody recomputed) or computing (a run is
// in flight).
//
// The row is the authority rather than the Job's state, and that is the point: a Job
// whose outcome is stale relative to its own domain row is exactly what a
// reconciliation and a concurrent recompute produce, and a Retry dispatched over a row
// another run owns is refused by the row's compare-and-set.
func TestAReductionJobsRetryIsGovernedByTheRowsOwnState(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	// An unreadable plan makes the run fail deterministically, which is what gives the
	// Job an unsuccessful outcome to advertise a Retry on.
	reduction := createReductionRowForTest(t, ctx, `{not-json`, models.ReductionStatusFailed)

	if _, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil); err != nil {
		t.Fatalf("request the compute: %v", err)
	}
	snap := jobOfKindForTest(t, ctx, JobKindReductionCompute)
	snap = waitForSnapshot(t, ctx, snap.ID, "the clustering run to fail", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateFailed {
		t.Fatalf("a run with an unreadable plan ended %s, want failed", snap.State)
	}

	// The executor recorded the failure on the row, so the row is computable again and
	// the Job offers a Retry.
	var failed models.ResourceReduction
	if err := ctx.db.First(&failed, reduction.ID).Error; err != nil {
		t.Fatalf("reload the Reduction: %v", err)
	}
	if failed.Status != models.ReductionStatusFailed {
		t.Fatalf("the Reduction is %q after a failed run, want failed", failed.Status)
	}
	if !offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
		t.Fatalf("a failed run over a computable Reduction offered no Retry: %+v",
			advertisedForTest(t, ctx, snap.ID))
	}

	// A run in flight: no Retry, because a second run over a row another execution is
	// walking is what the busy refusal exists for.
	if err := ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", reduction.ID).
		Update("status", models.ReductionStatusComputing).Error; err != nil {
		t.Fatalf("move the Reduction to computing: %v", err)
	}
	if offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
		t.Fatalf("a busy Reduction still advertised a Retry")
	}

	// Computed since: there is nothing left to compute.
	if err := ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", reduction.ID).
		Update("status", models.ReductionStatusReady).Error; err != nil {
		t.Fatalf("move the Reduction to ready: %v", err)
	}
	if offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
		t.Fatalf("a ready Reduction advertised a Retry")
	}
}
