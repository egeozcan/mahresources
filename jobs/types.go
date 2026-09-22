// Package jobs owns the durable Job control plane: identity, normalized
// lifecycle, ownership and visibility, lineage, significant events, progress
// snapshots, claims, outputs, commands and retention.
//
// It sits below application_context (which supplies the Kind adapters and the
// authorization-aware facades) and above models (which supplies the storage
// shape). Like groupio and search, it holds no database handle: every entry
// point takes a Deps carrying the caller's *gorm.DB, because transaction
// membership and request scope ride on that handle and a captured one would run
// outside both — silently.
//
// The module does not execute every workload. Downloads, plugin VMs, plugin
// commands, imports and Resource Reductions keep their specialized executors,
// which publish through this package rather than writing lifecycle rows
// directly.
package jobs

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// State is a Job's normalized lifecycle classification. A Kind may publish a
// finer Phase without redefining State.
type State string

const (
	// StateScheduled is accepted for a concrete future time.
	StateScheduled State = "scheduled"
	// StateQueued is eligible and waiting for dispatch.
	StateQueued State = "queued"
	// StateRunning means an executor owns the work.
	StateRunning State = "running"
	// StatePaused means an executor confirmed a resumable checkpoint.
	StatePaused State = "paused"
	// StateBlocked cannot proceed without policy, dependency or operator
	// resolution.
	StateBlocked State = "blocked"
	// StateSucceeded means required side effects and outputs committed.
	StateSucceeded State = "succeeded"
	// StateFailed means execution ended unsuccessfully.
	StateFailed State = "failed"
	// StateCancelled means an accepted cancellation won lifecycle ownership and
	// execution stopped. It says nothing about side effects already produced.
	StateCancelled State = "cancelled"
	// StateInterrupted means execution ended unexpectedly and requires a new Job
	// to continue.
	StateInterrupted State = "interrupted"
)

// AllStates lists every state, in the order the design presents them.
var AllStates = []State{
	StateScheduled, StateQueued, StateRunning, StatePaused, StateBlocked,
	StateSucceeded, StateFailed, StateCancelled, StateInterrupted,
}

// Valid reports whether s is a state this release knows.
func (s State) Valid() bool {
	for _, candidate := range AllStates {
		if s == candidate {
			return true
		}
	}
	return false
}

// Terminal reports whether s is an end state. Terminal state and identity are
// immutable: a continuation is a new Job.
func (s State) Terminal() bool {
	switch s {
	case StateSucceeded, StateFailed, StateCancelled, StateInterrupted:
		return true
	default:
		return false
	}
}

// VisibilityClass is the durable visibility a Kind fixes for its Jobs. Request
// input cannot choose it, and it is what the one shared visibility predicate
// reads.
type VisibilityClass string

const (
	// VisibilityOwner means the Job is listed for its owner (and for
	// administrators). A Job with no owner is therefore admin-only.
	VisibilityOwner VisibilityClass = "owner"
	// VisibilityAdmin means the Job is admin-only regardless of who submitted
	// it. Plugin command runs and imports are registered this way.
	VisibilityAdmin VisibilityClass = "admin"
)

// ReplayClass records whether acceptance carried replayable input.
type ReplayClass string

const (
	// ReplayClassReplayable means the Job's input can be replayed for a
	// Retry/Repeat, subject to current authorization and policy at that time.
	ReplayClassReplayable ReplayClass = "replayable"
	// ReplayClassNonReplayable means the input cannot be replayed at all — a
	// closure-backed mah.start_job, whose captured state died with the process
	// that made it. Such a Job never advertises Retry.
	ReplayClassNonReplayable ReplayClass = "non-replayable"
)

// Failure classes: a bounded taxonomy, because aggregates group on it and raw
// error text is neither a taxonomy nor command policy.
const (
	FailureClassPolicy       = "policy"
	FailureClassValidation   = "validation"
	FailureClassDependency   = "dependency"
	FailureClassTimeout      = "timeout"
	FailureClassCapacity     = "capacity"
	FailureClassConflict     = "conflict"
	FailureClassCancellation = "cancellation"
	FailureClassInternal     = "internal"
)

// FailureClasses lists the taxonomy. A failure that names a class outside it is
// refused rather than stored under a new spelling nobody aggregates.
var FailureClasses = []string{
	FailureClassPolicy, FailureClassValidation, FailureClassDependency,
	FailureClassTimeout, FailureClassCapacity, FailureClassConflict,
	FailureClassCancellation, FailureClassInternal,
}

// Lifecycle event types. Kind adapters may append their own types for
// phase/checkpoint facts; these are the ones the Service writes itself.
const (
	EventAccepted    = "accepted"
	EventScheduled   = "scheduled"
	EventQueued      = "queued"
	EventStarted     = "started"
	EventResumed     = "resumed"
	EventPaused      = "paused"
	EventBlocked     = "blocked"
	EventSucceeded   = "succeeded"
	EventFailed      = "failed"
	EventCancelled   = "cancelled"
	EventInterrupted = "interrupted"
)

// Bounds on everything searchable. A summary, event detail or failure message
// over its ceiling is refused at the boundary that accepts it rather than
// truncated silently.
const (
	MaxTitleBytes             = 200
	MaxSummaryBytes           = 16 << 10
	MaxEventDetailBytes       = 8 << 10
	MaxFailureMessageBytes    = 1000
	MaxFailureCodeBytes       = 60
	MaxFailureDiagnosticBytes = 500
	MaxKindBytes              = 120
	MaxOriginBytes            = 40
	MaxPhaseBytes             = 60
	MaxEventTypeBytes         = 60
	// DefaultPublishBatch bounds one publisher transaction's work.
	DefaultPublishBatch = 200
)

