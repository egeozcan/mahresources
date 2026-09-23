package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file holds the command surface: which controls a Job offers, what running
// one means, and how a repeat of the same request is answered without running it
// twice.
//
// Two facts shape all of it. The first is that a command is advertised by the
// Job rather than derived by a client from its Kind and state, so a Kind adapter
// answers for its own work and the host answers for the bookkeeping it owns —
// dismissal, pinning, forgetting sealed input, and the retry lineage. The second
// is that the host's answer may only ever be narrower than the adapter's: an
// adapter can say its work does not support a cancellation, and the host can say
// a cancellation cannot be honored on a Job that has already finished, but
// neither can turn a command into something the other refused.
//
// Running a command is one transaction per Job: the caller's request is claimed
// by its idempotency tuple, every durable precondition is rechecked against the
// row the transaction holds, and the effect commits with the record of it. Only
// the step that has to reach outside the database — telling an executor — happens
// between two transactions, and the record written before it is what stops a
// repeat from telling it twice.

// hostCommandKeys lists the keys the Service executes itself — the bookkeeping
// only the control plane can do: a viewer's own preferences, a Job's sealed
// input, and the lineage a re-run creates.
var hostCommandKeys = []string{
	CommandDismiss, CommandPin, CommandPinLineage, CommandForget, CommandRetry, CommandRepeat,
}

// hostOnlyCommandKeys lists the host-executed keys whose *advertisement* the host
// also answers for by itself. An adapter's answer for one of them is ignored
// rather than believed: the host's own conditions decide whether dismissal,
// pinning or forgetting can be honored, and a Kind has no authority over a
// viewer's preferences or another Job's sealed input.
//
// Retry and Repeat are deliberately not here. Whether a Kind's work may be re-run
// at all is the Kind's policy and its adapter answers it; whether the lineage and
// the sealed input allow it right now is the host's, which narrows that answer.
var hostOnlyCommandKeys = []string{
	CommandDismiss, CommandPin, CommandPinLineage, CommandForget,
}

// isHostCommandKey reports whether the host implements one key.
func isHostCommandKey(key string) bool {
	return containsCommandKey(hostCommandKeys, key)
}

// isHostOnlyCommandKey reports whether the host alone answers for one key.
func isHostOnlyCommandKey(key string) bool {
	return containsCommandKey(hostOnlyCommandKeys, key)
}

// containsCommandKey is the one membership test the two vocabularies share.
func containsCommandKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

// AdvertisedCommands reports the controls one visible Job offers the asker right
// now. A Job the asker may not see is answered exactly as one that does not
// exist, so an unauthorized probe cannot use the command surface to learn that a
// Job is there.
func (s *Service) AdvertisedCommands(ctx context.Context, deps Deps, access Access, jobID string) ([]Command, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("%w: no database handle", ErrInvalidCommand)
	}
	job, err := loadVisibleJob(deps.DB, access, jobID)
	if err != nil {
		return nil, err
	}
	return s.advertisedCommands(ctx, deps, access, job)
}

// loadVisibleJob reads one Job through the one shared visibility predicate.
// Every read that answers a principal goes through it, so "may this viewer see
// this Job" has one spelling rather than one per entry point.
func loadVisibleJob(db *gorm.DB, access Access, jobID string) (models.Job, error) {
	if strings.TrimSpace(jobID) == "" {
		return models.Job{}, fmt.Errorf("%w: empty job id", ErrNotFound)
	}
	var job models.Job
	err := jobQuery(db, access).Where("jobs.id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return models.Job{}, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return models.Job{}, fmt.Errorf("jobs: load visible job: %w", err)
	}
	return job, nil
}

// advertisedCommands is the one implementation of "what does this Job offer this
// principal": the adapter's answer, narrowed by the host-owned facts, plus the
// host's own commands. It is used both by the read above and by the execution
// path's recheck, so an advertisement and a command's execution cannot drift
// apart about which commands exist.
//
// A Kind this process has no adapter for contributes nothing rather than falling
// back to another executor: this process cannot run its work, so it cannot say
// what the work supports either. The host's own bookkeeping is unaffected — none
// of it needs an executor.
func (s *Service) advertisedCommands(ctx context.Context, deps Deps, access Access, job models.Job) ([]Command, error) {
	commands := make([]Command, 0, 8)
	seen := make(map[string]bool, 8)

	if adapter, _, err := s.adapterFor(job.Kind, job.KindVersion); err == nil {
		offered, advertiseErr := adapter.Commands(ctx, CommandContext{
			Snapshot: viewerSnapshot(job, access),
			Access:   access,
			// The handle this advertisement is computed on, so an adapter whose answer
			// needs a read reads it here rather than on a handle of its own: the
			// recheck inside a command's transaction runs with the *transaction's*
			// handle, and a second one would deadlock a one-connection pool.
			Deps: deps,
		})
		if advertiseErr != nil {
			// The adapter's own text is deliberately not carried: only the Kind
			// knows what in it is safe, and this error reaches a caller's page.
			return nil, fmt.Errorf("%w: %s v%d could not answer which commands its job offers",
				ErrInvalidCommand, job.Kind, job.KindVersion)
		}
		for _, command := range offered {
			if err := validateAdapterCommand(command); err != nil {
				return nil, err
			}
			if isHostOnlyCommandKey(command.Key) {
				// The host's own keys are decided below and nowhere else.
				continue
			}
			if seen[command.Key] {
				return nil, fmt.Errorf("%w: %s v%d advertised %s twice",
					ErrInvalidCommand, job.Kind, job.KindVersion, command.Key)
			}
			seen[command.Key] = true

			honorable, err := s.commandHonorable(deps, job, command.Key)
			if err != nil {
				return nil, err
			}
			if !honorable {
				continue
			}
			commands = append(commands, presentCommand(job, command))
		}
	}

	for _, command := range s.hostCommands(deps, access, job) {
		if seen[command.Key] {
			continue
		}
		seen[command.Key] = true
		commands = append(commands, command)
	}
	return commands, nil
}

// presentCommand stamps the host's own facts onto one adapter's answer: the
// version the advertisement was computed from and the canonical endpoint it is
// run through. Both belong to the Job rather than to the Kind — a client calls
// the Job's route — so an adapter that names either is corrected here rather than
// believed.
func presentCommand(job models.Job, command Command) Command {
	command.JobVersion = job.Version
	command.Endpoint = commandEndpoint(job.ID, command.Key)
	return command
}

// commandEndpoint is the canonical route for one Job's command. It is built in
// one place so every advertisement agrees about where a command is run.
func commandEndpoint(jobID, key string) string {
	return "/v1/jobs/" + jobID + "/commands/" + key
}

