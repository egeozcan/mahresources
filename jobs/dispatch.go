package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
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
	job, found, err := nextClaimable(deps.DB, request.Kind, request.KindVersion, request.JobID, now)
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
		// meantime matches no row. Moving the Job into running is this protocol's
		// own step: validateTransition refuses that target for every caller, and
		// this is the one admission that is allowed to write it.
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

	execution, err := s.executionFor(ctx, deps, claimed, claim, State(job.State), claimFromWaiting)
	if err != nil {
		return Execution{}, false, err
	}
	return execution, true, nil
}

// validateClaimRequest checks a claim and normalizes the budgets it occupies.
//
// The Kind's own budget is always taken, from the registered definition rather
// than from the request: a runtime cannot forget the budget its own Kind
// declares, and a request that names the same group again does not add a second
// budget for it.
//
// Two limits for one group are merged into the strictest positive one rather than
// the first one seen. The groups collide in practice, because a Kind may declare
// `CapacityGroup: "global"` — the deployment-wide group the runtime always asks
// for — and picking either limit is wrong: taking the Kind's would discard the
// deployment's ceiling (a Kind declaring no limit of its own would leave it
// unenforced altogether), and taking the deployment's would discard a Kind's own
// stricter cap. "Not enforced" is what a limit of zero means, so a zero never
// loosens a positive one.
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
	named := map[string]int{kindBudget.Group: 0}
	for _, budget := range request.Capacity {
		if strings.TrimSpace(budget.Group) == "" {
			return invalid("a capacity budget needs a group")
		}
		if budget.Limit < 0 {
			return invalid("budget %s has a negative limit", budget.Group)
		}
		if at, seen := named[budget.Group]; seen {
			budgets[at].Limit = strictestCapacityLimit(budgets[at].Limit, budget.Limit)
			continue
		}
		named[budget.Group] = len(budgets)
		budgets = append(budgets, budget)
	}
	request.Capacity = budgets
	return nil
}

