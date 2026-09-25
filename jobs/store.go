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

// requireExecutionToken refuses a write from an executor that does not own the
// Job. It is the fence every executor-side entry point opens with: progress,
// events, outputs and finishing all use it, so a replaced worker cannot publish
// anything through the token its dead claim held.
func requireExecutionToken(job models.Job, token string) error {
	if job.ExecutionToken != token {
		return fmt.Errorf("%w: job %s is not owned by this executor", ErrStaleExecution, job.ID)
	}
	return nil
}

// requireNonterminal refuses an executor-side write to a finished Job: terminal
// state and identity are immutable, and progress or an output published after
// the outcome would make the record describe an execution that had already
// ended.
func requireNonterminal(job models.Job) error {
	if State(job.State).Terminal() {
		return fmt.Errorf("%w: job %s is %s, and a terminal Job never changes",
			ErrIllegalTransition, job.ID, job.State)
	}
	return nil
}

// copyInt64 copies an optional amount so the stored column never shares a
// pointer with the caller's progress snapshot.
func copyInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
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

// executionTokenMatch is the predicate a lifecycle write carries so that it can
// only commit while the execution that decided it still owns the Job. The token
// is compared alongside the version and state because a release clears the token
// without moving either of the other two, and COALESCE tolerates the NULL a row
// written before the column existed can hold.
const executionTokenMatch = "COALESCE(execution_token, '') = ?"

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

// viewerSnapshot projects a stored row for one viewer's sight.
//
// A Job's failure carries two texts: the sanitized message a person reads, and a
// protected diagnostic reference — an internal path, a run id — that §8 keeps for
// administrative audit. The second one is what this withholds from everybody who
// is not an administrator, so no ordinary read, listing or lineage projection can
// turn an infrastructure pointer into a viewer-visible fact.
func viewerSnapshot(job models.Job, access Access) Snapshot {
	snap := snapshot(job)
	if access.Administrator {
		return snap
	}
	return withoutDiagnosticRef(snap)
}

// withoutDiagnosticRef returns the snapshot with the protected diagnostic
// reference removed. It copies the failure rather than mutating it, so a
// projection never rewrites the row another reader is holding.
func withoutDiagnosticRef(snap Snapshot) Snapshot {
	if snap.Failure == nil || snap.Failure.DiagnosticRef == "" {
		return snap
	}
	redacted := *snap.Failure
	redacted.DiagnosticRef = ""
	snap.Failure = &redacted
	return snap
}

// snapshot projects a stored row onto the bounded public view.
func snapshot(job models.Job) Snapshot {
	snap := Snapshot{
		ID:                 job.ID,
		Kind:               job.Kind,
		KindVersion:        job.KindVersion,
		State:              State(job.State),
		Phase:              job.Phase,
		Title:              job.Title,
		Summary:            json.RawMessage(job.Summary),
		OwnerUserID:        job.OwnerUserID,
		ActorUserID:        job.ActorUserID,
		Origin:             job.Origin,
		Visibility:         VisibilityClass(job.VisibilityClass),
		ReplayClass:        ReplayClass(job.ReplayClass),
		ExecutionPrincipal: executionPrincipalOf(job),
		Version:            job.Version,
		ControlIntent:      job.ControlIntent,
		AcceptedAt:         job.AcceptedAt,
		ScheduledFor:       job.ScheduledFor,
		QueuedAt:           job.QueuedAt,
		StartedAt:          job.StartedAt,
		LastResumedAt:      job.LastResumedAt,
		FinishedAt:         job.FinishedAt,
		RunningDuration:    job.RunningDuration,
		PausedDuration:     job.PausedDuration,
		BlockedDuration:    job.BlockedDuration,
		QueueDuration:      job.QueueDuration,
		ExpiresAt:          job.ExpiresAt,
		Progress: Progress{
			Phase:     job.Phase,
			Completed: job.ProgressCompleted,
			Total:     job.ProgressTotal,
			Unit:      job.ProgressUnit,
			Message:   job.ProgressMessage,
			ETA:       job.ProgressETA,
			Metrics:   decodeMetrics(job.ProgressMetrics),
		},
		ProgressSeries:    decodeSeries(job.ProgressSeries),
		ProgressUpdatedAt: job.ProgressUpdatedAt,
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

// appendEventTx stores one appended event on a handle that already holds the
// Job's row, applying the capacity rule first.
//
// The count and the sequence are read after that write rather than before it, so
// two appends racing on one Job cannot both take the same position; the unique
// index on (job_id, sequence) is the backstop that turns a mistake here into a
// refused write instead of a corrupted timeline.
func appendEventTx(tx *gorm.DB, job models.Job, event EventInput, now time.Time) error {
	var stored int64
	if err := tx.Model(&models.JobEvent{}).Where("job_id = ?", job.ID).Count(&stored).Error; err != nil {
		return fmt.Errorf("jobs: count events: %w", err)
	}

	if !event.ReservedHost && stored >= MaxOptionalEventsPerJob {
		return recordTruncationEvent(tx, job, now)
	}

	sequence, err := nextEventSequence(tx, job.ID)
	if err != nil {
		return err
	}
	row := newEvent(job.ID, sequence, job.Version, event.Type, event.Detail, event.ReservedHost, now)
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("jobs: store event: %w", err)
	}
	return nil
}

// recordTruncationEvent records the one visible warning that says optional event
// capacity was exhausted, and does nothing at all once it exists. It is a
// reserved host event, which is why it fits while the optional capacity that
// triggered it is full.
func recordTruncationEvent(tx *gorm.DB, job models.Job, now time.Time) error {
	var existing int64
	if err := tx.Model(&models.JobEvent{}).
		Where("job_id = ? AND type = ?", job.ID, EventTruncated).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("jobs: read truncation warning: %w", err)
	}
	if existing > 0 {
		return nil
	}

	sequence, err := nextEventSequence(tx, job.ID)
	if err != nil {
		return err
	}
	detail, err := json.Marshal(map[string]any{
		"reason":          "optional event capacity exhausted",
		"optionalCeiling": MaxOptionalEventsPerJob,
	})
	if err != nil {
		return fmt.Errorf("jobs: encode truncation detail: %w", err)
	}
	row := newEvent(job.ID, sequence, job.Version, EventTruncated, detail, true, now)
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("jobs: store truncation warning: %w", err)
	}
	return nil
}

