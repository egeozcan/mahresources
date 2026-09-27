package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

// readStall holds up the reads it is armed for: each one waits until its own
// statement's context is done, so a bounded read returns when its bound does
// and an unbounded one only after five seconds. Armed to fail, it fails them
// instead.
type readStall struct {
	match atomic.Pointer[func(*gorm.DB) bool]
	fail  atomic.Bool
	hits  atomic.Int64
}

func installReadStall(t *testing.T, db *gorm.DB) *readStall {
	t.Helper()
	stall := &readStall{}
	hook := func(db *gorm.DB) {
		match := stall.match.Load()
		if match == nil || !(*match)(db) {
			return
		}
		stall.hits.Add(1)
		if stall.fail.Load() {
			_ = db.AddError(errors.New("database is locked"))
			return
		}
		select {
		case <-db.Statement.Context.Done():
		case <-time.After(5 * time.Second):
		}
	}
	if err := db.Callback().Query().Before("gorm:query").Register("recheck-stall-query", hook); err != nil {
		t.Fatalf("register the query stall: %v", err)
	}
	// A raw query read through Scan runs the row chain, not the query chain.
	if err := db.Callback().Row().Before("gorm:row").Register("recheck-stall-row", hook); err != nil {
		t.Fatalf("register the row stall: %v", err)
	}
	return stall
}

func (s *readStall) arm(match func(*gorm.DB) bool, fail bool) {
	s.hits.Store(0)
	s.fail.Store(fail)
	s.match.Store(&match)
}

func (s *readStall) disarm() { s.match.Store(nil) }

func readsTable(table string) func(*gorm.DB) bool {
	return func(db *gorm.DB) bool { return db.Statement.Table == table }
}

func readsSQL(fragment string) func(*gorm.DB) bool {
	return func(db *gorm.DB) bool { return strings.Contains(db.Statement.SQL.String(), fragment) }
}

func timelineHasEvent(t *testing.T, ctx *MahresourcesContext, jobID, eventType string) bool {
	t.Helper()
	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID, 0, 100)
	if err != nil {
		t.Fatalf("read the timeline of %s: %v", jobID, err)
	}
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

// exerciseAReCheckThatCannotFinish drives one admission through each read its
// re-check of the acting principal makes, with that read stalled past the
// admission's bound or failing outright. The admission has already been granted
// its claim when the read is made, and holds the plugin's VM while it asks; it
// must answer "later" within its bound, give the claim back so the Job is waiting
// again with no slot of the budget held, and then run once the read answers. A
// read that could not answer is never recorded as a refusal.
func exerciseAReCheckThatCannotFinish(t *testing.T, ctx *MahresourcesContext) {
	saved := pluginActionAdmissionAttempt
	pluginActionAdmissionAttempt = 300 * time.Millisecond
	t.Cleanup(func() { pluginActionAdmissionAttempt = saved })

	pm := ctx.PluginManager()
	if !pm.IsEnabled(pluginActionTestPlugin) {
		if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
			t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
		}
	}
	scope := models.Group{Name: "recheck-scope"}
	if err := ctx.db.Create(&scope).Error; err != nil {
		t.Fatalf("seed the scope group: %v", err)
	}
	target := models.Resource{Name: "recheck-target.txt", OwnerId: &scope.ID}
	if err := ctx.db.Create(&target).Error; err != nil {
		t.Fatalf("seed the target: %v", err)
	}
	// A scoped actor, so the re-check reads the account, the subtree, the
	// plugin's scoped access and the target.
	actor := models.User{Username: "recheck-actor", Role: models.RoleUser, PasswordHash: "x", ScopeGroupId: &scope.ID}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	if err := ctx.SetPluginScopedAccess(pluginActionTestPlugin, true); err != nil {
		t.Fatalf("open the plugin to scoped principals: %v", err)
	}
	// Scoped access is read from the deployment's record of the plugin, which the
	// plugin manager alone does not write.
	if err := ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", pluginActionTestPlugin).
		Update("enabled", true).Error; err != nil {
		t.Fatalf("record the plugin as enabled: %v", err)
	}
	stall := installReadStall(t, ctx.db)

	cases := []struct {
		name  string
		fail  bool
		match func(*gorm.DB) bool
	}{
		{name: "the account stalls", match: readsTable("users")},
		{name: "the account fails", fail: true, match: readsTable("users")},
		{name: "the subtree stalls", match: readsSQL("WITH RECURSIVE tree")},
		{name: "the plugin's scoped access stalls", match: readsTable("plugin_states")},
		{name: "the target stalls", match: readsTable("resources")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := &pluginActionJobInput{
				Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
				EntityType: "resource", EntityID: target.ID, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("encode the input: %v", err)
			}
			owner := actor.ID
			accepted := acceptJobFor(t, ctx, jobs.Acceptance{
				Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
				Origin: "api", Title: "Async Work", OwnerUserID: &owner, ActorUserID: &owner,
				Replay: jobs.ReplayInput{Input: raw},
			})
			admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)

			stall.arm(c.match, c.fail)
			started := time.Now()
			got := admission.Admit(time.Time{})
			elapsed := time.Since(started)
			stall.disarm()
			if stall.hits.Load() == 0 {
				t.Fatal("the admission never made the read: the test did not reach it")
			}
			if got != plugin_system.AdmitLater {
				t.Fatalf("an admission whose re-check could not finish answered %v, want later", got)
			}
			if elapsed > 3*time.Second {
				t.Fatalf("the admission took %v with the plugin's VM held: its bound did not reach the re-check", elapsed)
			}
			waitFor(t, "the claim to be given back", func() bool { return !admission.stillReturning() })
			if state := jobStateForTest(t, ctx, accepted.ID); state != jobs.StateQueued {
				t.Fatalf("a Job whose claim was given back is %s, want queued", state)
			}
			if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
				t.Fatalf("a Job given back to the queue still holds %d slots of the budget", held)
			}

			if got := admission.Admit(time.Time{}); got != plugin_system.Admitted {
				t.Fatalf("the admission once the read answered said %v, want admitted", got)
			}
			if err := admission.Completed("done", nil); err != nil {
				t.Fatalf("report the outcome: %v", err)
			}
			waitFor(t, "the Job to succeed", func() bool {
				return jobStateForTest(t, ctx, accepted.ID) == jobs.StateSucceeded
			})
			if timelineHasEvent(t, ctx, accepted.ID, jobs.EventBlocked) {
				t.Fatal("a re-check that could not read was recorded as a refusal")
			}
		})
	}
}

