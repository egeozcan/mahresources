package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

// This file drives the plugin-action Kind: an async registered action, one
// scheduled occurrence, and a closure-backed mah.start_job — the three shapes of
// plugin background work that now have durable identity.
//
// Everything is taken through the seam its real caller uses: the context method
// the HTTP handler calls, a real plugin manager with a real Lua plugin, a real
// control plane and a running dispatch loop. A fake at any link would let the test
// pass while the deployed path ran unmirrored.

// pluginActionTestPlugin is the plugin whose actions and schedule these tests run.
const pluginActionTestPlugin = "action-plugin"

// pluginActionTestSource declares one async action per property worth pinning: an
// action that completes and records that it ran, one that fails, one that starts a
// closure-backed child job, one that floods progress, and a schedule.
const pluginActionTestSource = `
plugin = { name = "` + pluginActionTestPlugin + `", version = "1.0", api_version = 1,
           capabilities = { "actions", "jobs", "kv", "schedule", "hooks", "job_events" } }

local function bump(key)
    local n = tonumber(mah.kv.get(key) or "0") or 0
    mah.kv.set(key, tostring(n + 1))
end

function async_work(ctx)
    bump("ran")
    mah.job_progress(ctx.job_id, 50, "halfway")
    mah.job_complete(ctx.job_id, { message = "all done", entity = ctx.entity_id,
                                  note = ctx.params.note or "" })
end

function failing_work(ctx)
    mah.job_fail(ctx.job_id, "the action refused")
end

-- Reports failure and keeps executing. The plugin's report is a *request*: the
-- handler can still be writing when it makes it, and the durable Job must not be
-- ended — with its capacity and its claim handed back — while that is true.
function lingering_work(ctx)
    mah.job_fail(ctx.job_id, "the action refused")
    mah.kv.set("lingering", "reported")
    mah.sleep(2)
    mah.kv.set("lingering", "returned")
end

function closure_work(job_id)
    mah.kv.set("closure", "ran")
    mah.job_complete(job_id, { message = "closure done" })
end

function parent_work(ctx)
    local child = mah.start_job("child work", closure_work)
    mah.kv.set("child", child)
    mah.job_complete(ctx.job_id, { message = "parent done" })
end

function leaky_work(ctx)
    mah.job_progress(ctx.job_id, 10, "touching " .. ctx.params.secret)
    mah.job_fail(ctx.job_id, "failed on " .. ctx.params.secret ..
                 " at https://signed.example/x?token=" .. ctx.params.secret)
end

function chatty_work(ctx)
    mah.job_progress(ctx.job_id, 50, "halfway through " .. ctx.params.secret)
    mah.job_complete(ctx.job_id, { message = "finished " .. ctx.params.secret })
end

function burst_work(ctx)
    for i = 1, 400 do
        mah.job_progress(ctx.job_id, i % 100, string.rep("x", 900))
    end
    mah.job_complete(ctx.job_id, { message = "burst done" })
end

-- follow_up is armed by a key rather than always on, so the hook is inert in
-- every other test that shares this plugin.
function follow_up(event)
    if mah.kv.get("armed") ~= "yes" then return end
    bump("follow-ups")
    mah.start_job("follow-up work", closure_work)
end

function init()
    mah.action({ id = "async-work", label = "Async Work", entity = "resource", async = true,
                 params = { {name = "note", type = "text", label = "Note"} },
                 handler = async_work })
    mah.action({ id = "failing-work", label = "Failing Work", entity = "resource", async = true,
                 handler = failing_work })
    mah.action({ id = "lingering-work", label = "Lingering Work", entity = "resource", async = true,
                 handler = lingering_work })
    mah.action({ id = "retryable-work", label = "Retryable Work", entity = "resource", async = true,
                 retry = true, handler = failing_work })
    mah.action({ id = "parent-work", label = "Parent Work", entity = "resource", async = true,
                 handler = parent_work })
    mah.action({ id = "burst-work", label = "Burst Work", entity = "resource", async = true,
                 handler = burst_work })
    mah.action({ id = "leaky-work", label = "Leaky Work", entity = "resource", async = true,
                 params = { {name = "secret", type = "text", label = "Secret"} },
                 handler = leaky_work })
    mah.action({ id = "chatty-work", label = "Chatty Work", entity = "resource", async = true,
                 params = { {name = "secret", type = "text", label = "Secret"} },
                 handler = chatty_work })
    mah.schedule({ id = "tick", every = "1m", overlap = "skip", handler = function(job_id)
        bump("scheduled")
    end })
    mah.on("after_job_completed", follow_up)
end
`

// newPluginActionJobContext builds the shared job harness and enables the plugin
// these tests act through, failing the test if it declares nothing.
func newPluginActionJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newJobHarnessContext(t, true)
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("the harness context has no plugin manager")
	}
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	return ctx
}

// pluginActionJobBySubtype answers the newest Job of this Kind with one subtype.
func pluginActionJobBySubtype(t *testing.T, ctx *MahresourcesContext, subtype string, minCount int) jobs.Snapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
			jobs.Filter{Kinds: []string{JobKindPluginAction}}, jobs.Cursor{}, 50)
		if err == nil {
			matching := make([]jobs.Snapshot, 0, len(page.Jobs))
			for _, job := range page.Jobs {
				if pluginActionSubtypeOf(job.Summary) == subtype {
					matching = append(matching, job)
				}
			}
			if len(matching) >= minCount {
				return matching[0]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no durable %s job of subtype %q was ever accepted", JobKindPluginAction, subtype)
	return jobs.Snapshot{}
}

// pluginKVForTest reads one mah.kv value back as a string, which is the only
// durable thing these plugins write without further grants.
func pluginKVForTest(t *testing.T, ctx *MahresourcesContext, key string) string {
	t.Helper()
	value, found, err := ctx.PluginKVGet(pluginActionTestPlugin, key)
	if err != nil {
		t.Fatalf("read plugin kv %q: %v", key, err)
	}
	if !found {
		return ""
	}
	var raw any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return value
	}
	if text, ok := raw.(string); ok {
		return text
	}
	return value
}