// storeLegacyHandles records the legacy identifiers one accepted Job answers to.
//
// It runs inside acceptance's own transaction, so "a legacy submission response
// supplies a handle" is a property of the acceptance rather than of a second
// write that could be lost, and a handle can never name a Job that was never
// stored.
func storeLegacyHandles(tx *gorm.DB, jobID string, refs []LegacyRef, now time.Time) error {
	for _, ref := range refs {
		row := models.JobLegacyHandle{
			Namespace: ref.Namespace,
			Handle:    ref.Handle,
			JobID:     jobID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("jobs: store legacy handle %s/%s: %w", ref.Namespace, ref.Handle, err)
		}
	}
	return nil
}

// moveLegacyHandles points every handle the ancestor answers to at its successor,
// in the successor's own acceptance transaction.
//
// It is a movement rather than a new row because a handle names one current leaf:
// the client that kept polling the old id is asking about the execution that id
// now means, and inserting a second row for the successor would leave the handle
// resolving to the immutable ancestor the Retry deliberately did not change.
// Successors of a Retry are the only callers: a handle projects the linear Retry
// lineage, and a Repeat's branch is not on it.
func moveLegacyHandles(tx *gorm.DB, ancestorJobID, successorJobID string, now time.Time) error {
	result := tx.Model(&models.JobLegacyHandle{}).
		Where("job_id = ?", ancestorJobID).
		Updates(map[string]any{"job_id": successorJobID, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("jobs: move legacy handles: %w", result.Error)
	}
	return nil
}

// ResolveLegacyHandle answers which canonical Job one legacy identifier currently
// names, or ErrNotFound.
//
// It is the resolution half of the compatibility surface and nothing else: a
// handle carries no authority, so the caller authorizes the Job it resolves to
// exactly as it would any other — which is why this reads no visibility
// predicate and returns an identity rather than a snapshot.
func (s *Service) ResolveLegacyHandle(deps Deps, namespace, handle string) (string, error) {
	if deps.DB == nil {
		return "", fmt.Errorf("%w: no database handle", ErrNotFound)
	}
	if namespace == "" || handle == "" {
		return "", fmt.Errorf("%w: an empty legacy reference names nothing", ErrNotFound)
	}
	var row models.JobLegacyHandle
	err := deps.DB.Where("namespace = ? AND handle = ?", namespace, handle).First(&row).Error
	if err != nil {
		if isNotFound(err) {
			return "", fmt.Errorf("%w: %s/%s", ErrNotFound, namespace, handle)
		}
		return "", fmt.Errorf("jobs: resolve legacy handle: %w", err)
	}
	return row.JobID, nil
}

// LegacyHandlesFor lists the handles one Job currently answers to, ordered so a
// projection built from them is stable.
func (s *Service) LegacyHandlesFor(deps Deps, jobID string) ([]LegacyRef, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("%w: no database handle", ErrNotFound)
	}
	var rows []models.JobLegacyHandle
	err := deps.DB.Where("job_id = ?", jobID).Order("namespace asc, handle asc").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: read legacy handles: %w", err)
	}
	refs := make([]LegacyRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, LegacyRef{Namespace: row.Namespace, Handle: row.Handle})
	}
	return refs, nil
}