// TestAReCheckThatCannotFinishGivesTheClaimBack pins the bound on everything an
// admission does after its claim, with the plugin's VM held.
func TestAReCheckThatCannotFinishGivesTheClaimBack(t *testing.T) {
	exerciseAReCheckThatCannotFinish(t, newJobHarnessContext(t, false))
}

// TestAStalledReCheckHoldsNeitherTheVMNorASlot drives the same stall through the
// plugin's lane: while the re-check keeps stalling, the lane's head must keep
// giving the plugin's VM and its job slot back, so the plugin's hooks and pages,
// and other plugins' work, are never held behind it.
func TestAStalledReCheckHoldsNeitherTheVMNorASlot(t *testing.T) {
	saved := pluginActionAdmissionAttempt
	pluginActionAdmissionAttempt = 300 * time.Millisecond
	defer func() { pluginActionAdmissionAttempt = saved }()

	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	actor := models.User{Username: "stalled-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	stall := installReadStall(t, ctx.db)

	input := &pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
		EntityType: "resource", EntityID: 3, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	owner := actor.ID
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Async Work", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})
	stall.arm(readsTable("users"), false)
	if err := ctx.queueRegisteredPluginAction(pm, accepted.ID, "stalled-handle", &owner, input); err != nil {
		t.Fatalf("queue the action: %v", err)
	}
	waitFor(t, "the re-check to stall", func() bool { return stall.hits.Load() > 0 })

	_, L, err := pm.FindAction(pluginActionTestPlugin, "async-work")
	if err != nil {
		t.Fatalf("find the action: %v", err)
	}
	vm, _ := pm.TryLockVMWithin(context.Background(), L, 3*time.Second)
	if vm == nil {
		stall.disarm()
		t.Fatal("the plugin's VM stayed held past the admission's bound")
	}
	filled := make(chan func(), 1)
	go func() { filled <- pm.FillJobBudgetForTest() }()
	select {
	case release := <-filled:
		release()
	case <-time.After(3 * time.Second):
		stall.disarm()
		vm.Unlock()
		t.Fatal("a job slot stayed held by an admission that was not running anything")
	}
	stall.disarm()
	vm.Unlock()

	waitForJobState(t, ctx, accepted.ID, "the action to run once the re-check answers", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateSucceeded
	})
	if got := pluginKVForTest(t, ctx, "ran"); got != "1" {
		t.Fatalf("the handler ran %q times, want once", got)
	}
	if timelineHasEvent(t, ctx, accepted.ID, jobs.EventBlocked) {
		t.Fatal("a stalled re-check was recorded as a refusal")
	}
}

