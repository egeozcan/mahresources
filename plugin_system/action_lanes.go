package plugin_system

import (
	"context"
	"errors"
	"sync"
	"time"
)

// This file holds the per-plugin lane every async execution waits in before it
// may take anything shared.
//
// A plugin's VM runs one Lua call at a time, so a plugin can make progress on
// exactly one piece of background work at once. Everything else it has been
// asked to do is waiting for that VM, and waiting is only harmless while it
// holds nothing: an execution that takes one of the process's job slots, or the
// deployment's capacity, and *then* waits for its VM turns one plugin's backlog
// into slots every other plugin and every other Kind is waiting for.
//
// So the order is fixed: the plugin's lane first (in arrival order, holding
// nothing), then a job slot and the VM, taken together (acquireSlotAndVM), and
// only then the host's admission — the durable claim that occupies deployment
// capacity — asked for with the VM in hand, so the claim is only ever taken by
// work that starts at once. A full budget gives the slot and the VM back before
// waiting. The execution at the head of a lane is the only one of its plugin
// that competes for anything shared.

// hostAdmissionPollInterval is how long the head of a lane waits before asking
// the host again after the deployment's budget refused it. A refusal writes
// nothing, and the slot it is waiting for is freed by another execution ending,
// possibly in another process, which this one cannot be told about.
var hostAdmissionPollInterval = 500 * time.Millisecond

// errHostJobHeld reports that the durable Job named by a HostJobRef already has
// an execution waiting or running in this process. A second one would be a second
// in-memory entry for one Job, and the Job can only be claimed once anyway.
var errHostJobHeld = errors.New("this job already has an execution in this process")

// ErrHostJobHeld is errHostJobHeld for callers outside the package: the host
// asks for a Job to be run and learns that this process is already running it.
var ErrHostJobHeld = errHostJobHeld

// pluginLane serializes one plugin's async executions in the order they were
// submitted.
//
// A place is taken when the work is submitted, synchronously, rather than when
// its goroutine first runs: goroutines start in no particular order, and a
// plugin's work should start in the order it was asked for. Release hands the
// lane straight to the oldest waiter rather than letting the waiters race for it.
//
// A schedule's tick is the one exception to arrival order (joinAhead): it goes
// ahead of the plugin's queued actions and waits only for the work already
// running and for ticks that joined ahead before it. The exception is bounded:
// two ticks are never handed the lane in a row while an action is waiting, so a
// schedule that overruns its own interval cannot keep submitted work out.
type pluginLane struct {
	mu      sync.Mutex
	held    bool
	waiters []laneWaiter
	// servedAhead records that the lane was last handed to a tick that joined
	// ahead.
	servedAhead bool
}

// laneWaiter is one place in a lane's queue.
type laneWaiter struct {
	turn  chan struct{}
	ahead bool
}

// laneTicket is one execution's place in its plugin's lane.
type laneTicket struct {
	lane  *pluginLane
	turn  chan struct{}
	ahead bool
}

// join takes a place at the back of the lane. The ticket's turn has already come
// when the lane was free.
func (l *pluginLane) join() *laneTicket {
	return l.enter(false)
}

// joinAhead takes a place ahead of every waiter that joined at the back, behind
// the ones that joined ahead before it.
//
// It is for a schedule's tick, whose wait is bounded by the dispatch budget and
// which comes back at the next tick if it gives up. Behind a plugin's whole
// backlog of actions it would give up at every tick until the backlog drained;
// ahead of it, it waits for the handler that is running, which is what it always
// had to wait for.
func (l *pluginLane) joinAhead() *laneTicket {
	return l.enter(true)
}

func (l *pluginLane) enter(ahead bool) *laneTicket {
	turn := make(chan struct{})
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		l.held = true
		l.servedAhead = ahead
		close(turn)
		return &laneTicket{lane: l, turn: turn, ahead: ahead}
	}
	waiter := laneWaiter{turn: turn, ahead: ahead}
	if !ahead {
		l.waiters = append(l.waiters, waiter)
		return &laneTicket{lane: l, turn: turn, ahead: ahead}
	}
	at := 0
	for at < len(l.waiters) && l.waiters[at].ahead {
		at++
	}
	l.waiters = append(l.waiters, laneWaiter{})
	copy(l.waiters[at+1:], l.waiters[at:])
	l.waiters[at] = waiter
	return &laneTicket{lane: l, turn: turn, ahead: ahead}
}

// rejoin takes a new place in the same lane, the way this ticket took its own.
func (t *laneTicket) rejoin() *laneTicket {
	return t.lane.enter(t.ahead)
}

