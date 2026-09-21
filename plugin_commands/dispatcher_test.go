package plugin_commands

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type dispatcherTestSettings struct {
	pending int
	quota   int64
}

func (s dispatcherTestSettings) StagingRoot() string        { return "/staging" }
func (s dispatcherTestSettings) PendingPerPluginLimit() int { return s.pending }
func (s dispatcherTestSettings) PerRunQuota() int64 {
	if s.quota > 0 {
		return s.quota
	}
	return 1 << 30
}
func (s dispatcherTestSettings) GlobalStagingQuota() int64        { return 1 << 31 }
func (s dispatcherTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s dispatcherTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s dispatcherTestSettings) CommandPath() string              { return "/bin" }

type dispatcherTestStore struct {
	mu                     sync.Mutex
	runs                   map[string]RunRecord
	bootSessionIDs         map[string]string
	finishes               map[string]RunFinish
	importFinishes         map[string]ImportFinish
	cancelled              []string
	markRunErr             error
	finishRunErr           error
	finishRunStarted       chan struct{}
	finishRunRelease       <-chan struct{}
	terminalRunReadStarted chan struct{}
	terminalRunReadRelease <-chan struct{}
	terminalRunReadOnce    sync.Once
	finishImportErr        error
	finishImportCalls      int
	interruptImportsErr    error
}