// commandHonorable reports whether the host can honor one command the adapter
// advertised. The host narrows only where it owns a durable fact the adapter
// cannot see: a finished Job offers no cancellation or pause, its retry lineage
// is what decides whether a Retry is possible, and a cancellation that has
// already won refuses a pause and a resume alike.
//
// A key the host knows nothing about is left to the adapter, which is the only
// thing that can answer for it. Narrowing is all the host does: nothing here can
// make a command appear that the adapter did not offer.
func (s *Service) commandHonorable(deps Deps, job models.Job, key string) (bool, error) {
	state := State(job.State)
	switch key {
	case CommandCancel:
		return !state.Terminal(), nil
	case CommandPause:
		// A pause is confirmed by the execution that reaches the checkpoint, so a Job
		// nothing is running has nobody to reach one: §1's table defines paused as an
		// executor's confirmation, and offering it for waiting work would be a hold
		// nobody agreed to. A cancellation that already won refuses it too.
		return state == StateRunning && job.ControlIntent != ControlIntentCancel, nil
	case CommandResume:
		// Held work returns to the queue only while a cancellation has not won it: a
		// Job a cancellation owns ends cancelled, and handing its work back to the
		// queue would be resuming work that can never publish a success again.
		if (state != StatePaused && state != StateBlocked) || job.ControlIntent == ControlIntentCancel {
			return false, nil
		}
		// And only while nothing unresolved still owns the work. Returning a Job to
		// the queue releases whatever claim its token names — a quarantine included —
		// and §3 permits that release only once the owning runtime has proved the
		// external work quiescent. A quarantine is the statement that nobody could
		// prove it, so an ordinary Resume would admit a second execution of work that
		// may still be running. The proof is the owning execution finishing the Job,
		// not a person asking for it.
		unresolved, err := s.unresolvedClaim(deps, job.ID)
		if err != nil {
			return false, err
		}
		return !unresolved, nil
	case CommandRetry:
		return s.retryableLeaf(deps, job)
	case CommandRepeat:
		return state == StateSucceeded && s.commandReplayAvailable(deps, job), nil
	default:
		return true, nil
	}
}

// unresolvedClaim reports whether a Job is still owned by a claim nobody resolved:
// one that is held, or one that was quarantined because its execution could not be
// proved quiescent.
//
// It answers for the Job's ownership rather than for its state, because the two can
// disagree: a quarantined Job is blocked, and reading that state alone says nothing
// about whether the process that started its work is still running. That is the
// question every attempt to release the claim has to ask first.
func (s *Service) unresolvedClaim(deps Deps, jobID string) (bool, error) {
	return unresolvedClaimOn(deps.DB, jobID)
}

// unresolvedClaimOn is unresolvedClaim on a caller's own handle, so a recheck inside
// a command's transaction reads the rows that transaction will write.
func unresolvedClaimOn(db *gorm.DB, jobID string) (bool, error) {
	var count int64
	if err := db.Model(&models.JobClaim{}).
		Where("job_id = ? AND state IN ?", jobID, unresolvedClaimStates()).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("jobs: read claim state for %s: %w", jobID, err)
	}
	return count > 0, nil
}

// retryableLeaf reports whether this Job is the quiescent leaf a Retry may branch
// from: unsuccessful, finished, with input this process can still open, and with
// no successor already recorded.
//
// The successor check is what makes "the current leaf" mean one thing. Retry
// lineage is linear by default, so a Job that already has a Retry successor is
// not the leaf any more — and while that successor is still active there is no
// leaf to retry at all, which is exactly §4's "at most one active recovery Job
// exists in the chain".
func (s *Service) retryableLeaf(deps Deps, job models.Job) (bool, error) {
	switch State(job.State) {
	case StateFailed, StateCancelled, StateInterrupted:
	default:
		return false, nil
	}
	if !s.commandReplayAvailable(deps, job) {
		return false, nil
	}
	successors, err := retrySuccessors(deps.DB, job.ID)
	if err != nil {
		return false, err
	}
	return len(successors) == 0, nil
}

// retrySuccessors lists the Jobs that record jobID as the ancestor of a Retry.
// FromJobID is the successor for every lineage relation, so the ancestors of a
// Job are the links it is the FROM endpoint of.
func retrySuccessors(db *gorm.DB, jobID string) ([]string, error) {
	var ids []string
	err := db.Model(&models.JobLink{}).
		Where("type = ? AND to_job_id = ?", string(LinkRetryOf), jobID).
		Order("from_job_id").Pluck("from_job_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: read retry successors: %w", err)
	}
	return ids, nil
}

// commandReplayAvailable reports whether this process can open one Job's sealed
// input right now — which is the precondition every command that copies input
// has, and one of the two facts a Retry's advertisement turns on.
func (s *Service) commandReplayAvailable(deps Deps, job models.Job) bool {
	if ReplayClass(job.ReplayClass) != ReplayClassReplayable {
		return false
	}
	return s.replayAvailabilityOf(deps.DB, replayKeys(deps), job, deps.now()) == ReplayAvailable
}

// hostCommands is the vocabulary the host offers without asking any Kind: a
// viewer's own dismissal and pinning, pinning a whole visible lineage, and
// forgetting sealed input.
//
// Each is offered only where it can be honored. Dismissal is for finished work —
// it is the "dismiss finished" operation, not a way to hide something that is
// still running — and the preference commands need a viewer for the row to
// belong to, which is why an administrator acting with no account of their own is
// offered neither.
func (s *Service) hostCommands(deps Deps, access Access, job models.Job) []Command {
	commands := make([]Command, 0, 4)
	terminal := State(job.State).Terminal()

	if access.UserID != 0 {
		if terminal {
			commands = append(commands, hostCommand(job, CommandDismiss, "Dismiss", false, true, ""))
		}
		commands = append(commands,
			hostCommand(job, CommandPin, "Pin", false, true, ""),
			hostCommand(job, CommandPinLineage, "Pin visible lineage", false, false, ""),
		)
	}
	if terminal && s.commandReplayAvailable(deps, job) {
		commands = append(commands, hostCommand(job, CommandForget, "Forget replay input", true, false,
			"Forget this job's replay input? Retry and Repeat will no longer be possible."))
	}
	return commands
}

// hostCommand builds one host-owned command declaration.
func hostCommand(job models.Job, key, label string, destructive, bulk bool, confirmation string) Command {
	return presentCommand(job, Command{
		Key:          key,
		Label:        label,
		Destructive:  destructive,
		Bulk:         bulk,
		Confirmation: confirmation,
	})
}

// validateAdapterCommand checks one command a Kind advertised. A malformed
// answer is refused rather than published: the command surface is what a client
// uses to decide what it may safely offer a person, and a keyless or oversized
// entry is a bug the host must not pass through as if it were a control.
func validateAdapterCommand(command Command) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidCommand, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(command.Key) == "" {
		return invalid("a command needs a key")
	}
	if len(command.Key) > MaxCommandKeyBytes {
		return invalid("command key is %d bytes, over the %d-byte ceiling", len(command.Key), MaxCommandKeyBytes)
	}
	if len(command.Label) > MaxCommandLabelBytes {
		return invalid("command label is %d bytes, over the %d-byte ceiling", len(command.Label), MaxCommandLabelBytes)
	}
	if len(command.Confirmation) > MaxCommandConfirmationBytes {
		return invalid("command confirmation is %d bytes, over the %d-byte ceiling", len(command.Confirmation), MaxCommandConfirmationBytes)
	}
	if len(command.Presentation) > 0 {
		if len(command.Presentation) > MaxCommandPresentationBytes {
			return invalid("command presentation is %d bytes, over the %d-byte ceiling",
				len(command.Presentation), MaxCommandPresentationBytes)
		}
		if !json.Valid(command.Presentation) {
			return invalid("command presentation is not valid JSON")
		}
	}
	return nil
}

