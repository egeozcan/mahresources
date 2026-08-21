package plugin_system

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/auth"

	lua "github.com/yuin/gopher-lua"
)

// A plugin write that fires a hook re-entering a VM already on the call chain
// used to deadlock permanently: the VM mutex is not reentrant, the 5s Lua
// timeout cannot preempt a block inside a Go call, and the lock is never
// released. Every test here therefore runs under a hard deadline and *fails*
// rather than hanging, or a regression wedges the suite instead of reporting.
const reentryTestTimeout = 20 * time.Second

// hookRecorder is the shared observation point. Bound clones of hookFiringDB all
// write here, so assertions read one object.
type hookRecorder struct {
	mu          sync.Mutex
	probes      []uint
	writes      []string
	actors      []uint
	probeActors map[uint]uint
}

func (r *hookRecorder) probe(id, actor uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probes = append(r.probes, id)
	if r.probeActors == nil {
		r.probeActors = map[uint]uint{}
	}
	r.probeActors[id] = actor
}

// actorForProbe returns the actor bound for the call that looked up id.
func (r *hookRecorder) actorForProbe(id uint) (uint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.probeActors[id]
	return a, ok
}

func (r *hookRecorder) write(kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, kind)
}

func (r *hookRecorder) actor(a uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actors = append(r.actors, a)
}

func (r *hookRecorder) snapshot() (probes []uint, writes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	probes = append(probes, r.probes...)
	writes = append(writes, r.writes...)
	sort.Slice(probes, func(i, j int) bool { return probes[i] < probes[j] })
	return probes, writes
}

func (r *hookRecorder) lastActor() (uint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.actors) == 0 {
		return 0, false
	}
	return r.actors[len(r.actors)-1], true
}

// hookFiringDB models what application_context does on a write: it dispatches
// the entity's after-hook, passing the invocation it was bound with. That
// forwarding is the whole mechanism under test — plugin_system builds the
// Invocation, hands it to the binder, and gets it back when the write it caused
// fires a hook.
type hookFiringDB struct {
	mockQuerier
	stubWriter

	pm  *PluginManager
	rec *hookRecorder
	inv *Invocation
}

func (d *hookFiringDB) BindInvocation(inv *Invocation) (EntityQuerier, EntityWriter) {
	d.rec.actor(inv.ActorUserID)
	bound := &hookFiringDB{pm: d.pm, rec: d.rec, inv: inv}
	return bound, bound
}

func (d *hookFiringDB) CreateGroup(map[string]any) (map[string]any, error) {
	d.rec.write("group")
	d.pm.RunAfterHooks(d.inv, "after_group_create", map[string]any{"id": float64(7)})
	return map[string]any{"id": float64(7)}, nil
}

func (d *hookFiringDB) CreateNote(map[string]any) (map[string]any, error) {
	d.rec.write("note")
	d.pm.RunAfterHooks(d.inv, "after_note_create", map[string]any{"id": float64(9)})
	return map[string]any{"id": float64(9)}, nil
}

// GetTagData is how a Lua hook reports that it ran: it looks up a tag id unique
// to that hook.
func (d *hookFiringDB) GetTagData(id uint) (map[string]any, error) {
	var actor uint
	if d.inv != nil {
		actor = d.inv.ActorUserID
	}
	d.rec.probe(id, actor)
	return map[string]any{"id": float64(id), "name": "probe"}, nil
}

// reentryFixture wires a set of plugins to a hook-firing fake DB and renders the
// "page_bottom" injection slot the trigger plugin registers.
type reentryFixture struct {
	pm  *PluginManager
	rec *hookRecorder
}

func newReentryFixture(t *testing.T, plugins map[string]string) *reentryFixture {
	t.Helper()
	dir := t.TempDir()
	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writePlugin(t, dir, name, plugins[name])
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	// Close acquires every VM lock, so on a regression it blocks on the wedged
	// one forever and the whole binary dies at the Go test timeout instead of
	// printing why. Bound it: the test has already reported the deadlock, and a
	// lock that is never released cannot be cleaned up by anyone.
	t.Cleanup(func() {
		closed := make(chan struct{})
		go func() { pm.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Log("PluginManager.Close did not return: a VM lock is wedged (see the failure above)")
		}
	})

	rec := &hookRecorder{}
	db := &hookFiringDB{pm: pm, rec: rec}
	pm.SetEntityQuerier(db)
	pm.SetEntityWriter(db)
	pm.SetPrincipalBinder(db)

	for _, name := range names {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}
	return &reentryFixture{pm: pm, rec: rec}
}