func newDispatcherTestStore() *dispatcherTestStore {
	return &dispatcherTestStore{
		runs: map[string]RunRecord{}, bootSessionIDs: map[string]string{}, finishes: map[string]RunFinish{}, importFinishes: map[string]ImportFinish{},
	}
}
func (s *dispatcherTestStore) CreateRun(r RunRecord, _ RunOutput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; ok {
		return errors.New("duplicate")
	}
	s.runs[r.ID] = r
	return nil
}
func (s *dispatcherTestStore) MarkRunRunning(id string, started time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markRunErr != nil {
		return false, s.markRunErr
	}
	record, ok := s.runs[id]
	if !ok || record.Status != RunStatusQueued {
		return false, nil
	}
	record.Status = RunStatusRunning
	record.StartedAt = &started
	s.runs[id] = record
	return true, nil
}
func (s *dispatcherTestStore) SetRunProcessGroup(id string, pgid int, bootSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[id]
	if !ok || record.Status != RunStatusRunning || record.ProcessGroupID != nil {
		return errors.New("run is not awaiting a process group")
	}
	record.ProcessGroupID = &pgid
	s.runs[id] = record
	if s.bootSessionIDs == nil {
		s.bootSessionIDs = make(map[string]string)
	}
	s.bootSessionIDs[id] = bootSessionID
	return nil
}
func (s *dispatcherTestStore) RequestRunCancel(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[id]
	if !ok {
		return ErrRunNotFound
	}
	if RunStatusTerminal(record.Status) || record.CancelRequested {
		return ErrRunNotCancellable
	}
	record.CancelRequested = true
	record.Error = reason
	s.runs[id] = record
	s.cancelled = append(s.cancelled, id)
	return nil
}
func (s *dispatcherTestStore) FinishRun(id string, f RunFinish) (bool, error) {
	if s.finishRunStarted != nil {
		select {
		case s.finishRunStarted <- struct{}{}:
		default:
		}
	}
	if s.finishRunRelease != nil {
		<-s.finishRunRelease
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finishRunErr != nil {
		return false, s.finishRunErr
	}
	record, ok := s.runs[id]
	if !ok || RunStatusTerminal(record.Status) {
		return false, nil
	}
	record.Status = f.Status
	record.Error = f.Error
	record.ExitCode = f.ExitCode
	record.OutputUnverified = f.OutputUnverified
	record.FinishedAt = &f.FinishedAt
	s.runs[id] = record
	s.finishes[id] = f
	return true, nil
}
func (s *dispatcherTestStore) Run(id string) (RunRecord, RunOutput, error) {
	s.mu.Lock()
	record, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		return RunRecord{}, RunOutput{}, ErrRunNotFound
	}
	if RunStatusTerminal(record.Status) && s.terminalRunReadStarted != nil {
		s.terminalRunReadOnce.Do(func() { close(s.terminalRunReadStarted) })
		if s.terminalRunReadRelease != nil {
			<-s.terminalRunReadRelease
		}
	}
	return record, RunOutput{}, nil
}
func (s *dispatcherTestStore) Runs(Access) ([]RunView, error) { return nil, nil }
func (s *dispatcherTestStore) NonterminalRuns() ([]RecoveryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []RecoveryRun
	for _, run := range s.runs {
		if !RunStatusTerminal(run.Status) {
			result = append(result, RecoveryRun{RunRecord: run, BootSessionID: s.bootSessionIDs[run.ID]})
		}
	}
	return result, nil
}
func (s *dispatcherTestStore) ExpiredTerminalRunBoundary(time.Time) (*RetentionCursor, error) {
	return nil, nil
}
func (s *dispatcherTestStore) ExpiredTerminalRuns(time.Time, *RetentionCursor, *RetentionCursor, int) ([]RunRecord, error) {
	return nil, nil
}
func (s *dispatcherTestStore) MarkRunExchangeRemoved(string, time.Time) error { return nil }
func (s *dispatcherTestStore) PruneRunOutputs(time.Time) (int64, error)       { return 0, nil }
func (s *dispatcherTestStore) ImportMap(string, string) (ImportMapEntry, bool, error) {
	return ImportMapEntry{}, false, nil
}
func (s *dispatcherTestStore) ClaimImport(ImportClaimRequest) (ImportClaimResult, error) {
	return ImportClaimResult{}, nil
}
func (s *dispatcherTestStore) MarkImportRunning(string, time.Time) (bool, error) { return true, nil }
func (s *dispatcherTestStore) CancelPendingImport(string, string, time.Time) (bool, error) {
	// Dispatcher-only tests treat jobs accepted by the fake live registry as
	// already running. Task 9's lifecycle store exercises the pending winner.
	return false, nil
}
func (s *dispatcherTestStore) FinishImport(id string, finish ImportFinish) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishImportCalls++
	if s.finishImportErr != nil {
		return false, s.finishImportErr
	}
	s.importFinishes[id] = finish
	return true, nil
}
func (s *dispatcherTestStore) SetImportSourceDeletePending(id string, pending bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	finish := s.importFinishes[id]
	finish.SourceDeletePending = pending
	s.importFinishes[id] = finish
	return nil
}
func (s *dispatcherTestStore) InterruptNonterminalImports(time.Time) error {
	return s.interruptImportsErr
}
func (s *dispatcherTestStore) NonterminalImports() ([]ImportRecord, error) { return nil, nil }
func (s *dispatcherTestStore) HasNonterminalImports(string) (bool, error)  { return false, nil }

type registeredCommand struct {
	spec   RunJobSpec
	cancel func(string) error
	run    func(context.Context, Progress) Outcome
}
type registeredImport struct {
	spec ImportJobSpec
	run  func(context.Context, Progress) Outcome
}

type dispatcherTestJobs struct {
	mu         sync.Mutex
	commands   []registeredCommand
	imports    []registeredImport
	commandErr error
	importErr  error
}

func (j *dispatcherTestJobs) SubmitCommandJob(s RunJobSpec, c func(string) error, r func(context.Context, Progress) Outcome) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.commandErr != nil {
		return "", j.commandErr
	}
	j.commands = append(j.commands, registeredCommand{s, c, r})
	return "job-" + s.RunID, nil
}
func (j *dispatcherTestJobs) SubmitImportJob(s ImportJobSpec, r func(context.Context, Progress) Outcome) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.importErr != nil {
		return "", j.importErr
	}
	j.imports = append(j.imports, registeredImport{spec: s, run: r})
	return "job-" + s.ImportID, nil
}
func (j *dispatcherTestJobs) commandCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.commands)
}
func (j *dispatcherTestJobs) commandSnapshot() []registeredCommand {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]registeredCommand(nil), j.commands...)
}
func (j *dispatcherTestJobs) importCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.imports)
}
func (j *dispatcherTestJobs) importSnapshot() []registeredImport {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]registeredImport(nil), j.imports...)
}

