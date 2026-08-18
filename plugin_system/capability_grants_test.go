package plugin_system

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// stateForPlugin returns the LState of an enabled plugin, for probing which
// parts of the mah table were installed.
func stateForPlugin(t *testing.T, pm *PluginManager, name string) *lua.LState {
	t.Helper()
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for i, p := range pm.plugins {
		if p.Name == name {
			return pm.states[i]
		}
	}
	t.Fatalf("plugin %q is not enabled", name)
	return nil
}

// mahHas reports whether a dotted mah path is installed in the plugin's VM.
func mahHas(t *testing.T, pm *PluginManager, plugin, expr string) bool {
	t.Helper()
	L := stateForPlugin(t, pm, plugin)
	if err := L.DoString("__probe = (" + expr + ") ~= nil"); err != nil {
		// Indexing through an absent submodule is itself an answer: absent.
		return false
	}
	return lua.LVAsBool(L.GetGlobal("__probe"))
}

func mustEnable(t *testing.T, dir, name, code string) *PluginManager {
	t.Helper()
	writePlugin(t, dir, name, code)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin(name); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

func TestDeclaredCapabilitiesInstallOnlyWhatWasDeclared(t *testing.T) {
	pm := mustEnable(t, t.TempDir(), "narrow", `
plugin = { name = "narrow", version = "1.0", api_version = 1,
           capabilities = { "db:read", "render" } }
function init() end
`)

	granted := []string{
		"mah.db", "mah.db.query_resources", "mah.db.get_resource", "mah.db.count_notes",
		"mah.db.mrql_query", "mah.db.get_resource_data", "mah.db.list_tags",
		"mah.shortcode", "mah.block_type", "mah.display_type",
		// Always installed: none of these reads or writes anything outside the
		// plugin itself.
		"mah.json", "mah.json.encode", "mah.util", "mah.util.now", "mah.log",
		"mah.html_escape", "mah.sleep", "mah.abort", "mah.doc", "mah.get_setting",
	}
	for _, path := range granted {
		if !mahHas(t, pm, "narrow", path) {
			t.Errorf("%s missing, but its capability was declared", path)
		}
	}

	withheld := []string{
		"mah.db.create_note", "mah.db.update_resource", "mah.db.delete_group",
		"mah.db.add_tags", "mah.db.create_resource_from_url",
		"mah.kv", "mah.http", "mah.image",
		"mah.on", "mah.inject", "mah.page", "mah.menu", "mah.api",
		"mah.action", "mah.start_job", "mah.job_progress",
	}
	for _, path := range withheld {
		if mahHas(t, pm, "narrow", path) {
			t.Errorf("%s installed, but its capability was not declared", path)
		}
	}
}

func TestDbWriteImpliesDbRead(t *testing.T) {
	// The writers return the entity they wrote, and patch_note(id, {}) changes
	// nothing while handing back the whole note — so a write-only grant was
	// already a read of anything by id. The implication is explicit rather than
	// hidden, so the label an operator consents to is what they get.
	pm := mustEnable(t, t.TempDir(), "writer", `
plugin = { name = "writer", version = "1.0", api_version = 1, capabilities = { "db:write" } }
function init() end
`)
	if !mahHas(t, pm, "writer", "mah.db.create_note") {
		t.Error("declared db:write did not install the writers")
	}
	if !mahHas(t, pm, "writer", "mah.db.query_resources") {
		t.Error("db:write must install the readers: patch already returns whole entities")
	}
}

func TestDbReadNeverImpliesDbWrite(t *testing.T) {
	// The direction that matters. A read-only plugin must reach no writer.
	pm := mustEnable(t, t.TempDir(), "reader", `
plugin = { name = "reader", version = "1.0", api_version = 1, capabilities = { "db:read" } }
function init() end
`)
	for _, path := range []string{
		"mah.db.create_note", "mah.db.update_note", "mah.db.patch_note", "mah.db.delete_note",
		"mah.db.add_tags", "mah.db.remove_tags", "mah.db.add_groups", "mah.db.add_resources_to_note",
		// The three that are declared on EntityQuerier but are writes. They have
		// no other behavioural assertion anywhere, and an arch rule alone left
		// add_resource_version_from_url — fetch a URL and make it the current
		// content of any resource — one word away from the read side.
		"mah.db.create_resource_from_url", "mah.db.create_resource_from_data",
		"mah.db.add_resource_version_from_url",
	} {
		if mahHas(t, pm, "reader", path) {
			t.Errorf("%s installed for a db:read-only plugin", path)
		}
	}
}

func TestTopLevelCodeNeverSeesMah(t *testing.T) {
	// mah is installed after the top-level code runs, which is what makes the
	// manifest read and the real run identical environments — and what stops
	// top-level code committing side effects under grants that the manifest
	// check is about to reject. A refusal can roll back registrations; it
	// cannot roll back a created tag.
	pm := mustEnable(t, t.TempDir(), "peeker", `
plugin = { name = "peeker", version = "1.0", api_version = 1, capabilities = { "db:write", "kv" } }
saw_mah_at_top_level = mah ~= nil
function init()
    saw_mah_in_init = mah ~= nil
end
`)
	L := stateForPlugin(t, pm, "peeker")
	if lua.LVAsBool(L.GetGlobal("saw_mah_at_top_level")) {
		t.Error("top-level code could reach mah, so it can act before its manifest has been checked")
	}
	if !lua.LVAsBool(L.GetGlobal("saw_mah_in_init")) {
		t.Error("init() could not reach mah")
	}
}

func TestTheManifestCannotDependOnMah(t *testing.T) {
	// The attack this closes: declare a narrow manifest to anyone reading the
	// source, while the read that decides the grants takes the other branch.
	// With mah absent in both runs the branch resolves the same way in each, so
	// the manifest is a function of the file alone.
	dir := t.TempDir()
	writePlugin(t, dir, "split", `
if mah then
    plugin = { name = "split", version = "1.0", api_version = 1, capabilities = {} }
else
    plugin = { name = "split", version = "1.0", api_version = 1, capabilities = { "kv" } }
end
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("split"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	// The mah-less branch is the one that ran, in both executions.
	if !mahHas(t, pm, "split", "mah.kv") {
		t.Error("the grants do not match the branch the manifest read took")
	}
}

func TestAManifestThatVariesBetweenRunsIsRefused(t *testing.T) {
	// Both runs see the same bytes in the same environment, so a disagreement
	// means the declaration is not a function of the file. Nothing may be
	// granted on that basis.
	dir := t.TempDir()
	writePlugin(t, dir, "flaky", `
plugin = { name = "flaky", version = "1.0", api_version = 1 }
seen = seen or 0
if tostring({}):len() > 0 then
    -- A table address differs between runs; use it to vary the declaration.
    if tonumber(tostring({}):sub(-1), 16) then
        plugin.capabilities = { "kv" }
    else
        plugin.capabilities = { "db:read" }
    end
end
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// Either it declares the same thing twice and loads, or it varies and is
	// refused — but it must never load with a manifest it did not declare.
	if err := pm.EnablePlugin("flaky"); err != nil {
		if !strings.Contains(err.Error(), "each time it runs") {
			t.Fatalf("unexpected refusal: %v", err)
		}
		return
	}
	if mahHas(t, pm, "flaky", "mah.kv") == mahHas(t, pm, "flaky", "mah.db.query_resources") {
		t.Error("the loaded plugin has neither or both of the two declarations it could have made")
	}
}

func TestTopLevelMahUseIsRefusedWithAClearMessage(t *testing.T) {
	// Registration belongs in init(). The manifest read executes top-level code
	// without mah, so a top-level mah call fails there — which is the enforcement,
	// and the message has to say so rather than leaving "attempt to index a nil
	// value" as the whole explanation.
	dir := t.TempDir()
	writePlugin(t, dir, "eager", `
plugin = { name = "eager", version = "1.0", api_version = 1, capabilities = { "pages" } }
mah.page("home", function(ctx) return "ok" end)
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// Discovery skips it, so it is never offered for enabling.
	if dp := pm.GetDiscoveredPlugin("eager"); dp != nil {
		t.Fatal("a plugin that calls mah at the top level was discovered")
	}
}

func TestHeaderExecutionIsBounded(t *testing.T) {
	// The manifest read executes untrusted top-level code at boot, for every
	// plugin directory. Unbounded, one plugin.lua with a runaway loop holds the
	// whole server's startup.
	dir := t.TempDir()
	writePlugin(t, dir, "spin", `
while true do end
plugin = { name = "spin", version = "1.0" }
`)
	writePlugin(t, dir, "fine", `
plugin = { name = "fine", version = "1.0" }
`)

	done := make(chan *PluginManager, 1)
	go func() {
		pm, err := NewPluginManager(dir)
		if err != nil {
			t.Error(err)
		}
		done <- pm
	}()

	select {
	case pm := <-done:
		defer pm.Close()
		if pm.GetDiscoveredPlugin("spin") != nil {
			t.Error("a plugin that never finishes its top-level code was discovered")
		}
		if pm.GetDiscoveredPlugin("fine") == nil {
			t.Error("the runaway plugin took its neighbour down with it")
		}
	case <-time.After(pluginHeaderTimeout + 15*time.Second):
		t.Fatal("NewPluginManager did not return: a runaway plugin.lua holds startup")
	}
}

func TestAJobStartedFromInitWaitsForTheLoad(t *testing.T) {
	// init() runs on the plugin's LState. mah.start_job hands the same state to
	// a worker goroutine, which takes the VM lock — free, because the load
	// never took it — and runs Lua concurrently with init(). Two goroutines
	// inside one gopher-lua state corrupt its stack.
	pm := mustEnable(t, t.TempDir(), "racy", `
plugin = { name = "racy", version = "1.0", api_version = 1, capabilities = { "jobs" } }
init_finished = false
saw = "job never ran"
function init()
    mah.start_job("work", function(id)
        if init_finished then saw = "after" else saw = "during" end
        mah.job_complete(id)
    end)
    -- Long enough that an unsynchronized worker would win the race.
    local x = 0
    for i = 1, 400000 do x = x + i end
    init_finished = true
end
`)

	deadline := time.Now().Add(20 * time.Second)
	for {
		jobs := pm.GetAllActionJobs()
		finished := len(jobs) > 0
		for i := range jobs {
			if s := jobs[i].Status; s != "completed" && s != "failed" {
				finished = false
			}
		}
		if finished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}

	L := stateForPlugin(t, pm, "racy")
	mu := pm.LockVM(L)
	if mu == nil {
		t.Fatal("plugin vanished")
	}
	saw := L.GetGlobal("saw").String()
	mu.Unlock()

	if saw != "after" {
		t.Errorf("the job callback ran %s the load: two goroutines were inside one LState", saw)
	}
}

func TestDisablingTwiceIsSafe(t *testing.T) {
	// Teardown has three entry points — DisablePlugin, a failed load and Close —
	// and two of them can be in flight at once. Closing an LState twice (or
	// closing one somebody else is executing) segfaults inside gopher-lua, so
	// the vmLocks entry is the ownership token: whoever removes it closes the
	// state, and everybody else backs off.
	//
	// This exercises the live disable path deliberately. It used to exercise a
	// helper that production no longer called, which is exactly how that path
	// drifted away from the protocol it was supposed to share.
	pm := mustEnable(t, t.TempDir(), "twice", `
plugin = { name = "twice", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init() mah.page("home", function(ctx) return "ok" end) end
`)

	if err := pm.DisablePlugin("twice"); err != nil {
		t.Fatal(err)
	}
	if err := pm.DisablePlugin("twice"); err == nil {
		t.Error("disabling twice reported success the second time")
	}
	assertNoOrphanRegistrations(t, pm)

	// And concurrently, which is the shape that actually races.
	pm2 := mustEnable(t, t.TempDir(), "twice2", `
plugin = { name = "twice2", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init() mah.page("home", function(ctx) return "ok" end) end
`)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pm2.DisablePlugin("twice2")
		}()
	}
	wg.Wait()
	assertNoOrphanRegistrations(t, pm2)
}

func TestTheHeaderVMIsSandboxedLikeTheRealOne(t *testing.T) {
	// Reading a plugin's header executes its top-level code — at boot, for
	// every plugin directory, enabled or not. gopher-lua's loadfile opens an
	// arbitrary path, so leaving it in place gave a *disabled* plugin an
	// arbitrary-path open on every start, and the parse error quotes the
	// offending source line. The load VM has always stripped these; the header
	// VM is the one that runs unasked.
	for _, name := range []string{"dofile", "loadfile", "load", "loadstring"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writePlugin(t, dir, "nosy", `
plugin = { name = "nosy", version = "1.0" }
probe = `+name+`
`)
			pm, err := NewPluginManager(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pm.Close)

			// Discovery has to keep working; what must be gone is the function.
			dp := pm.GetDiscoveredPlugin("nosy")
			if dp == nil {
				t.Fatal("plugin was not discovered")
			}
			if err := pm.EnablePlugin("nosy"); err != nil {
				t.Fatalf("EnablePlugin: %v", err)
			}
			L := stateForPlugin(t, pm, "nosy")
			if L.GetGlobal("probe") != lua.LNil {
				t.Errorf("%s is still reachable from plugin code", name)
			}
		})
	}
}

func TestActionsGrantsTheJobReportersButNotStartJob(t *testing.T) {
	// An async action's handler is handed a job_id and is expected to call
	// mah.job_progress/job_complete/job_fail on it — that is the documented
	// pattern. Requiring "jobs" for those would hand a plugin an id it cannot
	// use, with no error to explain why. Creating work of its own is a
	// different power, so start_job stays behind "jobs".
	pm := mustEnable(t, t.TempDir(), "acts", `
plugin = { name = "acts", version = "1.0", api_version = 1, capabilities = { "actions" } }
function init() end
`)
	for _, path := range []string{"mah.job_progress", "mah.job_complete", "mah.job_fail"} {
		if !mahHas(t, pm, "acts", path) {
			t.Errorf("%s missing: an async action cannot report on the job it was given", path)
		}
	}
	if mahHas(t, pm, "acts", "mah.start_job") {
		t.Error("mah.start_job installed without the jobs capability")
	}
}

func TestAPluginCannotTouchAnotherPluginsJob(t *testing.T) {
	// The reporters look a job up by id in one process-wide map. Nothing
	// checked that the job belonged to the caller, so one plugin could complete
	// or fail another's work.
	dir := t.TempDir()
	writePlugin(t, dir, "owner", `
plugin = { name = "owner", version = "1.0", api_version = 1, capabilities = { "jobs" } }
function init() end
`)
	writePlugin(t, dir, "thief", `
plugin = { name = "thief", version = "1.0", api_version = 1, capabilities = { "jobs" } }
function steal(id)
    mah.job_fail(id, "hijacked")
end
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	for _, name := range []string{"owner", "thief"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	job := &ActionJob{
		ID: "job-owned-by-owner", Source: "plugin", PluginName: "owner",
		ActionID: "start_job", Label: "work", EntityType: "custom",
		Status: "running", Message: "working",
	}
	pm.actionJobsMu.Lock()
	pm.actionJobs[job.ID] = job
	pm.actionJobsMu.Unlock()

	L := stateForPlugin(t, pm, "thief")
	err = L.CallByParam(lua.P{Fn: L.GetGlobal("steal"), NRet: 0, Protect: true}, lua.LString(job.ID))
	if err == nil {
		t.Fatal("a plugin failed another plugin's job")
	}

	job.mu.Lock()
	status, message := job.Status, job.Message
	job.mu.Unlock()
	if status != "running" || message != "working" {
		t.Errorf("the job was modified: status=%q message=%q", status, message)
	}
}

func TestLegacyPluginKeepsTheFullSurface(t *testing.T) {
	pm := mustEnable(t, t.TempDir(), "legacy", `
plugin = { name = "legacy", version = "1.0" }
function init() end
`)
	for _, path := range []string{
		"mah.db.query_resources", "mah.db.create_note", "mah.kv", "mah.http",
		"mah.image", "mah.on", "mah.inject", "mah.page", "mah.api", "mah.action",
		"mah.start_job", "mah.shortcode",
	} {
		if !mahHas(t, pm, "legacy", path) {
			t.Errorf("%s missing: a plugin without a manifest keeps the full surface", path)
		}
	}
}

func TestUngrantedModuleIsAbsentNotStubbed(t *testing.T) {
	// Absence has to be feature-detectable, or a plugin cannot degrade.
	pm := mustEnable(t, t.TempDir(), "detect", `
plugin = { name = "detect", version = "1.0", api_version = 1, capabilities = { "db:read" } }
has_kv = nil
function init()
    if mah.kv then has_kv = true else has_kv = false end
end
`)
	L := stateForPlugin(t, pm, "detect")
	if lua.LVAsBool(L.GetGlobal("has_kv")) {
		t.Error("mah.kv reported present")
	}
	if L.GetGlobal("has_kv") == lua.LNil {
		t.Error("init() did not run: feature detection raised instead of reading nil")
	}
}

func TestCallingAnUngrantedFunctionFailsTheLoad(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "greedy", `
plugin = { name = "greedy", version = "1.0", api_version = 1, capabilities = { "db:read" } }
function init()
    mah.kv.set("k", "v")
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("greedy"); err == nil {
		t.Fatal("expected the load to fail: the plugin used a capability it did not declare")
	}
	if len(pm.Plugins()) != 0 {
		t.Error("a plugin whose init() failed must not be listed as enabled")
	}
}

func TestApiVersionTooNewIsRefused(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "future", `
plugin = { name = "future", version = "1.0", api_version = 99 }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("future")
	if err == nil {
		t.Fatal("expected a plugin declaring a newer api_version to be refused")
	}
	if !strings.Contains(err.Error(), "api_version") {
		t.Errorf("the refusal must say what is wrong, got %v", err)
	}
}

func TestWithheldModulesAreLogged(t *testing.T) {
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	mustEnable(t, t.TempDir(), "quiet", `
plugin = { name = "quiet", version = "1.0", api_version = 1, capabilities = { "db:read" } }
function init() end
`)

	out := buf.String()
	// A nil index deep inside a plugin is undiagnosable without this.
	for _, want := range []string{"quiet", "mah.kv", "kv"} {
		if !strings.Contains(out, want) {
			t.Errorf("load log does not mention %q; got:\n%s", want, out)
		}
	}
}

func TestAChangedManifestIsRefusedUntilRestart(t *testing.T) {
	// The discovered list is what the manage UI renders and what an operator is
	// looking at when they press enable. Loading a different capability list
	// than the one on screen would make that display wrong for the rest of the
	// process's life — the same argument that already refuses a renamed file.
	dir := t.TempDir()
	writePlugin(t, dir, "swapped", `
plugin = { name = "swapped", version = "1.0", api_version = 1, capabilities = { "db:read" } }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	writePlugin(t, dir, "swapped", `
plugin = { name = "swapped", version = "1.0", api_version = 1, capabilities = { "kv" } }
function init() end
`)

	err = pm.EnablePlugin("swapped")
	if err == nil {
		t.Fatal("a file whose capabilities changed since startup was loaded")
	}
	if !strings.Contains(err.Error(), "restart") {
		t.Errorf("the refusal must say what to do about it, got %v", err)
	}
	if len(pm.Plugins()) != 0 {
		t.Error("the plugin is listed as enabled")
	}
}

func TestAnEditThatDoesNotTouchTheManifestStillLoads(t *testing.T) {
	// Only the manifest and the identity are pinned to what discovery saw. The
	// rest of the file is read fresh, because the grants have to describe the
	// code that actually runs.
	dir := t.TempDir()
	writePlugin(t, dir, "edited", `
plugin = { name = "edited", version = "1.0", api_version = 1, capabilities = { "kv" } }
function init() marker = "first" end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	writePlugin(t, dir, "edited", `
plugin = { name = "edited", version = "1.0", api_version = 1, capabilities = { "kv" } }
function init() marker = "second" end
`)

	if err := pm.EnablePlugin("edited"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	L := stateForPlugin(t, pm, "edited")
	if got := L.GetGlobal("marker").String(); got != "second" {
		t.Errorf("marker = %q: the load ran a stale copy of the file", got)
	}
}

func TestTopLevelCodeRunsUnderTheLiveManifestsGrants(t *testing.T) {
	// The sharp version of the same rule: top-level code runs *before* any
	// post-hoc comparison could notice the file changed, so the grants have to
	// be right before the first line executes.
	dir := t.TempDir()
	writePlugin(t, dir, "toplevel", `
plugin = { name = "toplevel", version = "1.0", api_version = 1, capabilities = { "kv" } }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// The file now declares nothing and uses mah.kv at top level.
	writePlugin(t, dir, "toplevel", `
plugin = { name = "toplevel", version = "1.0", api_version = 1, capabilities = {} }
mah.kv.set("k", "v")
function init() end
`)

	if err := pm.EnablePlugin("toplevel"); err == nil {
		t.Fatal("expected the load to fail: top-level code used a capability the file does not declare")
	}
}

func TestLoadRefusesARenamedPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "renamed", `
plugin = { name = "renamed", version = "1.0", api_version = 1, capabilities = {} }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	writePlugin(t, dir, "renamed", `
plugin = { name = "something-else", version = "1.0", api_version = 1, capabilities = {} }
function init() end
`)

	err = pm.EnablePlugin("renamed")
	if err == nil {
		t.Fatal("expected a refusal: the operator enabled a name, not a file")
	}
	if !strings.Contains(err.Error(), "something-else") {
		t.Errorf("the refusal must name what it found, got %v", err)
	}
}

func TestAFailedLoadLeavesNothingCallable(t *testing.T) {
	// A plugin that registers and then fails init() used to keep its
	// registrations while its VM was closed underneath them. The next request
	// to that endpoint entered a closed LState — a segfault inside gopher-lua,
	// triggered by a plugin that merely failed to load. Under-declaring a
	// capability is now the most likely way to fail init(), so this path went
	// from obscure to routine.
	dir := t.TempDir()
	writePlugin(t, dir, "halfdead", `
plugin = { name = "halfdead", version = "1.0", api_version = 1, capabilities = { "api", "render", "pages" } }
function init()
    mah.api("GET", "boom", function(ctx) ctx.json({ ok = true }) end)
    mah.page("page", function(ctx) return "<p>hi</p>" end)
    mah.shortcode({ name = "sc", label = "SC", render = function() return "x" end })
    error("init blew up")
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("halfdead"); err == nil {
		t.Fatal("expected EnablePlugin to fail")
	}

	// Each of these would have entered the closed state.
	resp := pm.HandleAPI(context.Background(), "halfdead", "GET", "boom", PageContext{
		Path: "/v1/plugins/halfdead/boom", Method: "GET",
	})
	if resp.StatusCode == 200 {
		t.Error("a registration from a failed load is still serving requests")
	}
	if len(pm.GetPages()) != 0 {
		t.Errorf("pages survived a failed load: %v", pm.GetPages())
	}
	if pm.GetPluginShortcode("plugin:halfdead:sc") != nil {
		t.Error("a shortcode survived a failed load")
	}
	if len(pm.Plugins()) != 0 {
		t.Error("a plugin whose init() failed is listed as enabled")
	}
}

func TestPluginNameMustBeUsable(t *testing.T) {
	// The name is a map key, a URL segment in every menu href, and the prefix of
	// every shortcode the plugin registers. "My Plugin" produced shortcodes
	// nobody could write and a space inside a URL.
	for _, name := range []string{"My Plugin", "UPPER", "has space", "sla/sh", "1leading"} {
		dir := t.TempDir()
		writePlugin(t, dir, "p", `plugin = { name = "`+name+`", version = "1.0" }`)
		pm, err := NewPluginManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(pm.DiscoveredPlugins()) != 0 {
			t.Errorf("plugin name %q was accepted", name)
		}
		pm.Close()
	}
}

func TestTheHeaderAndLoadVMsOpenTheSameLibraries(t *testing.T) {
	// Any difference between the two environments is a discriminator: a plugin
	// could declare one manifest when a library is present and another when it
	// is not, exactly as it once could with mah.
	dir := t.TempDir()
	writePlugin(t, dir, "sniff", `
plugin = { name = "sniff", version = (coroutine and "load-vm" or "header-vm"), api_version = 1, capabilities = {} }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	dp := pm.GetDiscoveredPlugin("sniff")
	if dp == nil {
		t.Fatal("not discovered")
	}
	if dp.Version != "load-vm" {
		t.Errorf("the manifest read saw a different environment than the load: version = %q", dp.Version)
	}
}

func TestAnImplausiblyLargePluginIsRefused(t *testing.T) {
	// The file is read whole, twice per load, at boot, for every directory
	// including ones nobody enabled.
	dir := t.TempDir()
	big := make([]byte, maxPluginSourceSize+1)
	for i := range big {
		big[i] = '-'
	}
	writePlugin(t, dir, "huge", "--[[\n"+string(big)+"\n]]\nplugin = { name = \"huge\", version = \"1.0\" }")

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if pm.GetDiscoveredPlugin("huge") != nil {
		t.Error("an oversized plugin.lua was read and discovered")
	}
}

func TestARevokedPluginStopsBeingDispatchableImmediately(t *testing.T) {
	// A job still running when the drain times out holds a fully-installed mah
	// table, so it can register a hook after the teardown swept — leaving a
	// revoked plugin serving hooks and injections while the UI reports it
	// disabled, until the process exits. Revocation has to take effect when the
	// operator asks, not when the plugin's work happens to finish.
	pm := mustEnable(t, t.TempDir(), "ghost", `
plugin = { name = "ghost", version = "1.0", api_version = 1, capabilities = { "jobs", "hooks" } }
function init()
    mah.start_job("linger", function(id)
        mah.sleep(7)
        mah.on("after_note_create", function(ctx) end)
        mah.job_complete(id)
    end)
end
`)
	L := stateForPlugin(t, pm, "ghost")

	start := time.Now()
	if err := pm.DisablePlugin("ghost"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}
	if elapsed := time.Since(start); elapsed > retireDrainTimeout+2*time.Second {
		t.Errorf("DisablePlugin waited %s for the job; the caller must not wait out its allowance", elapsed)
	}

	// The state may still be executing, but nothing may dispatch into it.
	if mu := pm.LockVM(L); mu != nil {
		mu.Unlock()
		t.Error("a revoked plugin's VM is still dispatchable")
	}

	// And once the worker stops, its late registration is swept too.
	deadline := time.Now().Add(20 * time.Second)
	for {
		pm.mu.RLock()
		hooks := len(pm.hooks["after_note_create"])
		pm.mu.RUnlock()
		if hooks == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a hook registered after revocation was never swept")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestATeardownDoesNotRevokeANamesakeThatReplacedIt(t *testing.T) {
	// The sweep runs by name, arbitrarily later. If the operator re-enabled the
	// plugin in the meantime, the dead load's sweep would strip the live one's
	// pages and actions.
	dir := t.TempDir()
	writePlugin(t, dir, "twin", `
plugin = { name = "twin", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init() mah.page("home", function(ctx) return "ok" end) end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("twin"); err != nil {
		t.Fatal(err)
	}
	dead := stateForPlugin(t, pm, "twin")

	if err := pm.DisablePlugin("twin"); err != nil {
		t.Fatal(err)
	}
	if err := pm.EnablePlugin("twin"); err != nil {
		t.Fatal(err)
	}

	// The dead load's sweep arrives late.
	pm.mu.Lock()
	pm.unregisterPluginLocked("twin", dead)
	pm.mu.Unlock()

	if len(pm.GetPages()) == 0 {
		t.Error("the live plugin's page was removed by the dead load's teardown")
	}
}

func TestAGlobalsMetatableCannotReachOutsideAProtectedCall(t *testing.T) {
	// Reading `plugin`, `settings` and `init` all go through _G from Go,
	// outside any PCall, and _G honours metamethods — on a *miss*, which is the
	// case that matters: `init` is optional and `settings` usually absent, so a
	// plugin does not even have to try to trigger it.
	//
	// The manifest read is the worst of the two: it runs at boot for every
	// plugin directory, enabled or not, so this is a server that cannot start
	// because of a file nobody turned on — and disabling it is no remedy,
	// because discovery does not consult enablement.
	t.Run("no plugin table: discovery must fail, not panic", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "meta", `setmetatable(_G, { __index = function(t, k) error("boom") end })`)

		pm, err := NewPluginManager(dir) // must not panic
		if err != nil {
			t.Fatal(err)
		}
		defer pm.Close()
		if pm.GetDiscoveredPlugin("meta") != nil {
			t.Error("a plugin with no plugin table was discovered")
		}
	})

	t.Run("no init: the load must not panic", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, "meta2", `
setmetatable(_G, { __index = function(t, k) error("boom") end })
plugin = { name = "meta2", version = "1.0", api_version = 1, capabilities = {} }
`)
		pm, err := NewPluginManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer pm.Close()
		if pm.GetDiscoveredPlugin("meta2") == nil {
			t.Fatal("not discovered")
		}
		if err := pm.EnablePlugin("meta2"); err != nil {
			t.Fatalf("EnablePlugin: %v", err)
		}
		L := stateForPlugin(t, pm, "meta2")
		if L.G.Global.Metatable != lua.LNil {
			t.Error("the globals metatable survived into the loaded VM")
		}
	})
}

func TestATimedOutLoadCannotWedgeTheManagerAfterClose(t *testing.T) {
	// init() is unbounded and Close's wait for it is not, so a load can still be
	// running when Close finishes. Every registration function writes its map
	// while holding pm.mu; if Close had niled those maps, the write would panic
	// between Lock and Unlock — caught by the protected Lua call around it, but
	// leaving pm.mu held for the life of the process, so the next teardown
	// blocks forever.
	dir := t.TempDir()
	writePlugin(t, dir, "late", `
plugin = { name = "late", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init()
    mah.sleep(7)
    mah.page("home", function(ctx) return "ok" end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	enabled := make(chan error, 1)
	go func() { enabled <- pm.EnablePlugin("late") }()
	time.Sleep(300 * time.Millisecond) // let init() get inside its sleep

	pm.Close()

	select {
	case <-enabled:
	case <-time.After(30 * time.Second):
		t.Fatal("the load never returned: the manager is wedged")
	}

	// And the manager's lock is still usable afterwards.
	done := make(chan struct{})
	go func() { pm.Plugins(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pm.mu is held forever")
	}
}

func TestTwoDirectoriesClaimingOneNameAreBothDropped(t *testing.T) {
	// Electing a winner is an identity takeover: the winner inherits the other's
	// persisted enabled flag, its stored settings — API keys included, and
	// mah.get_setting needs no capability — and its KV namespace. A directory
	// sorting earlier must not be able to take a name that way.
	dir := t.TempDir()
	writePlugin(t, dir, "a-attacker", `plugin = { name = "victim", version = "9.9", api_version = 1, capabilities = { "db:write" } }`)
	writePlugin(t, dir, "z-victim", `plugin = { name = "victim", version = "1.0", api_version = 1, capabilities = {} }`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if dp := pm.GetDiscoveredPlugin("victim"); dp != nil {
		t.Errorf("a contested name was awarded to %q (version %q)", dp.Dir, dp.Version)
	}
	if len(pm.DiscoveredPlugins()) != 0 {
		t.Errorf("expected both claimants dropped, got %d", len(pm.DiscoveredPlugins()))
	}
}

func TestADeadGenerationCannotClobberItsReplacement(t *testing.T) {
	// Parameterised over a worker that finishes *inside* the drain and one that
	// outlasts it. The earlier version only tried the second, which is the one
	// branch that dropped the liveness entry early — so it pinned the asymmetry
	// rather than the rule. Revocation now happens at the start of teardown, so
	// both must behave identically.
	for _, sleep := range []int{1, 7} {
		t.Run(fmt.Sprintf("worker sleeps %ds (drain is %s)", sleep, retireDrainTimeout), func(t *testing.T) {
			deadGenerationCannotClobber(t, sleep)
		})
	}
}

func deadGenerationCannotClobber(t *testing.T, sleepSeconds int) {
	// A revoked plugin's worker keeps running with a fully-installed mah table.
	// Registering by name alone, it could overwrite the *replacement*
	// generation's page — same plugin name, same path — leaving the enabled
	// plugin's page pointing at a closed VM. Neither deleting by name nor
	// skipping the delete fixes that; the write itself has to be refused.
	dir := t.TempDir()
	const manifest = `plugin = { name = "gen", version = "1.0", api_version = 1, capabilities = { "jobs", "pages" } }`

	// Generation one starts a job that outlives the drain and then registers
	// over the page. The manifest is identical in both files, so the load is
	// not refused for having changed.
	writePlugin(t, dir, "gen", manifest+`
function init()
    mah.page("home", function(ctx) return "first" end)
    mah.start_job("linger", function(id)
        mah.sleep(`+fmt.Sprint(sleepSeconds)+`)
        mah.page("home", function(ctx) return "from the dead generation" end)
        mah.job_complete(id)
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("gen"); err != nil {
		t.Fatal(err)
	}
	if err := pm.DisablePlugin("gen"); err != nil { // drains 5s, then revokes
		t.Fatal(err)
	}

	// Generation two: no job, same manifest.
	writePlugin(t, dir, "gen", manifest+`
function init() mah.page("home", function(ctx) return "second" end) end
`)
	if err := pm.EnablePlugin("gen"); err != nil {
		t.Fatal(err)
	}
	live := stateForPlugin(t, pm, "gen")

	// Let the dead generation's worker reach its late mah.page call and its
	// teardown finish.
	time.Sleep(time.Duration(sleepSeconds+3) * time.Second)

	pm.mu.RLock()
	entry, ok := pm.pages["gen"]["home"]
	pm.mu.RUnlock()
	if !ok {
		t.Fatal("the live generation's page is gone: the dead one overwrote it and its sweep then removed it")
	}
	if entry.state != live {
		t.Error("the live generation's page was overwritten by the dead one and now points at a closed VM")
	}
}

func TestALateSweepLeavesALoadingReplacementAlone(t *testing.T) {
	// The other ordering: the replacement is inside init(), has registered, and
	// is not yet published — so a rule that asked "does another *published*
	// plugin hold this name?" would answer no and delete the new registrations.
	dir := t.TempDir()
	writePlugin(t, dir, "concurrent", `
plugin = { name = "concurrent", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init() mah.page("home", function(ctx) return "ok" end) end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("concurrent"); err != nil {
		t.Fatal(err)
	}
	dead := stateForPlugin(t, pm, "concurrent")
	if err := pm.DisablePlugin("concurrent"); err != nil {
		t.Fatal(err)
	}
	if err := pm.EnablePlugin("concurrent"); err != nil {
		t.Fatal(err)
	}

	// A sweep for the dead state, arriving at any time, must not touch the
	// live one's registrations.
	pm.mu.Lock()
	pm.unregisterPluginLocked("concurrent", dead)
	pm.mu.Unlock()

	if len(pm.GetPages()) != 1 {
		t.Error("the replacement's page was removed by the dead generation's sweep")
	}
}

// assertNoOrphanRegistrations checks the invariant the teardown state machine
// exists to maintain: no registration may name a state that is not live.
//
// Every worst finding in rounds 4, 5 and 6 was a violation of exactly this, and
// none of the arch guards could have caught any of them — they all pin the
// *installation* side. This is the teardown side, checkable at any quiescent
// point.
func assertNoOrphanRegistrations(t *testing.T, pm *PluginManager) {
	t.Helper()
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	live := func(L *lua.LState) bool {
		if L == nil {
			return false
		}
		_, ok := pm.vmLocks[L]
		return ok
	}
	bad := func(what string, L *lua.LState) {
		if !live(L) {
			t.Errorf("%s names a state that is no longer live: a dispatch to it enters a closed VM, "+
				"or shadows a live generation's registration of the same key", what)
		}
	}

	for event, entries := range pm.hooks {
		for _, e := range entries {
			bad("hook "+event, e.state)
		}
	}
	for slot, entries := range pm.injections {
		for _, e := range entries {
			bad("injection "+slot, e.state)
		}
	}
	for name, paths := range pm.pages {
		for path, entry := range paths {
			bad("page "+name+"/"+path, entry.state)
		}
	}
	for _, m := range pm.menuItems {
		bad("menu item "+m.FullPath, m.state)
	}
	for name, actions := range pm.actions {
		for _, a := range actions {
			bad("action "+name+"/"+a.ID, a.state)
		}
	}
	for name, types := range pm.blockTypes {
		for _, bt := range types {
			bad("block type "+name+"/"+bt.TypeName, bt.State)
		}
	}
	for name, types := range pm.displayTypes {
		for _, dt := range types {
			bad("display type "+name+"/"+dt.TypeName, dt.State)
		}
	}
	for name, codes := range pm.shortcodes {
		for _, sc := range codes {
			bad("shortcode "+name+"/"+sc.TypeName, sc.State)
		}
	}
	for name, docs := range pm.docs {
		for _, d := range docs {
			bad("doc "+name+"/"+d.Name, d.State)
		}
	}
	for name, endpoints := range pm.apiEndpoints {
		for key, ep := range endpoints {
			bad("endpoint "+name+"/"+key, ep.state)
		}
	}
}

func TestNoRegistrationOutlivesItsVM(t *testing.T) {
	// The invariant, across the interleaving that produced the worst finding in
	// three consecutive review rounds: a job outlasting the drain, a disable, a
	// prompt re-enable, and a late sweep landing after the replacement loaded.
	dir := t.TempDir()
	const manifest = `plugin = { name = "churn", version = "1.0", api_version = 1,
           capabilities = { "jobs", "pages", "render", "actions", "api", "hooks", "inject" } }`
	const registrations = `
    mah.page("p1", function(ctx) return "ok" end)
    mah.api("GET", "e1", function(ctx) ctx.json({}) end)
    mah.shortcode({ name = "sc", label = "SC", render = function() return "x" end })
    mah.block_type({ type = "card", label = "Card",
        render_view = function(ctx) return "" end,
        render_edit = function(ctx) return "" end })
    mah.on("after_note_create", function(ctx) end)
    mah.inject("head", function(ctx) return "" end)
    mah.action({ id = "a1", label = "A", entity = "resource", handler = function() return {} end })
    mah.doc({ name = "d1", label = "D" })
`

	// Generation one keeps a worker alive past the drain, and that worker tries
	// to register again after its plugin was revoked.
	writePlugin(t, dir, "churn", manifest+`
function init()`+registrations+`
    mah.start_job("linger", function(id)
        mah.sleep(7)
        mah.page("p1", function(ctx) return "from the dead generation" end)
        mah.job_complete(id)
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("churn"); err != nil {
		t.Fatal(err)
	}
	assertNoOrphanRegistrations(t, pm)

	if err := pm.DisablePlugin("churn"); err != nil { // drains, times out, revokes
		t.Fatal(err)
	}
	assertNoOrphanRegistrations(t, pm)

	// Generation two: same manifest, no job.
	writePlugin(t, dir, "churn", manifest+`
function init()`+registrations+`
end
`)
	if err := pm.EnablePlugin("churn"); err != nil {
		t.Fatal(err)
	}
	live := stateForPlugin(t, pm, "churn")
	assertNoOrphanRegistrations(t, pm)

	// Let the dead generation's worker reach its late registration and its
	// teardown finish.
	time.Sleep(5 * time.Second)
	assertNoOrphanRegistrations(t, pm)

	// The live generation is intact, and its page is still its own.
	if !pm.IsEnabled("churn") {
		t.Fatal("the plugin is not enabled")
	}
	pm.mu.RLock()
	entry, ok := pm.pages["churn"]["p1"]
	pm.mu.RUnlock()
	if !ok {
		t.Error("the live generation's page is gone")
	} else if entry.state != live {
		t.Error("the live generation's page points at the dead generation's VM")
	}
	if pm.GetPluginShortcode("plugin:churn:sc") == nil {
		t.Error("the live generation's shortcode was swept")
	}
}

func TestARegistrationFromACoroutineIsUsable(t *testing.T) {
	// A registration made inside a coroutine used to be stamped with the
	// coroutine's own LState. Dispatch and teardown are both keyed on the main
	// state, so such an entry could never fire and could never be removed —
	// it sat in the registry forever, and survived DisablePlugin.
	dir := t.TempDir()
	writePlugin(t, dir, "coro", `
plugin = { name = "coro", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init()
    local co = coroutine.create(function()
        mah.page("from-coroutine", function(ctx) return "ok" end)
    end)
    coroutine.resume(co)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("coro"); err != nil {
		t.Fatal(err)
	}
	main := stateForPlugin(t, pm, "coro")

	pm.mu.RLock()
	entry, ok := pm.pages["coro"]["from-coroutine"]
	pm.mu.RUnlock()
	if !ok {
		t.Fatal("the coroutine's page was not registered")
	}
	if entry.state != main {
		t.Error("the page is stamped with the coroutine's state, so nothing can dispatch to it or remove it")
	}

	// And it goes away with the plugin.
	if err := pm.DisablePlugin("coro"); err != nil {
		t.Fatal(err)
	}
	if len(pm.GetPages()) != 0 {
		t.Errorf("pages survived the disable: %v", pm.GetPages())
	}
	assertNoOrphanRegistrations(t, pm)
}

func TestAnActionRunsOnItsOwnVM(t *testing.T) {
	// FindAction paired an action with "whichever state currently holds this
	// plugin name". A registration that outlived its generation would then run
	// against the replacement's VM — executing code the replacement does not
	// contain, and calling a handler compiled in one LState on another, which
	// is not a thing gopher-lua permits.
	dir := t.TempDir()
	writePlugin(t, dir, "act", `
plugin = { name = "act", version = "1.0", api_version = 1, capabilities = { "actions" } }
function init()
    mah.action({ id = "a1", label = "A", entity = "resource",
                 handler = function(ctx) return { success = true, message = "first" } end })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("act"); err != nil {
		t.Fatal(err)
	}
	first := stateForPlugin(t, pm, "act")

	action, state, err := pm.FindAction("act", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if state != first {
		t.Error("the action was paired with a state that is not the one that registered it")
	}
	if action.state != first {
		t.Error("the action is not stamped with the state that registered it")
	}
}

func TestAReplacementEnabledDuringTheDrainKeepsItsRegistrations(t *testing.T) {
	// The sharper interleaving: the operator does not wait for the disable to
	// return. DisablePlugin removes the plugin from pm.plugins and then drains,
	// so an enable arriving 150ms later succeeds and registers a *second*
	// generation while the first is still winding down — and the first's worker
	// is still holding a fully-installed mah table.
	dir := t.TempDir()
	const manifest = `plugin = { name = "beta", version = "1.0", api_version = 1, capabilities = { "pages", "jobs" } }`

	writePlugin(t, dir, "beta", manifest+`
function init()
    mah.page("home", function(ctx) return "first" end)
    mah.start_job("linger", function(id)
        mah.sleep(2)
        mah.page("home", function(ctx) return "from the dead generation" end)
        mah.job_complete(id)
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("beta"); err != nil {
		t.Fatal(err)
	}

	writePlugin(t, dir, "beta", manifest+`
function init() mah.page("home", function(ctx) return "second" end) end
`)

	disabled := make(chan error, 1)
	go func() { disabled <- pm.DisablePlugin("beta") }()
	time.Sleep(150 * time.Millisecond)

	if err := pm.EnablePlugin("beta"); err != nil {
		t.Fatalf("EnablePlugin during the drain: %v", err)
	}
	live := stateForPlugin(t, pm, "beta")
	if err := <-disabled; err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	// Past the dead worker's late registration and the teardown behind it.
	time.Sleep(6 * time.Second)

	if !pm.IsEnabled("beta") {
		t.Fatal("the replacement is not enabled")
	}
	pm.mu.RLock()
	entry, ok := pm.pages["beta"]["home"]
	pm.mu.RUnlock()
	if !ok {
		t.Error("the replacement's page is gone: the dead generation overwrote it during the drain " +
			"and its sweep then removed it")
	} else if entry.state != live {
		t.Error("the replacement's page points at the dead generation's VM")
	}
	assertNoOrphanRegistrations(t, pm)
}

func TestAFailedLoadIsRevokedBeforeItsLockIsReleased(t *testing.T) {
	// A request can already be queued on the loading plugin's VM lock. If the
	// failure path releases that lock before revoking, the queued request
	// acquires it, finds the plugin still live, and serves a page or endpoint
	// belonging to a load that failed.
	dir := t.TempDir()
	writePlugin(t, dir, "halfdead", `
plugin = { name = "halfdead", version = "1.0", api_version = 1, capabilities = { "api", "pages" } }
function init()
    mah.api("GET", "boom", function(ctx) ctx.json({ ok = true }) end)
    mah.page("home", function(ctx) return "served" end)
    error("init blew up")
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	// Hammer the endpoint while the load runs, so a request is queued on the
	// VM lock at the moment init() fails.
	stop := make(chan struct{})
	served := make(chan int, 64)
	for i := 0; i < 8; i++ {
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp := pm.HandleAPI(context.Background(), "halfdead", "GET", "boom", PageContext{
					Path: "/v1/plugins/halfdead/boom", Method: "GET",
				})
				if resp.StatusCode == 200 {
					select {
					case served <- resp.StatusCode:
					default:
					}
				}
			}
		}()
	}

	if err := pm.EnablePlugin("halfdead"); err == nil {
		close(stop)
		t.Fatal("expected the load to fail")
	}
	time.Sleep(200 * time.Millisecond)
	close(stop)

	select {
	case <-served:
		t.Error("an endpoint of a plugin whose init() failed served a request")
	default:
	}
	assertNoOrphanRegistrations(t, pm)
}

func TestTheWithheldLogDoesNotNameInstalledFunctions(t *testing.T) {
	// The job_* reporters come with either actions or jobs. Naming them on the
	// withheld line for one while the other is granted tells an operator a
	// function is absent when it is installed — and the load log is the only
	// place an ungranted key is ever explained.
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	pm := mustEnable(t, t.TempDir(), "acts", `
plugin = { name = "acts", version = "1.0", api_version = 1, capabilities = { "actions" } }
function init() end
`)
	out := buf.String()
	if strings.Contains(out, "job_* reporters") {
		t.Errorf("the withheld line claims the job_* reporters are absent, but actions installs them:\n%s", out)
	}
	if !strings.Contains(out, "mah.start_job") {
		t.Errorf("the withheld line should still name mah.start_job, which actions does not install:\n%s", out)
	}
	for _, path := range []string{"mah.job_progress", "mah.job_complete", "mah.job_fail"} {
		if !mahHas(t, pm, "acts", path) {
			t.Errorf("%s is not installed, so the log was right and this test is wrong", path)
		}
	}
}

func TestARevokedPluginCannotStillWriteTheDatabase(t *testing.T) {
	// Revocation stops new dispatch and new registrations, but a worker already
	// inside keeps its fully-installed mah table until it finishes — up to five
	// minutes. Without a liveness check on the DB accessors, DisablePlugin
	// returns, the UI shows the plugin off, and mah.db.create_tag still
	// succeeds.
	//
	// The worker reports through mah.log, because by the time it runs its VM is
	// closed and its globals are unreadable.
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	dir := t.TempDir()
	writePlugin(t, dir, "lingerer", `
plugin = { name = "lingerer", version = "1.0", api_version = 1, capabilities = { "jobs", "db:write" } }
function init()
    mah.start_job("linger", function(id)
        mah.sleep(7)
        local result, err = mah.db.create_tag({ name = "written-after-disable" })
        mah.log("info", "after-revocation write succeeded: " .. tostring(result ~= nil))
        mah.job_complete(id)
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	// A writer that would accept the call, so the refusal has to come from the
	// liveness check rather than from the write failing anyway.
	writer := &acceptingWriter{}
	pm.SetEntityQuerier(writer)
	pm.SetEntityWriter(writer)
	if err := pm.EnablePlugin("lingerer"); err != nil {
		t.Fatal(err)
	}

	// Let the worker acquire the VM lock and get inside its sleep. Revoked
	// before it starts, it simply never runs — which is correct, and proves
	// nothing about a worker that is already executing.
	time.Sleep(500 * time.Millisecond)

	if err := pm.DisablePlugin("lingerer"); err != nil { // drains 5s, then returns
		t.Fatal(err)
	}

	// Let the worker reach its write, well after the disable returned.
	time.Sleep(5 * time.Second)

	out := buf.String()
	if !strings.Contains(out, "after-revocation write succeeded:") {
		t.Fatalf("the worker never reached its write, so this test proved nothing. log:\n%s", out)
	}
	if strings.Contains(out, "after-revocation write succeeded: true") {
		t.Error("a revoked plugin wrote to the database after DisablePlugin returned")
	}
}

// acceptingWriter accepts the one call this file needs, so a refusal can only
// have come from the liveness check.
type acceptingWriter struct {
	mockQuerier
	stubWriter
}

func (a *acceptingWriter) CreateTag(opts map[string]any) (map[string]any, error) {
	return map[string]any{"id": float64(1), "name": opts["name"]}, nil
}

// syncBuffer is a log sink safe to read while a plugin's goroutine is still
// writing to it. bytes.Buffer is not, and a worker that logs after its plugin
// was disabled is exactly the case these tests are built around.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestADisableDuringInitIsNotLost(t *testing.T) {
	// A loading plugin is not in pm.plugins, so a disable arriving during a slow
	// init() found nothing and answered "not enabled" — which the context layer
	// reads as success and persists as enabled=false. The load then published
	// normally, leaving the plugin running with every granted host function
	// while the operator and the database both believed it was off.
	//
	// The disable now waits for the load to finish and then disables it
	// properly, so the answer it gives is true when it gives it.
	dir := t.TempDir()
	writePlugin(t, dir, "slow", `
plugin = { name = "slow", version = "1.0", api_version = 1, capabilities = { "pages" } }
function init()
    mah.page("home", function(ctx) return "ok" end)
    mah.sleep(2)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	enabled := make(chan error, 1)
	go func() { enabled <- pm.EnablePlugin("slow") }()
	time.Sleep(400 * time.Millisecond) // inside init()

	if err := pm.DisablePlugin("slow"); err != nil {
		t.Fatalf("DisablePlugin during init(): %v", err)
	}

	<-enabled
	if pm.IsEnabled("slow") {
		t.Error("the plugin is running although it was disabled")
	}
	if len(pm.GetPages()) != 0 {
		t.Errorf("its registrations survived: %v", pm.GetPages())
	}
	assertNoOrphanRegistrations(t, pm)
}

func TestADisableCannotSucceedWhileInitIsWedged(t *testing.T) {
	// The honest failure. init() is deliberately unbounded, so a plugin that
	// never returns from it cannot be disabled — and saying so is the only
	// answer that is true. The first design reported success here and left the
	// plugin running inside init() forever, un-disableable and un-enablable,
	// with the database recording it as off.
	dir := t.TempDir()
	writePlugin(t, dir, "wedged", `
plugin = { name = "wedged", version = "1.0", api_version = 1, capabilities = {} }
function init() while true do end end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	go func() { _ = pm.EnablePlugin("wedged") }()
	time.Sleep(400 * time.Millisecond)

	err = pm.DisablePlugin("wedged")
	if err == nil {
		t.Fatal("disabling a plugin wedged inside init() reported success")
	}
	if !strings.Contains(err.Error(), "still loading") {
		t.Errorf("the refusal must say why, got %v", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	// Close is a deferred shutdown step and a t.Cleanup in a great many tests,
	// so it gets called twice — and closing a closed channel panics.
	pm := mustEnable(t, t.TempDir(), "twiceclosed", `
plugin = { name = "twiceclosed", version = "1.0", api_version = 1, capabilities = {} }
function init() end
`)
	pm.Close()
	pm.Close() // must not panic

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); pm.Close() }()
	}
	wg.Wait()
}

func TestARevokedPluginCannotReachTheNetwork(t *testing.T) {
	// The DB and KV accessors refuse a revoked VM; egress was the one channel
	// that did not. A plugin an operator has just disabled could keep making
	// arbitrary outbound requests for the rest of its async allowance, carrying
	// whatever it already held in Lua locals — including a key it read from
	// mah.get_setting before the disable. That is the channel a plugin gets
	// disabled *for*.
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// The manifest names the server's loopback address, so revocation is the
	// only thing that can refuse this request. Left undeclared, the dial-time
	// egress deny would refuse it whether or not DisablePlugin closed the
	// channel, and the test would go green with the revocation check deleted.
	host, _, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}

	pm := mustEnable(t, t.TempDir(), "exfil", `
plugin = { name = "exfil", version = "1.0", api_version = 1, capabilities = { "jobs", "http" },
           network = { "`+host+`" }, allow_private_hosts = true }
function init()
    mah.start_job("linger", function(id)
        mah.sleep(7)
        local resp = mah.http.get_sync("`+srv.URL+`/after-disable")
        mah.log("info", "after-revocation request reached the server: " .. tostring(resp.error == nil))
        mah.job_complete(id)
    end)
end
`)
	time.Sleep(500 * time.Millisecond) // let the worker get inside

	if err := pm.DisablePlugin("exfil"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)

	out := buf.String()
	if !strings.Contains(out, "after-revocation request reached the server:") {
		t.Fatal("the worker never reached its request, so this test proved nothing")
	}
	if strings.Contains(out, "after-revocation request reached the server: true") {
		t.Error("a revoked plugin made an outbound request after DisablePlugin returned")
	}
}
