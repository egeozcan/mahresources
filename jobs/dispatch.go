package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file holds the dispatch protocol: claiming one waiting Job for one
// runtime, fenced by an execution token and bounded by durable capacity, and
// handing it to the Kind's adapter as an Execution.
//
// The split it is built on is the one the whole module is built on — decide
// outside the transaction, write inside it, with the transaction's first
// statement being the guarded write. That is what makes two runtimes contending
// for one Job admit exactly one of them, on SQLite and PostgreSQL alike.

// claimRowColumns are the columns a claim writes and a takeover rewrites. The
// identity and the Kind are not among them: a Job's Kind never changes, and the
// row keeps the instant it was first claimed as its CreatedAt.
var claimRowColumns = []string{
	"claimant", "execution_token", "state", "claimed_at", "heartbeat_at",
	"lease_expires_at", "released_at", "release_reason", "updated_at",
}

// errClaimContended is the internal answer for a Job that moved between the
// candidate read and the guarded write. It is not an error the caller sees: a
// claim that lost the race reports "nothing was claimed", which is the same
// thing it would report for an empty queue and is what a polling runtime acts
// on.
var errClaimContended = errors.New("jobs: the job was claimed by another runtime")

// Claim claims the next Job of one Kind that is waiting to run, and returns the
// execution a runtime dispatches it with.
//
// The claim is one transaction: it moves the Job to running, installs the
// fencing token, occupies every capacity budget, writes the claim row and
// records the started event. A Job is therefore never running without the
// capacity that admitted it, and a claim refused for capacity writes nothing.
//
// Nothing found and every budget full are both reported as "not claimed", with
// no error: a runtime cannot act differently on either answer, and the queue is
// re-read on the next pass.
func (s *Service) Claim(ctx context.Context, deps Deps, request ClaimRequest) (Execution, bool, error) {
	_, definition, err := s.adapterFor(request.Kind, request.KindVersion)
	if err != nil {
		return Execution{}, false, err
	}
	if err := validateClaimRequest(&request, definition); err != nil {
		return Execution{}, false, err
	}

	now := deps.now()
	job, found, err := nextClaimable(deps.DB, request.Kind, request.KindVersion, now)
	if err != nil || !found {
		return Execution{}, false, err
	}

	lease := request.Lease
	if lease <= 0 {
		lease = definition.claimLease()
	}
	token := types.NewUUIDv7()

	var claimed models.Job
	var claim models.JobClaim
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The guarded update is the admission: it is the transaction's first
		// statement (so SQLite takes the writer lock before it reads anything),
		// it is predicated on the state and version the candidate read saw, and
		// it requires an unowned Job — so a Job another runtime claimed in the
		// meantime matches no row.
		next, updates := applyTransition(job, Transition{To: StateRunning}, deps.retention(), now)
		updates["execution_token"] = token
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ? AND (execution_token IS NULL OR execution_token = '')",
				job.ID, job.Version, job.State).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("jobs: claim job: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errClaimContended
		}

		if err := acquireCapacityTx(tx, request.Capacity, job.ID, token, now); err != nil {
			return err
		}

		claim = models.JobClaim{
			JobID:          job.ID,
			Kind:           job.Kind,
			KindVersion:    job.KindVersion,
			Claimant:       request.Claimant,
			ExecutionToken: token,
			State:          models.JobClaimStateHeld,
			ClaimedAt:      now,
			HeartbeatAt:    now,
			LeaseExpiresAt: now.Add(lease),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		// One row per Job, so a Job claimed again after a reconciliation takes
		// the row over rather than inserting a second one. The takeover is
		// conditional on the row being released: a held or quarantined claim is
		// an execution that has not ended, and the Job guard above already
		// refused those — this is the second, row-level half of the same fence.
		result = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job_id"}},
			DoUpdates: clause.AssignmentColumns(claimRowColumns),
			Where: clause.Where{Exprs: []clause.Expression{
				clause.Eq{Column: "job_claims.state", Value: models.JobClaimStateReleased},
			}},
		}).Create(&claim)
		if result.Error != nil {
			return fmt.Errorf("jobs: store claim: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errClaimContended
		}

		sequence, err := nextEventSequence(tx, job.ID)
		if err != nil {
			return err
		}
		event := newEvent(job.ID, sequence, next.Version, EventStarted, nil, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store started event: %w", err)
		}
		claimed = next
		return nil
	})
	switch {
	case errors.Is(err, errClaimContended), errors.Is(err, ErrCapacityExhausted):
		return Execution{}, false, nil
	case err != nil:
		return Execution{}, false, err
	}

	execution, err := s.executionFor(ctx, deps, claimed, claim)
	if err != nil {
		return Execution{}, false, err
	}
	return execution, true, nil
}

