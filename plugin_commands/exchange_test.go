package plugin_commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type exchangeTestSettings struct{ root string }

func (s exchangeTestSettings) StagingRoot() string              { return s.root }
func (s exchangeTestSettings) PendingPerPluginLimit() int       { return 100 }
func (s exchangeTestSettings) PerRunQuota() int64               { return 8 << 30 }
func (s exchangeTestSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s exchangeTestSettings) ExchangeRetention() time.Duration { return 7 * 24 * time.Hour }
func (s exchangeTestSettings) OutputRetention() time.Duration   { return 24 * time.Hour }
func (s exchangeTestSettings) CommandPath() string              { return "/bin" }

type exchangeTestStore struct {
	*dispatcherTestStore
	muImports          sync.Mutex
	nonterminalImports map[string]bool
}

func newExchangeTestStore() *exchangeTestStore {
	return &exchangeTestStore{
		dispatcherTestStore: newDispatcherTestStore(),
		nonterminalImports:  make(map[string]bool),
	}
}

func (s *exchangeTestStore) HasNonterminalImports(runID string) (bool, error) {
	s.muImports.Lock()
	defer s.muImports.Unlock()
	return s.nonterminalImports[runID], nil
}

func exchangeRunDir(root, plugin, runID string) string {
	return filepath.Join(root, "plugin_exchange", plugin, runID)
}

