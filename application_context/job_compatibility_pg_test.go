//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mahresources/jobs"
)

func TestCanonicalRetryAndStaleLegacyControlConflictOnPostgres(t *testing.T) {
	first, _, _ := newPostgresOwnershipFixture(t, 1)
	sqlDB, err := first.db.DB()
	if err != nil {
		t.Fatalf("underlying PostgreSQL database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	registerCompatKind(t, first)

	access := jobs.Access{UserID: 7, Administrator: true}
	accepted := acceptJobFor(t, first, jobs.Acceptance{
		Kind: compatTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7),
		Replay:      jobs.ReplayInput{Input: json.RawMessage(`{"url":"https://example.test/a"}`)},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: "pg-race-handle"}},
	})
	execution, claimed, err := first.JobService().Claim(context.Background(), first.jobDeps(), jobs.ClaimRequest{
		Kind: compatTestKind, KindVersion: 1, Claimant: "pg-legacy-control-race",
	})
	if err != nil || !claimed {
		t.Fatalf("claim Job before legacy projection: claimed=%t err=%v", claimed, err)
	}
	legacyProjection, err := first.GetJob(accepted.ID)
	if err != nil || legacyProjection.State != jobs.StateRunning {
		t.Fatalf("read running legacy projection: state=%s err=%v", legacyProjection.State, err)
	}
	failed, err := first.JobService().Finish(first.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: legacyProjection.Version,
		Outcome:         jobs.StateFailed,
		Failure:         &jobs.Failure{Code: "boom", Class: jobs.FailureClassInternal},
	})
	if err != nil {
		t.Fatalf("finish source Job after projection: %v", err)
	}

	canonicalRetry, err := first.JobService().ExecuteCommand(context.Background(), first.jobDeps(), jobs.CommandRequest{
		JobID: failed.ID, Key: jobs.CommandRetry, IdempotencyKey: "pg-canonical-retry",
		ExpectedVersion: failed.Version, Actor: access,
	})
	if err != nil {
		t.Fatalf("canonical Retry: %v", err)
	}

	// This is the cancellation a legacy handler prepared from its earlier running
	// projection. The handle now names the queued successor; it must not cancel it.
	_, err = first.JobService().ExecuteCommand(context.Background(), first.jobDeps(), jobs.CommandRequest{
		JobID: legacyProjection.ID, Key: jobs.CommandCancel, IdempotencyKey: "pg-stale-legacy-control",
		ExpectedVersion: legacyProjection.Version, Actor: access,
		LegacyRef: &jobs.LegacyRef{Namespace: DownloadHandleNamespace, Handle: "pg-race-handle"},
	})
	if !errors.Is(err, jobs.ErrVersionConflict) {
		t.Fatalf("stale legacy control error = %v, want ErrVersionConflict", err)
	}

	successor, err := first.GetJob(canonicalRetry.SuccessorID)
	if err != nil || successor.State != jobs.StateQueued || successor.ControlIntent != "" {
		t.Fatalf("stale cancel affected successor: state=%s intent=%s err=%v", successor.State, successor.ControlIntent, err)
	}
	resolved, err := first.JobService().ResolveLegacyHandle(first.jobDeps(), DownloadHandleNamespace, "pg-race-handle")
	if err != nil || resolved != canonicalRetry.SuccessorID {
		t.Fatalf("handle after stale control = %q, %v; want successor %q", resolved, err, canonicalRetry.SuccessorID)
	}
}
