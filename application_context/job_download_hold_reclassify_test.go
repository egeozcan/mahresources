package application_context

import (
	"encoding/json"
	"testing"

	"mahresources/jobs"
	"mahresources/models/query_models"
)

// blockDownloadForTest accepts one download Job and blocks it with the given
// phase and blocked-event detail, the way an earlier release recorded a hold.
func blockDownloadForTest(t *testing.T, ctx *MahresourcesContext, phase, detail string) jobs.Snapshot {
	t.Helper()
	input, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/held.bin"}, "")
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	blocked, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: accepted.ID, ExpectedVersion: accepted.Version, To: jobs.StateBlocked, Phase: phase,
		Event: jobs.EventInput{Type: jobs.EventBlocked, Detail: json.RawMessage(detail)},
	})
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	return blocked
}

// TestAHoldAnEarlierReleaseRecordedAsBlockedIsPaused pins what becomes of a paused
// download an earlier release stored: it was recorded blocked with the reason
// "paused", sometimes with the phase "paused" and sometimes without, and it reads
// as the paused Job it is. A block for any other reason stays blocked, and so does
// a Job whose latest block is a refusal even though it was held before.
func TestAHoldAnEarlierReleaseRecordedAsBlockedIsPaused(t *testing.T) {
	ctx := newJobHarnessContext(t, false)

	withPhase := blockDownloadForTest(t, ctx, "paused", `{"reason":"paused","resume":"restarts-from-the-beginning"}`)
	withoutPhase := blockDownloadForTest(t, ctx, "", `{"reason":"paused"}`)
	refused := blockDownloadForTest(t, ctx, "", `{"reason":"role-refused"}`)

	heldThenRefused := blockDownloadForTest(t, ctx, "paused", `{"reason":"paused"}`)
	queued, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: heldThenRefused.ID, ExpectedVersion: heldThenRefused.Version, To: jobs.StateQueued,
	})
	if err != nil {
		t.Fatalf("resume the earlier hold: %v", err)
	}
	if _, err := ctx.JobService().Transition(ctx.jobDeps(), jobs.Transition{
		JobID: queued.ID, ExpectedVersion: queued.Version, To: jobs.StateBlocked,
		Event: jobs.EventInput{Type: jobs.EventBlocked, Detail: json.RawMessage(`{"reason":"scope-refused"}`)},
	}); err != nil {
		t.Fatalf("block for a refusal: %v", err)
	}

	reclassified, err := ctx.ReclassifyDownloadHolds()
	if err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	if reclassified != 2 {
		t.Fatalf("%d holds were reclassified, want 2", reclassified)
	}
	for _, id := range []string{withPhase.ID, withoutPhase.ID} {
		snap := jobSnapshot(t, ctx.JobService(), ctx, id)
		if snap.State != jobs.StatePaused || snap.Phase != "" || snap.Progress.Message != jobDownloadPausedMessage {
			t.Fatalf("a stored hold reads %s/%s %q, want paused saying what Resume does", snap.State, snap.Phase, snap.Progress.Message)
		}
	}
	for _, id := range []string{refused.ID, heldThenRefused.ID} {
		if snap := jobSnapshot(t, ctx.JobService(), ctx, id); snap.State != jobs.StateBlocked {
			t.Fatalf("a refused download reads %s, want it still blocked", snap.State)
		}
	}

	// A second start finds nothing left to move.
	if again, err := ctx.ReclassifyDownloadHolds(); err != nil || again != 0 {
		t.Fatalf("a second pass reclassified %d (%v), want none", again, err)
	}
}