type nopProgress struct{}

func (nopProgress) SetPhase(string)               {}
func (nopProgress) SetPhaseProgress(int64, int64) {}
func (nopProgress) SetAuthoritativeStatus(string) {}

type dispatcherTestExecutor struct{ store Store }

type dispatcherExecutorFunc func(context.Context, QueuedRun) Outcome

func (f dispatcherExecutorFunc) Execute(ctx context.Context, run QueuedRun) Outcome {
	return f(ctx, run)
}

func (e dispatcherTestExecutor) Execute(ctx context.Context, q QueuedRun) Outcome {
	if e.store != nil {
		_, _ = e.store.MarkRunRunning(q.RunID, time.Now().UTC())
	}
	<-ctx.Done()
	outcome := Outcome{Status: RunStatusCancelled, Error: context.Cause(ctx).Error()}
	if e.store != nil {
		_, err := e.store.FinishRun(q.RunID, RunFinish{
			Status: outcome.Status, Error: outcome.Error, FinishedAt: time.Now().UTC(),
		})
		if err != nil {
			outcome.Error += "; persist terminal command: " + err.Error()
		}
	}
	return outcome
}

func commandRequest(plugin string, callback func(Result)) CommandRequest {
	return CommandRequest{PluginName: plugin, PluginGeneration: 1, Declaration: Declaration{Name: "run", Argv: []string{"tool", "{{value}}"}, Timeout: time.Minute}, Params: map[string]string{"value": "x"}, Completion: callback}
}

func startTestDispatcher(t *testing.T, pending int) (*Dispatcher, *dispatcherTestStore, *dispatcherTestJobs) {
	t.Helper()
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{store: store}, Settings: dispatcherTestSettings{pending: pending}})
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = d.Stop(ctx)
	})
	return d, store, jobs
}

func TestSubmitRefusesInputsBeforeAnyDurableWork(t *testing.T) {
	declaration := Declaration{Name: "fetch", Argv: []string{"tool", "--cookies", "cookies.txt"}, Timeout: time.Minute, Inputs: []string{"cookies.txt"}}
	// Bare dispatcher: both refusals return from Submit before any queue or
	// store is touched, which is the whole point of validating synchronously.
	d := NewDispatcher(Dependencies{Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10, quota: 8}})
	if _, err := d.Submit(CommandRequest{PluginName: "p", Declaration: declaration, Inputs: map[string]string{"other.txt": "x"}}); err == nil || !strings.Contains(err.Error(), "other.txt") {
		t.Fatalf("undeclared input err = %v", err)
	}
	if _, err := d.Submit(CommandRequest{PluginName: "p", Declaration: declaration, Inputs: map[string]string{"cookies.txt": "123456789"}}); err == nil || !strings.Contains(err.Error(), "total") {
		t.Fatalf("per-run quota err = %v", err)
	}
}

func TestSubmitAcceptsDeclaredInputs(t *testing.T) {
	d, _, _ := startTestDispatcher(t, 10)
	declaration := Declaration{Name: "fetch", Argv: []string{"tool", "--cookies", "cookies.txt"}, Timeout: time.Minute, Inputs: []string{"cookies.txt"}}
	runID, err := d.Submit(CommandRequest{PluginName: "inputs", PluginGeneration: 1, Declaration: declaration, Inputs: map[string]string{"cookies.txt": "SID=secret"}})
	if err != nil || runID == "" {
		t.Fatalf("Submit with declared inputs = %q, %v", runID, err)
	}
}

