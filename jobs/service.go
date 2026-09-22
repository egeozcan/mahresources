package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Service is the Job control plane. It holds no database handle: every entry
// point takes a Deps, because transaction membership and request scope ride on
// that handle. A Service is therefore safe to keep for the life of the process.
//
// What it does hold is process-lifetime configuration that is not a database
// handle: the Kind-owned replay codecs, and the Kind adapters that fix how each
// Kind's work is dispatched. Those are built once at startup and are not scoped,
// transactional or per-caller, so they belong here rather than on the handle —
// the same line groupio and search draw for their filesystems and backends.
type Service struct {
	replayMu     sync.Mutex
	replayCodecs map[replayCodecKey]ReplayCodec

	adapterMu sync.Mutex
	adapters  map[kindVersion]AdapterRegistration
}

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

	// The input is encoded and sealed before the transaction opens, so a Kind
	// codec that cannot summarize or encode its own input refuses acceptance
	// rather than leaving a Job whose Retry could never work. The sealed bytes
	// are the only form of it that reaches the database.
	var envelope *models.JobReplayEnvelope
	if len(acceptance.Replay.Input) > 0 {
		sealed, summary, err := s.sealReplay(deps, job, acceptance.Replay, now)
		if err != nil {
			return Snapshot{}, err
		}
		envelope = &sealed
		job.Summary = types.JSON(summary)
	}

	var stored Snapshot
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return fmt.Errorf("jobs: store job: %w", err)
		}
		event := newEvent(job.ID, 1, job.Version, EventAccepted, nil, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store accepted event: %w", err)
		}
		if envelope != nil {
			if err := tx.Create(envelope).Error; err != nil {
				return fmt.Errorf("jobs: store replay envelope: %w", err)
			}
		}
		stored = snapshot(job)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	// Filled after commit rather than inside it: the envelope's availability is
	// a read of the row that has just been written, and answering it on the
	// caller's own handle keeps the transaction's statements to writes.
	stored.ReplayAvailability = s.replayAvailabilityOf(deps.DB, replayKeys(deps), job, deps.now())
	return stored, nil
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
	return s.snapshotFor(deps, job), nil
}

// snapshotFor projects a stored row and answers the one question about replay
// input a public snapshot carries: whether this process can open it. It never
// returns the envelope and never decrypts it — a listing or a detail read must
// not spend a key operation per row to render a boolean.
//
// It is for the reader paths (Get, Accept, Forget). Executor-side transitions
// deliberately use plain snapshot(): they run inside a write transaction, and
// answering this question there would take a second database connection while
// the first is held, which is the shape that deadlocks a pool of one.
func (s *Service) snapshotFor(deps Deps, job models.Job) Snapshot {
	snap := snapshot(job)
	snap.ReplayAvailability = s.replayAvailabilityOf(deps.DB, replayKeys(deps), job, deps.now())
	return snap
}

