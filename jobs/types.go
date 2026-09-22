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

	"mahresources/models"

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

// OutputAvailability is what a viewer may do with one output right now. It is
// independent of the Job's outcome: an expired artifact never rewrites the
// success that produced it, and a succeeded Job with an expired artifact still
// succeeded.
type OutputAvailability string

const (
	// OutputAvailable means the output is published and still usable.
	OutputAvailable OutputAvailability = "available"
	// OutputExpired means its planned expiry has passed. It is visible in
	// advance and recorded rather than inferred from the clock at read time, so
	// every reader agrees about what happened.
	OutputExpired OutputAvailability = "expired"
	// OutputRemoved means the artifact is confirmed gone — including the case
	// where a sweep found it already missing, which is removed rather than an
	// error.
	OutputRemoved OutputAvailability = "removed"
)

// Output types. The vocabulary is closed so that a reader — a detail page, a
// command, an export — can branch on it with one spelling per concept, and so
// that a Kind cannot invent a type nobody knows how to open.
const (
	OutputTypeEntity       = "entity"
	OutputTypeArtifact     = "artifact"
	OutputTypeReport       = "report"
	OutputTypeSummary      = "summary"
	OutputTypeExternalLink = "external-link"
	OutputTypeLog          = "log"
)

// OutputTypes lists the vocabulary. A type outside it is refused rather than
// stored under a new spelling nobody can open.
var OutputTypes = []string{
	OutputTypeEntity, OutputTypeArtifact, OutputTypeReport,
	OutputTypeSummary, OutputTypeExternalLink, OutputTypeLog,
}

// OutputInput is one typed output an adapter publishes. Reference is the bounded
// payload the adapter's own reader understands — an entity id, an artifact name
// and size, a report URL — and this module never interprets it. Required marks
// the outputs a Job's success depends on: every stored required output must be
// available before Finish may record success.
type OutputInput struct {
	Key       string
	Type      string
	Label     string
	Reference json.RawMessage
	Required  bool
	ExpiresAt *time.Time
}

// Output is the bounded view of a published output.
type Output struct {
	ID           string
	JobID        string
	Key          string
	Type         string
	Label        string
	Reference    json.RawMessage
	Required     bool
	Availability OutputAvailability
	Version      uint64
	ExpiresAt    *time.Time
	RemovedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Link types. A Job keeps its own identity and outcome inside a lineage; the
// link only records how two Jobs relate. The spellings are the storage model's,
// so an inserted row and a query can never disagree about one of them.
type LinkType string

const (
	// LinkRetryOf relates a successor to the unsuccessful Job it recovers from:
	// FromJobID is the successor, ToJobID the ancestor.
	LinkRetryOf LinkType = models.JobLinkRetryOf
	// LinkRepeatOf relates a successor to the successful Job it re-runs:
	// FromJobID is the successor, ToJobID the ancestor.
	LinkRepeatOf LinkType = models.JobLinkRepeatOf
	// LinkParentChild relates independent workflow stages: FromJobID is the
	// parent, ToJobID the child.
	LinkParentChild LinkType = models.JobLinkParentChild
)

// LinkTypes lists the vocabulary. A relation outside it is refused rather than
// stored under a spelling nothing can follow.
var LinkTypes = []string{
	string(LinkRetryOf), string(LinkRepeatOf), string(LinkParentChild),
}

// LinkRequest records one typed relation between two Jobs.
type LinkRequest struct {
	Type      LinkType
	FromJobID string
	ToJobID   string
}

// FinishRequest ends one execution. It is a terminal transition plus the output
// verification success depends on, so a caller confirms the artifacts it
// promised and the outcome in one call rather than in two writes that could
// disagree.
type FinishRequest struct {
	ExecutionRef
	// ExpectedVersion is the optimistic precondition, exactly as on a
	// transition: a Job that moved underneath the executor is refused and must be
	// re-read before it is ended.
	ExpectedVersion uint64
	// Outcome is the terminal state to enter.
	Outcome State
	// Event may name a more specific terminal event; an empty Type derives it
	// from Outcome.
	Event   EventInput
	Failure *Failure
	// RequiredOutputs names the output keys success depends on. A named key with
	// no published row is missing; every stored required row is verified whether
	// or not it is named, because the durable row is the authority and an adapter
	// that forgot to name one must not slip past the check.
	RequiredOutputs []string
}

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
	// EventWarning is a bounded, non-fatal warning an adapter or the Service
	// records: an optional output that could not be made available, a recovery
	// summary. It never changes the Job's outcome.
	EventWarning = "warning"
	// EventTruncated is the single visible warning recorded when optional event
	// capacity is exhausted. It is itself a reserved host event, so it is
	// recorded even though the capacity that triggered it is full, and exactly
	// one of them exists per Job.
	EventTruncated = "events-truncated"
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
	MaxProgressUnitBytes      = 20
	MaxProgressMessageBytes   = 500
	// DefaultPublishBatch bounds one publisher transaction's work.
	DefaultPublishBatch = 200
)

