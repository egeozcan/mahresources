package plugin_system

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/plugin_commands"

	lua "github.com/yuin/gopher-lua"
)

type transactionMarker struct {
	EntityQuerier
	EntityWriter
	PluginLogger
	KVStore
}

type commandLuaHost struct {
	mu            sync.Mutex
	requests      []plugin_commands.CommandRequest
	submitEntered chan struct{}
	submitRelease <-chan struct{}
	submitOnce    sync.Once
	enforceAccess bool
	runs          []plugin_commands.RunView
	listing       plugin_commands.Listing
	readBody      []byte
	imported      plugin_commands.ImportSubmitResult
	imports       []plugin_commands.ImportSubmission
	thumbnails    []thumbnailCall
	discards      []string
}

type thumbnailCall struct {
	access     plugin_commands.Access
	runID      string
	name       string
	resourceID uint
}

func (h *commandLuaHost) SubmitPluginCommand(req plugin_commands.CommandRequest) (string, error) {
	if h.submitEntered != nil {
		h.submitOnce.Do(func() { close(h.submitEntered) })
	}
	if h.submitRelease != nil {
		<-h.submitRelease
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req)
	return "run-123", nil
}

func (h *commandLuaHost) allows(access plugin_commands.Access, runID string) bool {
	if !h.enforceAccess {
		return true
	}
	for _, run := range h.runs {
		if run.ID == runID {
			return access.AllowsRun(run.RunRecord)
		}
	}
	return false
}

