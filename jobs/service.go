package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	stored, _, err := s.accept(nil, deps, acceptance, nil)
	return stored, err
}

// AcceptClaimed is Accept for work whose executor already exists in the calling
// process: the Job is accepted *and* claimed in one transaction, and the caller
// is handed the execution that owns it.
//
// The window it closes is the reason it exists. A closure-backed
// `mah.start_job` owns a `*lua.LFunction` in the process that accepted it, and
// that Job is not waiting work: nothing but that one process can run it. Accepting
// it and then claiming it leaves an interval — the length of a dispatch loop's
// poll — in which the Job is ordinary queued work of a registered Kind, and a
// runtime that claims it there finds no callback, cannot run it, and must fail,
// block or withdraw a Job whose host was about to run it perfectly well.
//
// Accepting and claiming together removes the interval rather than arbitrating
// it: the Job is born running, fenced by its own token, occupying its budgets, and
// no other runtime ever sees it as waiting work. The caller owns the returned
// execution from that moment and must heartbeat it and end the Job — or release
// the claim — when its executor finishes.
//
// It is deliberately not the general path: restorable work is dispatched by being
// queued, which is what lets any runtime of the deployment run it.
func (s *Service) AcceptClaimed(ctx context.Context, deps Deps, acceptance Acceptance, request ClaimRequest) (Execution, Snapshot, error) {
	if acceptance.State != StateQueued {
		return Execution{}, Snapshot{}, fmt.Errorf("%w: a Job claimed at acceptance is accepted queued, not %q",
			ErrInvalidAcceptance, acceptance.State)
	}
	stored, execution, err := s.accept(ctx, deps, acceptance, &request)
	return execution, stored, err
}