// waitForJobState waits for one Job to satisfy cond.
func waitForJobState(t *testing.T, ctx *MahresourcesContext, jobID, what string, cond func(jobs.Snapshot) bool) jobs.Snapshot {
	t.Helper()
	return waitForSnapshot(t, ctx, jobID, what, cond)
}

// TestAnAsyncPluginActionAcceptsADurableJobBeforeItRuns is the whole
// dual-publication contract for a registered action: a durable Job exists before
// the handler runs, its summary is the sanitized provenance and never the params,
// the handler's own completion is the Job's outcome, and the id the client was
// answered with is the id the panel and the legacy endpoint resolve.
func TestAnAsyncPluginActionAcceptsADurableJobBeforeItRuns(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	handle, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 7,
		map[string]any{"note": "super-secret-note"}, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	if handle == "" || canonical == "" {
		t.Fatalf("the action answered handle %q canonical %q", handle, canonical)
	}

	job := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeRegistered, 1)
	if job.ID != canonical {
		t.Fatalf("the answered canonical id %s is not the accepted job %s", canonical, job.ID)
	}
	summary := string(job.Summary)
	for _, want := range []string{pluginActionSubtypeRegistered, pluginActionTestPlugin, "async-work"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("the sanitized summary %s does not name %q", summary, want)
		}
	}
	if strings.Contains(summary, "super-secret-note") {
		t.Fatalf("the sanitized summary carries a param value: %s", summary)
	}

	job = waitForJobState(t, ctx, job.ID, "the action to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateSucceeded {
		t.Fatalf("the action ended %s (%+v)", job.State, job.Failure)
	}
	if got := pluginKVForTest(t, ctx, "ran"); got != "1" {
		t.Fatalf("the handler ran %q times, want once", got)
	}

	// The compatibility projection answers to the id the client holds, and the
	// durable Job is the execution it describes.
	legacy := ctx.PluginManager().GetActionJob(handle)
	if legacy == nil {
		t.Fatalf("the legacy action-job endpoint has no entry for %q", handle)
	}
	if legacy.ID != handle {
		t.Fatalf("the projection is listed as %q, want %q", legacy.ID, handle)
	}
	if legacy.Status != "completed" {
		t.Fatalf("the projection says %q, want completed", legacy.Status)
	}
}

// TestAPluginActionRefusalIsRecheckedAtDispatch pins the re-validation: between
// the request and the run a plugin can be disabled, so the execution must not be
// able to enter a handler that no longer exists.
func TestAPluginActionRefusalIsRecheckedAtDispatch(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	// A Job accepted for an action the plugin does not declare is one no
	// dispatch may run. It is accepted directly rather than through the request
	// path, which validates first, because the property under test is what
	// *dispatch* does with input that no longer resolves.
	input, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered,
		Plugin:  pluginActionTestPlugin,
		Action:  "async-work",
		Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("build input: %v", err)
	}
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		State:       jobs.StateQueued,
		Origin:      "plugin",
		Title:       "Async Work",
		Replay:      jobs.ReplayInput{Input: input},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	if err := ctx.PluginManager().DisablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("disable the plugin: %v", err)
	}

	job := waitForJobState(t, ctx, accepted.ID, "the refused action to be settled", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateBlocked || s.State.Terminal()
	})
	if job.State != jobs.StateBlocked {
		t.Fatalf("the action ended %s, want blocked: a refusal is a decision for a person, not a failure", job.State)
	}
	if got := pluginKVForTest(t, ctx, "ran"); got != "" {
		t.Fatalf("a refused action still ran its handler (%q)", got)
	}
}

