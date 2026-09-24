package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/types"
)

// This file drives the command surface the way the canonical HTTP layer will:
// ask a Job what it offers under a principal's access, then run one of those
// commands idempotently. The properties worth pinning are all negative — a
// command that is not advertised is refused, a repeat does not run the executor
// twice, a Retry does not touch its ancestor — because those are the ones a
// convenience implementation would quietly lose.

// commandHarness is one command-surface fixture: a real file-backed database
// with the durable core, a registered Kind with a codec for its input, and a
// replay keyring — so a Job can be accepted with sealed input, claimed by a
// runtime and finished exactly the way production does.
type commandHarness struct {
	t       *testing.T
	svc     *Service
	deps    Deps
	adapter *testAdapter
	clock   time.Time
}

// commandKindInput is the sealed input every command fixture is accepted with.
// It is shaped like a remote download because that is the Kind the command
// matrix's first row describes, and because a Replay that has to survive into a
// successor is easiest to assert when the payload is recognizable.
const commandKindInput = `{"url":"https://files.example.test/media/clip.mp4","title":"clip.mp4"}`

func newCommandHarness(t *testing.T) *commandHarness {
	t.Helper()
	return newCommandHarnessOn(t, newTestDeps(t))
}

// newCommandHarnessOn builds the same fixture on a caller's handle, so the
// dialect-sensitive tests can drive the identical surface against PostgreSQL.
func newCommandHarnessOn(t *testing.T, deps Deps) *commandHarness {
	t.Helper()
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "command-surface-key")}
	svc := NewService()
	registerTestCodec(t, svc)
	adapter := registerTestAdapter(t, svc, testDefinition())
	return &commandHarness{
		t: t, svc: svc, deps: deps, adapter: adapter,
		clock: time.Date(2034, 7, 1, 12, 0, 0, 0, time.UTC),
	}
}

// now advances the harness clock and installs it, so every stored instant is the
// test's rather than the wall clock's and two successive writes are ordered.
func (h *commandHarness) now() time.Time {
	h.clock = h.clock.Add(time.Second)
	deps := h.deps
	deps.Now = func() time.Time { return h.clock }
	h.deps = deps
	return h.clock
}

// acceptReplayable accepts one queued Job of the test Kind carrying sealed
// input, which is the shape every command except the host's bookkeeping needs.
func (h *commandHarness) acceptReplayable(owner *uint) Snapshot {
	h.t.Helper()
	h.now()
	return acceptFor(h.t, h.svc, h.deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: owner, ActorUserID: owner, Title: "clip.mp4",
		Replay: ReplayInput{Input: json.RawMessage(commandKindInput)},
	})
}

// acceptPlain accepts one queued Job of another Kind with no replayable input.
func (h *commandHarness) acceptPlain(kind string, owner *uint) Snapshot {
	h.t.Helper()
	h.now()
	return acceptFor(h.t, h.svc, h.deps, Acceptance{
		Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: owner, Title: "plain",
		Replay: ReplayInput{NonReplayable: true},
	})
}

// claim takes the next queued Job of the test Kind the way a runtime does, and
// reports the execution that owns it.
func (h *commandHarness) claim(jobID string) Execution {
	h.t.Helper()
	h.now()
	execution, ok := claimOnce(h.t, h.svc, h.deps, "runtime-command")
	if !ok {
		h.t.Fatalf("job %s was not claimable", jobID)
	}
	if execution.JobID != jobID {
		h.t.Fatalf("claimed %s while acting on %s", execution.JobID, jobID)
	}
	return execution
}

// endExecution ends a claimed Job with one outcome, as its executor does.
func (h *commandHarness) endExecution(jobID string, execution Execution, outcome State) Snapshot {
	h.t.Helper()
	h.now()
	request := FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: jobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: jobRow(h.t, h.deps, jobID).Version,
		Outcome:         outcome,
	}
	if outcome == StateFailed {
		request.Failure = &Failure{Code: "boom", Class: FailureClassInternal}
	}
	finished, err := h.svc.Finish(h.deps, request)
	if err != nil {
		h.t.Fatalf("finish %s as %s: %v", jobID, outcome, err)
	}
	return finished
}

// fail walks a Job to a claimed, failed state — the shape Retry is for.
func (h *commandHarness) fail(jobID string) Snapshot {
	h.t.Helper()
	return h.endExecution(jobID, h.claim(jobID), StateFailed)
}

// succeed walks a Job to a claimed, successful state — the shape Repeat is for.
func (h *commandHarness) succeed(jobID string) Snapshot {
	h.t.Helper()
	return h.endExecution(jobID, h.claim(jobID), StateSucceeded)
}

// request is one command request from a viewer, with the version precondition
// filled from the Job as it currently stands.
func (h *commandHarness) request(jobID, key, idempotencyKey string, access Access) CommandRequest {
	h.t.Helper()
	return CommandRequest{
		JobID:           jobID,
		Key:             key,
		IdempotencyKey:  idempotencyKey,
		ExpectedVersion: jobRow(h.t, h.deps, jobID).Version,
		Actor:           access,
		Origin:          "api",
	}
}

// advertise asks one Job what it offers and fails the test on an error.
func (h *commandHarness) advertise(jobID string, access Access) []Command {
	h.t.Helper()
	commands, err := h.svc.AdvertisedCommands(context.Background(), h.deps, access, jobID)
	if err != nil {
		h.t.Fatalf("AdvertisedCommands(%s): %v", jobID, err)
	}
	return commands
}

// requireCommandKeys compares an advertisement against the exact set of keys it
// must offer. The order an advertisement returns is the adapter's followed by
// the host's, which a client does not depend on, so this compares sets.
func requireCommandKeys(t *testing.T, what string, commands []Command, want ...string) {
	t.Helper()
	got := make([]string, 0, len(commands))
	for _, command := range commands {
		got = append(got, command.Key)
	}
	sortedGot := slices.Clone(got)
	sortedWant := slices.Clone(want)
	slices.Sort(sortedGot)
	slices.Sort(sortedWant)
	if !slices.Equal(sortedGot, sortedWant) {
		t.Fatalf("%s offers %v, want %v", what, got, want)
	}
}

// onlyCommand returns the advertised entry for one key, failing the test when
// the Job does not offer it.
func onlyCommand(t *testing.T, commands []Command, key string) Command {
	t.Helper()
	for _, command := range commands {
		if command.Key == key {
			return command
		}
	}
	t.Fatalf("no %s among the advertised commands", key)
	return Command{}
}

// advertiseStateful installs the adapter's advertisement the way the command
// matrix reads: cancel and pause while work can run, Repeat for successful work,
// Retry for unsuccessful work, and a Kind-specific inspection command
// throughout. It also claims two of the host's own keys, which the host must
// ignore rather than believe.
func (h *commandHarness) advertiseStateful() {
	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		base := []Command{
			{Key: CommandCancel, Label: "Cancel", Destructive: true, Confirmation: "Stop this job?"},
			{Key: CommandPause, Label: "Pause"},
			{Key: CommandResume, Label: "Resume"},
			{Key: CommandDismiss, Label: "Dismiss", Bulk: true},
			{Key: CommandForget, Label: "Forget", Destructive: true},
			{Key: "inspect", Label: "Inspect output"},
		}
		switch commandContext.Snapshot.State {
		case StateSucceeded, StateFailed, StateCancelled, StateInterrupted:
			// A Kind declares which controls its work supports; whether this Job may
			// use one of them right now is the host's answer, which is why both are
			// offered here for every finished Job and narrowed afterwards by the
			// rules under test.
			return append(base,
				Command{Key: CommandRetry, Label: "Retry"},
				Command{Key: CommandRepeat, Label: "Repeat"},
			), nil
		default:
			return base, nil
		}
	}
}

// TestCommandAdvertisementFollowsTheAdapterAndTheHostOwnedVocabulary is §4's "the
// UI never derives them from Kind and state conditionals": what a Job offers is
// its adapter's answer, asked under the asking principal's access, narrowed by
// the host-owned facts it can honor, plus the host's own bookkeeping — and the
// version and endpoint on every entry are the host's, not the adapter's claim.
func TestCommandAdvertisementFollowsTheAdapterAndTheHostOwnedVocabulary(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	h.advertiseStateful()

	running := h.acceptReplayable(&owner)
	h.claim(running.ID)

	commands := h.advertise(running.ID, viewer)
	// Pause and cancel are on offer because the Job is running; the host's own two
	// keys the adapter listed are decided by the host (dismiss needs a finished
	// Job, forget needs sealed input), and pin/pin-lineage are the host's.
	requireCommandKeys(t, "a running job", commands,
		CommandCancel, CommandPause, "inspect", CommandPin, CommandUnpin, CommandPinLineage)

	for _, command := range commands {
		if command.JobVersion != jobRow(t, h.deps, running.ID).Version {
			t.Fatalf("%s was advertised from version %d, want the job's current %d",
				command.Key, command.JobVersion, jobRow(t, h.deps, running.ID).Version)
		}
		if want := "/v1/jobs/" + running.ID + "/commands/" + command.Key; command.Endpoint != want {
			t.Fatalf("%s names endpoint %q, want %q", command.Key, command.Endpoint, want)
		}
	}

	// The advertisement is asked under the asking principal's access, about the
	// Job as that viewer may see it.
	if h.adapter.advertisementCount() == 0 {
		t.Fatal("the adapter was never asked what the job offers")
	}

	failed := h.acceptReplayable(&owner)
	h.fail(failed.ID)
	commands = h.advertise(failed.ID, viewer)
	// Cancel and pause are gone — the host will not offer a control it cannot
	// honor — while the host's own dismissal, pinning, forgetting and the
	// adapter's Retry are there.
	requireCommandKeys(t, "a failed job with sealed input", commands,
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, CommandForget, CommandRetry, "inspect")

	forget := onlyCommand(t, commands, CommandForget)
	if !forget.Destructive || strings.TrimSpace(forget.Confirmation) == "" {
		t.Fatalf("forget = %+v, want a destructive command with confirmation text", forget)
	}

	// An administrator with no user of their own is offered no dismissal and no
	// pin: a preference row belongs to a viewer, and this one has nobody to belong
	// to. The Job's own commands are unaffected.
	requireCommandKeys(t, "an administrator with no user", h.advertise(failed.ID, Access{Administrator: true}),
		CommandForget, CommandRetry, "inspect")

	// A Kind this process has no adapter for: no workload command can be answered,
	// which is a refusal rather than a fallback to whichever executor is nearest.
	// The host's own bookkeeping still works, because none of it needs an executor.
	orphan := h.acceptPlain("group-export", &owner)
	requireCommandKeys(t, "a job whose kind has no adapter here", h.advertise(orphan.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage)

	// A Job the asker may not see is answered exactly as one that does not exist.
	if _, err := h.svc.AdvertisedCommands(context.Background(), h.deps, Access{UserID: owner + 1}, running.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a foreign viewer asking a job's commands = %v, want ErrNotFound", err)
	}
	if _, err := h.svc.AdvertisedCommands(context.Background(), h.deps, Access{Administrator: true}, running.ID); err != nil {
		t.Fatalf("an administrator could not read a job's commands: %v", err)
	}

	// An adapter whose answer is outside the vocabulary is refused rather than
	// published: a command with no key is not a command.
	h.adapter.advertise = func(context.Context, CommandContext) ([]Command, error) {
		return []Command{{Label: "no key"}}, nil
	}
	if _, err := h.svc.AdvertisedCommands(context.Background(), h.deps, viewer, running.ID); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("a malformed adapter command = %v, want ErrInvalidCommand", err)
	}
	h.adapter.advertise = func(context.Context, CommandContext) ([]Command, error) {
		return []Command{{Key: CommandCancel}, {Key: CommandCancel}}, nil
	}
	if _, err := h.svc.AdvertisedCommands(context.Background(), h.deps, viewer, running.ID); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("a duplicated adapter command = %v, want ErrInvalidCommand", err)
	}
}

