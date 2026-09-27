package jobs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mahresources/models"
)

// A transition that stops the work can say, in the same write, what the row now
// shows: once the Job leaves running its token is cleared, so a progress write
// after the transition could never land.
func TestJobTransitionReplacesProgressInTheSameWrite(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seedJob(t, deps, StateRunning, clock, 1)
	completed, total := int64(40), int64(100)
	snap, err := svc.Transition(deps, Transition{
		JobID: job.ID, ExpectedVersion: 1, To: StatePaused,
		Progress: &Progress{Completed: &completed, Total: &total, Unit: "bytes", Message: "Paused for a reason"},
	})
	if err != nil {
		t.Fatalf("transition with progress: %v", err)
	}
	if snap.State != StatePaused || snap.Progress.Message != "Paused for a reason" ||
		snap.Progress.Completed == nil || *snap.Progress.Completed != 40 {
		t.Fatalf("the paused snapshot is %s with progress %+v", snap.State, snap.Progress)
	}
	if stored := jobRow(t, deps, job.ID); stored.ProgressMessage != "Paused for a reason" || stored.State != string(StatePaused) {
		t.Fatalf("the stored row is %s with message %q", stored.State, stored.ProgressMessage)
	}

	other := seedJob(t, deps, StateRunning, clock, 1)
	negative := int64(-1)
	if _, err := svc.Transition(deps, Transition{
		JobID: other.ID, ExpectedVersion: 1, To: StatePaused, Progress: &Progress{Completed: &negative},
	}); !errors.Is(err, ErrInvalidProgress) {
		t.Fatalf("a transition with invalid progress = %v, want ErrInvalidProgress", err)
	}
	if stored := jobRow(t, deps, other.ID); stored.State != string(StateRunning) || stored.Version != 1 {
		t.Fatalf("the refused transition wrote %s at version %d", stored.State, stored.Version)
	}
}

// An earlier release recorded some Kinds' holds as blocked. PauseBlockedHold is the
// one path from blocked to paused, and it moves only a Job no execution owns and no
// cancellation has won.
func TestPauseBlockedHoldMovesOnlyAnUnownedBlock(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	detail := json.RawMessage(`{"reason":"paused"}`)

	held := seedJob(t, deps, StateBlocked, clock.Add(-time.Minute), 3)
	snap, err := svc.PauseBlockedHold(deps, held.ID, 3, detail, &Progress{Message: "Paused"})
	if err != nil {
		t.Fatalf("reclassify a hold: %v", err)
	}
	if snap.State != StatePaused || snap.Version != 4 || snap.Progress.Message != "Paused" {
		t.Fatalf("the hold is %s at version %d saying %q", snap.State, snap.Version, snap.Progress.Message)
	}
	stored := jobRow(t, deps, held.ID)
	if stored.BlockedDuration != time.Minute {
		t.Fatalf("the time spent blocked was banked as %s", stored.BlockedDuration)
	}
	events := jobEvents(t, deps, held.ID)
	if last := events[len(events)-1]; last.Type != EventPaused || string(last.Detail) != string(detail) {
		t.Fatalf("the reclassification recorded %s %s", last.Type, last.Detail)
	}

	if _, err := svc.PauseBlockedHold(deps, held.ID, 4, detail, nil); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("reclassifying a paused Job = %v, want ErrIllegalTransition", err)
	}

	stale := seedJob(t, deps, StateBlocked, clock, 2)
	if _, err := svc.PauseBlockedHold(deps, stale.ID, 1, detail, nil); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("a stale version = %v, want ErrVersionConflict", err)
	}

	quarantined := seedJob(t, deps, StateBlocked, clock, 1)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", quarantined.ID).
		Update("execution_token", "claim-q").Error; err != nil {
		t.Fatalf("seed a quarantine: %v", err)
	}
	if _, err := svc.PauseBlockedHold(deps, quarantined.ID, 1, detail, nil); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("reclassifying a quarantine = %v, want ErrIllegalTransition", err)
	}

	cancelling := seedJob(t, deps, StateBlocked, clock, 1)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", cancelling.ID).
		Update("control_intent", ControlIntentCancel).Error; err != nil {
		t.Fatalf("seed a cancellation: %v", err)
	}
	if _, err := svc.PauseBlockedHold(deps, cancelling.ID, 1, detail, nil); !errors.Is(err, ErrControlIntentWon) {
		t.Fatalf("reclassifying a cancelled hold = %v, want ErrControlIntentWon", err)
	}
	for _, id := range []string{quarantined.ID, cancelling.ID} {
		if stored := jobRow(t, deps, id); stored.State != string(StateBlocked) || stored.Version != 1 {
			t.Fatalf("a refused reclassification wrote %s at version %d", stored.State, stored.Version)
		}
	}
}