func completionDispatchActive(d *Dispatcher, plugin string) int {
	d.completionDispatch.mu.Lock()
	defer d.completionDispatch.mu.Unlock()
	if state := d.completionDispatch.byPlugin[plugin]; state != nil {
		return state.active
	}
	return 0
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func completeRegisteredCommand(t *testing.T, command registeredCommand) Outcome {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome := command.run(ctx, nopProgress{})
	if outcome.Status != RunStatusCancelled {
		t.Fatalf("completed command outcome = %+v", outcome)
	}
	return outcome
}

func TestDispatcherUsesDedicatedFairCommandSlots(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 100)
	ids := make([]string, 0, 6)
	for _, plugin := range []string{"a", "b", "a", "c", "a", "b"} {
		id, err := d.Submit(commandRequest(plugin, nil))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 4 })
	started := jobs.commandSnapshot()
	perPlugin := map[string]int{}
	startedByPlugin := map[string][]string{}
	for _, job := range started {
		perPlugin[job.spec.PluginName]++
		startedByPlugin[job.spec.PluginName] = append(startedByPlugin[job.spec.PluginName], job.spec.RunID)
	}
	if perPlugin["a"] > 2 {
		t.Fatalf("plugin a started %d commands", perPlugin["a"])
	}
	if perPlugin["b"] == 0 || perPlugin["c"] == 0 {
		t.Fatalf("eligible plugins were blocked: %#v", perPlugin)
	}
	if jobs.commandCount() != 4 {
		t.Fatalf("global active=%d", jobs.commandCount())
	}
	if got := startedByPlugin["a"]; len(got) != 2 || got[0] != ids[0] || got[1] != ids[2] {
		t.Fatalf("plugin A did not start FIFO: got %#v, submitted %#v", got, []string{ids[0], ids[2], ids[4]})
	}

	completeRegisteredCommand(t, started[0])
	waitFor(t, func() bool { return jobs.commandCount() == 5 })
	fifth := jobs.commandSnapshot()[4]
	if fifth.spec.PluginName != "a" || fifth.spec.RunID != ids[4] {
		t.Fatalf("released slot did not preserve round-robin/FIFO order: got %+v, want plugin a run %s", fifth.spec, ids[4])
	}
	completeRegisteredCommand(t, started[1])
	waitFor(t, func() bool { return jobs.commandCount() == 6 })
	sixth := jobs.commandSnapshot()[5]
	if sixth.spec.PluginName != "b" || sixth.spec.RunID != ids[5] {
		t.Fatalf("second released slot did not preserve per-plugin FIFO: got %+v, want plugin b run %s", sixth.spec, ids[5])
	}
}

func TestDispatcherPendingCapDoesNotRegisterOrLeakRun(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 3)
	// Two A commands own its execution slots; three more are pending.
	for i := 0; i < 5; i++ {
		if _, err := d.Submit(commandRequest("a", nil)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })
	store.mu.Lock()
	before := len(store.runs)
	store.mu.Unlock()
	if _, err := d.Submit(commandRequest("a", nil)); err == nil {
		t.Fatal("command beyond pending cap admitted")
	}
	store.mu.Lock()
	after := len(store.runs)
	store.mu.Unlock()
	if after != before {
		t.Fatalf("refused command created durable row: %d -> %d", before, after)
	}
	if jobs.commandCount() != 2 {
		t.Fatalf("pending work entered live jobs: %d", jobs.commandCount())
	}
	if _, err := d.Submit(commandRequest("b", nil)); err != nil {
		t.Fatalf("other plugin refused: %v", err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 3 })
}