// TestAScheduledOccurrenceMaterializesExactlyOneJob is the scheduler's own
// contract: a claimed row becomes one durable Job, the handler's run is that Job's
// outcome, the row records it — and a tick that skips a row whose plugin no longer
// declares it materializes nothing at all.
func TestAScheduledOccurrenceMaterializesExactlyOneJob(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	// A schedule row is owned by the operator who enabled the plugin, and an
	// unowned row is never claimed: the scheduler refuses it rather than falling
	// back to root. So the harness needs an administrator for the row to belong
	// to, exactly as the deployed flow has the person who enabled the plugin.
	operator := models.User{Username: "schedule-operator", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&operator).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	ctx.refreshRootAdmin()

	pm := ctx.PluginManager()
	if err := ctx.SyncPluginSchedules(pluginActionTestPlugin, pm.DeclaredSchedules(pluginActionTestPlugin)); err != nil {
		t.Fatalf("sync schedules: %v", err)
	}

	// A freshly registered schedule is due one interval out, so the row is made
	// due the way time makes it due — the design's cadence is a real due time,
	// not an immediate first run.
	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "tick").First(&row).Error; err != nil {
		t.Fatalf("load the schedule row: %v", err)
	}
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make the schedule due: %v", err)
	}

	scheduler := NewPluginScheduler(ctx, time.Minute)
	scheduler.Tick(time.Now())

	job := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeScheduled, 1)
	job = waitForJobState(t, ctx, job.ID, "the occurrence to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateSucceeded {
		t.Fatalf("the occurrence ended %s (%+v)", job.State, job.Failure)
	}
	if job.Origin != "schedule" {
		t.Fatalf("the occurrence's origin is %q, want schedule", job.Origin)
	}
	if got := pluginKVForTest(t, ctx, "scheduled"); got != "1" {
		t.Fatalf("the schedule handler ran %q times, want once", got)
	}

	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginActionTestPlugin, "tick").First(&row).Error; err != nil {
		t.Fatalf("reload the schedule row: %v", err)
	}
	if row.LastStatus != models.PluginScheduleStatusCompleted {
		t.Fatalf("the row records %q, want completed", row.LastStatus)
	}

	// A tick that finds no live registration claims nothing, so it materializes
	// nothing: the Job count is the measure of "materialized", and a registered
	// but disabled plugin has to leave it alone.
	before := countPluginActionJobs(t, ctx)
	if err := pm.DisablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("disable the plugin: %v", err)
	}
	if err := ctx.SyncPluginSchedules(pluginActionTestPlugin, pm.DeclaredSchedules(pluginActionTestPlugin)); err != nil {
		t.Fatalf("re-sync schedules: %v", err)
	}
	scheduler.dispatchWait = 50 * time.Millisecond
	scheduler.Tick(time.Now().Add(2 * time.Hour))
	if after := countPluginActionJobs(t, ctx); after != before {
		t.Fatalf("a tick with no live registration materialized %d job(s)", after-before)
	}
}

// countPluginActionJobs counts every Job of this Kind.
func countPluginActionJobs(t *testing.T, ctx *MahresourcesContext) int {
	t.Helper()
	page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
		jobs.Filter{Kinds: []string{JobKindPluginAction}}, jobs.Cursor{}, 200)
	if err != nil {
		t.Fatalf("list plugin jobs: %v", err)
	}
	return len(page.Jobs)
}

// TestAStartJobFromAnActionIsANonReplayableChildJob pins the closure-backed
// subtype: the nested work is its own Job linked to the action that asked for it,
// it runs, and it never offers a Retry — because its Lua function died with the
// process that held it and no input could bring it back.
func TestAStartJobFromAnActionIsANonReplayableChildJob(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "parent-work", 3, nil, "")
	if err != nil {
		t.Fatalf("run the parent action: %v", err)
	}
	parent := waitForJobState(t, ctx, canonical, "the parent action to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if parent.State != jobs.StateSucceeded {
		t.Fatalf("the parent action ended %s (%+v)", parent.State, parent.Failure)
	}

	child := pluginActionJobBySubtype(t, ctx, pluginActionSubtypeClosure, 1)
	child = waitForJobState(t, ctx, child.ID, "the closure to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if child.ReplayClass != jobs.ReplayClassNonReplayable {
		t.Fatalf("the closure job is %q, want non-replayable", child.ReplayClass)
	}
	if got := pluginKVForTest(t, ctx, "closure"); got != "ran" {
		t.Fatalf("the closure ran %q, want once", got)
	}

	lineage, err := ctx.JobService().Lineage(ctx.jobDeps(), jobs.Access{Administrator: true}, child.ID)
	if err != nil {
		t.Fatalf("read the closure's lineage: %v", err)
	}
	linked := false
	for _, relative := range lineage.Parents {
		if relative.ID == parent.ID {
			linked = true
		}
	}
	if !linked {
		t.Fatalf("the closure job is not a child of the action that started it: %+v", lineage)
	}

	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(),
		jobs.Access{Administrator: true}, child.ID)
	if err != nil {
		t.Fatalf("advertise commands: %v", err)
	}
	if offersCommand(commands, jobs.CommandRetry) {
		t.Fatalf("a closure-backed job offered a Retry: %+v", commands)
	}
}

// TestAFailedPluginActionOffersARetryAndTheRetryIsANewJob pins the two halves of
// the command contract that matter here: an unsuccessful registered action is
// retryable, and the Retry is a *new* Job linked to its ancestor rather than a
// reopening of it.
func TestAFailedPluginActionOffersARetryAndTheRetryIsANewJob(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "retryable-work", 4, nil, "")
	if err != nil {
		t.Fatalf("run the failing action: %v", err)
	}
	job := waitForJobState(t, ctx, canonical, "the action to fail", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateFailed {
		t.Fatalf("the action ended %s, want failed", job.State)
	}

	commands := advertisedForTest(t, ctx, job.ID)
	if !offersCommand(commands, jobs.CommandRetry) {
		t.Fatalf("a failed registered action offered no Retry: %+v", commands)
	}

	result, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: job.ID, Key: jobs.CommandRetry, IdempotencyKey: "retry-once",
		ExpectedVersion: job.Version, Actor: jobs.Access{Administrator: true},
	})
	if err != nil {
		t.Fatalf("retry the action: %v", err)
	}
	if result.SuccessorID == "" {
		t.Fatalf("the retry answered no successor job: %+v", result)
	}
	if result.SuccessorID == job.ID {
		t.Fatalf("the retry reopened the ancestor instead of creating a successor")
	}

	ancestor, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, job.ID)
	if err != nil {
		t.Fatalf("re-read the ancestor: %v", err)
	}
	if ancestor.State != jobs.StateFailed {
		t.Fatalf("the ancestor is now %s: a retry never changes its outcome", ancestor.State)
	}
}

