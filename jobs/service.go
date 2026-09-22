package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// Service is the Job control plane. It holds no database handle: every entry
// point takes a Deps, because transaction membership and request scope ride on
// that handle. A Service is therefore safe to keep for the life of the process.
type Service struct{}

// NewService returns the Job control plane.
func NewService() *Service {
	return &Service{}
}

// Accept durably accepts one Job and reports only after the Job row and its
// initial event have both committed.
//
// The write runs in deps.DB's transaction — the caller's, when the caller has
// one, so a Job can be accepted atomically with the domain write that justifies
// it — and opens one of its own when it does not. Execution begins after
// commit; acceptance never claims work that is already running.
func (s *Service) Accept(deps Deps, acceptance Acceptance) (Snapshot, error) {
	if err := validateAcceptance(&acceptance); err != nil {
		return Snapshot{}, err
	}

	now := deps.now()
	job := models.Job{
		ID:              types.NewUUIDv7(),
		Kind:            acceptance.Kind,
		KindVersion:     acceptance.KindVersion,
		State:           string(acceptance.State),
		Title:           acceptance.Title,
		Summary:         types.JSON(acceptance.Summary),
		OwnerUserID:     copyUint(acceptance.OwnerUserID),
		ActorUserID:     copyUint(acceptance.ActorUserID),
		Origin:          acceptance.Origin,
		VisibilityClass: string(acceptance.Visibility),
		ReplayClass:     string(replayClassOf(acceptance.Replay)),
		Version:         1,
		AcceptedAt:      now,
		ScheduledFor:    utcPtr(acceptance.ScheduledFor),
		StateEnteredAt:  &now,
		// Set explicitly rather than letting GORM's time.Now() through: every
		// stored instant in this module is UTC, and these two are stored instants.
		CreatedAt: now,
		UpdatedAt: now,
	}
	if acceptance.State == StateQueued {
		job.QueuedAt = &now
	}

	var snap Snapshot
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return fmt.Errorf("jobs: store job: %w", err)
		}
		event := newEvent(job.ID, 1, job.Version, EventAccepted, nil, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store accepted event: %w", err)
		}
		snap = snapshot(job)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// Get returns one visible Job. A Job the caller may not see is reported exactly
// as a Job that does not exist, so probing cannot distinguish the two.
func (s *Service) Get(deps Deps, access Access, jobID string) (Snapshot, error) {
	if strings.TrimSpace(jobID) == "" {
		return Snapshot{}, fmt.Errorf("%w: empty job id", ErrNotFound)
	}
	var job models.Job
	err := visibleTo(deps.DB, access).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return Snapshot{}, fmt.Errorf("jobs: load job: %w", err)
	}
	return snapshot(job), nil
}

// validateAcceptance checks and normalizes an acceptance in place.
func validateAcceptance(a *Acceptance) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidAcceptance, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(a.Kind) == "" {
		return invalid("kind is required")
	}
	if len(a.Kind) > MaxKindBytes {
		return invalid("kind is %d bytes, over the %d-byte ceiling", len(a.Kind), MaxKindBytes)
	}
	if a.KindVersion == 0 {
		return invalid("kind version must be at least 1")
	}
	if strings.TrimSpace(a.Origin) == "" {
		return invalid("origin is required")
	}
	if len(a.Origin) > MaxOriginBytes {
		return invalid("origin is %d bytes, over the %d-byte ceiling", len(a.Origin), MaxOriginBytes)
	}
	if !a.State.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownState, a.State)
	}
	if a.State.Terminal() {
		return invalid("a Job cannot be accepted in the terminal state %q", a.State)
	}
	if a.State != StateQueued && a.State != StateScheduled {
		return invalid("a Job is accepted queued or scheduled, not %q: execution begins after commit", a.State)
	}
	if a.State == StateScheduled && a.ScheduledFor == nil {
		return invalid("a scheduled Job must name the concrete time it is scheduled for")
	}
	switch a.Visibility {
	case "":
		a.Visibility = VisibilityOwner
	case VisibilityOwner, VisibilityAdmin:
	default:
		return invalid("unknown visibility class %q", a.Visibility)
	}
	if len(a.Title) > MaxTitleBytes {
		return invalid("title is %d bytes, over the %d-byte ceiling", len(a.Title), MaxTitleBytes)
	}
	if len(a.Summary) > 0 {
		if len(a.Summary) > MaxSummaryBytes {
			return invalid("summary is %d bytes, over the %d-byte ceiling", len(a.Summary), MaxSummaryBytes)
		}
		if !json.Valid(a.Summary) {
			return invalid("summary is not valid JSON")
		}
	} else {
		a.Summary = nil
	}
	if a.OwnerUserID != nil && *a.OwnerUserID == 0 {
		return invalid("owner user id 0 is not an identity")
	}
	if a.ActorUserID != nil && *a.ActorUserID == 0 {
		return invalid("actor user id 0 is not an identity")
	}
	a.ScheduledFor = utcPtr(a.ScheduledFor)

	seen := make(map[LegacyRef]struct{}, len(a.LegacyRefs))
	for _, ref := range a.LegacyRefs {
		if strings.TrimSpace(ref.Namespace) == "" || strings.TrimSpace(ref.Handle) == "" {
			return invalid("a legacy reference needs both a namespace and a handle")
		}
		if _, duplicate := seen[ref]; duplicate {
			return invalid("legacy reference %s/%s is repeated", ref.Namespace, ref.Handle)
		}
		seen[ref] = struct{}{}
	}
	return nil
}

