package plugin_commands

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type importTestSettings struct {
	root  string
	quota int64
}

func (s importTestSettings) StagingRoot() string        { return s.root }
func (s importTestSettings) PendingPerPluginLimit() int { return 100 }
func (s importTestSettings) PerRunQuota() int64 {
	if s.quota > 0 {
		return s.quota
	}
	return 8 << 30
}
func (s importTestSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s importTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s importTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s importTestSettings) CommandPath() string              { return "/bin" }

type importLifecycleStore struct {
	*exchangeTestStore
	claimMu        sync.Mutex
	maps           map[string]ImportMapEntry
	claims         map[string]ImportRecord
	finishFailures int
	finishAttempt  chan struct{}
	markEntered    chan struct{}
	allowMark      chan struct{}
	markOnce       sync.Once
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
		record.Status, record.PluginGeneration, record.CreatedByUserID, record.SourceDeletePending = ImportStatusPending, req.PluginGeneration, copyUint(req.CreatedByUserID), false
		s.claims[mapped.ImportID] = record
		mapped.Status, mapped.Error, mapped.ResourceID, mapped.SourceDeletePending = ImportStatusPending, "", nil, false
		s.maps[key] = mapped
		return ImportClaimResult{ImportID: mapped.ImportID, Status: ImportStatusPending, Enqueue: true}, nil
	case ImportStatusFailed, ImportStatusCancelled:
		record := ImportRecord{ID: req.ImportID, RunID: req.RunID, FileName: req.FileName, PluginGeneration: req.PluginGeneration, CreatedByUserID: copyUint(req.CreatedByUserID), Status: ImportStatusPending, CreatedAt: req.CreatedAt}
		s.claims[req.ImportID] = record
		mapped.ImportID, mapped.Status, mapped.Error, mapped.ResourceID, mapped.SourceDeletePending = req.ImportID, ImportStatusPending, "", nil, false
		s.maps[key] = mapped
		return ImportClaimResult{ImportID: req.ImportID, Status: ImportStatusPending, Created: true, Enqueue: true}, nil
	default:
		return ImportClaimResult{}, errors.New("bad map state")
	}
}
func (s *importLifecycleStore) MarkImportRunning(id string, started time.Time) (bool, error) {
	if s.markEntered != nil {
		s.markOnce.Do(func() { close(s.markEntered) })
		<-s.allowMark
	}
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
func (s *importLifecycleStore) CancelPendingImport(id, reason string, finished time.Time) (bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	record, ok := s.claims[id]
	if !ok || record.Status != ImportStatusPending {
		return false, nil
	}
	record.Status, record.Error, record.FinishedAt = ImportStatusCancelled, reason, &finished
	s.claims[id] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == id {
			mapped.Status, mapped.Error = ImportStatusCancelled, reason
			s.maps[key] = mapped
		}
	}
	return true, nil
}

