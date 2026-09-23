package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
)

// This file holds the deployment half of the quiescence rule: what a quarantine is
// for, that nothing releases one on a decision nobody could make, and that it is
// resolved when the proof finally arrives.
//
// The two facts it pins are the two halves of §3's claim: a claim nobody can resolve
// keeps its token and its deployment-wide capacity, and a claim whose runtime is
// *proved* gone — this host rebooted, the process no longer exists — is reconciled and
// its capacity comes back. The proof is only ever the adapter's answer, so the pass
// this drives asks the Kind again rather than deciding anything itself.

// jobTokenForTest reads the fencing token a Job's row records.
func jobTokenForTest(t *testing.T, ctx *MahresourcesContext, jobID string) string {
	t.Helper()
	var row models.Job
	if err := ctx.db.First(&row, "id = ?", jobID).Error; err != nil {
		t.Fatalf("load job %s: %v", jobID, err)
	}
	return row.ExecutionToken
}

// claimForQuarantineForTest accepts one export Job and claims it in one runtime's name,
// occupying the deployment budget explicitly: the harness has no configured global
// budget, and the capacity a quarantine holds is half of what these tests are about.
func claimForQuarantineForTest(t *testing.T, ctx *MahresourcesContext, handle string, claimant string, lease time.Duration) jobs.Snapshot {
	t.Helper()
	groupID := createExportGroupForTest(t, ctx, "quarantine-"+handle)
	input, err := json.Marshal(exportJobInput{Request: *exportRequestForTest(groupID)})
	if err != nil {
		t.Fatalf("encode the export input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind:        JobKindGroupExport,
		KindVersion: jobExportKindVersion,
		State:       jobs.StateQueued,
		Origin:      "api",
		Title:       "Group export",
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: GroupExportHandleNamespace, Handle: handle}},
	})
	_, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind:        JobKindGroupExport,
		KindVersion: jobExportKindVersion,
		JobID:       accepted.ID,
		Claimant:    claimant,
		Lease:       lease,
		Capacity:    []jobs.CapacityRef{{Group: jobs.CapacityGroupGlobal, Limit: 4}},
	})
	if err != nil || !claimed {
		t.Fatalf("claim the export as %q: claimed=%v err=%v", claimant, claimed, err)
	}
	return accepted
}

// TestAHeldExportIsQuarantinedRatherThanReleasedByAReconcilerWithNoKey is the finding
// itself: a second process that cannot read the Job's sealed input decides nothing
// about it.
//
// The input is what a reconciliation of this Kind is made of, and the process holding
// no key for it cannot tell a live export from a dead one. Handing the adapter a nil
// input and applying its "I could not decode this" answer released the claim, the token
// and the deployment-wide capacity of a worker that may still be exporting — the
// duplicate dispatch §3 forbids, on the strength of a read that never happened.
func TestAHeldExportIsQuarantinedRatherThanReleasedByAReconcilerWithNoKey(t *testing.T) {
	ctx := newClaimableExportContext(t)
	accepted := claimForQuarantineForTest(t, ctx, "export-nokey-1", "export-holder", 20*time.Millisecond)
	time.Sleep(40 * time.Millisecond)

	// A reconciler whose own controls hold no key for the envelope: a rotation not yet
	// rolled out, a key file lost with the data root.
	blind := jobs.Deps{DB: ctx.db}
	report, err := ctx.JobService().ReconcileExpired(context.Background(), blind, "runtime-without-the-key", 32)
	if err != nil {
		t.Fatalf("reconcile without the key: %v", err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != jobs.ReconcileExternalWorkUnproven {
		t.Fatalf("a hold nobody could decide was reported as %+v", report.Outcomes)
	}

	held, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatalf("read the held export: %v", err)
	}
	if held.State != jobs.StateBlocked {
		t.Fatalf("the export is %s, want blocked", held.State)
	}
	claim := storedClaim(t, ctx, accepted.ID)
	if claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("the claim is %s, want quarantined: a live export may still own it", claim.State)
	}
	if token := jobTokenForTest(t, ctx, accepted.ID); token == "" || claim.ExecutionToken != token {
		t.Fatalf("the quarantined claim's token %q and the job's %q disagree", claim.ExecutionToken, token)
	}
	if slots := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); slots != 1 {
		t.Fatalf("the deployment budget holds %d slots, want the quarantined claim's one", slots)
	}
	// The hold is a hold: §4's resume is what hands work back, and it may only be
	// offered once nothing unresolved owns the Job.
	if commands := advertisedForTest(t, ctx, accepted.ID); offersCommand(commands, jobs.CommandResume) {
		t.Fatalf("a Job whose live execution nobody could resolve offers a Resume: %+v", commands)
	}
}