func TestDisablePluginWaitsForCommandCompletionDispatch(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 100)
	entered := make(chan struct{})
	release := make(chan struct{})
	runID, err := d.Submit(commandRequest("a", func(Result) {
		close(entered)
		<-release
	}))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	command := jobs.commandSnapshot()[0]
	finished := make(chan Outcome, 1)
	go func() { finished <- command.run(context.Background(), nopProgress{}) }()
	waitFor(t, func() bool {
		record, _, err := d.deps.Store.Run(runID)
		return err == nil && record.Status == RunStatusRunning
	})

	disabled := make(chan error, 1)
	go func() { disabled <- d.DisablePlugin("a", "plugin disabled") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("command completion was not dispatched")
	}
	select {
	case err := <-disabled:
		t.Fatalf("disable returned before its command completion dispatch settled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-disabled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("disable did not return after command completion dispatch settled")
	}
	select {
	case outcome := <-finished:
		if outcome.Status != RunStatusCancelled {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled command worker did not finish")
	}
}

func TestDisablePluginWaitsAcrossTerminalPersistenceBeforeCompletionDelivery(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 100)
	terminalReadStarted := make(chan struct{})
	allowTerminalRead := make(chan struct{})
	store.terminalRunReadStarted = terminalReadStarted
	store.terminalRunReadRelease = allowTerminalRead
	callbackEntered := make(chan struct{})
	allowCallback := make(chan struct{})

	runID, err := d.Submit(commandRequest("a", func(Result) {
		close(callbackEntered)
		<-allowCallback
	}))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	command := jobs.commandSnapshot()[0]
	liveCtx, cancelLive := context.WithCancel(context.Background())
	finished := make(chan Outcome, 1)
	go func() { finished <- command.run(liveCtx, nopProgress{}) }()
	waitFor(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.runs[runID].Status == RunStatusRunning
	})

	cancelLive()
	select {
	case <-terminalReadStarted:
	case <-time.After(time.Second):
		t.Fatal("command did not persist terminal state before completion delivery")
	}
	store.mu.Lock()
	status := store.runs[runID].Status
	store.mu.Unlock()
	if !RunStatusTerminal(status) {
		t.Fatalf("run status at delivery barrier = %q, want terminal", status)
	}

	disabled := make(chan error, 1)
	go func() { disabled <- d.DisablePlugin("a", "plugin disabled") }()
	select {
	case err := <-disabled:
		t.Fatalf("disable returned in the terminal-persisted delivery gap: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(allowTerminalRead)
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("completion callback was not entered")
	}
	select {
	case err := <-disabled:
		t.Fatalf("disable returned before completion callback settled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(allowCallback)
	select {
	case err := <-disabled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("disable did not return after completion delivery settled")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("command worker did not return")
	}
}

func TestCommandCompletionLifecycleSettlesOnEarlyWorkerRefusalAndExecutorPanic(t *testing.T) {
	t.Run("worker refusal", func(t *testing.T) {
		d, _, jobs := startTestDispatcher(t, 100)
		if _, err := d.Submit(commandRequest("a", func(Result) {
			t.Error("worker refusal delivered a completion callback")
		})); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return jobs.commandCount() == 1 })
		d.workerMu.Lock()
		d.workerClosing = true
		d.workerMu.Unlock()
		outcome := jobs.commandSnapshot()[0].run(context.Background(), nopProgress{})
		d.workerMu.Lock()
		d.workerClosing = false
		d.workerMu.Unlock()
		if outcome.Status != RunStatusInterrupted {
			t.Fatalf("worker refusal outcome = %+v", outcome)
		}
		if active := completionDispatchActive(d, "a"); active != 0 {
			t.Fatalf("worker refusal leaked %d completion lifecycles", active)
		}
	})

	t.Run("executor panic", func(t *testing.T) {
		store := newDispatcherTestStore()
		jobs := &dispatcherTestJobs{}
		d := NewDispatcher(Dependencies{
			Store: store, Jobs: jobs, Settings: dispatcherTestSettings{pending: 100},
			Executor: dispatcherExecutorFunc(func(context.Context, QueuedRun) Outcome {
				panic("executor panic")
			}),
		})
		if err := d.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = d.Stop(ctx)
		})
		if _, err := d.Submit(commandRequest("a", func(Result) {
			t.Error("panicking executor delivered a completion callback")
		})); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return jobs.commandCount() == 1 })
		func() {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Error("executor panic was not observed")
				}
			}()
			jobs.commandSnapshot()[0].run(context.Background(), nopProgress{})
		}()
		if active := completionDispatchActive(d, "a"); active != 0 {
			t.Fatalf("executor panic leaked %d completion lifecycles", active)
		}
	})
}