// replayKeys is the keyring a call was made with, or nil when the caller
// configured none.
func replayKeys(deps Deps) *Keyring {
	if deps.Replay == nil {
		return nil
	}
	return deps.Replay.Keys
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
	if len(a.Replay.Input) > 0 && a.Replay.NonReplayable {
		return invalid("input cannot be supplied for work that declares non-replayable input")
	}
	if len(a.Replay.Input) > 0 && len(a.Summary) > 0 {
		// The summary is the only text a reader sees, so it is derived from the
		// input by the Kind's own sanitizer. Letting a request name it as well
		// would be a second path into the one place a secret must never reach.
		return invalid("a replayable Job's summary is produced by its Kind's sanitizer, not named by the request")
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

// UpdateProgress replaces the current progress snapshot of a Job an execution
// owns.
//
// A tick is a snapshot, not a Job Event: it updates the bounded progress columns
// in place and records nothing, because a download reporting every few hundred
// milliseconds would otherwise drown the timeline an operator reads. It does not
// consume the lifecycle version either — a version bump per tick would
// invalidate the version a transition in flight decided from, and a Job could
// then never finish while its executor kept reporting progress.
func (s *Service) UpdateProgress(deps Deps, ref ExecutionRef, progress Progress) (Snapshot, error) {
	if err := validateExecutionRef(ref); err != nil {
		return Snapshot{}, err
	}
	if err := validateProgress(progress); err != nil {
		return Snapshot{}, err
	}

	job, err := loadJob(deps.DB, ref.JobID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := requireExecutionToken(job, ref.ExecutionToken); err != nil {
		return Snapshot{}, err
	}
	if err := requireNonterminal(job); err != nil {
		return Snapshot{}, err
	}

	now := deps.now()
	next := job
	next.ProgressCompleted = copyInt64(progress.Completed)
	next.ProgressTotal = copyInt64(progress.Total)
	next.ProgressUnit = progress.Unit
	next.ProgressMessage = progress.Message
	next.ProgressETA = utcPtr(progress.ETA)
	if progress.Phase != "" {
		next.Phase = progress.Phase
	}

	updates := map[string]any{
		"progress_completed": next.ProgressCompleted,
		"progress_total":     next.ProgressTotal,
		"progress_unit":      next.ProgressUnit,
		"progress_message":   next.ProgressMessage,
		"progress_eta":       next.ProgressETA,
		"phase":              next.Phase,
		"updated_at":         now,
	}

	var snap Snapshot
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The transaction's first statement is the write, which takes the writer
		// lock before anything is read and is what serialises this against a
		// transition racing the same Job.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND execution_token = ? AND state = ?", job.ID, job.ExecutionToken, job.State).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("jobs: update progress: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: job %s changed while the progress update was decided", ErrVersionConflict, job.ID)
		}
		snap = snapshot(next)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// AppendEvent records one significant event on a Job an execution owns.
//
// Typed and bounded, because it is the timeline an operator reads: an event
// without a type says nothing, and an unbounded detail would turn the table into
// a log. Routine progress ticks do not come through here — they are snapshots.
//
// Optional capacity is bounded and the truncation is visible rather than silent:
// once it is exhausted, one `events-truncated` warning is recorded and later
// optional events are dropped without failing the Job or the caller. Dropping is
// deliberate — an adapter whose phase chatter overflowed a ceiling must not have
// that fail the work it is reporting on — and the reserved headroom is what
// keeps a lifecycle or terminal fact out of that bargain.
func (s *Service) AppendEvent(deps Deps, ref ExecutionRef, event EventInput) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	if err := validateAppendedEvent(event); err != nil {
		return err
	}

	job, err := loadJob(deps.DB, ref.JobID)
	if err != nil {
		return err
	}
	if err := requireExecutionToken(job, ref.ExecutionToken); err != nil {
		return err
	}

	now := deps.now()
	return deps.DB.Transaction(func(tx *gorm.DB) error {
		// The first statement is the write, both because it takes the writer lock
		// before anything is read (SQLite) and because it locks the Job row
		// (PostgreSQL) — which is also what serialises two events racing for the
		// same position on one Job's timeline.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND execution_token = ? AND state = ?", job.ID, job.ExecutionToken, job.State).
			Update("updated_at", now)
		if result.Error != nil {
			return fmt.Errorf("jobs: touch job: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: job %s changed while the event was being recorded", ErrVersionConflict, job.ID)
		}
		return appendEventTx(tx, job, event, now)
	})
}

// validateAppendedEvent checks an event appended on its own. The type is
// required here and optional on a transition, where the state being entered
// names the fact.
func validateAppendedEvent(event EventInput) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidEvent, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(event.Type) == "" {
		return invalid("an appended event must name its type")
	}
	if len(event.Type) > MaxEventTypeBytes {
		return invalid("event type is %d bytes, over the %d-byte ceiling", len(event.Type), MaxEventTypeBytes)
	}
	if len(event.Detail) > 0 {
		if len(event.Detail) > MaxEventDetailBytes {
			return invalid("event detail is %d bytes, over the %d-byte ceiling", len(event.Detail), MaxEventDetailBytes)
		}
		if !json.Valid(event.Detail) {
			return invalid("event detail is not valid JSON")
		}
	}
	return nil
}

// Link records one typed durable relation between two Jobs.
//
// Idempotent, because the relation is the fact and recording it twice is still
// one fact: (type, from, to) is unique, so a repeated Retry-link request is the
// row that is already there rather than a second one would-be ancestors have to
// be reconciled against.
//
// It writes the relation and nothing else. Lineage is deliberately not recorded
// as an event on either Job: an event lives on one Job's timeline, and a line
// naming the other endpoint's identity there would leak a relative a viewer is
// not entitled to see. A relation that must be announced is announced by the
// command that created it, in bounded terms.
//
// Both endpoints must exist, because these tables carry no foreign keys and a
// link to a Job that is not there is a row no reader can resolve. Authorization
// is deliberately not checked here: this is the host-side seam, and every read
// that follows a link applies the shared visibility predicate to the Job it
// reaches — a link grants no rights and makes no relative reachable.
func (s *Service) Link(deps Deps, request LinkRequest) error {
	if err := validateLinkRequest(request); err != nil {
		return err
	}

	now := deps.now()
	return deps.DB.Transaction(func(tx *gorm.DB) error {
		var present int64
		if err := tx.Model(&models.Job{}).
			Where("id IN ?", []string{request.FromJobID, request.ToJobID}).
			Count(&present).Error; err != nil {
			return fmt.Errorf("jobs: read link endpoints: %w", err)
		}
		if present != 2 {
			return fmt.Errorf("%w: a link needs both jobs to exist", ErrNotFound)
		}

		link := models.JobLink{
			Type:      string(request.Type),
			FromJobID: request.FromJobID,
			ToJobID:   request.ToJobID,
			CreatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("jobs: store link: %w", err)
		}
		return nil
	})
}

// validateLinkRequest checks a lineage request before anything is read.
func validateLinkRequest(request LinkRequest) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidLink, fmt.Sprintf(format, args...))
	}

	known := false
	for _, candidate := range LinkTypes {
		if string(request.Type) == candidate {
			known = true
			break
		}
	}
	if !known {
		return invalid("link type %q is not one of %s", request.Type, strings.Join(LinkTypes, ", "))
	}
	if strings.TrimSpace(request.FromJobID) == "" || strings.TrimSpace(request.ToJobID) == "" {
		return invalid("a link needs both endpoints")
	}
	if request.FromJobID == request.ToJobID {
		return invalid("a job is not its own relative")
	}
	return nil
}