// TestCommandIdempotencyReturnsTheRecordedOutcomeWithoutRunningItAgain is §4's
// "commands accept an idempotency key": the same (Job, command, actor, key) is
// answered with the outcome that was recorded for it — including when that
// outcome is a failure — and a key reused for a different request is refused
// rather than answered with somebody else's result.
func TestCommandIdempotencyReturnsTheRecordedOutcomeWithoutRunningItAgain(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	h.advertiseStateful()

	running := h.acceptReplayable(&owner)
	h.claim(running.ID)

	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{Status: CommandStatusFailed, Message: "the transfer already stopped"}, nil
	}
	request := h.request(running.ID, CommandCancel, "idem-once", viewer)
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps, request)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("a failed command = %v, want ErrCommandFailed", err)
	}
	if h.adapter.commandCount() != 1 {
		t.Fatalf("the first attempt reached the adapter %d times, want once", h.adapter.commandCount())
	}

	repeated, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-once", viewer))
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("a repeat of a failed command = %v, want ErrCommandFailed", err)
	}
	if repeated.Message != result.Message || repeated.Status != result.Status {
		t.Fatalf("the repeat answered %s/%q, want the recorded %s/%q",
			repeated.Status, repeated.Message, result.Status, result.Message)
	}
	if h.adapter.commandCount() != 1 {
		t.Fatalf("a repeat reached the adapter: %d invocations", h.adapter.commandCount())
	}

	stored := commandRequestRows(t, h.deps, running.ID)
	if len(stored) != 1 {
		t.Fatalf("%d command requests were recorded, want one per idempotency tuple", len(stored))
	}
	if stored[0].Status != models.JobCommandStatusFailed || stored[0].ActorUserID != owner ||
		stored[0].IdempotencyKey != "idem-once" || stored[0].CompletedAt == nil {
		t.Fatalf("the recorded request = %+v, want the failed completed request for actor %d", stored[0], owner)
	}

	// A different actor with the same key is a different tuple, and therefore its
	// own request rather than a replay of this one.
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{Status: CommandStatusSucceeded}, nil
	}
	admin := Access{Administrator: true}
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-once", admin)); err != nil {
		t.Fatalf("an administrator's own request under a key an ordinary user used: %v", err)
	}
	if h.adapter.commandCount() != 2 {
		t.Fatalf("the second actor's request did not reach the adapter (%d invocations)", h.adapter.commandCount())
	}

	// The same key reused for a different request is refused: the recorded outcome
	// describes the other request, so answering with it would be a lie.
	different := h.request(running.ID, CommandCancel, "idem-once", viewer)
	different.Origin = "cli"
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps, different); !errors.Is(err, ErrCommandKeyReused) {
		t.Fatalf("a reused key = %v, want ErrCommandKeyReused", err)
	}
	if h.adapter.commandCount() != 2 {
		t.Fatal("a reused key reached the adapter")
	}

	// A claim a process died holding is answered as in-flight rather than run
	// again: the effect may already have happened, and this mechanism exists to
	// stop exactly that from happening twice.
	hold := h.acceptReplayable(&owner)
	h.claim(hold.ID)
	inFlight := h.request(hold.ID, CommandCancel, "idem-inflight", viewer)
	if err := h.deps.DB.Create(&models.JobCommandRequest{
		ID: types.NewUUIDv7(), JobID: hold.ID, CommandKey: CommandCancel, ActorUserID: owner,
		IdempotencyKey: "idem-inflight", RequestHash: commandRequestHash(inFlight),
		Status: models.JobCommandStatusRunning, CreatedAt: h.now(),
	}).Error; err != nil {
		t.Fatalf("seed an abandoned claim: %v", err)
	}
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, inFlight)
	if !errors.Is(err, ErrCommandInFlight) {
		t.Fatalf("a repeat of an in-flight command = %v, want ErrCommandInFlight", err)
	}
	requireResult(t, "an in-flight repeat", result, CommandStatusFailed, CommandCodeInFlight)
	if h.adapter.commandCount() != 2 {
		t.Fatal("an in-flight repeat reached the adapter")
	}
}

// TestCancelIntentIsDurableBeforeTheExecutorIsAskedAndWinsOverSuccess is §4's
// durable-control-intent contract: a cancellation is recorded — with the event
// that announces it — before any executor hears about it, the Job keeps running
// until its execution stops, a success published afterwards cannot overwrite the
// cancellation that won, and a Job no execution owns is cancelled by the command
// itself rather than waiting for an executor that does not exist.
func TestCancelIntentIsDurableBeforeTheExecutorIsAskedAndWinsOverSuccess(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	h.advertiseStateful()

	running := h.acceptReplayable(&owner)
	execution := h.claim(running.ID)

	// The executor reads the Job at the instant it is asked. Anything it can see
	// there is what was durable before it was contacted, which is the property that
	// makes a cancellation survive an executor that never answers.
	var observed models.Job
	var observedEvents []models.JobEvent
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		observed = jobRow(t, h.deps, running.ID)
		observedEvents = jobEvents(t, h.deps, running.ID)
		return CommandOutcome{Status: CommandStatusSucceeded}, nil
	}
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-cancel", viewer))
	if err != nil {
		t.Fatalf("cancelling a running job: %v", err)
	}
	requireResult(t, "a cancellation request", result, CommandStatusSucceeded, CommandCodeRequested)

	if observed.ControlIntent != ControlIntentCancel {
		t.Fatalf("the executor was asked with control intent %q, want %q recorded first",
			observed.ControlIntent, ControlIntentCancel)
	}
	if observed.Phase != PhaseCancelling {
		t.Fatalf("the cancelling job's phase = %q, want %q", observed.Phase, PhaseCancelling)
	}
	if len(eventsOfTypeIn(observedEvents, EventControlRequested)) != 1 {
		t.Fatalf("the control request recorded %d events, want one", len(eventsOfTypeIn(observedEvents, EventControlRequested)))
	}
	// The Job is still running: cancellation has been asked for, not yet reached.
	if jobRow(t, h.deps, running.ID).State != string(StateRunning) {
		t.Fatalf("a job with a cancellation in flight is %s, want running", jobRow(t, h.deps, running.ID).State)
	}

	// A success published after the cancellation won is refused, and writes
	// nothing: the cancellation owns the outcome.
	_, err = h.svc.Finish(h.deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: running.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		Outcome:         StateSucceeded,
	})
	if !errors.Is(err, ErrControlIntentWon) {
		t.Fatalf("a success after a won cancellation = %v, want ErrControlIntentWon", err)
	}
	if row := jobRow(t, h.deps, running.ID); row.State != string(StateRunning) || row.ControlIntent != ControlIntentCancel {
		t.Fatalf("the refused success changed the job to %s/%q", row.State, row.ControlIntent)
	}

	// The cancellation the execution publishes is accepted, and resolves the intent
	// it was recorded for.
	cancelled, err := h.svc.Finish(h.deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: running.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		Outcome:         StateCancelled,
	})
	if err != nil {
		t.Fatalf("publishing the cancellation: %v", err)
	}
	if cancelled.State != StateCancelled || cancelled.ControlIntent != "" {
		t.Fatalf("the cancelled job = %s with intent %q, want cancelled and resolved", cancelled.State, cancelled.ControlIntent)
	}

	// A Job no execution owns has nobody to ask, so the command applies the outcome
	// itself: queued work must not start running on its way to being cancelled.
	queued := h.acceptReplayable(&owner)
	before := h.adapter.commandCount()
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, h.request(queued.ID, CommandCancel, "idem-queued", viewer))
	if err != nil {
		t.Fatalf("cancelling a queued job: %v", err)
	}
	requireResult(t, "a queued job's cancellation", result, CommandStatusSucceeded, CommandCodeApplied)
	if h.adapter.commandCount() != before {
		t.Fatal("a queued job's cancellation contacted an executor")
	}
	if row := jobRow(t, h.deps, queued.ID); row.State != string(StateCancelled) || row.ControlIntent != "" {
		t.Fatalf("the queued job is %s with intent %q, want cancelled and resolved", row.State, row.ControlIntent)
	}
}

// eventsOfTypeIn filters stored event rows down to one type.
func eventsOfTypeIn(events []models.JobEvent, eventType string) []models.JobEvent {
	matched := make([]models.JobEvent, 0, len(events))
	for _, event := range events {
		if event.Type == eventType {
			matched = append(matched, event)
		}
	}
	return matched
}

