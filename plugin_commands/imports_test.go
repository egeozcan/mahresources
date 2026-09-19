package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type importTestSettings struct{ root string }

func (s importTestSettings) StagingRoot() string              { return s.root }
func (s importTestSettings) PendingPerPluginLimit() int       { return 100 }
func (s importTestSettings) PerRunQuota() int64               { return 8 << 30 }
func (s importTestSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s importTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s importTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s importTestSettings) CommandPath() string              { return "/bin" }

type importLifecycleStore struct {
	*exchangeTestStore
	claimMu sync.Mutex
	maps    map[string]ImportMapEntry
	claims  map[string]ImportRecord
}

func newImportLifecycleStore() *importLifecycleStore {
	return &importLifecycleStore{exchangeTestStore: newExchangeTestStore(), maps: map[string]ImportMapEntry{}, claims: map[string]ImportRecord{}}
}
func importKey(runID, name string) string { return runID + "\x00" + name }
func (s *importLifecycleStore) ImportMap(runID, name string) (ImportMapEntry, bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	m, ok := s.maps[importKey(runID, name)]
	return m, ok, nil
}
func (s *importLifecycleStore) ClaimImport(req ImportClaimRequest) (ImportClaimResult, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	key := importKey(req.RunID, req.FileName)
	mapped, ok := s.maps[key]
	if !ok {
		record := ImportRecord{ID: req.ImportID, RunID: req.RunID, FileName: req.FileName, PluginGeneration: req.PluginGeneration, CreatedByUserID: copyUint(req.CreatedByUserID), Status: ImportStatusPending, CreatedAt: req.CreatedAt}
		s.claims[req.ImportID] = record
		mapped = ImportMapEntry{RunID: req.RunID, FileName: req.FileName, ImportID: req.ImportID, Status: ImportStatusPending}
		s.maps[key] = mapped
		return ImportClaimResult{ImportID: req.ImportID, Status: ImportStatusPending, Created: true, Enqueue: true}, nil
	}
	switch mapped.Status {
	case ImportStatusPending, ImportStatusRunning, ImportStatusSucceeded:
		return ImportClaimResult{ImportID: mapped.ImportID, ResourceID: copyUint(mapped.ResourceID), Status: mapped.Status}, nil
	case ImportStatusInterrupted:
		record := s.claims[mapped.ImportID]
		record.Status, record.PluginGeneration, record.CreatedByUserID = ImportStatusPending, req.PluginGeneration, copyUint(req.CreatedByUserID)
		s.claims[mapped.ImportID] = record
		mapped.Status, mapped.Error, mapped.ResourceID = ImportStatusPending, "", nil
		s.maps[key] = mapped
		return ImportClaimResult{ImportID: mapped.ImportID, Status: ImportStatusPending, Enqueue: true}, nil
	case ImportStatusFailed, ImportStatusCancelled:
		record := ImportRecord{ID: req.ImportID, RunID: req.RunID, FileName: req.FileName, PluginGeneration: req.PluginGeneration, CreatedByUserID: copyUint(req.CreatedByUserID), Status: ImportStatusPending, CreatedAt: req.CreatedAt}
		s.claims[req.ImportID] = record
		mapped.ImportID, mapped.Status, mapped.Error, mapped.ResourceID = req.ImportID, ImportStatusPending, "", nil
		s.maps[key] = mapped
		return ImportClaimResult{ImportID: req.ImportID, Status: ImportStatusPending, Created: true, Enqueue: true}, nil
	default:
		return ImportClaimResult{}, errors.New("bad map state")
	}
}
func (s *importLifecycleStore) MarkImportRunning(id string, started time.Time) (bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	record, ok := s.claims[id]
	if !ok || record.Status != ImportStatusPending || record.CreatedByUserID == nil {
		return false, nil
	}
	record.Status, record.StartedAt = ImportStatusRunning, &started
	s.claims[id] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == id {
			mapped.Status = ImportStatusRunning
			s.maps[key] = mapped
		}
	}
	return true, nil
}
func (s *importLifecycleStore) FinishImport(id string, finish ImportFinish) (bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	record, ok := s.claims[id]
	if !ok || ImportStatusTerminal(record.Status) {
		return false, nil
	}
	record.Status, record.Error, record.FinishedAt = finish.Status, finish.Error, &finish.FinishedAt
	s.claims[id] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == id {
			mapped.Status, mapped.Error, mapped.ResourceID = finish.Status, finish.Error, copyUint(finish.ResourceID)
			s.maps[key] = mapped
		}
	}
	return true, nil
}
func (s *importLifecycleStore) NonterminalImports() ([]ImportRecord, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	var out []ImportRecord
	for _, record := range s.claims {
		if !ImportStatusTerminal(record.Status) {
			out = append(out, record)
		}
	}
	return out, nil
}
func (s *importLifecycleStore) InterruptNonterminalImports(finished time.Time) error {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	for id, record := range s.claims {
		if !ImportStatusTerminal(record.Status) {
			record.Status, record.Error, record.FinishedAt = ImportStatusInterrupted, "server interrupted", &finished
			s.claims[id] = record
		}
	}
	for key, mapped := range s.maps {
		if mapped.Status == ImportStatusPending || mapped.Status == ImportStatusRunning {
			mapped.Status, mapped.Error = ImportStatusInterrupted, "server interrupted"
			s.maps[key] = mapped
		}
	}
	return nil
}
func (s *importLifecycleStore) HasNonterminalImports(runID string) (bool, error) {
	for _, record := range s.claims {
		if record.RunID == runID && !ImportStatusTerminal(record.Status) {
			return true, nil
		}
	}
	return false, nil
}
func (s *importLifecycleStore) RecordImportDeleteFailure(importID, message string) error {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	record := s.claims[importID]
	record.Error = message
	s.claims[importID] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == importID {
			mapped.Error = message
			s.maps[key] = mapped
		}
	}
	return nil
}