// TestAClosureJobWhoseRuntimeIsProvedGoneIsInterrupted pins §3's rule for
// non-restorable work: an expired lease proves nothing, so the recorded runtime
// identity is what decides — a process that cannot still exist interrupts the Job,
// and one that cannot be inspected leaves it blocked rather than running it again.
func TestAClosureJobWhoseRuntimeIsProvedGoneIsInterrupted(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	// A claim held by a process on another host: unprovable, so the Job must be
	// blocked rather than interrupted or redispatched.
	foreign := acceptClosureJobForTest(t, ctx, "another-host/boot-1/4242")
	claimJobForTestAs(t, ctx, foreign.ID, "another-host/boot-1/4242")
	expireAndReconcile(t, ctx)
	if got := jobStateForTest(t, ctx, foreign.ID); got != jobs.StateBlocked {
		t.Fatalf("a job whose runtime is uninspectable is %s, want blocked", got)
	}

	// The same host, a boot session this machine is not in: that process cannot
	// exist any more, so the callback is proved gone.
	goneUntil := plugin_system.CurrentRuntimeIdentity().Host + "/boot-that-ended/4243"
	gone := acceptClosureJobForTest(t, ctx, goneUntil)
	claimJobForTestAs(t, ctx, gone.ID, goneUntil)
	expireAndReconcile(t, ctx)
	if got := jobStateForTest(t, ctx, gone.ID); got != jobs.StateInterrupted {
		t.Fatalf("a job whose runtime is proved gone is %s, want interrupted", got)
	}
}

// TestAReconciliationJudgesTheClaimHolderNotTheSubmitter is the identity the
// question belongs to.
//
// A Job's sealed input records who *submitted* the work, and that provenance is
// immutable: a Retry submitted here and claimed there keeps the input it was
// accepted with, so judging the execution by it means judging live work by a
// process that has nothing to do with it — here, a boot session that has ended,
// which would interrupt a callback this very process is holding. The claim's
// claimant is the execution identity, recorded when the Job was dispatched.
func TestAReconciliationJudgesTheClaimHolderNotTheSubmitter(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	// The submission provenance names a process that cannot still exist.
	submittedBy := plugin_system.CurrentRuntimeIdentity().Host + "/boot-that-ended/4243"
	job := acceptClosureJobForTest(t, ctx, submittedBy)
	// The claim belongs to this process, which is alive and still owns the
	// callback.
	claimJobForTestAs(t, ctx, job.ID, plugin_system.CurrentRuntimeIdentity().String())
	expireAndReconcile(t, ctx)

	if got := jobStateForTest(t, ctx, job.ID); got != jobs.StateBlocked {
		t.Fatalf("a job whose execution runtime is alive is %s, want blocked for a person to resolve", got)
	}
}

// acceptClosureJobForTest accepts one closure-backed Job recording a given runtime
// identity, through the same acceptance a real start_job uses.
func acceptClosureJobForTest(t *testing.T, ctx *MahresourcesContext, runtime string) jobs.Snapshot {
	t.Helper()
	input, err := json.Marshal(pluginActionJobInput{
		Subtype: pluginActionSubtypeClosure,
		Plugin:  pluginActionTestPlugin,
		Label:   "child work",
		Runtime: runtime,
	})
	if err != nil {
		t.Fatalf("build input: %v", err)
	}
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		State:       jobs.StateQueued,
		Origin:      "plugin",
		Title:       "child work",
		Replay:      jobs.ReplayInput{NonReplayable: true},
		Summary:     pluginActionSummaryOf(input),
	})
	if err != nil {
		t.Fatalf("accept the closure job: %v", err)
	}
	return accepted
}

// claimJobForTestAs claims one Job in the name of one runtime identity, which is
// what reconciliation judges the execution by.
func claimJobForTestAs(t *testing.T, ctx *MahresourcesContext, jobID, claimant string) {
	t.Helper()
	_, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion,
		JobID: jobID, Claimant: claimant,
	})
	if err != nil || !claimed {
		t.Fatalf("claim %s: claimed=%v err=%v", jobID, claimed, err)
	}
}

// claimJobForTest claims one Job the way a runtime does, so a lease exists for
// reconciliation to expire.
func claimJobForTest(t *testing.T, ctx *MahresourcesContext, jobID string) {
	t.Helper()
	_, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion,
		JobID: jobID, Claimant: "plugin-action-test",
	})
	if err != nil || !claimed {
		t.Fatalf("claim %s: claimed=%v err=%v", jobID, claimed, err)
	}
}

// expireAndReconcile runs one reconciliation pass with a clock far enough ahead
// that every claim in the test has expired.
func expireAndReconcile(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	deps := ctx.jobDeps()
	deps.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := ctx.JobService().ReconcileExpired(context.Background(), deps, "plugin-action-test", 20); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

// jobStateForTest reads one Job's state, ignoring that some outcomes are errors.
func jobStateForTest(t *testing.T, ctx *MahresourcesContext, jobID string) jobs.State {
	t.Helper()
	snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		t.Fatalf("read job %s: %v", jobID, err)
	}
	return snap.State
}