// TestAnOccurrenceWhoseClaimIsStillComingBackIsWithdrawnWhenItLands pins the
// scheduler's side of a claim given back. The occurrence's dispatch budget runs
// out while the claim is still on its way back to the queue; the occurrence did
// not start, and once the claim lands it is withdrawn, not left waiting for a
// scheduler that has moved on.
func TestAnOccurrenceWhoseClaimIsStillComingBackIsWithdrawnWhenItLands(t *testing.T) {
	saved := pluginActionAdmissionAttempt
	pluginActionAdmissionAttempt = 200 * time.Millisecond
	defer func() { pluginActionAdmissionAttempt = saved }()

	ctx := newJobHarnessContext(t, false)
	pm := ctx.PluginManager()
	if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	operator := models.User{Username: "occurrence-operator", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&operator).Error; err != nil {
		t.Fatalf("seed the operator: %v", err)
	}
	reg, found := ctx.pluginScheduleRegistration(pm, pluginActionTestPlugin, "tick")
	if !found {
		t.Fatal("the fixture's tick schedule is not registered")
	}
	input := &pluginActionJobInput{
		Subtype: pluginActionSubtypeScheduled, Plugin: pluginActionTestPlugin, ScheduleID: "tick",
		Overlap: plugin_system.ScheduleOverlapSkip, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	owner := operator.ID
	occurrence := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "schedule", Title: "tick", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})

	var releaseFailures atomic.Int64
	releaseFailures.Store(2)
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("fail-first-releases", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateQueued) {
			return
		}
		if releaseFailures.Add(-1) >= 0 {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register the failing release: %v", err)
	}
	stall := installReadStall(t, ctx.db)
	stall.arm(readsTable("users"), false)

	run, err := ctx.runQueuedScheduledOccurrence(pm, occurrence.ID, reg, operator.ID, input, 400*time.Millisecond)
	stall.disarm()
	if err != nil {
		t.Fatalf("run the occurrence: %v", err)
	}
	if run.Started {
		t.Fatal("an occurrence whose re-check never finished was reported as started")
	}
	if stall.hits.Load() == 0 {
		t.Fatal("the occurrence never reached its re-check: the test did not reach the stall")
	}

	waitFor(t, "the occurrence to be withdrawn once its claim is back", func() bool {
		return jobStateForTest(t, ctx, occurrence.ID) == jobs.StateCancelled
	})
	if releaseFailures.Load() >= 0 {
		t.Fatalf("the injected release failures were not all consumed (%d left): the claim was not still coming back", releaseFailures.Load())
	}
	if got := lastEventType(t, ctx, occurrence.ID); got != pluginActionNotStartedEvent {
		t.Fatalf("the withdrawn occurrence's last event is %q, want %q", got, pluginActionNotStartedEvent)
	}
	if got := pluginKVForTest(t, ctx, "scheduled"); got != "" {
		t.Fatalf("the schedule handler ran %q times", got)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the withdrawn occurrence still holds %d slots", held)
	}
}

// TestAnAdmissionAsksNothingWhileItsClaimIsComingBack pins the order between a
// claim given back and the next question. Until the Job is waiting again it is
// running under the returned token, and a claim asked for then would read it as
// work another execution owns: the lane would drop an execution whose Job is on
// its way back to the queue, and a closure's Job would be left waiting for a
// callback nobody holds.
func TestAnAdmissionAsksNothingWhileItsClaimIsComingBack(t *testing.T) {
	saved := pluginActionAdmissionAttempt
	pluginActionAdmissionAttempt = 200 * time.Millisecond
	defer func() { pluginActionAdmissionAttempt = saved }()

	ctx := newJobHarnessContext(t, false)
	if err := ctx.PluginManager().EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
	}
	actor := models.User{Username: "returning-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	input := &pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
		EntityType: "resource", EntityID: 5, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	owner := actor.ID
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Async Work", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})
	admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)

	var releaseFailures atomic.Int64
	releaseFailures.Store(1)
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("fail-first-release", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateQueued) {
			return
		}
		if releaseFailures.Add(-1) >= 0 {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register the failing release: %v", err)
	}
	stall := installReadStall(t, ctx.db)
	stall.arm(readsTable("users"), true)
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitLater {
		t.Fatalf("an admission whose re-check could not read answered %v, want later", got)
	}
	stall.disarm()

	if !admission.stillReturning() {
		t.Fatal("the claim came back although its first release failed: the test did not reach the retry")
	}
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitLater {
		t.Fatalf("asking while the claim was still coming back answered %v, want later", got)
	}
	waitFor(t, "the claim to come back", func() bool { return !admission.stillReturning() })
	if got := admission.Admit(time.Time{}); got != plugin_system.Admitted {
		t.Fatalf("asking once the claim was back answered %v, want admitted", got)
	}
	if err := admission.Completed("done", nil); err != nil {
		t.Fatalf("report the outcome: %v", err)
	}
}
