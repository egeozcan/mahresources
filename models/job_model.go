package models

import (
	"errors"
	"fmt"
	"time"

	"mahresources/models/types"

	"gorm.io/gorm"
)

// This file holds the relational core of the Job control plane: the durable Job
// record, its immutable events, the single-row delivery-sequence allocator, the
// typed lineage links, the typed outputs a Job publishes, and the writer-epoch
// row that fences mixed-version writers against one database.
//
// The vocabulary — which states exist, which transitions are legal, what a
// visibility class means — belongs to the jobs/ package, which is the only
// writer. These types are the storage shape: strings and columns, no policy.

// Job is one accepted execution of background work.
//
// Identity is a UUIDv7 string: opaque, globally unique, time-ordered, and
// carrying no kind, owner, state or authorization decision. Terminal state and
// identity are immutable; a retry creates a new Job rather than reopening one.
//
// Owner, Actor and Origin are three separate provenance facts, not one field.
// Owner is the principal whose Job Center lists the Job, Actor is the principal
// whose current authority execution or a command uses, and Origin is what
// initiated it. They are historical: deleting a user nulls the live references
// through nullCreatorReferences without transferring authority. VisibilityClass
// is durable and is fixed by the registered Kind — request input cannot choose
// it — so an admin-only Kind (plugin commands, for instance) stays admin-only
// even when its submitter is an ordinary user, and stays admin-only after that
// submitter is demoted or deleted.
//
// The terminal outcome lives here, on the Job, and is never derived from a
// viewer's preferences: dismissal and pinning record a per-user view of the
// list, they do not rewrite what happened.
type Job struct {
	// ID carries the visible-pagination and administrator-ordering indexes: both
	// are keyset listings that page on (accepted_at, id), and a Job's accepted_at
	// is not unique, so the identity is the tiebreak the index must hold for the
	// page boundary to be walked rather than filtered.
	ID string `gorm:"primaryKey;size:36;index:idx_jobs_visible,priority:5;index:idx_jobs_admin_order,priority:2" json:"id"`

	// Kind and KindVersion name the family of work and the version of its
	// input semantics. A Kind adapter is registered per (Kind, KindVersion)
	// pair; no two share a pair.
	Kind        string `gorm:"size:120;not null;index:idx_jobs_kind_state,priority:1" json:"kind"`
	KindVersion uint   `gorm:"not null" json:"kindVersion"`

	// State is the normalized lifecycle state; Phase is an optional finer step
	// a Kind publishes ("downloading segments", "quarantined") and never
	// redefines State.
	State string `gorm:"size:20;not null;index:idx_jobs_kind_state,priority:2;index:idx_jobs_visible,priority:3;index:idx_jobs_retention,priority:1" json:"state"`
	Phase string `gorm:"size:60" json:"phase,omitempty"`

	// Title and Summary are the bounded, sanitized, searchable half of the
	// input. The opaque half — the replay envelope — is stored separately and
	// never returned by ordinary Job reads.
	Title   string     `gorm:"size:200" json:"title,omitempty"`
	Summary types.JSON `gorm:"type:json" json:"summary,omitempty"`

	OwnerUserID *uint `gorm:"index:idx_jobs_visible,priority:2" json:"ownerUserId,omitempty"`
	ActorUserID *uint `gorm:"index:idx_jobs_actor" json:"actorUserId,omitempty"`

	// Origin names what initiated the Job: ui, api, cli, plugin, schedule or
	// system.
	Origin string `gorm:"size:40;not null;index:idx_jobs_origin" json:"origin"`

	// VisibilityClass is "owner" or "admin". See the type comment.
	VisibilityClass string `gorm:"size:10;not null;index:idx_jobs_visible,priority:1" json:"visibilityClass"`

	// ExecutionPrincipal records which principal the Job's execution acts as:
	// "actor", "owner" or "host". It is durable because the user references
	// beside it are nulled when an account is deleted, and the class is what
	// distinguishes "an actor was recorded and is gone" — which blocks the Job
	// — from "this work intentionally has no actor" — which runs as the host.
	ExecutionPrincipal string `gorm:"size:10;not null" json:"executionPrincipal"`

	// ReplayClass records whether acceptance carried a replayable input
	// ("replayable") or an explicit non-replayable classification
	// ("non-replayable"). It is the durable half of "a replay envelope or an
	// explicit non-replayable classification"; the envelope itself is stored
	// beside it.
	ReplayClass string `gorm:"size:20;not null" json:"replayClass"`

	// ExecutionToken is the fencing token of the claim that currently owns the
	// Job. An executor may publish only with the token its claim was created
	// with. Empty means no claim owns the Job.
	ExecutionToken string `gorm:"size:36" json:"-"`

	// ControlIntent is durable control intent that has been requested but not
	// yet reached its outcome state: "" or "cancel" or "pause".
	ControlIntent      string     `gorm:"size:20" json:"controlIntent,omitempty"`
	ControlRequestedAt *time.Time `json:"controlRequestedAt,omitempty"`

	// Version is the optimistic concurrency counter. Every accepted transition
	// increments it; every write is guarded by the version the writer read.
	Version uint64 `gorm:"not null" json:"version"`

	// Failure is set when State is "failed" and empty otherwise.
	FailureCode          string `gorm:"size:60" json:"failureCode,omitempty"`
	FailureClass         string `gorm:"size:30" json:"failureClass,omitempty"`
	FailureMessage       string `gorm:"size:1000" json:"failureMessage,omitempty"`
	FailureDiagnosticRef string `gorm:"size:500" json:"failureDiagnosticRef,omitempty"`

	// Progress is the latest bounded snapshot. Routine ticks update these
	// columns; they are not events.
	ProgressCompleted *int64     `json:"progressCompleted,omitempty"`
	ProgressTotal     *int64     `json:"progressTotal,omitempty"`
	ProgressUnit      string     `gorm:"size:20" json:"progressUnit,omitempty"`
	ProgressMessage   string     `gorm:"size:500" json:"progressMessage,omitempty"`
	ProgressETA       *time.Time `json:"progressEta,omitempty"`

	// AcceptedAt is the acceptance instant, and the keyset column every visible
	// listing pages on. The rest are the common UTC instants; StateEnteredAt is
	// the bookkeeping behind the cumulative durations below and is deliberately
	// not part of a public snapshot.
	AcceptedAt     time.Time  `gorm:"not null;index:idx_jobs_visible,priority:4;index:idx_jobs_admin_order,priority:1" json:"acceptedAt"`
	ScheduledFor   *time.Time `json:"scheduledFor,omitempty"`
	QueuedAt       *time.Time `json:"queuedAt,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	LastResumedAt  *time.Time `json:"lastResumedAt,omitempty"`
	FinishedAt     *time.Time `gorm:"index:idx_jobs_retention,priority:2" json:"finishedAt,omitempty"`
	StateEnteredAt *time.Time `json:"-"`

	// Cumulative time spent in each state, accumulated from the transitions
	// rather than recomputed from the timestamps, so a Job that returns to the
	// queue several times measures all of them.
	RunningDuration time.Duration `gorm:"not null;default:0" json:"runningDuration"`
	PausedDuration  time.Duration `gorm:"not null;default:0" json:"pausedDuration"`
	BlockedDuration time.Duration `gorm:"not null;default:0" json:"blockedDuration"`
	QueueDuration   time.Duration `gorm:"not null;default:0" json:"queueDuration"`

	// ExpiresAt is the instant ordinary history retention may sweep this Job.
	// It is only ever set from a terminal state, because nonterminal work —
	// including blocked work — is never swept.
	ExpiresAt *time.Time `gorm:"index:idx_jobs_expiry" json:"expiresAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (j Job) GetId() string {
	return j.ID
}

