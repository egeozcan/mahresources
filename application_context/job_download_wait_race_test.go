package application_context

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models/query_models"

	"gorm.io/gorm"
)

// The dispatch loop reads the Kind's pass-over list, then selects the oldest
// waiting Job. A duplicate the previous iteration of the same pass claimed can
// find its URL busy and go back to the queue in between: the list was read
// before it named the Job, and the select sees it queued again. This test holds
// the claim's select until the duplicate is back in the queue, which is the
// interleaving the scheduler produces now and then on its own.
func TestAWaitingDuplicateIsNotClaimedAgainByTheSamePass(t *testing.T) {
	// The hook goes in before the runtime starts and is never taken out, because
	// changing GORM's callbacks while queries run is itself a data race, and the
	// download queue's workers query until the context is torn down. The database
	// is this test's own, so the hook goes with it.
	ctx := newJobHarnessContext(t, false)
	ctx.Config.MaxJobConcurrency = 2

	var target atomic.Value
	target.Store("")
	var armed atomic.Bool
	armed.Store(true)
	var scans, heldAt atomic.Int64
	isClaimSelect := func(db *gorm.DB) bool {
		// The claim's read: waiting Jobs no claim owns (jobs.waitingJobs), oldest first.
		order, ok := db.Statement.Clauses["ORDER BY"]
		where := db.Statement.Clauses["WHERE"]
		return ok && strings.Contains(fmt.Sprint(order.Expression), "accepted_at") && db.Statement.Table == "jobs" &&
			strings.Contains(fmt.Sprint(where.Expression), "execution_token IS NULL")
	}
	stateOf := func(id string) string {
		var state string
		_ = ctx.db.Raw("SELECT state FROM jobs WHERE id = ?", id).Scan(&state).Error
		return state
	}
	var excluder interface{ ClaimExclusions() []string }
	for _, registration := range ctx.JobService().Registrations() {
		if registration.Definition.Kind == JobKindRemoteDownload {
			excluder, _ = registration.Adapter.(interface{ ClaimExclusions() []string })
		}
	}
	if excluder == nil {
		t.Fatal("the download Kind names no Jobs to pass over")
	}
	named := func(id string) bool {
		for _, excluded := range excluder.ClaimExclusions() {
			if excluded == id {
				return true
			}
		}
		return false
	}
	if err := ctx.db.Callback().Query().Before("gorm:query").Register("test:hold_claim_select", func(db *gorm.DB) {
		if !isClaimSelect(db) {
			return
		}
		n := scans.Add(1)
		id, _ := target.Load().(string)
		if id == "" || !armed.Load() || stateOf(id) != string(jobs.StateRunning) || named(id) {
			return
		}
		// The duplicate is claimed and dispatching, and not named yet, so the
		// pass-over list this claim read before its select did not name it either.
		// Let its dispatch hand it back before this select runs.
		deadline := time.Now().Add(5 * time.Second)
		for stateOf(id) != string(jobs.StateQueued) && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		// Only a hand-back that landed while this select waited is the window; a
		// timeout is not, and the next duplicate tries again.
		if stateOf(id) == string(jobs.StateQueued) {
			heldAt.Store(n)
			armed.Store(false)
		}
	}); err != nil {
		t.Fatal(err)
	}
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{
		Claimant: "download-budget-test",
		Interval: 25 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)

	server, release := gatedDownloadServer(t)
	busyURL := server.URL + "/popular.bin"
	running, _ := submitRunning(t, ctx, busyURL)

	input, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: busyURL}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The window is between the dispatch loop's read of the list and its select,
	// and the duplicate's own dispatch, which runs alongside, may name it before
	// the loop reads the list (it is all in memory, so no hook can order that).
	// A duplicate that misses the window is still asserted on; another is
	// accepted until one lands in it.
	var duplicates []string
	for attempt := 0; attempt < 20 && heldAt.Load() == 0; attempt++ {
		accepted := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
			Origin: "api", Title: "duplicate", Replay: jobs.ReplayInput{Input: input},
		})
		duplicates = append(duplicates, accepted.ID)
		target.Store(accepted.ID)
		waitForSnapshot(t, ctx, accepted.ID, "the duplicate to wait for the URL", func(snap jobs.Snapshot) bool {
			return snap.State == jobs.StateQueued && snap.Phase == downloadWaitingPhase
		})
	}
	if heldAt.Load() == 0 {
		t.Fatal("the claim's select was never held")
	}

	// The held select ran, and a later one began: whatever the held select
	// claimed has been claimed by then, and handed back again.
	deadline := time.Now().Add(10 * time.Second)
	for scans.Load() <= heldAt.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	for _, id := range duplicates {
		waitForSnapshot(t, ctx, id, "the duplicate to wait for the URL", func(snap jobs.Snapshot) bool {
			return snap.State == jobs.StateQueued && snap.Phase == downloadWaitingPhase
		})
		timeline, err := ctx.GetJobTimeline(id, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		started := 0
		for _, event := range timeline {
			if event.Type == jobs.EventStarted {
				started++
			}
		}
		if started != 1 {
			t.Fatalf("the waiting duplicate was started %d times while its URL stayed busy", started)
		}
	}

	close(release)
	waitForSnapshot(t, ctx, running, "the first transfer to finish",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateSucceeded })
	for _, id := range duplicates {
		waitForSnapshot(t, ctx, id, "the duplicate to run once the URL is free",
			func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	}
}