func (h *commandLuaHost) CommandRuns(access plugin_commands.Access) ([]plugin_commands.RunView, error) {
	if !h.enforceAccess {
		return h.runs, nil
	}
	var visible []plugin_commands.RunView
	for _, run := range h.runs {
		if access.AllowsRun(run.RunRecord) {
			visible = append(visible, run)
		}
	}
	return visible, nil
}
func (h *commandLuaHost) ListCommandFiles(access plugin_commands.Access, runID string) (plugin_commands.Listing, error) {
	if !h.allows(access, runID) {
		return plugin_commands.Listing{}, fmt.Errorf("command run not accessible")
	}
	return h.listing, nil
}
func (h *commandLuaHost) ReadCommandFile(access plugin_commands.Access, runID, name string, maxBytes int64) ([]byte, error) {
	if !h.allows(access, runID) {
		return nil, fmt.Errorf("command run not accessible")
	}
	return append([]byte(nil), h.readBody...), nil
}
func (h *commandLuaHost) SetCommandResourceThumbnail(_ context.Context, access plugin_commands.Access, runID, name string, resourceID uint) error {
	if !h.allows(access, runID) {
		return fmt.Errorf("command run not accessible")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.thumbnails = append(h.thumbnails, thumbnailCall{access: access, runID: runID, name: name, resourceID: resourceID})
	return nil
}

func (h *commandLuaHost) SubmitCommandImport(sub plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error) {
	if !h.allows(sub.Access, sub.RunID) {
		return plugin_commands.ImportSubmitResult{}, fmt.Errorf("command run not accessible")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.imports = append(h.imports, sub)
	return h.imported, nil
}
func (h *commandLuaHost) DiscardCommandFile(access plugin_commands.Access, runID, name string) error {
	if !h.allows(access, runID) {
		return fmt.Errorf("command run not accessible")
	}
	h.discards = append(h.discards, runID+"/"+name)
	return nil
}
func (h *commandLuaHost) DiscardCommandRun(access plugin_commands.Access, runID string) error {
	if !h.allows(access, runID) {
		return fmt.Errorf("command run not accessible")
	}
	h.discards = append(h.discards, runID)
	return nil
}

func commandPluginSource(capabilities string) string {
	return `
plugin = {
  name = "commander", version = "1", api_version = 1,
  capabilities = {` + capabilities + `},
  commands = {{ name = "download", argv = {"tool", "--", "{{url}}"}, timeout = 60, sensitive_params = {"url"} }}
}
function init() end
`
}

func enableCommandPlugin(t *testing.T, capabilities string, host *commandLuaHost) (*PluginManager, *lua.LState) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "commander", commandPluginSource(capabilities))
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if host != nil {
		pm.SetCommandSubmitter(host)
		pm.SetExchangeMediator(host)
	}
	store := newSharedConsentStore()
	discovered := pm.GetDiscoveredPlugin("commander")
	if discovered == nil {
		t.Fatal("command plugin was not discovered")
	}
	grants, err := GrantsForEnable(discovered.Manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	store.records["commander"] = grants
	pm.SetConsentStore(store)
	if err := pm.EnablePlugin("commander"); err != nil {
		t.Fatal(err)
	}
	return pm, stateForPlugin(t, pm, "commander")
}

func TestPluginWithoutCommandsCapabilityReceivesNeitherModule(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "plain", `plugin = {name="plain", version="1", api_version=1, capabilities={"db:write"}}
function init() end`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	store := newSharedConsentStore()
	discovered := pm.GetDiscoveredPlugin("plain")
	grants, err := GrantsForEnable(discovered.Manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	store.records["plain"] = grants
	pm.SetConsentStore(store)
	if err := pm.EnablePlugin("plain"); err != nil {
		t.Fatal(err)
	}
	L := stateForPlugin(t, pm, "plain")
	if err := L.DoString(`assert(mah.commands == nil); assert(mah.fs == nil)`); err != nil {
		t.Fatal(err)
	}
}

func TestCommandHostUnavailableExplainsAutomaticRecoveryAndActivatesWithoutReload(t *testing.T) {
	pm, L := enableCommandPlugin(t, `"commands"`, nil)
	if err := L.DoString(`__missing_id, __missing_err = mah.commands.run("download", {url="literal"})`); err != nil {
		t.Fatal(err)
	}
	message := L.GetGlobal("__missing_err").String()
	for _, want := range []string{"runtime is unavailable", "quarantined recovery retries automatically", "/logs"} {
		if !strings.Contains(message, want) {
			t.Fatalf("unavailable command error %q does not contain %q", message, want)
		}
	}
	if strings.Contains(message, "startup recovery completes") {
		t.Fatalf("unavailable command error retained stale startup-only wording: %q", message)
	}

	host := &commandLuaHost{}
	pm.SetCommandSubmitter(host)
	if err := L.DoString(`__healed_id, __healed_err = mah.commands.run("download", {url="literal"})`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__healed_id").String() != "run-123" || L.GetGlobal("__healed_err") != lua.LNil {
		t.Fatalf("published command host was not resolved by the loaded plugin: %v / %v", L.GetGlobal("__healed_id"), L.GetGlobal("__healed_err"))
	}
}

func TestCommandCapabilityInstallsCommandsAndFSWithWriteSplit(t *testing.T) {
	host := &commandLuaHost{}
	_, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`
assert(type(mah.commands) == "table")
assert(type(mah.commands.run) == "function")
assert(type(mah.fs) == "table")
assert(type(mah.fs.runs) == "function")
assert(mah.fs.create_resource == nil)
`); err != nil {
		t.Fatal(err)
	}

	_, writer := enableCommandPlugin(t, `"commands", "db:write"`, &commandLuaHost{})
	if err := writer.DoString(`assert(type(mah.fs.create_resource) == "function")`); err != nil {
		t.Fatal(err)
	}
}

func TestCommandsRunCapturesLiteralParamsActorGenerationAndCallback(t *testing.T) {
	host := &commandLuaHost{}
	pm, L := enableCommandPlugin(t, `"commands"`, host)
	L.SetContext(withInvocation(context.Background(), NewInvocation(77)))
	if err := L.DoString(`
__run_id, __run_err = mah.commands.run("download", {url="https://example.invalid/a?x=1&y=2"}, function(result)
  __command_result = result
end)
`); err != nil {
		t.Fatal(err)
	}
	L.RemoveContext()
	if got := L.GetGlobal("__run_id").String(); got != "run-123" {
		t.Fatalf("run id = %q", got)
	}

	host.mu.Lock()
	if len(host.requests) != 1 {
		host.mu.Unlock()
		t.Fatalf("requests = %d", len(host.requests))
	}
	req := host.requests[0]
	host.mu.Unlock()
	if req.ActorUserID == nil || *req.ActorUserID != 77 || req.PluginGeneration == 0 {
		t.Fatalf("request identity = %+v generation=%d", req.ActorUserID, req.PluginGeneration)
	}
	if req.Params["url"] != "https://example.invalid/a?x=1&y=2" || req.Declaration.Name != "download" {
		t.Fatalf("request = %+v", req)
	}
	if !pm.GenerationActive("commander", req.PluginGeneration) {
		t.Fatal("request did not capture the live generation")
	}

	exit := 9
	req.Completion(plugin_commands.Result{OK: false, ExitCode: &exit, Error: "failed", RunID: "run-123"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu := pm.LockVM(L)
		if mu == nil {
			t.Fatal("plugin disappeared")
		}
		result := L.GetGlobal("__command_result")
		if tbl, ok := result.(*lua.LTable); ok {
			if tbl.RawGetString("run_id").String() != "run-123" || tbl.RawGetString("exit_code").String() != "9" || tbl.RawGetString("ok") != lua.LFalse {
				mu.Unlock()
				t.Fatalf("callback result = %v", result)
			}
			mu.Unlock()
			return
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("command callback did not run")
}

func TestCommandsSurfaceContainsOnlyApprovedRunFunction(t *testing.T) {
	host := &commandLuaHost{}
	_, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`
assert(type(mah.commands.run) == "function")
assert(mah.commands.preview == nil)
`); err != nil {
		t.Fatal(err)
	}
}

func TestCommandsRemainUnavailableUntilHostIsPublished(t *testing.T) {
	_, L := enableCommandPlugin(t, `"commands"`, nil)
	if err := L.DoString(`__id, __err = mah.commands.run("download", {url="literal"})`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__id") != lua.LNil || L.GetGlobal("__err") == lua.LNil {
		t.Fatalf("unwired command result = %v/%v", L.GetGlobal("__id"), L.GetGlobal("__err"))
	}
}

func TestCommandCallbackDropsAfterGenerationRevocation(t *testing.T) {
	host := &commandLuaHost{}
	pm, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`mah.commands.run("download", {url="literal"}, function() __revoked_callback = true end)`); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	req := host.requests[0]
	host.mu.Unlock()
	pm.mu.Lock()
	delete(pm.generations, L)
	for i, info := range pm.plugins {
		if info.Name == "commander" {
			pm.plugins = append(pm.plugins[:i], pm.plugins[i+1:]...)
			break
		}
	}
	pm.mu.Unlock()
	req.Completion(plugin_commands.Result{OK: true, RunID: "run-123"})
	settled := make(chan struct{})
	go func() {
		pm.actionWaitGroup("commander").Wait()
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(time.Second):
		t.Fatal("revoked command completion did not settle")
	}
	if L.GetGlobal("__revoked_callback") != lua.LNil {
		t.Fatal("revoked generation callback executed")
	}
}

func TestCommandsAndFSRefuseInsideDBTransaction(t *testing.T) {
	host := &commandLuaHost{}
	_, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
	invocation := NewInvocation(12).BoundToTransaction(&transactionMarker{})
	L.SetContext(withInvocation(context.Background(), invocation))
	defer L.RemoveContext()
	if err := L.DoString(`
__run, __run_err = mah.commands.run("download", {url="literal"})
__runs, __runs_err = mah.fs.runs()
__import, __import_err = mah.fs.create_resource("run", "out", {})
__thumbnail, __thumbnail_err = mah.fs.set_resource_thumbnail("run", "cover.jpg", 1)
`); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"__run", "__run_err"}, {"__runs", "__runs_err"}, {"__import", "__import_err"}, {"__thumbnail", "__thumbnail_err"}} {
		if L.GetGlobal(pair[0]) != lua.LNil || L.GetGlobal(pair[1]) == lua.LNil {
			t.Fatalf("transaction refusal %s = %v/%v", pair[0], L.GetGlobal(pair[0]), L.GetGlobal(pair[1]))
		}
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.requests) != 0 || len(host.imports) != 0 || len(host.thumbnails) != 0 {
		t.Fatal("transaction-bound request reached command host")
	}
}

