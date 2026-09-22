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
           capabilities = { "actions", "jobs", "kv", "schedule" } }

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

function closure_work(job_id)
    mah.kv.set("closure", "ran")
    mah.job_complete(job_id, { message = "closure done" })
end

function parent_work(ctx)
    local child = mah.start_job("child work", closure_work)
    mah.kv.set("child", child)
    mah.job_complete(ctx.job_id, { message = "parent done" })
end

function burst_work(ctx)
    for i = 1, 400 do
        mah.job_progress(ctx.job_id, i % 100, string.rep("x", 900))
    end
    mah.job_complete(ctx.job_id, { message = "burst done" })
end

function init()
    mah.action({ id = "async-work", label = "Async Work", entity = "resource", async = true,
                 params = { {name = "note", type = "text", label = "Note"} },
                 handler = async_work })
    mah.action({ id = "failing-work", label = "Failing Work", entity = "resource", async = true,
                 handler = failing_work })
    mah.action({ id = "parent-work", label = "Parent Work", entity = "resource", async = true,
                 handler = parent_work })
    mah.action({ id = "burst-work", label = "Burst Work", entity = "resource", async = true,
                 handler = burst_work })
    mah.schedule({ id = "tick", every = "1m", overlap = "skip", handler = function(job_id)
        bump("scheduled")
    end })
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

	_, canonical, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "failing-work", 4, nil, "")
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

	// A runtime identity from another host: unprovable, so the Job must be
	// blocked rather than interrupted or redispatched.
	foreign := acceptClosureJobForTest(t, ctx, "another-host/boot-1/4242")
	claimJobForTest(t, ctx, foreign.ID)
	expireAndReconcile(t, ctx)
	if got := jobStateForTest(t, ctx, foreign.ID); got != jobs.StateBlocked {
		t.Fatalf("a job whose runtime is uninspectable is %s, want blocked", got)
	}

	// The same host, a boot session this machine is not in: that process cannot
	// exist any more, so the callback is proved gone.
	gone := acceptClosureJobForTest(t, ctx, plugin_system.CurrentRuntimeIdentity().Host+"/boot-that-ended/4243")
	claimJobForTest(t, ctx, gone.ID)
	expireAndReconcile(t, ctx)
	if got := jobStateForTest(t, ctx, gone.ID); got != jobs.StateInterrupted {
		t.Fatalf("a job whose runtime is proved gone is %s, want interrupted", got)
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
