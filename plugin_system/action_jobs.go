package plugin_system

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
type ActionJob struct {
	ID           string         `json:"id"`
	Source       string         `json:"source"` // always "plugin"
	PluginName   string         `json:"pluginName"`
	ActionID     string         `json:"actionId"`
	Label        string         `json:"label"`
	EntityID     uint           `json:"entityId"`
	EntityType   string         `json:"entityType"`
	Status       string         `json:"status"`   // pending, running, completed, failed
	Progress     int            `json:"progress"` // 0-100
	Message      string         `json:"message"`
	Result       map[string]any `json:"result,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	mu           sync.RWMutex
	lastNotified time.Time // tracks when the last SSE notification was sent for throttling
	// ownerUserID is the user that submitted the action (RBAC). It is never
	// serialized to JSON; callers read it via Owner() to decide visibility so a
	// non-admin only sees the jobs it created.
	ownerUserID *uint
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

// Snapshot returns a copy of the ActionJob safe for serialization.
func (j *ActionJob) Snapshot() ActionJob { //nolint:govet // returns a field-by-field copy; mu is intentionally zero-valued
	j.mu.RLock()
	defer j.mu.RUnlock()

	snap := ActionJob{
		ID:          j.ID,
		Source:      j.Source,
		PluginName:  j.PluginName,
		ActionID:    j.ActionID,
		Label:       j.Label,
		EntityID:    j.EntityID,
		EntityType:  j.EntityType,
		Status:      j.Status,
		Progress:    j.Progress,
		Message:     j.Message,
		CreatedAt:   j.CreatedAt,
		ownerUserID: j.ownerUserID,
	}

	// Shallow copy of Result is safe because results are write-once:
	// set exactly once when the job completes, never mutated afterward.
	if j.Result != nil {
		snap.Result = make(map[string]any, len(j.Result))
		for k, v := range j.Result {
			snap.Result[k] = v
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
	}

	pm.actionJobsMu.Lock()
	pm.actionJobs[jobID] = job
	pm.actionJobsMu.Unlock()

	pm.notifyActionJobSubscribers("added", job)

	// Capture the handler and settings before spawning goroutine.
	handler := action.Handler
	settings := pm.GetPluginSettings(pluginName)

	// Track in-flight async actions so DisablePlugin can wait for completion.
	wg := pm.actionWaitGroup(pluginName)
	wg.Add(1)

	go func() {
		defer wg.Done()
		pm.runAsyncActionGoroutine(job, L, handler, entityID, params, settings)
	}()

	return jobID, nil
}

// acquireJobSlot takes one of the concurrent-async-job slots.
//
// A non-positive wait waits forever, which is what an action or a start_job
// wants: a user asked for that work and nothing else will ask again.
//
// A positive wait is for a caller that must not block indefinitely because it is
// holding something while it waits. The scheduler is the only such caller today,
// and what it holds is a database claim on the schedule row; a claim of
// unbounded lifetime cannot have a meaningful expiry, and its expiry is the only
// thing stopping a second process running the same schedule.
func (pm *PluginManager) acquireJobSlot(wait time.Duration) bool {
	if wait <= 0 {
		pm.actionSemaphore <- struct{}{}
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case pm.actionSemaphore <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
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

// executeAsyncJob is the common scaffold for running an async job goroutine.
// It handles panic recovery, semaphore, status transitions, error handling, and default completion.
// The work function performs the actual Lua call and returns its error.
func (pm *PluginManager) executeAsyncJob(job *ActionJob, logLabel string, work func() error) {
	pm.executeAsyncJobWithin(job, logLabel, 0, work)
}

// executeAsyncJobWithin is executeAsyncJob with a bound on how long it will wait
// for a free slot, and it reports whether the job ran at all.
//
// Returning false means nothing was touched: no status transition, no
// notification, no work. That is what lets a caller holding a resource treat a
// full budget as "not now" and give the resource back, rather than parking on
// the semaphore while it holds it.
// errJobDidNotStart is a work function's way of saying it never began.
//
// executeAsyncJobWithin's contract is "ran means the job entered its work", and
// a work function that spends its own bounded wait on something it could not get
// — today, a schedule waiting on the plugin's VM lock — has not. Without this it
// looked identical to a job that ran and failed: the panel announced "Action
// failed" to a screen reader, kept a failed row for a handler that was never
// entered, and the application log blamed the plugin for it. The alternative was
// to correct the status afterwards from the caller, and that was tried first: it
// left two mechanisms for one condition and still emitted the failure event.
var errJobDidNotStart = errors.New("the job never entered its work")

func (pm *PluginManager) executeAsyncJobWithin(job *ActionJob, logLabel string, wait time.Duration, work func() error) (ran bool) {
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.Message = fmt.Sprintf("panic: %v", r)
			job.mu.Unlock()
			pm.notifyActionJobSubscribers("updated", job)
			log.Printf("[plugin] panic in %s: %v", logLabel, r)
		}
	}()

	// Acquire semaphore slot (limits concurrent async actions).
	if !pm.acquireJobSlot(wait) {
		return false
	}
	// Set before any work, so a panic recovered above still reports that the job
	// ran: it did, and it failed, which is a different thing from never starting.
	ran = true
	defer func() { <-pm.actionSemaphore }()

	// Mark as running.
	job.mu.Lock()
	job.Status = "running"
	job.Message = "Running..."
	job.mu.Unlock()
	pm.notifyActionJobSubscribers("updated", job)

	err := work()

	if errors.Is(err, errJobDidNotStart) {
		// Nothing was entered, so there is no outcome to record and nothing to
		// tell subscribers: the caller removes the job entry, and a status
		// written here would be the last word the panel retained about it.
		return false
	}

	if err != nil {
		// Check if the Lua code already set the job to completed/failed via mah.job_complete/mah.job_fail.
		job.mu.RLock()
		alreadyDone := job.Status == "completed" || job.Status == "failed"
		job.mu.RUnlock()
		if alreadyDone {
			return true
		}

		errMsg := err.Error()
		if isAbort, reason := parseAbortError(err); isAbort {
			errMsg = reason
		}

		job.mu.Lock()
		job.Status = "failed"
		job.Message = errMsg
		job.mu.Unlock()
		pm.notifyActionJobSubscribers("updated", job)
		log.Printf("[plugin] %s failed: %v", logLabel, err)
		return true
	}

	// If the work function didn't already set a terminal status, mark completed.
	job.mu.RLock()
	alreadyDone := job.Status == "completed" || job.Status == "failed"
	job.mu.RUnlock()
	if !alreadyDone {
		job.mu.Lock()
		job.Status = "completed"
		job.Progress = 100
		job.Message = "Completed"
		job.mu.Unlock()
		pm.notifyActionJobSubscribers("updated", job)
	}
	return true
}

// runAsyncActionGoroutine executes the Lua handler in a background goroutine.
func (pm *PluginManager) runAsyncActionGoroutine(job *ActionJob, L *lua.LState, handler *lua.LFunction, entityID uint, params map[string]any, settings map[string]any) {
	pm.executeAsyncJob(job, fmt.Sprintf("async action %q/%q", job.PluginName, job.ActionID), func() error {
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

		mu := pm.LockVM(L)
		if mu == nil {
			return fmt.Errorf("plugin %q is no longer available", job.PluginName)
		}

		tbl := goToLuaTable(L, ctxData)

		// The submitter is captured at enqueue (ActionJob.ownerUserID), so an
		// async action's mah.db writes are attributed to whoever ran the action
		// rather than to nobody. Background-parented: a job outlives its request.
		timeoutCtx, cancel := context.WithTimeout(invocationContextForJob(job), asyncActionTimeout)
		L.SetContext(timeoutCtx)

		err := L.CallByParam(lua.P{
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
		// globally, and two jobs of the same plugin run one after another on the
		// same VM: converting outside the lock let one walk that table while the
		// next mutated it, which Go aborts the process for.
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
			job.mu.Unlock()
			pm.notifyActionJobSubscribers("updated", job)
		}

		return nil
	})
}

// runStartJobGoroutine executes a Lua callback from mah.start_job() in a background goroutine.
func (pm *PluginManager) runStartJobGoroutine(job *ActionJob, L *lua.LState, fn *lua.LFunction, jobID string) {
	pm.executeAsyncJob(job, fmt.Sprintf("start_job %q", job.PluginName), func() error {
		mu := pm.LockVM(L)
		if mu == nil {
			return fmt.Errorf("plugin %q is no longer available", job.PluginName)
		}
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

	snap := job.Snapshot()
	return &snap
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
func (pm *PluginManager) GetAllActionJobs() []ActionJob {
	pm.actionJobsMu.RLock()
	defer pm.actionJobsMu.RUnlock()

	result := make([]ActionJob, 0, len(pm.actionJobs))
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
	snap := job.Snapshot() //nolint:govet // snapshot intentionally copies with zero-valued mutex
	event := ActionJobEvent{Type: eventType, Job: &snap}

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
	var removed []*ActionJob
	ids := make([]string, 0)

	pm.actionJobsMu.Lock()
	for id, job := range pm.actionJobs {
		job.mu.RLock()
		status := job.Status
		owner := job.ownerUserID
		job.mu.RUnlock()

		if status != "completed" && status != "failed" {
			continue
		}
		if visible != nil && !visible(owner) {
			continue
		}
		delete(pm.actionJobs, id)
		removed = append(removed, job)
		// The map key rather than job.ID: same value, and it needs no lock.
		ids = append(ids, id)
	}
	pm.actionJobsMu.Unlock()

	for _, job := range removed {
		pm.notifyActionJobSubscribers("removed", job)
	}

	return ids
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