// validateClaimRequest checks a claim and normalizes the budgets it occupies.
//
// The Kind's own budget is always taken, from the registered definition rather
// than from the request: a runtime cannot forget the budget its own Kind
// declares, and a request that names the same group again is redundant rather
// than additive.
func validateClaimRequest(request *ClaimRequest, definition Definition) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidClaim, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(request.Claimant) == "" {
		return invalid("a claim needs a claimant")
	}
	if len(request.Claimant) > MaxClaimantBytes {
		return invalid("claimant is %d bytes, over the %d-byte ceiling", len(request.Claimant), MaxClaimantBytes)
	}
	if request.Lease < 0 {
		return invalid("lease is negative")
	}

	kindBudget := CapacityRef{Group: definition.capacityGroup(), Limit: definition.MaxConcurrent}
	budgets := []CapacityRef{kindBudget}
	named := map[string]bool{kindBudget.Group: true}
	for _, budget := range request.Capacity {
		if strings.TrimSpace(budget.Group) == "" {
			return invalid("a capacity budget needs a group")
		}
		if budget.Limit < 0 {
			return invalid("budget %s has a negative limit", budget.Group)
		}
		if named[budget.Group] {
			continue
		}
		named[budget.Group] = true
		budgets = append(budgets, budget)
	}
	request.Capacity = budgets
	return nil
}

// nextClaimable selects the oldest Job of a Kind that is waiting to run: queued
// work, and scheduled work whose time has come. A Job that is already owned, in
// any other state, is not a candidate.
func nextClaimable(db *gorm.DB, kind string, version uint, now time.Time) (models.Job, bool, error) {
	var job models.Job
	err := db.Where(
		"kind = ? AND kind_version = ? AND (execution_token IS NULL OR execution_token = '') "+
			"AND (state = ? OR (state = ? AND scheduled_for IS NOT NULL AND scheduled_for <= ?))",
		kind, version, string(StateQueued), string(StateScheduled), now,
	).Order("accepted_at, id").First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return models.Job{}, false, nil
		}
		return models.Job{}, false, fmt.Errorf("jobs: select claimable job: %w", err)
	}
	return job, true, nil
}

// acquireCapacityTx occupies one slot in every budget the claim draws on.
//
// The budgets are taken in group order, so two claims holding several of them
// cannot take them in opposite orders. Each budget is all-or-nothing within the
// claim transaction: a budget that cannot be entered rolls back the slot in the
// budgets already taken, and the Job's move to running with them.
func acquireCapacityTx(tx *gorm.DB, budgets []CapacityRef, jobID, token string, now time.Time) error {
	ordered := make([]CapacityRef, len(budgets))
	copy(ordered, budgets)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Group < ordered[j].Group })

	for _, budget := range ordered {
		if budget.Limit <= 0 {
			continue
		}
		if err := occupyCapacitySlot(tx, budget, jobID, token, now); err != nil {
			return err
		}
	}
	return nil
}