// validateExecutionRef checks an executor-side request names a Job.
func validateExecutionRef(ref ExecutionRef) error {
	if strings.TrimSpace(ref.JobID) == "" {
		return fmt.Errorf("%w: a job id is required", ErrInvalidExecution)
	}
	return nil
}

// validateProgress checks a progress snapshot against its bounds. Amounts are
// optional, but an amount that is present cannot be negative: a byte count below
// zero renders as a negative percentage rather than as indeterminate work.
func validateProgress(progress Progress) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidProgress, fmt.Sprintf(format, args...))
	}

	if len(progress.Phase) > MaxPhaseBytes {
		return invalid("phase is %d bytes, over the %d-byte ceiling", len(progress.Phase), MaxPhaseBytes)
	}
	if len(progress.Unit) > MaxProgressUnitBytes {
		return invalid("unit is %d bytes, over the %d-byte ceiling", len(progress.Unit), MaxProgressUnitBytes)
	}
	if len(progress.Message) > MaxProgressMessageBytes {
		return invalid("message is %d bytes, over the %d-byte ceiling", len(progress.Message), MaxProgressMessageBytes)
	}
	if progress.Completed != nil && *progress.Completed < 0 {
		return invalid("completed is negative")
	}
	if progress.Total != nil && *progress.Total < 0 {
		return invalid("total is negative")
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

	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		return Snapshot{}, err
	}
	return s.commitTransition(deps, prepared, nil)
}

// Finish ends a Job an execution owns, and is the only way success is recorded.
//
// It is a terminal transition plus the verification §7 requires: when the
// outcome is success, every required output the Job published — and every key
// the caller names — must be durable and available, and that check runs inside
// the same transaction as the terminal state and its event. A Job therefore
// never commits a success beside an output it cannot honestly claim, and a
// refused finish writes nothing at all: the Job stays where it is until its
// executor can satisfy the contract or records a failure instead.
//
// A failed, cancelled or interrupted finish verifies nothing: only success makes
// a promise about outputs.
func (s *Service) Finish(deps Deps, request FinishRequest) (Snapshot, error) {
	transition := Transition{
		JobID:           request.JobID,
		ExpectedVersion: request.ExpectedVersion,
		ExecutionToken:  request.ExecutionToken,
		To:              request.Outcome,
		Event:           request.Event,
		Failure:         request.Failure,
	}
	if err := validateTransition(&transition); err != nil {
		return Snapshot{}, err
	}
	if !request.Outcome.Terminal() {
		return Snapshot{}, fmt.Errorf("%w: Finish ends a Job, and %q is not a terminal state",
			ErrInvalidTransition, request.Outcome)
	}

	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		return Snapshot{}, err
	}

	var verify func(tx *gorm.DB) error
	if request.Outcome == StateSucceeded {
		verify = func(tx *gorm.DB) error {
			return verifyRequiredOutputs(tx, prepared.job.ID, request.RequiredOutputs)
		}
	}
	return s.commitTransition(deps, prepared, verify)
}