// ExecuteCommand runs one advertised command against one visible Job, at most
// once per (Job, command, actor, idempotency key).
//
// Every answer a command depends on is rechecked against the Job the transaction
// holds, rather than trusted from the read that advertised it: the Job may have
// moved between a page render and a click, and a command that acted on a stale
// decision would be a control applied to work nobody looked at. The refusals are
// deliberately shaped like the other lifecycle refusals — nothing written, a
// typed error, and the fresh snapshot where a caller has to re-decide.
func (s *Service) ExecuteCommand(ctx context.Context, deps Deps, request CommandRequest) (CommandResult, error) {
	if deps.DB == nil {
		return CommandResult{}, fmt.Errorf("%w: no database handle", ErrInvalidCommand)
	}
	if err := validateCommandRequest(&request); err != nil {
		return CommandResult{}, err
	}

	job, err := loadVisibleJob(deps.DB, request.Actor, request.JobID)
	if err != nil {
		return CommandResult{}, err
	}
	// A recorded outcome is the answer a repeat gets, and it is looked up before the
	// advertisement: a command that succeeded has already changed what the Job
	// offers — a Retry that created a successor leaves the retried Job no longer
	// advertising Retry — and a repeat of the request that did that has to be
	// answered with what it recorded rather than refused because the command is gone.
	// The Job's visibility is still the first question, so a repeat cannot be used to
	// learn that a hidden Job is there.
	recorded, err := s.recordedCommandOutcome(deps, request)
	if err != nil {
		return CommandResult{}, err
	}
	if recorded != nil {
		return s.replayCommandResult(deps, request, *recorded)
	}

	commands, err := s.advertisedCommands(ctx, deps, request.Actor, job)
	if err != nil {
		return CommandResult{}, err
	}
	if !offersCommand(commands, request.Key) {
		return refusedResult(job, request, CommandCodeNotAdvertised, "the job does not offer that command",
			fmt.Errorf("%w: job %s does not offer %s", ErrCommandNotAdvertised, job.ID, request.Key))
	}
	// The version the caller decided from is the whole of the staleness check: every
	// durable change to a Job — a transition, a control intent, a claim — moves it,
	// so a request that names the version it read is refused the moment anything it
	// may have decided from has changed.
	if request.ExpectedVersion != job.Version {
		return refusedResult(job, request, CommandCodeConflict, "the job changed since this command was prepared",
			fmt.Errorf("%w: job %s is at version %d, the request expected %d",
				ErrVersionConflict, job.ID, job.Version, request.ExpectedVersion))
	}

	// The host's own keys are dispatched here and nowhere else: a key it does not
	// own goes to the Kind's adapter, and a key it owns never does. A Job's lineage
	// is the control plane's bookkeeping, so a re-run is this service's work.
	if isHostCommandKey(request.Key) {
		if request.Key == CommandRetry || request.Key == CommandRepeat {
			return s.executeLineageCommand(ctx, deps, request)
		}
		return s.executeHostCommand(ctx, deps, request)
	}
	return s.executeWorkloadCommand(ctx, deps, request)
}

// recordedCommandOutcome finds the outcome already recorded for one command
// request's idempotency tuple, or nil when this request has not been made before.
func (s *Service) recordedCommandOutcome(deps Deps, request CommandRequest) (*models.JobCommandRequest, error) {
	var row models.JobCommandRequest
	err := deps.DB.Where("job_id = ? AND command_key = ? AND actor_user_id = ? AND idempotency_key = ?",
		request.JobID, request.Key, request.Actor.UserID, request.IdempotencyKey).First(&row).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("jobs: read command request: %w", err)
	}
	return &row, nil
}

// offersCommand reports whether an advertisement contains one key.
func offersCommand(commands []Command, key string) bool {
	for _, command := range commands {
		if command.Key == key {
			return true
		}
	}
	return false
}

// refusedResult is one command's refusal, carrying the Job as the asker may see
// it so a caller that lost a race is shown what it lost to.
func refusedResult(job models.Job, request CommandRequest, code, message string, err error) (CommandResult, error) {
	return CommandResult{
		JobID: job.ID, Key: request.Key,
		Status: CommandStatusFailed, Code: code, Message: message,
		Job: viewerSnapshot(job, request.Actor),
	}, err
}

// validateCommandRequest checks a command request before anything is read.
func validateCommandRequest(request *CommandRequest) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidCommand, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(request.JobID) == "" {
		return invalid("a command needs a job id")
	}
	if strings.TrimSpace(request.Key) == "" {
		return invalid("a command needs a key")
	}
	if len(request.Key) > MaxCommandKeyBytes {
		return invalid("command key is %d bytes, over the %d-byte ceiling", len(request.Key), MaxCommandKeyBytes)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return invalid("a command needs an idempotency key")
	}
	if len(request.IdempotencyKey) > MaxIdempotencyKeyBytes {
		return invalid("idempotency key is %d bytes, over the %d-byte ceiling",
			len(request.IdempotencyKey), MaxIdempotencyKeyBytes)
	}
	if request.ExpectedVersion == 0 {
		return invalid("expected version must be set")
	}
	if len(request.Origin) > MaxOriginBytes {
		return invalid("origin is %d bytes, over the %d-byte ceiling", len(request.Origin), MaxOriginBytes)
	}
	return nil
}

// commandOrigin is what a successor Job records as its origin: the surface the
// command came from when the caller named one, and otherwise the origin of the
// work being continued — the closest durable fact available to a request that
// did not say where it came from.
func commandOrigin(request CommandRequest, job models.Job) string {
	if origin := strings.TrimSpace(request.Origin); origin != "" {
		return origin
	}
	return job.Origin
}

// commandOutcome is one command's outcome in the form its row is written with.
type commandOutcome struct {
	status      string
	code        string
	message     string
	detail      json.RawMessage
	successorID string
}

// appliedOutcome is the outcome of a command whose durable effect is complete.
func appliedOutcome(message string, detail json.RawMessage) commandOutcome {
	return commandOutcome{status: models.JobCommandStatusSucceeded, code: CommandCodeApplied, message: message, detail: detail}
}

// completeCommandRequest records the outcome one command reached. The update is
// guarded on the request still being the running claim it was when it was made,
// so a record somebody else completed is never overwritten with a second answer.
func completeCommandRequest(tx *gorm.DB, id string, outcome commandOutcome, now time.Time) error {
	result := tx.Model(&models.JobCommandRequest{}).
		Where("id = ? AND status = ?", id, models.JobCommandStatusRunning).
		Updates(map[string]any{
			"status":           outcome.status,
			"code":             outcome.code,
			"message":          outcome.message,
			"detail":           types.JSON(outcome.detail),
			"successor_job_id": outcome.successorID,
			"completed_at":     now,
		})
	if result.Error != nil {
		return fmt.Errorf("jobs: record command outcome: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: command request %s changed while its outcome was being recorded",
			ErrVersionConflict, id)
	}
	return nil
}

// newCommandClaim builds the idempotency row one command request is recorded
// under.
func newCommandClaim(request CommandRequest, now time.Time) models.JobCommandRequest {
	return models.JobCommandRequest{
		ID:             types.NewUUIDv7(),
		JobID:          request.JobID,
		CommandKey:     request.Key,
		ActorUserID:    request.Actor.UserID,
		IdempotencyKey: request.IdempotencyKey,
		RequestHash:    commandRequestHash(request),
		Status:         models.JobCommandStatusRunning,
		CreatedAt:      now,
	}
}