// accept is the one acceptance body: it validates, seals the input, and writes
// the Job, its events, its handles and its envelope in one transaction — with the
// first claim installed inside the same transaction when the caller supplies one.
//
// ctx is used only to build the execution for an acceptance that carries a claim,
// and is nil for the ordinary path.
func (s *Service) accept(ctx context.Context, deps Deps, acceptance Acceptance, claim *ClaimRequest) (Snapshot, Execution, error) {
	if err := s.applyRegisteredVisibility(&acceptance); err != nil {
		return Snapshot{}, Execution{}, err
	}
	if err := validateAcceptance(&acceptance); err != nil {
		return Snapshot{}, Execution{}, err
	}

	// The claim's own admission is decided before the transaction opens, for the
	// reason the acceptance's validation is: a claim this Kind cannot honour — an
	// unregistered Kind, a claimant with no name — must refuse acceptance rather
	// than leave a Job whose executor does not exist.
	var lease time.Duration
	if claim != nil {
		if claim.Kind != acceptance.Kind || claim.KindVersion != acceptance.KindVersion {
			return Snapshot{}, Execution{}, fmt.Errorf("%w: a claim at acceptance names %s v%d, not %s v%d",
				ErrInvalidClaim, claim.Kind, claim.KindVersion, acceptance.Kind, acceptance.KindVersion)
		}
		_, definition, err := s.adapterFor(claim.Kind, claim.KindVersion)
		if err != nil {
			return Snapshot{}, Execution{}, err
		}
		if err := validateClaimRequest(claim, definition); err != nil {
			return Snapshot{}, Execution{}, err
		}
		lease = claim.Lease
		if lease <= 0 {
			lease = definition.claimLease()
		}
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
		// The class the execution acts as is durable from here, because the live
		// references beside it are cleared when an account is deleted.
		ExecutionPrincipal: string(principalClassOf(acceptance)),
		ReplayClass:        string(replayClassOf(acceptance.Replay)),
		Version:            1,
		AcceptedAt:         now,
		ScheduledFor:       utcPtr(acceptance.ScheduledFor),
		StateEnteredAt:     &now,
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
			return Snapshot{}, Execution{}, err
		}
		envelope = &sealed
		job.Summary = types.JSON(summary)
	}

	var (
		stored     Snapshot
		claimedJob models.Job
		claimRow   models.JobClaim
	)
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return fmt.Errorf("jobs: store job: %w", err)
		}
		claimed := job
		if claim != nil {
			// Born running, under this process's own token. The write is guarded the way
			// every claim's is — on the state and version just written — so a second
			// executor cannot take the same Job by writing first.
			token := types.NewUUIDv7()
			next, updates := applyTransition(job, Transition{To: StateRunning}, deps.retention(), now)
			updates["execution_token"] = token
			result := tx.Model(&models.Job{}).
				Where("id = ? AND version = ? AND state = ?", job.ID, job.Version, job.State).
				Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("jobs: claim the accepted job: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return errClaimContended
			}
			if err := acquireCapacityTx(tx, claim.Capacity, job.ID, token, now); err != nil {
				return err
			}
			claimRow = models.JobClaim{
				JobID:          job.ID,
				Kind:           job.Kind,
				KindVersion:    job.KindVersion,
				Claimant:       claim.Claimant,
				ExecutionToken: token,
				State:          models.JobClaimStateHeld,
				ClaimedAt:      now,
				HeartbeatAt:    now,
				LeaseExpiresAt: now.Add(lease),
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			// Stated as a claim insert rather than a create: a row already stored under
			// this Job id would be a claim this acceptance does not own, and the
			// transaction rolls back rather than writing a second one over it.
			result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claimRow)
			if result.Error != nil {
				return fmt.Errorf("jobs: store the acceptance claim: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return errClaimContended
			}
			claimed = next
		}
		// The legacy identifiers are written in the same transaction as the Job
		// they name: a handle is the identity a legacy client keeps polling, and
		// one stored by a second write could name a Job the rollback removed.
		if err := storeLegacyHandles(tx, job.ID, acceptance.LegacyRefs, now); err != nil {
			return err
		}
		// The lineage is written with the Job it describes. linkLineage takes the
		// endpoints' rows — and, on an engine with row locks, holds them for the rest of
		// this transaction — so a parent another transaction is inside, or a parent that
		// is gone, is answered here rather than assumed from a read taken before it.
		for _, parent := range acceptance.Parents {
			if err := linkLineage(tx, LinkRequest{
				Type: LinkParentChild, FromJobID: parent, ToJobID: job.ID,
			}, now); err != nil {
				return err
			}
		}
		event := newEvent(job.ID, 1, job.Version, EventAccepted, nil, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store accepted event: %w", err)
		}
		if claim != nil {
			// The claim's own event, in the same transaction as the claim: an accepted
			// Job that is running has to arrive with both facts or with neither.
			sequence, err := nextEventSequence(tx, job.ID)
			if err != nil {
				return err
			}
			started := newEvent(job.ID, sequence, claimed.Version, EventStarted, nil, true, now)
			if err := tx.Create(&started).Error; err != nil {
				return fmt.Errorf("jobs: store started event: %w", err)
			}
		}
		if envelope != nil {
			if err := tx.Create(envelope).Error; err != nil {
				return fmt.Errorf("jobs: store replay envelope: %w", err)
			}
		}
		claimedJob = claimed
		stored = snapshot(claimed)
		return nil
	})
	if err != nil {
		return Snapshot{}, Execution{}, err
	}
	// Filled after commit rather than inside it: the envelope's availability is
	// a read of the row that has just been written, and answering it on the
	// caller's own handle keeps the transaction's statements to writes. The
	// projection is the stored row whole: an adapter is the producer of this
	// acceptance rather than a viewer of it, and a Job that has just been
	// accepted cannot carry a protected failure diagnostic.
	stored.ReplayAvailability = s.replayAvailabilityOf(deps.DB, replayKeys(deps), claimedJob, deps.now())
	if claim == nil {
		return stored, Execution{}, nil
	}
	// A claim taken at acceptance takes work that was never waiting: the caller
	// already owns the executor, and its Job was born running. The state it
	// claimed from is therefore the queued state it was accepted in.
	execution, err := s.executionFor(ctx, deps, claimedJob, claimRow, StateQueued, claimFromWaiting)
	if err != nil {
		return stored, Execution{}, err
	}
	return stored, execution, nil
}