// renderOrTimeout renders the trigger slot, failing the test if the render has
// not returned in time. A wedged VM lock is never released, so the leaked
// goroutine is unavoidable; failing loudly instead of hanging is the point.
func (f *reentryFixture) renderOrTimeout(t *testing.T, ctx context.Context) string {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- f.pm.RenderSlot(ctx, "page_bottom", map[string]any{}, nil) }()
	select {
	case out := <-done:
		return out
	case <-time.After(reentryTestTimeout):
		t.Fatalf("render did not return within %s: a hook dispatch re-entered a VM "+
			"already on the call chain and deadlocked on its own VM mutex", reentryTestTimeout)
		return ""
	}
}

func triggerPlugin(name, body string) string {
	return fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "trigger" }
function init()
    mah.inject("page_bottom", function(ctx)
%s
        return "rendered"
    end)
end
`, name, body)
}

func hookPlugin(name, event, body string) string {
	return fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "hooks %s" }
function init()
    mah.on(%q, function(data)
%s
        return data
    end)
end
`, name, event, event, body)
}

// Case 1 — self. A plugin that hooks an event and writes the entity that fires
// it. The write must still happen; only the notification is dropped.
func TestHookReentry_SelfHookedWriteCompletes(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "writes what it hooks" }
function init()
    mah.on("after_note_create", function(data)
        mah.db.get_tag(1)
        return data
    end)
    mah.inject("page_bottom", function(ctx)
        mah.db.create_note({ name = "n" })
        return "rendered"
    end)
end
`,
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q, want %q", out, "rendered")
	}

	probes, writes := f.rec.snapshot()
	if len(writes) != 1 || writes[0] != "note" {
		t.Errorf("writes = %v, want exactly one note write: the write must still happen", writes)
	}
	if len(probes) != 0 {
		t.Errorf("probes = %v, want none: p1 must not be notified of a write it made itself", probes)
	}
}

// Case 2 — mutual. The case the original report missed and a single-state guard
// would still deadlock on: P writes something Q hooks, Q's hook writes something
// P hooks, arriving back at a lock P's outer frame still holds.
func TestHookReentry_MutualCycleCompletes(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "writes group, hooks note" }
function init()
    mah.on("after_note_create", function(data)
        mah.db.get_tag(1)
        return data
    end)
    mah.inject("page_bottom", function(ctx)
        mah.db.create_group({ name = "g" })
        return "rendered"
    end)
end
`,
		"p2": hookPlugin("p2", "after_group_create", `
        mah.db.get_tag(2)
        mah.db.create_note({ name = "from-hook" })`),
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q, want %q", out, "rendered")
	}

	probes, writes := f.rec.snapshot()
	if len(writes) != 2 || writes[0] != "group" || writes[1] != "note" {
		t.Errorf("writes = %v, want [group note]: both writes must happen", writes)
	}
	// p2's hook ran (probe 2). p1's after_note_create did not (no probe 1):
	// p1 is still executing its injection two frames up.
	if len(probes) != 1 || probes[0] != 2 {
		t.Errorf("probes = %v, want [2]: p2's hook must run and p1's must be skipped", probes)
	}
}

// Case 3 — cross-plugin, no cycle. The case the guard must NOT swallow: a
// different plugin's hook on a plugin's write still fires.
func TestHookReentry_CrossPluginHookStillFires(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": triggerPlugin("p1", `        mah.db.create_note({ name = "n" })`),
		"p2": hookPlugin("p2", "after_note_create", `        mah.db.get_tag(2)`),
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q, want %q", out, "rendered")
	}

	probes, _ := f.rec.snapshot()
	if len(probes) != 1 || probes[0] != 2 {
		t.Errorf("probes = %v, want [2]: a hook on another plugin's write must still fire", probes)
	}
}