// occupyCapacitySlot takes the lowest free slot in one budget.
//
// The insert is the admission rather than a count: (group, slot) is unique, so a
// slot two runtimes contend for is taken by exactly one of them and the loser
// moves to the next slot — and "no slot below the limit" is a full budget. A
// counter row would be shorter and would eventually disagree with the claims it
// describes; this cannot drift, because the rows *are* the occupancy.
func occupyCapacitySlot(tx *gorm.DB, budget CapacityRef, jobID, token string, now time.Time) error {
	for slot := 0; slot < budget.Limit; slot++ {
		lease := models.JobCapacityLease{
			ID:             types.NewUUIDv7(),
			CapacityGroup:  budget.Group,
			Slot:           slot,
			JobID:          jobID,
			ExecutionToken: token,
			AcquiredAt:     now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		// DO NOTHING rather than an error: a duplicate key would abort the whole
		// claim transaction on PostgreSQL, where the loser of a contended slot
		// still has every other slot to try.
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
		if result.Error != nil {
			return fmt.Errorf("jobs: occupy capacity in %s: %w", budget.Group, result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
	}
	return fmt.Errorf("%w: %s allows %d concurrent executions", ErrCapacityExhausted, budget.Group, budget.Limit)
}

// releaseCapacityTx frees every capacity slot one execution holds, by token, so
// a claim that was replaced cannot free the slots of the claim that replaced it.
func releaseCapacityTx(tx *gorm.DB, jobID, token string) error {
	if err := tx.Where("job_id = ? AND execution_token = ?", jobID, token).
		Delete(&models.JobCapacityLease{}).Error; err != nil {
		return fmt.Errorf("jobs: release capacity: %w", err)
	}
	return nil
}

// releaseClaimTx ends one execution's ownership of a Job: the claim is marked
// released, the capacity slots it held are freed, and the Job's fencing token is
// cleared, so a stopped execution cannot publish under a claim that has ended.
//
// A release under a token that holds nothing is not an error and writes nothing:
// every transition that leaves the running state comes through here, including
// the ones a host makes on a Job no claim ever touched, and a token that was
// already replaced by a reconciliation must not release the replacement's
// capacity.
func releaseClaimTx(tx *gorm.DB, jobID, token, reason string, now time.Time) error {
	if token == "" {
		return nil
	}
	// The Job's row first, always: every transaction that touches a Job's claim
	// or capacity takes it in this order, so a release racing a terminal
	// transition cannot leave each holding the row the other needs. The guarded
	// clear is also the "does this token own anything" question — a Job whose
	// token is not this one owns nothing, and a repeated release therefore writes
	// nothing more.
	job := tx.Model(&models.Job{}).Where("id = ? AND execution_token = ?", jobID, token).
		Update("execution_token", "")
	if job.Error != nil {
		return fmt.Errorf("jobs: clear execution token: %w", job.Error)
	}
	if job.RowsAffected == 0 {
		return nil
	}
	// Any claim this token still owns is released, quarantined ones included: a
	// quarantine says nobody could prove what happened to the work, and the
	// execution that owned it finishing the Job is that proof. Leaving it behind
	// would hold its capacity for a Job that has reached an outcome.
	result := tx.Model(&models.JobClaim{}).
		Where("job_id = ? AND execution_token = ? AND state <> ?", jobID, token, models.JobClaimStateReleased).
		Updates(map[string]any{
			"state":          models.JobClaimStateReleased,
			"released_at":    now,
			"release_reason": reason,
			"updated_at":     now,
		})
	if result.Error != nil {
		return fmt.Errorf("jobs: release claim: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return releaseCapacityTx(tx, jobID, token)
}

// executionFor builds the Execution for a claim and opens the input the
// execution runs with.
//
// Input that cannot be opened is not a reason to leave a Job running with an
// executor that will never start: when the refusal is one that blocks work —
// a key this process does not hold, a Kind version nothing can decode, a
// payload that fails authentication — the Job is blocked, fenced by the token
// that was just taken, and the claim is released with it.
func (s *Service) executionFor(ctx context.Context, deps Deps, job models.Job, claim models.JobClaim) (Execution, error) {
	// Who the work acts as is settled before what it runs with: a Job whose
	// recorded principal has been deleted may not be handed to an adapter at all,
	// because every adapter receives that identity as the authority its work
	// runs under.
	access, err := executionAccess(job)
	if err != nil {
		return Execution{}, s.blockUnrunnableJob(deps, job, claim, blockedReasonPrincipalMissing, err)
	}
	input, err := s.executionInput(deps, job)
	if err != nil {
		if ReplayBlocked(State(job.State), err) {
			return Execution{}, s.blockUnrunnableJob(deps, job, claim, blockedReasonInputUnavailable, err)
		}
		return Execution{}, err
	}
	return newExecution(ctx, deps, s, job, claim, access, input), nil
}

// Reasons a claimed Job is blocked by the control plane itself rather than by
// its adapter. They are stable codes on the Job's own timeline.
const (
	// blockedReasonInputUnavailable: the execution-required input cannot be
	// opened here, so the Job cannot run with what it was accepted with.
	blockedReasonInputUnavailable = "input-unavailable"
	// blockedReasonPrincipalMissing: the principal the Job's execution acts as
	// no longer exists, and no other principal may be substituted for it.
	blockedReasonPrincipalMissing = "principal-missing"
)

// executionInput opens the replay input an execution runs with. Work whose
// durable class says its input cannot be replayed — an explicitly non-replayable
// Kind — runs with none, because there is nothing to open and no promise was
// made about it.
//
// A Job whose class says its input *is* replayable and which has no envelope to
// open is the opposite case: it is refused, and the refusal blocks the Job.
// Running it with no input at all would execute incomplete work, which §3
// forbids outright.
func (s *Service) executionInput(deps Deps, job models.Job) (json.RawMessage, error) {
	opened, err := s.OpenReplay(deps, Access{Administrator: true}, job.ID)
	switch {
	case err == nil:
		return opened.Input, nil
	case errors.Is(err, ErrReplayAbsent) && ReplayClass(job.ReplayClass) == ReplayClassNonReplayable:
		return nil, nil
	default:
		return nil, err
	}
}

// blockUnrunnableJob blocks a Job an execution was claimed for but cannot run,
// under the claim's own token. It is the one place dispatch decides a Job's
// state itself, and it does so because the alternative — leaving it running with
// an executor that never started — is a Job nothing would ever resolve.
func (s *Service) blockUnrunnableJob(deps Deps, job models.Job, claim models.JobClaim, reason string, cause error) error {
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return fmt.Errorf("jobs: encode blocked detail: %w", err)
	}
	_, err = s.Transition(deps, Transition{
		JobID:           job.ID,
		ExpectedVersion: job.Version,
		ExecutionToken:  claim.ExecutionToken,
		To:              StateBlocked,
		Event:           EventInput{Type: EventBlocked, Detail: detail},
	})
	if err != nil {
		return fmt.Errorf("%w (and the job could not be blocked either: %v)", cause, err)
	}
	return cause
}

// newExecution builds the Execution an adapter is handed.
func newExecution(ctx context.Context, deps Deps, service *Service, job models.Job, claim models.JobClaim, access Access, input json.RawMessage) Execution {
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: claim.ExecutionToken}
	return Execution{
		JobID:          job.ID,
		Kind:           job.Kind,
		KindVersion:    job.KindVersion,
		Version:        job.Version,
		ExecutionToken: claim.ExecutionToken,
		Claimant:       claim.Claimant,
		Access:         access,
		Input:          input,
		report:         &executionReport{service: service, deps: deps.withContext(ctx), ref: ref},
	}
}

// withContext binds a context to the handle a report publishes through, so a
// cancelled runtime stops its executions' writes as well as its work.
func (d Deps) withContext(ctx context.Context) Deps {
	if ctx != nil && d.DB != nil {
		d.DB = d.DB.WithContext(ctx)
	}
	return d
}

// executionReport is the Service-backed report one execution publishes through.
// It is bound to one Job and one token: every call it makes carries that token,
// so an adapter cannot publish to a Job it does not own even by naming one.
type executionReport struct {
	service *Service
	deps    Deps
	ref     ExecutionRef
}

func (r *executionReport) Progress(progress Progress) (Snapshot, error) {
	return r.service.UpdateProgress(r.deps, r.ref, progress)
}

func (r *executionReport) Event(event EventInput) error {
	return r.service.AppendEvent(r.deps, r.ref, event)
}

func (r *executionReport) Output(output OutputInput) (Output, error) {
	return r.service.PublishOutput(r.deps, r.ref, output)
}

func (r *executionReport) Transition(transition Transition) (Snapshot, error) {
	transition.JobID = r.ref.JobID
	transition.ExecutionToken = r.ref.ExecutionToken
	return r.service.Transition(r.deps, transition)
}

func (r *executionReport) Finish(request FinishRequest) (Snapshot, error) {
	request.JobID = r.ref.JobID
	request.ExecutionToken = r.ref.ExecutionToken
	return r.service.Finish(r.deps, request)
}

// Heartbeat extends the lease of the claim one execution owns.
//
// It is the runtime's proof of life, and it is fenced by the token exactly as
// every other executor-side write is: a claim that was released, replaced by a
// reconciliation or taken over by another runtime matches no row, and the
// heartbeat is refused. A runtime that sees this refusal knows its execution was
// fenced and can stop it.
//
// The new expiry is the later of the stored one and now plus the extension, so
// a heartbeat arriving inside the lease never shortens it and one arriving after
// the lease ran out still leaves a usable lease rather than one in the past.
//
// Adding the extension *to* the stored expiry — which is what measuring from the
// later of now and the stored expiry and then adding to it amounts to — banks a
// whole lease per heartbeat: a runtime heartbeating every third of a lease builds
// an expiry hours in the future, and the abandoned claim it leaves behind is not
// reconcilable, and does not free its capacity, until all of that time passes.
// The lease means "this long since the last proof of life", and that is what the
// extension has to express.
func (s *Service) Heartbeat(deps Deps, ref ExecutionRef, extension time.Duration) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	if extension < 0 {
		return fmt.Errorf("%w: heartbeat extension is negative", ErrInvalidClaim)
	}

	now := deps.now()
	claim, err := loadClaim(deps.DB, ref.JobID)
	if err != nil {
		return err
	}
	if claim.State != models.JobClaimStateHeld || claim.ExecutionToken != ref.ExecutionToken {
		return fmt.Errorf("%w: job %s is not owned by this execution", ErrStaleExecution, ref.JobID)
	}

	// The comparison happens in the statement rather than on the value read
	// above, so two heartbeats racing cannot each compute an expiry from the same
	// stale one and write whichever landed last.
	extended := now.Add(extension)
	result := deps.DB.Model(&models.JobClaim{}).
		Where("job_id = ? AND execution_token = ? AND state = ?", ref.JobID, ref.ExecutionToken, models.JobClaimStateHeld).
		Updates(map[string]any{
			"lease_expires_at": gorm.Expr(
				"CASE WHEN lease_expires_at > ? THEN lease_expires_at ELSE ? END", extended, extended),
			"heartbeat_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return fmt.Errorf("jobs: extend claim lease: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: job %s was taken over while its lease was being extended", ErrStaleExecution, ref.JobID)
	}
	return nil
}

// ReleaseClaim ends one execution's ownership of a Job without changing the
// Job's state.
//
// It is the graceful half of the fence: an execution that stops — because its
// work ended, or because the runtime is shutting down and has quiesced the work
// — hands its claim back so the capacity it held is available again, rather than
// leaving a budget occupied until a lease expires. A release under a token that
// owns nothing is a no-op: releasing is idempotent, and the alternative would be
// a runtime that cannot safely report what it already reported.
//
// It deliberately does not move the Job's state. Where a Job that is no longer
// running belongs is the Kind's decision, made through its adapter's
// reconciliation or its own transitions.
func (s *Service) ReleaseClaim(deps Deps, ref ExecutionRef, reason string) (Snapshot, error) {
	if err := validateExecutionRef(ref); err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return Snapshot{}, fmt.Errorf("%w: a release needs a reason", ErrInvalidClaim)
	}
	if len(reason) > MaxReleaseReasonBytes {
		return Snapshot{}, fmt.Errorf("%w: release reason is %d bytes, over the %d-byte ceiling",
			ErrInvalidClaim, len(reason), MaxReleaseReasonBytes)
	}

	now := deps.now()
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		return releaseClaimTx(tx, ref.JobID, ref.ExecutionToken, reason, now)
	})
	if err != nil {
		return Snapshot{}, err
	}

	job, err := loadJob(deps.DB, ref.JobID)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot(job), nil
}