func addExchangeRun(t *testing.T, store *exchangeTestStore, root string, run RunRecord, files map[string]string) string {
	t.Helper()
	if run.Status == "" {
		run.Status = RunStatusSucceeded
	}
	if run.FinishedAt == nil && RunStatusTerminal(run.Status) {
		now := time.Now().UTC()
		run.FinishedAt = &now
	}
	store.runs[run.ID] = run
	dir := exchangeRunDir(root, run.PluginName, run.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func uintPtr(v uint) *uint { return &v }

func TestExchangeOwnershipStateAndNameMatrix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	settings := exchangeTestSettings{root: root}
	access := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}

	addExchangeRun(t, store, root, RunRecord{ID: "owned", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"ok.txt": "hello"})
	addExchangeRun(t, store, root, RunRecord{ID: "actorless", PluginName: "alpha", ActorlessAtSubmission: true}, map[string]string{"ok.txt": "shared"})
	addExchangeRun(t, store, root, RunRecord{ID: "deleted", PluginName: "alpha"}, map[string]string{"ok.txt": "denied"})
	addExchangeRun(t, store, root, RunRecord{ID: "running", PluginName: "alpha", CreatedByUserID: uintPtr(7), Status: RunStatusRunning}, map[string]string{"ok.txt": "partial"})
	addExchangeRun(t, store, root, RunRecord{ID: "unverified", PluginName: "alpha", CreatedByUserID: uintPtr(7), OutputUnverified: true}, map[string]string{"ok.txt": "unsafe"})
	addExchangeRun(t, store, root, RunRecord{ID: "other-plugin", PluginName: "beta", CreatedByUserID: uintPtr(7)}, map[string]string{"ok.txt": "other"})
	addExchangeRun(t, store, root, RunRecord{ID: "other-actor", PluginName: "alpha", CreatedByUserID: uintPtr(8)}, map[string]string{"ok.txt": "other"})
	addExchangeRun(t, store, root, RunRecord{ID: "swept", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, nil)
	if err := os.RemoveAll(exchangeRunDir(root, "alpha", "swept")); err != nil {
		t.Fatal(err)
	}

	exchange := NewExchange(store, settings)
	listing, err := exchange.List(access, "owned")
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Name != "ok.txt" {
		t.Fatalf("owned list = %+v, %v", listing, err)
	}
	body, err := exchange.Read(access, "actorless", "ok.txt", MaxReadBytes)
	if err != nil || string(body) != "shared" {
		t.Fatalf("actorless read = %q, %v", body, err)
	}
	if err := os.WriteFile(filepath.Join(exchangeRunDir(root, "alpha", "owned"), "discard.txt"), []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exchange.Discard(access, "owned", "discard.txt"); err != nil {
		t.Fatalf("discard regular file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exchangeRunDir(root, "alpha", "owned"), "discard.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded file still exists: %v", err)
	}

	for _, tc := range []struct {
		name, runID, phrase string
	}{
		{"missing", "missing", "run not found"},
		{"wrong plugin", "other-plugin", "run not found"},
		{"wrong actor", "other-actor", "run not found"},
		{"deleted actor", "deleted", "run not found"},
		{"nonterminal", "running", "run not finished"},
		{"swept", "swept", "run swept"},
		{"unverified", "unverified", "output unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := exchange.List(access, tc.runID)
			if err == nil || !strings.Contains(err.Error(), tc.phrase) {
				t.Fatalf("error = %v, want phrase %q", err, tc.phrase)
			}
		})
	}

	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b", strings.Repeat("x", MaxFileNameBytes+1)} {
		t.Run("invalid name "+fmt.Sprintf("%q", name), func(t *testing.T) {
			_, err := exchange.Read(access, "owned", name, 10)
			if err == nil || !strings.Contains(err.Error(), "invalid file name") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	for _, limit := range []int64{0, -1, MaxReadBytes + 1} {
		if _, err := exchange.Read(access, "owned", "ok.txt", limit); err == nil || !strings.Contains(err.Error(), "max_bytes") {
			t.Fatalf("limit %d error = %v", limit, err)
		}
	}
	if _, err := exchange.Read(access, "owned", "ok.txt", 4); err == nil || !strings.Contains(err.Error(), "exceeds max_bytes") {
		t.Fatalf("short read error = %v", err)
	}
	if _, err := exchange.Read(access, "owned", "gone.txt", 10); err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("missing file error = %v", err)
	}

	if err := exchange.DiscardRun(access, "unverified"); err != nil {
		t.Fatalf("discard unverified run: %v", err)
	}
	if _, err := os.Stat(exchangeRunDir(root, "alpha", "unverified")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified directory still exists: %v", err)
	}
}

func TestExchangeAuthorizesBeforeInputsAndLeaseState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	addExchangeRun(t, store, root, RunRecord{ID: "owned-order", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"ok": "ok"})
	addExchangeRun(t, store, root, RunRecord{ID: "running-order", PluginName: "alpha", CreatedByUserID: uintPtr(7), Status: RunStatusRunning}, map[string]string{"ok": "partial"})
	service := NewExchange(store, exchangeTestSettings{root: root}).(*exchangeService)
	owner := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}
	wrong := Access{PluginName: "alpha", ActorUserID: uintPtr(8)}

	if _, err := service.Read(wrong, "owned-order", "../bad", 0); !errors.Is(err, ErrExchangeRunNotFound) {
		t.Fatalf("unauthorized malformed read = %v, want run not found", err)
	}
	if err := service.Discard(wrong, "owned-order", "../bad"); !errors.Is(err, ErrExchangeRunNotFound) {
		t.Fatalf("unauthorized malformed discard = %v, want run not found", err)
	}
	if _, err := service.Read(owner, "running-order", "../bad", 0); !errors.Is(err, ErrExchangeRunNotFinished) {
		t.Fatalf("nonterminal malformed read = %v, want run not finished", err)
	}

	endSweep, ok := service.leases.BeginSweep("owned-order")
	if !ok {
		t.Fatal("begin sweep")
	}
	defer endSweep()
	if _, err := service.List(wrong, "owned-order"); !errors.Is(err, ErrExchangeRunNotFound) {
		t.Fatalf("unauthorized list during sweep = %v, want run not found", err)
	}
	if err := service.DiscardRun(wrong, "owned-order"); !errors.Is(err, ErrExchangeRunNotFound) {
		t.Fatalf("unauthorized discard_run during sweep = %v, want run not found", err)
	}
}

func TestExchangeStateRefusalPrecedesStagingRootInspection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "staging")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	store := newExchangeTestStore()
	for _, status := range []string{RunStatusQueued, RunStatusRunning} {
		id := "bad-root-" + status
		store.runs[id] = RunRecord{
			ID:              id,
			PluginName:      "alpha",
			CreatedByUserID: uintPtr(7),
			Status:          status,
		}
	}
	service := NewExchange(store, exchangeTestSettings{root: linkRoot})
	owner := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}
	wrong := Access{PluginName: "alpha", ActorUserID: uintPtr(8)}

	for _, status := range []string{RunStatusQueued, RunStatusRunning} {
		id := "bad-root-" + status
		if _, err := service.List(wrong, id); !errors.Is(err, ErrExchangeRunNotFound) {
			t.Fatalf("unauthorized %s run with bad root = %v, want run not found", status, err)
		}
		if _, err := service.List(owner, id); !errors.Is(err, ErrExchangeRunNotFinished) {
			t.Fatalf("authorized %s run with bad root = %v, want run not finished", status, err)
		}
	}
}