// commandRequestHash fingerprints what one idempotency key was used for. It is
// what turns "the same key" into "the same request": a key reused for a
// different surface or origin is refused rather than answered with the outcome of
// the request that got there first.
//
// The expected version is deliberately not part of it. A caller that repeats a
// request with a fresher version after a conflict is repeating the same intent —
// and the conflict wrote nothing to record anyway — while a key reused for a
// different command or origin is a different request this table has no outcome
// for.
func commandRequestHash(request CommandRequest) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		request.JobID,
		request.Key,
		strconv.FormatUint(uint64(request.Actor.UserID), 10),
		strings.TrimSpace(request.Origin),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// claimCommandRequest writes the idempotency row and reports whether this caller
// created it, together with the row as it stands when it did not.
//
// The insert comes first and is the caller's transaction's first statement, which
// is what serializes two identical requests on both engines: SQLite takes the
// writer lock before reading anything, and the unique tuple is what makes the
// loser's insert a no-op on PostgreSQL. Reading the row back is how the loser
// finds the outcome it must be answered with.
func claimCommandRequest(tx *gorm.DB, claim models.JobCommandRequest) (*models.JobCommandRequest, bool, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim).Error; err != nil {
		return nil, false, fmt.Errorf("jobs: claim command request: %w", err)
	}
	var stored models.JobCommandRequest
	if err := tx.Where("job_id = ? AND command_key = ? AND actor_user_id = ? AND idempotency_key = ?",
		claim.JobID, claim.CommandKey, claim.ActorUserID, claim.IdempotencyKey).First(&stored).Error; err != nil {
		return nil, false, fmt.Errorf("jobs: read command request: %w", err)
	}
	if stored.ID == claim.ID {
		return nil, true, nil
	}
	return &stored, false, nil
}

// replayCommandResult answers a repeat of a command from the row that recorded
// it, without reaching any executor.
//
// A failure is replayed as a failure: the command has already been decided, and
// answering a repeat with a fresh attempt would be exactly the second side effect
// this table exists to prevent. The Job is re-read rather than replayed, because
// the caller asked about a Job and the recorded outcome is about the command.
func (s *Service) replayCommandResult(deps Deps, request CommandRequest, row models.JobCommandRequest) (CommandResult, error) {
	if row.RequestHash != commandRequestHash(request) {
		return CommandResult{}, fmt.Errorf("%w: job %s %s", ErrCommandKeyReused, row.JobID, row.CommandKey)
	}
	if row.Status == models.JobCommandStatusRunning {
		return CommandResult{
			JobID: row.JobID, Key: row.CommandKey,
			Status: CommandStatusFailed, Code: CommandCodeInFlight,
			Message: "an identical request is still in flight",
		}, fmt.Errorf("%w: job %s %s", ErrCommandInFlight, row.JobID, row.CommandKey)
	}

	job, err := loadVisibleJob(deps.DB, request.Actor, row.JobID)
	if err != nil {
		return CommandResult{}, err
	}
	result := CommandResult{
		JobID: row.JobID, Key: row.CommandKey,
		Status: row.Status, Code: row.Code, Message: row.Message,
		Detail:      json.RawMessage(row.Detail),
		SuccessorID: row.SuccessorJobID,
		Job:         viewerSnapshot(job, request.Actor),
	}
	if row.Status == models.JobCommandStatusFailed {
		return result, fmt.Errorf("%w: job %s %s", ErrCommandFailed, row.JobID, row.CommandKey)
	}
	return result, nil
}

// finishSettledResult completes a result that was decided inside its own
// transaction: the Job is re-read through the shared visibility predicate, so a
// result never carries a row the asker may not see.
func (s *Service) finishSettledResult(deps Deps, request CommandRequest, result CommandResult) (CommandResult, error) {
	job, err := loadVisibleJob(deps.DB, request.Actor, request.JobID)
	if err != nil {
		return CommandResult{}, err
	}
	result.Job = viewerSnapshot(job, request.Actor)
	return result, nil
}

// ExecuteBulkCommand runs one command across a selection of Jobs and records one
// independent outcome for every one of them.
//
// A bulk command may partially succeed: each Job is resolved, authorized,
// advertised and executed on its own, so a Job that does not offer the command is
// refused for itself rather than refusing the selection. Eligibility is per Job
// for that reason — the command has to be advertised as bulk-capable for the Job
// it is about, and the Jobs that can act are the intersection of the selection
// with the Jobs that offer it.
//
// A malformed request has no Job to answer about, so it is answered once with an
// empty JobID rather than once per name; a request naming no Jobs is answered
// with nothing at all.
func (s *Service) ExecuteBulkCommand(ctx context.Context, deps Deps, request BulkCommandRequest) []CommandResult {
	if err := validateBulkCommandRequest(request); err != nil {
		return []CommandResult{{Key: request.Key, Status: CommandStatusFailed, Code: CommandCodeInvalid, Message: err.Error()}}
	}

	results := make([]CommandResult, 0, len(request.JobIDs))
	for index, jobID := range request.JobIDs {
		if index >= MaxBulkCommandJobs {
			results = append(results, CommandResult{
				JobID: jobID, Key: request.Key, Status: CommandStatusFailed, Code: CommandCodeInvalid,
				Message: fmt.Sprintf("a bulk command changes at most %d jobs", MaxBulkCommandJobs),
			})
			continue
		}
		results = append(results, s.executeBulkEntry(ctx, deps, request, jobID))
	}
	return results
}

// executeBulkEntry resolves one Job of a bulk selection and runs the command on
// it, answering with that Job's own outcome. Every refusal is this Job's: a
// selection is not one operation with one verdict.
func (s *Service) executeBulkEntry(ctx context.Context, deps Deps, request BulkCommandRequest, jobID string) CommandResult {
	entry := CommandRequest{
		JobID:          jobID,
		Key:            request.Key,
		IdempotencyKey: request.IdempotencyKey,
		Actor:          request.Actor,
		Origin:         request.Origin,
	}

	job, err := loadVisibleJob(deps.DB, request.Actor, jobID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CommandResult{
				JobID: jobID, Key: request.Key, Status: CommandStatusFailed,
				Code: CommandCodeNotFound, Message: "no such job or not visible to you",
			}
		}
		return CommandResult{
			JobID: jobID, Key: request.Key, Status: CommandStatusFailed,
			Code: CommandCodeFailed, Message: "the job could not be read",
		}
	}

	commands, err := s.advertisedCommands(ctx, deps, request.Actor, job)
	if err != nil {
		return CommandResult{
			JobID: jobID, Key: request.Key, Status: CommandStatusFailed,
			Code: CommandCodeFailed, Message: "the job's commands could not be read",
		}
	}
	advertised, ok := commandByKey(commands, request.Key)
	if !ok || !advertised.Bulk {
		return CommandResult{
			JobID: jobID, Key: request.Key, Status: CommandStatusFailed,
			Code: CommandCodeNotAdvertised, Message: "the job does not offer that command in bulk",
			Job: viewerSnapshot(job, request.Actor),
		}
	}

	// The version each Job is at when the bulk is resolved is the precondition a
	// bulk command runs under: one number cannot describe a whole selection, and a
	// Job that changed since it was listed is refused per Job like any other stale
	// request.
	entry.ExpectedVersion = job.Version
	settled, commandErr := s.ExecuteCommand(ctx, deps, entry)
	if settled.JobID == "" {
		settled.JobID = jobID
	}
	if settled.Key == "" {
		settled.Key = request.Key
	}
	if settled.Job.ID == "" {
		settled.Job = viewerSnapshot(job, request.Actor)
	}

	// A refusal that carries no result of its own — a request refused before the
	// Job was read, a database failure — is classified here, so a bulk answer is
	// machine-readable however the Job was refused.
	if settled.Status == "" {
		settled.Status = CommandStatusFailed
		settled.Code = commandCodeForError(commandErr)
		if settled.Message == "" {
			settled.Message = "the command did not run"
		}
	}
	return settled
}