// depth is how many executions hold or wait for the lane.
func (l *pluginLane) depth() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return 0
	}
	return 1 + len(l.waiters)
}

// wait waits for this ticket's turn. deadline bounds the wait (zero waits for as
// long as it takes), and done and revoked abandon it; either way a false answer
// has given the place up and holds nothing.
func (t *laneTicket) wait(done <-chan struct{}, deadline time.Time, revoked <-chan struct{}) bool {
	select {
	case <-t.turn:
		return true
	default:
	}
	var expired <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case <-t.turn:
		return true
	case <-done:
	case <-revoked:
	case <-expired:
	}
	t.leave()
	return false
}

// leave gives up a place that was never used: removed from the queue if it is
// still waiting there, or passed on if the lane was handed to it in the instant
// its wait gave up.
func (t *laneTicket) leave() {
	l := t.lane
	l.mu.Lock()
	for i, waiter := range l.waiters {
		if waiter.turn == t.turn {
			l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
			l.mu.Unlock()
			return
		}
	}
	l.mu.Unlock()
	l.release()
}

// release gives the lane to the next waiter, or frees it. The next waiter is the
// first in the queue, except that a tick is not handed the lane right after
// another tick while an action is waiting: the oldest action goes first.
func (l *pluginLane) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.waiters) == 0 {
		l.held = false
		l.servedAhead = false
		return
	}
	at := 0
	if l.waiters[0].ahead && l.servedAhead {
		for i, waiter := range l.waiters {
			if !waiter.ahead {
				at = i
				break
			}
		}
	}
	next := l.waiters[at]
	l.waiters = append(l.waiters[:at], l.waiters[at+1:]...)
	l.servedAhead = next.ahead
	close(next.turn)
}

// laneFor returns the lane of one plugin, by name: a disabled and re-enabled
// plugin keeps its lane, and the work queued in it keeps its place.
func (pm *PluginManager) laneFor(pluginName string) *pluginLane {
	pm.lanesMu.Lock()
	defer pm.lanesMu.Unlock()
	if pm.lanes == nil {
		pm.lanes = make(map[string]*pluginLane)
	}
	lane, ok := pm.lanes[pluginName]
	if !ok {
		lane = &pluginLane{}
		pm.lanes[pluginName] = lane
	}
	return lane
}

// LaneDepth is how many async executions of one plugin hold or wait for its
// lane in this process.
func (pm *PluginManager) LaneDepth(pluginName string) int {
	return pm.laneFor(pluginName).depth()
}

// AdmitResult is the host's answer when the head of a lane asks to start.
type AdmitResult int

const (
	// Admitted means the host now owns the durable Job under a claim for this
	// execution, and the reference's Sink reports into it from here on.
	Admitted AdmitResult = iota
	// AdmitLater means the Job is still waiting and this execution may not start
	// it yet: the deployment's budget is full, or the host could not finish
	// admitting it within its bound and is giving the claim back. The same
	// question may be asked again.
	AdmitLater
	// AdmitWithdrawn means the Job will never be this execution's to run: it
	// ended, it was blocked, or another runtime owns it. The host has recorded
	// whatever needed recording, and the execution leaves without starting.
	AdmitWithdrawn
	// AdmitDeferred means the host gave back the claim it had granted, because
	// it could not find out under it whether the Job may run. The Job is waiting
	// again. The execution leaves its lane for the host's deferral (HostDeferral),
	// so the plugin's other work is not held behind it, and rejoins at the back.
	AdmitDeferred
)

// HostDeferral is implemented by a HostAdmission that answers AdmitDeferred.
// Deferral is how long the execution stays out of its lane before it takes a
// place again.
type HostDeferral interface {
	Deferral() time.Duration
}

// HostJobNamer is implemented by a HostAdmission whose durable Job is accepted
// only when it is first admitted, rather than before it waited: a scheduled tick
// that never gets its plugin, a job slot or the deployment's budget leaves no Job
// behind. JobID answers that Job once it exists, and the execution takes it as
// its own from the admission on.
type HostJobNamer interface {
	JobID() string
}

// HostAdmission is the durable half of one queued execution: the claim the head
// of a lane asks for once it holds a job slot.
//
// It is optional on a HostJobRef. Without one the reference is already admitted
// — the host claimed the Job before handing it over — and its Sink is live from
// the start.
//
// deadline is when the asking execution stops waiting, or zero for never. The
// host bounds its own work by it, because an admission that returns after it is
// one the execution gives back rather than runs.
type HostAdmission interface {
	Admit(deadline time.Time) AdmitResult
}

// asyncOutcome is what one attempt to run an async execution came to.
type asyncOutcome int