// TestAPluginProgressBurstCannotDisplaceItsOutcome is the rate-limit property: a
// plugin reporting hundreds of progress ticks is a snapshot being replaced, not a
// timeline being filled, so the terminal event always lands and the stored message
// stays inside its bound.
func TestAPluginProgressBurstCannotDisplaceItsOutcome(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "burst-work", 9, nil, "")
	if err != nil {
		t.Fatalf("run the bursting action: %v", err)
	}
	job := waitForJobState(t, ctx, canonical, "the bursting action to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateSucceeded {
		t.Fatalf("the bursting action ended %s (%+v)", job.State, job.Failure)
	}

	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, job.ID, 0, 500)
	if err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	terminal := false
	for _, event := range events {
		if event.Type == jobs.EventSucceeded {
			terminal = true
		}
	}
	if !terminal {
		t.Fatalf("the succeeded job has no terminal event: %+v", events)
	}
	if len(events) > 40 {
		t.Fatalf("a progress burst wrote %d events: progress is a snapshot, not a timeline", len(events))
	}
	if len(job.Progress.Message) > jobs.MaxProgressMessageBytes {
		t.Fatalf("the stored progress message is %d bytes, over the %d-byte ceiling",
			len(job.Progress.Message), jobs.MaxProgressMessageBytes)
	}
}

// TestAPluginJobIsAnnouncedToJobEventObservers pins the seam the after_job_*
// hooks ride: a plugin Job reaching an end state is announced through the same
// observer the download queue publishes through, so a plugin hears about work it
// started exactly as it hears about a transfer.
func TestAPluginJobIsAnnouncedToJobEventObservers(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	observer := &recordingJobEventSink{}
	ctx.SetJobEventSink(observer)

	if _, _, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 11, nil, ""); err != nil {
		t.Fatalf("run the action: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if records := observer.snapshot(); len(records) > 0 {
			if records[0].Status != "completed" {
				t.Fatalf("the announcement says %q, want completed", records[0].Status)
			}
			if !strings.Contains(records[0].Name, "async-work") {
				t.Fatalf("the announcement names %q, which does not say what ran", records[0].Name)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a finished plugin job announced nothing to the job-event observer")
}

// TestThePluginActionKindRefusesToRunOnAContextWithoutAPluginSystem documents
// the degradation: no plugin system is an error rather than a Job that never
// runs, because the caller asked for plugin work and there is none.
func TestThePluginActionKindRefusesToRunOnAContextWithoutAPluginSystem(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	// The Kind's adapter is what refuses; the caller's own validation is a
	// separate, earlier layer.
	adapter := &pluginActionAdapter{ctx: ctx}
	execution := jobs.Execution{JobID: "no-such-job"}
	if err := adapter.Dispatch(context.Background(), execution); err == nil {
		t.Fatalf("dispatch of an execution with no input was accepted")
	}
}

// recordingJobEventSink collects what a deployment's job-event observer is told.
//
// It is the one fake in this file, and it is a *seam* rather than a collaborator
// the production path depends on: the behaviour under test is that the
// announcement is made at all, and the observer's own dispatch (hook names and
// payloads) is covered where it lives.
type recordingJobEventSink struct {
	mu      sync.Mutex
	records []download_queue.JobEventRecord
}

// RecordJobEvent implements download_queue.JobEventSink.
func (s *recordingJobEventSink) RecordJobEvent(record download_queue.JobEventRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

// snapshot returns what the observer was told, in order.
func (s *recordingJobEventSink) snapshot() []download_queue.JobEventRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]download_queue.JobEventRecord(nil), s.records...)
}

// newPluginActionJobContextWithDeploymentBudget builds the plugin-action harness
// the way a deployment configures it: the concurrency budget lives in the
// deployment's configuration, and the runtime reads it from there rather than
// from a second copy of the number.
func newPluginActionJobContextWithDeploymentBudget(t *testing.T, budget int) *MahresourcesContext {
	t.Helper()
	ctx := newJobHarnessContext(t, false)
	ctx.Config.MaxJobConcurrency = budget
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("the harness context has no plugin manager")
	}
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{
		Claimant: "budget-test", Interval: 25 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)
	return ctx
}

// TestAHostSidePluginActionObeysTheDeploymentConcurrencyBudget pins that plugin
// work claimed by the process that accepted it occupies the same deployment-wide
// budget the polling runtime's claims do.
//
// The budget is the deployment's own (`max-job-concurrency`), it is
// database-backed and shared across processes, and host-side execution was the one
// admission path that bypassed it: a registered action ran in the submitting
// process while every global slot was taken, which is how a deployment of three
// processes ran three times the budget it configured. With the budget occupied the
// accepted Job stays queued for the loop — accepted, visible, and not lost — and
// runs when the slot is free.
func TestAHostSidePluginActionObeysTheDeploymentConcurrencyBudget(t *testing.T) {
	ctx := newPluginActionJobContextWithDeploymentBudget(t, 1)

	// One execution of another Kind holds the deployment's only slot.
	release := make(chan struct{})
	blocking := newRuntimeTestAdapter()
	blocking.dispatch = func(context.Context, jobs.Execution) error {
		<-release
		return nil
	}
	if err := ctx.JobService().RegisterAdapter(blocking); err != nil {
		t.Fatalf("register the blocking kind: %v", err)
	}
	acceptRuntimeJob(t, ctx.JobService(), ctx)
	waitFor(t, "the deployment slot to be occupied", func() bool {
		return storedCapacity(t, ctx, jobs.CapacityGroupGlobal) == 1
	})

	handle, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 7, nil, "")
	if err != nil {
		t.Fatalf("the action was refused instead of queued: %v", err)
	}
	if handle == "" || canonical == "" {
		t.Fatalf("the action answered handle %q canonical %q", handle, canonical)
	}
	if got := pluginKVForTest(t, ctx, "ran"); got != "" {
		t.Fatalf("the handler ran %q times while the deployment budget was full", got)
	}
	queued, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, canonical)
	if err != nil {
		t.Fatalf("read the accepted job: %v", err)
	}
	if queued.State != jobs.StateQueued {
		t.Fatalf("the accepted action is %s while the budget is full, want queued for the loop", queued.State)
	}

	// Freeing the slot lets the loop dispatch it: the work was deferred, not lost.
	close(release)
	waitFor(t, "the accepted action to run and finish", func() bool {
		snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, canonical)
		return err == nil && snap.State == jobs.StateSucceeded
	})
	if got := pluginKVForTest(t, ctx, "ran"); got != "1" {
		t.Fatalf("the handler ran %q times, want once", got)
	}
}