// TestRetryCreatesOneLinkedSuccessorFromTheSealedInput is ADR 0007 at the module
// seam: a Retry creates a new Job from the unchanged input, links it as a
// retry-of successor, and leaves the ancestor exactly as it was — its outcome,
// its identity and its own row.
func TestRetryCreatesOneLinkedSuccessorFromTheSealedInput(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)
	before := jobRow(t, h.deps, ancestor.ID)

	request := h.request(ancestor.ID, CommandRetry, "idem-retry", viewer)
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps, request)
	if err != nil {
		t.Fatalf("retrying a failed job: %v", err)
	}
	requireResult(t, "a retry", result, CommandStatusSucceeded, CommandCodeApplied)
	if result.SuccessorID == "" || result.SuccessorID == ancestor.ID {
		t.Fatalf("the retry named successor %q", result.SuccessorID)
	}
	if !uuidV7Pattern.MatchString(result.SuccessorID) {
		t.Fatalf("the successor's identity %q is not a UUIDv7", result.SuccessorID)
	}

	successor := jobRow(t, h.deps, result.SuccessorID)
	if successor.State != string(StateQueued) {
		t.Fatalf("the successor is %s, want queued for dispatch", successor.State)
	}
	if successor.Kind != ancestor.Kind || successor.KindVersion != ancestor.KindVersion {
		t.Fatalf("the successor is %s v%d, want the ancestor's %s v%d",
			successor.Kind, successor.KindVersion, ancestor.Kind, ancestor.KindVersion)
	}
	if successor.OwnerUserID == nil || *successor.OwnerUserID != owner {
		t.Fatalf("the successor's owner = %v, want the requester", successor.OwnerUserID)
	}
	if successor.ActorUserID == nil || *successor.ActorUserID != owner {
		t.Fatalf("the successor's actor = %v, want the requester", successor.ActorUserID)
	}
	if successor.Title != ancestor.Title {
		t.Fatalf("the successor's title = %q, want the ancestor's %q", successor.Title, ancestor.Title)
	}
	if successor.Origin != "api" {
		t.Fatalf("the successor's origin = %q, want the surface the command came from", successor.Origin)
	}

	// The input is copied rather than referenced: the successor carries its own
	// envelope, sealed under the Kind's codec, opened to exactly what the ancestor
	// ran with.
	if successor.ReplayClass != string(ReplayClassReplayable) {
		t.Fatalf("the successor's replay class = %q, want replayable", successor.ReplayClass)
	}
	opened, err := h.svc.OpenReplay(h.deps, viewer, successor.ID)
	if err != nil {
		t.Fatalf("opening the successor's input: %v", err)
	}
	if string(opened.Input) != commandKindInput {
		t.Fatalf("the successor's input = %s, want the ancestor's %s", opened.Input, commandKindInput)
	}

	if successors := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false); len(successors) != 1 || successors[0] != successor.ID {
		t.Fatalf("the ancestor's retry successors = %v, want exactly the new job", successors)
	}
	if ancestors := jobLinks(t, h.deps, successor.ID, LinkRetryOf, true); len(ancestors) != 1 || ancestors[0] != ancestor.ID {
		t.Fatalf("the successor's retried ancestor = %v, want the finished job", ancestors)
	}

	// The ancestor is untouched: a Retry creates an execution, it does not rewrite
	// the one that failed.
	if after := jobRow(t, h.deps, ancestor.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("the retried ancestor changed:\n before: %+v\n after:  %+v", before, after)
	}

	// A repeat of the same request is answered with the successor that was
	// recorded, rather than creating a second one for the same intent.
	repeated, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(ancestor.ID, CommandRetry, "idem-retry", viewer))
	if err != nil {
		t.Fatalf("repeating a retry request: %v", err)
	}
	if repeated.SuccessorID != successor.ID {
		t.Fatalf("the repeat named successor %q, want the recorded %q", repeated.SuccessorID, successor.ID)
	}
	if again := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false); len(again) != 1 {
		t.Fatalf("the ancestor has %d successors after a repeated request, want one", len(again))
	}
}

// TestRetryRefusesSucceededWorkNonLeafAndUnopenableInput collects §4's retry
// eligibility rules at the seam a client meets them: Retry is for unsuccessful
// finished work that is the quiescent leaf of its chain and whose input this
// process can still open, and the command stops being advertised the moment one
// of those stops being true.
//
// The jobs are claimed as soon as they are accepted, because a claim takes the
// oldest waiting work of its Kind and a Retry leaves its successor waiting: a
// test that accepted several jobs before claiming one would be asserting against
// whichever the queue happened to hold first.
func TestRetryRefusesSucceededWorkNonLeafAndUnopenableInput(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	// Successful work is Repeat's, not Retry's.
	succeeded := h.acceptReplayable(&owner)
	h.succeed(succeeded.ID)
	requireCommandKeys(t, "a successful job", h.advertise(succeeded.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, CommandForget, CommandRepeat, "inspect")
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(succeeded.ID, CommandRetry, "idem-succeeded", viewer)); !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("retrying successful work = %v, want ErrCommandNotAdvertised", err)
	}
	if successors := jobLinks(t, h.deps, succeeded.ID, LinkRetryOf, false); len(successors) != 0 {
		t.Fatalf("a refused retry created %d successors", len(successors))
	}

	// Input that was forgotten cannot be run again, and the Job says so by not
	// offering Retry (or Forget) at all.
	forgotten := h.acceptReplayable(&owner)
	h.fail(forgotten.ID)
	if _, err := h.svc.ForgetReplay(h.deps, viewer, forgotten.ID); err != nil {
		t.Fatalf("forget the input: %v", err)
	}
	requireCommandKeys(t, "a job whose input was forgotten", h.advertise(forgotten.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, "inspect")
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(forgotten.ID, CommandRetry, "idem-forgotten", viewer)); !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("retrying forgotten input = %v, want ErrCommandNotAdvertised", err)
	}

	// Work that declares non-replayable input has nothing to re-run, however
	// unsuccessful it is.
	plain := h.acceptPlain(testKind, &owner)
	h.fail(plain.ID)
	requireCommandKeys(t, "a job with non-replayable input", h.advertise(plain.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, "inspect")

	// A successor exists: the ancestor is no longer the leaf, so a second Retry is
	// no longer offered — and the chain stays linear by advancing the leaf instead.
	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(ancestor.ID, CommandRetry, "idem-linear-1", viewer)); err != nil {
		t.Fatalf("the first retry: %v", err)
	}
	leaf := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false)[0]
	requireCommandKeys(t, "a job that already has a successor", h.advertise(ancestor.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, CommandForget, "inspect")
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(ancestor.ID, CommandRetry, "idem-linear-2", viewer)); !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("a second retry of a non-leaf = %v, want ErrCommandNotAdvertised", err)
	}
	if successors := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false); len(successors) != 1 {
		t.Fatalf("the ancestor has %d successors, want one", len(successors))
	}

	// The leaf the first retry created is the Job a later Retry advances.
	h.fail(leaf)
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(leaf, CommandRetry, "idem-linear-3", viewer)); err != nil {
		t.Fatalf("retrying the leaf: %v", err)
	}
	if successors := jobLinks(t, h.deps, leaf, LinkRetryOf, false); len(successors) != 1 {
		t.Fatalf("the leaf has %d successors, want one", len(successors))
	}
}

// TestRetryChainAdmitsOneSuccessorUnderConcurrentRequests is the active-leaf
// predicate: two Retries of one Job, each with its own idempotency key, admit
// exactly one successor. The loser is refused rather than forking the chain, and
// the refusal is a chain conflict rather than a version conflict — the ancestor
// never moved, so nothing about its version could have caught this.
func TestRetryChainAdmitsOneSuccessorUnderConcurrentRequests(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)

	requests := []CommandRequest{
		h.request(ancestor.ID, CommandRetry, "idem-race-a", viewer),
		h.request(ancestor.ID, CommandRetry, "idem-race-b", viewer),
	}
	errs := concurrent(
		func() error {
			_, err := h.svc.ExecuteCommand(context.Background(), h.deps, requests[0])
			return err
		},
		func() error {
			_, err := h.svc.ExecuteCommand(context.Background(), h.deps, requests[1])
			return err
		},
	)

	succeeded, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrCommandChainConflict), errors.Is(err, ErrCommandNotAdvertised):
			// The loser is refused either by the predicate or by the advertisement it
			// re-read after the winner committed; both are refusals that leave the
			// chain linear.
			refused++
		default:
			t.Fatalf("a racing retry failed with %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("%d retries succeeded and %d were refused, want one of each (%s)",
			succeeded, refused, describeErrors(errs))
	}
	if successors := jobLinks(t, h.deps, ancestor.ID, LinkRetryOf, false); len(successors) != 1 {
		t.Fatalf("one job has %d retry successors, want exactly one", len(successors))
	}
}

// TestRepeatBranchesFromSuccessfulWorkWithItsOwnRelation is ADR 0007's other
// half: a Repeat re-runs successful work as an independent Job with a repeat-of
// link, may branch — two repeats of one Job are two Jobs — and never changes the
// ancestor or becomes a Retry of it.
func TestRepeatBranchesFromSuccessfulWorkWithItsOwnRelation(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	successful := h.acceptReplayable(&owner)
	h.succeed(successful.ID)
	before := jobRow(t, h.deps, successful.ID)

	first, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(successful.ID, CommandRepeat, "idem-repeat-1", viewer))
	if err != nil {
		t.Fatalf("the first repeat: %v", err)
	}
	// Branching is what Repeat is for, so the command is still offered after one
	// repeat has already created a Job.
	requireCommandKeys(t, "a repeated job", h.advertise(successful.ID, viewer),
		CommandPin, CommandUnpin, CommandPinLineage, CommandDismiss, CommandForget, CommandRepeat, "inspect")
	second, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(successful.ID, CommandRepeat, "idem-repeat-2", viewer))
	if err != nil {
		t.Fatalf("the second repeat: %v", err)
	}

	if first.SuccessorID == second.SuccessorID || first.SuccessorID == "" {
		t.Fatalf("the two repeats named successors %q and %q, want two distinct jobs",
			first.SuccessorID, second.SuccessorID)
	}
	branches := jobLinks(t, h.deps, successful.ID, LinkRepeatOf, false)
	if len(branches) != 2 {
		t.Fatalf("the repeated job has %d repeats, want two branches", len(branches))
	}
	if retries := jobLinks(t, h.deps, successful.ID, LinkRetryOf, false); len(retries) != 0 {
		t.Fatalf("a repeat recorded %d retry links, want none", len(retries))
	}

	// Each branch is an ordinary queued Job carrying the same input, and its own
	// lineage read names the Job it repeated.
	for _, successorID := range []string{first.SuccessorID, second.SuccessorID} {
		successor := jobRow(t, h.deps, successorID)
		if successor.State != string(StateQueued) {
			t.Fatalf("repeat %s is %s, want queued", successorID, successor.State)
		}
		if ancestors := jobLinks(t, h.deps, successorID, LinkRepeatOf, true); len(ancestors) != 1 || ancestors[0] != successful.ID {
			t.Fatalf("repeat %s names ancestors %v, want the repeated job", successorID, ancestors)
		}
		opened, err := h.svc.OpenReplay(h.deps, viewer, successorID)
		if err != nil {
			t.Fatalf("opening repeat %s: %v", successorID, err)
		}
		if string(opened.Input) != commandKindInput {
			t.Fatalf("repeat %s carries %s, want the repeated job's input", successorID, opened.Input)
		}
		lineage, err := h.svc.Lineage(h.deps, viewer, successorID)
		if err != nil {
			t.Fatalf("lineage of repeat %s: %v", successorID, err)
		}
		if len(lineage.Ancestors) != 1 || lineage.Ancestors[0].ID != successful.ID {
			t.Fatalf("repeat %s reads %d ancestors, want the repeated job", successorID, len(lineage.Ancestors))
		}
	}

	// The repeated Job keeps its outcome and its identity: a Repeat runs the work
	// again, it does not rewrite what already succeeded.
	if after := jobRow(t, h.deps, successful.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("the repeated job changed:\n before: %+v\n after:  %+v", before, after)
	}
}

