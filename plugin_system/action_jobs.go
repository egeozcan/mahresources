package plugin_system

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mahresources/models/jobmetrics"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	actionJobRetention     = 1 * time.Hour
	actionJobCleanInterval = 5 * time.Minute
	maxConcurrentActions   = 3
)

// ActionJob represents an asynchronous plugin action execution.
//
// It is the *in-memory projection* of one execution: the jobs panel lists it and
// the action_* SSE stream announces it. When a host Job control plane is
// installed the durable Job is the authority — this entry mirrors what it reports
// — and without one this entry is the only lifecycle there is, which is what a
// bare manager, the package's own tests and a programmatic embedder see.
type ActionJob struct {
	ID             string `json:"id"`
	CanonicalJobID string `json:"canonicalJobId,omitempty"`
	Source         string `json:"source"` // always "plugin"
	PluginName     string `json:"pluginName"`
	ActionID       string `json:"actionId"`
	Label          string `json:"label"`
	EntityID       uint   `json:"entityId"`
	EntityType     string `json:"entityType"`
	Status         string `json:"status"`   // pending, running, completed, failed
	Progress       int    `json:"progress"` // 0-100
	Message        string `json:"message"`
	// Completed, Total and Unit are the count a plugin reported through the
	// table form of mah.job_progress, and Metrics the figures beside it.
	Completed    *int64              `json:"completed,omitempty"`
	Total        *int64              `json:"total,omitempty"`
	Unit         string              `json:"unit,omitempty"`
	Metrics      []jobmetrics.Metric `json:"metrics,omitempty"`
	Result       map[string]any      `json:"result,omitempty"`
	CreatedAt    time.Time           `json:"createdAt"`
	mu           sync.RWMutex
	lastNotified time.Time // tracks when the last SSE notification was sent for throttling
	// progressPending records a report the throttle held back. The latest one
	// is always on the entry; this says the durable Job has not seen it yet,
	// so the next report or the settlement sends it rather than dropping it.
	progressPending bool
	// ownerUserID is the user that submitted the action (RBAC). It is never
	// serialized to JSON; callers read it via Owner() to decide visibility so a
	// non-admin only sees the jobs it created.
	ownerUserID *uint
	// host is the durable Job this execution reports into, or nil when this
	// process has no control plane. It is read under mu like every other field.
	host *HostJobRef
	// hostSettled records that this execution's *outcome* has been reported to the
	// durable Job. It is deliberately not derived from Status: a handler may request
	// an outcome (mah.job_complete/mah.job_fail) and keep running, so the entry can
	// read as finished while the Job it owns is still running and still holds the
	// deployment's capacity — which is exactly the state a graceful shutdown has to
	// report as a lost callback.
	hostSettled bool
}

// Owner returns the user that submitted the action job, or nil when it was
// created without an authenticated user (e.g. auth disabled).
func (j *ActionJob) Owner() *uint {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ownerUserID
}

// ActionJobEvent represents a change in action job state for SSE broadcasting.
// Job points to a snapshot copy, safe for concurrent reads without locking.
type ActionJobEvent struct {
	Type string     `json:"type"` // "added", "updated", "removed"
	Job  *ActionJob `json:"job"`
}

