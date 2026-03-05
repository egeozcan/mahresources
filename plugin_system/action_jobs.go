package plugin_system

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	ID         string         `json:"id"`
	Source     string         `json:"source"`     // always "plugin"
	PluginName string         `json:"pluginName"`
	ActionID   string         `json:"actionId"`
	Label      string         `json:"label"`
	EntityID   uint           `json:"entityId"`
	EntityType string         `json:"entityType"`
	Status     string         `json:"status"`            // pending, running, completed, failed
	Progress   int            `json:"progress"`           // 0-100
	Message    string         `json:"message"`
	Result     map[string]any `json:"result,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	mu           sync.RWMutex
	lastNotified time.Time // tracks when the last SSE notification was sent for throttling
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
		ID:         j.ID,
		Source:     j.Source,
		PluginName: j.PluginName,
		ActionID:   j.ActionID,
		Label:      j.Label,
		EntityID:   j.EntityID,
		EntityType: j.EntityType,
		Status:     j.Status,
		Progress:   j.Progress,
		Message:    j.Message,
		CreatedAt:  j.CreatedAt,
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

// RunActionAsync validates and starts an async action execution, returning the job ID.
func (pm *PluginManager) RunActionAsync(pluginName, actionID string, entityID uint, params map[string]any) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	action, L, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return "", err
	}

	// Validate params.
	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return "", fmt.Errorf("validation failed: %s: %s", validationErrs[0].Field, validationErrs[0].Message)
	}

	jobID := generateActionJobID()
	job := &ActionJob{
		ID:         jobID,
		Source:     "plugin",
		PluginName: pluginName,
		ActionID:   actionID,
		Label:      action.Label,
		EntityID:   entityID,
		EntityType: action.Entity,
		Status:     "pending",
		Progress:   0,
		Message:    "Waiting to start...",
		CreatedAt:  time.Now(),
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

// executeAsyncJob is the common scaffold for running an async job goroutine.
// It handles panic recovery, semaphore, status transitions, error handling, and default completion.
// The work function performs the actual Lua call and returns its error.
func (pm *PluginManager) executeAsyncJob(job *ActionJob, logLabel string, work func() error) {
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
	pm.actionSemaphore <- struct{}{}
	defer func() { <-pm.actionSemaphore }()

	// Mark as running.
	job.mu.Lock()
	job.Status = "running"
	job.Message = "Running..."
	job.mu.Unlock()
	pm.notifyActionJobSubscribers("updated", job)

	err := work()

	if err != nil {
		// Check if the Lua code already set the job to completed/failed via mah.job_complete/mah.job_fail.
		job.mu.RLock()
		alreadyDone := job.Status == "completed" || job.Status == "failed"
		job.mu.RUnlock()
		if alreadyDone {
			return
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
		return
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

		mu := pm.VMLock(L)
		mu.Lock()

		tbl := goToLuaTable(L, ctxData)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), asyncActionTimeout)
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

		// Parse the return value while VM is still locked.
		ret := L.Get(-1)
		L.Pop(1)
		mu.Unlock()

		// If the handler returned a table, treat it as the result and mark completed.
		if retTbl, ok := ret.(*lua.LTable); ok {
			parsed := luaTableToGoMap(retTbl)
			job.mu.Lock()
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
		mu := pm.VMLock(L)
		mu.Lock()
		defer mu.Unlock()

		timeoutCtx, cancel := context.WithTimeout(context.Background(), asyncActionTimeout)
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