func TestDispatcherManagedLaneRefusalReleasesCommandSlot(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 100)
	jobs.mu.Lock()
	jobs.commandErr = errors.New("managed lane full")
	jobs.mu.Unlock()
	failed := make(chan Result, 1)
	failedID, err := d.Submit(commandRequest("a", func(result Result) { failed <- result }))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-failed:
		if result.RunID != failedID || result.Error == "" {
			t.Fatalf("dispatch failure result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch failure was not reported")
	}
	waitFor(t, func() bool { return completionDispatchActive(d, "a") == 0 })
	store.mu.Lock()
	finish := store.finishes[failedID]
	store.mu.Unlock()
	if finish.Status != RunStatusFailed {
		t.Fatalf("dispatch failure durable status = %+v", finish)
	}

	jobs.mu.Lock()
	jobs.commandErr = nil
	jobs.mu.Unlock()
	if _, err := d.Submit(commandRequest("a", nil)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
}

func TestDispatcherActiveCancelUsesDurableLatchAndExecutionCancel(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 100)
	completed := make(chan Result, 1)
	runID, err := d.Submit(commandRequest("a", func(result Result) { completed <- result }))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	registered := jobs.commandSnapshot()[0]
	outcome := make(chan Outcome, 1)
	go func() { outcome <- registered.run(context.Background(), nopProgress{}) }()

	if err := registered.cancel("operator cancelled"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		if result.RunID != runID || result.OK {
			t.Fatalf("completion = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("active cancellation did not stop executor")
	}
	select {
	case got := <-outcome:
		if got.Status != RunStatusCancelled {
			t.Fatalf("outcome = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("live run did not return after cancellation")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.cancelled) != 1 || store.cancelled[0] != runID {
		t.Fatalf("durable cancellation latch calls = %#v", store.cancelled)
	}
}

func TestDispatcherCancelsQueuedRunWithoutLiveRegistration(t *testing.T) {
	completed := make(chan Result, 1)
	d, store, jobs := startTestDispatcher(t, 1)
	for i := 0; i < 2; i++ {
		if _, err := d.Submit(commandRequest("a", nil)); err != nil {
			t.Fatal(err)
		}
	}
	queued, err := d.Submit(commandRequest("a", func(r Result) { completed <- r }))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })
	if err := d.Cancel(queued, "operator cancelled"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		if result.OK || result.RunID != queued {
			t.Fatalf("result=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("queued callback not delivered")
	}
	store.mu.Lock()
	finish, ok := store.finishes[queued]
	store.mu.Unlock()
	if !ok || finish.Status != RunStatusCancelled {
		t.Fatalf("finish=%+v, ok=%v", finish, ok)
	}
	if jobs.commandCount() != 2 {
		t.Fatalf("queued cancellation registered live job")
	}
	if _, err := d.Submit(commandRequest("a", nil)); err != nil {
		t.Fatalf("replacement refused: %v", err)
	}
}

func TestDispatcherQueuedCancellationRetriesPersistenceBeforeCallback(t *testing.T) {
	completed := make(chan Result, 1)
	d, store, jobs := startTestDispatcher(t, 10)
	for i := 0; i < 2; i++ {
		if _, err := d.Submit(commandRequest("a", nil)); err != nil {
			t.Fatal(err)
		}
	}
	queued, err := d.Submit(commandRequest("a", func(result Result) { completed <- result }))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })

	injected := errors.New("finish unavailable")
	store.mu.Lock()
	store.finishRunErr = injected
	store.mu.Unlock()
	if err := d.Cancel(queued, "operator cancelled"); !errors.Is(err, injected) {
		t.Fatalf("cancel error = %v, want %v", err, injected)
	}
	select {
	case result := <-completed:
		t.Fatalf("callback fired before durable cancellation: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}
	if jobs.commandCount() != 2 {
		t.Fatalf("cancellation awaiting persistence registered a live job: %d", jobs.commandCount())
	}

	store.mu.Lock()
	store.finishRunErr = nil
	store.mu.Unlock()
	select {
	case result := <-completed:
		if result.RunID != queued || result.OK || result.Error != "operator cancelled" {
			t.Fatalf("completion = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("queued cancellation was not retried")
	}
	store.mu.Lock()
	finish := store.finishes[queued]
	store.mu.Unlock()
	if finish.Status != RunStatusCancelled {
		t.Fatalf("durable finish = %+v", finish)
	}
}

func TestDispatcherCompletionMayReenterDispatcher(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 10)
	for i := 0; i < 2; i++ {
		if _, err := d.Submit(commandRequest("a", nil)); err != nil {
			t.Fatal(err)
		}
	}
	reentered := make(chan error, 1)
	queued, err := d.Submit(commandRequest("a", func(Result) {
		_, submitErr := d.Submit(commandRequest("b", nil))
		reentered <- submitErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return jobs.commandCount() == 2 })

	cancelled := make(chan error, 1)
	go func() { cancelled <- d.Cancel(queued, "operator cancelled") }()
	select {
	case err := <-cancelled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued cancellation deadlocked on completion callback")
	}
	select {
	case err := <-reentered:
		if err != nil {
			t.Fatalf("reentrant submit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("completion callback could not reenter dispatcher")
	}
	waitFor(t, func() bool { return jobs.commandCount() == 3 })
}

func TestDispatcherRetriesCommandDispatchFailurePersistence(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*dispatcherTestStore)
		clear     func(*dispatcherTestStore)
	}{
		{
			name: "mark running", configure: func(store *dispatcherTestStore) { store.markRunErr = errors.New("mark unavailable") },
			clear: func(store *dispatcherTestStore) { store.markRunErr = nil },
		},
		{
			name: "finish", configure: func(store *dispatcherTestStore) { store.finishRunErr = errors.New("finish unavailable") },
			clear: func(store *dispatcherTestStore) { store.finishRunErr = nil },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			d, store, jobs := startTestDispatcher(t, 10)
			jobs.mu.Lock()
			jobs.commandErr = errors.New("managed lane full")
			jobs.mu.Unlock()
			store.mu.Lock()
			test.configure(store)
			store.mu.Unlock()
			completed := make(chan Result, 1)
			runID, err := d.Submit(commandRequest("a", func(result Result) { completed <- result }))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case result := <-completed:
				t.Fatalf("callback fired before durable dispatch failure: %+v", result)
			case <-time.After(50 * time.Millisecond):
			}
			store.mu.Lock()
			test.clear(store)
			store.mu.Unlock()
			select {
			case result := <-completed:
				if result.RunID != runID || result.Error != "managed lane full" {
					t.Fatalf("completion = %+v", result)
				}
			case <-time.After(time.Second):
				t.Fatal("dispatch failure persistence was not retried")
			}
			store.mu.Lock()
			finish := store.finishes[runID]
			store.mu.Unlock()
			if finish.Status != RunStatusFailed {
				t.Fatalf("durable finish = %+v", finish)
			}
		})
	}
}

func TestDispatcherRetriesImportDispatchFailurePersistence(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 10)
	jobs.mu.Lock()
	jobs.importErr = errors.New("managed lane full")
	jobs.mu.Unlock()
	store.mu.Lock()
	store.finishImportErr = errors.New("finish unavailable")
	store.mu.Unlock()
	if err := d.submitImport(ImportJobSpec{ImportID: "import-retry", RunID: "run-import-retry", PluginName: "p"}, func(context.Context, Progress) Outcome {
		return Outcome{Status: ImportStatusSucceeded}
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	store.mu.Lock()
	_, finishedEarly := store.importFinishes["import-retry"]
	store.finishImportErr = nil
	store.mu.Unlock()
	if finishedEarly {
		t.Fatal("import dispatch failure was recorded despite injected store error")
	}
	waitFor(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.importFinishes["import-retry"].Status == ImportStatusFailed
	})
	if jobs.importCount() != 0 {
		t.Fatalf("failed import dispatch registered a live job: %d", jobs.importCount())
	}
}

func TestAwaitDispatcherReplyPrefersAcceptedReplyAfterShutdown(t *testing.T) {
	for i := 0; i < 1000; i++ {
		reply := make(chan error, 1)
		done := make(chan struct{})
		reply <- nil
		close(done)
		if err := awaitDispatcherReply(reply, done); err != nil {
			t.Fatalf("iteration %d returned %v for accepted work", i, err)
		}
	}
}

func TestDispatcherSubmitStopNeverRejectsAcceptedWork(t *testing.T) {
	for i := 0; i < 200; i++ {
		store := newDispatcherTestStore()
		jobs := &dispatcherTestJobs{}
		d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
		if err := d.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		submitResult := make(chan error, 1)
		stopResult := make(chan error, 1)
		go func() {
			<-start
			_, err := d.Submit(commandRequest("p", nil))
			submitResult <- err
		}()
		go func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stopResult <- d.Stop(ctx)
		}()
		close(start)
		submitErr := <-submitResult
		if err := <-stopResult; err != nil {
			t.Fatalf("iteration %d stop: %v", i, err)
		}
		store.mu.Lock()
		accepted := len(store.runs) != 0
		store.mu.Unlock()
		if accepted && submitErr != nil {
			t.Fatalf("iteration %d accepted a durable run but Submit returned %v", i, submitErr)
		}
	}
}

func TestDispatcherImportSubmitStopNeverRejectsAcceptedWork(t *testing.T) {
	for i := 0; i < 200; i++ {
		store := newDispatcherTestStore()
		jobs := &dispatcherTestJobs{}
		d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: 10}})
		if err := d.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		submitResult := make(chan error, 1)
		stopResult := make(chan error, 1)
		go func() {
			<-start
			submitResult <- d.submitImport(ImportJobSpec{ImportID: "import", RunID: "run-import", PluginName: "p"}, func(context.Context, Progress) Outcome {
				return Outcome{Status: ImportStatusSucceeded}
			})
		}()
		go func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stopResult <- d.Stop(ctx)
		}()
		close(start)
		submitErr := <-submitResult
		if err := <-stopResult; err != nil {
			t.Fatalf("iteration %d stop: %v", i, err)
		}
		accepted := jobs.importCount() != 0
		if accepted && submitErr != nil {
			t.Fatalf("iteration %d accepted an import job but submitImport returned %v", i, submitErr)
		}
	}
}