// commandByKey returns one advertised command by its key.
func commandByKey(commands []Command, key string) (Command, bool) {
	for _, command := range commands {
		if command.Key == key {
			return command, true
		}
	}
	return Command{}, false
}

// commandCodeForError classifies a command refusal that carried no result of its
// own, so a bulk answer is machine-readable whichever way the Job was refused.
func commandCodeForError(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return CommandCodeNotFound
	case errors.Is(err, ErrCommandNotAdvertised), errors.Is(err, ErrAdapterUnregistered):
		return CommandCodeNotAdvertised
	case errors.Is(err, ErrVersionConflict):
		return CommandCodeConflict
	case errors.Is(err, ErrCommandInFlight):
		return CommandCodeInFlight
	case errors.Is(err, ErrCommandKeyReused):
		return CommandCodeKeyReused
	case errors.Is(err, ErrCommandChainConflict):
		return CommandCodeChainConflict
	default:
		return CommandCodeFailed
	}
}

// validateBulkCommandRequest checks a bulk request before it is expanded.
func validateBulkCommandRequest(request BulkCommandRequest) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidCommand, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(request.Key) == "" {
		return invalid("a command needs a key")
	}
	if len(request.Key) > MaxCommandKeyBytes {
		return invalid("command key is %d bytes, over the %d-byte ceiling", len(request.Key), MaxCommandKeyBytes)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return invalid("a command needs an idempotency key")
	}
	if len(request.IdempotencyKey) > MaxIdempotencyKeyBytes {
		return invalid("idempotency key is %d bytes, over the %d-byte ceiling",
			len(request.IdempotencyKey), MaxIdempotencyKeyBytes)
	}
	if len(request.Origin) > MaxOriginBytes {
		return invalid("origin is %d bytes, over the %d-byte ceiling", len(request.Origin), MaxOriginBytes)
	}
	return nil
}

// executeWorkloadCommand runs one command a Kind's own work supports: the host
// records the durable control intent first, then tells the executor, and records
// what the executor answered.
//
// The order is the point. A cancellation that only tells an executor is a
// cancellation that vanishes when the executor does not answer; recording the
// intent and its event first means the Job says what was asked of it whatever
// happens next, and the executor is contacted only after that record has
// committed.
func (s *Service) executeWorkloadCommand(ctx context.Context, deps Deps, request CommandRequest) (CommandResult, error) {
	job, err := loadVisibleJob(deps.DB, request.Actor, request.JobID)
	if err != nil {
		return CommandResult{}, err
	}
	adapter, _, err := s.adapterFor(job.Kind, job.KindVersion)
	if err != nil {
		return refusedResult(job, request, CommandCodeNotAdvertised,
			"no executor in this process can run that command", err)
	}

	now := deps.now()
	claim := newCommandClaim(request, now)
	var (
		recorded  *models.JobCommandRequest
		requested bool
		settled   *CommandResult
	)
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		existing, claimed, err := claimCommandRequest(tx, claim)
		if err != nil {
			return err
		}
		if !claimed {
			recorded = existing
			return nil
		}

		current, err := s.recheckCommand(ctx, deps, tx, request)
		if err != nil {
			return err
		}
		target, err := prepareControlIntent(tx, current, request.Key, now)
		if err != nil {
			return err
		}
		if target == nil {
			requested = true
			return nil
		}

		// No execution owns this Job, so there is nobody to ask: the command's
		// outcome is the host's to apply, in the same transaction that recorded it.
		applied, err := s.applyCommandTransition(deps, tx, current, *target)
		if err != nil {
			return err
		}
		if hook, ok := adapter.(HostTransitionAdapter); ok {
			scoped := deps
			scoped.DB = tx
			if err := hook.ApplyHostTransition(ctx, scoped, viewerSnapshot(current, request.Actor), request.Key, applied.State); err != nil {
				return err
			}
		}
		outcome := appliedOutcome(commandAppliedMessage(applied.State), nil)
		if err := completeCommandRequest(tx, claim.ID, outcome, now); err != nil {
			return err
		}
		settled = &CommandResult{
			JobID: current.ID, Key: request.Key,
			Status: CommandStatusSucceeded, Code: outcome.code, Message: outcome.message,
		}
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	if recorded != nil {
		return s.replayCommandResult(deps, request, *recorded)
	}
	if settled != nil {
		if hook, ok := adapter.(HostTransitionCompletion); ok {
			hook.AfterHostTransition(ctx, viewerSnapshot(job, request.Actor), request.Key, StateCancelled)
		}
		return s.finishSettledResult(deps, request, *settled)
	}
	if !requested {
		return CommandResult{}, fmt.Errorf("%w: job %s offered no outcome for %s",
			ErrCommandNotAdvertised, request.JobID, request.Key)
	}

	current, err := loadVisibleJob(deps.DB, request.Actor, request.JobID)
	if err != nil {
		return CommandResult{}, err
	}
	outcome, execErr := adapter.ExecuteCommand(ctx, CommandExecution{
		JobID:           current.ID,
		Key:             request.Key,
		IdempotencyKey:  request.IdempotencyKey,
		ExpectedVersion: request.ExpectedVersion,
		Snapshot:        viewerSnapshot(current, request.Actor),
		Access:          request.Actor,
	})
	return s.settleWorkloadOutcome(deps, request, current, claim.ID, outcome, execErr)
}

// recheckCommand re-reads the Job a command is about inside the transaction that
// acts on it, under the caller's own visibility, and asks the Kind again whether
// the Job still offers the command.
//
// Both halves are the same question — "is the decision this request was prepared
// from still true?" — and the second half is the one that cannot be answered by a
// version. An adapter's advertisement is a statement about *current* policy and
// current state of the world: a plugin disabled, a registration replaced by a
// newer plugin.lua, a signed artifact removed, an operator's setting changed, a
// principal's scope narrowed. None of those move the Job's version, so a command
// that trusted the advertisement it read before the transaction could create a
// successor for work that is no longer admissible — §4's rule is that command
// execution atomically rechecks authorization, version, state, control intent and
// Kind policy, and this is where the last three are asked.
//
// The refusals are typed and write nothing: the claim, the effect and the
// outcome still commit together or not at all.
func (s *Service) recheckCommand(ctx context.Context, deps Deps, tx *gorm.DB, request CommandRequest) (models.Job, error) {
	current, err := loadVisibleJob(tx, request.Actor, request.JobID)
	if err != nil {
		return models.Job{}, err
	}
	if current.Version != request.ExpectedVersion {
		return models.Job{}, fmt.Errorf("%w: job %s changed while the command was being claimed",
			ErrVersionConflict, current.ID)
	}
	scoped := deps
	scoped.DB = tx
	offered, err := s.advertisedCommands(ctx, scoped, request.Actor, current)
	if err != nil {
		return models.Job{}, err
	}
	if !offersCommand(offered, request.Key) {
		return models.Job{}, fmt.Errorf("%w: job %s no longer offers %s", ErrCommandNotAdvertised, current.ID, request.Key)
	}
	return current, nil
}