// Case 4 — depth 2, no cycle. Proves the guard skips only what is genuinely on
// the chain rather than everything below the first plugin frame.
func TestHookReentry_DepthTwoWithoutCycleStillFires(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": triggerPlugin("p1", `        mah.db.create_group({ name = "g" })`),
		"p2": hookPlugin("p2", "after_group_create", `
        mah.db.get_tag(2)
        mah.db.create_note({ name = "from-hook" })`),
		"p3": hookPlugin("p3", "after_note_create", `        mah.db.get_tag(3)`),
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q, want %q", out, "rendered")
	}

	probes, writes := f.rec.snapshot()
	if len(writes) != 2 {
		t.Errorf("writes = %v, want both writes", writes)
	}
	if len(probes) != 2 || probes[0] != 2 || probes[1] != 3 {
		t.Errorf("probes = %v, want [2 3]: a third plugin two frames down is not on the "+
			"chain and must be notified", probes)
	}
}

// The guard must key on the accumulated state set, not on a blanket "we are
// inside a plugin" flag — which would pass cases 1 and 2 while silently
// disabling cross-plugin hooks wholesale.
func TestInvocation_ChainIsPerVMAndImmutable(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		writePlugin(t, dir, name, "plugin = { name = \""+name+"\", version = \"1.0\", description = \"\" }\nfunction init() end")
	}

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"one", "two", "three"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	pm.mu.RLock()
	states := append([]*lua.LState(nil), pm.states...)
	pm.mu.RUnlock()
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}

	inv := NewInvocation(0)
	chained := inv.with(states[0])

	if !chained.holds(states[0]) {
		t.Error("the state on the chain must be held")
	}
	if chained.holds(states[1]) {
		t.Error("a state not on the chain must not be held: the guard is per-VM, not global")
	}
	if inv.holds(states[0]) {
		t.Error("with() must not mutate the receiver: a nested call must not extend its parent's chain")
	}

	// Two siblings extending the same parent with DIFFERENT states. Appending the
	// same state twice cannot expose aliasing: both writes put the same value in
	// the same slot, so a naive append(inv.states, L) passes. These must not see
	// each other's addition.
	a := chained.with(states[1])
	b := chained.with(states[2])
	if !a.holds(states[0]) || !a.holds(states[1]) {
		t.Error("sibling a must hold its own chain")
	}
	if !b.holds(states[0]) || !b.holds(states[2]) {
		t.Error("sibling b must hold its own chain")
	}
	if a.holds(states[2]) {
		t.Error("sibling a sees sibling b's state: with() aliased the parent's backing array")
	}
	if b.holds(states[1]) {
		t.Error("sibling b sees sibling a's state: with() aliased the parent's backing array")
	}
	if len(chained.states) != 1 {
		t.Errorf("parent chain grew to %d states: with() aliased its backing array", len(chained.states))
	}
}

// The actor reaches the binder from the request context that item 07 already
// threads into every request-serving entry point.
func TestInvocation_ActorComesFromRequestPrincipal(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": triggerPlugin("p1", `        mah.db.get_tag(1)`),
	})

	ctx := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: 42})
	if out := f.renderOrTimeout(t, ctx); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}

	got, ok := f.rec.lastActor()
	if !ok {
		t.Fatal("binder was never called: the read did not go through the bound querier")
	}
	if got != 42 {
		t.Errorf("bound actor = %d, want 42: a plugin read must run as the requesting user", got)
	}
}

// With no principal on the context the actor is 0, which binds nothing and
// leaves the historical unbound behaviour in place.
func TestInvocation_NoPrincipalYieldsZeroActor(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": triggerPlugin("p1", `        mah.db.get_tag(1)`),
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}

	got, ok := f.rec.lastActor()
	if !ok {
		t.Fatal("binder was never called")
	}
	if got != 0 {
		t.Errorf("bound actor = %d, want 0", got)
	}
}

// mah.start_job never named an owner, and jobVisibleToPrincipal
// (server/api_handlers/download_queue_handlers.go) hides an ownerless job from
// every non-admin — including the user who just triggered it. The async *action*
// path had always set this; start_job simply never got the same treatment.
func TestStartJob_IsOwnedByTheTriggeringUser(t *testing.T) {
	const actor = 4242

	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "starts a job" }