const (
	// asyncRan means the work was entered (and settled).
	asyncRan asyncOutcome = iota
	// asyncGaveUp means a bounded wait ran out before the work was entered.
	asyncGaveUp
	// asyncWithdrawn means the host withdrew the Job before the work was entered.
	asyncWithdrawn
	// asyncClosing means the manager is shutting down. The entry is left in place
	// so the shutdown can report it as work that can never finish.
	asyncClosing
	// asyncNotStarted means the work itself reported that it never began.
	asyncNotStarted
	// asyncRevoked means the VM the work belongs to was revoked — its plugin was
	// disabled or reloaded — before the work was entered.
	asyncRevoked
	// asyncNotEntered means the host admitted the work and it was not entered
	// after all, and the host has been told so (HostJobSink.NotStarted).
	asyncNotEntered
)

// asyncBounds are the waits one execution may spend before its work is entered.
// A zero deadline waits for as long as it takes.
type asyncBounds struct {
	// revoked ends every wait when the VM the work belongs to is revoked.
	revoked <-chan struct{}
	// lane bounds the wait for the plugin's lane.
	lane time.Time
	// slotWait bounds the wait for a job slot and for the host's admission,
	// measured from the moment the plugin's VM is first held: a wait for the VM
	// that is meant to be unbounded must not spend it. Zero or less waits
	// forever.
	slotWait time.Duration
	// slot is an absolute bound on the same waits; it wins when it is earlier.
	slot time.Time
}

// slotDeadline is the bound on the job-slot and admission waits, fixed the first
// time the plugin's VM is held.
func (b asyncBounds) slotDeadline(now time.Time) time.Time {
	deadline := b.slot
	if b.slotWait > 0 {
		relative := now.Add(b.slotWait)
		if deadline.IsZero() || relative.Before(deadline) {
			deadline = relative
		}
	}
	return deadline
}

// acquireJobSlotUntil takes one of the process's job slots, waiting until the
// deadline (zero waits forever), until the manager closes, or until the VM the
// work belongs to is revoked.
func (pm *PluginManager) acquireJobSlotUntil(deadline time.Time, revoked <-chan struct{}) asyncOutcome {
	// A free slot is taken even when the deadline has passed: a select with a
	// free slot and an expired timer both ready picks either.
	select {
	case pm.actionSemaphore <- struct{}{}:
		return asyncRan
	default:
	}
	var expired <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case pm.actionSemaphore <- struct{}{}:
		return asyncRan
	case <-pm.done:
		return asyncClosing
	case <-revoked:
		return asyncRevoked
	case <-expired:
		return asyncGaveUp
	}
}

// admitNext is what the head of a lane does after asking the host once.
type admitNext int

const (
	// admitDone: the outcome stands, asyncRan meaning the work may start.
	admitDone admitNext = iota
	// admitAgain: ask again once the VM and the slot have been given back.
	admitAgain
	// admitStepOut: leave the lane for the host's deferral (HostDeferral).
	admitStepOut
)

// admitOnce asks the host for the durable claim once. It is asked with the
// plugin's VM already held, so the claim is only ever taken by work that can
// start at once: waiting for the VM with a claim held would hold a slot of the
// deployment's budget for work that is doing nothing.
func (pm *PluginManager) admitOnce(job *ActionJob, deadline time.Time) (asyncOutcome, admitNext) {
	ref := job.hostJobRef()
	if ref == nil || ref.Admission == nil {
		return asyncRan, admitDone
	}
	if pm.closed.Load() {
		return asyncClosing, admitDone
	}
	switch ref.Admission.Admit(deadline) {
	case Admitted:
		if namer, ok := ref.Admission.(HostJobNamer); ok && ref.JobID == "" {
			pm.nameHostJob(job, namer.JobID())
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			// Admitted too late: the caller that bounded this wait has stopped
			// waiting, so the execution does not start. The host holds the claim
			// it granted and settles it on the way out.
			return asyncGaveUp, admitDone
		}
		return asyncRan, admitDone
	case AdmitWithdrawn:
		return asyncWithdrawn, admitDone
	case AdmitDeferred:
		if _, ok := ref.Admission.(HostDeferral); ok {
			return asyncRan, admitStepOut
		}
		return asyncRan, admitAgain
	default:
		return asyncRan, admitAgain
	}
}

// nameHostJob records the durable Job an execution was admitted into, when the
// host accepted it only at the admission. The reference is replaced rather than
// written in place, because readers hold it without the job's lock.
func (pm *PluginManager) nameHostJob(job *ActionJob, jobID string) {
	if jobID == "" {
		return
	}
	job.mu.Lock()
	if job.host != nil {
		named := *job.host
		named.JobID = jobID
		job.host = &named
	}
	job.mu.Unlock()
}