// preferenceRow reads one viewer's preference row for one Job, or nil when the
// viewer has none — which is the same thing as having cleared both of its halves.
func preferenceRow(t *testing.T, deps Deps, jobID string, userID uint) *models.JobPreference {
	t.Helper()
	var row models.JobPreference
	err := deps.DB.Where("job_id = ? AND user_id = ?", jobID, userID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		t.Fatalf("load preference for %s: %v", jobID, err)
	}
	return &row
}

// pinnedJobIDs lists the Jobs one viewer has pinned.
func pinnedJobIDs(t *testing.T, deps Deps, userID uint) []string {
	t.Helper()
	var ids []string
	if err := deps.DB.Model(&models.JobPreference{}).
		Where("user_id = ? AND pinned_at IS NOT NULL", userID).Order("job_id").Pluck("job_id", &ids).Error; err != nil {
		t.Fatalf("list pins for %d: %v", userID, err)
	}
	return ids
}

// TestDismissAndPinChangeOnlyTheViewersOwnRelationship is §9's split between the
// two: dismissal is one viewer's view of one list and changes nothing about the
// Job, while pinning is a fact about the Job that exempts its metadata and events
// from retention for everybody.
func TestDismissAndPinChangeOnlyTheViewersOwnRelationship(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}
	admin := Access{UserID: 3, Administrator: true}

	finished := h.acceptReplayable(&owner)
	h.fail(finished.ID)

	listed := func(access Access, dismissed bool) []string {
		t.Helper()
		return pageIDs(listFor(t, h.svc, h.deps, access, Filter{Dismissed: boolPtr(dismissed)}, Cursor{}, 0))
	}
	if !slices.Contains(listed(viewer, false), finished.ID) {
		t.Fatal("a finished job is missing from its owner's default list")
	}

	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(finished.ID, CommandDismiss, "idem-dismiss", viewer))
	if err != nil {
		t.Fatalf("dismissing a finished job: %v", err)
	}
	requireResult(t, "a dismissal", result, CommandStatusSucceeded, CommandCodeApplied)

	if slices.Contains(listed(viewer, false), finished.ID) {
		t.Fatal("the dismissed job is still in the default list it was dismissed from")
	}
	if !slices.Contains(listed(viewer, true), finished.ID) {
		t.Fatal("the dismissed job is not in the list of dismissed jobs")
	}
	// Nobody else's list moved: a dismissal belongs to the viewer who asked for it.
	if !slices.Contains(listed(admin, false), finished.ID) {
		t.Fatal("one viewer's dismissal hid the job from an administrator's default list")
	}

	// Pinning is the other kind of fact: it exempts the Job's metadata and events
	// from ordinary retention for as long as anybody holds it.
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(finished.ID, CommandPin, "idem-pin", viewer))
	if err != nil {
		t.Fatalf("pinning a job: %v", err)
	}
	requireResult(t, "a pin", result, CommandStatusSucceeded, CommandCodeApplied)
	if pinned := pinnedJobIDs(t, h.deps, owner); len(pinned) != 1 || pinned[0] != finished.ID {
		t.Fatalf("pins = %v, want the pinned job", pinned)
	}
	if !result.Job.Pinned {
		t.Fatal("successful pin result does not show the viewer's pin")
	}
	viewerSnapshot, err := h.svc.Get(h.deps, viewer, finished.ID)
	if err != nil {
		t.Fatalf("read pinned job as its viewer: %v", err)
	}
	if !viewerSnapshot.Pinned {
		t.Fatal("the viewer's job snapshot does not show its pin")
	}
	otherSnapshot, err := h.svc.Get(h.deps, Access{UserID: 8, Administrator: true}, finished.ID)
	if err != nil {
		t.Fatalf("read pinned job as another viewer: %v", err)
	}
	if otherSnapshot.Pinned {
		t.Fatal("one viewer's pin leaked into another viewer's snapshot")
	}
	page, err := h.svc.List(h.deps, viewer, Filter{}, Cursor{}, 10)
	if err != nil {
		t.Fatalf("list jobs with pin state: %v", err)
	}
	foundPinned := false
	for _, snapshot := range page.Jobs {
		if snapshot.ID == finished.ID {
			foundPinned = snapshot.Pinned
		}
	}
	if !foundPinned {
		t.Fatal("the viewer's list snapshot does not show its pin")
	}
	otherPage, err := h.svc.List(h.deps, Access{UserID: 8, Administrator: true}, Filter{}, Cursor{}, 10)
	if err != nil {
		t.Fatalf("list jobs as another viewer: %v", err)
	}
	for _, snapshot := range otherPage.Jobs {
		if snapshot.ID == finished.ID && snapshot.Pinned {
			t.Fatal("one viewer's pin leaked into another viewer's list snapshot")
		}
	}
	commands, err := h.svc.AdvertisedCommands(context.Background(), h.deps, viewer, finished.ID)
	if err != nil {
		t.Fatalf("read pinned command surface: %v", err)
	}
	if !hasCommand(commands, CommandUnpin) || !hasCommand(commands, CommandPin) {
		t.Fatalf("pinned command surface = %#v, want both idempotent bulk commands", commands)
	}
	if !slices.Contains(pageIDs(listFor(t, h.svc, h.deps, viewer, Filter{Command: CommandUnpin}, Cursor{}, 0)), finished.ID) {
		t.Fatal("a pinned job is missing from the Unpin command filter")
	}

	result, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(finished.ID, CommandUnpin, "idem-unpin", viewer))
	if err != nil {
		t.Fatalf("unpinning a job: %v", err)
	}
	requireResult(t, "an unpin", result, CommandStatusSucceeded, CommandCodeApplied)
	if result.Job.Pinned {
		t.Fatal("successful unpin result still shows the viewer's pin")
	}
	viewerSnapshot, err = h.svc.Get(h.deps, viewer, finished.ID)
	if err != nil {
		t.Fatalf("read unpinned job: %v", err)
	}
	if viewerSnapshot.Pinned {
		t.Fatal("the viewer's pin remained after Unpin")
	}
	commands, err = h.svc.AdvertisedCommands(context.Background(), h.deps, viewer, finished.ID)
	if err != nil {
		t.Fatalf("read unpinned command surface: %v", err)
	}
	if !hasCommand(commands, CommandUnpin) || !hasCommand(commands, CommandPin) {
		t.Fatalf("unpinned command surface = %#v, want both idempotent bulk commands", commands)
	}
	if !slices.Contains(pageIDs(listFor(t, h.svc, h.deps, viewer, Filter{Command: CommandUnpin}, Cursor{}, 0)), finished.ID) {
		t.Fatal("an unpinned job is missing from the idempotent Unpin bulk command filter")
	}
	// Pinning again leaves the test's retention assertion below meaningful.
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(finished.ID, CommandPin, "idem-pin-again", viewer))
	if err != nil {
		t.Fatalf("pinning a job again: %v", err)
	}
	requireResult(t, "a second pin", result, CommandStatusSucceeded, CommandCodeApplied)

	// The Job's own deadline is what retention reads, so a pin is only worth
	// anything if the sweep honors it: an expired but pinned Job stays.
	if err := h.deps.DB.Model(&models.Job{}).Where("id = ?", finished.ID).
		Update("expires_at", h.now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("age the job's deadline: %v", err)
	}
	if _, err := h.svc.Sweep(h.deps, RetentionPolicy{}, SweepCursor{}, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := h.svc.Get(h.deps, viewer, finished.ID); err != nil {
		t.Fatalf("a pinned job was swept: %v", err)
	}
}

func hasCommand(commands []Command, key string) bool {
	for _, command := range commands {
		if command.Key == key {
			return true
		}
	}
	return false
}

// TestLineagePinningCoversVisibleRelativesAndNothingHidden is §9's "Pinning one
// Job does not pin related Jobs" read the other way round: the explicit lineage
// pin covers one hop of the relatives the asker may see, and a relative that is
// hidden is neither pinned nor named — lineage grants no transitive visibility.
func TestLineagePinningCoversVisibleRelativesAndNothingHidden(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}
	admin := Access{UserID: 3, Administrator: true}

	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)
	// The administrator retries the owner's Job, so the successor belongs to the
	// administrator and the owner cannot see it at all.
	retried, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(ancestor.ID, CommandRetry, "idem-admin-retry", admin))
	if err != nil {
		t.Fatalf("the administrator's retry: %v", err)
	}
	if _, err := h.svc.Get(h.deps, viewer, retried.SuccessorID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the owner can read the administrator's successor: %v", err)
	}

	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(ancestor.ID, CommandPinLineage, "idem-lineage-own", viewer))
	if err != nil {
		t.Fatalf("pinning a visible lineage: %v", err)
	}
	requireResult(t, "a lineage pin", result, CommandStatusSucceeded, CommandCodeApplied)

	var detail struct {
		Pinned  []string `json:"pinned"`
		Refused []struct {
			JobID  string `json:"jobId"`
			Reason string `json:"reason"`
		} `json:"refused"`
	}
	if err := json.Unmarshal(result.Detail, &detail); err != nil {
		t.Fatalf("the lineage pin detail is not readable: %v", err)
	}
	if len(detail.Pinned) != 1 || detail.Pinned[0] != ancestor.ID {
		t.Fatalf("the owner pinned %v, want only their own job", detail.Pinned)
	}
	if strings.Contains(string(result.Detail), retried.SuccessorID) {
		t.Fatalf("the hidden successor is named in the result: %s", result.Detail)
	}
	if pinned := pinnedJobIDs(t, h.deps, owner); len(pinned) != 1 || pinned[0] != ancestor.ID {
		t.Fatalf("the owner's pins = %v, want only their own job", pinned)
	}

	// The same command as the administrator reaches both Jobs: each relative is
	// authorized on its own, and both are visible to an administrator.
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(retried.SuccessorID, CommandPinLineage, "idem-lineage-admin", admin))
	if err != nil {
		t.Fatalf("pinning an administrator's lineage: %v", err)
	}
	if err := json.Unmarshal(result.Detail, &detail); err != nil {
		t.Fatalf("the lineage pin detail is not readable: %v", err)
	}
	if len(detail.Pinned) != 2 || !slices.Contains(detail.Pinned, ancestor.ID) || !slices.Contains(detail.Pinned, retried.SuccessorID) {
		t.Fatalf("the administrator pinned %v, want both relatives", detail.Pinned)
	}
}