function init()
    mah.inject("page_bottom", function(ctx)
        return mah.start_job("work", function(job_id) end)
    end)
end
`,
	})

	reqCtx := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: actor})
	jobID := f.renderOrTimeout(t, reqCtx)
	if jobID == "" {
		t.Fatal("start_job returned no job id")
	}

	job := f.pm.GetActionJob(jobID)
	if job == nil {
		t.Fatalf("job %q not found", jobID)
	}
	owner := job.Owner()
	if owner == nil {
		t.Fatal("the job has no owner: its own submitter cannot see it at /jobs")
	}
	if *owner != actor {
		t.Errorf("job owner = %d, want %d", *owner, actor)
	}
}

// With no principal the job stays ownerless, which is the correct answer under
// auth-off and preserves the historical behaviour.
func TestStartJob_NoPrincipalLeavesJobUnowned(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "starts a job" }
function init()
    mah.inject("page_bottom", function(ctx)
        return mah.start_job("work", function(job_id) end)
    end)
end
`,
	})

	jobID := f.renderOrTimeout(t, context.Background())
	job := f.pm.GetActionJob(jobID)
	if job == nil {
		t.Fatalf("job %q not found", jobID)
	}
	if owner := job.Owner(); owner != nil {
		t.Errorf("job owner = %d, want none", *owner)
	}
}

// actorFor must read the Invocation, not just the request principal.
//
// Hooks, async jobs and drained HTTP callbacks carry their actor on an
// Invocation installed on the LState's context; they have no auth.Principal
// there. Reading the principal alone made an mah.http callback registered from
// inside a hook run as nobody, silently un-attributing everything it wrote.
func TestActorFor_ReadsTheInvocationNotOnlyThePrincipal(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", "plugin = { name = \"one\", version = \"1.0\", description = \"\" }\nfunction init() end")
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("one"); err != nil {
		t.Fatal(err)
	}
	pm.mu.RLock()
	L := pm.states[0]
	pm.mu.RUnlock()

	// A hook/job-shaped context: an Invocation, no principal.
	L.SetContext(withInvocation(context.Background(), NewInvocation(77)))
	if got := pm.actorFor(L); got != 77 {
		t.Errorf("actorFor = %d, want 77 from the invocation", got)
	}
	L.RemoveContext()

	// A request-shaped context: a principal, no invocation.
	L.SetContext(auth.WithPrincipal(context.Background(), &auth.Principal{UserID: 88}))
	if got := pm.actorFor(L); got != 88 {
		t.Errorf("actorFor = %d, want 88 from the request principal", got)
	}
	L.RemoveContext()

	// The auth-disabled system principal is not a human actor.
	L.SetContext(auth.WithPrincipal(context.Background(), &auth.Principal{SuperUser: true, UserID: 99}))
	if got := pm.actorFor(L); got != 0 {
		t.Errorf("actorFor = %d, want 0: with auth off every request is the same implicit admin", got)
	}
	L.RemoveContext()
}

// A nested hook dispatch must not wait forever for another plugin's VM.
//
// The invocation chain cannot see a cycle formed across goroutines: A holds P
// and waits for Q while B holds Q and waits for P, and neither target is on its
// own chain. Both waits were unbounded, and a block inside a Go call is not
// interruptible by the Lua deadline, so that cycle was permanent — on the code
// this replaces too. Bounding the nested wait is what removes the "permanent".
func TestTryLockVMWithin_GivesUpWhenTheVMIsHeldElsewhere(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", "plugin = { name = \"one\", version = \"1.0\", description = \"\" }\nfunction init() end")
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("one"); err != nil {
		t.Fatal(err)
	}
	pm.mu.RLock()
	L := pm.states[0]
	pm.mu.RUnlock()

	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}

	start := time.Now()
	mu, timedOut := pm.TryLockVMWithin(context.Background(), L, 60*time.Millisecond)
	elapsed := time.Since(start)

	if mu != nil {
		mu.Unlock()
		held.Unlock()
		t.Fatal("TryLockVMWithin acquired a lock that was already held")
	}
	if !timedOut {
		held.Unlock()
		t.Fatal("TryLockVMWithin reported the plugin as gone; it is alive and merely busy")
	}
	if elapsed > 5*time.Second {
		t.Errorf("waited %s, far past the 60ms bound", elapsed)
	}

	// Releasing lets it through, so the bound is a timeout and not a refusal.
	held.Unlock()
	mu2, _ := pm.TryLockVMWithin(context.Background(), L, time.Second)
	if mu2 == nil {
		t.Fatal("TryLockVMWithin failed on a free lock")
	}
	mu2.Unlock()
}

