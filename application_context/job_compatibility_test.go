package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

// This file drives the compatibility-handle seam: the durable mapping from a
// legacy identifier to the canonical Job it currently names. The properties worth
// pinning are that a handle survives the process that created it, that it follows
// a linear Retry lineage without ever rewriting an ancestor's identity, and that
// resolving one rechecks visibility exactly as reading the Job would.

// compatTestKind is the Kind the compatibility tests accept their Jobs under. It
// is not a real adapter's Kind: these tests are about identity and movement, not
// about an executor.
const compatTestKind = "compat-test"

// registerCompatKind teaches the control plane one Kind whose input can be sealed,
// which is what a Retry needs in order to copy it.
func registerCompatKind(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	if !jobs.HasReplayCodec(ctx.JobService(), compatTestKind, 1) {
		if err := ctx.JobService().RegisterReplayCodec(compatTestKind, 1, jobs.ReplayCodec{
			Sanitize: func(input json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{"url":"https://example.test/a"}`), nil
			},
			Encode: func(input json.RawMessage) (json.RawMessage, error) { return input, nil },
			Decode: func(payload json.RawMessage, _ uint) (json.RawMessage, error) { return payload, nil },
			Migrate: func(payload json.RawMessage, _, _ uint) (json.RawMessage, error) {
				return payload, nil
			},
		}); err != nil {
			t.Fatalf("register replay codec: %v", err)
		}
	}
	if _, ok := ctx.JobService().AdapterFor(compatTestKind, 1); !ok {
		adapter := newRuntimeTestAdapter()
		adapter.def.Kind = compatTestKind
		adapter.def.KindVersion = 1
		// The adapter is the only thing that can say whether its work may be
		// re-run at all; the host narrows that answer by the lineage and the
		// sealed input. These tests are about the host's half.
		adapter.advertise = func(context.Context, jobs.CommandContext) ([]jobs.Command, error) {
			return []jobs.Command{{Key: jobs.CommandRetry, Label: "Retry"}}, nil
		}
		if err := ctx.JobService().RegisterAdapter(adapter); err != nil {
			t.Fatalf("register adapter: %v", err)
		}
	}
}

// TestALegacyHandleFollowsTheRetryLeafAndNeverRewritesTheAncestor is the whole
// point of the handle table: a client keeps one id across successive retries,
// every attempt keeps its own immutable identity, and the handle always names the
// execution that id means *now*.
func TestALegacyHandleFollowsTheRetryLeafAndNeverRewritesTheAncestor(t *testing.T) {
	ctx := newJobContext(t)
	registerCompatKind(t, ctx)

	first := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: compatTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"url":"https://example.test/a"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: "abc123"}},
	})

	resolved, err := ctx.ResolveJobHandle(DownloadHandleNamespace, "abc123")
	if err != nil {
		t.Fatalf("resolve a freshly accepted handle: %v", err)
	}
	if resolved.ID != first.ID {
		t.Fatalf("handle resolved to %s, want the accepted job %s", resolved.ID, first.ID)
	}

	firstFailed := finishJobFor(t, ctx, first, jobs.StateFailed)
	firstFailure := firstFailed.Failure
	if firstFailure == nil {
		t.Fatalf("the failed attempt recorded no failure")
	}

	retried, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID:           firstFailed.ID,
		Key:             jobs.CommandRetry,
		IdempotencyKey:  "compat-retry-1",
		ExpectedVersion: firstFailed.Version,
	})
	if err != nil {
		t.Fatalf("retry the failed job: %v", err)
	}
	if retried.SuccessorID == "" {
		t.Fatalf("the retry created no successor: %+v", retried)
	}
	if retried.SuccessorID == first.ID {
		t.Fatalf("the retry reused the ancestor's identity %s", first.ID)
	}

	resolved, err = ctx.ResolveJobHandle(DownloadHandleNamespace, "abc123")
	if err != nil {
		t.Fatalf("resolve the handle after a retry: %v", err)
	}
	if resolved.ID != retried.SuccessorID {
		t.Fatalf("handle resolved to %s after a retry, want the successor %s", resolved.ID, retried.SuccessorID)
	}
	if resolved.Kind != compatTestKind || resolved.State != jobs.StateQueued {
		t.Fatalf("the successor is %s/%s, want a queued %s", resolved.Kind, resolved.State, compatTestKind)
	}

	// The ancestor keeps its own identity and its own outcome. A handle moving to
	// the successor is not an edit of what the ancestor reached.
	ancestor, err := ctx.GetJob(first.ID)
	if err != nil {
		t.Fatalf("read the ancestor: %v", err)
	}
	if ancestor.State != jobs.StateFailed {
		t.Fatalf("the ancestor is now %s, want its own failed outcome", ancestor.State)
	}
	if ancestor.Failure == nil || ancestor.Failure.Code != firstFailure.Code {
		t.Fatalf("the ancestor's failure changed from %v to %v", firstFailure, ancestor.Failure)
	}

	// A successor retried in turn keeps the same handle moving rather than
	// creating a second kind of identity for it.
	secondFailed := finishJobFor(t, ctx, resolved, jobs.StateCancelled)
	retried2, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID:           secondFailed.ID,
		Key:             jobs.CommandRetry,
		IdempotencyKey:  "compat-retry-2",
		ExpectedVersion: secondFailed.Version,
	})
	if err != nil {
		t.Fatalf("retry the successor: %v", err)
	}
	resolved, err = ctx.ResolveJobHandle(DownloadHandleNamespace, "abc123")
	if err != nil {
		t.Fatalf("resolve the handle after a second retry: %v", err)
	}
	if resolved.ID != retried2.SuccessorID {
		t.Fatalf("handle resolved to %s after the second retry, want %s", resolved.ID, retried2.SuccessorID)
	}
}

// TestHandleResolutionRechecksVisibilityAtTheFacade proves a handle grants no
// rights: whoever resolves it authorizes the Job it names exactly as they would
// any other read, and a hidden Job is answered like a missing handle.
func TestHandleResolutionRechecksVisibilityAtTheFacade(t *testing.T) {
	ctx := newJobContext(t)

	owned := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "somebody else's download",
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: "hidden-1"}},
	})

	other := ctx.WithPrincipal(&auth.Principal{UserID: 8, Username: "other", Role: models.RoleUser})
	if _, err := other.ResolveJobHandle(DownloadHandleNamespace, "hidden-1"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("a handle resolved a Job its asker may not see: %v", err)
	}
	owner := ctx.WithPrincipal(&auth.Principal{UserID: 7, Username: "owner", Role: models.RoleUser})
	resolved, err := owner.ResolveJobHandle(DownloadHandleNamespace, "hidden-1")
	if err != nil {
		t.Fatalf("the owner could not resolve their own handle: %v", err)
	}
	if resolved.ID != owned.ID {
		t.Fatalf("the owner's handle resolved to %s, want %s", resolved.ID, owned.ID)
	}
	if _, err := ctx.ResolveJobHandle(DownloadHandleNamespace, "no-such-handle"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("an unknown handle resolved: %v", err)
	}
	if _, err := ctx.ResolveJobHandle("no-such-namespace", "hidden-1"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("a handle resolved out of its namespace: %v", err)
	}
}

// TestARefusedRetryLeavesTheHandleWhereItWas pins the atomicity the movement
// needs: the successor's acceptance, its link and the handle movement either all
// commit or none of them do, so a retry the lineage policy refuses cannot move a
// handle onto an execution that was never created.
func TestARefusedRetryLeavesTheHandleWhereItWas(t *testing.T) {
	ctx := newJobContext(t)
	registerCompatKind(t, ctx)

	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: compatTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"url":"https://example.test/a"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: "once-only"}},
	})
	failed := finishJobFor(t, ctx, job, jobs.StateFailed)

	successor, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: failed.ID, Key: jobs.CommandRetry, IdempotencyKey: "first", ExpectedVersion: failed.Version,
	})
	if err != nil {
		t.Fatalf("first retry: %v", err)
	}

	// A second retry of the same ancestor is refused: the chain is linear and its
	// leaf is the successor.
	if _, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: failed.ID, Key: jobs.CommandRetry, IdempotencyKey: "second", ExpectedVersion: failed.Version,
	}); err == nil {
		t.Fatalf("a second retry of one ancestor was accepted")
	}
	resolved, err := ctx.ResolveJobHandle(DownloadHandleNamespace, "once-only")
	if err != nil {
		t.Fatalf("resolve after the refused retry: %v", err)
	}
	if resolved.ID != successor.SuccessorID {
		t.Fatalf("the refused retry moved the handle to %s, want it left on %s", resolved.ID, successor.SuccessorID)
	}
}
