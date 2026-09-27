package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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
// instead; armed slow, it holds each one for a fixed time, or until its context
// is done if that comes first.
type readStall struct {
	match atomic.Pointer[func(*gorm.DB) bool]
	fail  atomic.Bool
	slow  atomic.Int64
	hits  atomic.Int64

	mu    sync.Mutex
	times []time.Time
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
		stall.mu.Lock()
		stall.times = append(stall.times, time.Now())
		stall.mu.Unlock()
		if stall.fail.Load() {
			_ = db.AddError(errors.New("database is locked"))
			return
		}
		hold := 5 * time.Second
		if slow := time.Duration(stall.slow.Load()); slow > 0 {
			hold = slow
		}
		select {
		case <-db.Statement.Context.Done():
		case <-time.After(hold):
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
	s.mu.Lock()
	s.times = nil
	s.mu.Unlock()
	s.fail.Store(fail)
	s.slow.Store(0)
	s.match.Store(&match)
}

func (s *readStall) armSlow(match func(*gorm.DB) bool, hold time.Duration) {
	s.arm(match, false)
	s.slow.Store(int64(hold))
}

func (s *readStall) disarm() { s.match.Store(nil) }

func (s *readStall) hitTimes() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.times...)
}

func readsTable(table string) func(*gorm.DB) bool {
	return func(db *gorm.DB) bool { return db.Statement.Table == table }
}

func readsSQL(fragment string) func(*gorm.DB) bool {
	return func(db *gorm.DB) bool { return strings.Contains(db.Statement.SQL.String(), fragment) }
}

func countTimelineEvents(t *testing.T, ctx *MahresourcesContext, jobID, eventType string) int {
	t.Helper()
	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID, 0, 500)
	if err != nil {
		t.Fatalf("read the timeline of %s: %v", jobID, err)
	}
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func timelineHasEvent(t *testing.T, ctx *MahresourcesContext, jobID, eventType string) bool {
	t.Helper()
	return countTimelineEvents(t, ctx, jobID, eventType) > 0
}

// setAdmissionBounds shortens the admission's bound and one recheck's bound for
// one test.
func setAdmissionBounds(t *testing.T, admission, recheck time.Duration) {
	t.Helper()
	savedAdmission, savedRecheck := pluginActionAdmissionAttempt, pluginActionRecheckAttempt
	pluginActionAdmissionAttempt, pluginActionRecheckAttempt = admission, recheck
	t.Cleanup(func() {
		pluginActionAdmissionAttempt, pluginActionRecheckAttempt = savedAdmission, savedRecheck
	})
}

// acceptRegisteredActionForTest accepts one queued async-work action as actor.
func acceptRegisteredActionForTest(t *testing.T, ctx *MahresourcesContext, actor uint, entityID uint) (jobs.Snapshot, *pluginActionJobInput) {
	t.Helper()
	input := &pluginActionJobInput{
		Subtype: pluginActionSubtypeRegistered, Plugin: pluginActionTestPlugin, Action: "async-work",
		EntityType: "resource", EntityID: entityID, Runtime: plugin_system.CurrentRuntimeIdentity().String(),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}
	owner := actor
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Async Work", OwnerUserID: &owner, ActorUserID: &owner,
		Replay: jobs.ReplayInput{Input: raw},
	})
	return accepted, input
}

// seedScopedActorForTest makes a user confined to a group holding one resource,
// and opens the action plugin to scoped principals, so the actor's re-check
// reads the account, the subtree, the plugin's scoped access and the target.
func seedScopedActorForTest(t *testing.T, ctx *MahresourcesContext, name string) (models.User, models.Resource) {
	t.Helper()
	scope := models.Group{Name: name + "-scope"}
	if err := ctx.db.Create(&scope).Error; err != nil {
		t.Fatalf("seed the scope group: %v", err)
	}
	target := models.Resource{Name: name + "-target.txt", OwnerId: &scope.ID}
	if err := ctx.db.Create(&target).Error; err != nil {
		t.Fatalf("seed the target: %v", err)
	}
	actor := models.User{Username: name, Role: models.RoleUser, PasswordHash: "x", ScopeGroupId: &scope.ID}
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
	return actor, target
}