// hostJobRef answers the durable Job this execution reports into, or nil.
func (j *ActionJob) hostJobRef() *HostJobRef {
	if j == nil {
		return nil
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.host
}

// reportHostJob calls one sink method outside the job's own lock.
//
// Outside, because a report reaches a database and the panel reads this entry
// under that lock: doing the I/O inside would stall every reader behind one
// progress tick. A job with no host Job reports nowhere, which is the whole
// difference a process without a control plane sees.
func reportHostJob(job *ActionJob, report func(HostJobSink) error) error {
	ref := job.hostJobRef()
	if ref == nil || ref.Sink == nil {
		return nil
	}
	return report(ref.Sink)
}

// reportHostJobOnce reports one execution's terminal outcome to its durable Job, at
// most once per execution — where "once" counts deliveries, not attempts.
//
// One report per execution is a property of the *outcome*, not of the entry's
// status: the plugin's own request is recorded when it is made but published only
// once its callback returns, so a panic unwinding that callback and the settle path
// can both reach here, and exactly one of them may speak for the Job.
//
// A refused report therefore leaves the execution unsettled rather than marking it
// delivered: the outcome stands, the host retains it, and `reportLostCallbacks` at
// shutdown still knows this callback's outcome never reached the Job — which is what
// turns a transient write failure into an interrupted Job the next process can see
// rather than a Job that runs and is heartbeated forever.
func reportHostJobOnce(job *ActionJob, report func(HostJobSink) error) {
	job.mu.Lock()
	if job.hostSettled {
		job.mu.Unlock()
		return
	}
	job.mu.Unlock()
	if err := reportHostJob(job, report); err != nil {
		return
	}
	job.mu.Lock()
	job.hostSettled = true
	job.mu.Unlock()
}

// reportLostCallbacks tells the host that the callbacks of every execution still
// running in this process will never finish.
//
// Called from Close, and only from there: a lease expiry proves nothing about a
// callback, while stopping the VM proves the *lua.LFunction cannot run again.
// Both queued and running work is named — a job that never started is as
// unfinishable as one that did — and the host decides what that means for each.
//
// What is *not* named is an execution whose outcome has already been reported: the
// in-memory status cannot answer that question, because a handler that called
// mah.job_fail and kept running reads as failed while its durable Job is still
// running.
func (pm *PluginManager) reportLostCallbacks(reason string) {
	pm.actionJobsMu.RLock()
	running := make([]*ActionJob, 0, len(pm.actionJobs))
	for _, job := range pm.actionJobs {
		job.mu.RLock()
		host := job.host
		settled := job.hostSettled
		job.mu.RUnlock()
		if host != nil && host.Sink != nil && !settled {
			running = append(running, job)
		}
	}
	pm.actionJobsMu.RUnlock()

	for _, job := range running {
		_ = reportHostJob(job, func(sink HostJobSink) error { sink.CallbackLost(reason); return nil })
	}
}

// flushHeldProgress sends a report the throttle held back, or one whose write
// failed, before any outcome is reported: once the Job is terminal a progress
// write is refused, so the Job would end on an earlier report's counts and
// metrics. Every path that reports an outcome calls it first. A report that is
// refused again stays held for the next caller.
func flushHeldProgress(job *ActionJob) {
	job.mu.Lock()
	pending := job.progressPending
	progress := job.hostProgressLocked()
	job.progressPending = false
	job.mu.Unlock()
	if !pending {
		return
	}
	if err := reportHostJob(job, func(sink HostJobSink) error { return sink.Progress(progress) }); err != nil {
		job.mu.Lock()
		job.progressPending = true
		job.mu.Unlock()
	}
}

// hostProgressLocked is the entry's latest report in the shape the durable Job
// takes. The caller holds mu.
func (j *ActionJob) hostProgressLocked() HostProgress {
	return HostProgress{
		Percent: j.Progress, Message: j.Message,
		Completed: copyCount(j.Completed), Total: copyCount(j.Total), Unit: j.Unit,
		Metrics: jobmetrics.Clone(j.Metrics),
	}
}

func copyCount(v *int64) *int64 {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}

// Snapshot returns a copy of the ActionJob safe for serialization.
func (j *ActionJob) Snapshot() *ActionJob {
	j.mu.RLock()
	defer j.mu.RUnlock()

	snap := &ActionJob{
		ID:             j.ID,
		CanonicalJobID: j.CanonicalJobID,
		Source:         j.Source,
		PluginName:     j.PluginName,
		ActionID:       j.ActionID,
		Label:          j.Label,
		EntityID:       j.EntityID,
		EntityType:     j.EntityType,
		Status:         j.Status,
		Progress:       j.Progress,
		Message:        j.Message,
		Completed:      copyCount(j.Completed),
		Total:          copyCount(j.Total),
		Unit:           j.Unit,
		Metrics:        jobmetrics.Clone(j.Metrics),
		CreatedAt:      j.CreatedAt,
		ownerUserID:    j.ownerUserID,
	}

	// Shallow copy of Result is safe because results are write-once:
	// set exactly once when the job completes, never mutated afterward.
	if j.Result != nil {
		snap.Result = make(map[string]any, len(j.Result))
		for k, v := range j.Result {
			snap.Result[k] = v
		}
	}

	// The host reference is copied by value: an *ActionJob the panel reads must not
	// carry a pointer into the live entry, and the sink is what a caller of a
	// snapshot never uses.
	if j.host != nil {
		host := *j.host
		snap.host = &host
		if host.JobID != "" {
			snap.CanonicalJobID = host.JobID
		}
	}

	return snap
}

// generateActionJobID creates a short random ID for action jobs.
func generateActionJobID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// RunActionAsync validates and starts an async action execution, returning the
// job ID. The job is created without an owner; use RunActionAsyncForOwner to
// record the submitting user for RBAC visibility.
//
// No generation check: this entry point has no production caller and no
// validated registration to bind to. Anything that grows one should call
// RunActionAsyncForOwner with a fingerprint instead.
func (pm *PluginManager) RunActionAsync(pluginName, actionID string, entityID uint, params map[string]any) (string, error) {
	return pm.RunActionAsyncForOwner(nil, pluginName, actionID, entityID, params, "")
}

// RunActionAsyncForOwner is RunActionAsync but tags the job with the submitting
// user so it is only listed/streamed to that user (and admins).
// expectFilters is the fingerprint of the registration the caller validated
// against, or "" to skip the check. See checkActionUnchanged.
func (pm *PluginManager) RunActionAsyncForOwner(ownerUserID *uint, pluginName, actionID string, entityID uint, params map[string]any, expectFilters string) (string, error) {
	return pm.RunActionAsyncForHost(nil, ownerUserID, pluginName, actionID, entityID, params, expectFilters)
}

// RunActionAsyncForHost is RunActionAsyncForOwner with a durable host Job to
// report the execution into.
//
// host names the Job the host accepted for this execution and the sink it reports
// through; the caller owns that Job's lifecycle and this only publishes facts into
// it. Nil is the no-control-plane shape and keeps the in-memory entry as the only
// record, exactly as before.
//
// The in-memory ActionJob is created either way and its id is *host.Handle* when
// one is given: the id the client was answered with, the id the panel renders and
// the id the legacy action-job endpoint resolves all have to be the same string,
// or one execution would be two rows seen two ways.
//
// A host Job this process already has an execution for is not given a second
// one: the existing entry's id is answered, so handing a waiting Job to this
// process twice results in one execution.
func (pm *PluginManager) RunActionAsyncForHost(host *HostJobRef, ownerUserID *uint, pluginName, actionID string, entityID uint, params map[string]any, expectFilters string) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	action, L, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return "", err
	}
	if err := checkActionUnchanged(action, expectFilters); err != nil {
		return "", err
	}

	// Validate params.
	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return "", fmt.Errorf("validation failed: %s: %s", validationErrs[0].Field, validationErrs[0].Message)
	}

	jobID := generateActionJobID()
	if host != nil && host.Handle != "" {
		jobID = host.Handle
	}
	job := &ActionJob{
		ID:          jobID,
		Source:      "plugin",
		PluginName:  pluginName,
		ActionID:    actionID,
		Label:       action.Label,
		EntityID:    entityID,
		EntityType:  action.Entity,
		Status:      "pending",
		Progress:    0,
		Message:     "Waiting to start...",
		CreatedAt:   time.Now(),
		ownerUserID: ownerUserID,
		host:        host,
	}

	pm.actionJobsMu.Lock()
	if !pm.holdHostJobLocked(host) {
		pm.actionJobsMu.Unlock()
		return jobID, nil
	}
	pm.actionJobs[jobID] = job
	pm.actionJobsMu.Unlock()

	pm.notifyActionJobSubscribers("added", job)

	// Track in-flight async actions so DisablePlugin can wait for completion.
	wg := pm.actionWaitGroup(pluginName)
	wg.Add(1)
	ticket := pm.laneFor(pluginName).join()
	revoked := pm.stateRevoked(L)

	go func() {
		defer wg.Done()
		defer pm.releaseHostJob(host)
		switch pm.runAsyncActionGoroutine(job, ticket, revoked, entityID, params, expectFilters) {
		case asyncGaveUp, asyncWithdrawn:
			pm.dropUnstartedJob(job)
		case asyncRevoked:
			pm.abandonUnstartedJob(job)
		}
	}()

	return jobID, nil
}

