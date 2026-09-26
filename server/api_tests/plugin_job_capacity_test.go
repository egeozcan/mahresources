package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// capacityBusyPlugin runs one piece of background work that holds its VM until the
// test opens the gate, which is how a plugin's backlog is made deterministic.
const capacityBusyPlugin = `
plugin = { name = "busy-plugin", version = "1.0", api_version = 1, capabilities = { "actions", "jobs", "kv" } }

function gated(ctx)
    local n = tonumber(mah.kv.get("started") or "0") + 1
    mah.kv.set("started", tostring(n))
    while mah.kv.get("gate") ~= "open" do mah.sleep(0.02) end
end

function init()
    mah.action({ id = "gated", label = "Gated", entity = "resource", async = true, handler = gated })
end
`

// capacityIdlePlugin is a second plugin whose VM is free: its async action is
// instant, and its synchronous action starts a closure-backed job.
const capacityIdlePlugin = `
plugin = { name = "idle-plugin", version = "1.0", api_version = 1, capabilities = { "actions", "jobs", "kv" } }

function quick(ctx)
    mah.job_complete(ctx.job_id, { message = "quick done" })
end

function spawn(ctx)
    local id = mah.start_job("spawned work", function(job_id)
        mah.kv.set("closure", "ran")
        mah.job_complete(job_id, { message = "closure done" })
    end)
    mah.kv.set("spawned", id)
    return { success = true, message = "spawned " .. id }
end

function init()
    mah.action({ id = "quick", label = "Quick", entity = "resource", async = true, handler = quick })
    mah.action({ id = "spawn", label = "Spawn", entity = "resource", handler = spawn })
end
`

// setupPluginCapacityEnv serves both plugins under a deployment budget, with a
// dispatch loop running the way a deployment runs one.
func setupPluginCapacityEnv(t *testing.T, budget int) *TestContext {
	t.Helper()
	root := t.TempDir()
	for name, source := range map[string]string{"busy-plugin": capacityBusyPlugin, "idle-plugin": capacityIdlePlugin} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(source), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	tc := setupTestEnvWithConfig(t, func(config *application_context.MahresourcesConfig) {
		config.PluginPath = root
		config.MaxJobConcurrency = budget
	})
	pm := tc.AppCtx.PluginManager()
	if pm == nil {
		t.Fatal("the app context has no plugin manager")
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"busy-plugin", "idle-plugin"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("enable %s: %v", name, err)
		}
	}
	runtime := application_context.NewJobRuntime(tc.AppCtx, tc.AppCtx.JobService(), application_context.JobRuntimeConfig{
		Claimant: "capacity-test", Interval: 50 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)
	// The gate opens on the way out whatever happens, so a failing test does not
	// leave a handler looping until its five-minute deadline.
	t.Cleanup(func() { _ = tc.AppCtx.PluginKVSet("busy-plugin", "gate", `"open"`) })
	return tc
}

func capacityKV(t *testing.T, tc *TestContext, plugin, key string) string {
	t.Helper()
	value, found, err := tc.AppCtx.PluginKVGet(plugin, key)
	if err != nil {
		t.Fatalf("read %s kv %q: %v", plugin, key, err)
	}
	if !found {
		return ""
	}
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	return decoded
}

func capacityJob(t *testing.T, tc *TestContext, id string) jobs.Snapshot {
	t.Helper()
	snap, err := tc.AppCtx.JobService().Get(jobs.Deps{DB: tc.DB}, jobs.Access{Administrator: true}, id)
	if err != nil {
		t.Fatalf("read job %s: %v", id, err)
	}
	return snap
}

func capacityWait(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", within, what)
}

func globalSlotsHeld(t *testing.T, tc *TestContext) int64 {
	t.Helper()
	var held int64
	if err := tc.DB.Model(&models.JobCapacityLease{}).Where("capacity_group = ?", jobs.CapacityGroupGlobal).
		Count(&held).Error; err != nil {
		t.Fatalf("count global capacity: %v", err)
	}
	return held
}