// loadClaim reads one Job's claim.
func loadClaim(db *gorm.DB, jobID string) (models.JobClaim, error) {
	var claim models.JobClaim
	err := db.Where("job_id = ?", jobID).First(&claim).Error
	if err != nil {
		if isNotFound(err) {
			return models.JobClaim{}, fmt.Errorf("%w: no claim on job %s", ErrStaleExecution, jobID)
		}
		return models.JobClaim{}, fmt.Errorf("jobs: load claim: %w", err)
	}
	return claim, nil
}

// ReconcileFailureCode is the bounded failure a Job records when its adapter
// answered "fail" for an expired claim. It names the decision rather than the
// adapter's own error text: a Job's failure record is a taxonomy a reader
// groups on, and raw error text is neither that nor something that is safe to
// store.
const ReconcileFailureCode = "reconciliation-failed"

// errReconcileSuperseded reports that a Job moved while it was being reconciled:
// the adapter that was asked no longer owns it, so there is nothing to apply and
// nothing to record.
var errReconcileSuperseded = errors.New("jobs: the job changed while it was being reconciled")

// Reasons a claim is quarantined, recorded on the Job's own timeline. They say
// why nobody could decide the claim, not who was to blame.
const (
	// quarantineReasonAdapterMissing: this process has no adapter for the Kind,
	// so no executor exists here that could prove anything about the work.
	quarantineReasonAdapterMissing = "adapter-missing"
	// quarantineReasonNonRestorable: the Kind declared that its work cannot be
	// run again, so a lease expiry is not enough to queue it a second time.
	quarantineReasonNonRestorable = "non-restorable-work"
	// quarantineReasonUnprovenWork: the adapter could not prove the external work
	// the claim started has stopped.
	quarantineReasonUnprovenWork = "external-work-unproven"
	// quarantineReasonPrincipalMissing: the principal the Job's execution acts as
	// was deleted. No adapter answer could be carried out, because every one of
	// them would run the work as a principal the Job was not accepted under.
	quarantineReasonPrincipalMissing = "principal-missing"
)