// A nested before-hook dispatch that cannot take its VM lock must FAIL the
// operation, not skip the hook.
//
// This is the asymmetry between the two dispatchers, and it is easy to lose. For
// an after-hook a timeout is a missed notification of something already
// committed, so skipping is honest. For a before-hook the hook that did not run
// may be the one that would have vetoed — and a plugin's VM being busy is
// ordinary (a 120s sync HTTP call holds it), so skipping would mean unrelated
// contention silently disables a protection hook. Anyone later collapsing the
// two dispatchers onto one code path reintroduces exactly that bypass.
func TestRunBeforeHooks_NestedDispatchFailsClosedWhenTheVMIsBusy(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "guard", `
plugin = { name = "guard", version = "1.0", description = "vetoes" }
function init()
    mah.on("before_note_delete", function(data)
        mah.abort("protected")
        return data
    end)
end
`)
	writePlugin(t, dir, "other", "plugin = { name = \"other\", version = \"1.0\", description = \"\" }\nfunction init() end")

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"guard", "other"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	hooks := pm.GetHooks("before_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	guardVM := hooks[0].state

	// Another goroutine is inside the guard plugin — an ordinary situation.
	held := pm.LockVM(guardVM)
	if held == nil {
		t.Fatal("could not hold the guard VM")
	}
	defer held.Unlock()

	// A *nested* dispatch: the caller already holds some other plugin's VM.
	pm.mu.RLock()
	var otherVM *lua.LState
	for _, st := range pm.states {
		if st != guardVM {
			otherVM = st
		}
	}
	pm.mu.RUnlock()
	if otherVM == nil {
		t.Fatal("expected a second plugin state")
	}
	nested := NewInvocation(0).with(otherVM)

	done := make(chan error, 1)
	go func() {
		_, err := pm.RunBeforeHooks(context.Background(), nested, "before_note_delete", map[string]any{"id": float64(1)})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunBeforeHooks succeeded while a veto hook could not run: " +
				"contention silently bypassed the guard")
		}
		if !errors.Is(err, ErrHookVMBusy) {
			t.Errorf("error = %v, want ErrHookVMBusy", err)
		}
	case <-time.After(hookLockWait + 15*time.Second):
		t.Fatal("RunBeforeHooks never returned: the nested wait is not bounded")
	}
}

// The after-hook side of the same situation: skipped, logged, and the operation
// is unaffected, because the change it describes has already committed.
func TestRunAfterHooks_NestedDispatchSkipsWhenTheVMIsBusy(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "watcher", `
plugin = { name = "watcher", version = "1.0", description = "watches" }
function init()
    mah.on("after_note_delete", function(data) return data end)
end
`)
	writePlugin(t, dir, "other", "plugin = { name = \"other\", version = \"1.0\", description = \"\" }\nfunction init() end")

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"other", "watcher"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	hooks := pm.GetHooks("after_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	watcherVM := hooks[0].state

	held := pm.LockVM(watcherVM)
	if held == nil {
		t.Fatal("could not hold the watcher VM")
	}
	defer held.Unlock()

	pm.mu.RLock()
	var otherVM *lua.LState
	for _, st := range pm.states {
		if st != watcherVM {
			otherVM = st
		}
	}
	pm.mu.RUnlock()
	nested := NewInvocation(0).with(otherVM)

	done := make(chan struct{})
	go func() {
		pm.RunAfterHooks(nested, "after_note_delete", map[string]any{"id": float64(1)})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(hookLockWait + 15*time.Second):
		t.Fatal("RunAfterHooks never returned: the nested wait is not bounded")
	}
}