// TestAPluginBacklogHoldsOneSlotAndLeavesTheRestToOtherWork is the deployment-level
// shape of a plugin's backlog. A plugin runs one handler at a time, so of six
// actions submitted together one is running and five are waiting for that plugin —
// queued, with no started event, and holding none of the deployment's budget. What
// they must not do is take the budget with them: another plugin's action and
// another Kind's transfer both run to completion while the backlog is still
// waiting, and the backlog itself then drains one at a time, in the order it was
// submitted.
func TestAPluginBacklogHoldsOneSlotAndLeavesTheRestToOtherWork(t *testing.T) {
	tc := setupPluginCapacityEnv(t, 4)

	const backlog = 6
	ids := make([]string, 0, backlog)
	for entity := 1; entity <= backlog; entity++ {
		_, id, err := tc.AppCtx.RunPluginActionAsync(nil, "busy-plugin", "gated", uint(entity), nil, "")
		if err != nil {
			t.Fatalf("submit busy action %d: %v", entity, err)
		}
		ids = append(ids, id)
	}
	capacityWait(t, "the first busy action to start", 10*time.Second, func() bool {
		return capacityKV(t, tc, "busy-plugin", "started") == "1"
	})
	// Several dispatch ticks: long enough for anything that claims waiting work to
	// have claimed it.
	time.Sleep(400 * time.Millisecond)

	running, queued := 0, 0
	for _, id := range ids {
		snap := capacityJob(t, tc, id)
		switch snap.State {
		case jobs.StateRunning:
			running++
		case jobs.StateQueued:
			queued++
			if snap.StartedAt != nil {
				t.Fatalf("waiting job %s records a start at %s", id, snap.StartedAt)
			}
		default:
			t.Fatalf("busy job %s is %s while its plugin's gate is closed", id, snap.State)
		}
	}
	if running != 1 || queued != backlog-1 {
		t.Fatalf("busy jobs: %d running and %d queued, want 1 running and %d queued", running, queued, backlog-1)
	}
	if held := globalSlotsHeld(t, tc); held != 1 {
		t.Fatalf("the deployment budget holds %d slots for one running plugin action, want 1", held)
	}

	// Another plugin is not starved.
	_, idleID, err := tc.AppCtx.RunPluginActionAsync(nil, "idle-plugin", "quick", 1, nil, "")
	if err != nil {
		t.Fatalf("submit the idle plugin's action: %v", err)
	}
	capacityWait(t, "the other plugin's action to finish", 10*time.Second, func() bool {
		return capacityJob(t, tc, idleID).State == jobs.StateSucceeded
	})

	// Another Kind is not starved.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("x", 500)))
	}))
	t.Cleanup(srv.Close)
	submitted := tc.AppCtx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/small.bin"}, nil, "", "api")
	if len(submitted) != 1 || submitted[0].Err != nil || submitted[0].CanonicalJobID == "" {
		t.Fatalf("submit the download: %+v", submitted)
	}
	capacityWait(t, "a download to finish while the backlog waits", 10*time.Second, func() bool {
		return capacityJob(t, tc, submitted[0].CanonicalJobID).State == jobs.StateSucceeded
	})
	if got := capacityKV(t, tc, "busy-plugin", "started"); got != "1" {
		t.Fatalf("%s busy actions started while the gate was closed, want 1", got)
	}

	if err := tc.AppCtx.PluginKVSet("busy-plugin", "gate", `"open"`); err != nil {
		t.Fatalf("open the gate: %v", err)
	}
	capacityWait(t, "the backlog to drain", 20*time.Second, func() bool {
		busyRunning := 0
		finished := 0
		for _, id := range ids {
			switch capacityJob(t, tc, id).State {
			case jobs.StateRunning:
				busyRunning++
			case jobs.StateSucceeded:
				finished++
			}
		}
		if busyRunning > 1 {
			t.Fatalf("%d of one plugin's actions were running at once", busyRunning)
		}
		return finished == backlog
	})

	type start struct {
		id string
		at time.Time
	}
	starts := make([]start, 0, backlog)
	for _, id := range ids {
		snap := capacityJob(t, tc, id)
		if snap.StartedAt == nil {
			t.Fatalf("finished job %s records no start", id)
		}
		starts = append(starts, start{id: id, at: *snap.StartedAt})
	}
	sort.SliceStable(starts, func(i, j int) bool { return starts[i].at.Before(starts[j].at) })
	for i, got := range starts {
		if got.id != ids[i] {
			t.Fatalf("the backlog started out of order: position %d is %s, want %s", i, got.id, ids[i])
		}
	}
}