// TestPinLineageReportsTheRelativesThePinLimitRefused is the bounded case: the
// per-viewer pin limit is still a limit when a command pins a whole lineage, and
// the relatives it refused are named rather than silently dropped — a partially
// applied command that says what it did not do.
func TestPinLineageReportsTheRelativesThePinLimitRefused(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(8)
	viewer := Access{UserID: owner}

	successful := h.acceptReplayable(&owner)
	h.succeed(successful.ID)
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(successful.ID, CommandRepeat, "idem-limit-repeat", viewer)); err != nil {
		t.Fatalf("repeating successful work: %v", err)
	}

	h.deps.PinLimit = 1
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(successful.ID, CommandPinLineage, "idem-limit-lineage", viewer))
	if err != nil {
		t.Fatalf("pinning a lineage at the limit: %v", err)
	}
	requireResult(t, "a lineage pin at the limit", result, CommandStatusSucceeded, CommandCodeApplied)

	var detail struct {
		Pinned  []string `json:"pinned"`
		Refused []struct {
			JobID  string `json:"jobId"`
			Reason string `json:"reason"`
		} `json:"refused"`
	}
	if err := json.Unmarshal(result.Detail, &detail); err != nil {
		t.Fatalf("the lineage pin detail is not readable: %v", err)
	}
	if len(detail.Pinned) != 1 || detail.Pinned[0] != successful.ID {
		t.Fatalf("the pin at the limit pinned %v, want the job itself", detail.Pinned)
	}
	if len(detail.Refused) != 1 || detail.Refused[0].Reason != "pin-limit" {
		t.Fatalf("the refusals = %+v, want one relative refused by the pin limit", detail.Refused)
	}
	if pinned := pinnedJobIDs(t, h.deps, owner); len(pinned) != 1 {
		t.Fatalf("pins at a limit of one = %v", pinned)
	}
}

// TestForgetThroughTheCommandPurgesInputAndKeepsHistory is §9's "Forget replay
// data removes the replay envelope without removing sanitized history": the
// ciphertext and its nonce are cleared, the durable reason is written, and the
// Job's own record and timeline stay exactly where they were.
func TestForgetThroughTheCommandPurgesInputAndKeepsHistory(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	finished := h.acceptReplayable(&owner)
	h.fail(finished.ID)
	timeline, err := h.svc.Timeline(h.deps, viewer, finished.ID, 0, 0)
	if err != nil {
		t.Fatalf("timeline before forgetting: %v", err)
	}

	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(finished.ID, CommandForget, "idem-forget", viewer))
	if err != nil {
		t.Fatalf("forgetting replay input: %v", err)
	}
	requireResult(t, "a forget", result, CommandStatusSucceeded, CommandCodeApplied)

	var envelope models.JobReplayEnvelope
	if err := h.deps.DB.Where("job_id = ?", finished.ID).First(&envelope).Error; err != nil {
		t.Fatalf("load the envelope: %v", err)
	}
	if envelope.PurgedAt == nil || envelope.PurgeReason != models.JobReplayPurgeForgotten {
		t.Fatalf("the purged envelope = %+v, want the forgotten marker", envelope)
	}
	if len(envelope.Ciphertext) != 0 || len(envelope.Nonce) != 0 {
		t.Fatalf("the purged envelope still carries %d ciphertext and %d nonce bytes",
			len(envelope.Ciphertext), len(envelope.Nonce))
	}
	if _, err := h.svc.OpenReplay(h.deps, viewer, finished.ID); !errors.Is(err, ErrReplayForgotten) {
		t.Fatalf("opening forgotten input = %v, want ErrReplayForgotten", err)
	}

	// The history is untouched: forgetting input is not deleting the Job.
	snap, err := h.svc.Get(h.deps, viewer, finished.ID)
	if err != nil {
		t.Fatalf("the job was removed with its input: %v", err)
	}
	if snap.State != StateFailed || snap.ReplayAvailability != ReplayForgotten {
		t.Fatalf("the job reads %s/%s, want a failed job whose input was forgotten", snap.State, snap.ReplayAvailability)
	}
	after, err := h.svc.Timeline(h.deps, viewer, finished.ID, 0, 0)
	if err != nil {
		t.Fatalf("timeline after forgetting: %v", err)
	}
	if len(after) != len(timeline) {
		t.Fatalf("forgetting changed the timeline from %d events to %d", len(timeline), len(after))
	}
}

// TestBulkCommandRecordsAnIndependentOutcomePerJob is §4's bulk contract: the
// command has to be offered in bulk for the Job it is about, every Job is
// resolved and refused on its own, and one Job's refusal never becomes the
// selection's.
func TestBulkCommandRecordsAnIndependentOutcomePerJob(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	first := h.acceptReplayable(&owner)
	h.fail(first.ID)
	second := h.acceptReplayable(&owner)
	h.fail(second.ID)

	bulk := func(key string, ids ...string) []CommandResult {
		t.Helper()
		return h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
			JobIDs: ids, Key: key, IdempotencyKey: "idem-bulk", Actor: viewer, Origin: "api",
		})
	}

	results := bulk(CommandDismiss, first.ID, second.ID)
	if len(results) != 2 {
		t.Fatalf("a bulk dismissal answered %d results, want one per job", len(results))
	}
	for index, expected := range []string{first.ID, second.ID} {
		if results[index].JobID != expected {
			t.Fatalf("result %d is about %s, want %s in the order the selection named them",
				index, results[index].JobID, expected)
		}
		requireResult(t, "a bulk dismissal", results[index], CommandStatusSucceeded, CommandCodeApplied)
		if row := preferenceRow(t, h.deps, expected, owner); row == nil || row.DismissedAt == nil {
			t.Fatalf("job %s was not dismissed by the bulk command", expected)
		}
	}

	// A Job that does not offer the command is refused for itself, and the rest of
	// the selection still happens: a client that selected one unsuitable Job has
	// not lost the other nine.
	queued := h.acceptReplayable(&owner)
	missing := "8f0c2b1e-0000-7000-8000-000000000000"
	results = bulk(CommandPin, second.ID, queued.ID, missing)
	if len(results) != 3 {
		t.Fatalf("a mixed bulk answered %d results, want three", len(results))
	}
	requireResult(t, "the finished job of a mixed bulk", results[0], CommandStatusSucceeded, CommandCodeApplied)
	requireResult(t, "the queued job of a mixed bulk", results[1], CommandStatusSucceeded, CommandCodeApplied)
	requireResult(t, "the missing job of a mixed bulk", results[2], CommandStatusFailed, CommandCodeNotFound)
	if results[2].JobID != missing {
		t.Fatalf("the refusal is about %q, want the job it was about", results[2].Job.ID)
	}

	// A destructive command the host does not offer in bulk is refused for every
	// selected Job even though every one of them offers it per Job — the Jobs that
	// can act are the selection's intersection with the Jobs that offer the command
	// *in bulk*, and the refusal is per Job like every other.
	results = bulk(CommandForget, second.ID, first.ID)
	for index, result := range results {
		requireResult(t, "a command the host does not offer in bulk", result, CommandStatusFailed, CommandCodeNotAdvertised)
		if result.Job.ID == "" || result.Job.ID != []string{second.ID, first.ID}[index] {
			t.Fatalf("the refusal is about %q, want the job it was about", result.Job.ID)
		}
	}
	for _, jobID := range []string{first.ID, second.ID} {
		var envelope models.JobReplayEnvelope
		if err := h.deps.DB.Where("job_id = ?", jobID).First(&envelope).Error; err != nil {
			t.Fatalf("load envelope for %s: %v", jobID, err)
		}
		if envelope.PurgedAt != nil {
			t.Fatalf("a bulk forget that was refused purged job %s's input", jobID)
		}
	}

	// A command the Job offers but not in bulk is refused as a bulk command, and no
	// executor is asked about it.
	h.claim(queued.ID)
	before := h.adapter.commandCount()
	results = bulk(CommandCancel, queued.ID)
	if len(results) != 1 {
		t.Fatalf("a bulk cancel answered %d results, want one", len(results))
	}
	requireResult(t, "a command that is not bulk", results[0], CommandStatusFailed, CommandCodeNotAdvertised)
	if h.adapter.commandCount() != before {
		t.Fatal("a command refused as non-bulk still reached the executor")
	}

	// The ceiling refuses the Jobs past it rather than dropping them: a bulk command
	// is one transaction and one outcome per Job, so its width is bounded.
	ids := make([]string, 0, MaxBulkCommandJobs+1)
	ids = append(ids, first.ID)
	for i := 1; i <= MaxBulkCommandJobs; i++ {
		ids = append(ids, fmt.Sprintf("8f0c2b1e-%04d-7000-8000-%012d", i, i))
	}
	results = h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: ids, Key: CommandPin, IdempotencyKey: "idem-bulk-wide", Actor: viewer, Origin: "api",
	})
	if len(results) != len(ids) {
		t.Fatalf("a request past the ceiling answered %d results for %d jobs", len(results), len(ids))
	}
	if last := results[len(results)-1]; last.Status != CommandStatusFailed || last.Code != CommandCodeInvalid {
		t.Fatalf("the job past the ceiling = %s/%s, want an individual refusal", last.Status, last.Code)
	}

	// A malformed request has no Job to answer about, so it is answered once rather
	// than once per name; a request naming no Jobs is answered with nothing.
	results = h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{first.ID, second.ID}, Key: "", IdempotencyKey: "idem-bulk-bad", Actor: viewer,
	})
	if len(results) != 1 || results[0].Code != CommandCodeInvalid {
		t.Fatalf("a malformed bulk answered %+v, want one invalid-request result", results)
	}
	if results := h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		Key: CommandPin, IdempotencyKey: "idem-bulk-empty", Actor: viewer,
	}); len(results) != 0 {
		t.Fatalf("an empty selection answered %d results", len(results))
	}
}

