//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// These tests run the dialect-sensitive halves of the command surface against
// PostgreSQL: the idempotency tuple's unique index under two connections, the
// retry chain's row lock, and the race between a cancellation that won and a
// success decided before it. SQLite serializes writers, so a race there cannot
// show whether the database is what admits one winner; PostgreSQL can.

// TestCommandIdempotencyAdmitsOneExecutionAcrossConnectionsPG races two identical
// command requests on two connections. The unique tuple is what admits exactly
// one execution: the loser's insert is a no-op, so it reads the claim the winner
// committed and is answered with it — or told that it is still in flight —
// rather than reaching the executor a second time.
func TestCommandIdempotencyAdmitsOneExecutionAcrossConnectionsPG(t *testing.T) {
	h := newCommandHarnessOn(t, newPGDeps(t))
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	for iteration := 0; iteration < 5; iteration++ {
		running := h.acceptReplayable(&owner)
		execution := h.claim(running.ID)

		var mu sync.Mutex
		calls := 0
		h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return CommandOutcome{Status: CommandStatusSucceeded, Message: "stopping"}, nil
		}

		request := h.request(running.ID, CommandCancel, "idem-pg", viewer)
		errs := concurrent(
			func() error {
				_, err := h.svc.ExecuteCommand(context.Background(), h.deps, request)
				return err
			},
			func() error {
				_, err := h.svc.ExecuteCommand(context.Background(), h.deps, request)
				return err
			},
		)

		if calls != 1 {
			t.Fatalf("iteration %d: the executor was invoked %d times for one idempotency tuple", iteration, calls)
		}
		rows := commandRequestRows(t, h.deps, running.ID)
		if len(rows) != 1 {
			t.Fatalf("iteration %d: %d command requests were recorded, want one", iteration, len(rows))
		}
		if rows[0].Status != models.JobCommandStatusSucceeded && rows[0].Status != models.JobCommandStatusRunning {
			t.Fatalf("iteration %d: the recorded request is %s", iteration, rows[0].Status)
		}
		for _, err := range errs {
			switch {
			case err == nil, errors.Is(err, ErrCommandInFlight):
			default:
				t.Fatalf("iteration %d: a racing request failed with %v", iteration, err)
			}
		}

		// The Job is ended as cancelled under its own execution, so the iteration's
		// claim gives its capacity back: this Kind admits four executions, and five
		// iterations that each left one running would be refused by the budget rather
		// than by the tuple under test.
		h.endExecution(running.ID, execution, StateCancelled)
	}
}

// TestBulkCommandReplaysRunningAndCompletedOutcomeBeforeAdvertisementPG runs a
// duplicate bulk command while the first request is outside its transaction in
// the executor. The intent makes Cancel disappear from the current
// advertisement, so the duplicate must resolve the keyed in-flight result
// before checking that advertisement. After the executor returns, another
// repeat must resolve the completed result the same way.
func TestBulkCommandReplaysRunningAndCompletedOutcomeBeforeAdvertisementPG(t *testing.T) {
	h := newCommandHarnessOn(t, newPGDeps(t))
	owner := uint(7)
	viewer := Access{UserID: owner}
	running := h.acceptReplayable(&owner)
	h.claim(running.ID)

	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		if commandContext.Snapshot.State == StateRunning && commandContext.Snapshot.ControlIntent == "" {
			return []Command{{Key: CommandCancel, Label: "Cancel", Bulk: true}}, nil
		}
		return nil, nil
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var callsMu sync.Mutex
	calls := 0
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		close(entered)
		<-release
		return CommandOutcome{Status: CommandStatusSucceeded, Message: "stop requested"}, nil
	}

	bulk := func() []CommandResult {
		return h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
			JobIDs: []string{running.ID}, Key: CommandCancel,
			IdempotencyKey: "idem-pg-bulk-cancel", Actor: viewer, Origin: "api",
		})
	}
	firstDone := make(chan []CommandResult, 1)
	go func() { firstDone <- bulk() }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first bulk cancellation did not reach the executor")
	}

	// PostgreSQL can serve this through a separate transaction while the first
	// goroutine is held in the executor. The recorded request is still running,
	// and the current adapter advertisement no longer contains Cancel.
	second := bulk()
	if len(second) != 1 || second[0].Status != CommandStatusFailed || second[0].Code != CommandCodeInFlight {
		t.Fatalf("the duplicate bulk cancellation answered %+v, want the recorded in-flight outcome", second)
	}

	releaseOnce.Do(func() { close(release) })
	var first []CommandResult
	select {
	case first = <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the first bulk cancellation did not finish after releasing its executor")
	}
	if len(first) != 1 || first[0].Status != CommandStatusSucceeded || first[0].Code != CommandCodeRequested {
		t.Fatalf("the first bulk cancellation answered %+v, want a successful requested result", first)
	}

	third := bulk()
	if len(third) != 1 || third[0].Status != CommandStatusSucceeded || third[0].Code != CommandCodeRequested {
		t.Fatalf("the completed bulk cancellation replay answered %+v, want the stored success", third)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if calls != 1 {
		t.Fatalf("the executor ran %d times for one bulk idempotency tuple, want once", calls)
	}
	if rows := commandRequestRows(t, h.deps, running.ID); len(rows) != 1 || rows[0].Status != models.JobCommandStatusSucceeded {
		t.Fatalf("the Job has command request rows %+v, want one completed request", rows)
	}
}