// TestARestartedRuntimeResolvesAQuarantineWhoseRuntimeIsProvedGone is the recovery
// edge, driven through the runtime seam a deployment actually has.
//
// The quarantine above has no resolution at all without this: the claim leaves the
// expiry scan, a Resume is refused while it stands, and the capacity it holds is
// deployment-wide — so a process that dies mid-quarantine takes a slot out of the
// budget for every Kind, for ever. The restart's own reconciliation tick is what
// resolves it, because the claim's recorded runtime is now provably gone and the
// adapter is the only thing that may say so.
func TestARestartedRuntimeResolvesAQuarantineWhoseRuntimeIsProvedGone(t *testing.T) {
	ctx := newClaimableExportContext(t)
	accepted := claimForQuarantineForTest(t, ctx, "export-nokey-2", goneRuntimeIdentityForTest(), 20*time.Millisecond)
	time.Sleep(40 * time.Millisecond)

	blind := jobs.Deps{DB: ctx.db}
	if _, err := ctx.JobService().ReconcileExpired(context.Background(), blind, "runtime-without-the-key", 32); err != nil {
		t.Fatalf("reconcile without the key: %v", err)
	}
	if claim := storedClaim(t, ctx, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("the claim is %s, want quarantined before the restart", claim.State)
	}

	// The deployment restarts: one runtime with the key, one tick. It reconciles what
	// it finds before it claims anything, which is the order the deployed loop uses.
	restarted := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{
		Claimant: "restarted-runtime", Interval: time.Hour,
		GlobalCapacity: 4,
	})
	restarted.tick(context.Background())

	// The quarantine is resolved, the export now runs, and the capacity comes back when
	// it ends: work that was stuck for ever is work the deployment is doing again.
	succeeded := waitForSnapshot(t, ctx, accepted.ID, "the quarantined export to run and finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if succeeded.State != jobs.StateSucceeded {
		t.Fatalf("the recovered export ended %s (%+v)", succeeded.State, succeeded.Failure)
	}
	if slots := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); slots != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the work ended", slots)
	}
}

// TestALiveReductionIsNotQueuedOverByAnExpiredClaim is the clustering Kind's half of
// the same rule, and the one a domain row makes easy to get backwards.
//
// A Reduction at `computing` is a run somebody is executing right now — in this process
// or in another one, and nothing in memory can tell the two apart. Queuing the Job
// again releases its claim and its capacity, starts a replacement that the row's own
// compare-and-set then refuses as busy (blocking the Job for a person), and leaves the
// original worker free to land the plan it was computing. The Generation fence stops
// some stale *writes*; it proves nothing about the worker, and it does not give the
// capacity back.
func TestALiveReductionIsNotQueuedOverByAnExpiredClaim(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	reduction := createReductionRowForTest(t, ctx, `{"clusters":[]}`, models.ReductionStatusComputing)

	input, err := json.Marshal(reductionComputeJobInput{ReductionID: reduction.ID, Version: reduction.Version})
	if err != nil {
		t.Fatalf("encode the clustering input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind:        JobKindReductionCompute,
		KindVersion: jobReductionKindVersion,
		State:       jobs.StateQueued,
		Origin:      "api",
		Title:       "Cluster a Resource Reduction",
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: ReductionComputeHandleNamespace, Handle: "compute-live-1"}},
	})
	// A runtime this reconciler cannot inspect: no identity, so no proof either way.
	if _, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind:        JobKindReductionCompute,
		KindVersion: jobReductionKindVersion,
		JobID:       accepted.ID,
		Claimant:    "a-clustering-runtime-on-another-host",
		Lease:       20 * time.Millisecond,
		Capacity:    []jobs.CapacityRef{{Group: jobs.CapacityGroupGlobal, Limit: 4}},
	}); err != nil || !claimed {
		t.Fatalf("claim the clustering run: claimed=%v err=%v", claimed, err)
	}
	time.Sleep(40 * time.Millisecond)

	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcileExternalWorkUnproven {
		t.Fatalf("a Reduction still being computed was decided %q, want it left unresolved", decision)
	}
	held, err := ctx.GetJob(accepted.ID)
	if err != nil {
		t.Fatalf("read the clustering job: %v", err)
	}
	if held.State != jobs.StateBlocked {
		t.Fatalf("the clustering job is %s, want blocked while its run may still be computing", held.State)
	}
	if claim := storedClaim(t, ctx, accepted.ID); claim.State == models.JobClaimStateReleased {
		t.Fatalf("the claim was released, so a replacement would be dispatched over a live clustering run")
	}
	if slots := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); slots != 1 {
		t.Fatalf("the deployment budget holds %d slots, want the live run's one kept", slots)
	}
}