// TestBulkCommandReplaysBeforeCheckingCurrentAdvertisement covers a command
// whose successful effect removes that command from the Job's current
// advertisement. A repeated bulk request must return the stored per-Job result
// before asking whether the command could be run again now.
func TestBulkCommandReplaysBeforeCheckingCurrentAdvertisement(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	first := h.acceptReplayable(&owner)
	second := h.acceptReplayable(&owner)

	// The first Job allows bulk cancellation while queued. The second advertises
	// the same command only for individual use, so this selection has both a
	// successful per-Job result and an independent bulk refusal.
	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		if commandContext.Snapshot.ID == first.ID && commandContext.Snapshot.State == StateQueued {
			return []Command{{Key: CommandCancel, Label: "Cancel", Bulk: true}}, nil
		}
		if commandContext.Snapshot.ID == second.ID && commandContext.Snapshot.State == StateQueued {
			return []Command{{Key: CommandCancel, Label: "Cancel"}}, nil
		}
		return nil, nil
	}

	bulk := func(origin string) []CommandResult {
		t.Helper()
		return h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
			JobIDs: []string{first.ID, second.ID}, Key: CommandCancel,
			IdempotencyKey: "idem-bulk-cancel-replay", Actor: viewer, Origin: origin,
		})
	}

	results := bulk("api")
	if len(results) != 2 {
		t.Fatalf("the first bulk cancellation answered %d results, want two", len(results))
	}
	requireResult(t, "the eligible Job's bulk cancellation", results[0], CommandStatusSucceeded, CommandCodeApplied)
	requireResult(t, "the individually advertised Job's bulk cancellation", results[1], CommandStatusFailed, CommandCodeNotAdvertised)
	if got := State(jobRow(t, h.deps, first.ID).State); got != StateCancelled {
		t.Fatalf("the first Job is %s after cancellation, want cancelled", got)
	}
	if got := State(jobRow(t, h.deps, second.ID).State); got != StateQueued {
		t.Fatalf("the independently refused Job is %s, want queued", got)
	}

	// The first Job no longer advertises Cancel. Replaying the same key must use
	// its recorded result, while the second Job's bulk refusal remains local to
	// that entry.
	results = bulk("api")
	if len(results) != 2 {
		t.Fatalf("the repeated bulk cancellation answered %d results, want two", len(results))
	}
	requireResult(t, "the replayed cancellation", results[0], CommandStatusSucceeded, CommandCodeApplied)
	if results[0].Job.State != StateCancelled || results[0].Job.Version != jobRow(t, h.deps, first.ID).Version {
		t.Fatalf("the replayed result has stale Job snapshot %+v", results[0].Job)
	}
	requireResult(t, "the repeated independent bulk refusal", results[1], CommandStatusFailed, CommandCodeNotAdvertised)

	if rows := commandRequestRows(t, h.deps, first.ID); len(rows) != 1 {
		t.Fatalf("the successful Job has %d recorded command requests, want one", len(rows))
	}
	if rows := commandRequestRows(t, h.deps, second.ID); len(rows) != 0 {
		t.Fatalf("the refused Job has %d recorded command requests, want none", len(rows))
	}

	// The same tuple remains the same request across the single and bulk
	// surfaces, and a fresh version does not change that identity.
	single, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(first.ID, CommandCancel, "idem-bulk-cancel-replay", viewer))
	if err != nil {
		t.Fatalf("replaying the bulk request through the single-command surface: %v", err)
	}
	requireResult(t, "the cross-surface replay", single, CommandStatusSucceeded, CommandCodeApplied)

	// A different key is a new attempt, so current eligibility still applies.
	differentKey := h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{first.ID}, Key: CommandCancel,
		IdempotencyKey: "idem-bulk-cancel-new", Actor: viewer, Origin: "api",
	})
	if len(differentKey) != 1 || differentKey[0].Status != CommandStatusFailed || differentKey[0].Code != CommandCodeNotAdvertised {
		t.Fatalf("a different key after cancellation answered %+v, want a fresh non-advertised refusal", differentKey)
	}

	// Origin is part of the request hash, while ExpectedVersion is deliberately
	// absent from it. A changed origin cannot borrow the recorded result.
	results = bulk("cli")
	if len(results) != 2 || results[0].Status != CommandStatusFailed || results[0].Code != CommandCodeKeyReused {
		t.Fatalf("reusing the key with a different origin answered %+v, want a key-reused refusal for the first Job", results)
	}
	if results[1].Code != CommandCodeNotAdvertised {
		t.Fatalf("the second Job's independent refusal changed to %q", results[1].Code)
	}

	// Visibility is checked before a durable result is exposed, so a different
	// viewer still receives the same missing answer as for an unknown Job.
	hidden := h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{first.ID}, Key: CommandCancel,
		IdempotencyKey: "idem-bulk-cancel-replay", Actor: Access{UserID: owner + 1}, Origin: "api",
	})
	if len(hidden) != 1 || hidden[0].Code != CommandCodeNotFound || hidden[0].Job.ID != "" {
		t.Fatalf("a viewer without access received %+v for the recorded command, want a not-found result without a Job snapshot", hidden)
	}
}

// TestSingleCommandOutcomeDoesNotBecomeBulkEligibleOnReplay verifies that an
// idempotent replay does not let a single-only command cross the bulk boundary.
// A bulk-origin outcome remains reusable by the single surface, which preserves
// the lost-response case when a client retries through a different route.
func TestSingleCommandOutcomeDoesNotBecomeBulkEligibleOnReplay(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	singleOnly := h.acceptReplayable(&owner)
	bulkCapable := h.acceptReplayable(&owner)

	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		return []Command{{Key: "inspect", Label: "Inspect", Bulk: commandContext.Snapshot.ID == bulkCapable.ID}}, nil
	}

	single, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(singleOnly.ID, "inspect", "idem-single-only", viewer))
	if err != nil {
		t.Fatalf("the single-only command failed: %v", err)
	}
	requireResult(t, "the single-only command", single, CommandStatusSucceeded, CommandCodeApplied)

	bulk := h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{singleOnly.ID}, Key: "inspect", IdempotencyKey: "idem-single-only",
		Actor: viewer, Origin: "api",
	})
	if len(bulk) != 1 || bulk[0].Status != CommandStatusFailed || bulk[0].Code != CommandCodeNotAdvertised {
		t.Fatalf("replaying the single-only command as bulk answered %+v, want a bulk eligibility refusal", bulk)
	}

	// The failed mode crossing does not alter the recorded request or block its
	// original single-command replay.
	single, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(singleOnly.ID, "inspect", "idem-single-only", viewer))
	if err != nil {
		t.Fatalf("replaying the single-only command through its original surface: %v", err)
	}
	requireResult(t, "the repeated single-only command", single, CommandStatusSucceeded, CommandCodeApplied)
	if rows := commandRequestRows(t, h.deps, singleOnly.ID); len(rows) != 1 || rows[0].BulkEligible {
		t.Fatalf("single-only command has recorded requests %+v, want one non-bulk-eligible request", rows)
	}

	// A command that was bulk-capable when first executed can replay across to
	// the single surface even after a response was lost.
	bulk = h.svc.ExecuteBulkCommand(context.Background(), h.deps, BulkCommandRequest{
		JobIDs: []string{bulkCapable.ID}, Key: "inspect", IdempotencyKey: "idem-bulk-origin",
		Actor: viewer, Origin: "api",
	})
	if len(bulk) != 1 {
		t.Fatalf("the bulk-capable command answered %d results, want one", len(bulk))
	}
	requireResult(t, "the bulk-capable command", bulk[0], CommandStatusSucceeded, CommandCodeApplied)
	if rows := commandRequestRows(t, h.deps, bulkCapable.ID); len(rows) != 1 || !rows[0].BulkEligible {
		t.Fatalf("bulk-capable command has recorded requests %+v, want one bulk-eligible request", rows)
	}
	single, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(bulkCapable.ID, "inspect", "idem-bulk-origin", viewer))
	if err != nil {
		t.Fatalf("replaying the bulk command through the single surface: %v", err)
	}
	requireResult(t, "the bulk-to-single replay", single, CommandStatusSucceeded, CommandCodeApplied)
	if h.adapter.commandCount() != 2 {
		t.Fatalf("the adapter ran %d times, want one execution for each Job", h.adapter.commandCount())
	}
}

// TestCommandRefusesRequestsOutsideItsBounds is the boundary half of the command
// contract: a malformed request is refused before anything is read, nothing is
// recorded for it, and no executor ever hears about it.
func TestCommandRefusesRequestsOutsideItsBounds(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	job := h.acceptReplayable(&owner)
	h.fail(job.ID)
	sound := h.request(job.ID, CommandRetry, "idem-sound", viewer)

	cases := map[string]CommandRequest{
		"no job id":          {Key: CommandRetry, IdempotencyKey: "idem", ExpectedVersion: 1, Actor: viewer},
		"no key":             {JobID: job.ID, IdempotencyKey: "idem", ExpectedVersion: 1, Actor: viewer},
		"no idempotency key": {JobID: job.ID, Key: CommandRetry, ExpectedVersion: 1, Actor: viewer},
		"a blank idempotency key": func() CommandRequest {
			request := sound
			request.IdempotencyKey = "   "
			return request
		}(),
		"no expected version": {JobID: job.ID, Key: CommandRetry, IdempotencyKey: "idem", Actor: viewer},
		"an oversized key": func() CommandRequest {
			request := sound
			request.Key = strings.Repeat("k", MaxCommandKeyBytes+1)
			return request
		}(),
		"an oversized idempotency key": func() CommandRequest {
			request := sound
			request.IdempotencyKey = strings.Repeat("i", MaxIdempotencyKeyBytes+1)
			return request
		}(),
		"an oversized origin": func() CommandRequest {
			request := sound
			request.Origin = strings.Repeat("o", MaxOriginBytes+1)
			return request
		}(),
	}
	for name, request := range cases {
		if _, err := h.svc.ExecuteCommand(context.Background(), h.deps, request); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("a command with %s = %v, want ErrInvalidCommand", name, err)
		}
	}
	if rows := commandRequestRows(t, h.deps, job.ID); len(rows) != 0 {
		t.Fatalf("%d command requests were recorded for refused requests", len(rows))
	}
	if h.adapter.commandCount() != 0 {
		t.Fatal("a malformed request reached the adapter")
	}

	// A bulk request is checked once, before it is expanded: the same malformed
	// fields are refused without reading a single Job.
	for name, request := range map[string]BulkCommandRequest{
		"no key":             {JobIDs: []string{job.ID}, IdempotencyKey: "idem", Actor: viewer},
		"no idempotency key": {JobIDs: []string{job.ID}, Key: CommandRetry, Actor: viewer},
		"an oversized key": {
			JobIDs: []string{job.ID}, Key: strings.Repeat("k", MaxCommandKeyBytes+1),
			IdempotencyKey: "idem", Actor: viewer,
		},
	} {
		results := h.svc.ExecuteBulkCommand(context.Background(), h.deps, request)
		if len(results) != 1 || results[0].Code != CommandCodeInvalid {
			t.Errorf("a bulk command with %s answered %+v, want one invalid-request result", name, results)
		}
	}
}

