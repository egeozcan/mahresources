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
	"fmt"
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

// PrincipalClass names the principal a Job's execution acts as, fixed at
// acceptance and durable from then on. It exists because the live user
// references are nulled when an account is deleted, which makes "an actor was
// recorded and has since been removed" indistinguishable from "no actor was
// ever recorded" — and the two must not resolve the same way. A recorded actor
// that is gone blocks the Job; an intentionally actorless one runs as the host.
type PrincipalClass string

const (
	// PrincipalActor acts as the Job's actor: the principal whose authority the
	// execution or a command uses.
	PrincipalActor PrincipalClass = "actor"
	// PrincipalOwner acts as the Job's owner, for work whose submitter and actor
	// are the same person and whose adapter recorded only the owner.
	PrincipalOwner PrincipalClass = "owner"
	// PrincipalHost is intentional actorlessness: the host runs the work as
	// itself, which is what a system-originated Job declares. It is a declaration,
	// never a fallback for a principal that disappeared.
	PrincipalHost PrincipalClass = "host"
)

// PrincipalClasses lists the vocabulary. A class outside it is refused rather
// than stored under a new spelling nothing can resolve.
var PrincipalClasses = []PrincipalClass{PrincipalActor, PrincipalOwner, PrincipalHost}

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

// ReleaseRequest is one execution's decision to stop owning a Job: which execution
// it is, why it is stopping, and — when the Job is still running — the nonterminal
// state the releasing runtime decided the Job is left in.
//
// It is a request rather than three arguments because the reason and the state are
// one decision: the reason is what the claim records about the execution that gave
// the Job up, and the state is what the Job is left as. A Job that already left
// running is only handed back — its state is whatever the adapter that ended it
// decided — so To is read when the Job is still running and nowhere else.
type ReleaseRequest struct {
	ExecutionRef
	// Reason is the bounded, classified reason the release records on the claim. It
	// is the runtime's own word for why it stopped, which is why it is kept: a
	// claim released by a state change records "state-changed", and a claim handed
	// back because its execution ended, quiesced or was superseded records that
	// instead.
	Reason string
	// To is the nonterminal state a running Job is left in: whether the work belongs
	// back in the queue for the next process, or paused, or blocked for a person. It
	// is applied atomically with the release, because the two separately would leave a
	// running Job owned by nobody. It is read only where the named execution owns a
	// running Job, so it is required there and ignored by a release that owns nothing.
	// An end state is refused here (ErrReleaseTerminalState) rather than applied:
	// ending a Job is Finish's decision, and Finish is where the failure taxonomy and
	// the required-output verification a terminal outcome needs belong.
	To State
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
	// EventOutputPublished records one output becoming durable and current for its
	// key: §6 lists publication among the significant facts, and a replacement
	// publication — the same key produced again — is one too, or a reader is left
	// with the first reference and no record that it was superseded.
	EventOutputPublished = "output-published"
	// EventOutputExpired records an output reaching its own planned deadline. §7
	// records the event with the availability change, so the timeline says what
	// became of an artifact rather than leaving every reader to compare a clock
	// against a stored instant.
	EventOutputExpired = "output-expired"
	// EventOutputRemoved is the confirmation that one output's artifact is really
	// gone. §7 records a Job Event when an expiry is confirmed rather than merely
	// planned, and this is the confirmation for a removal an adapter performed:
	// the row's availability is what this database believes, and the event is what
	// says somebody established the bytes are gone.
	EventOutputRemoved = "output-removed"
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

// Listing bounds. A page is bounded because the history it walks is expected to
// hold millions of Jobs: a caller asks for what it can render, and one that asks
// for more than the ceiling is refused rather than served a page it did not ask
// for.
const (
	// DefaultPageSize is the page one listing returns when it names no size.
	DefaultPageSize = 50
	// MaxPageSize is the largest page a caller may ask for.
	MaxPageSize = 200
	// DefaultPinLimit is how many Jobs one viewer may pin when the deployment
	// configures no limit. Pinning exempts a Job's metadata and events from
	// ordinary retention, so an unbounded pin list is unbounded history.
	DefaultPinLimit = 100
)

// Event-scan bounds. A timeline and a catch-up page are bounded for the same
// reason a listing is: the durable stream is a cursor a client walks, not a
// response that carries a Job's whole history.
const (
	// DefaultEventPageSize is how many events one scan returns when it names no
	// size.
	DefaultEventPageSize = 200
	// MaxEventPageSize is the largest event page a caller may ask for.
	MaxEventPageSize = 1000
)

// Analysis window bounds. The default is what an interactive question means, and
// the ceiling is where a longer answer stops being an interactive aggregate and
// becomes an export Job.
const (
	// DefaultSummaryWindow is the window an aggregate covers when it names none.
	DefaultSummaryWindow = 30 * 24 * time.Hour
	// MaxSummaryWindow is the longest window an interactive aggregate accepts.
	MaxSummaryWindow = 90 * 24 * time.Hour
)

// Retention defaults and bounds. The two windows are the design's: a month of
// ordinary history, three months for work that did not succeed.
const (
	// DefaultHistoryRetention is how long succeeded and cancelled Jobs stay when
	// nothing configures a window.
	DefaultHistoryRetention = 30 * 24 * time.Hour
	// DefaultAttentionRetention is how long failed and interrupted Jobs stay.
	DefaultAttentionRetention = 90 * 24 * time.Hour
	// DefaultSweepBatch bounds one retention pass when it names no batch.
	DefaultSweepBatch = 100
	// MaxSweepBatch is the largest batch a caller may ask a sweep to delete.
	MaxSweepBatch = 1000
	// DefaultArtifactCleanupRetry is how long an artifact a cleanup could not
	// establish was gone waits before a sweep asks about it again: a Job an
	// unresolved claim protects, and a Kind this process cannot run, both leave
	// the bytes where they are, and the artifact that comes due next must not wait
	// behind them. It is deliberately shorter than the reconcile retry that
	// follows a claim, because an artifact's deadline has its own clock: bytes
	// promised gone an hour ago should not wait long for the pass that can remove
	// them.
	DefaultArtifactCleanupRetry = 15 * time.Minute
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
	// PinLimit is how many Jobs the asking viewer may pin in this deployment. 0
	// means "not configured", which selects DefaultPinLimit rather than
	// "unlimited": a limit a missing value removed would be no limit at all.
	PinLimit int
	// Retention is the ordinary history retention this deployment configured. It
	// rides on the handle for the same reason Replay does — a facade reads it
	// live, so an operator's change applies to the next terminal transition — and
	// a terminal Job's deadline is stamped from it in the terminal transition's
	// own transaction. A nil policy means "not configured", which selects the
	// design's defaults rather than stamping no deadline at all.
	Retention *RetentionPolicy
	// Replay is the deployment's replay configuration for this call: the keyring
	// the module seals and opens envelopes with, and how long finished work's
	// input stays readable. It rides on the handle for the same reason the
	// handle does — a facade builds it from live settings on every call — and a
	// nil value means replay is not configured here, which is a refusal to seal
	// (never a licence to store input in the clear).
	Replay *ReplayConfig
}

// ReplayConfig is what a caller must know to store and read replay input: the
// keys, and the retention that starts at terminal completion.
type ReplayConfig struct {
	// Keys seals and opens envelopes. Nil refuses acceptance of any input.
	Keys *Keyring
	// Retention is how long a finished Job's envelope stays readable. Zero means
	// "not configured", which keeps it indefinitely rather than expiring it on
	// write — the same rule the download retentions follow, and the safe
	// direction for input that a nonterminal Job may still need.
	Retention time.Duration
}

// retention is the policy a call was made with, with the design's defaults
// standing in for an unconfigured one: a terminal Job must always acquire a
// deadline, because a Job that never expires is a Job nobody may ever remove.
func (d Deps) retention() RetentionPolicy {
	if d.Retention == nil {
		return RetentionPolicy{}
	}
	return *d.Retention
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

// Filter selects the Jobs one visible listing, summary or event scan returns.
//
// Every dimension is a durable relational fact — columns and the preference and
// lineage rows — because a listing that filtered in Go would answer its page and
// its counts from different sets, which is how an aggregate leaks a Job its list
// hides. A zero field means "not asked": no predicate is added for it.
type Filter struct {
	// States, Kinds and Origins narrow on the normalized stored spellings. An
	// unknown state is refused rather than matching nothing, because a typo is a
	// question the caller meant to ask and an empty page would hide the mistake.
	States  []string
	Kinds   []string
	Origins []string

	OwnerID *uint
	ActorID *uint

	AcceptedAfter  *time.Time
	AcceptedBefore *time.Time

	// Relationship narrows to Jobs that are the FROM endpoint of a lineage link
	// of that type — the successor a Retry or Repeat created, or the parent of a
	// child stage. It takes a LinkType spelling, so there is one name per
	// relation.
	Relationship string

	// Search matches the bounded, sanitized text a viewer may read: the UUID,
	// title, sanitized summary, sanitized failure message and output labels. It
	// never reaches ciphertext or a protected diagnostic reference.
	Search string

	// Pinned and Dismissed are the asking viewer's own preferences. They are
	// pointers because "not asked" is a third state: nil adds no predicate,
	// false asks for the Jobs the viewer has not pinned or dismissed, and true
	// for the ones they have. Dismissal belongs to one viewer's default list, so
	// it is always asked of Access.UserID and never of the Job's owner.
	Pinned    *bool
	Dismissed *bool

	// Command narrows to Jobs currently offering a command key. It is refused:
	// a command's availability is advertised by its Kind adapter at read time
	// and is not a durable column, so a listing could not answer it without
	// post-filtering in Go — which is exactly the drift the constructor exists
	// to prevent. The command surface is what makes this dimension answerable,
	// and until it does a request naming it is refused rather than silently
	// matching nothing.
	Command string
}

// Cursor is the keyset position a listing continues from: the accepted instant
// and identity of the last Job the previous page returned. It is a value rather
// than an offset because history grows under the reader — a Job accepted while a
// page is open moves every later row down an offset and silently skips one.
type Cursor struct {
	AcceptedAt time.Time
	ID         string
}

// PreferenceRequest is one viewer's change to one Job: whether they dismiss it
// from their default list, whether they pin it, or both. A nil field leaves the
// viewer's answer to that question exactly as it was — which is why the two are
// pointers: "set this" and "clear this" are different requests, and a request
// that names neither is refused rather than doing nothing quietly.
type PreferenceRequest struct {
	JobID     string
	Dismissed *bool
	Pinned    *bool
}

// RetentionPolicy is the ordinary history retention an operator configured: how
// long finished work stays after it reached a terminal state. It is two windows
// because §9 draws that line — succeeded and cancelled work ages out sooner than
// failed and interrupted work, which someone may still be analyzing.
//
// A zero window means "not configured" and selects the design's default, never
// "expire now": a deployment that has not set a value must not delete history on
// the next sweep.
type RetentionPolicy struct {
	History   time.Duration
	Attention time.Duration
}

// windowFor is the retention one terminal state is measured by.
func (p RetentionPolicy) windowFor(state State) time.Duration {
	if state == StateFailed || state == StateInterrupted {
		return p.attention()
	}
	return p.history()
}

func (p RetentionPolicy) history() time.Duration {
	if p.History <= 0 {
		return DefaultHistoryRetention
	}
	return p.History
}

func (p RetentionPolicy) attention() time.Duration {
	if p.Attention <= 0 {
		return DefaultAttentionRetention
	}
	return p.Attention
}

// SweepCursor is where a bounded retention sweep continues from: the finish
// instant and identity of the last Job it examined. Like a listing cursor it is
// a keyset rather than an offset, and for the same reason — the rows in front of
// it are being deleted while it is used.
type SweepCursor struct {
	FinishedAt time.Time
	ID         string
}

// SweepResult is one bounded pass.
type SweepResult struct {
	// Examined is how many expired Jobs the pass looked at, pruned or not.
	Examined int
	// Pruned is how many it took out of ordinary history.
	Pruned int
	// Skipped is how many it left alone: pinned work, work an unresolved claim
	// still protects, and work whose artifacts nothing could establish were
	// removed.
	Skipped int
	// Outputs is how many output rows had their availability recorded: an artifact
	// past its own deadline, an output of a Job whose history the pass pruned, and
	// an artifact a cleanup acknowledged as removed whether or not that history
	// could follow it.
	Outputs int
	// Envelopes is how many replay envelopes the pass purged.
	Envelopes int
	// Next continues the walk, or is nil when this pass reached the end of the
	// expired range.
	Next *SweepCursor
}

// Summary is the bounded aggregate over the Jobs one filter and one window
// match, under the same visibility predicate the listing uses.
type Summary struct {
	// Window, From and To say what the numbers describe: the analysis window,
	// with From and To the instants it covers. AcceptedAt is the column it
	// measures, because the cohort a question is asked about is the work that was
	// accepted in that period.
	Window time.Duration
	From   time.Time
	To     time.Time

	Total   int64
	ByState map[string]int64
	ByKind  map[string]int64

	// Succeeded, Failed and Terminal are the settled outcomes the rate is drawn
	// from. SuccessRate is zero when nothing settled, rather than a division by
	// an empty set.
	Succeeded   int64
	Failed      int64
	Terminal    int64
	SuccessRate float64

	// Queue is measured over every Job in the window; Run over the Jobs that
	// started, because a Job that never ran has no run duration to report rather
	// than a zero one.
	Queue DurationStats
	Run   DurationStats

	// Failures groups the settled failures by their bounded classification,
	// commonest first. It never carries an error message.
	Failures []FailureClassCount
}

// DurationStats is a median and a high percentile, which is what a duration
// question is actually asking: an average over a long tail describes nothing a
// person recognizes.
type DurationStats struct {
	Median time.Duration
	P95    time.Duration
}

// FailureClassCount is one line of a failure breakdown.
type FailureClassCount struct {
	Class string
	Count int64
}

// Page is one bounded, newest-first page of visible Jobs. Next is the cursor to
// continue from, or nil when the listing reached its end.
type Page struct {
	Jobs []Snapshot
	Next *Cursor
}

// Event is the bounded public view of one Job Event: the sanitized fact and
// where it sits in the Job's timeline. It never carries executor-internal state,
// and its Detail is the bounded sanitized JSON the recording site wrote.
type Event struct {
	ID         string
	JobID      string
	Sequence   uint64
	JobVersion uint64
	Type       string
	Detail     json.RawMessage
	// ReservedHost marks the facts optional adapter traffic may never displace.
	ReservedHost bool
	// DeliverySequence is the commit-safe stream position, and it is nil until
	// the post-commit publisher has assigned one. The per-Job timeline is
	// readable without it.
	DeliverySequence *uint64
	CreatedAt        time.Time
}

// Lineage is one visible Job's relatives, as far as the asker may see them: one
// hop of each relation, with every relative authorized independently.
//
// It is deliberately one hop. Lineage does not grant transitive visibility, and
// a hidden relative is neither named nor counted — so a list here says which
// Jobs are related, never how many were withheld.
type Lineage struct {
	Job Snapshot
	// Ancestors are the Jobs this one directly retries or repeats, newest first.
	Ancestors []Snapshot
	// Successors are the Jobs that directly retry or repeat this one.
	Successors []Snapshot
	// Parents are the Jobs this one is a child stage of.
	Parents []Snapshot
	// Children are this Job's child stages.
	Children []Snapshot
}

// ReplayAvailability is what a viewer can do with a Job's replay input right
// now. It is the only thing an ordinary snapshot says about the envelope:
// whether it can be opened, never what it holds. It is independent of the Job's
// outcome — a succeeded Job whose input expired still succeeded — and a value
// other than available is what suppresses Retry and Repeat.
type ReplayAvailability string

const (
	// ReplayAvailabilityNone means there is no input to replay at all: the Job
	// declared non-replayable input, so no envelope exists and none ever will.
	ReplayAvailabilityNone ReplayAvailability = ""
	// ReplayAvailable means a sealed envelope exists and this process holds the
	// key and the Kind codec needed to open it.
	ReplayAvailable ReplayAvailability = "available"
	// ReplayExpired means replay retention passed. The input is gone; history is
	// not.
	ReplayExpired ReplayAvailability = "expired"
	// ReplayForgotten means the input was explicitly purged.
	ReplayForgotten ReplayAvailability = "forgotten"
	// ReplayUnreadable means the input cannot be produced here: no envelope was
	// stored, this process does not hold the key that sealed it, or the Kind
	// version has no registered codec to decode it. It is deliberately one value
	// for three causes — a viewer is entitled to know that Retry is unavailable,
	// not to distinguish a missing key from a corrupt row — while the errors the
	// host gets from OpenReplay do distinguish them.
	ReplayUnreadable ReplayAvailability = "unreadable"
)

// OpenedReplay is one Job's decrypted, migrated input, handed to the executor
// that is about to run it. It is the only value that ever carries the opened
// bytes, and it is never part of a snapshot.
type OpenedReplay struct {
	JobID         string
	Kind          string
	KindVersion   uint
	SchemaVersion uint
	// Input is the Kind's own input, decoded by its registered codec.
	Input json.RawMessage
	// MigratedFrom names the Kind version the envelope was written at when the
	// codec had to migrate it. Nil means it was decoded at its own version.
	MigratedFrom *uint
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
//
// Input is the already validated JSON the adapter wants stored opaquely — the
// Kind's own shape, which this module neither interprets nor stores in the
// clear. It is only ever supplied for replayable work: a non-replayable
// classification that still carried input would be a licence to store bytes
// nothing may replay, so acceptance refuses the pair.
type ReplayInput struct {
	NonReplayable bool
	Input         json.RawMessage
}

// Acceptance is one durable acceptance request. It is produced by a Kind
// adapter rather than by request input.
//
// Visibility is validated against the Kind's registered Definition and replaced
// by it: the Definition is what fixes whether the Kind is owner-visible or
// admin-only, and the field is only an adapter's claim about its own Kind, which
// is refused when it disagrees.
type Acceptance struct {
	Kind        string
	KindVersion uint
	// State is the initial state, always a nonterminal one: execution begins
	// after commit, so acceptance cannot claim work is already running.
	State       State
	OwnerUserID *uint
	ActorUserID *uint
	Origin      string
	Visibility  VisibilityClass
	// ExecutionPrincipal is the principal the execution acts as. An empty value
	// is derived from the recorded references — the actor when there is one, else
	// the owner, else the host — which is what every adapter that does not need
	// to say anything explicit gets.
	ExecutionPrincipal PrincipalClass
	Title              string
	Summary            json.RawMessage
	Replay             ReplayInput
	ScheduledFor       *time.Time
	LegacyRefs         []LegacyRef
}

// Snapshot is the bounded public view of one Job: what a list, a detail page or
// an adapter sees. It never carries the replay envelope or any executor-internal
// state.
type Snapshot struct {
	ID          string
	Kind        string
	KindVersion uint
	State       State
	Phase       string
	Title       string
	Summary     json.RawMessage
	OwnerUserID *uint
	ActorUserID *uint
	Origin      string
	Visibility  VisibilityClass
	// ExecutionPrincipal is the principal this execution acts as. It is a
	// durable fact, not a live lookup: an account deleted since acceptance still
	// names the class, which is what the dispatch refusal reads.
	ExecutionPrincipal PrincipalClass
	ReplayClass        ReplayClass
	// ReplayAvailability says whether this viewer can open the input. A snapshot
	// never carries the envelope itself.
	//
	// It is answered by the reader paths — Get, Accept, Forget — and left empty
	// by an executor-side transition, which reports the Job's own new state and
	// has no viewer to answer for.
	ReplayAvailability ReplayAvailability
	Version            uint64
	ControlIntent      string
	Failure            *Failure
	Progress           Progress
	AcceptedAt         time.Time
	ScheduledFor       *time.Time
	QueuedAt           *time.Time
	StartedAt          *time.Time
	LastResumedAt      *time.Time
	FinishedAt         *time.Time
	RunningDuration    time.Duration
	PausedDuration     time.Duration
	BlockedDuration    time.Duration
	QueueDuration      time.Duration
	ExpiresAt          *time.Time
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
	// consume: host lifecycle, terminal, control and output facts. It is
	// host-internal. Transition ignores it — a lifecycle transition *is* a host
	// fact and is always recorded as one — and AppendEvent, which is the path an
	// adapter's own events take, does not honour a supplied value: an adapter
	// cannot claim the reserved headroom for its phase chatter by labelling it a
	// host fact, so what it appends is bounded by the optional ceiling exactly
	// like every other adapter event.
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
	// ErrInvalidFilter is a listing, summary or event-scan filter outside the
	// vocabulary this release can answer — an unknown state, a relationship that
	// is not a LinkType, or a dimension that is not a durable fact.
	ErrInvalidFilter = errors.New("jobs: invalid filter")
	// ErrInvalidCursor is a keyset position that cannot be continued from.
	ErrInvalidCursor = errors.New("jobs: invalid cursor")
	// ErrInvalidPage is a page size outside the bound a listing accepts.
	ErrInvalidPage = errors.New("jobs: invalid page size")
	// ErrInvalidPreference is a preference request that cannot be acted on: one
	// that names nothing to change, or one made by a principal with no user to
	// belong to.
	ErrInvalidPreference = errors.New("jobs: invalid job preference")
	// ErrPinLimitReached refuses a pin that would take a viewer past the
	// deployment's per-viewer limit. Nothing is written: the viewer unpins
	// something first, or an operator raises the limit.
	ErrPinLimitReached = errors.New("jobs: the pin limit is reached")
	// ErrInvalidWindow is an aggregate window past the interactive ceiling.
	// Refused rather than clamped: a caller asking for a year of history is
	// asking a different question, and one that should become an export Job.
	ErrInvalidWindow = errors.New("jobs: invalid aggregate window")
	// ErrNotFound covers both "no such Job" and "a Job you may not see". The two
	// are deliberately indistinguishable, so an unauthorized probe learns
	// nothing from the difference.
	ErrNotFound = errors.New("jobs: job not found")
	// ErrInvalidAcceptance is a malformed acceptance request.
	ErrInvalidAcceptance = errors.New("jobs: invalid acceptance")
	// ErrExecutionPrincipalUnavailable refuses execution of a Job whose recorded
	// principal no longer exists. Authority is never transferred on user
	// deletion: a Job that was accepted to act as an actor or an owner that has
	// since been removed is blocked, rather than running as the surviving owner,
	// as root, or as the host.
	ErrExecutionPrincipalUnavailable = errors.New("jobs: the principal this job acts as is gone")
	// ErrInvalidTransition is a malformed transition request.
	ErrInvalidTransition = errors.New("jobs: invalid transition")
	// ErrUnknownState is a state this release does not know.
	ErrUnknownState = errors.New("jobs: unknown job state")
	// ErrIllegalTransition is a transition the state machine does not permit.
	ErrIllegalTransition = errors.New("jobs: illegal state transition")
	// ErrRunningRequiresClaim refuses a transition into running. Entering running
	// is what a claim does: Claim writes the state, the fencing token, the claim
	// and the capacity that admitted the Job together, so a Job that is running is
	// one an execution owns, one a heartbeat keeps alive, and one a reconciliation
	// can find. A transition that entered running on its own would leave work
	// nobody owns and nothing reconciles.
	ErrRunningRequiresClaim = errors.New("jobs: entering running is what a claim does")
	// ErrReleaseNeedsState refuses a release of an execution whose Job is still
	// running, when the release names no state to leave it in. A release that wrote
	// only the token would leave a running Job owned by nobody: not claimable, since
	// Claim takes queued or scheduled work, and not reconcilable, since the expiry
	// scan looks for held claims.
	ErrReleaseNeedsState = errors.New("jobs: a release of a running job must name the state it is left in")
	// ErrReleaseTerminalState refuses a release that names an end state. A release
	// is the graceful half of the claim fence: it hands a Job back to whoever comes
	// next, so the state it names is one the next process can pick the work up in.
	// Ending a Job is Finish's decision, and Finish is the entry point that carries
	// the terminal contract — the failure taxonomy a failed Job must record, and the
	// required outputs a success is verified against (see successVerification). A
	// release that could name an end state would be a second way for work to end,
	// reached around both.
	ErrReleaseTerminalState = errors.New("jobs: a release may not end a job; ending one is what Finish does")
	// ErrVersionConflict is an optimistic-concurrency conflict. The caller read
	// a stale Job and must re-read and re-decide.
	ErrVersionConflict = errors.New("jobs: job version conflict")
	// ErrStaleExecution is a transition from an executor whose token no longer
	// owns the Job.
	ErrStaleExecution = errors.New("jobs: stale execution token")

	// ErrInvalidReplayKey is a JOB_REPLAY_KEY value that is not one or more
	// base64-encoded 32-byte keys, or that repeats one.
	ErrInvalidReplayKey = errors.New("jobs: invalid replay key")
	// ErrReplayKeyRequired reports a deployment that could accept durable secret
	// work without a key it will still hold after a restart. It is refused at
	// startup rather than at the first acceptance, because the failure it
	// prevents is silent: envelopes written under a per-boot key are unreadable
	// once that process is gone.
	ErrReplayKeyRequired = errors.New("jobs: a stable replay key is required")
	// ErrInvalidReplayCodec is a Kind codec registration missing a hook or
	// repeating a Kind/version pair.
	ErrInvalidReplayCodec = errors.New("jobs: invalid replay codec")
	// ErrInvalidReplay is a malformed replay request: input where none may be
	// stored, or input beyond its bound.
	ErrInvalidReplay = errors.New("jobs: invalid replay input")
	// ErrReplayAbsent means the Job has no sealed input: it declared
	// non-replayable input, or it was accepted without any.
	ErrReplayAbsent = errors.New("jobs: no replay input is stored")
	// ErrReplayEnvelopeMissing means a Job whose durable class says its input is
	// replayable has no envelope to run with. Whatever removed it — a purge, a
	// migration that could not convert it, a partial restore — the Job may not be
	// executed without it: running work with incomplete input is the one thing
	// §3 forbids outright, so the Job is blocked for a person to resolve.
	ErrReplayEnvelopeMissing = errors.New("jobs: a replayable job has no replay envelope")
	// ErrReplayKeyUnavailable means the envelope names a key this process does
	// not hold. It is distinguishable from corruption on purpose: a rotation
	// that has not been rolled out everywhere, or a key file lost with the data
	// root, is an operator problem, while failed authentication is a data
	// problem.
	ErrReplayKeyUnavailable = errors.New("jobs: the replay key for this envelope is unavailable")
	// ErrReplayCodecUnregistered means no Kind codec is registered that can
	// decode the envelope — no decoder, or no migration from the version it was
	// written at.
	ErrReplayCodecUnregistered = errors.New("jobs: no replay codec is registered for this job kind")
	// ErrReplayCorrupt means the ciphertext failed authentication: it was
	// tampered with, or it was read under a different Job, Kind or Kind version
	// than the one that sealed it.
	ErrReplayCorrupt = errors.New("jobs: replay envelope failed authentication")
	// ErrReplayDecodeFailed means the sealed bytes opened but the Kind's own
	// codec refused them (a payload its decoder or migration cannot read).
	ErrReplayDecodeFailed = errors.New("jobs: replay input could not be decoded")
	// ErrReplayExpired means replay retention passed for a finished Job.
	ErrReplayExpired = errors.New("jobs: replay input has expired")
	// ErrReplayForgotten means the input was explicitly purged.
	ErrReplayForgotten = errors.New("jobs: replay input was forgotten")
	// ErrReplayExecutionRequired refuses a purge while the input is still
	// execution-required: nonterminal work must first be cancelled or reconciled
	// to a terminal state, because purging it would leave a Job that can neither
	// run nor be recovered.
	ErrReplayExecutionRequired = errors.New("jobs: replay input is still required by a nonterminal job")

	// ErrInvalidDefinition is a Kind definition this release cannot dispatch:
	// missing or oversized identity, a version below one, a negative budget, or a
	// (Kind, version) pair that already has an adapter.
	ErrInvalidDefinition = errors.New("jobs: invalid kind definition")
	// ErrAdapterUnregistered means no adapter in this process can run the Kind.
	// It is a refusal rather than a fallback: work whose Kind this release cannot
	// execute is never handed to another Kind's adapter.
	ErrAdapterUnregistered = errors.New("jobs: no adapter is registered for this job kind")
	// ErrInvalidClaim is a malformed claim request.
	ErrInvalidClaim = errors.New("jobs: invalid claim request")
	// ErrCapacityExhausted means every budget a claim asked to occupy is full.
	// Nothing is written: a Job is never left running without the capacity that
	// admitted it.
	ErrCapacityExhausted = errors.New("jobs: the capacity budget is full")
	// ErrInvalidReconcileDecision is a reconciliation answer outside the
	// vocabulary, or one this Kind may not be given.
	ErrInvalidReconcileDecision = errors.New("jobs: invalid reconciliation decision")
)

// Dispatch vocabulary. The defaults are what a Kind that declares nothing
// gets, and they are chosen to be safe rather than convenient: a claim lives
// two minutes without a heartbeat, and reconciliation runs in bounded batches
// so one pass over a large backlog is a series of small writes.
const (
	// DefaultClaimLease is how long a claim survives without a heartbeat when
	// the Kind declares no lease of its own.
	DefaultClaimLease = 2 * time.Minute
	// CapacityGroupGlobal is the deployment-wide concurrency budget, shared by
	// every Kind whose runtime takes it. A Kind that names its own group draws
	// on a per-Kind budget instead.
	CapacityGroupGlobal = "global"
	// DefaultReconcileBatch bounds one reconciliation pass.
	DefaultReconcileBatch = 50
	// DefaultReconcileRetry is how long a claim a reconciliation pass could not
	// decide waits before it may be asked about again. It is a schedule rather
	// than a state: the claim keeps its token, its lease and its capacity.
	DefaultReconcileRetry = 30 * time.Second
	// MaxReconcileRetry caps that wait, so a Kind whose reconciler never answers
	// is retried at a bounded interval rather than abandoned.
	MaxReconcileRetry = 30 * time.Minute
	// DefaultClaimBatch bounds how many Jobs one pass over one Kind claims, so a
	// runtime that falls behind does not claim an unbounded amount of work in a
	// single tick.
	DefaultClaimBatch = 8
	// MaxClaimantBytes bounds the claimant identity a claim records.
	MaxClaimantBytes = 120
	// MaxCapacityGroupBytes bounds a concurrency budget's name, which is stored
	// on every capacity row that occupies it and on nothing else.
	MaxCapacityGroupBytes = 120
	// MaxReleaseReasonBytes bounds the bounded reason a release records.
	MaxReleaseReasonBytes = 40
)

// Release reasons the control plane records. They are stable codes rather than
// prose, because "why did this execution stop owning the Job" is what an
// operator filters on, and free text is not a taxonomy.
const (
	// ReleaseReasonStateChanged is a transition that left the running state: a
	// Job that is finished, paused, blocked or queued is not owned by an
	// execution.
	ReleaseReasonStateChanged = "state-changed"
	// ReleaseReasonExecutionEnded is an execution that returned without ending
	// itself, so the runtime ended its ownership.
	ReleaseReasonExecutionEnded = "execution-ended"
)

// Definition is what a Kind fixes about itself before the control plane
// dispatches any of its work: identity, whether its work may be redispatched
// after runtime loss, which concurrency budget it draws on, and how long a claim
// on it survives without a heartbeat.
type Definition struct {
	Kind        string
	KindVersion uint
	// Restorable marks work a fresh runtime may run again after the one that
	// claimed it disappeared. Closure-backed work — a mah.start_job whose Lua
	// function died with its process — is not restorable, and the control plane
	// refuses to re-queue it on a lease expiry alone: the Job stays blocked with
	// its claim held until something proves the external work stopped.
	Restorable bool
	// Visibility is the durable visibility class every Job of this Kind is
	// accepted with. It is declared here rather than chosen per acceptance
	// because it is a property of the work — a plugin command run is an operator
	// concern nobody but an administrator inspects, whatever account submitted it
	// — and a class a caller could omit would be a class that Kind did not
	// actually fix. The zero value is VisibilityOwner, the class of ordinary
	// user-facing work; an admin-only Kind must say so.
	Visibility VisibilityClass
	// CapacityGroup names the concurrency budget the Kind's executions draw on.
	// An empty group means the Kind's own budget; a Kind that names a group
	// shares it with every other Kind naming it.
	CapacityGroup string
	// MaxConcurrent caps executions inside that budget across the deployment. 0
	// means the budget is not enforced.
	MaxConcurrent int
	// Lease is how long a claim on this Kind survives without a heartbeat. 0
	// selects DefaultClaimLease.
	Lease time.Duration
}

// capacityGroup is the budget the Kind actually draws on, with the empty group
// resolved to the Kind's own.
func (d Definition) capacityGroup() string {
	if d.CapacityGroup == "" {
		return d.Kind
	}
	return d.CapacityGroup
}

// claimLease is the lease the Kind actually uses.
func (d Definition) claimLease() time.Duration {
	if d.Lease <= 0 {
		return DefaultClaimLease
	}
	return d.Lease
}

// EffectiveLease is how long a claim on this Kind survives without a heartbeat,
// with the default applied when the Kind declares none.
//
// It is exported because more than the claim needs the number: a runtime that
// heartbeats a running execution has to extend the lease the claim was created
// with, and a Kind that declared a short lease would otherwise be reconciled out
// from under a runtime that thought it had the default.
func (d Definition) EffectiveLease() time.Duration { return d.claimLease() }

// CapacityRef names one concurrency budget a claim must occupy and how many
// executions that budget allows across the deployment. A limit of 0 means the
// budget is not enforced.
//
// A claim takes its budgets in a deterministic order, so two claims holding
// several of them cannot take them in opposite orders and deadlock.
type CapacityRef struct {
	Group string
	Limit int
}

// ClaimRequest asks the Service to claim the next Job of one Kind that is
// waiting to run, and to hand it back as an Execution.
type ClaimRequest struct {
	Kind        string
	KindVersion uint
	// Claimant identifies the runtime taking the claim: a process, or a named
	// worker inside one. It is recorded as evidence, never as authority — the
	// execution token is what fences publication.
	Claimant string
	// Capacity names every budget the claim must occupy. A claim that cannot
	// occupy all of them is refused and writes nothing at all.
	Capacity []CapacityRef
	// Lease overrides the Kind's declared lease for this claim.
	Lease time.Duration
}

// Execution is everything a Kind adapter is given to run one claimed Job: its
// identity, the fencing token, the decoded input, the principal the work acts
// as, and the report it publishes through.
//
// It deliberately carries no database handle and no lifecycle table: an adapter
// runs work and reports facts, and the Service owns every write. A zero
// Execution has no report, so every publish through one is refused rather than
// silently dropped.
type Execution struct {
	JobID       string
	Kind        string
	KindVersion uint
	// Version is the Job version the claim created. It is the version an adapter
	// reports its first transition against; every transition and finish returns
	// the next one.
	Version        uint64
	ExecutionToken string
	Claimant       string
	// Access is the principal the work acts as: the Job's actor when it recorded
	// one, otherwise its owner. A zero UserID means the Job has no acting
	// identity — the host itself is running the work.
	Access Access
	// Input is the decoded, migrated replay input the Job was accepted with, or
	// nil for work that stores none.
	Input json.RawMessage

	report ExecutionReport
}

// ExecutionReport is the write seam a running execution publishes through. Every
// method is bound to one Job and one execution token, so an adapter cannot aim a
// publish at another Job even if it names one.
type ExecutionReport interface {
	Progress(Progress) (Snapshot, error)
	Event(EventInput) error
	Output(OutputInput) (Output, error)
	Transition(Transition) (Snapshot, error)
	Finish(FinishRequest) (Snapshot, error)
}

// reportOr returns the report this execution publishes through, or a refusal
// when it has none — a zero Execution is not a way to write unattributed facts.
func (e Execution) reportOr() (ExecutionReport, error) {
	if e.report == nil {
		return nil, fmt.Errorf("%w: execution %s has no report", ErrInvalidExecution, e.JobID)
	}
	return e.report, nil
}

// Progress replaces the execution's bounded progress snapshot.
func (e Execution) Progress(progress Progress) (Snapshot, error) {
	report, err := e.reportOr()
	if err != nil {
		return Snapshot{}, err
	}
	return report.Progress(progress)
}

// Event appends one significant event to the execution's Job.
func (e Execution) Event(event EventInput) error {
	report, err := e.reportOr()
	if err != nil {
		return err
	}
	return report.Event(event)
}

// Output publishes one typed output on the execution's Job.
func (e Execution) Output(output OutputInput) (Output, error) {
	report, err := e.reportOr()
	if err != nil {
		return Output{}, err
	}
	return report.Output(output)
}

// Transition applies one lifecycle transition under the execution's own token.
func (e Execution) Transition(transition Transition) (Snapshot, error) {
	report, err := e.reportOr()
	if err != nil {
		return Snapshot{}, err
	}
	return report.Transition(transition)
}

// Finish ends the execution with a terminal outcome.
func (e Execution) Finish(request FinishRequest) (Snapshot, error) {
	report, err := e.reportOr()
	if err != nil {
		return Snapshot{}, err
	}
	return report.Finish(request)
}

// ReconcileDecision is what a Kind adapter answers when a claim's lease expired
// and the control plane asks what should happen to the Job.
//
// It exists because only the Kind knows whether the external work the claim
// started is still running: expiry permits reconciliation, it never proves that
// execution stopped. The Service applies the answer; it never chooses one
// itself.
type ReconcileDecision string

const (
	// ReconcileResume keeps the Job running under a fresh token owned by the
	// reconciling runtime, which dispatches it again. Capacity is kept, because
	// the execution is continuing rather than being re-admitted.
	ReconcileResume ReconcileDecision = "resume"
	// ReconcileQueue returns the Job to the queue and frees its claim and
	// capacity, so normal dispatch runs it again.
	ReconcileQueue ReconcileDecision = "queue"
	// ReconcileSucceed ends the Job successfully, verifying the required outputs
	// exactly as an executor's own Finish does.
	ReconcileSucceed ReconcileDecision = "succeed"
	// ReconcileFail ends the Job unsuccessfully with a bounded reconciliation
	// failure.
	ReconcileFail ReconcileDecision = "fail"
	// ReconcileBlock blocks the Job and frees its claim and capacity: the adapter
	// knows the execution is not running and is not safe to rerun.
	ReconcileBlock ReconcileDecision = "block"
	// ReconcileInterrupt ends the Job as interrupted: the execution ended
	// unexpectedly and continuing it would require a new Job.
	ReconcileInterrupt ReconcileDecision = "interrupt"
	// ReconcileRemainRunning leaves the Job and its token exactly as they are and
	// extends the lease: the adapter proved the owning runtime is still working,
	// so nobody else may take the Job over.
	ReconcileRemainRunning ReconcileDecision = "remain-running"
	// ReconcileExternalWorkUnproven is the answer for an adapter that cannot
	// prove the external work the expired claim started has stopped. The Job
	// becomes blocked but keeps its claim, its lease and its capacity, and it
	// leaves the expiry scan, so no replacement is ever dispatched over work that
	// may still be running. This is failure-safe, not failure-free: the Job stays
	// blocked until an operator resolves it.
	ReconcileExternalWorkUnproven ReconcileDecision = "blocked-external-work-unproven"
)

// ReconcileDecisions lists the vocabulary. A decision outside it is refused
// rather than guessed at.
var ReconcileDecisions = []ReconcileDecision{
	ReconcileResume, ReconcileQueue, ReconcileSucceed, ReconcileFail,
	ReconcileBlock, ReconcileInterrupt, ReconcileRemainRunning,
	ReconcileExternalWorkUnproven,
}

// ReconcileRequest is what an adapter is told about a claim whose lease expired:
// the Job as the Service read it, who held it, when its lease ran out, the input
// it was running with when that could still be opened, and the same report seam
// a live execution gets — still under the expired claim's token, because until
// the decision is applied that token is still what owns the Job.
type ReconcileRequest struct {
	Snapshot       Snapshot
	Claimant       string
	LeaseExpiredAt time.Time
	// Input is the decoded replay input the expired execution ran with, or nil
	// when it cannot be produced (a missing key, a purged envelope, work that
	// stores none). An adapter that needs it must not guess.
	Input     json.RawMessage
	Access    Access
	Execution Execution
}

// ReconcileReport is what one reconciliation pass did.
type ReconcileReport struct {
	// Examined is how many expired claims the pass looked at.
	Examined int
	// Deferred is how many of them the pass could not decide: an adapter that
	// did not answer, or answered outside the vocabulary. Nothing was applied on
	// their behalf, they keep their claim and the capacity that goes with it, and
	// they are scheduled for a later pass rather than for the next one — which is
	// what keeps a Kind whose reconciler is failing from occupying every batch.
	Deferred int
	// Outcomes records the applied decisions, in the order they were applied.
	Outcomes []ReconcileOutcome
	// Resume holds the executions the caller must dispatch: a resume keeps the
	// Job running under a fresh token, and only the runtime can run it.
	Resume []Execution
}

// ReconcileOutcome records one applied decision.
type ReconcileOutcome struct {
	JobID    string
	Decision ReconcileDecision
	Snapshot Snapshot
}

// ArtifactRef names one artifact a Job published, by the output key it was
// published under and the bounded reference its publisher wrote. The control
// plane never interprets the reference; it hands it back to the Kind that wrote
// it.
type ArtifactRef struct {
	Key       string
	Reference json.RawMessage
}

// ArtifactCleanupRequest asks a Kind's adapter to remove — or to confirm the
// absence of — the artifacts one expired Job published, before the history that
// points at them is pruned.
//
// It exists because "available" and "removed" are two different facts: a sweep
// marking an output removed says what the database believes, and only the Kind
// knows whether the bytes an artifact reference names are still on a disk, in a
// bucket, or held open by something else. §9 requires the removal to be
// established before the reference to it goes.
type ArtifactCleanupRequest struct {
	JobID       string
	Kind        string
	KindVersion uint
	// Artifacts is every artifact output the Job published.
	Artifacts []ArtifactRef
}

// ArtifactCleanupResult is the adapter's acknowledgement of one cleanup.
//
// A result that does not account for every artifact is a refusal: the reference
// stays, and with it the Job, so an operator can still find what was left
// behind. An already-missing artifact is reported as removed — §7 treats it as
// removed rather than as an error.
type ArtifactCleanupResult struct {
	// Removed lists the keys whose artifact is confirmed gone.
	Removed []string
	// Retained lists the keys whose artifact is still there: still in use, held
	// by an external service, or refused by policy.
	Retained []string
}

// Command is one control a Job currently offers. It is advertised by the Job
// rather than inferred by a client from its state or Kind, and it names the
// version it was computed from so a stale call can be refused.
//
// The command surface is implemented on top of this seam; an adapter answers
// which commands its Jobs offer and runs the ones the host invokes.
type Command struct {
	Key          string
	Label        string
	Endpoint     string
	JobVersion   uint64
	Destructive  bool
	Bulk         bool
	Confirmation string
	Presentation json.RawMessage
}

// CommandContext is what an adapter is told when the host asks which commands a
// Job offers.
type CommandContext struct {
	Snapshot Snapshot
	Access   Access
}

// CommandExecution is one command the host is running against a Job an adapter
// owns.
type CommandExecution struct {
	JobID           string
	Key             string
	IdempotencyKey  string
	ExpectedVersion uint64
	Snapshot        Snapshot
	Access          Access
}

// CommandOutcome is what an adapter reports about one command it ran.
type CommandOutcome struct {
	Status  string
	Message string
	Detail  json.RawMessage
}