func TestDispatcherImportQueueHasIndependentCapAndTwoSlots(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 2)
	for i := 0; i < 4; i++ {
		if err := d.submitImport(ImportJobSpec{ImportID: string(rune('a' + i)), RunID: "run-" + string(rune('a'+i)), PluginName: "p"}, func(context.Context, Progress) Outcome { return Outcome{Status: ImportStatusSucceeded} }); err != nil {
			t.Fatalf("submit import %d: %v", i, err)
		}
	}
	waitFor(t, func() bool { return jobs.importCount() == 2 })
	if err := d.submitImport(ImportJobSpec{ImportID: "overflow", RunID: "run-overflow", PluginName: "p"}, func(context.Context, Progress) Outcome { return Outcome{} }); err == nil {
		t.Fatal("import beyond pending cap admitted")
	}
	if jobs.importCount() != 2 {
		t.Fatalf("pending imports entered live jobs: %d", jobs.importCount())
	}
	started := jobs.importSnapshot()
	if got := started[0].run(context.Background(), nopProgress{}); got.Status != ImportStatusSucceeded {
		t.Fatalf("first import outcome = %+v", got)
	}
	waitFor(t, func() bool { return jobs.importCount() == 3 })
	third := jobs.importSnapshot()[2]
	if third.spec.ImportID != "c" {
		t.Fatalf("released import slot started %q, want c", third.spec.ImportID)
	}
	if got := started[1].run(context.Background(), nopProgress{}); got.Status != ImportStatusSucceeded {
		t.Fatalf("second import outcome = %+v", got)
	}
	waitFor(t, func() bool { return jobs.importCount() == 4 })
	fourth := jobs.importSnapshot()[3]
	if fourth.spec.ImportID != "d" {
		t.Fatalf("second released import slot started %q, want d", fourth.spec.ImportID)
	}
}