// Deps is the per-call input. Rebuild it for every call — never cache one — so
// that a transactional handle is picked up rather than whichever handle
// happened to be current when the Service was built. Now is injectable so
// timestamp and duration behavior is testable; a nil Now means the wall clock.
type Deps struct {
	DB  *gorm.DB
	Now func() time.Time
}

// now returns the current instant in UTC. Every stored instant is normalized to
// UTC on the way in, so the two supported databases agree about what a stored
// timestamp means.
func (d Deps) now() time.Time {
	if d.Now == nil {
		return time.Now().UTC()
	}
	return d.Now().UTC()
}

// Access is the asking principal as the visibility predicate needs it: an
// administrator sees everything, everybody else sees Jobs they own under the
// public visibility class.
type Access struct {
	UserID        uint
	Administrator bool
}

// Progress is the latest bounded progress snapshot. Routine ticks update it;
// they are not events.
type Progress struct {
	Phase     string
	Completed *int64
	Total     *int64
	Unit      string
	Message   string
	ETA       *time.Time
}

// Failure describes an unsuccessful outcome in bounded, sanitized terms.
type Failure struct {
	Code          string
	Class         string
	Message       string
	DiagnosticRef string
}

// LegacyRef is one legacy identifier a Job was migrated from or submitted with,
// such as a download handle. It carries no authority: a handle resolves under
// current authorization, and its movement is recorded separately from the
// canonical UUID, which never changes meaning.
type LegacyRef struct {
	Namespace string
	Handle    string
}

// ReplayInput is an adapter's declaration about the input a Job is accepted
// with. NonReplayable marks work whose input cannot be replayed at all; the
// default is that the input is replayable, and acceptance records which of the
// two it is.
type ReplayInput struct {
	NonReplayable bool
}

// Acceptance is one durable acceptance request. It is produced by a Kind
// adapter — never by request input — which is why Visibility is a field here
// rather than something the Service guesses: the adapter's registered
// Definition is the only thing that knows whether its Kind is owner-visible or
// admin-only.
type Acceptance struct {
	Kind        string
	KindVersion uint
	// State is the initial state, always a nonterminal one: execution begins
	// after commit, so acceptance cannot claim work is already running.
	State        State
	OwnerUserID  *uint
	ActorUserID  *uint
	Origin       string
	Visibility   VisibilityClass
	Title        string
	Summary      json.RawMessage
	Replay       ReplayInput
	ScheduledFor *time.Time
	LegacyRefs   []LegacyRef
}

// Snapshot is the bounded public view of one Job: what a list, a detail page or
// an adapter sees. It never carries the replay envelope or any executor-internal
// state.
type Snapshot struct {
	ID              string
	Kind            string
	KindVersion     uint
	State           State
	Phase           string
	Title           string
	Summary         json.RawMessage
	OwnerUserID     *uint
	ActorUserID     *uint
	Origin          string
	Visibility      VisibilityClass
	ReplayClass     ReplayClass
	Version         uint64
	ControlIntent   string
	Failure         *Failure
	Progress        Progress
	AcceptedAt      time.Time
	ScheduledFor    *time.Time
	QueuedAt        *time.Time
	StartedAt       *time.Time
	LastResumedAt   *time.Time
	FinishedAt      *time.Time
	RunningDuration time.Duration
	PausedDuration  time.Duration
	BlockedDuration time.Duration
	QueueDuration   time.Duration
	ExpiresAt       *time.Time
}

// Terminal reports whether the snapshot's Job reached an end state.
func (s Snapshot) Terminal() bool {
	return s.State.Terminal()
}

// Transition is one lifecycle request. It names the Job, the version the caller
// believes is current, and the state to enter.
type Transition struct {
	JobID string
	// ExpectedVersion is the optimistic precondition. A transition that does not
	// match the stored version is refused and writes nothing.
	ExpectedVersion uint64
	// ExecutionToken, when non-empty, must equal the Job's stored token. An
	// executor that lost its claim cannot publish through a stale token.
	ExecutionToken string
	To             State
	Phase          string
	Event          EventInput
	Failure        *Failure
}

// EventInput is a significant event to record alongside a transition (or on its
// own). Type is optional: an empty Type derives the lifecycle event name from
// the transition itself.
type EventInput struct {
	Type   string
	Detail json.RawMessage
}

// Errors callers can match. Every one of them is refusal-shaped: nothing is
// written and no event is recorded.
var (
	// ErrNotFound covers both "no such Job" and "a Job you may not see". The two
	// are deliberately indistinguishable, so an unauthorized probe learns
	// nothing from the difference.
	ErrNotFound = errors.New("jobs: job not found")
	// ErrInvalidAcceptance is a malformed acceptance request.
	ErrInvalidAcceptance = errors.New("jobs: invalid acceptance")
	// ErrInvalidTransition is a malformed transition request.
	ErrInvalidTransition = errors.New("jobs: invalid transition")
	// ErrUnknownState is a state this release does not know.
	ErrUnknownState = errors.New("jobs: unknown job state")
	// ErrIllegalTransition is a transition the state machine does not permit.
	ErrIllegalTransition = errors.New("jobs: illegal state transition")
	// ErrVersionConflict is an optimistic-concurrency conflict. The caller read
	// a stale Job and must re-read and re-decide.
	ErrVersionConflict = errors.New("jobs: job version conflict")
	// ErrStaleExecution is a transition from an executor whose token no longer
	// owns the Job.
	ErrStaleExecution = errors.New("jobs: stale execution token")
)