// TestSingleCommandOutcomeDoesNotBecomeBulkEligibleOnReplayPG pins the two
// cross-surface directions on PostgreSQL: a stored single-only command cannot
// be replayed as a bulk success, while a valid bulk command can be replayed
// through the single route after its response is lost.
func TestSingleCommandOutcomeDoesNotBecomeBulkEligibleOnReplayPG(t *testing.T) {
	h := newCommandHarnessOn(t, newPGDeps(t))
	owner := uint(7)
	viewer := Access{UserID: owner}
	singleOnly := h.acceptReplayable(&owner)
	bulkCapable := h.acceptReplayable(&owner)

	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		return []Command{{Key: "inspect", Label: "Inspect", Bulk: commandContext.Snapshot.ID == bulkCapable.ID}}, nil
	}

	single, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(singleOnly.ID, "inspect", "idem-pg-single-only", viewer))
	if err != nil {
		t.Fatalf("the single-only command failed: %v", err)
	}
	if single.Status != CommandStatusSucceeded || single.Code != CommandCodeApplied {
		t.Fatalf("the single-only command answered %s/%s, want succeeded/applied", single.Status, single.Code)
	}

	bulk := h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{singleOnly.ID}, Key: "inspect", IdempotencyKey: "idem-pg-single-only",
		Actor: viewer, Origin: "api",
	})
	if len(bulk) != 1 || bulk[0].Status != CommandStatusFailed || bulk[0].Code != CommandCodeNotAdvertised {
		t.Fatalf("replaying the single-only command as bulk answered %+v, want a bulk eligibility refusal", bulk)
	}
	if rows := commandRequestRows(t, h.deps, singleOnly.ID); len(rows) != 1 || rows[0].BulkEligible {
		t.Fatalf("single-only command has recorded requests %+v, want one non-bulk-eligible request", rows)
	}

	bulk = h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{bulkCapable.ID}, Key: "inspect", IdempotencyKey: "idem-pg-bulk-origin",
		Actor: viewer, Origin: "api",
	})
	if len(bulk) != 1 || bulk[0].Status != CommandStatusSucceeded || bulk[0].Code != CommandCodeApplied {
		t.Fatalf("the bulk-capable command answered %+v, want succeeded/applied", bulk)
	}
	if rows := commandRequestRows(t, h.deps, bulkCapable.ID); len(rows) != 1 || !rows[0].BulkEligible {
		t.Fatalf("bulk-capable command has recorded requests %+v, want one bulk-eligible request", rows)
	}
	single, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(bulkCapable.ID, "inspect", "idem-pg-bulk-origin", viewer))
	if err != nil || single.Status != CommandStatusSucceeded || single.Code != CommandCodeApplied {
		t.Fatalf("bulk-to-single replay answered %+v with error %v, want succeeded/applied", single, err)
	}
	if h.adapter.commandCount() != 2 {
		t.Fatalf("the adapter ran %d times, want one execution for each Job", h.adapter.commandCount())
	}
}