// FillJobBudgetForTest saturates the async job budget and returns a release
// function that is safe to call more than once.
//
// Exported for application_context's scheduler tests, which have to observe what
// a tick does when it cannot get a slot and cannot reach this package's
// unexported semaphore. Idempotent release because the interesting test frees
// the budget mid-way and still defers a cleanup.
func (pm *PluginManager) FillJobBudgetForTest() func() {
	for i := 0; i < maxConcurrentActions; i++ {
		pm.actionSemaphore <- struct{}{}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for i := 0; i < maxConcurrentActions; i++ {
				<-pm.actionSemaphore
			}
		})
	}
}

// asyncWork is one execution's Lua side.
//
// lock takes the plugin's VM for the execution and answers it held: waiting for
// it when wait is set, and otherwise answering errVMBusy at once when it is
// taken. It answers errPluginGone when the plugin is no longer there to run it,
// or an error wrapping errJobDidNotStart when a bounded wait for the VM ran out.
// live reports, with the VM held, whether the VM lock answered for is still the
// plugin's. run enters the handler with the VM held, and releases it before
// returning.
type asyncWork struct {
	lock func(wait bool) (*vmMutex, error)
	live func() bool
	run  func(mu *vmMutex) error
}

// errPluginGone is asyncWork.lock's answer for a plugin that was disabled or
// reloaded: its VM is gone, and nothing was entered.
var errPluginGone = errors.New("the plugin is no longer available")