func enableActionPluginForTest(t *testing.T, ctx *MahresourcesContext) *plugin_system.PluginManager {
	t.Helper()
	pm := ctx.PluginManager()
	if !pm.IsEnabled(pluginActionTestPlugin) {
		if err := pm.EnablePlugin(pluginActionTestPlugin); err != nil {
			t.Fatalf("enable %s: %v", pluginActionTestPlugin, err)
		}
	}
	return pm
}

// exerciseAReCheckThatCannotFinish drives one admission through each read its
// re-check of the acting principal makes, with that read stalled past the
// admission's bound or failing outright. The admission has already been granted
// its claim when the read is made, and holds the plugin's VM while it asks; it
// must answer within its bound, give the claim back so the Job is waiting again
// with no slot of the budget held, defer until a re-check without a claim
// answers, and then run. A read that could not answer is never recorded as a
// refusal.
func exerciseAReCheckThatCannotFinish(t *testing.T, ctx *MahresourcesContext) {
	setAdmissionBounds(t, 300*time.Millisecond, 2*time.Second)
	enableActionPluginForTest(t, ctx)
	actor, target := seedScopedActorForTest(t, ctx, "recheck-actor")
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
			accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, target.ID)
			admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)

			stall.arm(c.match, c.fail)
			started := time.Now()
			got := admission.Admit(time.Time{})
			elapsed := time.Since(started)
			stall.disarm()
			if stall.hits.Load() == 0 {
				t.Fatal("the admission never made the read: the test did not reach it")
			}
			if got != plugin_system.AdmitDeferred {
				t.Fatalf("an admission whose re-check could not finish answered %v, want deferred", got)
			}
			if elapsed > 3*time.Second {
				t.Fatalf("the admission took %v with the plugin's VM held: its bound did not reach the re-check", elapsed)
			}
			if ready, _ := admission.Recheck(context.Background()); !ready {
				t.Fatal("the re-check without a claim did not answer once the read did")
			}
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
	setAdmissionBounds(t, 300*time.Millisecond, 300*time.Millisecond)
	ctx := newJobHarnessContext(t, false)
	pm := enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "stalled-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	stall := installReadStall(t, ctx.db)

	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 3)
	owner := actor.ID
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

// TestAReCheckThatKeepsFailingGivesItsClaimBackOnce pins what a re-check that
// cannot answer for a long time costs. The claim is given back once, and never
// again: the Job's timeline holds one start however long the failure lasts. The
// re-checks made without a claim back off, and while they fail the Job is out of
// its plugin's lane, so the plugin's other work runs. Once the check answers,
// the Job runs.
func TestAReCheckThatKeepsFailingGivesItsClaimBackOnce(t *testing.T) {
	setAdmissionBounds(t, 300*time.Millisecond, 300*time.Millisecond)
	ctx := newJobHarnessContext(t, false)
	pm := enableActionPluginForTest(t, ctx)
	// Only the scoped actor's re-check reads the target, so only its Job fails.
	stuckActor, target := seedScopedActorForTest(t, ctx, "stuck-actor")
	otherActor := models.User{Username: "other-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&otherActor).Error; err != nil {
		t.Fatalf("seed the other actor: %v", err)
	}
	stall := installReadStall(t, ctx.db)
	stall.arm(readsTable("resources"), true)

	stuck, stuckInput := acceptRegisteredActionForTest(t, ctx, stuckActor.ID, target.ID)
	other, otherInput := acceptRegisteredActionForTest(t, ctx, otherActor.ID, 9)
	stuckOwner, otherOwner := stuckActor.ID, otherActor.ID
	if err := ctx.queueRegisteredPluginAction(pm, stuck.ID, "stuck-handle", &stuckOwner, stuckInput); err != nil {
		t.Fatalf("queue the stuck action: %v", err)
	}
	if err := ctx.queueRegisteredPluginAction(pm, other.ID, "other-handle", &otherOwner, otherInput); err != nil {
		t.Fatalf("queue the other action: %v", err)
	}

	waitForJobState(t, ctx, other.ID, "the plugin's other work to run behind a failing re-check", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateSucceeded
	})
	waitFor(t, "four re-checks", func() bool { return stall.hits.Load() >= 4 })
	times := stall.hitTimes()
	// The first read is the one under the claim and the second the first
	// re-check without one, straight after; from there each wait doubles.
	if gap := times[2].Sub(times[1]); gap < 900*time.Millisecond {
		t.Fatalf("the second re-check came %v after the first, want a backoff of about a second", gap)
	}
	if gap := times[3].Sub(times[2]); gap < 1800*time.Millisecond {
		t.Fatalf("the third re-check came %v after the second, want the backoff doubled", gap)
	}
	if started := countTimelineEvents(t, ctx, stuck.ID, jobs.EventStarted); started != 1 {
		t.Fatalf("a Job whose re-check kept failing was started %d times, want once", started)
	}
	if state := jobStateForTest(t, ctx, stuck.ID); state != jobs.StateQueued {
		t.Fatalf("the Job whose re-check keeps failing is %s, want queued", state)
	}

	stall.disarm()
	job := waitForJobState(t, ctx, stuck.ID, "the Job to run once its re-check answers", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateSucceeded {
		t.Fatalf("the Job ended %s, want succeeded", job.State)
	}
	if started := countTimelineEvents(t, ctx, stuck.ID, jobs.EventStarted); started != 2 {
		t.Fatalf("the Job was started %d times in all, want its one give-back and its run", started)
	}
}