// TestRetryChainAdmitsOneSuccessorAcrossConnectionsPG races two Retries of one
// Job, each with its own idempotency key, on two connections. The ancestor's row
// is taken FOR UPDATE before the successor predicate is read, so the loser reads
// the link the winner committed and is refused: one Job, one successor.
func TestRetryChainAdmitsOneSuccessorAcrossConnectionsPG(t *testing.T) {
	h := newCommandHarnessOn(t, newPGDeps(t))
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}
	admin := Access{UserID: 3, Administrator: true}

	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)

	requests := []CommandRequest{
		h.request(ancestor.ID, CommandRetry, "idem-pg-retry-a", admin),
		h.request(ancestor.ID, CommandRetry, "idem-pg-retry-b", viewer),
	}
	errs := concurrent(
		func() error {
			_, err := h.svc.ExecuteCommand(context.Background(), h.deps, requests[0])
			return err
		},
		func() error {
			_, err := h.svc.ExecuteCommand(context.Background(), h.deps, requests[1])
			return err
		},
	)

	succeeded, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrCommandChainConflict), errors.Is(err, ErrCommandNotAdvertised):
			refused++
		default:
			t.Fatalf("a racing retry failed with %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("%d retries succeeded and %d were refused, want one of each (%s)",
			succeeded, refused, describeErrors(errs))
	}
	successors := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false)
	if len(successors) != 1 {
		t.Fatalf("one job has %d retry successors, want exactly one", len(successors))
	}
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), successors[0]) {
			t.Fatalf("the losing retry disclosed a successor UUID: %v", err)
		}
	}
	if errs[0] == nil {
		if _, err := h.svc.Get(h.deps, viewer, successors[0]); !errors.Is(err, ErrNotFound) {
			t.Fatalf("the administrator's successor is visible to its losing owner: %v", err)
		}
		if errs[1] != nil && errors.Is(errs[1], ErrCommandChainConflict) && strings.Contains(errs[1].Error(), successors[0]) {
			t.Fatalf("the losing owner learned the hidden successor UUID: %v", errs[1])
		}
	}
}

func TestRetryWaitsForConcurrentForgetOfReplayEnvelopePG(t *testing.T) {
	assertSuccessorWaitsForReplayPurgePG(t, CommandRetry, models.JobReplayPurgeForgotten)
}

func TestRetryWaitsForConcurrentExpiryOfReplayEnvelopePG(t *testing.T) {
	assertSuccessorWaitsForReplayPurgePG(t, CommandRetry, models.JobReplayPurgeExpired)
}

func TestRepeatWaitsForConcurrentForgetOfReplayEnvelopePG(t *testing.T) {
	assertSuccessorWaitsForReplayPurgePG(t, CommandRepeat, models.JobReplayPurgeForgotten)
}

