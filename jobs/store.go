package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file holds the persistence mechanics: reading a Job, the one visibility
// predicate every read shares, the guarded lifecycle write, and the post-commit
// delivery-sequence publisher. Policy — which transitions are legal, what an
// acceptance must say — lives in service.go.

// loadJob reads one Job by identity, with no visibility predicate: it serves
// executors and adapters, which address a Job they already own. Every
// user-facing read goes through Get, which applies visibility.
func loadJob(db *gorm.DB, id string) (models.Job, error) {
	var job models.Job
	err := db.Where("id = ?", id).First(&job).Error
	if isNotFound(err) {
		return models.Job{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return models.Job{}, fmt.Errorf("jobs: load job: %w", err)
	}
	return job, nil
}

// isNotFound reports whether a GORM read missed its row. It is one function so
// every read agrees about what "missing" means before it is rewritten into
// ErrNotFound — the error a caller must not be able to distinguish from
// "hidden".
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// visibleTo applies the shared visibility predicate.
//
// An administrator sees every Job. Everybody else sees a Job only when it
// carries the public visibility class and they own it. That single rule makes
// ownerless and deleted-owner Jobs admin-only for free — the column is NULL and
// compares equal to nothing — and keeps an admin-only Kind (a plugin command
// run, say) hidden from the ordinary user who submitted it, including after a
// role demotion, because the class is a durable property of the row rather than
// a decision recomputed from the asker.
func visibleTo(db *gorm.DB, access Access) *gorm.DB {
	if access.Administrator {
		return db
	}
	return db.Where("visibility_class = ? AND owner_user_id = ?", string(VisibilityOwner), access.UserID)
}

// utcPtr normalizes an optional instant to UTC, allocating a fresh pointer so a
// caller's value can never be written through by a later mutation.
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// replayClassOf maps an acceptance's replay declaration onto the stored class.
func replayClassOf(r ReplayInput) ReplayClass {
	if r.NonReplayable {
		return ReplayClassNonReplayable
	}
	return ReplayClassReplayable
}

// snapshot projects a stored row onto the bounded public view.
func snapshot(job models.Job) Snapshot {
	snap := Snapshot{
		ID:              job.ID,
		Kind:            job.Kind,
		KindVersion:     job.KindVersion,
		State:           State(job.State),
		Phase:           job.Phase,
		Title:           job.Title,
		Summary:         json.RawMessage(job.Summary),
		OwnerUserID:     job.OwnerUserID,
		ActorUserID:     job.ActorUserID,
		Origin:          job.Origin,
		Visibility:      VisibilityClass(job.VisibilityClass),
		ReplayClass:     ReplayClass(job.ReplayClass),
		Version:         job.Version,
		ControlIntent:   job.ControlIntent,
		AcceptedAt:      job.AcceptedAt,
		ScheduledFor:    job.ScheduledFor,
		QueuedAt:        job.QueuedAt,
		StartedAt:       job.StartedAt,
		LastResumedAt:   job.LastResumedAt,
		FinishedAt:      job.FinishedAt,
		RunningDuration: job.RunningDuration,
		PausedDuration:  job.PausedDuration,
		BlockedDuration: job.BlockedDuration,
		QueueDuration:   job.QueueDuration,
		ExpiresAt:       job.ExpiresAt,
		Progress: Progress{
			Phase:     job.Phase,
			Completed: job.ProgressCompleted,
			Total:     job.ProgressTotal,
			Unit:      job.ProgressUnit,
			Message:   job.ProgressMessage,
			ETA:       job.ProgressETA,
		},
	}
	if job.FailureCode != "" || job.FailureClass != "" || job.FailureMessage != "" || job.FailureDiagnosticRef != "" {
		snap.Failure = &Failure{
			Code:          job.FailureCode,
			Class:         job.FailureClass,
			Message:       job.FailureMessage,
			DiagnosticRef: job.FailureDiagnosticRef,
		}
	}
	return snap
}

// lockEventSequence returns the single allocator row, holding it against every
// other publisher for the rest of the transaction.
//
// The row is created on first use, and that insert is deliberately the first
// statement of the publisher's transaction: it is a write, so on SQLite the
// transaction takes the writer lock before it reads anything, which is what
// keeps a read snapshot from being promoted to a write after another connection
// committed (SQLITE_BUSY_SNAPSHOT, for which SQLite does not invoke the busy
// handler). On PostgreSQL the row is locked FOR UPDATE instead, which serializes
// publishers across processes.
func lockEventSequence(tx *gorm.DB) (models.JobEventSequence, error) {
	seed := models.JobEventSequence{ID: models.JobEventSequenceRowID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return models.JobEventSequence{}, fmt.Errorf("jobs: seed event sequence: %w", err)
	}

	query := tx
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var seq models.JobEventSequence
	if err := query.Where("id = ?", models.JobEventSequenceRowID).First(&seq).Error; err != nil {
		return models.JobEventSequence{}, fmt.Errorf("jobs: lock event sequence: %w", err)
	}
	return seq, nil
}

// newEvent builds one lifecycle event row. The identity is a UUIDv7 of its own:
// events are immutable facts and are never addressed by a caller-supplied id.
func newEvent(jobID string, sequence, jobVersion uint64, eventType string, detail []byte, reserved bool, now time.Time) models.JobEvent {
	return models.JobEvent{
		ID:           types.NewUUIDv7(),
		JobID:        jobID,
		Sequence:     sequence,
		JobVersion:   jobVersion,
		Type:         eventType,
		Detail:       types.JSON(detail),
		ReservedHost: reserved,
		CreatedAt:    now,
	}
}