// errVMBusy is asyncWork.lock's answer, without waiting, for a VM somebody else
// holds.
var errVMBusy = errors.New("the plugin's VM is busy")

// lockVMFor is asyncWork.lock for a VM that is always waited for when asked to
// wait.
func (pm *PluginManager) lockVMFor(L *lua.LState, wait bool) (*vmMutex, error) {
	if wait {
		mu := pm.LockVM(L)
		if mu == nil {
			return nil, errPluginGone
		}
		return mu, nil
	}
	mu, busy := pm.TryLockVMWithin(context.Background(), L, 0)
	switch {
	case mu != nil:
		return mu, nil
	case busy:
		return nil, errVMBusy
	default:
		return nil, errPluginGone
	}
}

// executeAsyncJob is the common scaffold for running an async job goroutine:
// the plugin's lane, a job slot, the VM, the host's admission, panic recovery,
// status transitions, error handling and default completion.
//
// Every wait here is unbounded, which is what an action or a start_job wants: a
// user asked for that work and nothing else will ask again. None of them holds
// anything another plugin needs — see action_lanes.go for the order.
func (pm *PluginManager) executeAsyncJob(job *ActionJob, logLabel string, ticket *laneTicket, revoked <-chan struct{}, work asyncWork) asyncOutcome {
	return pm.runAsyncJob(job, logLabel, asyncBounds{revoked: revoked}, ticket, work)
}

// executeAsyncJobWithin is executeAsyncJob with bounds on the waits before the
// work, and it reports whether the job ran at all.
//
// The bounds are for a caller that must not block indefinitely because it is
// holding something while it waits. The scheduler is the only such caller today,
// and what it holds is a database claim on the schedule row; a claim of unbounded
// lifetime cannot have a meaningful expiry, and its expiry is the only thing
// stopping a second process running the same schedule.
//
// Returning false means the work was never entered: no outcome was recorded and
// no failure was announced. That is what lets a caller holding a resource treat a
// full budget as "not now" and give the resource back.
func (pm *PluginManager) executeAsyncJobWithin(job *ActionJob, logLabel string, bounds asyncBounds, ticket *laneTicket, work asyncWork) (ran bool) {
	return pm.runAsyncJob(job, logLabel, bounds, ticket, work) == asyncRan
}

// errJobDidNotStart is a work function's way of saying it never began.
//
// The runner's contract is "ran means the job entered its work", and work that
// spends its own bounded wait on something it could not get — today, a schedule
// waiting on the plugin's VM lock — has not. Without this it looked identical to
// a job that ran and failed: the panel announced "Action failed" to a screen
// reader, kept a failed row for a handler that was never entered, and the
// application log blamed the plugin for it.
var errJobDidNotStart = errors.New("the job never entered its work")