// strictestCapacityLimit combines two limits for one capacity group: the tighter
// enforced one, or zero when neither is enforced.
func strictestCapacityLimit(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// nextClaimable selects the oldest Job of a Kind that is waiting to run: queued
// work, and scheduled work whose time has come. A Job that is already owned, in
// any other state, is not a candidate.
//
// A named job id narrows the read to that one Job, which is how an executor that
// already knows which Job it is running — a host-side executor that materialized
// it a moment ago — takes it under a claim rather than taking whatever happens to
// be oldest.
func nextClaimable(db *gorm.DB, kind string, version uint, jobID string, now time.Time) (models.Job, bool, error) {
	query := db.Where(
		"kind = ? AND kind_version = ? AND (execution_token IS NULL OR execution_token = '') "+
			"AND (state = ? OR (state = ? AND scheduled_for IS NOT NULL AND scheduled_for <= ?))",
		kind, version, string(StateQueued), string(StateScheduled), now,
	)
	if jobID != "" {
		// Predicated on the Kind as well the id: a claim that named a Job of
		// another Kind would hand it to an adapter that does not own its input
		// shape, and the caller here is the one place a job id arrives untyped.
		query = query.Where("jobs.id = ?", jobID)
	}
	var job models.Job
	err := query.Order("accepted_at, jobs.id").First(&job).Error
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
// The budgets are taken in group order, and that order is the order the group
// locks are taken in as well as the order the slots are, so two claims holding
// several budgets cannot take either in opposite orders: they do not deadlock and
// neither of them admits past the other's count. Each budget is all-or-nothing
// within the claim transaction: a budget that cannot be entered rolls back the
// slot in the budgets already taken, and the Job's move to running with them.
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

// occupyCapacitySlot takes the lowest free slot in one budget, if the budget has
// room for one more execution.
//
// The rows are the occupancy — a counter would drift from the claims it is
// supposed to describe — and the whole of the budget is what they are counted
// against, not the slot range the *current* limit admits. A limit can be lowered
// under a running deployment, and the executions admitted under the wider one
// keep the higher-numbered slots they took: taken alone, "is a slot below the
// limit free" then admits a second execution into a budget whose one execution is
// already running in slot 1, and a restart with a lower `max-job-concurrency`
// does the same to whatever an unresolved claim still holds. So the count is the
// admission.
//
// Counting and taking are one decision, and a decision has to be one the next
// runtime can see, so both happen under a serialization of the group across
// processes (lockCapacityGroup). The insert is still the last word within that:
// (group, slot) is unique, so a slot that is occupied is not taken twice.
func occupyCapacitySlot(tx *gorm.DB, budget CapacityRef, jobID, token string, now time.Time) error {
	if err := lockCapacityGroup(tx, budget.Group); err != nil {
		return err
	}
	var occupied int64
	if err := tx.Model(&models.JobCapacityLease{}).
		Where("capacity_group = ?", budget.Group).Count(&occupied).Error; err != nil {
		return fmt.Errorf("jobs: count capacity in %s: %w", budget.Group, err)
	}
	if occupied >= int64(budget.Limit) {
		return capacityExhausted(budget)
	}
	// A free slot below the limit always exists here: every slot below it being
	// occupied would mean at least `limit` occupied rows, and the count just said
	// there are fewer. The insert still decides, so a slot the count could not
	// have known about is taken by exactly one runtime.
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
		// DO NOTHING rather than an error: the loop is looking for a free slot, and
		// a slot that is already taken is simply passed over. Nothing inside this
		// group can be taking one alongside us — the count was decided under the
		// group's lock — and an error here would abort a claim that still has
		// another slot to try.
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
		if result.Error != nil {
			return fmt.Errorf("jobs: occupy capacity in %s: %w", budget.Group, result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}
	}
	return capacityExhausted(budget)
}

// capacityExhausted is the refusal every full budget reports, with the group and
// the limit that refused it, so a log line says which budget was at its ceiling.
func capacityExhausted(budget CapacityRef) error {
	return fmt.Errorf("%w: %s allows %d concurrent executions", ErrCapacityExhausted, budget.Group, budget.Limit)
}

// lockCapacityGroup holds one capacity budget's admission for the rest of the
// claim transaction, so the count it decides on is a count no other runtime can
// be inside of.
//
// The engines hold it the way each of them can. SQLite has no row locks but one
// writer, and the claim's first statement is a write (see this file's header),
// so no other connection is inside a claim while this one is: the count and the
// insert are already one decision there. PostgreSQL runs several writers, so the
// group is held with a transaction-scoped advisory lock — the instrument this
// tree uses for a decision no single row can carry
// (application_context/group_tree_lock.go) — released when the claim's
// transaction ends, committed or rolled back alike.
//
// A dialect with neither is refused rather than admitted: work admitted without
// a serialization of its group is exactly the over-admission this lock exists to
// prevent, and it would be invisible until a limit was lowered.
func lockCapacityGroup(tx *gorm.DB, group string) error {
	switch tx.Dialector.Name() {
	case "sqlite":
		return nil
	case "postgres":
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", capacityAdvisoryLockKey(group)).Error; err != nil {
			return fmt.Errorf("jobs: serialize capacity admission in %s: %w", group, err)
		}
		return nil
	default:
		return fmt.Errorf("jobs: the %s dialect has no capacity admission lock", tx.Dialector.Name())
	}
}

// capacityAdvisoryLockKey is the PostgreSQL advisory lock one capacity group's
// admissions share, derived from the group's name.
//
// The mapping only has to be stable and near-unique: two budgets whose names
// hash to one key share a lock, which costs them some concurrency and nothing
// else, because the occupancy rows are what admits and refuses work. It is
// deliberately not a constant: two budgets that share a lock for no reason would
// serialize every claim of a Kind that names a group of its own against the
// deployment-wide budget.
func capacityAdvisoryLockKey(group string) int64 {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(group))
	return int64(digest.Sum64())
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

// claimOrigin says which piece of work a claim was taken over, and it is the whole
// of the quiescence rule.
//
// A claim is taken in two places: over work that was *waiting* (queued or
// scheduled, admitted by a submission or by a dispatch loop) and over work an
// *expired* claim already owned (a reconciliation's resume). Nothing is running
// behind the first — no executor exists until the claim is handed to one — so a Job
// the control plane cannot hand to an adapter there is this process's own problem,
// and its claim may be handed back. Behind the second an execution may still be
// running somewhere, and an unreadable input or a vanished principal proves nothing
// about it: releasing that claim is how a Resume is dispatched over live work.
type claimOrigin int

const (
	// claimFromWaiting is work that was not running when the claim was taken.
	claimFromWaiting claimOrigin = iota
	// claimFromExpired is an expired claim replaced by a reconciliation.
	claimFromExpired
)

// executionFor builds the Execution for a claim and opens the input the
// execution runs with.
//
// Input that cannot be opened is not a reason to leave a Job running with an
// executor that will never start: when the refusal is one that blocks work —
// a key this process does not hold, a Kind version nothing can decode, a
// payload that fails authentication — the Job is blocked, fenced by the token
// that was just taken. Whether that block also hands the claim back is what
// claimOrigin decides: for work that was waiting it does, and for a claim taken
// over an expired one it does not, because the execution it replaced may still be
// running the work.
func (s *Service) executionFor(ctx context.Context, deps Deps, job models.Job, claim models.JobClaim, claimedFrom State, origin claimOrigin) (Execution, error) {
	// Who the work acts as is settled before what it runs with: a Job whose
	// recorded principal has been deleted may not be handed to an adapter at all,
	// because every adapter receives that identity as the authority its work
	// runs under.
	access, err := executionAccess(job)
	if err != nil {
		return Execution{}, s.unrunnableClaim(deps, origin, job, claim,
			blockedReasonPrincipalMissing, quarantineReasonPrincipalMissing, err)
	}
	input, err := s.executionInput(deps, job)
	if err != nil {
		if ReplayBlocked(State(job.State), err) {
			return Execution{}, s.unrunnableClaim(deps, origin, job, claim,
				blockedReasonInputUnavailable, quarantineReasonInputUnavailable, err)
		}
		return Execution{}, err
	}
	return newExecution(ctx, deps, s, job, claim, access, input, claimedFrom), nil
}

// unrunnableClaim decides what becomes of a claim the control plane cannot hand to
// an adapter: an ordinary block that hands the claim back, or a quarantine that keeps
// it.
func (s *Service) unrunnableClaim(deps Deps, origin claimOrigin, job models.Job, claim models.JobClaim, blockReason, quarantineReason string, cause error) error {
	if origin == claimFromExpired {
		return s.quarantineUnrunnableClaim(deps, job, claim, quarantineReason, cause)
	}
	return s.blockUnrunnableJob(deps, job, claim, blockReason, cause)
}

// quarantineUnrunnableClaim is blockUnrunnableJob for a claim that was taken over an
// expired one.
//
// It blocks the Job and keeps the replacement claim — with its token and the capacity
// it holds — because the execution the claim replaced is unproven: it may be running
// the work in another process, and handing the claim back here would let the next
// Resume dispatch a second execution of it. The claim's own outcome, or a
// reconciliation that finds evidence the runtime is gone, is what resolves it.
func (s *Service) quarantineUnrunnableClaim(deps Deps, job models.Job, claim models.JobClaim, reason string, cause error) error {
	if _, err := s.quarantineClaim(deps, job, claim, reason, deps.now()); err != nil {
		return fmt.Errorf("%w (and the job could not be quarantined either: %v)", cause, err)
	}
	return cause
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
func newExecution(ctx context.Context, deps Deps, service *Service, job models.Job, claim models.JobClaim, access Access, input json.RawMessage, claimedFrom State) Execution {
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: claim.ExecutionToken}
	return Execution{
		JobID:          job.ID,
		Kind:           job.Kind,
		KindVersion:    job.KindVersion,
		Version:        job.Version,
		ExecutionToken: claim.ExecutionToken,
		Claimant:       claim.Claimant,
		ClaimedFrom:    claimedFrom,
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
// A *quarantined* claim whose token is still this execution's is deliberately not
// refused. The quarantine withholds the renewal, not the ownership: nobody could
// prove what became of the work, so no other executor may take it — a Resume is
// refused while an unresolved claim exists, and an expired scan never revisits a
// quarantined claim — and the execution holding the token is the one thing that can
// end it, by reporting its own outcome. Refusing here told a live executor it had
// been fenced, and an executor that obeys that instruction stops its work while its
// worker is still running: a capacity-queued transfer was abandoned mid-flight and
// its claim released, which is the duplicate dispatch the quarantine exists to
// prevent.
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
	if claim.State == models.JobClaimStateQuarantined && claim.ExecutionToken == ref.ExecutionToken {
		// Still this execution's, and nobody else may take it: there is nothing to
		// extend here, and nothing to report as lost.
		return nil
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

// OwnsQuarantinedJob reports whether one execution is the owner of a Job whose
// claim was quarantined: the Job is blocked, still carries this execution's token,
// and its claim row is the unresolved one. It is the question a runtime has to ask
// before it hands an execution's ownership back, because a quarantine is not the
// executor saying its work stopped — it is the admission that nobody could prove it
// did, and only the owning execution's own outcome resolves it.
func (s *Service) OwnsQuarantinedJob(deps Deps, ref ExecutionRef) (bool, error) {
	if err := validateExecutionRef(ref); err != nil {
		return false, err
	}
	if ref.ExecutionToken == "" {
		return false, nil
	}
	job, err := loadJob(deps.DB, ref.JobID)
	if err != nil {
		return false, err
	}
	if State(job.State) != StateBlocked || job.ExecutionToken != ref.ExecutionToken {
		return false, nil
	}
	claim, err := loadClaim(deps.DB, ref.JobID)
	if err != nil {
		return false, err
	}
	return claim.State == models.JobClaimStateQuarantined && claim.ExecutionToken == ref.ExecutionToken, nil
}

// ReleaseClaim ends one execution's ownership of a Job.
//
// It is the graceful half of the fence: an execution that stops — because its
// work ended, or because the runtime is shutting down and has quiesced the work
// — hands its claim back so the capacity it held is available again, rather than
// leaving a budget occupied until a lease expires. A release under a token that
// owns nothing is a no-op: releasing is idempotent, and the alternative would be
// a runtime that cannot safely report what it already reported.
//
// A Job that is still running is left in the nonterminal state the request names,
// in the same transaction that hands the claim back, and a request that names none
// is refused: releasing the ownership of a running Job without ending its running
// would leave work that nothing can claim and nothing reconciles. Where a Job that
// already left running belongs was the decision of whatever ended it, so only
// ownership changes here — which is the ordinary case, and the one every adapter
// that moves its own Job through the lifecycle leaves behind.
//
// A release never ends a Job. The state it may name is one the next process can
// pick the work up in, and an end state is refused rather than applied
// (ErrReleaseTerminalState): the terminal contract belongs to Finish — the failure
// taxonomy a failed Job records, and the required outputs a successful one is
// verified against — and Transition carries the same verification for the running
// -> succeeded it permits, so no path to success is left unguarded.
func (s *Service) ReleaseClaim(deps Deps, request ReleaseRequest) (Snapshot, error) {
	if err := validateReleaseRequest(request); err != nil {
		return Snapshot{}, err
	}

	job, err := loadJob(deps.DB, request.JobID)
	if err != nil {
		return Snapshot{}, err
	}

	// Only the execution that owns a running Job may end its running, so only that
	// request has to name the state to leave it in. A release under a token that owns
	// nothing is a no-op whatever state the Job is in: releasing is idempotent, and a
	// runtime must be able to report what it already reported.
	if State(job.State) == StateRunning && job.ExecutionToken == request.ExecutionToken && request.ExecutionToken != "" {
		if request.To == "" {
			return Snapshot{}, fmt.Errorf("%w: job %s", ErrReleaseNeedsState, job.ID)
		}
		// One write: the transition leaves running and hands the claim back
		// inside its own transaction, and it is what makes "the Job is in the state
		// the adapter decided" and "this execution no longer owns it" the same
		// instant rather than two.
		prepared, err := prepareTransition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: request.ExecutionToken,
			To: request.To,
		})
		if err != nil {
			return Snapshot{}, err
		}
		prepared.releaseReason = request.Reason
		return s.commitTransition(deps, prepared, nil)
	}

	now := deps.now()
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		return releaseClaimTx(tx, job.ID, request.ExecutionToken, request.Reason, now)
	})
	if err != nil {
		return Snapshot{}, err
	}

	released, err := loadJob(deps.DB, job.ID)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot(released), nil
}