// TestASlowReCheckStillRunsTheJob pins a check that answers, but only after
// longer than an admission may hold the plugin's VM. The re-check without a
// claim may wait for it; the claim made straight after it acts on that answer
// when the check under the claim runs out again, rather than giving the claim
// back a second time.
func TestASlowReCheckStillRunsTheJob(t *testing.T) {
	setAdmissionBounds(t, 300*time.Millisecond, 3*time.Second)
	ctx := newJobHarnessContext(t, false)
	pm := enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "slow-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	stall := installReadStall(t, ctx.db)
	stall.armSlow(readsTable("users"), 600*time.Millisecond)
	defer stall.disarm()

	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 4)
	owner := actor.ID
	if err := ctx.queueRegisteredPluginAction(pm, accepted.ID, "slow-handle", &owner, input); err != nil {
		t.Fatalf("queue the action: %v", err)
	}
	job := waitForJobState(t, ctx, accepted.ID, "the action to run despite its slow re-check", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if job.State != jobs.StateSucceeded {
		t.Fatalf("the action ended %s, want succeeded", job.State)
	}
	if started := countTimelineEvents(t, ctx, accepted.ID, jobs.EventStarted); started != 2 {
		t.Fatalf("the action was started %d times, want one give-back and its run", started)
	}
	if timelineHasEvent(t, ctx, accepted.ID, jobs.EventBlocked) {
		t.Fatal("a slow re-check was recorded as a refusal")
	}
}

// TestAnOldReCheckAnswerIsNotActedOn pins the freshness of the answer a claim may
// act on. An answer from an earlier turn says what the account could do then; the
// account is disabled after it, and the claim that follows must not run the Job
// on it even though the check under that claim cannot answer. Only an answer
// taken again, just before the claim, is acted on, and it refuses.
func TestAnOldReCheckAnswerIsNotActedOn(t *testing.T) {
	setAdmissionBounds(t, 300*time.Millisecond, 2*time.Second)
	ctx := newJobHarnessContext(t, false)
	enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "changing-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	stall := installReadStall(t, ctx.db)
	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 6)
	admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)

	stall.arm(readsTable("users"), true)
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitDeferred {
		t.Fatalf("an admission whose re-check failed answered %v, want deferred", got)
	}
	stall.disarm()
	if ready, _ := admission.Recheck(context.Background()); !ready {
		t.Fatal("the re-check without a claim did not answer")
	}

	if err := ctx.db.Model(&models.User{}).Where("id = ?", actor.ID).Update("disabled", true).Error; err != nil {
		t.Fatalf("disable the account: %v", err)
	}
	time.Sleep(pluginActionAdmissionAttempt + 100*time.Millisecond)

	stall.arm(readsTable("users"), true)
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitDeferred {
		t.Fatalf("an admission holding only an old answer said %v, want deferred without a claim", got)
	}
	if started := countTimelineEvents(t, ctx, accepted.ID, jobs.EventStarted); started != 1 {
		t.Fatalf("an old answer was claimed on: the Job was started %d times", started)
	}
	stall.disarm()
	if ready, _ := admission.Recheck(context.Background()); !ready {
		t.Fatal("the fresh re-check did not answer")
	}
	stall.arm(readsTable("users"), true)
	got := admission.Admit(time.Time{})
	stall.disarm()
	if got != plugin_system.AdmitWithdrawn {
		t.Fatalf("a claim made on a fresh refusal answered %v, want withdrawn", got)
	}
	waitFor(t, "the refusal to be recorded", func() bool {
		return jobStateForTest(t, ctx, accepted.ID) == jobs.StateBlocked
	})
	if got := pluginKVForTest(t, ctx, "ran"); got != "" {
		t.Fatalf("the Job of a disabled account ran %q times", got)
	}
}