// assertSuccessorWaitsForReplayPurgePG makes Forget or expiry purge hold the
// envelope row after writing its tombstone, then starts Retry or Repeat while
// that write is uncommitted. The input read must wait on the envelope row and
// then refuse the tombstone instead of publishing a successor from bytes the
// purge already removed.
func assertSuccessorWaitsForReplayPurgePG(t *testing.T, commandKey, purgeReason string) {
	t.Helper()
	h := newCommandHarnessOn(t, newPGDeps(t))
	h.advertiseStateful()
	if purgeReason == models.JobReplayPurgeExpired {
		h.deps.Replay.Retention = time.Hour
	}
	owner := uint(7)
	viewer := Access{UserID: owner}
	ancestor := h.acceptReplayable(&owner)
	var terminal Snapshot
	if commandKey == CommandRetry {
		terminal = h.fail(ancestor.ID)
	} else {
		terminal = h.succeed(ancestor.ID)
	}
	if purgeReason == models.JobReplayPurgeExpired {
		h.clock = h.clock.Add(2 * time.Hour)
	}
	request := h.request(terminal.ID, commandKey, "successor-after-purge", viewer)
	retryDeps := h.deps
	if purgeReason == models.JobReplayPurgeExpired {
		// Expiry is already enforced by the stored deadline before the sweep runs.
		// Model a retrying PostgreSQL process whose clock has not advanced as far as
		// the process that selected and purges the expired envelope.
		retryNow := h.clock.Add(-2 * time.Hour)
		retryDeps.Now = func() time.Time { return retryNow }
	}

	purgeUpdated := make(chan struct{})
	releasePurge := make(chan struct{})
	openReadStarted := make(chan struct{})
	openReadFinished := make(chan struct{})
	releaseOpenRead := make(chan struct{})
	var releasePurgeOnce, releaseOpenReadOnce sync.Once
	letPurgeFinish := func() { releasePurgeOnce.Do(func() { close(releasePurge) }) }
	letOpenReadFinish := func() { releaseOpenReadOnce.Do(func() { close(releaseOpenRead) }) }
	t.Cleanup(func() {
		letPurgeFinish()
		letOpenReadFinish()
		_ = h.deps.DB.Callback().Update().Remove("test:retry-purge-row-lock")
		_ = h.deps.DB.Callback().Query().Remove("test:retry-purge-open-before")
		_ = h.deps.DB.Callback().Query().Remove("test:retry-purge-open-after")
	})

	var pausePurge atomic.Bool
	pausePurge.Store(true)
	if err := h.deps.DB.Callback().Update().After("gorm:update").Register("test:retry-purge-row-lock", func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Table != "job_replay_envelopes" ||
			!strings.Contains(strings.ToLower(db.Statement.SQL.String()), "purged_at") ||
			!pausePurge.CompareAndSwap(true, false) {
			return
		}
		close(purgeUpdated)
		<-releasePurge
	}); err != nil {
		t.Fatalf("register replay purge barrier: %v", err)
	}

	purgeDone := make(chan error, 1)
	go func() {
		if purgeReason == models.JobReplayPurgeForgotten {
			_, err := h.svc.ForgetReplay(h.deps, viewer, terminal.ID)
			purgeDone <- err
			return
		}
		count, err := h.svc.PurgeExpiredReplay(h.deps, 10)
		if err == nil && count != 1 {
			err = fmt.Errorf("purged %d envelopes, want one", count)
		}
		purgeDone <- err
	}()
	select {
	case <-purgeUpdated:
	case err := <-purgeDone:
		t.Fatalf("replay purge finished before its envelope update barrier: %v", err)
	case <-time.After(5 * time.Second):
		select {
		case err := <-purgeDone:
			t.Fatalf("replay purge did not reach its envelope update: %v", err)
		default:
			t.Fatal("replay purge did not reach its envelope update")
		}
	}

	// Register after the purge's update is paused. Only the FOR UPDATE read used
	// to copy input is gated; Retry's earlier availability reads remain free to
	// observe the old committed version.
	if err := h.deps.DB.Callback().Query().Before("gorm:query").Register("test:retry-purge-open-before", func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "job_replay_envelopes" {
			if _, locking := db.Statement.Clauses["FOR"]; !locking {
				return
			}
			close(openReadStarted)
		}
	}); err != nil {
		t.Fatalf("register Retry input read start barrier: %v", err)
	}
	if err := h.deps.DB.Callback().Query().After("gorm:query").Register("test:retry-purge-open-after", func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "job_replay_envelopes" &&
			strings.Contains(strings.ToUpper(db.Statement.SQL.String()), "FOR UPDATE") {
			close(openReadFinished)
			<-releaseOpenRead
		}
	}); err != nil {
		t.Fatalf("register Retry input read completion barrier: %v", err)
	}

	retryDone := make(chan error, 1)
	go func() {
		_, err := h.svc.ExecuteCommand(context.Background(), retryDeps, request)
		retryDone <- err
	}()
	select {
	case <-openReadStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("successor command did not reach its locking input read")
	}

	// With the row lock, the read waits for the purge transaction. Without it,
	// PostgreSQL returns the old envelope version here and Retry can commit.
	openedBeforePurgeCommit := false
	select {
	case <-openReadFinished:
		openedBeforePurgeCommit = true
	case <-time.After(200 * time.Millisecond):
	}
	if openedBeforePurgeCommit {
		letOpenReadFinish()
		select {
		case <-retryDone:
		case <-time.After(5 * time.Second):
			t.Fatal("successor command did not finish after reading the replay envelope")
		}
		letPurgeFinish()
		if err := <-purgeDone; err != nil {
			t.Fatalf("finish replay purge: %v", err)
		}
		t.Fatal("successor command read the envelope before Forget/expiry committed")
	}

	letPurgeFinish()
	if err := <-purgeDone; err != nil {
		t.Fatalf("finish replay purge: %v", err)
	}
	select {
	case <-openReadFinished:
	case <-time.After(5 * time.Second):
		t.Fatal("successor input read did not continue after the purge committed")
	}
	letOpenReadFinish()
	select {
	case err := <-retryDone:
		if purgeReason == models.JobReplayPurgeForgotten && !errors.Is(err, ErrReplayForgotten) {
			t.Fatalf("successor after Forget = %v, want ErrReplayForgotten", err)
		}
		if purgeReason == models.JobReplayPurgeExpired && !errors.Is(err, ErrReplayExpired) {
			t.Fatalf("successor after expiry = %v, want ErrReplayExpired", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("successor command did not finish after reading the purged envelope")
	}
	linkType := LinkRetryOf
	if commandKey == CommandRepeat {
		linkType = LinkRepeatOf
	}
	if successors := jobLinks(t, h.deps, terminal.ID, linkType, false); len(successors) != 0 {
		t.Fatalf("a purge racing %s left %d successors, want none", commandKey, len(successors))
	}
}

// TestACancellationThatWonIsNeverOverwrittenByARacingSuccessPG is §4's "once
// cancellation intent wins, later success cannot overwrite it" as a real race:
// a cancellation request and a success publication run against one Job at the
// same instant, and whatever the order, the Job ends in exactly one of the two
// outcomes the design allows. A Job that succeeded with a cancellation recorded
// against it is the state that must never exist.
//
// The guard is the version: recording the intent moves it, so a success decided
// from the version before it commits matches no row and is refused, whichever
// engine is underneath.
func TestACancellationThatWonIsNeverOverwrittenByARacingSuccessPG(t *testing.T) {
	h := newCommandHarnessOn(t, newPGDeps(t))
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	for iteration := 0; iteration < 5; iteration++ {
		running := h.acceptReplayable(&owner)
		execution := h.claim(running.ID)
		h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
			return CommandOutcome{Status: CommandStatusSucceeded}, nil
		}

		// One side asks for the cancellation; the other publishes the success the
		// execution had reached anyway.
		errs := concurrent(
			func() error {
				_, err := h.svc.ExecuteCommand(context.Background(), h.deps,
					h.request(running.ID, CommandCancel, "idem-pg-cancel", viewer))
				return err
			},
			func() error {
				_, err := h.svc.Finish(h.deps, FinishRequest{
					ExecutionRef:    ExecutionRef{JobID: running.ID, ExecutionToken: execution.ExecutionToken},
					ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
					Outcome:         StateSucceeded,
				})
				return err
			},
		)
		for _, err := range errs {
			switch {
			case err == nil,
				errors.Is(err, ErrVersionConflict),
				errors.Is(err, ErrControlIntentWon),
				errors.Is(err, ErrCommandNotAdvertised),
				errors.Is(err, ErrCommandInFlight):
			default:
				t.Fatalf("iteration %d: a racing outcome failed with %v", iteration, err)
			}
		}

		row := jobRow(t, h.deps, running.ID)
		switch State(row.State) {
		case StateSucceeded:
			if row.ControlIntent != "" {
				t.Fatalf("iteration %d: the job succeeded carrying control intent %q", iteration, row.ControlIntent)
			}
		case StateRunning:
			if row.ControlIntent != ControlIntentCancel {
				t.Fatalf("iteration %d: the job is running with control intent %q, want a won cancellation",
					iteration, row.ControlIntent)
			}
		default:
			t.Fatalf("iteration %d: the racing commands left the job %s", iteration, row.State)
		}
		// Whether the job is finished or still running with a won cancellation, its
		// capacity goes back before the next iteration claims again.
		if State(row.State) == StateRunning {
			h.endExecution(running.ID, execution, StateCancelled)
		}
	}
}