// A coroutine's mah.db call must run as the current request's actor.
//
// The coroutine library is open to plugins, and gopher-lua hands a Go function
// the coroutine's own LState. LState.NewThread copies the parent context at
// creation and never refreshes it, so a coroutine created during init() carries
// no context at all and its writes were unattributed.
//
// The coroutine here is created at init() deliberately, and that is the only
// shape this test covers. A coroutine created *during a request* derives its
// context from that request's, so cancelling the request kills the coroutine
// too and it cannot be resumed later at all — pre-existing gopher-lua behaviour,
// not something this change alters. Resuming the init()-created one under two
// different principals is the property worth pinning: it must follow whichever
// request is live now, not the one it happened to be created under.
func TestInvocation_CoroutineWriteUsesTheCurrentRequestActor(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		// The coroutine is created at init() time, when there is no context at
		// all: the case that produced actor 0.
		"p1": `
plugin = { name = "p1", version = "1.0", description = "writes from a coroutine" }
local co
function init()
    co = coroutine.wrap(function()
        while true do
            mah.db.get_tag(1)
            coroutine.yield()
        end
    end)
    mah.inject("page_bottom", function(ctx)
        co()
        return "rendered"
    end)
end
`,
	})

	ctx := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: 4242})
	if out := f.renderOrTimeout(t, ctx); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}
	got, ok := f.rec.lastActor()
	if !ok {
		t.Fatal("the coroutine never reached the bound querier")
	}
	if got != 4242 {
		t.Errorf("bound actor = %d, want 4242: a coroutine must run as the current request", got)
	}

	// Resumed under a different principal, the same coroutine must follow the
	// live request rather than any context it has of its own.
	ctx2 := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: 7})
	if out := f.renderOrTimeout(t, ctx2); out != "rendered" {
		t.Fatalf("second slot output = %q", out)
	}
	got2, _ := f.rec.lastActor()
	if got2 != 7 {
		t.Errorf("bound actor = %d, want 7: a resumed coroutine must not carry a stale actor", got2)
	}
}

// The re-entry guard must recognise a coroutine as its own plugin.
//
// Hooks are registered against the main state, so a chain recording the
// coroutine's pointer never matches it — and the dispatch then goes back around
// for a lock the outer frame already holds.
func TestHookReentry_CoroutineWriteIsRecognisedAsTheSamePlugin(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "hooks what its coroutine writes" }
local co
function init()
    mah.on("after_note_create", function(data)
        mah.db.get_tag(1)
        return data
    end)
    co = coroutine.wrap(function()
        mah.db.create_note({ name = "from-coroutine" })
        coroutine.yield()
    end)
    mah.inject("page_bottom", function(ctx)
        co()
        return "rendered"
    end)
end
`,
	})

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}

	probes, writes := f.rec.snapshot()
	if len(writes) != 1 || writes[0] != "note" {
		t.Errorf("writes = %v, want one note write", writes)
	}
	if len(probes) != 0 {
		t.Errorf("probes = %v, want none: the plugin must not be notified of a write its own "+
			"coroutine made while it was running", probes)
	}
}

// A plugin disabled *while a nested dispatch is already waiting* for its lock is
// a skip, not a failure: reporting contention would fail a caller's write over a
// hook that no longer exists.
//
// The teardown has to happen during the wait. Removing the vmLocks entry first
// makes TryLockVMWithin return from its VMLock guard before it ever reaches the
// timeout path — which is how the first version of this test passed against a
// deliberately broken implementation.
func TestTryLockVMWithin_ReportsADisabledPluginAsGoneNotBusy(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", "plugin = { name = \"one\", version = \"1.0\", description = \"\" }\nfunction init() end")
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("one"); err != nil {
		t.Fatal(err)
	}
	pm.mu.RLock()
	L := pm.states[0]
	pm.mu.RUnlock()

	held := pm.LockVM(L)
	if held == nil {
		t.Fatal("could not take the VM lock")
	}
	defer held.Unlock()

	type result struct {
		mu   *vmMutex
		busy bool
	}
	done := make(chan result, 1)
	go func() {
		mu, busy := pm.TryLockVMWithin(context.Background(), L, 500*time.Millisecond)
		done <- result{mu, busy}
	}()

	// Tear the plugin down mid-wait, the way DisablePlugin does once the hooks
	// are already unregistered.
	time.Sleep(50 * time.Millisecond)
	pm.mu.Lock()
	delete(pm.vmLocks, L)
	pm.mu.Unlock()

	select {
	case got := <-done:
		if got.mu != nil {
			got.mu.Unlock()
			t.Fatal("acquired a lock for a state that is no longer live")
		}
		if got.busy {
			t.Error("a plugin torn down during the wait was reported as busy: a nested " +
				"before-hook would fail the caller's write over a hook that no longer exists")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("TryLockVMWithin never returned")
	}
}

// The real DisablePlugin lifecycle, not a hand-rolled approximation of it.
//
// DisablePlugin unregisters the hooks first and only drops the vmLocks entry
// once it has acquired the VM lock — so for the whole time it is waiting for
// that lock, a torn-down plugin still looks live. A nested before-hook dispatch
// that times out in that window would fail the caller's write over a hook that
// no longer exists. Deleting the vmLocks entry by hand (as an earlier version of
// this test did) never reaches that state.
func TestLockVMForHook_PluginBeingDisabledIsASkipNotContention(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "guard", `
plugin = { name = "guard", version = "1.0", description = "vetoes" }
function init()
    mah.on("before_note_delete", function(data)
        mah.abort("protected")
        return data
    end)
