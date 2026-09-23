package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/afero"

	"mahresources/jobs"
)

// This file is the command-policy seam's own contract: what a Kind's advertisement
// is allowed to read, and on which handle it reads it.
//
// A command runs in one transaction — the request is claimed by its idempotency
// tuple, every durable precondition is rechecked against the row the transaction
// holds, and the effect commits with it — and the *recheck* asks the Kind again
// whether the Job still offers the command. A Kind whose answer needs a read
// therefore reads it inside that transaction, which means on that transaction's
// handle. A Kind that reached for the process's own handle instead took a second
// database connection while the first was held: a guaranteed deadlock against a pool
// of one, and avoidable contention against every other pool.

// oneConnectionJobContext is the harness with the deployment's pool pinned to a
// single connection, which is what makes a second concurrent handle a hang rather
// than a slow query. No dispatch loop runs: the command path under test is driven
// directly.
func oneConnectionJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newJobHarnessContext(t, false)
	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return ctx
}

// retryableApplyForTest accepts one failed import-apply Job whose replay evidence is
// on disk.
//
// It is the cheapest Job whose *advertisement* reads the database: the Retry an
// import apply offers is decided from its own sealed input — which is opened through
// the control plane — and from the plan and archive the input names. A Kind with no
// such answer would exercise nothing here.
func retryableApplyForTest(t *testing.T, ctx *MahresourcesContext, handle string) jobs.Snapshot {
	t.Helper()
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("mkdir _imports: %v", err)
	}
	for _, path := range []string{importPlanPathFor(handle), importArchivePathFor(handle)} {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: handle,
		Plan:        importPlanPathFor(handle),
		Decisions: ImportDecisions{
			MappingActions:  map[string]MappingAction{},
			DanglingActions: map[string]DanglingAction{},
		},
	})
	if err != nil {
		t.Fatalf("encode the apply input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: "seam-" + handle}},
	})
	return finishJobFor(t, ctx, accepted, jobs.StateFailed)
}

// TestACommandRecheckReadsOnTheTransactionItHolds drives a real Kind's Retry through
// the public command surface over a one-connection pool.
//
// The transaction that claims the command holds the pool's only connection while it
// rechecks the advertisement; an adapter that opened a handle of its own there waited
// for a connection that could not be freed until it returned. The assertion is
// therefore an absence — the command completes — and it is run on a goroutine so a
// regression is a failed test rather than a hanging suite.
func TestACommandRecheckReadsOnTheTransactionItHolds(t *testing.T) {
	ctx := oneConnectionJobContext(t)
	const handle = "seam0001"
	failed := retryableApplyForTest(t, ctx, handle)

	// The advertisement is honest to begin with: the command exists, decided from the
	// same reads the recheck will repeat.
	if !offersCommand(advertisedForTest(t, ctx, failed.ID), jobs.CommandRetry) {
		t.Fatalf("the failed apply offers no Retry to run")
	}

	type outcome struct {
		result jobs.CommandResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
			JobID:           failed.ID,
			Key:             jobs.CommandRetry,
			IdempotencyKey:  "seam-retry",
			ExpectedVersion: failed.Version,
		})
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("the retry was refused: %v", got.err)
		}
		if got.result.SuccessorID == "" {
			t.Fatalf("the retry created no successor: %+v", got.result)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the command never returned: its recheck opened a second connection while the transaction held the only one")
	}
}