// preparedTransition is a transition that has passed every precondition, with
// the row it decided from, the row it will leave behind, the column values and
// the event its transaction will write. Deciding outside the transaction and
// writing inside it is deliberate: the transaction's first statement must be the
// write, so that SQLite takes the writer lock before it reads anything rather
// than promoting a read snapshot afterwards (SQLITE_BUSY_SNAPSHOT, for which its
// busy handler does not run).
type preparedTransition struct {
	job     models.Job
	next    models.Job
	updates map[string]any
	event   models.JobEvent
}

// prepareTransition loads the Job, applies every precondition, and derives what
// the write will look like.
func prepareTransition(deps Deps, transition Transition) (preparedTransition, error) {
	job, err := loadJob(deps.DB, transition.JobID)
	if err != nil {
		return preparedTransition{}, err
	}
	if job.Version != transition.ExpectedVersion {
		return preparedTransition{}, fmt.Errorf("%w: job %s is at version %d, the request expected %d",
			ErrVersionConflict, job.ID, job.Version, transition.ExpectedVersion)
	}
	if err := requireExecutionToken(job, transition.ExecutionToken); err != nil {
		return preparedTransition{}, err
	}
	if !canTransition(State(job.State), transition.To) {
		return preparedTransition{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, job.State, transition.To)
	}

	sequence, err := nextEventSequence(deps.DB, job.ID)
	if err != nil {
		return preparedTransition{}, err
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
	return preparedTransition{job: job, next: next, updates: updates, event: event}, nil
}

// commitTransition writes one decided transition, its event, and — when the
// caller supplies one — the verification the transition's outcome depends on,
// all in one transaction on the caller's handle. The verification runs after the
// guarded update for the same reason the update comes first: the transaction is
// a write transaction from its first statement. Its refusal rolls the update
// back, so the outcome and the evidence for it still commit together or not at
// all.
func (s *Service) commitTransition(deps Deps, prepared preparedTransition, verify func(tx *gorm.DB) error) (Snapshot, error) {
	var snap Snapshot
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ?", prepared.job.ID, prepared.job.Version, prepared.job.State).
			Updates(prepared.updates)
		if result.Error != nil {
			return fmt.Errorf("jobs: apply transition: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: job %s changed while the transition was decided", ErrVersionConflict, prepared.job.ID)
		}
		if verify != nil {
			if err := verify(tx); err != nil {
				return err
			}
		}
		// A Job that just reached an end state gives its sealed input its replay
		// deadline. It is stamped here, in the terminal transition's own
		// transaction, so that "expiry starts at finished_at" is a property of
		// the write rather than of a sweep that might run much later — and so a
		// nonterminal Job, whose envelope is execution-required, never acquires
		// a deadline at all.
		if prepared.next.FinishedAt != nil {
			if err := stampReplayExpiry(tx, prepared.next, deps.Replay, deps.now()); err != nil {
				return err
			}
		}
		// An execution owns a Job only while it is running, so a transition that
		// leaves that state ends the ownership: the claim is released, its
		// capacity slots are freed, and the fencing token is cleared so a
		// stopped execution cannot publish under it any more. A paused, queued,
		// blocked or finished Job therefore never holds a slot or a token that
		// no execution can use. A release under a token that owns nothing is a
		// no-op, which is what makes this safe for a host-side transition on a
		// Job no claim ever touched.
		if prepared.next.State != string(StateRunning) {
			if err := releaseClaimTx(tx, prepared.job.ID, prepared.job.ExecutionToken, ReleaseReasonStateChanged, deps.now()); err != nil {
				return err
			}
		}
		if err := tx.Create(&prepared.event).Error; err != nil {
			return fmt.Errorf("jobs: store lifecycle event: %w", err)
		}
		snap = snapshot(prepared.next)
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