// runAsyncJob takes, in order, the plugin's lane, a job slot, the plugin's VM and
// the host's admission, and only then enters the work. See action_lanes.go for
// why that order is the whole design: an execution that is waiting holds nothing
// shared. The VM comes before the admission so that a claim — a slot of the
// deployment's budget — is only taken by work that can start at once; and a full
// budget gives the VM back before waiting, so a plugin's hooks and pages are
// never held behind the budget.
//
// ticket is the place in the lane the caller took when the work was submitted,
// or nil to take one now.
func (pm *PluginManager) runAsyncJob(job *ActionJob, logLabel string, bounds asyncBounds, ticket *laneTicket, work asyncWork) (outcome asyncOutcome) {
	started := false
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plugin] panic in %s: %v", logLabel, r)
			if !started {
				// Nothing was admitted, so there is no Job of this execution's to
				// fail: the host still holds it waiting.
				outcome = asyncGaveUp
				return
			}
			message := fmt.Sprintf("panic: %v", r)
			job.mu.Lock()
			job.Status = "failed"
			job.Message = message
			job.mu.Unlock()
			pm.notifyActionJobSubscribers("updated", job)
			flushHeldProgress(job)
			reportHostJobOnce(job, func(sink HostJobSink) error { return sink.Failed(message) })
			// It ran, and it failed, which is a different thing from never starting.
			outcome = asyncRan
		}
	}()

	if ticket == nil {
		ticket = pm.laneFor(job.PluginName).join()
	}
	if !ticket.wait(pm.done, bounds.lane, bounds.revoked) {
		switch {
		case pm.closed.Load():
			return asyncClosing
		case isClosed(bounds.revoked):
			return asyncRevoked
		default:
			return asyncGaveUp
		}
	}
	defer ticket.lane.release()

	slotDeadline := bounds.slotDeadline(time.Now())
	var mu *vmMutex
	for {
		held, got := pm.acquireSlotAndVM(work, slotDeadline, bounds.revoked)
		if got != asyncRan {
			return got
		}
		got, retry := pm.admitOnce(job, slotDeadline)
		if !retry && got == asyncRan {
			mu = held
			break
		}
		held.Unlock()
		<-pm.actionSemaphore
		if !retry {
			return got
		}
		if paused := pm.pauseBeforeAdmission(slotDeadline, bounds.revoked); paused != asyncRan {
			return paused
		}
	}
	defer func() { <-pm.actionSemaphore }()

	started = true
	if !work.live() {
		// The plugin was disabled or reloaded while the claim was being asked
		// for: the VM was revoked under the lock this execution holds. The claim
		// was granted, so the Job ends here, as work whose plugin went away.
		mu.Unlock()
		pm.settleActionJob(job, logLabel, fmt.Errorf("plugin %q is no longer available", job.PluginName))
		return asyncRan
	}
	job.mu.Lock()
	job.Status = "running"
	job.Message = "Running..."
	job.mu.Unlock()
	pm.notifyActionJobSubscribers("updated", job)
	_ = reportHostJob(job, func(sink HostJobSink) error { sink.Started("Running..."); return nil })

	err := work.run(mu)

	if errors.Is(err, errJobDidNotStart) {
		// Nothing was entered, so there is no outcome to record and nothing to
		// tell subscribers: the caller removes the job entry, and a status
		// written here would be the last word the panel retained about it.
		return asyncNotStarted
	}

	pm.settleActionJob(job, logLabel, err)
	return asyncRan
}

// acquireSlotAndVM takes one of the process's job slots and the plugin's VM
// together, and never waits for one while holding the other: a slot held while
// the VM is busy with a synchronous call is a slot no other plugin can use, and
// the VM held while waiting for a slot holds the plugin's hooks and pages behind
// other plugins' work. It waits for the VM, takes a slot if one is free, and
// otherwise gives the VM back, waits for a slot, and takes the VM only if it is
// free, until both come together.
func (pm *PluginManager) acquireSlotAndVM(work asyncWork, deadline time.Time, revoked <-chan struct{}) (*vmMutex, asyncOutcome) {
	outcomeOf := func(err error) asyncOutcome {
		if errors.Is(err, errJobDidNotStart) {
			return asyncGaveUp
		}
		return asyncRevoked
	}
	for {
		mu, err := work.lock(true)
		if err != nil {
			return nil, outcomeOf(err)
		}
		select {
		case pm.actionSemaphore <- struct{}{}:
			return mu, asyncRan
		default:
		}
		mu.Unlock()
		if got := pm.acquireJobSlotUntil(deadline, revoked); got != asyncRan {
			return nil, got
		}
		mu, err = work.lock(false)
		if err == nil {
			return mu, asyncRan
		}
		<-pm.actionSemaphore
		if !errors.Is(err, errVMBusy) {
			return nil, outcomeOf(err)
		}
	}
}

