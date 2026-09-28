//go:build postgres

package jobs

import (
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

// The blocked reason read on Postgres: the correlated latest-event subquery
// answers one row per Job and the reason the latest blocked event recorded.
func TestBlockedReasonsReadTheLatestBlockedEventPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	now := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	job := seedJob(t, deps, StateBlocked, now, 3)
	for sequence, event := range []struct{ kind, detail string }{
		{EventBlocked, `{"reason":"plugin-unavailable"}`},
		{EventQueued, `{}`},
		{EventBlocked, `{"reason":"role-refused"}`},
	} {
		if err := deps.DB.Create(&models.JobEvent{
			ID: types.NewUUIDv7(), JobID: job.ID, Sequence: uint64(sequence + 1), JobVersion: uint64(sequence + 1),
			Type: event.kind, Detail: types.JSON(event.detail), CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	reasons, err := svc.BlockedReasons(deps, []Snapshot{{ID: job.ID, State: StateBlocked}})
	if err != nil {
		t.Fatalf("BlockedReasons: %v", err)
	}
	if reasons[job.ID] != "role-refused" {
		t.Fatalf("reason = %q; want the latest", reasons[job.ID])
	}
}