// Get returns one visible Job. A Job the caller may not see is reported exactly
// as a Job that does not exist, so probing cannot distinguish the two.
func (s *Service) Get(deps Deps, access Access, jobID string) (Snapshot, error) {
	job, err := loadVisibleJob(deps.DB, access, jobID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshots := []Snapshot{s.snapshotFor(deps, access, job)}
	if err := fillViewerPinState(deps.DB, access, snapshots); err != nil {
		return Snapshot{}, err
	}
	return snapshots[0], nil
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
func (s *Service) snapshotFor(deps Deps, access Access, job models.Job) Snapshot {
	snap := viewerSnapshot(job, access)
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
	switch a.ExecutionPrincipal {
	case "":
		a.ExecutionPrincipal = principalClassOf(*a)
	case PrincipalActor:
		if a.ActorUserID == nil {
			return invalid("an execution that acts as its actor must name one")
		}
	case PrincipalOwner:
		if a.OwnerUserID == nil {
			return invalid("an execution that acts as its owner must name one")
		}
	case PrincipalHost:
	default:
		return invalid("unknown execution principal %q", a.ExecutionPrincipal)
	}
	if len(a.Replay.Input) > 0 && a.Replay.NonReplayable {
		return invalid("input cannot be supplied for work that declares non-replayable input")
	}
	if len(a.Replay.Input) == 0 && !a.Replay.NonReplayable {
		// §3: acceptance persists a replay envelope *or* an explicit
		// non-replayable classification. The class defaults to replayable, so
		// accepting without either stored a Job that promised replayable input
		// and had none — unreplayable by construction and, once dispatch refused
		// to run it, blocked for a reason nobody chose. Work with no input of its
		// own passes an explicit empty payload (JSON null) if it is genuinely
		// replayable.
		return invalid("replayable work must be accepted with its input; supply it or declare the work non-replayable")
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
	parentSeen := make(map[string]struct{}, len(a.Parents))
	for _, parent := range a.Parents {
		if strings.TrimSpace(parent) == "" {
			return invalid("a parent link needs a job id")
		}
		if _, duplicate := parentSeen[parent]; duplicate {
			return invalid("parent %s is repeated", parent)
		}
		parentSeen[parent] = struct{}{}
	}
	return nil
}

// applyRegisteredVisibility fixes the visibility class an acceptance is stored
// with from the Kind's registered definition.
//
// The class is a durable property of the work, and it is what the one shared
// visibility predicate reads — so it cannot be request input. Deriving it here
// is what makes "an admin-only Kind cannot produce owner-visible Jobs" true by
// construction rather than by every adapter remembering to pass the field, and
// an acceptance that contradicts its own Kind's declaration is refused rather
// than believed.
//
// A Kind this process has no adapter for is left exactly as it was declared:
// nothing here can run the work, so nothing here can fix its class either, and
// dispatch refuses it on that same ground.
func (s *Service) applyRegisteredVisibility(acceptance *Acceptance) error {
	_, definition, err := s.adapterFor(acceptance.Kind, acceptance.KindVersion)
	if err != nil {
		return nil
	}
	if acceptance.Visibility != "" && acceptance.Visibility != definition.Visibility {
		return fmt.Errorf("%w: %s v%d is registered %s-visible, and the acceptance names %s",
			ErrInvalidAcceptance, acceptance.Kind, acceptance.KindVersion, definition.Visibility, acceptance.Visibility)
	}
	acceptance.Visibility = definition.Visibility
	return nil
}

// principalClassOf is which principal an acceptance's execution acts as: what it
// declared, or — when it declared nothing — the actor it recorded, else the owner
// it recorded, else the host.
//
// The order is the one dispatch used to apply at run time, moved to acceptance
// where the references still exist. Deciding it later is what let a deleted actor
// silently become the owner, and a deleted owner silently become the host.
func principalClassOf(a Acceptance) PrincipalClass {
	if a.ExecutionPrincipal != "" {
		return a.ExecutionPrincipal
	}
	switch {
	case a.ActorUserID != nil:
		return PrincipalActor
	case a.OwnerUserID != nil:
		return PrincipalOwner
	default:
		return PrincipalHost
	}
}

// executionPrincipalOf reads the class of a stored row, deriving it for rows
// written before the column existed: those carry the same references the
// derivation reads, and they were accepted under the same rule.
func executionPrincipalOf(job models.Job) PrincipalClass {
	if job.ExecutionPrincipal != "" {
		return PrincipalClass(job.ExecutionPrincipal)
	}
	switch {
	case job.ActorUserID != nil:
		return PrincipalActor
	case job.OwnerUserID != nil:
		return PrincipalOwner
	default:
		return PrincipalHost
	}
}

// executionAccess is the principal an execution acts as, or the refusal that
// replaces the fallback it used to make.
//
// A recorded principal that is gone is a refusal, never a substitution: the Job
// was accepted to act as that account, and running the work as the owner, as
// root or as the host would transfer authority that user deletion removed.
func executionAccess(job models.Job) (Access, error) {
	switch executionPrincipalOf(job) {
	case PrincipalHost:
		return Access{}, nil
	case PrincipalOwner:
		if job.OwnerUserID == nil {
			return Access{}, fmt.Errorf("%w: job %s was accepted to act as its owner, and that account is gone",
				ErrExecutionPrincipalUnavailable, job.ID)
		}
		return Access{UserID: *job.OwnerUserID}, nil
	default:
		if job.ActorUserID == nil {
			return Access{}, fmt.Errorf("%w: job %s was accepted to act as its actor, and that account is gone",
				ErrExecutionPrincipalUnavailable, job.ID)
		}
		return Access{UserID: *job.ActorUserID}, nil
	}
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
	// The series is advanced from the row read above, outside the transaction,
	// so the transaction still opens with its write. The executor holding the
	// token is the only writer of progress, so the read is current; a tick that
	// races a transition loses one sample at worst, and the guarded write below
	// refuses it anyway.
	if err := applyProgressHistory(&next, updates, now, progress, false); err != nil {
		return Snapshot{}, err
	}

	var snap Snapshot
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The transaction's first statement is the write, which takes the writer
		// lock before anything is read and is what serialises this against a
		// transition racing the same Job.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND "+executionTokenMatch+" AND state = ?", job.ID, job.ExecutionToken, job.State).
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
//
// That headroom is spent by the host only. A caller's own ReservedHost flag is
// cleared here rather than forwarded: this is the path an adapter's events take,
// and a flag the caller could set would make the reservation a budget the traffic
// it is reserved against can spend — the ceiling would then hold for nobody.
func (s *Service) AppendEvent(deps Deps, ref ExecutionRef, event EventInput) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	if err := validateAppendedEvent(event); err != nil {
		return err
	}
	event.ReservedHost = false

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
			Where("id = ? AND "+executionTokenMatch+" AND state = ?", job.ID, job.ExecutionToken, job.State).
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
//
// Storing the relation and confirming its endpoints are one decision, ordered by
// the engine's own concurrency control: retention deletes an expired Job and the
// relations incident to it, and a relation stored against an endpoint that went in
// the gap is exactly the dangling row the tables' missing foreign keys cannot
// catch. linkLineage is that ordering.
func (s *Service) Link(deps Deps, request LinkRequest) error {
	if err := validateLinkRequest(request); err != nil {
		return err
	}

	now := deps.now()
	return deps.DB.Transaction(func(tx *gorm.DB) error {
		return linkLineage(tx, request, now)
	})
}

// linkLineage stores one relation and confirms both endpoints are there, in the
// order the engine's concurrency control requires.
//
// The relation is written first on both engines, and a missing endpoint rolls the
// transaction back with it — so the write that takes the lock leaves nothing
// behind when the answer is no. That order is what the endpoints are then read
// under, which is the whole of the fix: the existence check is not a check-then-act
// a sweep can slip past.
//
// The engines differ in what holds the endpoints while they are read, exactly as
// lockPruneTarget does. SQLite has no row locks and serializes writers, so the
// insert is the write that takes the writer lock before anything is read; a
// transaction that reads first and writes afterwards is refused outright when
// another connection commits in between (SQLITE_BUSY_SNAPSHOT, which SQLite does
// not put through the busy handler). PostgreSQL has row locks, so both endpoints
// are taken FOR UPDATE in ascending id order before either is read — the order two
// relations naming the same pair in opposite directions would otherwise take in
// reverse — and retention's own delete of an endpoint waits on that row.
func linkLineage(tx *gorm.DB, request LinkRequest, now time.Time) error {
	if err := storeLink(tx, request, now); err != nil {
		return err
	}
	return holdLinkEndpoints(tx, request)
}

// storeLink writes the relation itself. It is idempotent: (type, from, to) is
// unique, so a repeated request is the row that is already there.
func storeLink(tx *gorm.DB, request LinkRequest, now time.Time) error {
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
}

// holdLinkEndpoints confirms both endpoints exist, holding their rows for the rest
// of the transaction on the engine that has row locks. A missing endpoint is
// ErrNotFound, and the relation written on the way in rolls back with that verdict.
func holdLinkEndpoints(tx *gorm.DB, request LinkRequest) error {
	// Sorted, and locked in that order: two transactions taking the same row locks
	// in opposite orders deadlock, and a relation is stored in both directions.
	ids := []string{request.FromJobID, request.ToJobID}
	sort.Strings(ids)

	query := tx.Model(&models.Job{}).Where("id IN ?", ids).Order("id")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var present []string
	if err := query.Pluck("id", &present).Error; err != nil {
		return fmt.Errorf("jobs: read link endpoints: %w", err)
	}
	if len(present) != 2 {
		return fmt.Errorf("%w: a link needs both jobs to exist", ErrNotFound)
	}
	return nil
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
	return ValidateMetrics(progress.Metrics)
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
// The shape of it follows the design: work is accepted queued or scheduled, reaches
// running through a claim, and may be paused, blocked, returned to the queue, or
// finished. A reconciliation may send a running Job back to the queue rather than
// guessing that it is still alive, and blocked work returns to the queue when policy
// or an operator unblocks it. Running is deliberately absent from every target set,
// including running's own: entering running is what a claim does, and
// validateTransition refuses it here before the table is consulted.
//
// One edge is deliberately not in the table because it is not the machine's to make:
// the execution that owns a *quarantined* Job may end it, success included. See
// quarantineSettlementAllowed, which is where the token fence for it lives.
var legalTargets = map[State][]State{
	StateScheduled: {StateQueued, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateQueued:    {StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateRunning:   {StateQueued, StatePaused, StateBlocked, StateSucceeded, StateFailed, StateCancelled, StateInterrupted},
	StatePaused:    {StateQueued, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateBlocked:   {StateQueued, StateCancelled, StateFailed, StateInterrupted},
}

// quarantineSettlementAllowed answers the one lifecycle edge the state machine does
// not name: the execution that owns a quarantined Job may end it.
//
// A quarantine is `blocked` with the execution token still recorded, because nobody
// could prove the external work the claim started had stopped (§3). The execution
// that owns that token is the one thing that *can* prove it, and it proves it by
// reporting what became of its own work — its real outcome, a success included.
// Without this edge a quarantined Job whose work in fact succeeded could never reach
// its outcome: the row would stay blocked and its capacity — a deployment-wide slot —
// would stay occupied for ever, including across a restart, because an expired scan
// never revisits a quarantined claim and a Resume is refused while one is unresolved.
//
// The fence is the token, and that is why this is not a hole in the machine. The
// block path releases the claim in the same transaction that enters `blocked`, so a
// blocked Job carrying a token is a quarantined one by construction, while a blocked
// Job with no token is an ordinary hold no execution owns. A caller naming some other
// execution's token cannot write at all: requireExecutionToken has refused it already.
func quarantineSettlementAllowed(job models.Job, transition Transition) bool {
	if State(job.State) != StateBlocked || !transition.To.Terminal() {
		return false
	}
	return job.ExecutionToken != "" && transition.ExecutionToken == job.ExecutionToken
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
	return s.commitTransition(deps, prepared, successVerification(deps, prepared, transition.To))
}

// successVerification is the output check §7 attaches to the outcome itself
// rather than to one entry point: a Job becomes succeeded only after every
// required output is durable and available.
//
// Transition permits running -> succeeded, so a check that lived only in Finish
// would leave one success boundary unguarded — an adapter could publish an
// artifact it cannot serve and then record success by transitioning. The check
// is the same function in both calls, and it runs inside the same transaction as
// the state it is about to justify.
func successVerification(deps Deps, prepared preparedTransition, to State) func(tx *gorm.DB) error {
	if to != StateSucceeded {
		return nil
	}
	now := deps.now()
	return func(tx *gorm.DB) error {
		return verifyRequiredOutputs(tx, prepared.job.ID, nil, now)
	}
}

// Finish ends a Job an execution owns, and is the way an executor ends one
// deliberately.
//
// It is a terminal transition plus the verification §7 requires: when the
// outcome is success, every required output the Job published — and every key
// the caller names — must be durable and available, and that check runs inside
// the same transaction as the terminal state and its event. Transition carries
// the same check for the same outcome, because the contract belongs to success
// rather than to this entry point; Finish is where a caller declares *which*
// outputs it promised. A Job therefore
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
	// The outcome is checked before the transition is validated: "Finish ends a
	// Job" is this entry point's own contract, and a caller that named a
	// non-terminal outcome is told that rather than one of the lifecycle's other
	// refusals.
	if !request.Outcome.Terminal() {
		return Snapshot{}, fmt.Errorf("%w: Finish ends a Job, and %q is not a terminal state",
			ErrInvalidTransition, request.Outcome)
	}
	if request.FinalProgress != nil {
		if err := validateProgress(*request.FinalProgress); err != nil {
			return Snapshot{}, err
		}
	}
	if err := validateTransition(&transition); err != nil {
		return Snapshot{}, err
	}

	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		return Snapshot{}, err
	}
	if request.FinalProgress != nil {
		if err := applyFinalProgress(&prepared, *request.FinalProgress, deps.now()); err != nil {
			return Snapshot{}, err
		}
	} else if len(prepared.next.ProgressSeries) > 0 {
		// Most executors finish without a final snapshot; their last tick may
		// have landed inside the sampling interval and never reached the
		// series. Closing the series with the stored snapshot puts the final
		// count on the graph and in the average rate.
		if err := applyProgressHistory(&prepared.next, prepared.updates, deps.now(), storedProgress(prepared.next), true); err != nil {
			return Snapshot{}, err
		}
	}

	var verify func(tx *gorm.DB) error
	if request.Outcome == StateSucceeded {
		now := deps.now()
		verify = func(tx *gorm.DB) error {
			return verifyRequiredOutputs(tx, prepared.job.ID, request.RequiredOutputs, now)
		}
	}
	return s.commitTransition(deps, prepared, verify)
}

// applyFinalProgress adds a FinishRequest's optional final snapshot to the same
// guarded write as its terminal transition. Progress is copied into both the
// prepared model and its update map so the committed snapshot and stored row
// describe the same outcome.
func applyFinalProgress(prepared *preparedTransition, progress Progress, now time.Time) error {
	prepared.next.ProgressCompleted = copyInt64(progress.Completed)
	prepared.next.ProgressTotal = copyInt64(progress.Total)
	prepared.next.ProgressUnit = progress.Unit
	prepared.next.ProgressMessage = progress.Message
	prepared.next.ProgressETA = utcPtr(progress.ETA)
	prepared.updates["progress_completed"] = prepared.next.ProgressCompleted
	prepared.updates["progress_total"] = prepared.next.ProgressTotal
	prepared.updates["progress_unit"] = prepared.next.ProgressUnit
	prepared.updates["progress_message"] = prepared.next.ProgressMessage
	prepared.updates["progress_eta"] = prepared.next.ProgressETA
	if progress.Phase != "" {
		prepared.next.Phase = progress.Phase
		prepared.updates["phase"] = prepared.next.Phase
	}
	return applyProgressHistory(&prepared.next, prepared.updates, now, progress, true)
}

// storedProgress is the progress snapshot a row currently holds.
func storedProgress(job models.Job) Progress {
	return Progress{
		Completed: copyInt64(job.ProgressCompleted), Total: copyInt64(job.ProgressTotal),
		Unit: job.ProgressUnit, Message: job.ProgressMessage, ETA: job.ProgressETA,
		Metrics: decodeMetrics(job.ProgressMetrics),
	}
}

// applyProgressHistory adds a snapshot's metrics and its series sample to a
// progress write. The series column is only written when the sample changed it,
// which is at most once a second for a Job ticking faster than that, so the
// history costs one small JSON write per interval rather than one per tick.
func applyProgressHistory(next *models.Job, updates map[string]any, now time.Time, progress Progress, final bool) error {
	metrics, err := encodeMetrics(progress.Metrics)
	if err != nil {
		return err
	}
	next.ProgressMetrics = metrics
	updates["progress_metrics"] = metrics

	series, changed := advanceSeries(decodeSeries(next.ProgressSeries), now, progress, final)
	if changed {
		encoded, err := json.Marshal(series)
		if err != nil {
			return fmt.Errorf("jobs: encode progress series: %w", err)
		}
		next.ProgressSeries = types.JSON(encoded)
		updates["progress_series"] = next.ProgressSeries
	}
	updatedAt := now.UTC()
	next.ProgressUpdatedAt = &updatedAt
	updates["progress_updated_at"] = next.ProgressUpdatedAt
	return nil
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
	// The event is described rather than built: its position on the Job's
	// timeline is allocated inside the transaction, after the guarded write, so
	// it is the next position at commit time rather than the next position at
	// decision time.
	eventType   string
	eventDetail json.RawMessage
	at          time.Time
	// releaseReason is what the claim records when leaving running hands it back. A
	// transition's own reason is that it moved the Job; a release made as the
	// adapter's decision records the reason that decision gave.
	releaseReason string
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
	if !canTransition(State(job.State), transition.To) && !quarantineSettlementAllowed(job, transition) {
		return preparedTransition{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, job.State, transition.To)
	}
	// A cancellation that won owns the outcome: §4 makes a later success
	// unrepresentable rather than merely discouraged, because an executor that
	// finishes anyway would otherwise erase what the person watching it asked for.
	// The refusal leaves the Job exactly where it is — running, with its intent —
	// so the execution can still publish the cancellation it was asked for.
	//
	// A success decided *before* the cancellation cannot reach the write either,
	// and needs no second guard to stop it: recording the intent moves the Job's
	// version, and the guarded update below carries the version (and the state and
	// the token) the caller decided from. The two refusals together are what make
	// "once cancellation intent wins, later success cannot overwrite it" true of
	// every interleaving rather than of the ones a read happens to see.
	if transition.To == StateSucceeded && job.ControlIntent == ControlIntentCancel {
		return preparedTransition{}, fmt.Errorf("%w: job %s", ErrControlIntentWon, job.ID)
	}

	now := deps.now()
	next, updates := applyTransition(job, transition, deps.retention(), now)
	return preparedTransition{
		job:           job,
		next:          next,
		updates:       updates,
		eventType:     eventTypeFor(transition, job.StartedAt != nil),
		eventDetail:   transition.Event.Detail,
		at:            now,
		releaseReason: ReleaseReasonStateChanged,
	}, nil
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
		// The predicate carries the execution token as well as the version and
		// state the caller read, because a release clears the token without
		// moving either of them: an execution whose claim was handed back in the
		// window between the decision and this write must not commit it. A Job
		// with no token matches an empty one, so a host-side transition on work no
		// claim ever touched still applies.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ? AND "+executionTokenMatch,
				prepared.job.ID, prepared.job.Version, prepared.job.State, prepared.job.ExecutionToken).
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
			if err := stampReplayExpiry(tx, prepared.next, deps.replayRetention(), deps.now()); err != nil {
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
			if err := releaseClaimTx(tx, prepared.job.ID, prepared.job.ExecutionToken, prepared.releaseReason, deps.now()); err != nil {
				return err
			}
		}
		// The event's position is allocated here rather than where the transition
		// was decided: the guarded update above has locked the Job's row by now,
		// so an executor appending its own phase event is either before this
		// position or behind the lock — never on it. Reading the maximum outside
		// the transaction let an append take the position the terminal event was
		// about to claim, and the unique index then rolled the completion back.
		sequence, err := nextEventSequence(tx, prepared.job.ID)
		if err != nil {
			return err
		}
		event := newEvent(prepared.job.ID, sequence, prepared.next.Version, prepared.eventType, prepared.eventDetail, true, prepared.at)
		if err := tx.Create(&event).Error; err != nil {
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
//
// The retention policy is passed in because a terminal transition is where a Job
// acquires its deadline: the window that governs this Job is the one in effect
// when it finished, stamped from finished_at, so a later policy change does not
// retroactively rewrite history and a Job's own expiry is visible in advance.
func applyTransition(job models.Job, transition Transition, policy RetentionPolicy, now time.Time) (models.Job, map[string]any) {
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

	// A pause request is resolved by the Job leaving running — whether it reached the
	// checkpoint it asked for or went somewhere else instead. Only the execution that
	// owns a running Job can confirm a checkpoint, so a Job that ends up blocked or
	// back in the queue has none, and an intent left standing there is a request no
	// execution can ever resolve: it would read as outstanding for the rest of the
	// Job's life, and the command that moves the Job on would report itself as merely
	// requested because of it.
	//
	// A cancellation's intent is deliberately not resolved this way: §4 resolves it by
	// cancellation, which is why a Job that blocks while being cancelled still refuses
	// every success and is still the Job a cancel command ends.
	if State(job.State) == StateRunning && next.ControlIntent == ControlIntentPause {
		next.ControlIntent = ""
		next.ControlRequestedAt = nil
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
	case StateCancelled, StateFailed, StateInterrupted, StateSucceeded:
		// Any end state resolves whatever control was asked for: the intent records
		// a request that has not reached its outcome yet, and this is the outcome.
		next.ControlIntent = ""
		next.ControlRequestedAt = nil
	}
	if transition.To.Terminal() {
		next.FinishedAt = &now
		// Retention starts at terminal completion, never at acceptance: a Job
		// that spent a month queued is not a Job whose history is a month old.
		expires := now.Add(policy.windowFor(transition.To)).UTC()
		next.ExpiresAt = &expires
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
		"control_intent":         next.ControlIntent,
		"control_requested_at":   next.ControlRequestedAt,
		"running_duration":       next.RunningDuration,
		"paused_duration":        next.PausedDuration,
		"blocked_duration":       next.BlockedDuration,
		"queue_duration":         next.QueueDuration,
		"queued_at":              next.QueuedAt,
		"started_at":             next.StartedAt,
		"last_resumed_at":        next.LastResumedAt,
		"finished_at":            next.FinishedAt,
		"expires_at":             next.ExpiresAt,
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
	// Running is admitted by a claim and nothing else. Checked here, before the
	// state machine, because it is true whatever the Job is in now: a Job that is
	// running has an execution, a token, a claim and the capacity that admitted
	// it, and a transition that minted the state without any of them would leave
	// work nothing can claim, nothing reconciles and no release can free.
	if transition.To == StateRunning {
		return fmt.Errorf("%w: job %s", ErrRunningRequiresClaim, transition.JobID)
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

	// Most runtime ticks have nothing to publish. Check before opening a write
	// transaction so an idle publisher never seeds or locks the global allocator
	// row. An event committed after this read remains unsequenced for the next
	// tick, which is preferable to contending on SQLite's single writer lock.
	var pendingCount int64
	if err := deps.DB.Model(&models.JobEvent{}).
		Where("delivery_sequence IS NULL").
		Count(&pendingCount).Error; err != nil {
		return 0, fmt.Errorf("jobs: count unsequenced events: %w", err)
	}
	if pendingCount == 0 {
		return 0, nil
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