// copyUint copies an optional user id so the caller's value can never be written
// through, and so the stored column never shares a pointer with a request.
func copyUint(v *uint) *uint {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

// legalTargets is the state machine, written once. Terminal states have no
// targets at all: an end state is final, and continuation is a new Job.
//
// The shape of it follows the design: work is accepted queued or scheduled,
// reaches running through dispatch, and may be paused, blocked, returned to the
// queue, or finished. A reconciliation may send a running Job back to the queue
// rather than guessing that it is still alive, and blocked work returns to the
// queue when policy or an operator unblocks it — never straight to running,
// because entering running is what a claim does.
var legalTargets = map[State][]State{
	StateScheduled: {StateQueued, StateRunning, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateQueued:    {StateRunning, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateRunning:   {StateQueued, StatePaused, StateBlocked, StateSucceeded, StateFailed, StateCancelled, StateInterrupted},
	StatePaused:    {StateQueued, StateRunning, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateBlocked:   {StateQueued, StateCancelled, StateFailed, StateInterrupted},
}

// canTransition reports whether the state machine permits from -> to. A
// transition to the state a Job is already in is refused: phase and progress
// updates have their own path, and a same-state "transition" would otherwise
// record a lifecycle event for something that did not happen.
func canTransition(from, to State) bool {
	for _, candidate := range legalTargets[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// Transition applies one lifecycle change, atomically with its event.
//
// The write is guarded by the version and state the caller read AND by the
// execution token that owns the Job, so two executors cannot both move it and a
// stale one cannot publish anything at all. When the guarded update matches no
// row, the caller read a stale Job and gets ErrVersionConflict with nothing
// written.
//
// Durations are accumulated from the transitions rather than derived from the
// timestamps, using an injectable clock, so a Job that returns to the queue
// several times measures all of them and the arithmetic is testable.
func (s *Service) Transition(deps Deps, transition Transition) (Snapshot, error) {
	if err := validateTransition(&transition); err != nil {
		return Snapshot{}, err
	}

	job, err := loadJob(deps.DB, transition.JobID)
	if err != nil {
		return Snapshot{}, err
	}
	if job.Version != transition.ExpectedVersion {
		return Snapshot{}, fmt.Errorf("%w: job %s is at version %d, the request expected %d",
			ErrVersionConflict, job.ID, job.Version, transition.ExpectedVersion)
	}
	if job.ExecutionToken != transition.ExecutionToken {
		return Snapshot{}, fmt.Errorf("%w: job %s is not owned by this executor", ErrStaleExecution, job.ID)
	}
	if !canTransition(State(job.State), transition.To) {
		return Snapshot{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, job.State, transition.To)
	}

	sequence, err := nextEventSequence(deps.DB, job.ID)
	if err != nil {
		return Snapshot{}, err
	}

	now := deps.now()
	next, updates := applyTransition(job, transition, now)
	event := newEvent(
		job.ID,
		sequence,
		next.Version,
		eventTypeFor(transition, job.StartedAt != nil),
		transition.Event.Detail,
		true,
		now,
	)

	var snap Snapshot
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ?", job.ID, job.Version, job.State).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("jobs: apply transition: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: job %s changed while the transition was decided", ErrVersionConflict, job.ID)
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store lifecycle event: %w", err)
		}
		snap = snapshot(next)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// applyTransition derives the next stored row and the column values to write for
// one legal transition. It is pure: the transaction below is what makes it
// durable.
func applyTransition(job models.Job, transition Transition, now time.Time) (models.Job, map[string]any) {
	next := job
	next.State = string(transition.To)
	next.Phase = transition.Phase
	next.Version = job.Version + 1
	next.StateEnteredAt = &now

	// Bank the time spent in the state being left. A clock that appears to move
	// backwards — a corrected NTP step, a test's injected clock — banks zero
	// rather than subtracting.
	if job.StateEnteredAt != nil {
		elapsed := now.Sub(*job.StateEnteredAt)
		if elapsed < 0 {
			elapsed = 0
		}
		switch State(job.State) {
		case StateRunning:
			next.RunningDuration = job.RunningDuration + elapsed
		case StatePaused:
			next.PausedDuration = job.PausedDuration + elapsed
		case StateBlocked:
			next.BlockedDuration = job.BlockedDuration + elapsed
		case StateQueued:
			next.QueueDuration = job.QueueDuration + elapsed
		}
	}

	switch transition.To {
	case StateQueued:
		// QueuedAt is the first instant the Job became queueable; the cumulative
		// QueueDuration measures every wait.
		if next.QueuedAt == nil {
			next.QueuedAt = &now
		}
	case StateRunning:
		if next.StartedAt == nil {
			next.StartedAt = &now
		} else {
			next.LastResumedAt = &now
		}
	}
	if transition.To.Terminal() {
		next.FinishedAt = &now
	}
	if transition.Failure != nil {
		next.FailureCode = transition.Failure.Code
		next.FailureClass = transition.Failure.Class
		next.FailureMessage = transition.Failure.Message
		next.FailureDiagnosticRef = transition.Failure.DiagnosticRef
	}

	updates := map[string]any{
		"state":                  next.State,
		"phase":                  next.Phase,
		"version":                next.Version,
		"state_entered_at":       next.StateEnteredAt,
		"running_duration":       next.RunningDuration,
		"paused_duration":        next.PausedDuration,
		"blocked_duration":       next.BlockedDuration,
		"queue_duration":         next.QueueDuration,
		"queued_at":              next.QueuedAt,
		"started_at":             next.StartedAt,
		"last_resumed_at":        next.LastResumedAt,
		"finished_at":            next.FinishedAt,
		"failure_code":           next.FailureCode,
		"failure_class":          next.FailureClass,
		"failure_message":        next.FailureMessage,
		"failure_diagnostic_ref": next.FailureDiagnosticRef,
		"updated_at":             now,
	}
	return next, updates
}

// nextEventSequence continues a Job's own timeline: the sequence every stored
// event already reaches, plus one.
//
// Reading it outside the write is safe because the guarded update is what makes
// a transition exclusive: a writer whose version no longer matches matches no
// row and records no event, so two transitions never reach the insert together.
// The unique index on (job_id, sequence) turns a mistake here into a refused
// write rather than a corrupted timeline.
func nextEventSequence(db *gorm.DB, jobID string) (uint64, error) {
	var last *uint64
	if err := db.Model(&models.JobEvent{}).Where("job_id = ?", jobID).
		Select("MAX(sequence)").Scan(&last).Error; err != nil {
		return 0, fmt.Errorf("jobs: read timeline position: %w", err)
	}
	if last == nil {
		return 1, nil
	}
	return *last + 1, nil
}

// lifecycleEventTypes names the event each state's entry records when the caller
// does not name a more specific one.
var lifecycleEventTypes = map[State]string{
	StateScheduled:   EventScheduled,
	StateQueued:      EventQueued,
	StateRunning:     EventStarted,
	StatePaused:      EventPaused,
	StateBlocked:     EventBlocked,
	StateSucceeded:   EventSucceeded,
	StateFailed:      EventFailed,
	StateCancelled:   EventCancelled,
	StateInterrupted: EventInterrupted,
}

// eventTypeFor names the lifecycle event a transition records. An adapter may
// name a more specific one (a phase checkpoint, a bounded recovery summary); the
// derived default is what the state change is called, and entering running a
// second time is "resumed" rather than "started".
func eventTypeFor(transition Transition, alreadyStarted bool) string {
	if transition.Event.Type != "" {
		return transition.Event.Type
	}
	if transition.To == StateRunning && alreadyStarted {
		return EventResumed
	}
	if name, ok := lifecycleEventTypes[transition.To]; ok {
		return name
	}
	return string(transition.To)
}

// validateTransition checks and normalizes a transition request.
func validateTransition(transition *Transition) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidTransition, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(transition.JobID) == "" {
		return invalid("job id is required")
	}
	if transition.ExpectedVersion == 0 {
		return invalid("expected version must be set")
	}
	if !transition.To.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownState, transition.To)
	}
	if len(transition.Phase) > MaxPhaseBytes {
		return invalid("phase is %d bytes, over the %d-byte ceiling", len(transition.Phase), MaxPhaseBytes)
	}
	if len(transition.Event.Type) > MaxEventTypeBytes {
		return invalid("event type is %d bytes, over the %d-byte ceiling", len(transition.Event.Type), MaxEventTypeBytes)
	}
	if len(transition.Event.Detail) > 0 {
		if len(transition.Event.Detail) > MaxEventDetailBytes {
			return invalid("event detail is %d bytes, over the %d-byte ceiling", len(transition.Event.Detail), MaxEventDetailBytes)
		}
		if !json.Valid(transition.Event.Detail) {
			return invalid("event detail is not valid JSON")
		}
	}

	if transition.To == StateFailed {
		if transition.Failure == nil {
			return invalid("a failed Job must record why it failed")
		}
		if strings.TrimSpace(transition.Failure.Code) == "" {
			return invalid("a failure needs a code")
		}
		if len(transition.Failure.Code) > MaxFailureCodeBytes {
			return invalid("failure code is %d bytes, over the %d-byte ceiling", len(transition.Failure.Code), MaxFailureCodeBytes)
		}
		if !knownFailureClass(transition.Failure.Class) {
			return invalid("failure class %q is not one of %s", transition.Failure.Class, strings.Join(FailureClasses, ", "))
		}
		if len(transition.Failure.Message) > MaxFailureMessageBytes {
			return invalid("failure message is %d bytes, over the %d-byte ceiling", len(transition.Failure.Message), MaxFailureMessageBytes)
		}
		if len(transition.Failure.DiagnosticRef) > MaxFailureDiagnosticBytes {
			return invalid("failure diagnostic reference is %d bytes, over the %d-byte ceiling", len(transition.Failure.DiagnosticRef), MaxFailureDiagnosticBytes)
		}
	} else if transition.Failure != nil {
		return invalid("only a transition to failed may record a failure")
	}
	return nil
}

// knownFailureClass reports whether class is in the bounded taxonomy.
func knownFailureClass(class string) bool {
	for _, candidate := range FailureClasses {
		if class == candidate {
			return true
		}
	}
	return false
}

// PublishPendingEvents assigns delivery sequences to committed events that do
// not have one yet, and reports how many it published.
//
// This is the only place in the whole module that touches the sequence
// allocator, and it lives outside every domain transaction on purpose.
// PostgreSQL sequences — and any counter allocated inside a transaction — are
// not commit-ordered: an event allocated first but committed last would take a
// lower delivery sequence than one a subscriber has already consumed, and that
// subscriber would never see it. Publishing after commit makes the sequence
// describe the order facts became durable, which is the only order a resumable
// cursor can be built on.
//
// Per-Job timelines are unaffected: they are ordered by their own sequence and
// are readable the instant the domain transaction commits.
func (s *Service) PublishPendingEvents(deps Deps, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultPublishBatch
	}

	published := 0
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		sequence, err := lockEventSequence(tx)
		if err != nil {
			return err
		}

		var pending []models.JobEvent
		if err := tx.Where("delivery_sequence IS NULL").
			Order("created_at, id").
			Limit(limit).
			Find(&pending).Error; err != nil {
			return fmt.Errorf("jobs: read unsequenced events: %w", err)
		}
		if len(pending) == 0 {
			return nil
		}

		for i := range pending {
			sequence.Value++
			// The row is claimed by the update, not by the read: a publisher
			// that lost its read is filtered out here rather than assigning a
			// second sequence to an event somebody else already published.
			result := tx.Model(&models.JobEvent{}).
				Where("id = ? AND delivery_sequence IS NULL", pending[i].ID).
				Update("delivery_sequence", sequence.Value)
			if result.Error != nil {
				return fmt.Errorf("jobs: assign delivery sequence: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				continue
			}
			published++
		}

		if published == 0 {
			return nil
		}
		if err := tx.Model(&models.JobEventSequence{}).
			Where("id = ?", models.JobEventSequenceRowID).
			Update("value", sequence.Value).Error; err != nil {
			return fmt.Errorf("jobs: advance event sequence: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, nil
}
