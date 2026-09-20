package plugin_commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleStore struct {
	*dispatcherTestStore
	expired        []RunRecord
	nonterminal    map[string]bool
	prunedBefore   time.Time
	pruneCalls     atomic.Int32
	pruneNotify    chan struct{}
	pruneBlock     <-chan struct{}
	pruneStarted   chan struct{}
	blockAfter     int32
	exchangeMarked []string
	requestedLimit int
}

func (s *lifecycleStore) ExpiredTerminalRuns(_ time.Time, limit int) ([]RunRecord, error) {
	s.requestedLimit = limit
	if limit > len(s.expired) {
		limit = len(s.expired)
	}
	return append([]RunRecord(nil), s.expired[:limit]...), nil
}
func (s *lifecycleStore) MarkRunExchangeRemoved(runID string, _ time.Time) error {
	s.exchangeMarked = append(s.exchangeMarked, runID)
	for i, run := range s.expired {
		if run.ID == runID {
			s.expired = append(s.expired[:i], s.expired[i+1:]...)
			break
		}
	}
	return nil
}
func (s *lifecycleStore) HasNonterminalImports(runID string) (bool, error) {
	return s.nonterminal[runID], nil
}
func (s *lifecycleStore) PruneRunOutputs(before time.Time) (int64, error) {
	s.prunedBefore = before
	call := s.pruneCalls.Add(1)
	if s.pruneNotify != nil {
		select {
		case s.pruneNotify <- struct{}{}:
		default:
		}
	}
	if s.pruneBlock != nil && call > s.blockAfter {
		if s.pruneStarted != nil {
			select {
			case s.pruneStarted <- struct{}{}:
			default:
			}
		}
		<-s.pruneBlock
	}
	return 1, nil
}

func TestPluginCommandLifecycleSweepHonorsLeasesImportsAndRetention(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	store := &lifecycleStore{
		dispatcherTestStore: newDispatcherTestStore(),
		nonterminal:         map[string]bool{"importing": true},
		expired: []RunRecord{
			{ID: "delete", PluginName: "plugin", Status: RunStatusSucceeded},
			{ID: "leased", PluginName: "plugin", Status: RunStatusFailed},
			{ID: "importing", PluginName: "plugin", Status: RunStatusCancelled},
		},
	}
	for _, id := range []string{"delete", "leased", "importing"} {
		dir := filepath.Join(root, "plugin_exchange", "plugin", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "out"), []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	leases := NewLeaseManager()
	release, err := leases.Acquire("leased")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	settings := lifecycleSettings{root: root, exchange: 2 * time.Hour, output: 3 * time.Hour}
	d := NewDispatcher(Dependencies{Store: store, Settings: settings, Leases: leases})

	if err := d.sweep(now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "plugin_exchange", "plugin", "delete")); !os.IsNotExist(err) {
		t.Fatalf("expired unpinned run remains: %v", err)
	}
	for _, id := range []string{"leased", "importing"} {
		if _, err := os.Stat(filepath.Join(root, "plugin_exchange", "plugin", id)); err != nil {
			t.Fatalf("protected run %s removed: %v", id, err)
		}
	}
	if store.pruneCalls.Load() != 1 || !store.prunedBefore.Equal(now.Add(-settings.output)) {
		t.Fatalf("prune = calls %d before %s", store.pruneCalls.Load(), store.prunedBefore)
	}
}

func TestPluginCommandLifecycleSweepUsesOneBoundedBatchAndMarksRemovedRuns(t *testing.T) {
	root := t.TempDir()
	store := &lifecycleStore{
		dispatcherTestStore: newDispatcherTestStore(),
		nonterminal:         map[string]bool{},
		expired: []RunRecord{
			{ID: "one", PluginName: "plugin", Status: RunStatusSucceeded},
			{ID: "two", PluginName: "plugin", Status: RunStatusFailed},
			{ID: "three", PluginName: "plugin", Status: RunStatusCancelled},
		},
	}
	for _, id := range []string{"one", "two"} {
		dir := filepath.Join(root, "plugin_exchange", "plugin", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	d := NewDispatcher(Dependencies{Store: store, Settings: lifecycleSettings{root: root, exchange: time.Hour, output: time.Hour}})
	d.sweepBatchSize = 2

	if err := d.sweep(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if store.requestedLimit != 2 {
		t.Fatalf("expired query limit = %d, want 2", store.requestedLimit)
	}
	if got := strings.Join(store.exchangeMarked, ","); got != "one,two" {
		t.Fatalf("marked runs = %q, want one,two", got)
	}
	if len(store.expired) != 1 || store.expired[0].ID != "three" {
		t.Fatalf("unswept rows = %+v, want only three", store.expired)
	}
}

func TestPluginCommandLifecyclePeriodicSweepStopsWithDispatcher(t *testing.T) {
	base := newDispatcherTestStore()
	store := &lifecycleStore{dispatcherTestStore: base, nonterminal: map[string]bool{}, pruneNotify: make(chan struct{}, 8)}
	settings := lifecycleSettings{root: t.TempDir(), exchange: time.Hour, output: time.Hour}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: &dispatcherTestJobs{}, Executor: dispatcherTestExecutor{store: base},
		Settings: settings,
	})
	d.sweepInterval = 10 * time.Millisecond
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Start performs the first sweep synchronously. Observe one subsequent tick.
	select {
	case <-store.pruneNotify:
	case <-time.After(time.Second):
		t.Fatal("initial sweep was not observed")
	}
	select {
	case <-store.pruneNotify:
	case <-time.After(time.Second):
		t.Fatal("periodic sweep was not observed")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	stoppedAt := store.pruneCalls.Load()
	time.Sleep(30 * time.Millisecond)
	if got := store.pruneCalls.Load(); got != stoppedAt {
		t.Fatalf("sweep continued after Stop: before=%d after=%d", stoppedAt, got)
	}
}

func TestDispatcherStopHonorsDeadlineWhilePeriodicSweepIsBlocked(t *testing.T) {
	base := newDispatcherTestStore()
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	store := &lifecycleStore{
		dispatcherTestStore: base,
		nonterminal:         map[string]bool{},
		pruneBlock:          block,
		pruneStarted:        started,
		blockAfter:          1, // Start's required initial sweep remains synchronous.
	}
	d := NewDispatcher(Dependencies{
		Store: store, Jobs: &dispatcherTestJobs{}, Executor: dispatcherTestExecutor{store: base},
		Settings: lifecycleSettings{root: t.TempDir(), exchange: time.Hour, output: time.Hour},
	})
	d.sweepInterval = time.Millisecond
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		close(block)
		t.Fatal("periodic sweep did not block")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	err := d.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		close(block)
		t.Fatalf("Stop error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		close(block)
		t.Fatalf("Stop exceeded its bound: %s", elapsed)
	}
	close(block)
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not finish after blocked sweep released")
	}
}

type lifecycleSettings struct {
	root     string
	exchange time.Duration
	output   time.Duration
}

func (s lifecycleSettings) StagingRoot() string              { return s.root }
func (s lifecycleSettings) PendingPerPluginLimit() int       { return 100 }
func (s lifecycleSettings) PerRunQuota() int64               { return 8 << 30 }
func (s lifecycleSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s lifecycleSettings) ExchangeRetention() time.Duration { return s.exchange }
func (s lifecycleSettings) OutputRetention() time.Duration   { return s.output }
func (s lifecycleSettings) CommandPath() string              { return "/bin" }