func TestExchangeRefusesSymlinkedManagedDirectoryComponents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	for _, component := range []string{"plugin_exchange", "plugin"} {
		t.Run(component, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			store := newExchangeTestStore()
			store.runs["component-link"] = RunRecord{ID: "component-link", PluginName: "alpha", CreatedByUserID: uintPtr(7), Status: RunStatusSucceeded}
			outsideRun := filepath.Join(outside, "alpha", "component-link")
			if err := os.MkdirAll(outsideRun, 0o700); err != nil {
				t.Fatal(err)
			}
			secret := filepath.Join(outsideRun, "secret")
			if err := os.WriteFile(secret, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch component {
			case "plugin_exchange":
				if err := os.Symlink(outside, filepath.Join(root, "plugin_exchange")); err != nil {
					t.Fatal(err)
				}
			case "plugin":
				if err := os.Mkdir(filepath.Join(root, "plugin_exchange"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "alpha"), filepath.Join(root, "plugin_exchange", "alpha")); err != nil {
					t.Fatal(err)
				}
			}

			exchange := NewExchange(store, exchangeTestSettings{root: root})
			access := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}
			if _, err := exchange.List(access, "component-link"); err == nil {
				t.Fatal("list followed a managed-directory symlink")
			}
			if err := exchange.DiscardRun(access, "component-link"); err == nil {
				t.Fatal("discard_run followed a managed-directory symlink")
			}
			body, err := os.ReadFile(secret)
			if err != nil || string(body) != "outside" {
				t.Fatalf("outside file changed: %q, %v", body, err)
			}
		})
	}
}

func TestExchangeRefusesSymlinkedStagingRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	realRoot := t.TempDir()
	store := newExchangeTestStore()
	addExchangeRun(t, store, realRoot, RunRecord{ID: "root-link", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"file": "secret"})
	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "staging")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	exchange := NewExchange(store, exchangeTestSettings{root: linkRoot})
	_, err := exchange.List(Access{PluginName: "alpha", ActorUserID: uintPtr(7)}, "root-link")
	if err == nil || !strings.Contains(err.Error(), "staging root is a symlink") {
		t.Fatalf("symlinked staging root error = %v", err)
	}
}

