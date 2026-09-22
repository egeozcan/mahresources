//go:build postgres && json1 && fts5

package jobs

import (
	"testing"
	"time"

	"mahresources/models"
)

// TestReplayEnvelopeLifecycleOnPostgresPG runs the envelope's whole life against
// the other supported engine, because the shapes it writes differ per dialect:
// the ciphertext and nonce are a nullable bytea, the purge sets them to NULL
// through an expression, and the sweep's batch is bounded by a subquery with
// LIMIT. SQLite tolerates several spellings of each; PostgreSQL does not.
func TestReplayEnvelopeLifecycleOnPostgresPG(t *testing.T) {
	deps := newPGDeps(t)
	if err := deps.DB.AutoMigrate(&models.JobReplayEnvelope{}); err != nil {
		t.Fatalf("migrate replay envelopes: %v", err)
	}
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	clock := time.Date(2036, 2, 3, 4, 5, 6, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material"), Retention: time.Hour}

	// A finished Job whose window has passed, and one that is still running.
	expired := acceptFixtureReplayJob(t, svc, deps)
	advanceReplayJob(t, svc, deps, advanceReplayJob(t, svc, deps, expired, StateRunning), StateFailed)
	running := acceptFixtureReplayJob(t, svc, deps)
	advanceReplayJob(t, svc, deps, running, StateRunning)

	opened, err := svc.OpenReplay(deps, Access{Administrator: true}, expired.ID)
	if err != nil {
		t.Fatalf("OpenReplay on Postgres: %v", err)
	}
	if len(opened.Input) == 0 {
		t.Fatal("the round trip produced no input")
	}

	clock = clock.Add(2 * time.Hour)
	purged, err := svc.PurgeExpiredReplay(deps, 10)
	if err != nil {
		t.Fatalf("PurgeExpiredReplay on Postgres: %v", err)
	}
	if purged != 1 {
		t.Fatalf("the sweep purged %d envelopes, want 1", purged)
	}
	envelope := replayEnvelopeRow(t, deps, expired.ID)
	if envelope.PurgedAt == nil || envelope.PurgeReason != models.JobReplayPurgeExpired {
		t.Fatalf("purged envelope = %+v, want a durable expiry marker", envelope)
	}
	if envelope.Ciphertext != nil || envelope.Nonce != nil {
		t.Fatalf("the purged envelope still holds a bytea value: %+v", envelope)
	}
	if availability := svc.snapshotFor(deps, Access{Administrator: true}, jobRow(t, deps, expired.ID)).ReplayAvailability; availability != ReplayExpired {
		t.Fatalf("availability = %q, want %q", availability, ReplayExpired)
	}
	if _, err := svc.OpenReplay(deps, Access{Administrator: true}, expired.ID); err == nil {
		t.Fatal("the purged envelope still opened")
	}

	// The running Job's input is untouched, and an explicit Forget is refused
	// while it is execution-required.
	if envelope := replayEnvelopeRow(t, deps, running.ID); envelope.PurgedAt != nil {
		t.Fatalf("a running Job's envelope was swept: %+v", envelope)
	}
	if _, err := svc.ForgetReplay(deps, Access{Administrator: true}, running.ID); err == nil {
		t.Fatal("Forget of a running Job must be refused")
	}
}