// isClosed reports whether a channel is closed, without waiting.
func isClosed(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// abandonUnstartedJob ends the in-memory entry of an execution that will never
// start because its VM went away while it waited. With a host Job the host is
// told the callback is lost — it decides whether the Job waits for another
// process or ends — and the entry goes; without one the entry is the only
// record, so it ends failed, as work whose plugin disappeared always has.
func (pm *PluginManager) abandonUnstartedJob(job *ActionJob) {
	if ref := job.hostJobRef(); ref != nil && ref.Sink != nil {
		ref.Sink.CallbackLost("plugin-unavailable")
		pm.dropUnstartedJob(job)
		return
	}
	job.mu.Lock()
	job.Status = "failed"
	job.Message = fmt.Sprintf("plugin %q is no longer available", job.PluginName)
	job.mu.Unlock()
	pm.notifyActionJobSubscribers("updated", job)
}

// settleActionJob records one execution's outcome once its callback has returned,
// and is the only place plugin background work ends a durable Job.
//
// The timing is the whole of it. mah.job_complete and mah.job_fail *request* an
// outcome; the callback that called them can keep running — sleeping, writing
// through mah.db, calling mah.http — and §3 keeps an execution's occupied capacity
// and its unresolved claim held until the runtime that owns the callback proves the
// external work is quiescent. Reporting from inside the Lua call therefore ended a
// Job whose Lua was still writing, freed the deployment's slot for it, and offered a
// Retry that a second worker could start beside the handler that had not stopped.
// The handler has returned by the time this runs, so the request is settled and this
// is where it becomes an outcome.
//
// A status the plugin set itself wins over the error the host unwound: the plugin's
// own report is about work it declared finished, and a Go-level failure raised after
// it says nothing about whether that work is done.
func (pm *PluginManager) settleActionJob(job *ActionJob, logLabel string, workErr error) {
	flushHeldProgress(job)

	job.mu.Lock()
	status := job.Status
	message := job.Message
	result := job.Result
	if status != "completed" && status != "failed" {
		if workErr != nil {
			status = "failed"
			if isAbort, reason := parseAbortError(workErr); isAbort {
				message = reason
			} else {
				message = workErr.Error()
			}
		} else {
			status = "completed"
			message = "Completed"
		}
		job.Status = status
		job.Message = message
	}
	if status == "completed" {
		job.Progress = 100
	}
	job.mu.Unlock()

	pm.notifyActionJobSubscribers("updated", job)
	if status == "failed" {
		if workErr != nil {
			log.Printf("[plugin] %s failed: %v", logLabel, workErr)
		}
		reportHostJobOnce(job, func(sink HostJobSink) error { return sink.Failed(message) })
		return
	}
	reportHostJobOnce(job, func(sink HostJobSink) error { return sink.Completed(message, result) })
}

// resolveQueuedAction finds the registration a queued action runs, once it has
// reached the head of its lane rather than when it was queued: a plugin can be
// reloaded while work waits for it, and the handler to run is the one registered
// now. It is refused unless it is still the action the submission was checked
// against — the same filters, the same kind of entity, and params that still
// validate — because a replacement that changed any of them would receive a
// payload nobody validated for it.
func (pm *PluginManager) resolveQueuedAction(job *ActionJob, params map[string]any, expectFilters string) (ActionRegistration, *lua.LState, error) {
	action, L, err := pm.FindAction(job.PluginName, job.ActionID)
	if err != nil {
		return ActionRegistration{}, nil, fmt.Errorf("plugin %q is no longer available", job.PluginName)
	}
	if err := checkActionUnchanged(action, expectFilters); err != nil {
		return ActionRegistration{}, nil, err
	}
	if action.Entity != job.EntityType {
		return ActionRegistration{}, nil, fmt.Errorf("%w: it now acts on %s, not %s", errActionChanged, action.Entity, job.EntityType)
	}
	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return ActionRegistration{}, nil, fmt.Errorf("%w: validation failed: %s: %s",
			errActionChanged, validationErrs[0].Field, validationErrs[0].Message)
	}
	return action, L, nil
}