// TestAPluginJobEventHookThatStartsWorkCannotFeedItself is the causal boundary of
// the hook feed.
//
// An after_job_completed hook may legitimately start work with mah.start_job, and
// that is exactly the shape that can never terminate once plugin Jobs announce
// their own terminal events: the hook hears the completion of the Job it started,
// starts another, and so on for as long as the deployment runs. The rule is causal
// — a Job started from the delivery of a terminal job event does not deliver one —
// so the chain ends after the one follow-up the hook asked for.
func TestAPluginJobEventHookThatStartsWorkCannotFeedItself(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	// The hook feed is what this test is about, so the deployment's own observer is
	// the one installed — the seam between a terminal Job and the hooks is the
	// property under test, and a fake there would test the fake.
	dispatcher := NewJobEventDispatcher(ctx)
	ctx.SetJobEventSink(dispatcher)
	t.Cleanup(dispatcher.Stop)
	if err := ctx.PluginKVSet(pluginActionTestPlugin, "armed", `"yes"`); err != nil {
		t.Fatalf("arm the hook: %v", err)
	}

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 5, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	waitForJobState(t, ctx, canonical, "the action to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	waitFor(t, "the hook to start its follow-up", func() bool {
		return pluginKVForTest(t, ctx, "follow-ups") != ""
	})
	waitFor(t, "the follow-up closure to finish", func() bool {
		page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
			jobs.Filter{Kinds: []string{JobKindPluginAction}}, jobs.Cursor{}, 50)
		if err != nil {
			return false
		}
		for _, job := range page.Jobs {
			if pluginActionSubtypeOf(job.Summary) == pluginActionSubtypeClosure && job.State.Terminal() {
				return true
			}
		}
		return false
	})

	// The follow-up's completion is not delivered back to the hook, so nothing else
	// is started and the chain is over.
	time.Sleep(750 * time.Millisecond)
	if got := pluginKVForTest(t, ctx, "follow-ups"); got != "1" {
		t.Fatalf("the job-event hook started %q follow-ups, want exactly one: a hook that starts work was fed by that work's completion", got)
	}
}

// TestAClosureJobIsNeverClaimableByTheDispatchLoop is the acceptance race at
// mah.start_job, driven rather than reasoned about.
//
// A closure Job owns a *lua.LFunction in the process that accepted it, so it is
// not waiting work: no other runtime can run it. Accepting it and claiming it in
// two steps leaves an interval in which it *is* ordinary queued work of a
// registered Kind, and a dispatch loop polling that queue claims it there, finds
// no callback, and takes the Job away from the host that was about to run it —
// which the plugin sees as a refused mah.start_job. The loop here polls every few
// milliseconds, far faster than the window it used to need, and every closure must
// still be born owned by the process whose VM holds the callback.
func TestAClosureJobIsNeverClaimableByTheDispatchLoop(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("the harness context has no plugin manager")
	}
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{
		Claimant: "race-loop", Interval: 5 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)

	const parents = 3
	for attempt := 0; attempt < parents; attempt++ {
		_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "parent-work", uint(attempt+1), nil, "")
		if err != nil {
			t.Fatalf("attempt %d: start the parent action: %v", attempt, err)
		}
		parent := waitForJobState(t, ctx, canonical, "the parent action to finish", func(s jobs.Snapshot) bool {
			return s.State.Terminal()
		})
		if parent.State != jobs.StateSucceeded {
			t.Fatalf("attempt %d: the parent action ended %s (%+v): its mah.start_job was not the host's to run",
				attempt, parent.State, parent.Failure)
		}
	}

	closures := func() []jobs.Snapshot {
		page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
			jobs.Filter{Kinds: []string{JobKindPluginAction}}, jobs.Cursor{}, 50)
		if err != nil {
			t.Fatalf("list plugin jobs: %v", err)
		}
		found := make([]jobs.Snapshot, 0, parents)
		for _, job := range page.Jobs {
			if pluginActionSubtypeOf(job.Summary) == pluginActionSubtypeClosure {
				found = append(found, job)
			}
		}
		return found
	}

	waitFor(t, "every closure to finish", func() bool {
		found := closures()
		if len(found) < parents {
			return false
		}
		for _, job := range found {
			if !job.State.Terminal() {
				return false
			}
		}
		return true
	})

	owned := plugin_system.CurrentRuntimeIdentity().String()
	for _, job := range closures() {
		if job.State != jobs.StateSucceeded {
			t.Fatalf("closure %s ended %s: the dispatch loop took work only its host can run", job.ID, job.State)
		}
		if claim := storedClaim(t, ctx, job.ID); claim.Claimant != owned {
			t.Fatalf("closure %s was claimed by %q, want the process holding the callback (%q)",
				job.ID, claim.Claimant, owned)
		}
	}
}