// Event capacity. Optional Kind traffic — phase chatter, checkpoints, adapter
// warnings — is bounded, and the headroom reserved for host facts inside that
// ceiling is what a lifecycle, terminal, control or output fact can always rely
// on. Refusing a terminal event because an adapter was talkative would make an
// adapter's chatter decide whether a Job may finish.
const (
	// MaxEventsPerJob is the point a Job's timeline reaches before optional
	// traffic is truncated. Host facts are recorded past it — they are the ones
	// the reservation exists for — and the reservation is what makes that
	// unreachable in practice.
	MaxEventsPerJob = 500
	// ReservedHostEventCapacity is the headroom inside MaxEventsPerJob that
	// optional traffic may never consume.
	ReservedHostEventCapacity = 100
	// MaxOptionalEventsPerJob is what optional traffic is actually bounded by:
	// the ceiling minus the reserved headroom.
	MaxOptionalEventsPerJob = MaxEventsPerJob - ReservedHostEventCapacity
)

// Bounds on the typed outputs one Job may publish.
const (
	// MaxOutputsPerJob bounds one Job's output list. Publishing a key the Job
	// already has is always allowed; it replaces rather than adds.
	MaxOutputsPerJob        = 64
	MaxOutputKeyBytes       = 120
	MaxOutputTypeBytes      = 40
	MaxOutputLabelBytes     = 200
	MaxOutputReferenceBytes = 8 << 10
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

// ExecutionRef names the execution that owns a Job's writes: which Job, and the
// execution token the claim on it was created with. An executor may publish only
// under the token it owns, so a worker whose claim was replaced cannot report
// progress, append events, publish outputs or finish the Job someone else now
// owns.
//
// JobID and ExecutionToken are named rather than embedded so that a caller
// building one from a request cannot confuse them with an access check: a token
// is a fence, not an authorization decision.
type ExecutionRef struct {
	JobID          string
	ExecutionToken string
}

// Progress is the latest bounded progress snapshot. Routine ticks update it;
// they are not events.
//
// An empty Phase means "no phase change": ticks are frequent and the phase is
// the coarser label a transition or an earlier tick set, so a byte tick must not
// erase it. Every other field replaces what was stored, and a nil Completed or
// Total clears it rather than keeping the previous value.
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
// own). Type is optional on a transition, where an empty Type derives the
// lifecycle event name from the state being entered; an event appended on its
// own must name its type, because there is no transition to derive one from.
type EventInput struct {
	Type   string
	Detail json.RawMessage
	// ReservedHost marks an event whose capacity optional Kind traffic may never
	// consume: host lifecycle, terminal, control and output facts. Transition
	// ignores it — a lifecycle transition *is* a host fact and is always recorded
	// as one — while AppendEvent honours it, so an adapter cannot claim the
	// reserved headroom for its own phase chatter.
	ReservedHost bool
}

// Errors callers can match. Every one of them is refusal-shaped: nothing is
// written and no event is recorded.
var (
	// ErrInvalidExecution is a malformed execution reference: an executor-side
	// request that does not name a Job, or names one in a way that cannot be
	// acted on.
	ErrInvalidExecution = errors.New("jobs: invalid execution reference")
	// ErrInvalidProgress is a progress snapshot outside its bounds.
	ErrInvalidProgress = errors.New("jobs: invalid progress")
	// ErrInvalidEvent is an event appended outside its bounds, or without the
	// type that says what fact it records.
	ErrInvalidEvent = errors.New("jobs: invalid event")
	// ErrInvalidOutput is an output published outside its bounds, or with a type
	// outside the vocabulary.
	ErrInvalidOutput = errors.New("jobs: invalid output")
	// ErrOutputCapacityExhausted means one Job already holds its ceiling of
	// distinct output keys. Republishing a key it has is not refused.
	ErrOutputCapacityExhausted = errors.New("jobs: output capacity exhausted")
	// ErrRequiredOutputUnavailable refuses success whose required outputs are not
	// durable and available — a key never published, or a required row that is
	// expired or removed. Nothing is written: the Job stays where it is until its
	// executor can honestly satisfy the contract.
	ErrRequiredOutputUnavailable = errors.New("jobs: required output is missing or unavailable")
	// ErrInvalidLink is a lineage request outside the vocabulary, or one that
	// links a Job to itself. Naming a Job that does not exist is ErrNotFound:
	// there is nothing to relate it to.
	ErrInvalidLink = errors.New("jobs: invalid job link")
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