// runAsyncActionGoroutine executes the Lua handler in a background goroutine.
//
// The VM it locks is the plugin's current one, found by name when the
// execution's turn comes, and the handler it runs is the registration found then
// (resolveQueuedAction), with the settings read then too: a plugin can be
// reloaded while its work waits.
func (pm *PluginManager) runAsyncActionGoroutine(job *ActionJob, ticket *laneTicket, revoked <-chan struct{}, entityID uint, params map[string]any, expectFilters string) asyncOutcome {
	var L *lua.LState
	return pm.executeAsyncJob(job, fmt.Sprintf("async action %q/%q", job.PluginName, job.ActionID), ticket, revoked, asyncWork{
		lock: func(wait bool) (*vmMutex, error) {
			_, current, err := pm.FindAction(job.PluginName, job.ActionID)
			if err != nil {
				return nil, errPluginGone
			}
			mu, err := pm.lockVMFor(current, wait)
			if err != nil {
				return nil, err
			}
			L = current
			return mu, nil
		},
		live: func() bool { return pm.stillRegistered(L) },
		run: func(mu *vmMutex) error {
			// Resolved again under the lock: the VM cannot be revoked while it is
			// held, so this is the registration that will run.
			action, resolved, err := pm.resolveQueuedAction(job, params, expectFilters)
			if err != nil {
				mu.Unlock()
				return err
			}
			if resolved != L {
				mu.Unlock()
				return fmt.Errorf("plugin %q is no longer available", job.PluginName)
			}
			handler := action.Handler
			settings := pm.GetPluginSettings(job.PluginName)

			// Build context table: { entity_id = N, params = {...}, settings = {...}, job_id = "..." }
			ctxData := map[string]any{
				"entity_id": entityID,
				"job_id":    job.ID,
			}
			if params != nil {
				ctxData["params"] = params
			} else {
				ctxData["params"] = map[string]any{}
			}
			if settings != nil {
				ctxData["settings"] = settings
			} else {
				ctxData["settings"] = map[string]any{}
			}

			tbl := goToLuaTable(L, ctxData)

			// The submitter is captured at enqueue (ActionJob.ownerUserID), so an
			// async action's mah.db writes are attributed to whoever ran the action
			// rather than to nobody. Background-parented: a job outlives its request.
			timeoutCtx, cancel := context.WithTimeout(invocationContextForJob(job), asyncActionTimeout)
			L.SetContext(timeoutCtx)

			err = L.CallByParam(lua.P{
				Fn:      handler,
				NRet:    1,
				Protect: true,
			}, tbl)

			L.RemoveContext()
			cancel()

			if err != nil {
				mu.Unlock()
				return err
			}

			// Parse the return value while the VM is still locked — which is what
			// this comment always claimed, while the unlock sat above the
			// conversion. An async handler can return a table the plugin holds
			// globally, and two jobs of the same plugin run one after another on
			// the same VM: converting outside the lock let one walk that table
			// while the next mutated it, which Go aborts the process for.
			ret := L.Get(-1)
			L.Pop(1)
			var parsed map[string]any
			retTbl, isTable := ret.(*lua.LTable)
			if isTable {
				parsed = luaTableToGoMap(retTbl)
			}
			mu.Unlock()

			// If the handler returned a table, treat it as the result and mark completed.
			if isTable {
				job.mu.Lock()
				// Unless the handler already decided. A handler that calls
				// mah.job_fail and then returns a diagnostic table meant to fail,
				// and overwriting that with "completed" contradicts the documented
				// contract in the direction that hides the failure.
				if job.Status == "failed" || job.Status == "cancelled" {
					job.mu.Unlock()
					return nil
				}
				job.Status = "completed"
				job.Progress = 100
				if msg, ok := parsed["message"].(string); ok {
					job.Message = msg
				} else {
					job.Message = "Completed"
				}
				job.Result = parsed
				message := job.Message
				job.mu.Unlock()
				pm.notifyActionJobSubscribers("updated", job)
				// Not reported once: this is the handler's *own* return value, and
				// the settle path below publishes the same outcome through the
				// once-guarded call. Attempting it here is only so a Job is not left
				// without an outcome if that path is never reached, and a refusal is
				// swallowed for the settle path to make good on.
				flushHeldProgress(job)
				_ = reportHostJob(job, func(sink HostJobSink) error { return sink.Completed(message, parsed) })
			}

			return nil
		},
	})
}

// runStartJobGoroutine executes a Lua callback from mah.start_job() in a background goroutine.
func (pm *PluginManager) runStartJobGoroutine(job *ActionJob, ticket *laneTicket, L *lua.LState, fn *lua.LFunction, jobID string) asyncOutcome {
	return pm.executeAsyncJob(job, fmt.Sprintf("start_job %q", job.PluginName), ticket, pm.stateRevoked(L), asyncWork{
		lock: func(wait bool) (*vmMutex, error) { return pm.lockVMFor(L, wait) },
		live: func() bool { return pm.stillRegistered(L) },
		run: func(mu *vmMutex) error {
			defer mu.Unlock()

			timeoutCtx, cancel := context.WithTimeout(invocationContextForJob(job), asyncActionTimeout)
			L.SetContext(timeoutCtx)
			defer func() {
				L.RemoveContext()
				cancel()
			}()

			return L.CallByParam(lua.P{
				Fn:      fn,
				NRet:    0,
				Protect: true,
			}, lua.LString(jobID))
		},
	})
}

// GetActionJob returns a snapshot of the action job with the given ID, or nil if not found.
func (pm *PluginManager) GetActionJob(jobID string) *ActionJob {
	pm.actionJobsMu.RLock()
	job, ok := pm.actionJobs[jobID]
	pm.actionJobsMu.RUnlock()

	if !ok {
		return nil
	}

	return job.Snapshot()
}

// actionWaitGroup returns (or creates) the WaitGroup for tracking in-flight async actions of a plugin.
func (pm *PluginManager) actionWaitGroup(pluginName string) *sync.WaitGroup {
	pm.actionJobsMu.Lock()
	defer pm.actionJobsMu.Unlock()

	wg, ok := pm.actionInFlight[pluginName]
	if !ok {
		wg = &sync.WaitGroup{}
		pm.actionInFlight[pluginName] = wg
	}
	return wg
}