// ReconcileExpired asks each expired claim's adapter what should happen to its
// Job, and applies the answer.
//
// It never decides a Job's fate itself. Lease expiry permits reconciliation; it
// proves nothing about whether the work is still running, and only the Kind
// knows how to tell. A missing adapter is the one answer the control plane makes
// on its own, and it is the fail-safe one: the Job is blocked with its claim
// held, because no executor here exists that could prove the external work
// stopped.
//
// A claim whose adapter cannot answer or answers "nothing" is left exactly as it
// is for the next pass. The pass is bounded, and it is a pass rather than a sweep
// because the caller polls it.
func (s *Service) ReconcileExpired(ctx context.Context, deps Deps, claimant string, limit int) (ReconcileReport, error) {
	if strings.TrimSpace(claimant) == "" {
		return ReconcileReport{}, fmt.Errorf("%w: a reconciliation pass needs a claimant", ErrInvalidClaim)
	}
	if limit <= 0 {
		limit = DefaultReconcileBatch
	}

	now := deps.now()
	claims, err := expiredClaims(deps.DB, now, limit)
	if err != nil {
		return ReconcileReport{}, err
	}
	if len(claims) == 0 {
		return ReconcileReport{}, nil
	}

	report := ReconcileReport{Examined: len(claims)}
	// An adapter that answers outside the vocabulary has decided nothing, and its
	// Job is left exactly as it is. The failure is carried back to the caller
	// rather than aborting the pass, so one buggy Kind cannot starve every other
	// Kind's reconciliation behind it.
	var decisionErr error
	for _, claim := range claims {
		job, err := loadJob(deps.DB, claim.JobID)
		if err != nil {
			return report, err
		}
		if State(job.State) != StateRunning || job.ExecutionToken != claim.ExecutionToken {
			// The Job moved while its claim was expiring, so the token owns
			// nothing any more. There is nothing to reconcile and, crucially,
			// nothing to apply.
			continue
		}

		access, accessErr := executionAccess(job)
		if accessErr != nil {
			// The principal this execution acts as is gone. No adapter answer
			// could be executed — every one of them would run the work as
			// somebody the deleted account was not — and the conservative answer
			// is the same one a missing adapter gets: the Job is blocked with its
			// claim and capacity held, because releasing them would be releasing
			// work nobody has proved stopped.
			snap, err := s.quarantineClaim(deps, job, claim, quarantineReasonPrincipalMissing, now)
			if err != nil {
				return report, err
			}
			report.Outcomes = append(report.Outcomes, ReconcileOutcome{
				JobID: job.ID, Decision: ReconcileExternalWorkUnproven, Snapshot: snap,
			})
			continue
		}

		adapter, definition, err := s.adapterFor(claim.Kind, claim.KindVersion)
		if err != nil {
			snap, err := s.quarantineClaim(deps, job, claim, quarantineReasonAdapterMissing, now)
			if err != nil {
				return report, err
			}
			report.Outcomes = append(report.Outcomes, ReconcileOutcome{
				JobID: job.ID, Decision: ReconcileExternalWorkUnproven, Snapshot: snap,
			})
			continue
		}

		decision, err := adapter.Reconcile(ctx, s.reconcileRequest(ctx, deps, job, claim, access))
		if err != nil {
			// An adapter that could not answer has not decided anything, and
			// nothing may be applied on its behalf: an error here is a
			// reconciler that could not do its job, not a Job that failed.
			continue
		}

		if !validReconcileDecision(decision) {
			if decisionErr == nil {
				decisionErr = fmt.Errorf("%w: %s v%d answered %q",
					ErrInvalidReconcileDecision, claim.Kind, claim.KindVersion, decision)
			}
			continue
		}

		if !definition.Restorable && (decision == ReconcileResume || decision == ReconcileQueue) {
			// Closure-backed work: its in-memory state died with the process
			// that claimed it, so a lease expiry alone may never send it back
			// to the queue or to a fresh execution.
			snap, err := s.quarantineClaim(deps, job, claim, quarantineReasonNonRestorable, now)
			if err != nil {
				return report, err
			}
			report.Outcomes = append(report.Outcomes, ReconcileOutcome{
				JobID: job.ID, Decision: ReconcileExternalWorkUnproven, Snapshot: snap,
			})
			continue
		}

		if decision == ReconcileRemainRunning {
			ref := ExecutionRef{JobID: job.ID, ExecutionToken: claim.ExecutionToken}
			if err := s.Heartbeat(deps, ref, definition.claimLease()); err != nil {
				return report, err
			}
			report.Outcomes = append(report.Outcomes, ReconcileOutcome{
				JobID: job.ID, Decision: decision, Snapshot: snapshot(job),
			})
			continue
		}

		if decision == ReconcileResume {
			resumedJob, resumedClaim, err := s.resumeClaim(deps, job, claim, claimant, definition.claimLease(), now)
			if err != nil {
				if errors.Is(err, errReconcileSuperseded) {
					continue
				}
				return report, err
			}
			execution, err := s.executionFor(ctx, deps, resumedJob, resumedClaim)
			if err != nil {
				// The resumed execution could not be built — its input is not
				// readable in this process — and executionFor has blocked the
				// Job and released the replacement claim for it.
				continue
			}
			report.Resume = append(report.Resume, execution)
			report.Outcomes = append(report.Outcomes, ReconcileOutcome{
				JobID: job.ID, Decision: decision, Snapshot: snapshot(resumedJob),
			})
			continue
		}

		applied, snap, err := s.applyReconcileDecision(deps, job, claim, decision, now)
		if err != nil {
			if errors.Is(err, errReconcileSuperseded) {
				continue
			}
			return report, err
		}
		report.Outcomes = append(report.Outcomes, ReconcileOutcome{
			JobID: job.ID, Decision: applied, Snapshot: snap,
		})
	}
	return report, decisionErr
}