// JobEvent is one immutable significant fact in a Job's timeline. Routine
// progress ticks are not events.
//
// Sequence is the per-Job ordering and JobVersion is the Job version the fact
// belongs to; together with JobID they are unique, so a timeline can never
// contain two events claiming the same position.
//
// DeliverySequence is the global, commit-safe ordering of the resumable stream,
// and it is deliberately nullable: an event is committed by the lifecycle
// transaction that recorded it and only afterwards assigned a delivery sequence
// by the publisher, on its own transaction. Allocating it inside the domain
// transaction would order events by allocation rather than by commit, and
// PostgreSQL sequences are not commit-ordered — an event allocated first but
// committed last would take a lower delivery sequence than one a subscriber has
// already consumed, and that subscriber would never see it.
type JobEvent struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	JobID      string     `gorm:"size:36;not null;uniqueIndex:idx_job_events_timeline,priority:1" json:"jobId"`
	Sequence   uint64     `gorm:"not null;uniqueIndex:idx_job_events_timeline,priority:2" json:"sequence"`
	JobVersion uint64     `gorm:"not null" json:"jobVersion"`
	Type       string     `gorm:"size:60;not null" json:"type"`
	Detail     types.JSON `gorm:"type:json" json:"detail,omitempty"`

	// ReservedHost marks an event whose capacity cannot be displaced by
	// optional Kind traffic: lifecycle and terminal events.
	ReservedHost bool `gorm:"not null;default:false" json:"reservedHost"`

	CreatedAt time.Time `gorm:"not null" json:"createdAt"`

	// DeliverySequence is nullable until the post-commit publisher assigns it.
	// The index serves both scans: the publication scan for
	// `delivery_sequence IS NULL` and a subscriber's cursor
	// `delivery_sequence > cursor`.
	DeliverySequence *uint64 `gorm:"index:idx_job_events_delivery" json:"deliverySequence,omitempty"`
}