// prepareControlIntent records the durable intent one control command asks for
// and reports the state the host applies by itself when no execution owns the
// Job.
//
// An unowned Job has nobody to ask, so a cancellation ends it: §4's "queued or
// scheduled work with no executor may transition immediately". An owned Job is
// left running — a Job being cancelled is still running until its execution stops
// — and the state change is the executor's to publish under its own claim. Pause
// has no such branch: it is confirmed by an execution, so it is only ever offered
// for work one owns.
func prepareControlIntent(tx *gorm.DB, job models.Job, key string, now time.Time) (*State, error) {
	switch key {
	case CommandCancel:
		if job.ExecutionToken == "" {
			target := StateCancelled
			return &target, nil
		}
		return nil, requestControlIntent(tx, job, ControlIntentCancel, PhaseCancelling, now)
	case CommandPause:
		if job.ExecutionToken == "" {
			// Nothing is running this Job, so there is no execution to reach a
			// checkpoint and no state this command could honestly reach: it is refused
			// rather than replacing a paused state an executor never confirmed.
			return nil, fmt.Errorf("%w: job %s is not running", ErrCommandNotAdvertised, job.ID)
		}
		return nil, requestControlIntent(tx, job, ControlIntentPause, PhasePausing, now)
	default:
		return nil, nil
	}
}

// requestControlIntent records what a viewer asked of an execution, with the verb
// and the phase it enters while the request is outstanding, in one guarded write
// with the event that announces it.
//
// The version moves with it. A control request is a durable change to a Job, and
// every reader that decided from the old version — an executor about to publish
// success, a client about to offer another command — has to re-decide against
// the version that carries it.
func requestControlIntent(tx *gorm.DB, job models.Job, intent, phase string, now time.Time) error {
	if job.ControlIntent == ControlIntentCancel && intent != ControlIntentCancel {
		return fmt.Errorf("%w: job %s", ErrControlIntentWon, job.ID)
	}

	result := tx.Model(&models.Job{}).
		Where("id = ? AND version = ? AND state = ? AND "+executionTokenMatch,
			job.ID, job.Version, job.State, job.ExecutionToken).
		Updates(map[string]any{
			"control_intent":       intent,
			"control_requested_at": now,
			"phase":                phase,
			"version":              job.Version + 1,
			"updated_at":           now,
		})
	if result.Error != nil {
		return fmt.Errorf("jobs: record control intent: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: job %s changed while the control request was recorded", ErrVersionConflict, job.ID)
	}

	detail, err := json.Marshal(map[string]string{"intent": intent})
	if err != nil {
		return fmt.Errorf("jobs: encode control request event: %w", err)
	}
	sequence, err := nextEventSequence(tx, job.ID)
	if err != nil {
		return err
	}
	event := newEvent(job.ID, sequence, job.Version+1, EventControlRequested, detail, true, now)
	if err := tx.Create(&event).Error; err != nil {
		return fmt.Errorf("jobs: store control request event: %w", err)
	}
	return nil
}

// applyCommandTransition applies the state change a command's outcome implies,
// through the same lifecycle entry point every other writer uses, so the state
// machine, the version guard, the cumulative durations and the event are the ones
// a transition always gets.
func (s *Service) applyCommandTransition(deps Deps, tx *gorm.DB, job models.Job, to State) (Snapshot, error) {
	scoped := deps
	scoped.DB = tx
	return s.Transition(scoped, Transition{
		JobID:           job.ID,
		ExpectedVersion: job.Version,
		ExecutionToken:  job.ExecutionToken,
		To:              to,
	})
}

// commandAppliedMessage is the bounded text one immediately applied control
// command reports.
func commandAppliedMessage(state State) string {
	switch state {
	case StateCancelled:
		return "cancelled"
	case StatePaused:
		return "paused"
	case StateQueued:
		return "queued"
	default:
		return "applied"
	}
}

// settleWorkloadOutcome records what an executor answered about one command, and
// is where a successful resume becomes durable work the runtime can pick up.
//
// The executor's own error text is deliberately not recorded: only the Kind knows
// what in it is safe, and this row is read back to a caller. A Kind that wants to
// explain itself returns a bounded message and detail instead.
func (s *Service) settleWorkloadOutcome(deps Deps, request CommandRequest, job models.Job, claimID string, outcome CommandOutcome, execErr error) (CommandResult, error) {
	now := deps.now()
	record := commandOutcome{status: models.JobCommandStatusSucceeded, code: CommandCodeApplied}
	switch {
	case execErr != nil:
		record = commandOutcome{
			status:  models.JobCommandStatusFailed,
			code:    CommandCodeFailed,
			message: "the command could not be run",
		}
	case !validCommandOutcome(outcome):
		record = commandOutcome{
			status:  models.JobCommandStatusFailed,
			code:    CommandCodeFailed,
			message: "the executor answered outside the command vocabulary",
		}
	default:
		record.status = outcome.Status
		record.message = outcome.Message
		record.detail = outcome.Detail
		if outcome.Status == CommandStatusFailed {
			record.code = CommandCodeFailed
		}
	}
	// A command whose effect is still in the executor's hands reports what was
	// requested rather than what was applied: the Job keeps its state, and the
	// record says which control is outstanding.
	if record.status == models.JobCommandStatusSucceeded {
		if current, err := loadVisibleJob(deps.DB, request.Actor, job.ID); err == nil && current.ControlIntent != "" {
			record.code = CommandCodeRequested
		}
	}

	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		// The transaction's first statement is the write, for the module's usual
		// reason: on SQLite the writer lock has to be taken before anything is read,
		// and this update is also what says the claim this outcome belongs to is
		// still the one that was made.
		if err := completeCommandRequest(tx, claimID, record, now); err != nil {
			return err
		}
		if record.status == models.JobCommandStatusSucceeded && request.Key == CommandResume {
			return s.applyResume(tx, deps, request)
		}
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}

	result := CommandResult{
		JobID: job.ID, Key: request.Key,
		Status: record.status, Code: record.code, Message: record.message, Detail: record.detail,
	}
	settled, settleErr := s.finishSettledResult(deps, request, result)
	if record.status == models.JobCommandStatusFailed {
		return settled, fmt.Errorf("%w: job %s %s", ErrCommandFailed, job.ID, request.Key)
	}
	return settled, settleErr
}

// validCommandOutcome reports whether an executor's answer is one the module
// knows: a status it can record, and bounded text to record with it.
func validCommandOutcome(outcome CommandOutcome) bool {
	switch outcome.Status {
	case CommandStatusSucceeded, CommandStatusFailed:
	default:
		return false
	}
	if len(outcome.Message) > MaxCommandMessageBytes {
		return false
	}
	if len(outcome.Detail) > 0 {
		if len(outcome.Detail) > MaxCommandDetailBytes || !json.Valid(outcome.Detail) {
			return false
		}
	}
	return true
}

// applyResume moves held work back to the queue once its executor has accepted it
// again.
//
// It is the one success the host must make durable itself. Pause and cancel are
// published by the execution that owns the Job — it is the thing that reaches the
// checkpoint or stops — but a paused or blocked Job has no execution left to
// publish anything, so nothing but the command's own outcome can return it to the
// queue.
func (s *Service) applyResume(tx *gorm.DB, deps Deps, request CommandRequest) error {
	current, err := loadVisibleJob(tx, request.Actor, request.JobID)
	if err != nil {
		return err
	}
	switch State(current.State) {
	case StatePaused, StateBlocked:
	default:
		return nil
	}
	// The advertisement refused this while an unresolved claim owned the Job, and
	// the executor that answered ran between two transactions — so the question is
	// asked again here, against the row this write is about to change. A quarantine
	// that landed in the gap would otherwise be released by a command that was
	// prepared before it existed.
	if unresolved, err := unresolvedClaimOn(tx, current.ID); err != nil {
		return err
	} else if unresolved {
		return fmt.Errorf("%w: job %s is still owned by an unresolved claim",
			ErrCommandNotAdvertised, current.ID)
	}
	_, err = s.applyCommandTransition(deps, tx, current, StateQueued)
	return err
}

