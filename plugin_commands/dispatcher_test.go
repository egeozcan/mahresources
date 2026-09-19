package plugin_commands

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type dispatcherTestSettings struct{ pending int }

func (s dispatcherTestSettings) StagingRoot() string              { return "/staging" }
func (s dispatcherTestSettings) PendingPerPluginLimit() int       { return s.pending }
func (s dispatcherTestSettings) PerRunQuota() int64               { return 1 << 30 }
func (s dispatcherTestSettings) GlobalStagingQuota() int64        { return 1 << 31 }
func (s dispatcherTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s dispatcherTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s dispatcherTestSettings) CommandPath() string              { return "/bin" }

type dispatcherTestStore struct {
	mu                  sync.Mutex
	runs                map[string]RunRecord
	finishes            map[string]RunFinish
	importFinishes      map[string]ImportFinish
	cancelled           []string
	markRunErr          error
	finishRunErr        error
	finishRunStarted    chan struct{}
	finishRunRelease    <-chan struct{}
	finishImportErr     error
	finishImportCalls   int
	interruptImportsErr error
}

func newDispatcherTestStore() *dispatcherTestStore {
	return &dispatcherTestStore{
		runs: map[string]RunRecord{}, finishes: map[string]RunFinish{}, importFinishes: map[string]ImportFinish{},
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
func (s *dispatcherTestStore) SetRunProcessGroup(id string, pgid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[id]
	if !ok || record.Status != RunStatusRunning || record.ProcessGroupID != nil {
		return errors.New("run is not awaiting a process group")
	}
	record.ProcessGroupID = &pgid
	s.runs[id] = record
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
	defer s.mu.Unlock()
	record, ok := s.runs[id]
	if !ok {
		return RunRecord{}, RunOutput{}, ErrRunNotFound
	}
	return record, RunOutput{}, nil
}
func (s *dispatcherTestStore) Runs(Access) ([]RunView, error)                     { return nil, nil }
func (s *dispatcherTestStore) NonterminalRuns() ([]RunRecord, error)              { return nil, nil }
func (s *dispatcherTestStore) ExpiredTerminalRuns(time.Time) ([]RunRecord, error) { return nil, nil }
func (s *dispatcherTestStore) PruneRunOutputs(time.Time) (int64, error)           { return 0, nil }
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

type dispatcherTestExecutor struct{ store Store }

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
