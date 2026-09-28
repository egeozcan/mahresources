package jobs

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/types"
)

// A blocked Job's reason is on its timeline, in the blocked event that blocked
// it; a Job blocked twice is blocked for the second reason, and a Job that is no
// longer blocked has none, whatever an earlier event said.
func TestBlockedReasonsReadTheLatestBlockedEvent(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	now := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	event := func(jobID string, sequence uint64, eventType, detail string) {
		t.Helper()
		if err := deps.DB.Create(&models.JobEvent{
			ID: types.NewUUIDv7(), JobID: jobID, Sequence: sequence, JobVersion: sequence,
			Type: eventType, Detail: types.JSON(detail), CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	twice := seedJob(t, deps, StateBlocked, now, 3)
	event(twice.ID, 1, EventBlocked, `{"reason":"plugin-unavailable"}`)
	event(twice.ID, 2, EventQueued, `{}`)
	event(twice.ID, 3, EventBlocked, `{"reason":"role-refused"}`)
	unexplained := seedJob(t, deps, StateBlocked, now, 1)
	event(unexplained.ID, 1, EventBlocked, `{}`)
	resumed := seedJob(t, deps, StateRunning, now, 2)
	event(resumed.ID, 1, EventBlocked, `{"reason":"scope-refused"}`)

	snapshots := []Snapshot{
		{ID: twice.ID, State: StateBlocked},
		{ID: unexplained.ID, State: StateBlocked},
		{ID: resumed.ID, State: StateRunning},
	}
	reasons, err := svc.BlockedReasons(deps, snapshots)
	if err != nil {
		t.Fatalf("BlockedReasons: %v", err)
	}
	if reasons[twice.ID] != "role-refused" {
		t.Fatalf("a Job blocked twice reads %q; want the latest reason", reasons[twice.ID])
	}
	if reason, ok := reasons[unexplained.ID]; ok {
		t.Fatalf("a blocked event with no reason reads %q", reason)
	}
	if reason, ok := reasons[resumed.ID]; ok {
		t.Fatalf("a Job that is no longer blocked reads %q", reason)
	}
}

// A Job blocked and resumed many times keeps every blocked event on its
// timeline. The read answers one row per Job, the latest, however long that
// history grows: a listing must not scan and carry a Job's whole history to
// say one line.
func TestBlockedReasonsReadOneRowPerJob(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	now := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	job := seedJob(t, deps, StateBlocked, now, 400)
	for sequence := uint64(1); sequence <= 400; sequence++ {
		eventType, detail := EventBlocked, fmt.Sprintf(`{"reason":"reason-%d"}`, sequence)
		if sequence%2 == 0 {
			eventType, detail = EventQueued, `{}`
		}
		if sequence == 399 {
			detail = `{"reason":"role-refused"}`
		}
		if err := deps.DB.Create(&models.JobEvent{
			ID: types.NewUUIDv7(), JobID: job.ID, Sequence: sequence, JobVersion: sequence,
			Type: eventType, Detail: types.JSON(detail), CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	var rows int64
	callback := "test:count-blocked-reason-rows"
	if err := deps.DB.Callback().Raw().After("gorm:raw").Register(callback, func(tx *gorm.DB) {
		rows += tx.Statement.RowsAffected
	}); err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		rows += tx.Statement.RowsAffected
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = deps.DB.Callback().Raw().Remove(callback)
		_ = deps.DB.Callback().Query().Remove(callback)
	})

	reasons, err := svc.BlockedReasons(deps, []Snapshot{{ID: job.ID, State: StateBlocked}})
	if err != nil {
		t.Fatalf("BlockedReasons: %v", err)
	}
	if reasons[job.ID] != "role-refused" {
		t.Fatalf("reason = %q; want the latest", reasons[job.ID])
	}
	if rows != 1 {
		t.Fatalf("the read returned %d rows for one Job; want one", rows)
	}
}