// executeHostCommand runs one of the commands the host owns outright: a viewer's
// dismissal or pinning, pinning a whole visible lineage, and forgetting sealed
// input. Everything it does is durable, so it is one transaction: the claim, the
// effect and the outcome commit together.
func (s *Service) executeHostCommand(ctx context.Context, deps Deps, request CommandRequest) (CommandResult, error) {
	now := deps.now()
	claim := newCommandClaim(request, now)
	var (
		recorded *models.JobCommandRequest
		settled  *CommandResult
	)
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		existing, claimed, err := claimCommandRequest(tx, claim)
		if err != nil {
			return err
		}
		if !claimed {
			recorded = existing
			return nil
		}
		current, err := s.recheckCommand(ctx, deps, tx, request)
		if err != nil {
			return err
		}
		scoped := deps
		scoped.DB = tx
		outcome, err := s.applyHostCommand(ctx, scoped, request, current)
		if err != nil {
			return err
		}
		if err := completeCommandRequest(tx, claim.ID, outcome, now); err != nil {
			return err
		}
		settled = &CommandResult{
			JobID: current.ID, Key: request.Key,
			Status: outcome.status, Code: outcome.code, Message: outcome.message, Detail: outcome.detail,
		}
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	if recorded != nil {
		return s.replayCommandResult(deps, request, *recorded)
	}
	return s.finishSettledResult(deps, request, *settled)
}

// applyHostCommand performs one host-owned command's durable effect.
func (s *Service) applyHostCommand(_ context.Context, deps Deps, request CommandRequest, job models.Job) (commandOutcome, error) {
	switch request.Key {
	case CommandDismiss:
		if err := s.SetPreference(deps, request.Actor, PreferenceRequest{JobID: job.ID, Dismissed: boolPointer(true)}); err != nil {
			return commandOutcome{}, err
		}
		return appliedOutcome("dismissed", nil), nil
	case CommandPin:
		if err := s.SetPreference(deps, request.Actor, PreferenceRequest{JobID: job.ID, Pinned: boolPointer(true)}); err != nil {
			return commandOutcome{}, err
		}
		return appliedOutcome("pinned", nil), nil
	case CommandPinLineage:
		return s.pinVisibleLineage(deps, request, job)
	case CommandForget:
		if _, err := s.ForgetReplay(deps, request.Actor, job.ID); err != nil {
			return commandOutcome{}, err
		}
		return appliedOutcome("replay input forgotten", nil), nil
	default:
		return commandOutcome{}, fmt.Errorf("%w: %s is not a host-owned command", ErrInvalidCommand, request.Key)
	}
}

// lineagePinRefusal is one relative a lineage pin could not pin, and the bounded
// reason it could not. The reason is a code rather than an error's text: this
// detail is stored and read back to a caller.
type lineagePinRefusal struct {
	JobID  string `json:"jobId"`
	Reason string `json:"reason"`
}

// pinVisibleLineage pins this Job and every relative of its lineage the asker may
// see, one preference row at a time.
//
// One hop, and independently authorized: lineage grants no transitive reach, so a
// hidden relative is not pinned and not named — the detail says which Jobs were
// pinned and which were refused, and a Job the asker cannot see is in neither.
// The per-viewer pin limit still applies to every one of them, so a lineage that
// would exceed it is partially pinned and says which relatives were refused for
// it rather than failing the whole command.
func (s *Service) pinVisibleLineage(deps Deps, request CommandRequest, job models.Job) (commandOutcome, error) {
	lineage, err := s.Lineage(deps, request.Actor, job.ID)
	if err != nil {
		return commandOutcome{}, err
	}
	if err := s.SetPreference(deps, request.Actor, PreferenceRequest{JobID: job.ID, Pinned: boolPointer(true)}); err != nil {
		return commandOutcome{}, err
	}

	pinned := []string{job.ID}
	refused := make([]lineagePinRefusal, 0)
	for _, relative := range lineageRelatives(lineage) {
		err := s.SetPreference(deps, request.Actor, PreferenceRequest{JobID: relative.ID, Pinned: boolPointer(true)})
		switch {
		case err == nil:
			pinned = append(pinned, relative.ID)
		case errors.Is(err, ErrPinLimitReached):
			refused = append(refused, lineagePinRefusal{JobID: relative.ID, Reason: "pin-limit"})
		case errors.Is(err, ErrNotFound):
			refused = append(refused, lineagePinRefusal{JobID: relative.ID, Reason: "gone"})
		default:
			refused = append(refused, lineagePinRefusal{JobID: relative.ID, Reason: "refused"})
		}
	}

	detail, err := json.Marshal(map[string]any{"pinned": pinned, "refused": refused})
	if err != nil {
		return commandOutcome{}, fmt.Errorf("jobs: encode lineage pin detail: %w", err)
	}
	return appliedOutcome(fmt.Sprintf("pinned %d of %d visible related jobs", len(pinned), len(pinned)+len(refused)), detail), nil
}