type importTestImporter struct {
	mu          sync.Mutex
	validations []ImportValidation
	bodies      [][]byte
	validateErr error
	resourceID  uint
	started     chan struct{}
	proceed     chan struct{}
	once        sync.Once
}

func (i *importTestImporter) ValidateImport(v ImportValidation) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.validations = append(i.validations, v)
	return i.validateErr
}
func (i *importTestImporter) ImportResource(ctx context.Context, source ImportSource, _ ResourceFields, _ string) (uint, error) {
	if i.started != nil {
		i.once.Do(func() { close(i.started) })
	}
	if i.proceed != nil {
		select {
		case <-i.proceed:
		case <-ctx.Done():
			return 0, context.Cause(ctx)
		}
	}
	body, err := os.ReadFile(source.Path)
	if err != nil {
		return 0, err
	}
	i.mu.Lock()
	i.bodies = append(i.bodies, body)
	i.mu.Unlock()
	return i.resourceID, nil
}

func importHarness(t *testing.T) (*Dispatcher, *importLifecycleStore, *dispatcherTestJobs, *importTestImporter, string, ImportSubmission) {
	t.Helper()
	root := t.TempDir()
	store := newImportLifecycleStore()
	actor := uint(7)
	addExchangeRun(t, store.exchangeTestStore, root, RunRecord{ID: "run-one", PluginName: "alpha", CreatedByUserID: &actor}, map[string]string{"result.bin": "complete-output"})
	jobs := &dispatcherTestJobs{}
	importer := &importTestImporter{resourceID: 91}
	d := NewDispatcher(Dependencies{Store: store, Jobs: jobs, Executor: dispatcherTestExecutor{store: store}, Settings: importTestSettings{root: root}, Leases: NewLeaseManager()})
	d.SetImporter(importer)
	ctx, cancel := context.WithCancel(context.Background())
	requireNoError(t, d.Start(ctx))
	t.Cleanup(func() {
		cancel()
		stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = d.Stop(stopCtx)
	})
	sub := ImportSubmission{Access: Access{PluginName: "alpha", ActorUserID: &actor}, RunID: "run-one", Name: "result.bin", Fields: ResourceFields{Name: "made"}, PluginGeneration: 3, ActorUserID: &actor}
	return d, store, jobs, importer, root, sub
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func waitForImportJobs(t *testing.T, jobs *dispatcherTestJobs, count int) []registeredImport {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := jobs.importSnapshot()
		if len(got) == count {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("import job count = %d, want %d", jobs.importCount(), count)
	return nil
}

func TestSubmitImportStateTableAndSucceededShortCircuit(t *testing.T) {
	for _, tc := range []struct {
		name, status                      string
		wantQueue, wantSame, wantResource bool
	}{
		{"absent", "", true, false, false},
		{"pending", ImportStatusPending, false, true, false},
		{"running", ImportStatusRunning, false, true, false},
		{"succeeded", ImportStatusSucceeded, false, true, true},
		{"interrupted", ImportStatusInterrupted, true, true, false},
		{"failed", ImportStatusFailed, true, false, false},
		{"cancelled", ImportStatusCancelled, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, store, jobs, _, root, sub := importHarness(t)
			oldID := "old-claim"
			if tc.status != "" {
				resource := uint(88)
				mapped := ImportMapEntry{RunID: sub.RunID, FileName: sub.Name, ImportID: oldID, Status: tc.status}
				if tc.wantResource {
					mapped.ResourceID = &resource
					requireNoError(t, os.RemoveAll(exchangeRunDir(root, "alpha", sub.RunID)))
				}
				store.maps[importKey(sub.RunID, sub.Name)] = mapped
				store.claims[oldID] = ImportRecord{ID: oldID, RunID: sub.RunID, FileName: sub.Name, Status: tc.status, CreatedByUserID: copyUint(sub.ActorUserID)}
			}
			callback := make(chan ImportResult, 1)
			sub.Completion = func(r ImportResult) { callback <- r }
			got, err := d.SubmitImport(sub)
			requireNoError(t, err)
			if tc.wantSame && got.ImportID != oldID {
				t.Fatalf("id = %q, want %q", got.ImportID, oldID)
			}
			if !tc.wantSame && tc.status != "" && got.ImportID == oldID {
				t.Fatal("terminal failure reused historical claim id")
			}
			if tc.wantResource && (got.ResourceID == nil || *got.ResourceID != 88) {
				t.Fatalf("resource = %v", got.ResourceID)
			}
			if tc.wantQueue {
				waitForImportJobs(t, jobs, 1)
			} else if jobs.importCount() != 0 {
				t.Fatalf("registered %d jobs", jobs.importCount())
			}
			if tc.wantResource {
				select {
				case <-callback:
					t.Fatal("synchronous success invoked async callback")
				case <-time.After(20 * time.Millisecond):
				}
			}
		})
	}
}

func TestSubmitImportSnapshotsRunsAndDeletesOnlyAfterDurableSuccess(t *testing.T) {
	d, store, jobs, importer, root, sub := importHarness(t)
	completed := make(chan ImportResult, 1)
	sub.Completion = func(r ImportResult) { completed <- r }
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	claimDir := filepath.Join(root, "import_tmp", got.ImportID)
	if entries, err := os.ReadDir(claimDir); err != nil || len(entries) != 1 {
		t.Fatalf("claim snapshot = %v, %v", entries, err)
	}
	outcome := registered[0].run(context.Background(), nopProgress{})
	if outcome.Status != ImportStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	result := <-completed
	if !result.OK || result.ResourceID == nil || *result.ResourceID != 91 {
		t.Fatalf("result = %+v", result)
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.Status != ImportStatusSucceeded || mapped.ResourceID == nil {
		t.Fatalf("map = %+v", mapped)
	}
	if _, err := os.Stat(filepath.Join(exchangeRunDir(root, "alpha", sub.RunID), sub.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source not removed after success: %v", err)
	}
	if _, err := os.Stat(claimDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("claim temp not removed: %v", err)
	}
	importer.mu.Lock()
	defer importer.mu.Unlock()
	if string(importer.bodies[0]) != "complete-output" {
		t.Fatalf("snapshot body = %q", importer.bodies[0])
	}
}

func TestSubmitImportDisableCancelsPendingButNotRunningAndShutdownWinsLateSuccess(t *testing.T) {
	t.Run("disable pending", func(t *testing.T) {
		d, store, jobs, _, _, sub := importHarness(t)
		// Occupy both import slots so this claim remains in the private queue.
		release := make(chan struct{})
		for i := 0; i < 2; i++ {
			requireNoError(t, d.submitImport(ImportJobSpec{ImportID: "park" + string(rune('a'+i)), RunID: sub.RunID, PluginName: "other"}, func(context.Context, Progress) Outcome { <-release; return Outcome{Status: ImportStatusSucceeded} }))
		}
		waitForImportJobs(t, jobs, 2)
		got, err := d.SubmitImport(sub)
		requireNoError(t, err)
		requireNoError(t, d.DisablePlugin("alpha", "plugin disabled"))
		mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
		if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusCancelled {
			t.Fatalf("map = %+v", mapped)
		}
		close(release)
	})

	t.Run("disable lets running import finish", func(t *testing.T) {
		d, store, jobs, importer, _, sub := importHarness(t)
		importer.started, importer.proceed = make(chan struct{}), make(chan struct{})
		got, err := d.SubmitImport(sub)
		requireNoError(t, err)
		registered := waitForImportJobs(t, jobs, 1)
		outcomes := make(chan Outcome, 1)
		go func() { outcomes <- registered[0].run(context.Background(), nopProgress{}) }()
		<-importer.started
		requireNoError(t, d.DisablePlugin("alpha", "plugin disabled"))
		close(importer.proceed)
		if outcome := <-outcomes; outcome.Status != ImportStatusSucceeded {
			t.Fatalf("outcome = %+v", outcome)
		}
		mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
		if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusSucceeded {
			t.Fatalf("map = %+v", mapped)
		}
	})

	t.Run("shutdown interrupts running read", func(t *testing.T) {
		d, store, jobs, importer, _, sub := importHarness(t)
		importer.started, importer.proceed = make(chan struct{}), make(chan struct{})
		got, err := d.SubmitImport(sub)
		requireNoError(t, err)
		registered := waitForImportJobs(t, jobs, 1)
		outcomes := make(chan Outcome, 1)
		go func() { outcomes <- registered[0].run(context.Background(), nopProgress{}) }()
		<-importer.started
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		requireNoError(t, d.Stop(stopCtx))
		if outcome := <-outcomes; outcome.Status != ImportStatusInterrupted {
			t.Fatalf("outcome = %+v", outcome)
		}
		mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
		if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusInterrupted {
			t.Fatalf("map = %+v", mapped)
		}
	})

	t.Run("durable shutdown beats late worker success", func(t *testing.T) {
		d, store, jobs, _, root, sub := importHarness(t)
		got, err := d.SubmitImport(sub)
		requireNoError(t, err)
		registered := waitForImportJobs(t, jobs, 1)
		store.claimMu.Lock()
		record := store.claims[got.ImportID]
		record.Status = ImportStatusInterrupted
		store.claims[got.ImportID] = record
		mapped := store.maps[importKey(sub.RunID, sub.Name)]
		mapped.Status, mapped.Error = ImportStatusInterrupted, "server interrupted"
		store.maps[importKey(sub.RunID, sub.Name)] = mapped
		store.claimMu.Unlock()
		outcome := registered[0].run(context.Background(), nopProgress{})
		if outcome.Status != ImportStatusInterrupted {
			t.Fatalf("outcome = %+v", outcome)
		}
		if _, err := os.Stat(filepath.Join(exchangeRunDir(root, "alpha", sub.RunID), sub.Name)); err != nil {
			t.Fatalf("late worker removed source: %v", err)
		}
	})
}

func TestSubmitImportDeleteFailureKeepsDurableSuccessAndShortCircuits(t *testing.T) {
	d, store, jobs, _, root, sub := importHarness(t)
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	source := filepath.Join(exchangeRunDir(root, "alpha", sub.RunID), sub.Name)
	requireNoError(t, os.Remove(source))
	requireNoError(t, os.Symlink(filepath.Join(root, "outside-secret"), source))
	outcome := registered[0].run(context.Background(), nopProgress{})
	if outcome.Status != ImportStatusSucceeded {
		t.Fatalf("outcome=%+v", outcome)
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusSucceeded || mapped.ResourceID == nil || mapped.Error == "" {
		t.Fatalf("map=%+v", mapped)
	}
	jobs.mu.Lock()
	jobs.imports = nil
	jobs.mu.Unlock()
	again, err := d.SubmitImport(sub)
	requireNoError(t, err)
	if again.ResourceID == nil || *again.ResourceID != *mapped.ResourceID || jobs.importCount() != 0 {
		t.Fatalf("short circuit=%+v jobs=%d", again, jobs.importCount())
	}
}

func TestSubmitImportManagedAdmissionFailureIsTerminalAndCleansTemp(t *testing.T) {
	d, store, jobs, _, root, sub := importHarness(t)
	jobs.importErr = errors.New("managed lane full")
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
		if mapped.Status == ImportStatusFailed {
			if _, statErr := os.Stat(filepath.Join(root, "import_tmp", got.ImportID)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("temp remains: %v", statErr)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("managed admission failure remained pending")
}

func TestRecoverInterruptsImportsAndRemovesNestedUploadSnapshot(t *testing.T) {
	root := t.TempDir()
	store := newImportLifecycleStore()
	actor := uint(7)
	store.claims["claim"] = ImportRecord{ID: "claim", RunID: "run", FileName: "out", Status: ImportStatusRunning, CreatedByUserID: &actor}
	store.maps[importKey("run", "out")] = ImportMapEntry{RunID: "run", FileName: "out", ImportID: "claim", Status: ImportStatusRunning}
	dir := filepath.Join(root, "import_tmp", "claim")
	requireNoError(t, os.MkdirAll(dir, 0o700))
	requireNoError(t, os.WriteFile(filepath.Join(dir, "upload-partial"), []byte("partial"), 0o600))
	d := NewDispatcher(Dependencies{Store: store, Jobs: &dispatcherTestJobs{}, Executor: dispatcherTestExecutor{store: store}, Settings: importTestSettings{root: root}, Inspector: &recoveryInspector{states: map[int][]GroupIdentity{}}})
	requireNoError(t, d.Recover(context.Background()))
	mapped, _, _ := store.ImportMap("run", "out")
	if mapped.Status != ImportStatusInterrupted {
		t.Fatalf("map = %+v", mapped)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp remains: %v", err)
	}
}