func (s *importLifecycleStore) FinishImport(id string, finish ImportFinish) (bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	if s.finishFailures > 0 {
		s.finishFailures--
		if s.finishAttempt != nil {
			select {
			case s.finishAttempt <- struct{}{}:
			default:
			}
		}
		return false, errors.New("finish unavailable")
	}
	record, ok := s.claims[id]
	if !ok || ImportStatusTerminal(record.Status) {
		return false, nil
	}
	record.Status, record.Error, record.SourceDeletePending, record.FinishedAt = finish.Status, finish.Error, finish.SourceDeletePending, &finish.FinishedAt
	s.claims[id] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == id {
			mapped.Status, mapped.Error, mapped.ResourceID, mapped.SourceDeletePending = finish.Status, finish.Error, copyUint(finish.ResourceID), finish.SourceDeletePending
			s.maps[key] = mapped
		}
	}
	return true, nil
}
func (s *importLifecycleStore) SetImportSourceDeletePending(id string, pending bool) error {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	record, ok := s.claims[id]
	if !ok || record.Status != ImportStatusSucceeded {
		return errors.New("import is not succeeded")
	}
	record.SourceDeletePending = pending
	s.claims[id] = record
	for key, mapped := range s.maps {
		if mapped.ImportID == id && mapped.Status == ImportStatusSucceeded {
			mapped.SourceDeletePending = pending
			s.maps[key] = mapped
			return nil
		}
	}
	return errors.New("succeeded import map is missing")
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

type importTestImporter struct {
	mu            sync.Mutex
	validations   []ImportValidation
	bodies        [][]byte
	validateErr   error
	resourceID    uint
	started       chan struct{}
	proceed       chan struct{}
	ignoreContext bool
	once          sync.Once
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
		if i.ignoreContext {
			<-i.proceed
		} else {
			select {
			case <-i.proceed:
			case <-ctx.Done():
				return 0, context.Cause(ctx)
			}
		}
	}
	if source.File == nil {
		return 0, errors.New("snapshot file is unavailable")
	}
	if _, err := source.File.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	body, err := io.ReadAll(source.File)
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
			if got.CompletionRegistered != tc.wantQueue {
				t.Fatalf("completion registered = %v, want %v", got.CompletionRegistered, tc.wantQueue)
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

func TestSubmitImportDeletesSourceAfterDurableSuccess(t *testing.T) {
	d, store, jobs, importer, root, sub := importHarness(t)
	completed := make(chan ImportResult, 1)
	sub.Completion = func(r ImportResult) { completed <- r }
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	claimDir := filepath.Join(root, "import_tmp", got.ImportID)
	if _, err := os.Stat(claimDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("submission copied bytes synchronously: %v", err)
	}
	outcome := registered[0].run(context.Background(), nopProgress{})
	if outcome.Status != ImportStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	result := <-completed
	if !result.OK || result.ResourceID == nil || *result.ResourceID != 91 || result.SourceDeletePending {
		t.Fatalf("result = %+v", result)
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.Status != ImportStatusSucceeded || mapped.ResourceID == nil || mapped.Error != "" || mapped.SourceDeletePending {
		t.Fatalf("map = %+v", mapped)
	}
	sourcePath := filepath.Join(exchangeRunDir(root, "alpha", sub.RunID), sub.Name)
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful import source still exists: %v", err)
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

func TestSubmitImportEnforcesQuotaBeforeCopying(t *testing.T) {
	d, store, jobs, importer, root, sub := importHarness(t)
	d.deps.Settings = importTestSettings{root: root, quota: 20}
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	outcome := registered[0].run(context.Background(), nopProgress{})
	if outcome.Status != ImportStatusFailed || !strings.Contains(outcome.Error, "quota") {
		t.Fatalf("outcome = %+v", outcome)
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusFailed {
		t.Fatalf("map = %+v", mapped)
	}
	importer.mu.Lock()
	defer importer.mu.Unlock()
	if len(importer.bodies) != 0 {
		t.Fatal("quota-refused import copied bytes into the application adapter")
	}
}

func TestImportCompletionWaitsForDurableTerminalWrite(t *testing.T) {
	d, store, jobs, _, _, sub := importHarness(t)
	completed := make(chan ImportResult, 1)
	sub.Completion = func(result ImportResult) { completed <- result }
	store.claimMu.Lock()
	store.finishFailures = 2
	store.finishAttempt = make(chan struct{}, 2)
	store.claimMu.Unlock()
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	outcomes := make(chan Outcome, 1)
	go func() { outcomes <- registered[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-store.finishAttempt:
	case <-time.After(time.Second):
		t.Fatal("terminal persistence was not attempted")
	}
	select {
	case result := <-completed:
		t.Fatalf("completion published before durable terminal state: %+v", result)
	default:
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusRunning {
		t.Fatalf("map before durable finish = %+v", mapped)
	}
	select {
	case outcome := <-outcomes:
		if outcome.Status != ImportStatusSucceeded {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("import did not retry terminal persistence")
	}
	result := <-completed
	if !result.OK {
		t.Fatalf("completion = %+v", result)
	}
	mapped, _, _ = store.ImportMap(sub.RunID, sub.Name)
	if mapped.Status != ImportStatusSucceeded {
		t.Fatalf("map after retry = %+v", mapped)
	}
}

func TestImportTempRootSymlinkIsRefused(t *testing.T) {
	d, store, jobs, _, root, sub := importHarness(t)
	outside := t.TempDir()
	requireNoError(t, os.Symlink(outside, filepath.Join(root, "import_tmp")))
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	outcome := registered[0].run(context.Background(), nopProgress{})
	if outcome.Status != ImportStatusFailed {
		t.Fatalf("outcome = %+v", outcome)
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusFailed {
		t.Fatalf("map = %+v", mapped)
	}
	entries, err := os.ReadDir(outside)
	requireNoError(t, err)
	if len(entries) != 0 {
		t.Fatalf("wrote through import_tmp symlink: %+v", entries)
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

	t.Run("timed out shutdown leaves claimed import recoverable", func(t *testing.T) {
		d, store, jobs, importer, _, sub := importHarness(t)
		importer.started, importer.proceed = make(chan struct{}), make(chan struct{})
		importer.ignoreContext = true
		got, err := d.SubmitImport(sub)
		requireNoError(t, err)
		registered := waitForImportJobs(t, jobs, 1)
		outcomes := make(chan Outcome, 1)
		go func() { outcomes <- registered[0].run(context.Background(), nopProgress{}) }()
		<-importer.started
		stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err = d.Stop(stopCtx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop error = %v, want deadline exceeded", err)
		}
		mapped, _, err := store.ImportMap(sub.RunID, sub.Name)
		requireNoError(t, err)
		if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusRunning {
			t.Fatalf("timed-out shutdown destroyed import recovery evidence: %+v", mapped)
		}
		close(importer.proceed)
		select {
		case <-outcomes:
		case <-time.After(time.Second):
			t.Fatal("import worker did not release")
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

func TestSubmitImportDisableCancelsAnActiveButDurablyPendingClaim(t *testing.T) {
	d, store, jobs, _, _, sub := importHarness(t)
	store.markEntered = make(chan struct{})
	store.allowMark = make(chan struct{})
	got, err := d.SubmitImport(sub)
	requireNoError(t, err)
	registered := waitForImportJobs(t, jobs, 1)
	outcomes := make(chan Outcome, 1)
	go func() { outcomes <- registered[0].run(context.Background(), nopProgress{}) }()
	select {
	case <-store.markEntered:
	case <-time.After(time.Second):
		t.Fatal("import did not reach the pending-to-running transition")
	}

	requireNoError(t, d.DisablePlugin("alpha", "plugin disabled"))
	close(store.allowMark)
	select {
	case outcome := <-outcomes:
		if outcome.Status != ImportStatusCancelled || outcome.Error != "plugin disabled" {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("disabled pending import did not finish")
	}
	mapped, _, _ := store.ImportMap(sub.RunID, sub.Name)
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusCancelled || mapped.Error != "plugin disabled" {
		t.Fatalf("map = %+v", mapped)
	}
}

func TestUnlinkExchangeOpenedRegularAtRefusesAnExistingReplacement(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "run")
	requireNoError(t, os.MkdirAll(dirPath, 0o700))
	path := filepath.Join(dirPath, "result.bin")
	requireNoError(t, os.WriteFile(path, []byte("admitted"), 0o600))
	dir, err := os.Open(dirPath)
	requireNoError(t, err)
	defer dir.Close()
	admitted, err := openExchangeRegularAt(dir, "result.bin", nil)
	requireNoError(t, err)
	defer admitted.Close()

	requireNoError(t, os.Rename(path, filepath.Join(dirPath, "admitted-old")))
	requireNoError(t, os.WriteFile(path, []byte("replacement"), 0o600))
	err = unlinkExchangeOpenedRegularAt(dir, "result.bin", admitted)
	if !errors.Is(err, errExchangePathChanged) {
		t.Fatalf("cleanup error = %v, want changed-path refusal", err)
	}
	got, readErr := os.ReadFile(path)
	requireNoError(t, readErr)
	if string(got) != "replacement" {
		t.Fatalf("replacement = %q", got)
	}
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
	if mapped.ImportID != got.ImportID || mapped.Status != ImportStatusSucceeded || mapped.ResourceID == nil || mapped.Error != "" || !mapped.SourceDeletePending {
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