// JobEventSequence is the single serialized allocator row for delivery
// sequences. It is touched only by the post-commit publisher, never inside a
// domain or lifecycle transaction, so the allocator adds no cross-domain lock
// ordering.
type JobEventSequence struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Value     uint64    `gorm:"not null" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// JobEventSequenceRowID is the allocator's primary key. One row, always.
const JobEventSequenceRowID = 1

// Typed lineage link kinds. A Job keeps its own identity and outcome inside a
// lineage; the link only records how two Jobs relate.
const (
	// JobLinkRetryOf relates a successor to the unsuccessful Job it recovers
	// from: FromJobID is the successor, ToJobID the ancestor.
	JobLinkRetryOf = "retry-of"
	// JobLinkRepeatOf relates a successor to the successful Job it re-runs:
	// FromJobID is the successor, ToJobID the ancestor.
	JobLinkRepeatOf = "repeat-of"
	// JobLinkParentChild relates independent workflow stages: FromJobID is the
	// parent, ToJobID the child.
	JobLinkParentChild = "parent-child"
)

// JobLink is one typed durable relation between two Jobs, unique on
// (type, from, to) so the same relation cannot be recorded twice.
type JobLink struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Type      string    `gorm:"size:20;not null;uniqueIndex:idx_job_links_unique,priority:1" json:"type"`
	FromJobID string    `gorm:"size:36;not null;uniqueIndex:idx_job_links_unique,priority:2;index:idx_job_links_from" json:"fromJobId"`
	ToJobID   string    `gorm:"size:36;not null;uniqueIndex:idx_job_links_unique,priority:3;index:idx_job_links_to" json:"toJobId"`
	CreatedAt time.Time `json:"createdAt"`
}

// JobOutput is one typed result a Job published: an entity link, a downloadable
// artifact with an expiry, a report or detail link, a structured summary, an
// explicitly safe external link, or a verbose-log reference.
//
// Availability is independent of the Job's outcome and of the other outputs, so
// an artifact nobody can fetch any more never rewrites the success that produced
// it, and expiry is recorded on the output rather than inferred from the Job.
// Version is the output's own optimistic counter, separate from the Job's: an
// at-least-once executor republishing the same key after a crash replaces one
// row rather than consuming the lifecycle version.
//
// Key is unique per Job, which is what makes a republish a replacement. The
// reference is bounded JSON this module never interprets: which shape is safe is
// the Kind adapter's decision, and the Service only guarantees that it is
// bounded, valid, and never a filesystem path a reader could act on directly.
type JobOutput struct {
	ID    string `gorm:"primaryKey;size:36" json:"id"`
	JobID string `gorm:"size:36;not null;uniqueIndex:idx_job_outputs_key,priority:1;index:idx_job_outputs_job" json:"jobId"`
	Key   string `gorm:"size:120;not null;uniqueIndex:idx_job_outputs_key,priority:2" json:"key"`

	Type      string     `gorm:"size:40;not null" json:"type"`
	Label     string     `gorm:"size:200" json:"label,omitempty"`
	Reference types.JSON `gorm:"type:json" json:"reference,omitempty"`
	Required  bool       `gorm:"not null;default:false" json:"required"`

	// Availability is available, expired or removed. ExpiresAt is the planned
	// instant that becomes true at, and RemovedAt the instant a sweep confirmed
	// the artifact is gone — an already-missing artifact is removed rather than
	// an error.
	Availability string     `gorm:"size:20;not null" json:"availability"`
	ExpiresAt    *time.Time `gorm:"index:idx_job_outputs_expiry" json:"expiresAt,omitempty"`
	RemovedAt    *time.Time `json:"removedAt,omitempty"`

	// NextCleanupAt is when a sweep may next ask the Kind that published an
	// artifact to remove it, after a pass could not establish that its bytes are
	// gone — a Job an unresolved claim protects, or a Kind this process cannot
	// run. Deferral is what keeps a candidate nothing can act on from holding the
	// head of every batch; a publication of the same key is a new artifact and
	// clears it.
	NextCleanupAt *time.Time `json:"nextCleanupAt,omitempty"`

	Version   uint64    `gorm:"not null;default:1" json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// JobClaim is the durable claim one execution holds on one Job: who owns it,