func TestCommandsRunRejectsNonStringParamsBeforeHostSubmission(t *testing.T) {
	host := &commandLuaHost{}
	_, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`mah.commands.run("download", {url=42})`); err == nil {
		t.Fatal("non-string parameter was accepted")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.requests) != 0 {
		t.Fatal("invalid request reached the host")
	}
}

func TestCommandsRunFromCoroutineUsesRootVMForAdmissionAndCallback(t *testing.T) {
	host := &commandLuaHost{}
	pm, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`
local co = coroutine.create(function()
  __coroutine_run, __coroutine_err = mah.commands.run("download", {url="literal"}, function(result)
    __coroutine_result = result.run_id
  end)
end)
local ok, err = coroutine.resume(co)
assert(ok, err)
`); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	if len(host.requests) != 1 {
		host.mu.Unlock()
		t.Fatalf("requests = %d", len(host.requests))
	}
	request := host.requests[0]
	host.mu.Unlock()
	request.Completion(plugin_commands.Result{OK: true, RunID: "run-123"})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu := pm.LockVM(L)
		if mu == nil {
			t.Fatal("plugin disappeared")
		}
		got := L.GetGlobal("__coroutine_result").String()
		mu.Unlock()
		if got == "run-123" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("coroutine command callback did not run on the root VM")
}