// lineageRelatives is every relative one lineage read named, deduplicated and
// ordered so a pin pass over it is deterministic. A Job can be related in two
// ways at once, and pinning it twice is the same row.
func lineageRelatives(lineage Lineage) []Snapshot {
	seen := make(map[string]bool, 6)
	relatives := make([]Snapshot, 0, 6)
	for _, group := range [][]Snapshot{lineage.Ancestors, lineage.Successors, lineage.Parents, lineage.Children} {
		for _, relative := range group {
			if relative.ID == lineage.Job.ID || seen[relative.ID] {
				continue
			}
			seen[relative.ID] = true
			relatives = append(relatives, relative)
		}
	}
	slices.SortStableFunc(relatives, func(a, b Snapshot) int {
		if cmp := b.AcceptedAt.Compare(a.AcceptedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	return relatives
}

// boolPointer copies a bool so a request literal never shares its address with a
// stored update.
func boolPointer(v bool) *bool { return &v }

// executeLineageCommand runs one Retry or Repeat: a new Job from unchanged sealed
// input, linked to the Job it continues, with the ancestor untouched.
//
// Everything is one transaction — the claim, the successor, its link and the
// recorded outcome — because a successor without its link would be a Job with no
// lineage and a link without its successor a row nothing can resolve. The chain
// predicate is read under the retried Job's own row lock, which is what makes
// "at most one active recovery Job exists in the chain" true under two Retries at
// once rather than merely true when they arrive one after the other.
func (s *Service) executeLineageCommand(ctx context.Context, deps Deps, request CommandRequest) (CommandResult, error) {
	linkType := LinkRetryOf
	if request.Key == CommandRepeat {
		linkType = LinkRepeatOf
	}

	now := deps.now()
	claim := newCommandClaim(request, now)
	var (
		recorded *models.JobCommandRequest
		settled  *CommandResult
	)
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		existing, claimed, err := claimCommandRequest(tx, claim)
		if err != nil {
			return err
		}
		if !claimed {
			recorded = existing
			return nil
		}
		scoped := deps
		scoped.DB = tx
		return s.createSuccessor(ctx, scoped, tx, request, linkType, claim.ID, now, &settled)
	})
	if err != nil {
		return CommandResult{}, err
	}
	if recorded != nil {
		return s.replayCommandResult(deps, request, *recorded)
	}
	return s.finishSettledResult(deps, request, *settled)
}

// createSuccessor accepts the linked Job one Retry or Repeat asks for, inside the
// transaction that claimed the command.
func (s *Service) createSuccessor(ctx context.Context, deps Deps, tx *gorm.DB, request CommandRequest, linkType LinkType, claimID string, now time.Time, settled **CommandResult) error {
	job, err := s.recheckCommand(ctx, deps, tx, request)
	if err != nil {
		return err
	}
	if err := lockRetryChain(tx, job); err != nil {
		return err
	}
	// Retry lineage is linear, so a Job that already has a retry successor is not
	// the leaf and a second successor would make "the current leaf" mean two
	// things. Repeat is deliberately exempt: each repeat is an independent
	// execution of successful work, and branching is what it is for.
	if linkType == LinkRetryOf {
		if err := requireNoRetrySuccessor(tx, job); err != nil {
			return err
		}
	}

	// The input is opened here, inside the transaction, rather than copied as
	// sealed bytes: acceptance seals what the Kind's codec encodes, so writing the
	// successor from the ancestor's envelope would bypass the one place a Kind
	// declares what its input is.
	opened, err := s.OpenReplay(deps, request.Actor, job.ID)
	if err != nil {
		return err
	}

	owner := ownerReference(request.Actor)
	successor, err := s.Accept(deps, Acceptance{
		Kind:        job.Kind,
		KindVersion: job.KindVersion,
		State:       StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      commandOrigin(request, job),
		Title:       job.Title,
		Replay:      ReplayInput{Input: opened.Input},
	})
	if err != nil {
		return err
	}

	if err := s.Link(deps, LinkRequest{Type: linkType, FromJobID: successor.ID, ToJobID: job.ID}); err != nil {
		return err
	}

	// A Retry successor moves the ancestor's compatibility handles onto itself,
	// in the same transaction as its acceptance, its link and the command's
	// outcome: a legacy client polling its unchanged id must not be left on an
	// ancestor the successor has already replaced, and a movement recorded
	// separately could fail after the client was answered. A Repeat does not: a
	// handle projects the linear Retry lineage, and a repeat is a branch off it.
	if linkType == LinkRetryOf {
		if err := moveLegacyHandles(tx, job.ID, successor.ID, now); err != nil {
			return err
		}
	}

	outcome := appliedOutcome("a new job was created", nil)
	outcome.successorID = successor.ID
	if err := completeCommandRequest(tx, claimID, outcome, now); err != nil {
		return err
	}
	*settled = &CommandResult{
		JobID: job.ID, Key: request.Key,
		Status: outcome.status, Code: outcome.code, Message: outcome.message,
		SuccessorID: successor.ID,
	}
	return nil
}

// ownerReference is the owner and actor one successor Job records: the principal
// that asked for it. A host principal with no account of its own leaves the
// successor ownerless, which the shared visibility predicate makes admin-only
// rather than somebody else's.
func ownerReference(access Access) *uint {
	if access.UserID == 0 {
		return nil
	}
	owner := access.UserID
	return &owner
}

// requireNoRetrySuccessor refuses the successor a Retry asks for when the Job
// already has one. The predicate is read after the chain lock, so a Retry that
// lost the race reads the link the winner committed rather than the empty chain
// both callers saw when they advertised the command.
func requireNoRetrySuccessor(tx *gorm.DB, job models.Job) error {
	successors, err := retrySuccessors(tx, job.ID)
	if err != nil {
		return err
	}
	if len(successors) > 0 {
		// The successor can belong to somebody the actor cannot see (for example,
		// an administrator who retried an owner's Job). The conflict is all the
		// caller needs; naming the UUID would bypass lineage visibility.
		return fmt.Errorf("%w: job %s already has a retry successor", ErrCommandChainConflict, job.ID)
	}
	return nil
}

// lockRetryChain takes the rows a retry-lineage decision depends on, in canonical
// ID order, before the predicate above is read.
//
// On PostgreSQL those rows are held FOR UPDATE, so two Retries of one Job
// serialize: the loser reads the successor the winner committed and is refused.
// SQLite has no row locks, and does not need one here: the executor's own
// transaction has already taken the writer lock with its first statement (the
// idempotency claim), which is the same exclusion by a different mechanism —
// which is also why this writes nothing there rather than touching a finished
// Job's row for no reason.
//
// The *staging ancestors* are taken as well as the Retry chain, and that is
// retention's side of the same rule rather than a second concern. A successor still
// names the inputs its ancestor named — a Retry copies the ancestor's sealed input —
// so what a staged hand-off depends on is the whole lineage walk
// (jobs.LineageAncestors), not the linear chain. Retention prunes a candidate by
// taking its row, deleting it, and then walking *down* to see whether a live Job still
// depends on it; a retry that locked only its own chain could commit inside that
// window — after the walk had read "no live descendant" — and both transactions would
// commit: the ancestor gone, the queued successor left naming files nothing protects.
// Taking the ancestors FOR UPDATE makes the window unreachable: the pruning
// transaction holds the candidate, so this one waits, and by the time it can look the
// candidate is gone — the refusal this lock exists to produce.
func lockRetryChain(tx *gorm.DB, job models.Job) error {
	if tx.Dialector.Name() == "sqlite" {
		return nil
	}
	chain, err := retryChain(tx, job.ID)
	if err != nil {
		return err
	}
	start := append(append([]string(nil), chain...), job.ID)
	ancestors, err := LineageAncestors(tx, start, MaxStagingLineageHops)
	if err != nil {
		return err
	}
	ids := append(append([]string(nil), start...), ancestors...)
	slices.Sort(ids)
	ids = slices.Compact(ids)

	var locked []string
	if err := tx.Model(&models.Job{}).Where("id IN ?", ids).Order("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).Pluck("id", &locked).Error; err != nil {
		return fmt.Errorf("jobs: lock retry chain: %w", err)
	}
	if len(locked) != len(ids) {
		return fmt.Errorf("%w: a retry chain named a job that is gone", ErrNotFound)
	}
	return nil
}

// maxRetryChainHops bounds the walk up a Retry lineage. The chain is linear, so a
// chain longer than this is a corruption rather than a long history: the walk
// stops instead of following it forever.
const maxRetryChainHops = 64

// retryChain walks the Retry ancestors of one Job: the Jobs it directly retries,
// then theirs, until the chain ends.
func retryChain(tx *gorm.DB, jobID string) ([]string, error) {
	chain := make([]string, 0, 4)
	seen := map[string]bool{jobID: true}
	current := jobID

	for hop := 0; hop < maxRetryChainHops; hop++ {
		var ancestors []string
		if err := tx.Model(&models.JobLink{}).
			Where("type = ? AND from_job_id = ?", string(LinkRetryOf), current).
			Order("to_job_id").Pluck("to_job_id", &ancestors).Error; err != nil {
			return nil, fmt.Errorf("jobs: read retry chain: %w", err)
		}
		next := ""
		for _, ancestor := range ancestors {
			if !seen[ancestor] {
				seen[ancestor] = true
				next = ancestor
				break
			}
		}
		if next == "" {
			break
		}
		chain = append(chain, next)
		current = next
	}
	return chain, nil
}