func TestExchangeRefusesSpecialFilesAndSymlinkSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	dir := addExchangeRun(t, store, root, RunRecord{ID: "attacks", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"victim": "inside"})
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret")
	if err := os.WriteFile(secret, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := makeTestFIFO(filepath.Join(dir, "fifo")); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	specialNames := []string{"link", "directory", "fifo"}
	if err := makeTestDevice(filepath.Join(dir, "device")); err == nil {
		specialNames = append(specialNames, "device")
	}

	exchange := NewExchange(store, exchangeTestSettings{root: root})
	access := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}
	listing, err := exchange.List(access, "attacks")
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "victim" {
		t.Fatalf("special entries leaked through list: %+v", listing.Entries)
	}
	for _, name := range specialNames {
		if _, err := exchange.Read(access, "attacks", name, MaxReadBytes); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("read %s error = %v", name, err)
		}
		if err := exchange.Discard(access, "attacks", name); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("discard %s error = %v", name, err)
		}
	}

	service := exchange.(*exchangeService)
	service.afterLstat = func(name string) {
		if name != "victim" {
			return
		}
		service.afterLstat = nil
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secret, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Read(access, "attacks", "victim", MaxReadBytes); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("swapped read error = %v", err)
	}
	got, err := os.ReadFile(secret)
	if err != nil || string(got) != "outside-secret" {
		t.Fatalf("outside secret changed: %q, %v", got, err)
	}

	if err := os.Remove(filepath.Join(dir, "victim")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "victim"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	service.afterLstat = func(name string) {
		if name != "victim" {
			return
		}
		service.afterLstat = nil
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secret, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.Discard(access, "attacks", "victim"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("swapped discard error = %v", err)
	}
	got, err = os.ReadFile(secret)
	if err != nil || string(got) != "outside-secret" {
		t.Fatalf("outside secret removed or changed: %q, %v", got, err)
	}
}

func TestExchangeSpecialFileSwapDoesNotBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	dir := addExchangeRun(t, store, root, RunRecord{ID: "fifo-swap", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"victim": "inside"})
	service := NewExchange(store, exchangeTestSettings{root: root}).(*exchangeService)
	service.afterLstat = func(name string) {
		service.afterLstat = nil
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if err := makeTestFIFO(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan error, 1)
	go func() {
		_, err := service.Read(Access{PluginName: "alpha", ActorUserID: uintPtr(7)}, "fifo-swap", "victim", MaxReadBytes)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("FIFO swap error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO swap blocked while holding the run lease")
	}
	end, ok := service.leases.BeginSweep("fifo-swap")
	if !ok {
		t.Fatal("read leaked the run lease after FIFO refusal")
	}
	end()
}

func TestExchangeDirectoryPathSwapStaysDescriptorRelative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	dir := addExchangeRun(t, store, root, RunRecord{ID: "dir-swap", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"inside": "original"})
	service := NewExchange(store, exchangeTestSettings{root: root}).(*exchangeService)
	moved := filepath.Join(filepath.Dir(dir), "moved-original")
	service.afterOpenRunDir = func() {
		service.afterOpenRunDir = nil
		if err := os.Rename(dir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "substitute"), []byte("wrong"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := service.List(Access{PluginName: "alpha", ActorUserID: uintPtr(7)}, "dir-swap")
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "inside" {
		t.Fatalf("listing escaped opened directory: %+v", listing.Entries)
	}
}

func TestDiscardRunDoesNotDeleteSubstitutedTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	dir := addExchangeRun(t, store, root, RunRecord{ID: "discard-swap", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, map[string]string{"inside": "original"})
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewExchange(store, exchangeTestSettings{root: root}).(*exchangeService)
	moved := filepath.Join(filepath.Dir(dir), "moved-discard")
	service.beforeRemoveRun = func() {
		service.beforeRemoveRun = nil
		if err := os.Rename(dir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.DiscardRun(Access{PluginName: "alpha", ActorUserID: uintPtr(7)}, "discard-swap"); err == nil {
		t.Fatal("discard_run accepted a substituted run path")
	}
	body, err := os.ReadFile(secret)
	if err != nil || string(body) != "outside" {
		t.Fatalf("substituted outside tree changed: %q, %v", body, err)
	}
}

func TestExchangeListTruncatesAndDiscardRunChecksPins(t *testing.T) {
	if testing.Short() {
		t.Skip("creates 10,001 small files")
	}
	if runtime.GOOS == "windows" {
		t.Skip("v1 exchange mediation is unsupported on Windows")
	}
	root := t.TempDir()
	store := newExchangeTestStore()
	dir := addExchangeRun(t, store, root, RunRecord{ID: "many", PluginName: "alpha", CreatedByUserID: uintPtr(7)}, nil)
	for i := 0; i < MaxListEntries+1; i++ {
		file, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%05d", i)), os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "not-a-file"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "00000"), filepath.Join(dir, "not-listed")); err != nil {
		t.Fatal(err)
	}

	service := NewExchange(store, exchangeTestSettings{root: root}).(*exchangeService)
	access := Access{PluginName: "alpha", ActorUserID: uintPtr(7)}
	listing, err := service.List(access, "many")
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != MaxListEntries || !listing.Truncated {
		t.Fatalf("listing = %d entries, truncated=%v", len(listing.Entries), listing.Truncated)
	}

	store.nonterminalImports["many"] = true
	if err := service.DiscardRun(access, "many"); err == nil || !strings.Contains(err.Error(), "nonterminal import") {
		t.Fatalf("nonterminal import error = %v", err)
	}
	store.nonterminalImports["many"] = false
	release, err := service.leases.Acquire("many")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DiscardRun(access, "many"); err == nil || !strings.Contains(err.Error(), "active file operation") {
		t.Fatalf("active lease error = %v", err)
	}
	release()
	if err := service.DiscardRun(access, "many"); err != nil {
		t.Fatal(err)
	}
}