// TestAStartJobMadeWhileTheBudgetIsFullWaitsForASlot pins mah.start_job against a
// full deployment budget. Acceptance is the promise that the work is durable, and
// a full budget says nothing about work that may wait: the call returns its job
// id, the Job is queued, and the closure runs once a slot frees — rather than the
// call raising and the plugin's own request failing with it.
func TestAStartJobMadeWhileTheBudgetIsFullWaitsForASlot(t *testing.T) {
	tc := setupPluginCapacityEnv(t, 1)

	_, holder, err := tc.AppCtx.RunPluginActionAsync(nil, "busy-plugin", "gated", 1, nil, "")
	if err != nil {
		t.Fatalf("submit the action that holds the budget: %v", err)
	}
	capacityWait(t, "the holding action to start", 10*time.Second, func() bool {
		return capacityJob(t, tc, holder).State == jobs.StateRunning
	})

	result, err := tc.AppCtx.PluginManager().RunAction(context.Background(), "idle-plugin", "spawn", 1, nil, "")
	if err != nil {
		t.Fatalf("mah.start_job raised against a full budget: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("the action that called mah.start_job answered %+v", result)
	}
	spawned := capacityKV(t, tc, "idle-plugin", "spawned")
	if spawned == "" {
		t.Fatal("mah.start_job returned no job id")
	}
	projected, err := tc.AppCtx.ProjectActionJob(spawned)
	if err != nil || projected == nil {
		t.Fatalf("the spawned job's handle resolves to %+v (err %v)", projected, err)
	}
	closureJob := projected.CanonicalJobID
	time.Sleep(300 * time.Millisecond)
	if state := capacityJob(t, tc, closureJob).State; state != jobs.StateQueued {
		t.Fatalf("the closure job is %s while the budget is full, want queued", state)
	}
	if got := capacityKV(t, tc, "idle-plugin", "closure"); got != "" {
		t.Fatalf("the closure ran (%q) while the budget was full", got)
	}

	if err := tc.AppCtx.PluginKVSet("busy-plugin", "gate", `"open"`); err != nil {
		t.Fatalf("open the gate: %v", err)
	}
	capacityWait(t, "the closure to run once the slot freed", 10*time.Second, func() bool {
		return capacityJob(t, tc, closureJob).State == jobs.StateSucceeded
	})
	if got := capacityKV(t, tc, "idle-plugin", "closure"); got != "ran" {
		t.Fatalf("the closure job succeeded but its callback recorded %q", got)
	}
}

// holdGlobalSlot occupies the deployment's whole budget of one with a Job nobody
// runs, and returns the function that ends it.
func holdGlobalSlot(t *testing.T, tc *TestContext) func() {
	t.Helper()
	deps := jobs.Deps{DB: tc.DB}
	accepted, err := tc.AppCtx.JobService().Accept(deps, jobs.Acceptance{
		Kind: application_context.JobKindPluginAction, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", Title: "holding the budget", Replay: jobs.ReplayInput{NonReplayable: true},
		Summary: json.RawMessage(`{"subtype":"closure-start-job","plugin":"busy-plugin","runtime":"elsewhere/boot/1"}`),
	})
	if err != nil {
		t.Fatalf("accept the holding job: %v", err)
	}
	execution, err := tc.AppCtx.JobService().ClaimJob(context.Background(), deps, jobs.ClaimRequest{
		Kind: application_context.JobKindPluginAction, KindVersion: 1, JobID: accepted.ID,
		Claimant: "capacity-holder", Capacity: []jobs.CapacityRef{{Group: jobs.CapacityGroupGlobal, Limit: 1}},
	})
	if err != nil {
		t.Fatalf("claim the holding job: %v", err)
	}
	return func() {
		if _, err := tc.AppCtx.JobService().Finish(deps, jobs.FinishRequest{
			ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
			ExpectedVersion: execution.Version, Outcome: jobs.StateSucceeded,
		}); err != nil {
			t.Fatalf("end the holding job: %v", err)
		}
	}
}

// TestAPluginCommandWaitsForAFullBudgetInsteadOfFailing is the command runtime's
// half. A command accepted while the deployment's budget is full was answered 202
// and then failed for good, "not claimable", before it ever started. A full budget
// is a wait: the run stays queued, and runs to completion once the slot frees.
func TestAPluginCommandWaitsForAFullBudgetInsteadOfFailing(t *testing.T) {
	pluginDir := t.TempDir()
	commandDir := t.TempDir()
	stagingRoot := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "command-capacity.db")
	executable := installCommandIntegrationExecutable(t, commandDir)
	writeCommandIntegrationPlugin(t, pluginDir, executable)
	settings := commandIntegrationSettings{root: stagingRoot, commandPath: commandDir}

	tc, closeContext := openPersistentCommandTestContext(t, databasePath, pluginDir, afero.NewMemMapFs())
	tc.AppCtx.Config.MaxJobConcurrency = 1
	commandsStarted := false
	t.Cleanup(func() {
		if commandsStarted {
			_ = tc.AppCtx.StopPluginCommands()
		}
		closeContext()
	})
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	commandsStarted = true
	if _, err := tc.AppCtx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	_, bearer := commandIntegrationBearer(t, tc)
	if err := tc.AppCtx.WithPrincipal(nil).SetPluginEnabledWithOptions(
		commandIntegrationPluginName, true, application_context.PluginEnableOptions{ConfirmCommands: true},
	); err != nil {
		t.Fatal(err)
	}

	release := holdGlobalSlot(t, tc)
	start := doReq(tc, http.MethodGet, "/plugins/command-integration/start", map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "started:") {
		t.Fatalf("start page = %d %s", start.Code, start.Body.String())
	}
	run := waitForCommandRun(t, tc.AppCtx, "produce", "queued", "failed", "succeeded", "running")
	time.Sleep(1500 * time.Millisecond)
	record, _, err := tc.AppCtx.Run(run.ID)
	if err != nil {
		t.Fatalf("read the run: %v", err)
	}
	if record.Status != "queued" {
		t.Fatalf("a command accepted while the budget was full is %q (%s), want queued", record.Status, record.Error)
	}
	if record.JobID != "" {
		if state := capacityJob(t, tc, record.JobID).State; state != jobs.StateQueued {
			t.Fatalf("its Job is %s while the budget is full, want queued", state)
		}
	}

	release()
	succeeded := waitForCommandRun(t, tc.AppCtx, "produce", "succeeded", "failed")
	if succeeded.Status != "succeeded" {
		t.Fatalf("the command ended %s (%s) once the slot freed, want succeeded", succeeded.Status, succeeded.Error)
	}
	if succeeded.JobID != "" {
		capacityWait(t, "the command's Job to succeed", 10*time.Second, func() bool {
			return capacityJob(t, tc, succeeded.JobID).State == jobs.StateSucceeded
		})
	}
}