// stepOutOfLane is how the head of a lane waits for a host that deferred it: it
// gives its place up, so the plugin's other work goes first, stays out for the
// host's deferral, and takes a place at the back again. It answers asyncRan
// holding the lane again.
func (pm *PluginManager) stepOutOfLane(job *ActionJob, ticket **laneTicket, laneHeld *bool, waitCtx context.Context, deadline time.Time, revoked <-chan struct{}) asyncOutcome {
	deferral := job.hostJobRef().Admission.(HostDeferral).Deferral()
	if deferral <= 0 {
		deferral = hostAdmissionPollInterval
	}
	(*ticket).lane.release()
	*laneHeld = false
	ctx := waitCtx
	if !deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(waitCtx, deadline)
		defer cancel()
	}
	timer := time.NewTimer(deferral)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return pm.abandonedWait(revoked)
	}
	*ticket = (*ticket).rejoin()
	if !(*ticket).wait(pm.done, deadline, revoked) {
		return pm.abandonedWait(revoked)
	}
	*laneHeld = true
	return asyncRan
}

// expired reports whether an absolute deadline has passed. A lane turn, a job
// slot or the VM that is free when asked for is taken without waiting, so a
// caller's absolute deadline is asked again once they are held, and what was
// taken after it is given back: that caller has stopped waiting. A bound on one
// wait alone (asyncBounds.slotWait) is not asked again, because the wait it
// bounds did not happen.
func expired(deadline time.Time) bool {
	return !deadline.IsZero() && !time.Now().Before(deadline)
}

// abandonedWait says why a wait that did not end in its turn ended.
func (pm *PluginManager) abandonedWait(revoked <-chan struct{}) asyncOutcome {
	switch {
	case pm.closed.Load():
		return asyncClosing
	case isClosed(revoked):
		return asyncRevoked
	default:
		return asyncGaveUp
	}
}

// pauseBeforeAdmission waits before the head of a lane asks the host again,
// bounded by the deadline and abandoned when the manager closes or the VM is
// revoked.
func (pm *PluginManager) pauseBeforeAdmission(deadline time.Time, revoked <-chan struct{}) asyncOutcome {
	pause := hostAdmissionPollInterval
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return asyncGaveUp
		}
		if remaining < pause {
			pause = remaining
		}
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-pm.done:
		return asyncClosing
	case <-revoked:
		return asyncRevoked
	case <-timer.C:
		return asyncRan
	}
}

// holdHostJob records that this process has an execution for one durable Job,
// under its in-memory id, refusing a second. It is what makes handing a waiting
// Job to this process idempotent: the request that accepted it and a later pass
// that finds it waiting can both ask, and exactly one execution results. A
// refusal answers the id of the execution already held.
func (pm *PluginManager) holdHostJobLocked(host *HostJobRef, localID string) (string, bool) {
	if host == nil || host.JobID == "" {
		return localID, true
	}
	if held, ok := pm.hostHeld[host.JobID]; ok {
		return held, false
	}
	if pm.hostHeld == nil {
		pm.hostHeld = make(map[string]string)
	}
	pm.hostHeld[host.JobID] = localID
	return localID, true
}

// releaseHostJob forgets that this process has an execution for a Job.
func (pm *PluginManager) releaseHostJob(host *HostJobRef) {
	if host == nil || host.JobID == "" {
		return
	}
	pm.actionJobsMu.Lock()
	delete(pm.hostHeld, host.JobID)
	pm.actionJobsMu.Unlock()
}

// HostJobHeld reports whether this process already has an execution for the
// durable Job, waiting in a lane or running.
func (pm *PluginManager) HostJobHeld(jobID string) bool {
	pm.actionJobsMu.RLock()
	defer pm.actionJobsMu.RUnlock()
	_, held := pm.hostHeld[jobID]
	return held
}

// dropUnstartedJob removes the in-memory entry of an execution that will never
// start, so the panel is not left listing work nobody will run. An entry that
// has since been replaced under the same id (a Retry's successor sharing the
// handle) is not this one, so it is neither removed nor announced as removed.
func (pm *PluginManager) dropUnstartedJob(job *ActionJob) {
	pm.actionJobsMu.Lock()
	current, ok := pm.actionJobs[job.ID]
	removed := ok && current == job
	if removed {
		delete(pm.actionJobs, job.ID)
	}
	pm.actionJobsMu.Unlock()
	if removed {
		pm.notifyActionJobSubscribers("removed", job)
	}
}