// TestPauseAndResumeFollowTheExecutorThatConfirmsTheCheckpoint is §4's pause/resume
// pair at the seam a client meets them: a pause is durable before the executor is
// told, the Job keeps running until that executor reaches the checkpoint and
// publishes it, work nobody is running is offered no pause at all, and a resume
// the executor accepts returns the held Job to the queue — the one state change
// nothing else can publish, because a held Job has no execution left to do it.
func TestPauseAndResumeFollowTheExecutorThatConfirmsTheCheckpoint(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	running := h.acceptReplayable(&owner)
	execution := h.claim(running.ID)

	var observed models.Job
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		observed = jobRow(t, h.deps, running.ID)
		return CommandOutcome{Status: CommandStatusSucceeded, Message: "reaching a checkpoint"}, nil
	}
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandPause, "idem-pause", viewer))
	if err != nil {
		t.Fatalf("pausing a running job: %v", err)
	}
	requireResult(t, "a pause request", result, CommandStatusSucceeded, CommandCodeRequested)
	if observed.ControlIntent != ControlIntentPause || observed.Phase != PhasePausing {
		t.Fatalf("the executor was asked with intent %q phase %q, want a durable pause first",
			observed.ControlIntent, observed.Phase)
	}
	if jobRow(t, h.deps, running.ID).State != string(StateRunning) {
		t.Fatal("a job whose pause is in flight is not running any more")
	}

	// The checkpoint is the execution's to reach and publish, and reaching it
	// resolves the intent it was asked for.
	paused, err := h.svc.Transition(h.deps, Transition{
		JobID:           running.ID,
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		ExecutionToken:  execution.ExecutionToken,
		To:              StatePaused,
	})
	if err != nil {
		t.Fatalf("publishing the pause: %v", err)
	}
	if paused.State != StatePaused || paused.ControlIntent != "" {
		t.Fatalf("the paused job = %s with intent %q, want paused and resolved", paused.State, paused.ControlIntent)
	}

	// Work nobody is running is offered no pause: there is no execution to confirm a
	// checkpoint, and holding waiting work is not what a pause means.
	waiting := h.acceptReplayable(&owner)
	requireCommandKeys(t, "work nobody is running", h.advertise(waiting.ID, viewer),
		CommandCancel, "inspect", CommandPin, CommandUnpin, CommandPinLineage)
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(waiting.ID, CommandPause, "idem-pause-waiting", viewer)); !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("pausing waiting work = %v, want ErrCommandNotAdvertised", err)
	}
	if row := jobRow(t, h.deps, waiting.ID); row.State != string(StateQueued) {
		t.Fatalf("the refused pause left the waiting job %s", row.State)
	}

	// The resume reaches the executor, and its acceptance is the one thing that can
	// put the held Job back where dispatch picks it up.
	var asked CommandExecution
	h.adapter.execute = func(_ context.Context, request CommandExecution) (CommandOutcome, error) {
		asked = request
		return CommandOutcome{Status: CommandStatusSucceeded, Message: "resumed"}, nil
	}
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandResume, "idem-resume", viewer))
	if err != nil {
		t.Fatalf("resuming held work: %v", err)
	}
	requireResult(t, "a resume", result, CommandStatusSucceeded, CommandCodeApplied)
	if asked.Key != CommandResume || asked.Snapshot.State != StatePaused {
		t.Fatalf("the executor was handed %+v, want the resume of a paused job", asked)
	}
	if row := jobRow(t, h.deps, running.ID); row.State != string(StateQueued) {
		t.Fatalf("the resumed job is %s, want queued for dispatch", row.State)
	}
	if _, ok := claimOnce(t, h.svc, h.deps, "runtime-resume"); !ok {
		t.Fatal("a resumed job was not claimable again")
	}
}

// TestCommandResumeIsNotOfferedOnceACancellationHasWon is §4's "once cancellation
// intent wins, later success cannot overwrite it" read at the command surface: a
// Job a cancellation owns ends cancelled, and the work it would newly hold is
// work that can never succeed again — so a resume is not offered for it, exactly
// as a pause is not.
//
// The path is the one a cancellation that cannot be carried out takes: §3 keeps
// such a Job nonterminal and blocked rather than assuming its external work has
// stopped, so a cancellation's intent can outlive the execution that owned it.
// Offering resume there would return the Job to the queue carrying an intent that
// refuses every success it then reaches.
func TestCommandResumeIsNotOfferedOnceACancellationHasWon(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	running := h.acceptReplayable(&owner)
	execution := h.claim(running.ID)
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{Status: CommandStatusSucceeded}, nil
	}
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandCancel, "idem-won-cancel", viewer)); err != nil {
		t.Fatalf("cancelling a running job: %v", err)
	}

	blocked, err := h.svc.Transition(h.deps, Transition{
		JobID:           running.ID,
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		ExecutionToken:  execution.ExecutionToken,
		To:              StateBlocked,
	})
	if err != nil {
		t.Fatalf("blocking a cancelling job: %v", err)
	}
	if blocked.State != StateBlocked || blocked.ControlIntent != ControlIntentCancel {
		t.Fatalf("the blocked job = %s with intent %q, want blocked and still cancelled",
			blocked.State, blocked.ControlIntent)
	}

	// The cancellation still owns the Job, so what it offers is the control that ends
	// it — and not the one that would put its work back in the queue.
	requireCommandKeys(t, "a blocked job whose cancellation has won", h.advertise(running.ID, viewer),
		CommandCancel, "inspect", CommandPin, CommandUnpin, CommandPinLineage)

	before := h.adapter.commandCount()
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandResume, "idem-resume-won", viewer)); !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("resuming a job a cancellation owns = %v, want ErrCommandNotAdvertised", err)
	}
	if h.adapter.commandCount() != before {
		t.Fatal("a refused resume reached the executor")
	}
	row := jobRow(t, h.deps, running.ID)
	if row.State != string(StateBlocked) || row.ControlIntent != ControlIntentCancel {
		t.Fatalf("the refused resume left the job %s with intent %q, want it where it was",
			row.State, row.ControlIntent)
	}
	if rows := commandRequestRows(t, h.deps, running.ID); len(rows) != 1 {
		t.Fatalf("a refused resume recorded %d command requests, want only the cancellation's", len(rows))
	}

	// What the refusal is for: once a cancellation owns a Job, no later success can
	// overwrite it, wherever the Job is sent in the meantime. Returning this Job to
	// the queue by hand — which is what the resume would have done — leaves work that
	// runs and can never publish its success, so refusing the command is the honest
	// answer rather than a lost control.
	queued, err := h.svc.Transition(h.deps, Transition{
		JobID:           running.ID,
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		To:              StateQueued,
	})
	if err != nil {
		t.Fatalf("returning the cancelled job to the queue: %v", err)
	}
	if queued.ControlIntent != ControlIntentCancel {
		t.Fatalf("a requeued job lost the cancellation that owns it (intent %q)", queued.ControlIntent)
	}
	resumed := h.claim(running.ID)
	if _, err := h.svc.Finish(h.deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: running.ID, ExecutionToken: resumed.ExecutionToken},
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		Outcome:         StateSucceeded,
	}); !errors.Is(err, ErrControlIntentWon) {
		t.Fatalf("a success after a requeue of a cancelled job = %v, want ErrControlIntentWon", err)
	}
}

// TestCommandPauseIntentIsResolvedWhenWorkLeavesRunning is the other half of the
// intent's lifetime: a pause request is confirmed by the execution that owns a
// running Job, so a Job that leaves running for anything but the checkpoint it
// asked for has no execution left to confirm one and the request can never reach
// its outcome. Resolving it there is what keeps a stored intent meaning "this is
// still outstanding" rather than "this was once asked for", which is what the
// command that resumes the Job is classified against.
func TestCommandPauseIntentIsResolvedWhenWorkLeavesRunning(t *testing.T) {
	h := newCommandHarness(t)
	h.advertiseStateful()
	owner := uint(7)
	viewer := Access{UserID: owner}

	running := h.acceptReplayable(&owner)
	execution := h.claim(running.ID)
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{Status: CommandStatusSucceeded}, nil
	}
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandPause, "idem-pause-then-block", viewer)); err != nil {
		t.Fatalf("pausing a running job: %v", err)
	}
	if intent := jobRow(t, h.deps, running.ID).ControlIntent; intent != ControlIntentPause {
		t.Fatalf("the pause request recorded intent %q, want %q", intent, ControlIntentPause)
	}

	// The execution blocks instead of reaching a resumable checkpoint.
	blocked, err := h.svc.Transition(h.deps, Transition{
		JobID:           running.ID,
		ExpectedVersion: jobRow(t, h.deps, running.ID).Version,
		ExecutionToken:  execution.ExecutionToken,
		To:              StateBlocked,
	})
	if err != nil {
		t.Fatalf("blocking a pausing job: %v", err)
	}
	if blocked.ControlIntent != "" {
		t.Fatalf("the blocked job kept the pause request it can never resolve (intent %q)", blocked.ControlIntent)
	}
	if blocked.Phase != "" {
		t.Fatalf("the blocked job kept the phase %q of a request nothing can follow through", blocked.Phase)
	}

	// The resume that unblocks it is applied rather than merely requested: the host is
	// the one that moved the Job, and there is no outstanding control left to report.
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandResume, "idem-resume-blocked", viewer))
	if err != nil {
		t.Fatalf("resuming blocked work: %v", err)
	}
	requireResult(t, "a resume of work whose pause never reached a checkpoint", result, CommandStatusSucceeded, CommandCodeApplied)
	if row := jobRow(t, h.deps, running.ID); row.State != string(StateQueued) || row.ControlIntent != "" {
		t.Fatalf("the resumed job is %s with intent %q, want queued and resolved", row.State, row.ControlIntent)
	}
}

