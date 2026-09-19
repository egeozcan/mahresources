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
	mu        sync.Mutex
	runs      map[string]RunRecord
	finishes  map[string]RunFinish
	cancelled []string
}

func newDispatcherTestStore() *dispatcherTestStore {
	return &dispatcherTestStore{runs: map[string]RunRecord{}, finishes: map[string]RunFinish{}}
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
func (s *dispatcherTestStore) MarkRunRunning(string, time.Time) (bool, error) { return true, nil }
func (s *dispatcherTestStore) SetRunProcessGroup(string, int) error           { return nil }
func (s *dispatcherTestStore) RequestRunCancel(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[id]; !ok {
		return ErrRunNotFound
	}
	s.cancelled = append(s.cancelled, id)
	return nil
}
func (s *dispatcherTestStore) FinishRun(id string, f RunFinish) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishes[id] = f
	return true, nil
}
func (s *dispatcherTestStore) Run(string) (RunRecord, RunOutput, error) {
	return RunRecord{}, RunOutput{}, nil
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
func (s *dispatcherTestStore) FinishImport(string, ImportFinish) (bool, error)   { return true, nil }
func (s *dispatcherTestStore) InterruptNonterminalImports(time.Time) error       { return nil }
func (s *dispatcherTestStore) NonterminalImports() ([]ImportRecord, error)       { return nil, nil }
func (s *dispatcherTestStore) HasNonterminalImports(string) (bool, error)        { return false, nil }

type registeredCommand struct {
	spec   RunJobSpec
	cancel func(string) error
	run    func(context.Context, Progress) Outcome
}
type dispatcherTestJobs struct {
	mu         sync.Mutex
	commands   []registeredCommand
	imports    []ImportJobSpec
	commandErr error
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
	j.imports = append(j.imports, s)
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

type nopProgress struct{}

func (nopProgress) SetPhase(string)               {}
func (nopProgress) SetPhaseProgress(int64, int64) {}

type dispatcherTestExecutor struct{}

func (dispatcherTestExecutor) Execute(ctx context.Context, q QueuedRun) Outcome {
	<-ctx.Done()
	return Outcome{Status: RunStatusCancelled, Error: ctx.Err().Error()}
}

func commandRequest(plugin string, callback func(Result)) CommandRequest {
	return CommandRequest{PluginName: plugin, PluginGeneration: 1, Declaration: Declaration{Name: "run", Argv: []string{"tool", "{{value}}"}, Timeout: time.Minute}, Params: map[string]string{"value": "x"}, Completion: callback}
}

func startTestDispatcher(t *testing.T, pending int) (*Dispatcher, *dispatcherTestStore, *dispatcherTestJobs) {
	t.Helper()
	store := newDispatcherTestStore()
	jobs := &dispatcherTestJobs{}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{}, Settings: dispatcherTestSettings{pending: pending}})
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

func TestDispatcherImportQueueHasIndependentCapAndTwoSlots(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 2)
	for i := 0; i < 4; i++ {
		if err := d.submitImport(ImportJobSpec{ImportID: string(rune('a' + i)), PluginName: "p"}, func(context.Context, Progress) Outcome { return Outcome{Status: ImportStatusSucceeded} }); err != nil {
			t.Fatalf("submit import %d: %v", i, err)
		}
	}
	waitFor(t, func() bool { return jobs.importCount() == 2 })
	if err := d.submitImport(ImportJobSpec{ImportID: "overflow", PluginName: "p"}, func(context.Context, Progress) Outcome { return Outcome{} }); err == nil {
		t.Fatal("import beyond pending cap admitted")
	}
	if jobs.importCount() != 2 {
		t.Fatalf("pending imports entered live jobs: %d", jobs.importCount())
	}
}
