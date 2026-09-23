//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"sync"
	"testing"
	"time"

	"mahresources/jobs"
)

// The PostgreSQL arm of the command-policy seam: real Kind adapters running real
// commands, concurrently, on one database.
//
// What the engine adds here is that more than one command can be in flight at once.
// PostgreSQL runs several writers, so the recheck inside one command's transaction is
// not serialized behind another's by the writer lock the way it is on SQLite — the two
// transactions are genuinely concurrent and each holds its own connection. That is the
// shape a recheck opening a handle of its own would be visible in beyond the pool of
// one, and it is why this is driven rather than reasoned about.

// TestConcurrentCommandsRecheckOnTheirOwnTransactions runs two actual-adapter Retries
// at once, one per process context, over one PostgreSQL database.
func TestConcurrentCommandsRecheckOnTheirOwnTransactions(t *testing.T) {
	first, other, _ := newPostgresOwnershipFixture(t, 4)

	failedFirst := retryableApplyForTest(t, first, "pg0001")
	failedOther := retryableApplyForTest(t, other, "pg0002")

	type outcome struct {
		result jobs.CommandResult
		err    error
	}
	run := func(ctx *MahresourcesContext, failed jobs.Snapshot) func() outcome {
		return func() outcome {
			result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
				JobID:           failed.ID,
				Key:             jobs.CommandRetry,
				IdempotencyKey:  "pg-retry-" + failed.ID,
				ExpectedVersion: failed.Version,
			})
			return outcome{result: result, err: err}
		}
	}

	results := make([]outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = run(first, failedFirst)() }()
	go func() { defer wg.Done(); results[1] = run(other, failedOther)() }()

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent commands did not both return")
	}

	for index, got := range results {
		if got.err != nil {
			t.Fatalf("command %d was refused: %v", index, got.err)
		}
		if got.result.SuccessorID == "" {
			t.Fatalf("command %d created no successor: %+v", index, got.result)
		}
	}
	// Two successors, one per failed apply: the concurrent commands branched from two
	// different Jobs and neither interfered with the other.
	if results[0].result.SuccessorID == results[1].result.SuccessorID {
		t.Fatalf("both commands produced successor %s", results[0].result.SuccessorID)
	}
}