// TestAPluginJobKeepsItsOwnTextOutOfDurableHistory is the redaction boundary for
// plugin background work.
//
// A Job's input is sealed, but its failure, its progress snapshots, its terminal
// event and the hook payload built from them are ordinary durable history: a
// handler whose validator saw `context.params.secret`, or an HTTP error carrying a
// signed URL, must not be able to write that value into a surface every authorized
// reader can search. What the Job records is a bounded host-owned classification —
// the same shape every other Kind records — and the plugin's own words, with the
// Job's parameter values replaced, stay out of anything a viewer can list.
func TestAPluginJobKeepsItsOwnTextOutOfDurableHistory(t *testing.T) {
	const secret = "top-secret-token-value"
	ctx := newPluginActionJobContext(t)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "leaky-work", 9,
		map[string]any{"secret": secret}, "")
	if err != nil {
		t.Fatalf("run the failing action: %v", err)
	}
	job := waitForJobState(t, ctx, canonical, "the action to fail", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateFailed {
		t.Fatalf("the action ended %s, want failed", job.State)
	}
	if job.Failure == nil {
		t.Fatalf("the failed action records no failure")
	}
	if job.Failure.Code != pluginActionFailureCode {
		t.Fatalf("the failure is classified %q, want %q", job.Failure.Code, pluginActionFailureCode)
	}
	if job.Failure.Message != pluginActionFailureMessage {
		t.Fatalf("the failure message is %q: it has to be host-owned text, not the handler's own",
			job.Failure.Message)
	}
	if strings.Contains(job.Failure.Message, secret) {
		t.Fatalf("the durable failure carries a parameter value: %q", job.Failure.Message)
	}
	if job.Failure.DiagnosticRef != "" {
		t.Fatalf("the failure points at a stored diagnostic %q: the plugin's text is not durable anywhere",
			job.Failure.DiagnosticRef)
	}

	assertNoSecretInJobSurfaces(t, ctx, canonical, secret)

	// The same rule for a successful run: the completion message and the progress
	// snapshot are as durable as a failure is.
	_, chatty, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "chatty-work", 9,
		map[string]any{"secret": secret}, "")
	if err != nil {
		t.Fatalf("run the chatty action: %v", err)
	}
	finished := waitForJobState(t, ctx, chatty, "the action to succeed", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the chatty action ended %s (%+v)", finished.State, finished.Failure)
	}
	assertNoSecretInJobSurfaces(t, ctx, chatty, secret)
}

// assertNoSecretInJobSurfaces reads every surface a viewer can list and refuses a
// job whose text carries the value.
func assertNoSecretInJobSurfaces(t *testing.T, ctx *MahresourcesContext, jobID, secret string) {
	t.Helper()
	job, err := ctx.GetJob(jobID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if strings.Contains(string(job.Summary), secret) {
		t.Fatalf("the summary carries the value: %s", job.Summary)
	}
	if strings.Contains(job.Progress.Message, secret) {
		t.Fatalf("the progress snapshot carries the value: %q", job.Progress.Message)
	}
	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID, 0, 200)
	if err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Detail), secret) {
			t.Fatalf("the %s event carries the value: %s", event.Type, event.Detail)
		}
	}
	page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
		jobs.Filter{Kinds: []string{JobKindPluginAction}}, jobs.Cursor{}, 50)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for _, listed := range page.Jobs {
		if strings.Contains(listed.Title, secret) || strings.Contains(string(listed.Summary), secret) {
			t.Fatalf("the listing carries the value: %s / %s", listed.Title, listed.Summary)
		}
	}
}

// TestAPluginRetryExistsOnlyWhereTheRegistrationDeclaresIt is §16's row for
// registered actions and scheduled occurrences: "Cancel or Retry only when
// registration explicitly declares support".
//
// The host cannot know whether arbitrary Lua is idempotent — this Kind's own
// contract says it is not — so a Retry is an author's declaration rather than a
// consequence of a Job having failed. Both halves matter: an ordinary
// side-effecting action must offer nothing, and an action that *does* declare it
// must lose the option the moment the registration that declared it is gone,
// because the declaration is current policy rather than a property of the record.
func TestAPluginRetryExistsOnlyWhereTheRegistrationDeclaresIt(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	failedAction := func(actionID string) jobs.Snapshot {
		t.Helper()
		_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, actionID, 4, nil, "")
		if err != nil {
			t.Fatalf("run %s: %v", actionID, err)
		}
		return waitForJobState(t, ctx, canonical, "the action to fail", func(s jobs.Snapshot) bool {
			return s.State.Terminal()
		})
	}

	// An action that says nothing about replay is not retryable, however it ended.
	ordinary := failedAction("failing-work")
	if ordinary.State != jobs.StateFailed {
		t.Fatalf("the ordinary action ended %s, want failed", ordinary.State)
	}
	if offersCommand(advertisedForTest(t, ctx, ordinary.ID), jobs.CommandRetry) {
		t.Fatalf("an action that never declared safe replay offered a Retry")
	}

	// An action that declares it is retryable, and stays so for as long as the
	// registration is there.
	declared := failedAction("retryable-work")
	if !offersCommand(advertisedForTest(t, ctx, declared.ID), jobs.CommandRetry) {
		t.Fatalf("an action declaring safe replay offered no Retry")
	}

	// Disabling the plugin removes the registration, and with it the declaration:
	// the option is current policy, not a property of the finished Job.
	if err := ctx.PluginManager().DisablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("disable the plugin: %v", err)
	}
	if offersCommand(advertisedForTest(t, ctx, declared.ID), jobs.CommandRetry) {
		t.Fatalf("a disabled plugin's action still offered a Retry")
	}
	if _, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: declared.ID, Key: jobs.CommandRetry, IdempotencyKey: "retry-disabled",
		ExpectedVersion: declared.Version, Actor: jobs.Access{Administrator: true},
	}); err == nil {
		t.Fatalf("a Retry was accepted for an action whose registration no longer exists")
	}
}

