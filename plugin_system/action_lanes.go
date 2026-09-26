package plugin_system

import (
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
// nothing), then a job slot, then the host's admission — the durable claim that
// occupies deployment capacity — and only then the VM. The execution at the head
// of a lane is the only one of its plugin that competes for anything shared.

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
// running and for ticks that joined ahead before it.
type pluginLane struct {
	mu      sync.Mutex
	held    bool
	waiters []laneWaiter
}

// laneWaiter is one place in a lane's queue.
type laneWaiter struct {
	turn  chan struct{}
	ahead bool
}

// laneTicket is one execution's place in its plugin's lane.
type laneTicket struct {
	lane *pluginLane
	turn chan struct{}
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
		close(turn)
		return &laneTicket{lane: l, turn: turn}
	}
	waiter := laneWaiter{turn: turn, ahead: ahead}
	if !ahead {
		l.waiters = append(l.waiters, waiter)
		return &laneTicket{lane: l, turn: turn}
	}
	at := 0
	for at < len(l.waiters) && l.waiters[at].ahead {
		at++
	}
	l.waiters = append(l.waiters, laneWaiter{})
	copy(l.waiters[at+1:], l.waiters[at:])
	l.waiters[at] = waiter
	return &laneTicket{lane: l, turn: turn}
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
// long as it takes) and done abandons it; either way a false answer has given the
// place up and holds nothing.
func (t *laneTicket) wait(done <-chan struct{}, deadline time.Time) bool {
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

// release gives the lane to the oldest waiter, or frees it.
func (l *pluginLane) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.waiters) > 0 {
		next := l.waiters[0]
		l.waiters = l.waiters[1:]
		close(next.turn)
		return
	}
	l.held = false
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
	// AdmitLater means the deployment's budget is full. Nothing was written, and
	// the same question may be asked again.
	AdmitLater
	// AdmitWithdrawn means the Job will never be this execution's to run: it
	// ended, it was blocked, or another runtime owns it. The host has recorded
	// whatever needed recording, and the execution leaves without starting.
	AdmitWithdrawn
)

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
)

// asyncBounds are the waits one execution may spend before its work is entered.
// A zero deadline waits for as long as it takes.
type asyncBounds struct {
	// lane bounds the wait for the plugin's lane.
	lane time.Time
	// slotWait bounds the wait for a job slot and for the host's admission,
	// measured from the moment the lane is held. Zero or less waits forever.
	slotWait time.Duration
	// slot is an absolute bound on the same waits; it wins when it is earlier.
	slot time.Time
}

// slotDeadline is the bound on the job-slot and admission waits, fixed once the
// lane is held.
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
// deadline (zero waits forever) or until the manager closes.
func (pm *PluginManager) acquireJobSlotUntil(deadline time.Time) asyncOutcome {
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
	case <-expired:
		return asyncGaveUp
	}
}

// admitHostJob asks the host for the durable claim, asking again while the
// deployment's budget is full, until the deadline (zero waits forever).
func (pm *PluginManager) admitHostJob(job *ActionJob, deadline time.Time) asyncOutcome {
	ref := job.hostJobRef()
	if ref == nil || ref.Admission == nil {
		return asyncRan
	}
	for {
		if pm.closed.Load() {
			return asyncClosing
		}
		switch ref.Admission.Admit(deadline) {
		case Admitted:
			if !deadline.IsZero() && time.Now().After(deadline) {
				// Admitted too late: the caller that bounded this wait has
				// stopped waiting, so the execution does not start. The host
				// holds the claim it granted and settles it on the way out.
				return asyncGaveUp
			}
			return asyncRan
		case AdmitWithdrawn:
			return asyncWithdrawn
		}
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
		select {
		case <-pm.done:
			timer.Stop()
			return asyncClosing
		case <-timer.C:
		}
	}
}

// holdHostJob records that this process has an execution for one durable Job,
// refusing a second. It is what makes handing a waiting Job to this process
// idempotent: the request that accepted it and a later pass that finds it
// waiting can both ask, and exactly one execution results.
func (pm *PluginManager) holdHostJobLocked(host *HostJobRef) bool {
	if host == nil || host.JobID == "" {
		return true
	}
	if _, held := pm.hostHeld[host.JobID]; held {
		return false
	}
	if pm.hostHeld == nil {
		pm.hostHeld = make(map[string]struct{})
	}
	pm.hostHeld[host.JobID] = struct{}{}
	return true
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
// start, so the panel is not left listing work nobody will run.
func (pm *PluginManager) dropUnstartedJob(job *ActionJob) {
	pm.actionJobsMu.Lock()
	if current, ok := pm.actionJobs[job.ID]; ok && current == job {
		delete(pm.actionJobs, job.ID)
	}
	pm.actionJobsMu.Unlock()
	pm.notifyActionJobSubscribers("removed", job)
}