// under which fencing token, and until when.
//
// It is a row rather than more columns on the Job because it is the thing a
// runtime heartbeats and a reconciler scans: the lease and the last heartbeat
// are claim state, not Job state, and a released claim stays as the evidence of
// the execution that held it.
//
// JobID is the primary key — one Job has at most one claim — and the row is
// written in the same transaction as the Job's move to running, so there is no
// instant in which a Job is running without the claim that admits it. Releasing
// stamps ReleasedAt and clears the Job's execution token; a Job claimed again
// after reconciliation takes the same row over under a fresh token.
//
// State distinguishes the three ways a claim can stand: held (an execution owns
// the Job), quarantined (a lease expired and nobody could prove the external
// work stopped, so the claim and its capacity are kept, and the claim leaves the
// expiry scan so it is not reconciled in a loop), and released.
type JobClaim struct {
	JobID string `gorm:"primaryKey;size:36" json:"jobId"`

	Kind        string `gorm:"size:120;not null" json:"kind"`
	KindVersion uint   `gorm:"not null" json:"kindVersion"`

	// Claimant identifies the runtime that owns the execution: a process, or a
	// named worker inside one. It is evidence, not authority — the token is what
	// fences publication.
	Claimant       string `gorm:"size:120;not null" json:"claimant"`
	ExecutionToken string `gorm:"size:36;not null" json:"executionToken"`

	State string `gorm:"size:20;not null;index:idx_job_claims_expiry,priority:1" json:"state"`

	ClaimedAt   time.Time `gorm:"not null" json:"claimedAt"`
	HeartbeatAt time.Time `gorm:"not null" json:"heartbeatAt"`
	// LeaseExpiresAt is the instant the claim stops being trusted without a
	// heartbeat. Reaching it permits reconciliation; it never proves the external
	// work stopped.
	LeaseExpiresAt time.Time `gorm:"not null;index:idx_job_claims_expiry,priority:2" json:"leaseExpiresAt"`

	// ReconcileAttempts counts the reconciliation passes that asked about this
	// claim and applied nothing, and NextReconcileAt is the instant it may be
	// asked about again. They are a schedule, not a state: neither one changes
	// what the claim protects, so an undecided claim keeps its token, its lease
	// and the capacity it holds. A claim whose lease a heartbeat extended simply
	// leaves the scan until that lease lapses, and comes back with this instant
	// already behind it.
	ReconcileAttempts uint       `gorm:"not null;default:0" json:"reconcileAttempts"`
	NextReconcileAt   *time.Time `gorm:"index:idx_job_claims_reconcile" json:"nextReconcileAt,omitempty"`

	ReleasedAt    *time.Time `json:"releasedAt,omitempty"`
	ReleaseReason string     `gorm:"size:40" json:"releaseReason,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// The three ways a claim can stand. They are the stored spellings, so a scan and
// a writer cannot disagree about one of them.
const (
	// JobClaimStateHeld is a claim an execution owns.
	JobClaimStateHeld = "held"
	// JobClaimStateQuarantined is a claim whose lease expired in a way no
	// reconciler could resolve: it is kept, with its capacity, and is no longer
	// reconciled until an operator or a command resolves it.
	JobClaimStateQuarantined = "quarantined"
	// JobClaimStateReleased is a claim whose execution ended.
	JobClaimStateReleased = "released"
)

// JobCapacityLease occupies one numbered slot in one capacity budget.
//
// Capacity is occupancy rather than history: a row exists exactly while its
// execution holds the slot, and a released slot is deleted so it can be taken
// again. The unique index on (capacity_group, slot) is what makes admission
// race-free — two processes taking the last slot of a budget contend on the same
// index entry, and the loser falls through to a higher slot or to
// "budget full" — without a counter that could drift from the claims it is
// supposed to describe.
//
// A claim occupies one slot per budget it draws on, which is how a Kind's own
// budget and the deployment-wide budget are both enforced.
type JobCapacityLease struct {
	ID            string `gorm:"primaryKey;size:36" json:"id"`
	CapacityGroup string `gorm:"size:120;not null;uniqueIndex:idx_job_capacity_slot,priority:1;uniqueIndex:idx_job_capacity_job,priority:2" json:"capacityGroup"`
	Slot          int    `gorm:"not null;uniqueIndex:idx_job_capacity_slot,priority:2" json:"slot"`
	// A Job occupies at most one slot per budget, which is why (job_id,
	// capacity_group) is the unique pair rather than (job_id, token): one claim
	// draws on several budgets at once, and that is several rows.
	JobID          string    `gorm:"size:36;not null;uniqueIndex:idx_job_capacity_job,priority:1" json:"jobId"`
	ExecutionToken string    `gorm:"size:36;not null" json:"executionToken"`
	AcquiredAt     time.Time `gorm:"not null" json:"acquiredAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// JobReplayEnvelope is one Job's sealed replay input: the opaque half of what
// the Job was accepted with, kept apart from the bounded searchable summary so
// that no ordinary Job read, event, log line or JSON view can reach it.
//
// It is deliberately one row per Job, and that row is both the envelope and the
// durable purge marker. Purge never deletes the row: it clears the ciphertext
// and nonce and stamps PurgedAt/PurgeReason, so a reader can tell "this Job's
// input was forgotten" or "it expired" from "this Job never had any", and
// backfill can never resurrect input a purge removed (see the migration and
// plaintext-retirement tasks). An envelope that carries ciphertext therefore has
// no PurgedAt, and one with PurgedAt has none.
//
// Ciphertext is bound to the Job, Kind and Kind version by AES-256-GCM
// associated data, so a row moved to another Job — or read under another Kind's
// decoder — fails authentication rather than decrypting into the wrong
// execution. KeyID is a non-secret SHA-256 fingerprint of the key that sealed
// it: rotation reads old envelopes by their own key ID and always writes with
// the active one.
//
// ExpiresAt is replay retention, and it is stamped from terminal completion
// (finished_at) rather than acceptance. It stays NULL for every nonterminal
// state — scheduled, queued, running, paused and blocked work keeps
// execution-required input for as long as it needs it.
type JobReplayEnvelope struct {
	JobID string `gorm:"primaryKey;size:36" json:"jobId"`
	// Kind and KindVersion are the input semantics the sealed bytes were
	// written for, and part of what the ciphertext authenticates against.
	Kind        string `gorm:"size:120;not null" json:"kind"`
	KindVersion uint   `gorm:"not null" json:"kindVersion"`
	// SchemaVersion is the envelope encoding this module wrote, not the Kind's
	// input version.
	SchemaVersion uint `gorm:"not null" json:"schemaVersion"`
	// KeyID names the key that sealed the ciphertext. It is an identifier, not a
	// secret: the key itself is never stored.
	KeyID string `gorm:"size:64;not null" json:"keyId"`

	Nonce      []byte `json:"-"`
	Ciphertext []byte `json:"-"`

	CreatedAt time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ExpiresAt *time.Time `gorm:"index:idx_job_replay_expiry" json:"expiresAt,omitempty"`

	PurgedAt    *time.Time `json:"purgedAt,omitempty"`
	PurgeReason string     `gorm:"size:20" json:"purgeReason,omitempty"`
}

// JobReplayEnvelopeTable is the envelope table's name, spelled once because the
// purge sweep and the migrations both address it.
const JobReplayEnvelopeTable = "job_replay_envelopes"

// Why a replay envelope was purged. The reason is durable because it is what a
// reader is told: "forgotten" is an operator or owner decision, "expired" is
// retention, and the two are not interchangeable evidence.
const (
	// JobReplayPurgeForgotten is an explicit Forget of finished work's input.
	JobReplayPurgeForgotten = "forgotten"
	// JobReplayPurgeExpired is replay retention reached from terminal completion.
	JobReplayPurgeExpired = "expired"
)

// JobPreference is one viewer's relationship to one Job: whether they dismissed
// it from their default list and whether they pinned it.
//
// It is deliberately not part of the Job row. Dismissal belongs to a viewer —
// it hides the Job from *their* default list and changes nothing about what
// happened — and pinning exempts the Job's metadata and events from automatic
// expiry for everybody, so it has to be answerable as "is anybody pinning this",
// which a column per viewer cannot be. Both are recorded as instants rather than
// booleans so the fact of when a viewer dismissed or pinned something survives;
// clearing one NULLs its instant.
//
// The primary key is the pair, which is what makes a preference a row rather
// than a list: setting one twice replaces it instead of accumulating. UserID is
// indexed on its own because the per-user pin limit counts one viewer's pins,
// which the (job_id, user_id) key cannot answer.
type JobPreference struct {
	JobID  string `gorm:"primaryKey;size:36" json:"jobId"`
	UserID uint   `gorm:"primaryKey;index:idx_job_preferences_user" json:"userId"`

	// DismissedAt is when this viewer hid the Job from their default list.
	DismissedAt *time.Time `json:"dismissedAt,omitempty"`
	// PinnedAt is when this viewer pinned the Job. A pinned Job is exempt from
	// ordinary metadata retention for as long as any viewer holds one.
	PinnedAt *time.Time `json:"pinnedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// JobLegacyHandleTable is the compatibility-handle table's name, spelled once so
// the migration, the resolver and the tests cannot disagree about it.
const JobLegacyHandleTable = "job_legacy_handles"

// JobLegacyHandle is the durable mapping from one legacy identifier to the
// canonical Job that is its current projection.
//
// A legacy `id` is a handle, not an identity: it names the current leaf of a
// linear Retry lineage, so successive retries through one unchanged legacy id
// keep working while every execution keeps its own immutable UUID. The row is
// what makes that possible across a restart — an in-memory link would forget
// which execution a client's bookmark now means — and it is deliberately
// separate from the canonical identity, which never moves.
//
// The pair (namespace, handle) is the key because one numeric id space can be
// shared by several legacy surfaces (a download queue id and a plugin action job
// id are both short random strings).
type JobLegacyHandle struct {
	Namespace string `gorm:"primaryKey;size:40" json:"namespace"`
	Handle    string `gorm:"primaryKey;size:64" json:"handle"`

	// JobID is the canonical Job the handle currently projects. It is indexed
	// because the movement of a handle is read from the Job that supersedes it.
	JobID string `gorm:"size:36;not null;index:idx_job_legacy_handles_job" json:"jobId"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName pins the table name so the model and the migration agree.
func (JobLegacyHandle) TableName() string {
	return JobLegacyHandleTable
}

// JobCommandRequest is the durable idempotency record of one command a caller
// asked for: what it asked, who asked, the outcome it reached, and — for a Retry
// or a Repeat — the Job it created.
//
// It is a row rather than an in-memory guard because a command's effects are
// durable and a client that never saw the answer must be able to ask again: the
// unique tuple (job, command, actor, idempotency key) is what makes the repeat
// return the recorded outcome instead of running the executor a second time. The
// row is written before the effect is attempted and completed afterwards, so a
// process that dies mid-command leaves a claim that refuses a repeat rather than
// a second side effect.
//
// ActorUserID is a plain uint rather than the nullable reference the rest of the
// module uses, and 0 is the host principal — an administrator acting with no user
// of their own. A NULL column would make every host request a new tuple (NULLs
// compare equal to nothing), which is the one thing this table exists to prevent.
// The reference is deliberately not swept when the account is deleted: it is the
// identity the idempotency tuple is keyed on, so nulling it would let a later
// owner of the same numeric id replay somebody else's recorded outcome.
type JobCommandRequest struct {
	ID string `gorm:"primaryKey;size:36" json:"id"`

	JobID          string `gorm:"size:36;not null;uniqueIndex:idx_job_command_requests_idempotency,priority:1;index:idx_job_command_requests_job" json:"jobId"`
	CommandKey     string `gorm:"size:40;not null;uniqueIndex:idx_job_command_requests_idempotency,priority:2" json:"commandKey"`
	ActorUserID    uint   `gorm:"not null;uniqueIndex:idx_job_command_requests_idempotency,priority:3" json:"actorUserId"`
	IdempotencyKey string `gorm:"size:200;not null;uniqueIndex:idx_job_command_requests_idempotency,priority:4" json:"idempotencyKey"`
	// LegacyRequestKey aliases one keyed compatibility command independently of
	// JobID. Retry moves a legacy handle to its successor, but repeating the same
	// client request through that handle must still find the original outcome.
	// NULL leaves canonical requests on their original idempotency tuple.
	LegacyRequestKey *string `gorm:"size:64;uniqueIndex:idx_job_command_requests_legacy_request" json:"legacyRequestKey,omitempty"`

	// RequestHash is the fingerprint of what was asked for through this key, so a
	// key reused for a different request is refused rather than answered with the
	// other request's outcome.
	RequestHash string `gorm:"size:64;not null" json:"requestHash"`

	// Status is running, succeeded or failed. Code, Message, Detail and
	// SuccessorJobID are the recorded outcome a repeat is answered with.
	Status         string     `gorm:"size:20;not null" json:"status"`
	Code           string     `gorm:"size:40" json:"code,omitempty"`
	Message        string     `gorm:"size:1000" json:"message,omitempty"`
	Detail         types.JSON `gorm:"type:json" json:"detail,omitempty"`
	SuccessorJobID string     `gorm:"size:36" json:"successorJobId,omitempty"`

	CreatedAt   time.Time  `gorm:"not null" json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// The three ways a command request can stand. They are the stored spellings, so
// a repeat and the writer that recorded it cannot disagree about one of them.
const (
	// JobCommandStatusRunning is a command that has been claimed and whose effect
	// is being attempted. A repeat while it stands is refused rather than run
	// again: the first attempt may already have had an effect, and this table
	// cannot tell.
	JobCommandStatusRunning = "running"
	// JobCommandStatusSucceeded is a command whose effect is durable.
	JobCommandStatusSucceeded = "succeeded"
	// JobCommandStatusFailed is a command that was attempted and did not succeed.
	// The failure is recorded rather than erased, because a repeat must be
	// answered with it rather than with a second attempt.
	JobCommandStatusFailed = "failed"
)

// JobPinGuard is the per-viewer row preference admission serializes on, and the
// durable record of whether that viewer still exists to admit one.
//
// The per-user pin limit is a count of one viewer's committed pin rows, and a
// count is not itself a guard: two admissions that read it at the same time both
// see the limit unmet, because the rows they are about are different Jobs and
// nothing about them conflicts. So the count happens while this row — one per
// viewer, created on that viewer's first admission — is held, which is what makes
// the second admission see the first one's pin.
//
// DeletedAt is the tombstone: the instant the viewer's account was removed.
// Deleting an account marks this row — under the same lock every admission takes
// — and sweeps that viewer's preferences in the same transaction, so an
// admission that arrives afterwards is refused instead of writing a row whose
// viewer nobody can ask about. Without it an account deletion and a preference
// in flight could not be ordered against each other, and a pin that outlived its
// viewer would exempt the Job's metadata and events from retention for
// everybody, forever, with no one left who could unpin it. It is the same row as
// the fence rather than a second fact, because that is what leaves exactly one
// thing for the two operations to serialize on.
//
// It is a plain timestamp, not gorm.DeletedAt: the row is never hidden from a
// query — it is read to answer whether an admission may proceed.
type JobPinGuard struct {
	UserID    uint       `gorm:"primaryKey" json:"userId"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// JobWriterEpoch is the single row recording the minimum writer epoch this
// database accepts: the oldest release permitted to write to it.
//
// It exists because a release that changes the Job schema is unsupported once a
// newer release has advanced the epoch, and the failure mode of not knowing is
// silent: an older binary would keep writing rows the newer one cannot
// reconcile. Every process therefore checks this row immediately after opening
// the database and before it migrates, writes, cleans up or dispatches
// anything.
type JobWriterEpoch struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	MinimumEpoch uint64    `gorm:"not null" json:"minimumEpoch"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// TableName pins the table name the preflight's raw SQL reads. GORM's pluralizer
// would render it "job_writer_epoches", and the preflight runs before
// AutoMigrate, so a mismatch here is a check that silently passes.
func (JobWriterEpoch) TableName() string {
	return JobWriterEpochTable
}

// JobWriterEpochRowID is the epoch row's primary key. One row, always.
const JobWriterEpochRowID = 1

// JobWriterEpochSupported is the writer epoch this release understands. It is
// the Release-A epoch: the durable job core exists and every writer of these
// tables understands it. A later release may advance the stored minimum, never
// silently lower it.
const JobWriterEpochSupported uint64 = 1

// JobWriterEpochTable is the epoch table's name, spelled once because the
// preflight reads it with raw SQL before AutoMigrate has had a chance to run.
const JobWriterEpochTable = "job_writer_epochs"

// ErrJobWriterEpochTooNew reports that the database was last written by a
// release newer than this one. Startup must refuse rather than migrate, write,
// or dispatch.
var ErrJobWriterEpochTooNew = errors.New("job writer epoch is newer than this release supports")

// CheckJobWriterEpoch reads the writer epoch and refuses a database whose
// minimum epoch this release does not support.
//
// It is deliberately raw SQL, and deliberately tolerant of a missing table: it
// runs immediately after the database is opened, before AutoMigrate — so a
// database with no epoch table at all is a fresh (or pre-Job) database, which
// is allowed, while anything else about it is an error worth refusing over
// rather than guessing through.
func CheckJobWriterEpoch(db *gorm.DB) error {
	exists, err := jobCoreTableExists(db, JobWriterEpochTable)
	if err != nil {
		return fmt.Errorf("job writer epoch preflight: %w", err)
	}
	if !exists {
		return nil
	}

	var minimum uint64
	if err := db.Raw("SELECT minimum_epoch FROM job_writer_epochs WHERE id = 1").Scan(&minimum).Error; err != nil {
		return fmt.Errorf("job writer epoch preflight: %w", err)
	}
	if minimum > JobWriterEpochSupported {
		return fmt.Errorf("%w: this database requires writer epoch %d and this release supports %d; "+
			"a newer release has already run against it, so start that release instead of downgrading",
			ErrJobWriterEpochTooNew, minimum, JobWriterEpochSupported)
	}
	return nil
}

// EnsureJobWriterEpoch seeds the epoch row on a database that has just been
// migrated. An epoch that is already recorded is left exactly as it is: a
// release may advance the minimum epoch deliberately (after every older process
// has drained), and no code path may lower it by accident.
func EnsureJobWriterEpoch(db *gorm.DB) error {
	var existing JobWriterEpoch
	err := db.Where("id = ?", JobWriterEpochRowID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&JobWriterEpoch{ID: JobWriterEpochRowID, MinimumEpoch: JobWriterEpochSupported}).Error
	}
	if err != nil {
		return fmt.Errorf("job writer epoch seed: %w", err)
	}
	return nil
}

// jobCoreTableExists reports whether a table exists, with the two dialects'
// catalogues asked directly. It is used only by the preflight, which runs
// before the schema is guaranteed to exist.
func jobCoreTableExists(db *gorm.DB, name string) (bool, error) {
	var count int64
	var err error
	if db.Dialector.Name() == "postgres" {
		err = db.Raw(
			"SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?",
			name,
		).Scan(&count).Error
	} else {
		err = db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count).Error
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