// validReconcileDecision reports whether a decision is one this release knows.
func validReconcileDecision(decision ReconcileDecision) bool {
	for _, known := range ReconcileDecisions {
		if decision == known {
			return true
		}
	}
	return false
}

// expiredClaims reads the held claims whose lease has run out, oldest first, in
// a bounded batch. A quarantined claim is deliberately not here: it was already
// reconciled as far as anyone could, and rescanning it would turn an
// unresolvable Job into a reconciliation loop.
func expiredClaims(db *gorm.DB, now time.Time, limit int) ([]models.JobClaim, error) {
	var claims []models.JobClaim
	if err := db.Where("state = ? AND lease_expires_at <= ?", models.JobClaimStateHeld, now).
		Order("lease_expires_at, job_id").Limit(limit).Find(&claims).Error; err != nil {
		return nil, fmt.Errorf("jobs: read expired claims: %w", err)
	}
	return claims, nil
}

// reconcileRequest builds what one adapter is told about an expired claim. The
// input is best effort: a Kind whose input cannot be produced here is told so by
// a nil Input rather than by an error, because it still has to answer what
// should happen to the Job.
func (s *Service) reconcileRequest(ctx context.Context, deps Deps, job models.Job, claim models.JobClaim, access Access) ReconcileRequest {
	input, err := s.executionInput(deps, job)
	if err != nil {
		input = nil
	}
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: claim.ExecutionToken}
	execution := Execution{
		JobID:          job.ID,
		Kind:           job.Kind,
		KindVersion:    job.KindVersion,
		Version:        job.Version,
		ExecutionToken: claim.ExecutionToken,
		Claimant:       claim.Claimant,
		Access:         access,
		Input:          input,
		report:         &executionReport{service: s, deps: deps.withContext(ctx), ref: ref},
	}
	return ReconcileRequest{
		Snapshot:       snapshot(job),
		Claimant:       claim.Claimant,
		LeaseExpiredAt: claim.LeaseExpiresAt,
		Input:          input,
		Access:         access,
		Execution:      execution,
	}
}

