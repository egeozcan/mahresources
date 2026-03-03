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
	asyncActionTimeout     = 5 * time.Minute
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
	CreatedAt  time.Time      `json:"createdAt"`
	mu         sync.RWMutex
}

// ActionJobEvent represents a change in action job state for SSE broadcasting.
type ActionJobEvent struct {
	Type string     `json:"type"` // "added", "updated", "removed"
	Job  *ActionJob `json:"job"`
}

// Snapshot returns a copy of the ActionJob safe for serialization.
func (j *ActionJob) Snapshot() ActionJob {
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

	pm.notifyActionJobSubscribers(ActionJobEvent{Type: "added", Job: job})

	// Capture the handler and settings before spawning goroutine.
	handler := action.Handler
	settings := pm.GetPluginSettings(pluginName)

	go pm.runAsyncActionGoroutine(job, L, handler, entityID, params, settings)

	return jobID, nil
}

// runAsyncActionGoroutine executes the Lua handler in a background goroutine.
func (pm *PluginManager) runAsyncActionGoroutine(job *ActionJob, L *lua.LState, handler *lua.LFunction, entityID uint, params map[string]any, settings map[string]any) {
	// Panic recovery: mark job as failed on any panic.
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.Message = fmt.Sprintf("panic: %v", r)
			job.mu.Unlock()
			pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
			log.Printf("[plugin] panic in async action %q/%q: %v", job.PluginName, job.ActionID, r)
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
	pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})

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

	// Acquire the VM lock for the Lua call.
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
		pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
		log.Printf("[plugin] async action %q/%q failed: %v", job.PluginName, job.ActionID, err)
		return
	}

	// Parse the return value.
	ret := L.Get(-1)
	L.Pop(1)
	mu.Unlock()

	// Check if the Lua code already set the job to completed/failed via mah.job_complete/mah.job_fail.
	job.mu.RLock()
	alreadyDone := job.Status == "completed" || job.Status == "failed"
	job.mu.RUnlock()
	if alreadyDone {
		return
	}

	// If the handler returned a table, treat it as the result.
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
	} else {
		job.mu.Lock()
		job.Status = "completed"
		job.Progress = 100
		job.Message = "Completed"
		job.mu.Unlock()
	}

	pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
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

// notifyActionJobSubscribers sends an event to all action job subscribers (non-blocking).
func (pm *PluginManager) notifyActionJobSubscribers(event ActionJobEvent) {
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
	pm.actionJobsMu.Lock()
	defer pm.actionJobsMu.Unlock()

	cutoff := time.Now().Add(-actionJobRetention)
	for id, job := range pm.actionJobs {
		job.mu.RLock()
		status := job.Status
		created := job.CreatedAt
		job.mu.RUnlock()

		if (status == "completed" || status == "failed") && created.Before(cutoff) {
			delete(pm.actionJobs, id)
			pm.notifyActionJobSubscribers(ActionJobEvent{Type: "removed", Job: job})
		}
	}
}