func TestCommandAdmissionClosedBeforeEnableBlocksThePublishedGeneration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "commander", commandPluginSource(`"commands"`))
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	host := &commandLuaHost{}
	pm.SetCommandSubmitter(host)
	store := newSharedConsentStore()
	discovered := pm.GetDiscoveredPlugin("commander")
	grants, err := GrantsForEnable(discovered.Manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	store.records["commander"] = grants
	pm.SetConsentStore(store)

	generation, _ := pm.ClosePluginCommandAdmission("commander")
	if generation != 0 {
		t.Fatalf("generation before enable = %d, want 0", generation)
	}
	if err := pm.EnablePlugin("commander"); err != nil {
		t.Fatal(err)
	}
	L := stateForPlugin(t, pm, "commander")
	if err := L.DoString(`__blocked_id, __blocked_err = mah.commands.run("download", {url="blocked"})`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__blocked_id") != lua.LNil || L.GetGlobal("__blocked_err") == lua.LNil {
		t.Fatalf("future generation was admitted through a pre-existing close: %v/%v", L.GetGlobal("__blocked_id"), L.GetGlobal("__blocked_err"))
	}
	host.mu.Lock()
	if len(host.requests) != 0 {
		host.mu.Unlock()
		t.Fatal("closed future generation reached the host")
	}
	host.mu.Unlock()

	pm.ReopenPluginCommandAdmission("commander", generation)
	if err := L.DoString(`__reopened_id, __reopened_err = mah.commands.run("download", {url="allowed"})`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__reopened_id").String() != "run-123" || L.GetGlobal("__reopened_err") != lua.LNil {
		t.Fatalf("reopened generation = %v/%v", L.GetGlobal("__reopened_id"), L.GetGlobal("__reopened_err"))
	}
}

func TestCommandAdmissionCloseWaitsForSubmissionAndRefusesLaterCalls(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	host := &commandLuaHost{submitEntered: entered, submitRelease: release}
	pm, L := enableCommandPlugin(t, `"commands"`, host)

	callDone := make(chan error, 1)
	go func() {
		callDone <- L.DoString(`__first_id, __first_err = mah.commands.run("download", {url="first"})`)
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("submission did not reach the host")
	}

	closeDone := make(chan struct{})
	go func() {
		pm.ClosePluginCommandAdmission("commander")
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("admission closed before the in-flight host submission returned")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-callDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("admission close did not finish")
	}

	if err := L.DoString(`__late_id, __late_err = mah.commands.run("download", {url="late"})`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__late_id") != lua.LNil || L.GetGlobal("__late_err") == lua.LNil {
		t.Fatalf("late admission = %v/%v", L.GetGlobal("__late_id"), L.GetGlobal("__late_err"))
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.requests) != 1 {
		t.Fatalf("late command reached host; requests = %d", len(host.requests))
	}
}

func TestCommandsAndFSRefuseOnWindowsRuntime(t *testing.T) {
	host := &commandLuaHost{}
	pm, L := enableCommandPlugin(t, `"commands"`, host)
	pm.commandRuntimeGOOS = "windows"
	if err := L.DoString(`
__windows_run, __windows_run_err = mah.commands.run("download", {url="literal"})
__windows_runs, __windows_runs_err = mah.fs.runs()
`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__windows_run") != lua.LNil || L.GetGlobal("__windows_run_err") == lua.LNil ||
		L.GetGlobal("__windows_runs") != lua.LNil || L.GetGlobal("__windows_runs_err") == lua.LNil {
		t.Fatal("Windows command surfaces did not fail closed")
	}
}

func TestDurableCallbackPanicIsRecoveredAndReleasesBarrier(t *testing.T) {
	released := false
	runProtectedDurableCallback("test", func() { released = true }, func() { panic("boom") })
	if !released {
		t.Fatal("panicking callback retained its lifecycle barrier")
	}
}

func TestCommandCallbackTimeoutReleasesDisableBarrier(t *testing.T) {
	host := &commandLuaHost{}
	pm, L := enableCommandPlugin(t, `"commands"`, host)
	pm.durableCallbackTimeout = 20 * time.Millisecond
	if err := L.DoString(`mah.commands.run("download", {url="literal"}, function() while true do end end)`); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	request := host.requests[0]
	host.mu.Unlock()
	request.Completion(plugin_commands.Result{OK: true, RunID: "run-123"})

	done := make(chan struct{})
	go func() {
		pm.actionWaitGroup("commander").Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed-out callback retained the plugin disable barrier")
	}
}