// validateReleaseRequest checks and normalizes a release request.
func validateReleaseRequest(request ReleaseRequest) error {
	if err := validateExecutionRef(request.ExecutionRef); err != nil {
		return err
	}
	if strings.TrimSpace(request.Reason) == "" {
		return fmt.Errorf("%w: a release needs a reason", ErrInvalidClaim)
	}
	if len(request.Reason) > MaxReleaseReasonBytes {
		return fmt.Errorf("%w: release reason is %d bytes, over the %d-byte ceiling",
			ErrInvalidClaim, len(request.Reason), MaxReleaseReasonBytes)
	}
	if request.To != "" {
		if !request.To.Valid() {
			return fmt.Errorf("%w: %q", ErrUnknownState, request.To)
		}
		// Ending a Job is Finish's decision, whatever the Job's state is now: the
		// state is read only where an execution owns a running Job, and a request
		// that names an end state is the one thing that would apply a terminal
		// outcome without the validation and verification Finish carries.
		if request.To.Terminal() {
			return fmt.Errorf("%w: %q", ErrReleaseTerminalState, request.To)
		}
	}
	return nil
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
	// quarantineReasonInputUnavailable: the execution-required input cannot be
	// opened in this process, so no decision about the work could be made here. It
	// is not the same question as blockedReasonInputUnavailable, which is asked at
	// admission: there nothing is running yet, so the claim may be handed back,
	// while an expired claim may have a live execution behind it.
	quarantineReasonInputUnavailable = "input-unavailable"
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
// is — nothing is applied on its behalf, and it keeps its claim and the capacity
// that goes with it — but it is deferred to a later pass rather than to the next
// one, and the scan is ordered by when a claim is next due. A bounded pass that
// only ever read the oldest claims would otherwise let a Kind whose reconciler
// keeps failing occupy every batch, leaving the claims behind it — including ones
// an adapter could decide at once — unreconciled for as long as it failed. The
// pass is bounded, and it is a pass rather than a sweep because the caller polls
// it.
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

		// The input is opened before the adapter is asked, because a reconciler that
		// cannot produce it must not accept a decision made without it. A nil Input
		// already means "this Kind stores none", and an adapter reading it as "there
		// is nothing to decide with" answers the release it would answer for work it
		// had just decoded — which is how a process holding no key for the Job
		// released the claim, the token and the capacity of a worker that was still
		// running under the key it did not have.
		input, unreadable, inputErr := s.reconcileInput(deps, job)
		if inputErr != nil {
			// A database or infrastructure error is not evidence that the input is
			// absent, and it must not be converted into an adapter decision made with
			// nil input. Leave the held claim alone; its next bounded reconciliation
			// will retry the read.
			if decisionErr == nil {
				decisionErr = inputErr
			}
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}
		if unreadable {
			snap, err := s.quarantineClaim(deps, job, claim, quarantineReasonInputUnavailable, now)
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

		decision, err := adapter.Reconcile(ctx, s.reconcileRequest(ctx, deps, job, claim, access, input))
		if err != nil {
			// An adapter that could not answer has not decided anything, and
			// nothing may be applied on its behalf: an error here is a
			// reconciler that could not do its job, not a Job that failed. The
			// claim is left where it is and deferred to a later pass rather than
			// retried by the next one.
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}

		if !validReconcileDecision(decision) {
			if decisionErr == nil {
				decisionErr = fmt.Errorf("%w: %s v%d answered %q",
					ErrInvalidReconcileDecision, claim.Kind, claim.KindVersion, decision)
			}
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
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
			// Claimed running: a resume hands the *same* execution back under a fresh
			// token rather than admitting work that was waiting.
			execution, err := s.executionFor(ctx, deps, resumedJob, resumedClaim, StateRunning, claimFromExpired)
			if err != nil {
				// The resumed execution could not be built — its input is not
				// readable in this process — and executionFor has blocked the Job
				// and quarantined the replacement claim for it. It is deliberately
				// not handed back: it was taken over an expired one, so the
				// execution it replaced may still be running this work, and a
				// released claim is a Resume that dispatches a second copy.
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

// ReconcileUnrunnable blocks pending work this process has no adapter for.
//
// Reconciliation proper scans held claims, so work that is waiting rather than
// owned is in none of its batches: a Job accepted under a Kind that has since been
// removed, renamed or disabled at this deployment has no claim to expire and no
// registration to be visited through, and would otherwise stay queued (or
// scheduled) forever — not running, owned by nobody, and unrunnable by every
// executor here. §2/§3's rule for that work is the same one reconciliation
// applies to an expired claim no adapter can explain: it becomes `blocked`, which
// is where a person can see it, and never handed to a different executor.
//
// It takes no execution capacity and writes no claim, because nothing is
// executing: the only durable effect is the Job's own state and event, committed
// together by the same transition every other host-side change goes through. Like
// every pass here it is bounded, idempotent and driven by the caller's cadence —
// blocked work is not pending, so the next pass does not see it again.
func (s *Service) ReconcileUnrunnable(deps Deps, limit int) (ReconcileReport, error) {
	if limit <= 0 {
		limit = DefaultReconcileBatch
	}

	jobs, err := unclaimedPendingJobs(deps.DB, s.Registrations(), limit)
	if err != nil {
		return ReconcileReport{}, err
	}
	if len(jobs) == 0 {
		return ReconcileReport{}, nil
	}

	detail, err := json.Marshal(map[string]string{"reason": blockedReasonAdapterMissing})
	if err != nil {
		return ReconcileReport{}, fmt.Errorf("jobs: encode unrunnable detail: %w", err)
	}

	report := ReconcileReport{Examined: len(jobs)}
	for _, job := range jobs {
		snap, err := s.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: StateBlocked,
			Event: EventInput{Type: EventBlocked, Detail: detail},
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrVersionConflict), errors.Is(err, ErrIllegalTransition), errors.Is(err, ErrStaleExecution):
				// The Job moved, or was claimed and started, between the read above
				// and this write. Whatever it is now, it is not work this pass may
				// decide about.
				continue
			default:
				return report, err
			}
		}
		report.Outcomes = append(report.Outcomes, ReconcileOutcome{
			JobID: job.ID, Decision: ReconcileBlock, Snapshot: snap,
		})
	}
	return report, nil
}

// blockedReasonAdapterMissing is the reason a Job the control plane itself blocks
// records when this process has no executor for its Kind.
const blockedReasonAdapterMissing = "adapter-missing"

// unclaimedPendingJobs reads the pending Jobs of Kinds no adapter is registered
// for, oldest first, in a bounded batch.
//
// The registered pairs are excluded by the query rather than filtered afterwards,
// because a page of runnable work must not be what stands between this pass and
// the work it exists for: the backlog of a Kind this process *can* run is
// unbounded, and reading it every tick would never reach the rest.
func unclaimedPendingJobs(db *gorm.DB, registrations []AdapterRegistration, limit int) ([]models.Job, error) {
	query := db.Where("(execution_token IS NULL OR execution_token = '')").
		Where("state IN ?", []string{string(StateQueued), string(StateScheduled)})
	if len(registrations) > 0 {
		clauses := make([]string, 0, len(registrations))
		args := make([]any, 0, len(registrations)*2)
		for _, registration := range registrations {
			clauses = append(clauses, "(kind = ? AND kind_version = ?)")
			args = append(args, registration.Definition.Kind, registration.Definition.KindVersion)
		}
		query = query.Where("NOT ("+strings.Join(clauses, " OR ")+")", args...)
	}

	var jobs []models.Job
	if err := query.Order("accepted_at, jobs.id").Limit(limit).Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("jobs: read pending work no adapter can run: %w", err)
	}
	return jobs, nil
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

// ReconcileQuarantined asks each quarantined claim's adapter again, so that a
// quarantine can consume evidence that appeared after it was taken.
//
// A quarantined claim keeps its Job's token and the capacity that admitted it
// precisely because nobody could prove the external work had stopped, and §3 forbids
// releasing any of it on that silence. The proof can arrive later, and it does not
// arrive by itself: the runtime that owned the work can be proved gone (this host
// rebooted since, or the process no longer exists), or the Kind's own durable
// evidence can turn up without it (a Reduction whose row is ready again, an archive
// at the published path, a plan restored to its unconsumed name). Both are answers
// the adapter gives, so this pass asks it again and applies what it answers.
//
// It is deliberately not part of ReconcileExpired. A quarantined claim is not expired
// work waiting for a decision — it is work that was *already* decided as far as
// anyone here could — and rescanning it at that cadence would turn an unresolvable
// Job into a reconciliation loop. It has its own due schedule (the same widening
// backoff an undecidable claim gets) and its own vocabulary: only a decision that
// *resolves* the quarantine is applied, so "nothing has changed" leaves the claim and
// its capacity exactly where they are and no amount of time releases unproven work.
// A quarantine with no resolution at all is therefore not a leak of the *decision*
// but of the proof — and the one proof that always eventually arrives is the owning
// process being gone, which is what makes this pass the difference between a
// deployment that recovers a dead runtime's capacity and one that never does.
func (s *Service) ReconcileQuarantined(ctx context.Context, deps Deps, claimant string, limit int) (ReconcileReport, error) {
	if strings.TrimSpace(claimant) == "" {
		return ReconcileReport{}, fmt.Errorf("%w: a reconciliation pass needs a claimant", ErrInvalidClaim)
	}
	if limit <= 0 {
		limit = DefaultReconcileBatch
	}

	now := deps.now()
	claims, err := quarantinedClaims(deps.DB, now, limit)
	if err != nil {
		return ReconcileReport{}, err
	}
	if len(claims) == 0 {
		return ReconcileReport{}, nil
	}

	report := ReconcileReport{Examined: len(claims)}
	var decisionErr error
	for _, claim := range claims {
		job, err := loadJob(deps.DB, claim.JobID)
		if err != nil {
			return report, err
		}
		if State(job.State) != StateBlocked || job.ExecutionToken != claim.ExecutionToken {
			// The quarantine was settled by its own execution, replaced, or the Job
			// moved on while this pass was reading: there is nothing here to ask
			// about any more.
			continue
		}

		access, accessErr := executionAccess(job)
		if accessErr != nil {
			// The principal the work acts as is gone. No adapter answer could be
			// carried out — every one of them would run the work as somebody the
			// deleted account was not — so the quarantine stays and this pass asks
			// again later rather than releasing work nobody has proved stopped.
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}

		adapter, definition, err := s.adapterFor(claim.Kind, claim.KindVersion)
		if err != nil {
			// No executor here can prove anything about this Kind's work, which is
			// often *why* it was quarantined: a Kind registered in a process that
			// starts later is exactly the evidence this pass exists for.
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}

		// Best effort, and deliberately so: the input is opened for an adapter that
		// wants it, but a quarantine exists because something could not be decided,
		// and the evidence that resolves it — a runtime that is provably gone, a
		// durable artifact the Kind can see — does not depend on this process being
		// able to decode the input. An adapter handed none must answer from that
		// evidence rather than from the input it was not given.
		input, unreadable, inputErr := s.reconcileInput(deps, job)
		if inputErr != nil {
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}
		if unreadable {
			input = nil
		}
		decision, err := adapter.Reconcile(ctx, s.reconcileRequest(ctx, deps, job, claim, access, input))
		if err != nil {
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}
		if !validReconcileDecision(decision) {
			if decisionErr == nil {
				decisionErr = fmt.Errorf("%w: %s v%d answered %q",
					ErrInvalidReconcileDecision, claim.Kind, claim.KindVersion, decision)
			}
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}
		if !definition.Restorable && (decision == ReconcileResume || decision == ReconcileQueue) {
			// Closure-backed work is never redispatched after runtime loss, however
			// it is asked, and "queue" is a redispatch.
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
			continue
		}
		if !resolvesQuarantine(decision) {
			// The adapter answered that nothing has changed: it still cannot prove
			// the work stopped, or it sees the execution still running. The
			// quarantine stays exactly where it is.
			if err := deferClaimReconcile(deps, claim, now); err != nil {
				return report, err
			}
			report.Deferred++
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

// resolvesQuarantine reports whether a decision settles a quarantined claim, and it
// is deliberately narrower than "a decision was applied".
//
// Every answer that moves the Job on — back to the queue, or to an outcome — is the
// adapter saying that the work is not running and what should happen to it. The rest
// are the answers that mean "still nothing proved": a resume or a remain-running
// would keep ownership where it is (and the first would hand a blocked Job to a fresh
// execution), an unproven answer is the same silence in a different word, and a block
// is the state the Job is already in.
func resolvesQuarantine(decision ReconcileDecision) bool {
	switch decision {
	case ReconcileQueue, ReconcileSucceed, ReconcileFail, ReconcileInterrupt:
		return true
	default:
		return false
	}
}

// quarantinedClaims reads the quarantined claims whose own re-ask is due, soonest
// first, in a bounded batch. The schedule is the same one an undecidable claim's
// deferral uses, so a quarantine nothing can resolve is retried at a bounded
// interval rather than on every tick.
func quarantinedClaims(db *gorm.DB, now time.Time, limit int) ([]models.JobClaim, error) {
	var claims []models.JobClaim
	if err := db.Where("state = ?", models.JobClaimStateQuarantined).
		Where("(next_reconcile_at IS NULL OR next_reconcile_at <= ?)", now).
		Order("COALESCE(next_reconcile_at, lease_expires_at), job_id").Limit(limit).Find(&claims).Error; err != nil {
		return nil, fmt.Errorf("jobs: read quarantined claims: %w", err)
	}
	return claims, nil
}

// expiredClaims reads the held claims whose lease has run out and whose own
// reconciliation schedule is due, soonest first, in a bounded batch. A quarantined
// claim is deliberately not here: it was already reconciled as far as anyone
// could, and rescanning it would turn an unresolvable Job into a reconciliation
// loop.
//
// The order is when each claim is next due, which is its lease expiry until a pass
// has deferred it: a claim nothing could decide sinks behind every claim whose own
// lease has expired and that no pass has asked about yet, so a failing Kind cannot
// hold the batch against work it has never been asked about.
func expiredClaims(db *gorm.DB, now time.Time, limit int) ([]models.JobClaim, error) {
	var claims []models.JobClaim
	if err := db.Where("state = ? AND lease_expires_at <= ?", models.JobClaimStateHeld, now).
		Where("(next_reconcile_at IS NULL OR next_reconcile_at <= ?)", now).
		Order("COALESCE(next_reconcile_at, lease_expires_at), job_id").Limit(limit).Find(&claims).Error; err != nil {
		return nil, fmt.Errorf("jobs: read expired claims: %w", err)
	}
	return claims, nil
}

// deferClaimReconcile schedules the next pass that may ask about a claim a
// reconciliation pass could not decide.
//
// It writes nothing else. The claim keeps its state, its token, its lease and the
// capacity it holds: an adapter that did not answer has proved nothing about the
// external work, and releasing any of it on that silence is how duplicate side
// effects happen. The wait widens with each undecided attempt, so a Kind whose
// reconciler is broken is still retried and still fails to hold up the claims
// behind it.
//
// The write is guarded by the token and by the state the claim was read in, so a
// claim that was resumed, released or settled while the pass was running is not
// deferred by mistake — there is nothing left to defer about it.
func deferClaimReconcile(deps Deps, claim models.JobClaim, at time.Time) error {
	result := deps.DB.Model(&models.JobClaim{}).
		Where("job_id = ? AND state = ? AND execution_token = ?",
			claim.JobID, claim.State, claim.ExecutionToken).
		Updates(map[string]any{
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"next_reconcile_at":  at.Add(reconcileRetryDelay(claim.ReconcileAttempts + 1)),
			"updated_at":         at,
		})
	if result.Error != nil {
		return fmt.Errorf("jobs: defer reconciliation of %s: %w", claim.JobID, result.Error)
	}
	return nil
}

// reconcileRetryDelay is how long a claim waits after an attempt that decided
// nothing. It doubles from DefaultReconcileRetry and stops at MaxReconcileRetry,
// which is the same shape the download queue's backoff uses: the first deferral is
// short enough to recover a transient adapter failure quickly, and a reconciler
// that never answers is retried forever at a bounded interval rather than
// abandoned.
func reconcileRetryDelay(attempts uint) time.Duration {
	delay := DefaultReconcileRetry
	for attempt := uint(1); attempt < attempts && delay < MaxReconcileRetry; attempt++ {
		delay *= 2
	}
	if delay > MaxReconcileRetry {
		return MaxReconcileRetry
	}
	return delay
}

// reconcileInput opens the input a reconciliation of one claim needs, and reports
// whether the Job's execution-required input is unavailable in this process.
//
// The distinction is the whole reason this is not executionInput: a Kind that stores
// no input at all — an explicitly non-replayable declaration — has nothing to open
// and nothing to prove, while a Job whose own class says its input is replayable and
// which cannot produce it here is *undecided*. An open that fails for any other
// reason is reported as "not unavailable" for the same reason executionInput
// refuses it: the caller must see the failure rather than a Job run without it.
func (s *Service) reconcileInput(deps Deps, job models.Job) (json.RawMessage, bool, error) {
	input, err := s.executionInput(deps, job)
	if err == nil {
		return input, false, nil
	}
	if ReplayBlocked(State(job.State), err) {
		return nil, true, nil
	}
	return nil, false, err
}

// reconcileRequest builds what one adapter is told about an expired claim. The input
// is what reconciliation opened for it, and it is nil when the Kind stores none: a
// reconciler that could not open execution-required input never asks an adapter at
// all (see ReconcileExpired), so a nil Input here is never a decision made blind.
func (s *Service) reconcileRequest(ctx context.Context, deps Deps, job models.Job, claim models.JobClaim, access Access, input json.RawMessage) ReconcileRequest {
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