// applyReconcileDecision applies one decision to a Job that is still running
// under the claim the adapter answered about, and reports the decision that was
// actually applied.
//
// A decision the control plane will not apply — a success whose required output
// is missing, say — becomes a visible blocked Job rather than an untruthful
// outcome or a Job retried forever, and a decision the state machine refuses
// because the Job moved underneath the reconciler is applied as nothing at all.
func (s *Service) applyReconcileDecision(deps Deps, job models.Job, claim models.JobClaim, decision ReconcileDecision, now time.Time) (ReconcileDecision, Snapshot, error) {
	detail, err := json.Marshal(map[string]string{"reason": "lease-expired"})
	if err != nil {
		return "", Snapshot{}, fmt.Errorf("jobs: encode reconciliation detail: %w", err)
	}
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: claim.ExecutionToken}

	var snap Snapshot
	switch decision {
	case ReconcileQueue:
		snap, err = s.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: claim.ExecutionToken,
			To: StateQueued, Event: EventInput{Type: EventQueued, Detail: detail},
		})
	case ReconcileBlock:
		snap, err = s.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: claim.ExecutionToken,
			To: StateBlocked, Event: EventInput{Type: EventBlocked, Detail: detail},
		})
	case ReconcileInterrupt:
		snap, err = s.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: claim.ExecutionToken,
			To: StateInterrupted, Event: EventInput{Type: EventInterrupted, Detail: detail},
		})
	case ReconcileFail:
		snap, err = s.Finish(deps, FinishRequest{
			ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateFailed,
			Failure: &Failure{Code: ReconcileFailureCode, Class: FailureClassInternal},
		})
	case ReconcileSucceed:
		snap, err = s.Finish(deps, FinishRequest{
			ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded,
		})
	case ReconcileExternalWorkUnproven:
		snap, err = s.quarantineClaim(deps, job, claim, quarantineReasonUnprovenWork, now)
		return ReconcileExternalWorkUnproven, snap, err
	default:
		return "", Snapshot{}, fmt.Errorf("%w: %q", ErrInvalidReconcileDecision, decision)
	}

	switch {
	case err == nil:
		return decision, snap, nil
	case errors.Is(err, errReconcileSuperseded), errors.Is(err, ErrStaleExecution), errors.Is(err, ErrVersionConflict):
		return "", Snapshot{}, errReconcileSuperseded
	default:
		// The decision was refused on its merits — a success whose required
		// output is not available, an outcome this Job may not enter. The Job is
		// blocked with the refusal recorded, rather than left to be reconciled
		// again and again.
		refusal, marshalErr := json.Marshal(map[string]string{
			"reason": "reconciliation-refused", "decision": string(decision),
		})
		if marshalErr != nil {
			return "", Snapshot{}, fmt.Errorf("jobs: encode reconciliation refusal: %w", marshalErr)
		}
		blocked, blockErr := s.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: claim.ExecutionToken,
			To: StateBlocked, Event: EventInput{Type: EventBlocked, Detail: refusal},
		})
		if blockErr != nil {
			return "", Snapshot{}, blockErr
		}
		return ReconcileBlock, blocked, nil
	}
}