end
`)
	writePlugin(t, dir, "other", "plugin = { name = \"other\", version = \"1.0\", description = \"\" }\nfunction init() end")

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"guard", "other"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("EnablePlugin(%s): %v", name, err)
		}
	}

	hooks := pm.GetHooks("before_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	hook := hooks[0]

	// Hold the guard's VM so DisablePlugin cannot finish, leaving it parked
	// exactly in the window under test.
	held := pm.LockVM(hook.state)
	if held == nil {
		t.Fatal("could not hold the guard VM")
	}

	disabled := make(chan struct{})
	go func() {
		_ = pm.DisablePlugin("guard")
		close(disabled)
	}()

	// Wait until the hooks are unregistered, which is how far DisablePlugin gets
	// before it blocks on the VM lock we are holding.
	deadline := time.Now().Add(10 * time.Second)
	for len(pm.GetHooks("before_note_delete")) != 0 {
		if time.Now().After(deadline) {
			held.Unlock()
			<-disabled
			t.Fatal("DisablePlugin never unregistered the hook")
		}
		time.Sleep(2 * time.Millisecond)
	}

	pm.mu.RLock()
	var otherVM *lua.LState
	for _, st := range pm.states {
		if st != hook.state {
			otherVM = st
		}
	}
	pm.mu.RUnlock()
	nested := NewInvocation(0).with(otherVM)

	type result struct {
		mu   *vmMutex
		busy bool
	}
	got := make(chan result, 1)
	go func() {
		mu, busy := pm.lockVMForHook(context.Background(), nested, hook, "before_note_delete", 0)
		got <- result{mu, busy}
	}()

	select {
	case r := <-got:
		if r.mu != nil {
			r.mu.Unlock()
		}
		if r.busy {
			t.Error("a plugin mid-disable was reported as contention: a nested before-hook " +
				"would fail the caller's write over a hook that is already unregistered")
		}
	case <-time.After(hookLockWait + 20*time.Second):
		t.Error("lockVMForHook never returned")
	}

	held.Unlock()
	<-disabled
}

// A hook unregistered while the dispatcher was waiting must not run, even when
// the dispatcher then wins the lock.
//
// This is the other half of the disable window. DisablePlugin unregisters hooks
// and only drops the vmLocks entry once it has the VM lock, so between those two
// points the entry is still present and a dispatcher that acquires the lock
// would execute a hook that no longer exists — a disabled veto hook aborting a
// user's operation, or a disabled after-hook still firing its side effects.
//
// The intermediate state is constructed directly rather than by racing a real
// DisablePlugin, so the assertion cannot pass by accident because teardown
// happened to win the mutex.
func TestLockVMForHook_UnregisteredHookIsNotRunEvenWhenTheLockIsFree(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "guard", `
plugin = { name = "guard", version = "1.0", description = "vetoes" }
function init()
    mah.on("before_note_delete", function(data)
        mah.abort("protected")
        return data
    end)
end
`)
	writePlugin(t, dir, "control", `