// TestADispatchedPluginExecutionWaitsForItsOwnReportNotForAClock pins the
// observer's contract: waiting for a plugin's report is not the same job as
// running it, and a wait that expires must never become the Job's failure.
//
// The executor does not report on the dispatcher's schedule. The manager accepts
// the work and runs it when a VM and a job slot are free, which can be behind
// another plugin's five-minute Lua call — its own job-slot wait is unbounded for
// exactly that reason. A wall-clock bound in the adapter therefore expires while
// the callback is still queued, and the runtime records `dispatch-failed` on a Job
// whose handler can still run and still mutate data. What the wait is bounded by
// is the execution's own lifecycle, and a cancelled runtime leaves the Job
// unresolved — with its claim and its lease — for reconciliation.
func TestADispatchedPluginExecutionWaitsForItsOwnReportNotForAClock(t *testing.T) {
	ctx := newPluginActionJobContext(t)

	// Every plugin job slot is taken, so the manager accepts this work and cannot
	// start it: the state a dispatched execution has to wait through.
	releaseBudget := ctx.PluginManager().FillJobBudgetForTest()
	defer releaseBudget()

	input, err := json.Marshal(pluginActionJobInput{
		Subtype:  pluginActionSubtypeRegistered,
		Plugin:   pluginActionTestPlugin,
		Action:   "async-work",
		EntityID: 3,
		Runtime:  plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion,
		JobID: accepted.ID, Claimant: plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil || !claimed {
		t.Fatalf("claim the accepted job: claimed=%v err=%v", claimed, err)
	}

	adapter := &pluginActionAdapter{ctx: ctx}
	dispatchCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adapter.Dispatch(dispatchCtx, execution) }()

	// The execution is waiting for a report that cannot come yet, and it is still
	// the execution's to wait for.
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the dispatch reported a failure for work that is still live: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dispatch outlived the runtime that owns it")
	}

	// Nothing was decided about the Job: it is still running, still owned by the
	// execution that is still holding the callback, and still occupying capacity.
	snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, accepted.ID)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if snap.State != jobs.StateRunning {
		t.Fatalf("the job is %s after its runtime stopped waiting, want running and unresolved", snap.State)
	}
	if snap.Failure != nil {
		t.Fatalf("the job recorded the failure %+v: a wait ending is not work failing", snap.Failure)
	}
	if claim := storedClaim(t, ctx, accepted.ID); claim.State != models.JobClaimStateHeld {
		t.Fatalf("the claim is %s, want held for reconciliation", claim.State)
	}
}

// TestAPluginOutcomeIsPublishedOnlyAfterItsCallbackReturns is the quiescence
// requirement of §3 applied to the one executor whose handler keeps running after
// it has spoken.
//
// mah.job_fail *requests* an outcome; the Lua that called it can go on sleeping,
// reading and writing. Publishing from inside the call ended the durable Job while
// its handler was still executing — handing the deployment's capacity back, and
// offering a Retry that another worker could start beside the callback that had not
// stopped. The outcome is therefore published when the callback returns, which is
// the only point at which the work is provably quiescent.
func TestAPluginOutcomeIsPublishedOnlyAfterItsCallbackReturns(t *testing.T) {
	ctx := newPluginActionJobContextWithDeploymentBudget(t, 2)

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "lingering-work", 4, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}

	// The handler has reported its failure and is still executing: the sleep it is
	// inside is the work that has not stopped.
	waitFor(t, "the handler to report failure and keep running", func() bool {
		return pluginKVForTest(t, ctx, "lingering") == "reported"
	})

	running, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, canonical)
	if err != nil {
		t.Fatalf("read the job: %v", err)
	}
	if running.State != jobs.StateRunning {
		t.Fatalf("the job is %s while its handler is still running, want running", running.State)
	}
	if claim := storedClaim(t, ctx, canonical); claim.State != models.JobClaimStateHeld {
		t.Fatalf("the claim is %s while the handler still runs, want held", claim.State)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the deployment budget holds %d slots for one running handler, want one", held)
	}
	// Retry is the control the premature outcome used to hand out: a Job that is
	// still running does not offer one, and its lineage has no leaf to retry.
	advertised, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(),
		jobs.Access{Administrator: true}, canonical)
	if err != nil {
		t.Fatalf("read the advertised commands: %v", err)
	}
	if offersCommand(advertised, jobs.CommandRetry) {
		t.Fatalf("a job whose handler is still executing offered a Retry")
	}
	if _, err := ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: canonical, Key: jobs.CommandRetry, IdempotencyKey: "lingering-retry",
		ExpectedVersion: running.Version, Actor: jobs.Access{Administrator: true},
	}); err == nil {
		t.Fatalf("a Retry was accepted for a job whose handler is still running")
	}

	// The callback returns, and only now is the requested outcome the Job's own.
	waitFor(t, "the handler to return", func() bool {
		return pluginKVForTest(t, ctx, "lingering") == "returned"
	})
	finished := waitForJobState(t, ctx, canonical, "the job to reach its outcome", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateFailed {
		t.Fatalf("the job ended %s (%+v), want the failure the handler requested", finished.State, finished.Failure)
	}
	if claim := storedClaim(t, ctx, canonical); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the handler returned, want released", claim.State)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the handler returned", held)
	}
}