// --- helpers shared by the later cycles -------------------------------------

// commandRequestRows is how many command requests are recorded for one Job. A
// refusal must record nothing at all, so the count is the assertion that says so.
func commandRequestRows(t *testing.T, deps Deps, jobID string) []models.JobCommandRequest {
	t.Helper()
	var rows []models.JobCommandRequest
	if err := deps.DB.Where("job_id = ?", jobID).Order("created_at, id").Find(&rows).Error; err != nil {
		t.Fatalf("load command requests for %s: %v", jobID, err)
	}
	return rows
}

// TestCommandExecutionRechecksWhatItDecidedFrom is §4's command-race contract at
// the module seam: a command the Job does not offer is refused with nothing
// written, a stale request is refused with the fresh snapshot it lost to, a Job
// the asker may not see is not-found, and an advertised workload command reaches
// its Kind's adapter under the asker's own access.
func TestCommandExecutionRechecksWhatItDecidedFrom(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}
	h.advertiseStateful()

	running := h.acceptReplayable(&owner)
	h.claim(running.ID)

	// A key the Job does not offer is refused without touching anything: not the
	// adapter, not the command table.
	result, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, "hang-up", "idem-refused", viewer))
	if !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("an unadvertised command = %v, want ErrCommandNotAdvertised", err)
	}
	requireResult(t, "an unadvertised command", result, CommandStatusFailed, CommandCodeNotAdvertised)
	if rows := commandRequestRows(t, h.deps, running.ID); len(rows) != 0 {
		t.Fatalf("a refused command recorded %d rows", len(rows))
	}
	if h.adapter.commandCount() != 0 {
		t.Fatal("a refused command reached the adapter")
	}

	// A pin is viewer state and does not move the Job version. A later stale
	// command must still return the current pin state with its fresh snapshot.
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandPin, "idem-pin-before-stale", viewer)); err != nil {
		t.Fatalf("pin before stale command: %v", err)
	}

	// A request decided from an older version is refused, and the answer carries
	// the Job as it stands now, including the viewer's pin state.
	stale := h.request(running.ID, CommandCancel, "idem-stale", viewer)
	stale.ExpectedVersion = running.Version
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, stale)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("a stale command = %v, want ErrVersionConflict", err)
	}
	requireResult(t, "a stale command", result, CommandStatusFailed, CommandCodeConflict)
	if result.Job.Version != jobRow(t, h.deps, running.ID).Version {
		t.Fatalf("the conflict returned version %d, want the job's current %d",
			result.Job.Version, jobRow(t, h.deps, running.ID).Version)
	}
	if !result.Job.Pinned {
		t.Fatal("the stale command conflict lost the viewer's pinned state")
	}

	// A Job the asker may not see is answered exactly as one that does not exist.
	if _, err := h.svc.ExecuteCommand(context.Background(), h.deps,
		h.request(running.ID, CommandCancel, "idem-foreign", Access{UserID: owner + 1})); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a command on a foreign job = %v, want ErrNotFound", err)
	}

	// An advertised workload command reaches the adapter, which is handed the Job,
	// the caller's key and the asker's own access — never the caller's claim about
	// who they are.
	h.adapter.execute = func(_ context.Context, execution CommandExecution) (CommandOutcome, error) {
		if execution.Access != viewer {
			t.Errorf("the adapter was handed access %+v, want %+v", execution.Access, viewer)
		}
		return CommandOutcome{
			Status:  CommandStatusSucceeded,
			Message: "stopping",
			Detail:  json.RawMessage(`{"phase":"stopping"}`),
		}, nil
	}
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-cancel", viewer))
	if err != nil {
		t.Fatalf("running an advertised command: %v", err)
	}
	requireResult(t, "an accepted cancellation request", result, CommandStatusSucceeded, CommandCodeRequested)
	if result.Message != "stopping" {
		t.Fatalf("the recorded message = %q, want the adapter's own bounded text", result.Message)
	}
	if string(result.Detail) != `{"phase":"stopping"}` {
		t.Fatalf("the recorded detail = %s, want the adapter's own", result.Detail)
	}

	last := h.adapter.lastCommand()
	if last.JobID != running.ID || last.Key != CommandCancel || last.IdempotencyKey != "idem-cancel" {
		t.Fatalf("the adapter was handed %+v, want the job, the key and the caller's idempotency key", last)
	}
	if last.Snapshot.ID != running.ID || last.Snapshot.State != StateRunning {
		t.Fatalf("the adapter was handed snapshot %+v, want the running job it owns", last.Snapshot)
	}

	// A failure the adapter reports is recorded as the command's outcome and
	// returned with it, so a caller that repeats the request is told what happened
	// rather than told to try again.
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{Status: CommandStatusFailed, Message: "the transfer already stopped"}, nil
	}
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-failed", viewer))
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("a failed command = %v, want ErrCommandFailed", err)
	}
	requireResult(t, "a failed command", result, CommandStatusFailed, CommandCodeFailed)
	if result.Message != "the transfer already stopped" {
		t.Fatalf("the recorded failure = %q, want the adapter's bounded text", result.Message)
	}

	// An adapter that returns an error is a failure like any other, and its text is
	// deliberately not carried: only the Kind knows what in its own error is safe.
	h.adapter.execute = func(context.Context, CommandExecution) (CommandOutcome, error) {
		return CommandOutcome{}, errors.New("Bearer secret-token was refused by the origin")
	}
	result, err = h.svc.ExecuteCommand(context.Background(), h.deps, h.request(running.ID, CommandCancel, "idem-error", viewer))
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("an adapter error = %v, want ErrCommandFailed", err)
	}
	requireResult(t, "an adapter error", result, CommandStatusFailed, CommandCodeFailed)
	for _, leak := range []string{"secret-token", "Bearer"} {
		if strings.Contains(result.Message, leak) || strings.Contains(string(result.Detail), leak) {
			t.Fatalf("the recorded outcome carries the adapter's error text (%q)", leak)
		}
	}
}

// requireResult compares one command result's classification.
func requireResult(t *testing.T, what string, result CommandResult, status, code string) {
	t.Helper()
	if result.Status != status || result.Code != code {
		t.Fatalf("%s = %s/%s, want %s/%s", what, result.Status, result.Code, status, code)
	}
}

// jobLinks lists one Job's lineage links of one type, with the far endpoint.
func jobLinks(t *testing.T, deps Deps, jobID string, linkType LinkType, from bool) []string {
	t.Helper()
	column := "to_job_id"
	if from {
		column = "from_job_id"
	}
	other := "from_job_id"
	if from {
		other = "to_job_id"
	}
	var ids []string
	if err := deps.DB.Model(&models.JobLink{}).
		Where(column+" = ? AND type = ?", jobID, string(linkType)).Order(other).
		Pluck(other, &ids).Error; err != nil {
		t.Fatalf("load %s links for %s: %v", linkType, jobID, err)
	}
	return ids
}

// concurrent runs one function twice at the same instant and reports both
// errors, which is how a race that only PostgreSQL or two SQLite connections can
// show is driven.
func concurrent(first, second func() error) []error {
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i, run := range []func() error{first, second} {
		wg.Add(1)
		go func(idx int, run func() error) {
			defer wg.Done()
			<-start
			errs[idx] = run()
		}(i, run)
	}
	close(start)
	wg.Wait()
	return errs
}

// describeErrors renders two concurrent results for a failure message.
func describeErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, fmt.Sprint(err))
	}
	return strings.Join(parts, " | ")
}

// TestACommandRechecksTheKindAdvertisementInsideItsTransaction is §4's atomic
// recheck, in the one place a version cannot cover.
//
// An advertisement is a statement about current policy — a plugin that is enabled,
// a registration that still exists, an artifact that is still there. Between the
// read that offered a command and the transaction that acts on it, any of those
// can change, and none of them moves the Job's version. So the Kind is asked again
// *inside* the transaction: a mutation committed on the strength of a decision
// that is no longer true is exactly the successor the Astra review found, one that
// could only block afterwards.
func TestACommandRechecksTheKindAdvertisementInsideItsTransaction(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}

	// The advertisement answers differently the second time it is asked, which is
	// what "policy changed between the page render and the click" looks like from
	// here. The first answer is the one the requester decided from.
	var calls int
	var mu sync.Mutex
	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			base := []Command{{Key: CommandCancel, Label: "Cancel"}, {Key: CommandRetry, Label: "Retry"}}
			return base, nil
		}
		return nil, nil
	}

	ancestor := h.acceptReplayable(&owner)
	h.fail(ancestor.ID)

	_, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(ancestor.ID, CommandRetry, "idem-revoked", viewer))
	if !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("a Retry the Kind stopped offering was applied (err = %v)", err)
	}
	successors, err := retrySuccessors(h.deps.DB, ancestor.ID)
	if err != nil {
		t.Fatalf("read the retry lineage: %v", err)
	}
	if len(successors) != 0 {
		t.Fatalf("a refused Retry still created %v", successors)
	}
}

// TestAControlIntentIsRecheckedInsideItsTransaction is the same seam for the other
// half of §4: a cancellation or pause persists a durable intent *before* the
// executor is told, and that record may not be written for a command the Kind no
// longer supports.
func TestAControlIntentIsRecheckedInsideItsTransaction(t *testing.T) {
	h := newCommandHarness(t)
	owner := uint(7)
	viewer := Access{UserID: owner}

	h.advertiseStateful()
	job := h.acceptReplayable(&owner)
	execution := h.claim(job.ID)

	// From here on the Kind offers nothing, which is what a plugin disabled or a
	// registration replaced looks like once the request has already been prepared.
	var calls int
	var mu sync.Mutex
	h.adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return []Command{{Key: CommandCancel, Label: "Cancel", Destructive: true}}, nil
		}
		return nil, nil
	}

	_, err := h.svc.ExecuteCommand(context.Background(), h.deps, h.request(job.ID, CommandCancel, "idem-cancel-revoked", viewer))
	if !errors.Is(err, ErrCommandNotAdvertised) {
		t.Fatalf("a cancellation the Kind stopped offering was recorded (err = %v)", err)
	}
	row := jobRow(t, h.deps, job.ID)
	if row.ControlIntent != "" {
		t.Fatalf("the Job records control intent %q for a command the Kind refused", row.ControlIntent)
	}
	if row.State != string(StateRunning) || row.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the Job moved to %s under token %q", row.State, row.ExecutionToken)
	}
}