plugin = { name = "control", version = "1.0", description = "stays registered" }
function init()
    mah.on("after_note_delete", function(data) return data end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	for _, name := range []string{"control", "guard"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatal(err)
		}
	}

	hooks := pm.GetHooks("before_note_delete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	hook := hooks[0]

	// Exactly DisablePlugin's intermediate state: hooks gone, vmLocks entry and
	// the live VM still present, lock free.
	pm.mu.Lock()
	delete(pm.hooks, "before_note_delete")
	pm.mu.Unlock()

	for _, tc := range []struct {
		name string
		inv  *Invocation
	}{
		{"top-level dispatch", NewInvocation(0)},
		{"nested dispatch", NewInvocation(0).with(hook.state)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu, busy := pm.lockVMForHook(context.Background(), tc.inv, hook, "before_note_delete", 0)
			if mu != nil {
				mu.Unlock()
				t.Error("an unregistered hook was handed a lock and would have run: a disabled " +
					"veto hook can still abort a user's operation")
			}
			if busy {
				t.Error("an unregistered hook was reported as contention rather than skipped")
			}
		})
	}

	// Positive control, on a hook that is still registered: without it an
	// implementation that skipped *every* hook would pass the two cases above.
	// It uses a second plugin's hook rather than re-enabling this one —
	// EnablePlugin on an already-enabled plugin returns an error, so the earlier
	// version of this control never ran at all.
	live := pm.GetHooks("after_note_delete")
	if len(live) != 1 {
		t.Fatalf("expected the control hook to be registered, got %d", len(live))
	}
	mu, busy := pm.lockVMForHook(context.Background(), NewInvocation(0), live[0], "after_note_delete", hookLockWait)
	if mu == nil {
		t.Errorf("a registered hook was skipped (busy=%v): the guard rejects everything", busy)
	} else {
		mu.Unlock()
	}
}

// recordingPluginLogger captures what the host would write to /logs.
type recordingPluginLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *recordingPluginLogger) PluginLog(pluginName, level, message string, _ map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, level+"|"+pluginName+"|"+message)
}

func (l *recordingPluginLogger) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

// A skipped after-hook must be reported at /logs, not merely dropped. The plan
// promises the operator can find out; without this assertion, deleting the
// warning entirely leaves every other test green.
func TestRunAfterHooks_SkippedDispatchIsLoggedForTheOperator(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": `
plugin = { name = "p1", version = "1.0", description = "writes what it hooks" }
function init()
    mah.on("after_note_create", function(data) return data end)
    mah.inject("page_bottom", function(ctx)
        mah.db.create_note({ name = "n" })
        return "rendered"
    end)
end
`,
	})
	logger := &recordingPluginLogger{}
	f.pm.SetPluginLogger(logger)

	if out := f.renderOrTimeout(t, context.Background()); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}

	entries := logger.all()
	found := false
	for _, e := range entries {
		if strings.Contains(e, "after_note_create") && strings.Contains(e, "p1") && strings.HasPrefix(e, "warning|") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning naming the skipped hook reached the application log (entries: %v)", entries)
	}
}

// The actor must be captured at the mah.http registration sites, not merely be
// available from actorFor. A callback registered inside a hook is drained long
// after that hook returned, so reading the principal there would find nothing —
// this pins the call sites that unit-testing actorFor alone leaves free.
func TestHttpCallback_RegisteredInsideAHookKeepsTheActor(t *testing.T) {
	f := newReentryFixture(t, map[string]string{
		"p1": triggerPlugin("p1", `        mah.db.create_note({ name = "n" })`),
		// An unsupported scheme fails validation synchronously and queues the
		// error callback, which keeps the test off the network.
		"p2": hookPlugin("p2", "after_note_create", `
        mah.http.get("ftp://example.invalid/x", function(resp)
            mah.db.get_tag(55)
        end)`),
	})

	ctx := auth.WithPrincipal(context.Background(), &auth.Principal{UserID: 4242})
	if out := f.renderOrTimeout(t, ctx); out != "rendered" {
		t.Fatalf("slot output = %q", out)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		if actor, ok := f.rec.actorForProbe(55); ok {
			if actor != 4242 {
				t.Errorf("the drained callback ran as actor %d, want 4242: the actor must be "+
					"captured where the callback is registered", actor)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the error callback never ran")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
