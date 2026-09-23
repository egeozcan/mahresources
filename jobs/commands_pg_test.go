//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"mahresources/models"
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