// resumeClaim hands an expired Job's claim to the reconciling runtime under a
// fresh token.
//
// The replacement is one transaction and it re-tokens the capacity rows with it:
// capacity survives a resume — the execution is continuing rather than being
// re-admitted — and a row still stamped with the expired token would be released
// by nobody and freed by the replacement's own completion.
func (s *Service) resumeClaim(deps Deps, job models.Job, claim models.JobClaim, claimant string, lease time.Duration, now time.Time) (models.Job, models.JobClaim, error) {
	token := types.NewUUIDv7()
	next := job
	next.Version = job.Version + 1
	next.ExecutionToken = token
	replacement := models.JobClaim{
		JobID:          job.ID,
		Kind:           job.Kind,
		KindVersion:    job.KindVersion,
		Claimant:       claimant,
		ExecutionToken: token,
		State:          models.JobClaimStateHeld,
		ClaimedAt:      now,
		HeartbeatAt:    now,
		LeaseExpiresAt: now.Add(lease),
		CreatedAt:      claim.CreatedAt,
		UpdatedAt:      now,
	}

	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		// The Job's token goes first: it is the write that fences the expired
		// execution out of every publish path, and it is predicated on the token
		// that is being replaced.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ? AND execution_token = ?",
				job.ID, job.Version, job.State, claim.ExecutionToken).
			Updates(map[string]any{
				"execution_token": token,
				"version":         next.Version,
				"updated_at":      now,
			})
		if result.Error != nil {
			return fmt.Errorf("jobs: replace execution token: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errReconcileSuperseded
		}

		result = tx.Model(&models.JobClaim{}).
			Where("job_id = ? AND execution_token = ? AND state = ?", job.ID, claim.ExecutionToken, models.JobClaimStateHeld).
			Updates(map[string]any{
				"claimant":         claimant,
				"execution_token":  token,
				"state":            models.JobClaimStateHeld,
				"claimed_at":       now,
				"heartbeat_at":     now,
				"lease_expires_at": now.Add(lease),
				"released_at":      nil,
				"release_reason":   "",
				"updated_at":       now,
			})
		if result.Error != nil {
			return fmt.Errorf("jobs: replace claim: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errReconcileSuperseded
		}

		if err := tx.Model(&models.JobCapacityLease{}).
			Where("job_id = ? AND execution_token = ?", job.ID, claim.ExecutionToken).
			Update("execution_token", token).Error; err != nil {
			return fmt.Errorf("jobs: re-token capacity: %w", err)
		}

		sequence, err := nextEventSequence(tx, job.ID)
		if err != nil {
			return err
		}
		event := newEvent(job.ID, sequence, next.Version, EventResumed, nil, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store resumed event: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Job{}, models.JobClaim{}, err
	}
	return next, replacement, nil
}

// quarantineClaim blocks a Job whose expired claim nobody could resolve, and
// keeps the claim — with the capacity it holds — exactly where it is.
//
// The claim is the write that takes the row out of the expiry scan, so the Job
// cannot be reconciled in a loop; the Job keeps its token, because that token is
// what an executor which is in fact alive can still finish under, and what proves
// which execution the quarantine is about. Releasing any of it would be a claim
// that nobody proved anything about, and the whole point of this path is that
// dispatch over unproven work is how duplicate external side effects happen.
func (s *Service) quarantineClaim(deps Deps, job models.Job, claim models.JobClaim, reason string, now time.Time) (Snapshot, error) {
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return Snapshot{}, fmt.Errorf("jobs: encode quarantine detail: %w", err)
	}

	next, updates := applyTransition(job, Transition{To: StateBlocked}, deps.retention(), now)
	var snap Snapshot
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The Job's row first, as every lifecycle, release and reconciliation
		// transaction takes it: quarantining in the opposite order from the
		// terminal transition is what let a quarantine and a finish hold the two
		// rows each other needed.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ? AND execution_token = ?",
				job.ID, job.Version, job.State, claim.ExecutionToken).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("jobs: block quarantined job: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errReconcileSuperseded
		}

		result = tx.Model(&models.JobClaim{}).
			Where("job_id = ? AND execution_token = ? AND state = ?", job.ID, claim.ExecutionToken, models.JobClaimStateHeld).
			Updates(map[string]any{"state": models.JobClaimStateQuarantined, "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("jobs: quarantine claim: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errReconcileSuperseded
		}

		sequence, err := nextEventSequence(tx, job.ID)
		if err != nil {
			return err
		}
		event := newEvent(job.ID, sequence, next.Version, EventBlocked, detail, true, now)
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("jobs: store quarantine event: %w", err)
		}
		snap = snapshot(next)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}