// GetAllActionJobs returns snapshots of all action jobs.
func (pm *PluginManager) GetAllActionJobs() []*ActionJob {
	pm.actionJobsMu.RLock()
	defer pm.actionJobsMu.RUnlock()

	result := make([]*ActionJob, 0, len(pm.actionJobs))
	for _, job := range pm.actionJobs {
		result = append(result, job.Snapshot())
	}
	return result
}

// SubscribeActionJobs creates a channel that receives action job events.
func (pm *PluginManager) SubscribeActionJobs() chan ActionJobEvent {
	ch := make(chan ActionJobEvent, 100)

	pm.actionSubsMu.Lock()
	pm.actionSubs[ch] = struct{}{}
	pm.actionSubsMu.Unlock()

	return ch
}

// UnsubscribeActionJobs removes a subscriber channel and closes it.
func (pm *PluginManager) UnsubscribeActionJobs(ch chan ActionJobEvent) {
	pm.actionSubsMu.Lock()
	delete(pm.actionSubs, ch)
	pm.actionSubsMu.Unlock()
	close(ch)
}

// notifyActionJobSubscribers snapshots the job and sends the event to all subscribers (non-blocking).
func (pm *PluginManager) notifyActionJobSubscribers(eventType string, job *ActionJob) {
	event := ActionJobEvent{Type: eventType, Job: job.Snapshot()}

	pm.actionSubsMu.RLock()
	defer pm.actionSubsMu.RUnlock()

	for ch := range pm.actionSubs {
		select {
		case ch <- event:
		default:
			// Channel full, skip (subscriber is slow)
		}
	}
}

// ClearFinishedActionJobs removes every completed or failed action job the caller
// may see and returns the ids that went. Running and pending jobs are kept.
//
// UI bug hunt 2026-07-29, finding 40: the jobs panel shows download jobs and
// plugin action jobs in one list, so a "Clear completed" that only reached the
// download queue would leave rows the button visibly failed to remove.
//
// The ids and not a count, for the reason DownloadManager.ClearFinished gives: the
// panel has to dismiss exactly what the server cleared, and its own idea of which
// rows were finished is a snapshot taken before the request went out.
//
// visible is the caller's RBAC predicate over the job's owner, matching the
// filtering the queue and the SSE stream already apply.
func (pm *PluginManager) ClearFinishedActionJobs(visible func(owner *uint) bool) []string {
	removed := pm.ClearFinishedActionJobSnapshots(visible)
	ids := make([]string, 0, len(removed))
	for _, job := range removed {
		ids = append(ids, job.ID)
	}
	return ids
}

// ClearFinishedActionJobSnapshots removes the same rows as
// ClearFinishedActionJobs and returns the removed rows' stable handle and
// canonical target identities. The handler uses the canonical ID to ensure a
// stale in-memory ancestor clear cannot mark a handle's Retry successor as
// cleared.
func (pm *PluginManager) ClearFinishedActionJobSnapshots(visible func(owner *uint) bool) []*ActionJob {
	var removed []*ActionJob
	cleared := make([]*ActionJob, 0)

	pm.actionJobsMu.Lock()
	for id, job := range pm.actionJobs {
		job.mu.RLock()
		status := job.Status
		owner := job.ownerUserID
		canonicalID := job.CanonicalJobID
		if job.host != nil && job.host.JobID != "" {
			canonicalID = job.host.JobID
		}
		job.mu.RUnlock()

		if status != "completed" && status != "failed" {
			continue
		}
		if visible != nil && !visible(owner) {
			continue
		}
		delete(pm.actionJobs, id)
		removed = append(removed, job)
		cleared = append(cleared, &ActionJob{ID: id, CanonicalJobID: canonicalID})
	}
	pm.actionJobsMu.Unlock()

	for _, job := range removed {
		pm.notifyActionJobSubscribers("removed", job)
	}

	return cleared
}

// cleanupOldActionJobs removes completed/failed action jobs older than actionJobRetention.
func (pm *PluginManager) cleanupOldActionJobs() {
	var removed []*ActionJob

	pm.actionJobsMu.Lock()
	cutoff := time.Now().Add(-actionJobRetention)
	for id, job := range pm.actionJobs {
		job.mu.RLock()
		status := job.Status
		created := job.CreatedAt
		job.mu.RUnlock()

		if (status == "completed" || status == "failed") && created.Before(cutoff) {
			delete(pm.actionJobs, id)
			removed = append(removed, job)
		}
	}
	pm.actionJobsMu.Unlock()

	for _, job := range removed {
		pm.notifyActionJobSubscribers("removed", job)
	}
}