// TestADeferredAdmissionClaimsNothingUntilItsClaimIsBack pins the order between a
// claim given back and the next claim. Until the Job is waiting again it is
// running under the returned token, and a claim asked for then would read it as
// work another execution owns; the re-check without a claim waits for the claim
// to be back before it answers, and nothing is claimed before it has.
func TestADeferredAdmissionClaimsNothingUntilItsClaimIsBack(t *testing.T) {
	setAdmissionBounds(t, 200*time.Millisecond, 2*time.Second)
	ctx := newJobHarnessContext(t, false)
	enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "returning-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 5)
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
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitDeferred {
		t.Fatalf("an admission whose re-check could not read answered %v, want deferred", got)
	}
	stall.disarm()

	if !admission.stillReturning() {
		t.Fatal("the claim came back although its first release failed: the test did not reach the retry")
	}
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitDeferred {
		t.Fatalf("asking while the claim was still coming back answered %v, want deferred", got)
	}
	if started := countTimelineEvents(t, ctx, accepted.ID, jobs.EventStarted); started != 1 {
		t.Fatalf("the Job was claimed %d times while its claim was coming back", started)
	}
	if ready, _ := admission.Recheck(context.Background()); !ready {
		t.Fatal("the re-check did not answer once the claim was back")
	}
	if admission.stillReturning() {
		t.Fatal("the re-check answered before the claim was back")
	}
	if got := admission.Admit(time.Time{}); got != plugin_system.Admitted {
		t.Fatalf("asking once the claim was back answered %v, want admitted", got)
	}
	if err := admission.Completed("done", nil); err != nil {
		t.Fatalf("report the outcome: %v", err)
	}
}

// TestAClaimWhoseInputCannotBeReadRunsWithTheQueuedInput pins the one read after
// a claim that is not a re-check: the sealed input. The admission holds the same
// input in memory, as it was sealed or opened, so a read that fails is not a
// reason to give the claim back, which an admission may do only once.
func TestAClaimWhoseInputCannotBeReadRunsWithTheQueuedInput(t *testing.T) {
	setAdmissionBounds(t, 300*time.Millisecond, 2*time.Second)
	ctx := newJobHarnessContext(t, false)
	enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "input-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 8)
	admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)

	stall := installReadStall(t, ctx.db)
	stall.arm(readsTable("job_replay_envelopes"), true)
	got := admission.Admit(time.Time{})
	stall.disarm()
	if stall.hits.Load() == 0 {
		t.Fatal("the claim never read its input: the test did not reach it")
	}
	if got != plugin_system.Admitted {
		t.Fatalf("a claim whose input read failed answered %v, want admitted with the queued input", got)
	}
	if started := countTimelineEvents(t, ctx, accepted.ID, jobs.EventStarted); started != 1 {
		t.Fatalf("the Job was started %d times, want once", started)
	}
	if err := admission.Completed("done", nil); err != nil {
		t.Fatalf("report the outcome: %v", err)
	}
}

// TestAnOccurrenceWhoseClaimIsStillComingBackIsWithdrawnWhenItLands pins the
// scheduler's side of a claim given back. The occurrence's dispatch budget runs
// out while the claim is still on its way back to the queue; the occurrence did
// not start, and once the claim lands it is withdrawn, not left waiting for a
// scheduler that has moved on.
func TestAnOccurrenceWhoseClaimIsStillComingBackIsWithdrawnWhenItLands(t *testing.T) {
	setAdmissionBounds(t, 200*time.Millisecond, 2*time.Second)
	ctx := newJobHarnessContext(t, false)
	pm := enableActionPluginForTest(t, ctx)
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

// TestAnOccurrenceAdmittedTooLateIsWithdrawnEvenIfTheFirstWriteFails pins the
// claim a late admission leaves behind. The scheduler has stopped waiting, so
// the occurrence does not start, and its claim is heartbeated until the
// withdrawal lands: a withdrawal dropped on one failed write would leave it
// running, holding a slot of the budget, for good.
func TestAnOccurrenceAdmittedTooLateIsWithdrawnEvenIfTheFirstWriteFails(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	occurrence := acceptOccurrenceForTest(t, ctx)
	admission := ctx.newPluginActionAdmission(occurrence.ID,
		&pluginActionJobInput{Subtype: pluginActionSubtypeScheduled, Plugin: pluginActionTestPlugin}, nil)
	if got := admission.Admit(time.Time{}); got != plugin_system.Admitted {
		t.Fatalf("admit the occurrence: %v", got)
	}

	var failures atomic.Int64
	failures.Store(1)
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("fail-first-withdrawal", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateCancelled) {
			return
		}
		if failures.Add(-1) >= 0 {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register the failing withdrawal: %v", err)
	}

	run, err := ctx.settleUnstartedOccurrence(admission)
	if err != nil {
		t.Fatalf("settle the occurrence: %v", err)
	}
	if run.Started {
		t.Fatal("an occurrence admitted too late was reported as started")
	}
	waitFor(t, "the late occurrence to be withdrawn", func() bool {
		return jobStateForTest(t, ctx, occurrence.ID) == jobs.StateCancelled
	})
	if failures.Load() >= 0 {
		t.Fatal("the injected failure was not consumed: the test did not reach the retry")
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the withdrawn occurrence still holds %d slots", held)
	}
}

// TestAClaimWhoseInputCannotBeOpenedIsBlockedFromTheAdmission pins the block of an
// input that cannot be opened when the control plane could not write it within
// the admission's bound: the admission holds the claim, so it records the block
// itself, after the VM is released, rather than leaving the Job running.
func TestAClaimWhoseInputCannotBeOpenedIsBlockedFromTheAdmission(t *testing.T) {
	setAdmissionBounds(t, 300*time.Millisecond, 2*time.Second)
	ctx := newJobHarnessContext(t, false)
	enableActionPluginForTest(t, ctx)
	actor := models.User{Username: "unopenable-actor", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&actor).Error; err != nil {
		t.Fatalf("seed the actor: %v", err)
	}
	accepted, input := acceptRegisteredActionForTest(t, ctx, actor.ID, 10)
	if err := ctx.db.Where("job_id = ?", accepted.ID).Delete(&models.JobReplayEnvelope{}).Error; err != nil {
		t.Fatalf("lose the sealed input: %v", err)
	}
	var stalled atomic.Bool
	if err := ctx.db.Callback().Update().Before("gorm:update").Register("stall-the-first-block", func(db *gorm.DB) {
		updates, ok := db.Statement.Dest.(map[string]any)
		if !ok || db.Statement.Table != "jobs" || updates["state"] != string(jobs.StateBlocked) {
			return
		}
		if stalled.CompareAndSwap(false, true) {
			<-db.Statement.Context.Done()
		}
	}); err != nil {
		t.Fatalf("register the stalled block: %v", err)
	}

	admission := ctx.newPluginActionAdmission(accepted.ID, input, ctx.registeredActionRefusal)
	if got := admission.Admit(time.Time{}); got != plugin_system.AdmitWithdrawn {
		t.Fatalf("a claim whose input cannot be opened answered %v, want withdrawn", got)
	}
	if !stalled.Load() {
		t.Fatal("the control plane never tried to block the Job: the test did not reach it")
	}
	waitFor(t, "the admission to block the Job", func() bool {
		return jobStateForTest(t, ctx, accepted.ID) == jobs.StateBlocked
	})
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the blocked Job still holds %d slots", held)
	}
}
