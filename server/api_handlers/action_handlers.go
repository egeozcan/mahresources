package api_handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mahresources/constants"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
)

// PluginActionRunner provides access to plugin-action infrastructure.
type PluginActionRunner interface {
	PluginManager() *plugin_system.PluginManager
	ActionEntityRefReader() plugin_system.EntityRefReader
}

// actionRunRequest is the JSON body for POST /v1/jobs/action/run
type actionRunRequest struct {
	Plugin    string         `json:"plugin"`
	Action    string         `json:"action"`
	EntityIDs []uint         `json:"entity_ids"`
	Params    map[string]any `json:"params"`
}

// GetPluginActionsHandler handles GET /v1/plugin/actions
// Query params: entity (required), content_type, category_id, note_type_id
func GetPluginActionsHandler(ctx PluginActionRunner) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			w.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(w).Encode([]plugin_system.ActionRegistration{})
			return
		}

		entity := r.URL.Query().Get("entity")
		if entity == "" {
			http_utils.HandleError(fmt.Errorf("entity query parameter is required"), w, r, http.StatusBadRequest)
			return
		}

		// Build optional entity data for filter matching.
		entityData := make(map[string]any)
		if ct := r.URL.Query().Get("content_type"); ct != "" {
			entityData["content_type"] = ct
		}
		if cidStr := r.URL.Query().Get("category_id"); cidStr != "" {
			if cid, err := strconv.ParseUint(cidStr, 10, 64); err == nil {
				entityData["category_id"] = uint(cid)
			}
		}
		if ntidStr := r.URL.Query().Get("note_type_id"); ntidStr != "" {
			if ntid, err := strconv.ParseUint(ntidStr, 10, 64); err == nil {
				entityData["note_type_id"] = uint(ntid)
			}
		}

		var entityDataPtr map[string]any
		if len(entityData) > 0 {
			entityDataPtr = entityData
		}

		actions := pm.GetActions(entity, entityDataPtr)
		if actions == nil {
			actions = []plugin_system.ActionRegistration{}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(actions)
	}
}

// GetActionRunHandler handles POST /v1/jobs/action/run
// JSON body: { "plugin": "...", "action": "...", "entity_ids": [...], "params": {...} }
func GetActionRunHandler(ctx PluginActionRunner) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http_utils.HandleError(fmt.Errorf("plugin system is not available"), w, r, http.StatusServiceUnavailable)
			return
		}

		var req actionRunRequest
		limitedBody := io.LimitReader(r.Body, 1024*1024) // 1MB limit
		if err := json.NewDecoder(limitedBody).Decode(&req); err != nil {
			http_utils.HandleError(fmt.Errorf("invalid JSON body: %w", err), w, r, http.StatusBadRequest)
			return
		}

		if req.Plugin == "" || req.Action == "" {
			http_utils.HandleError(fmt.Errorf("plugin and action fields are required"), w, r, http.StatusBadRequest)
			return
		}

		if len(req.EntityIDs) == 0 {
			http_utils.HandleError(fmt.Errorf("entity_ids must contain at least one ID"), w, r, http.StatusBadRequest)
			return
		}

		// Find the action to determine sync vs async.
		action, _, err := pm.FindAction(req.Plugin, req.Action)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusNotFound)
			return
		}

		// Enforce BulkMax limit if the action defines one.
		if action.BulkMax > 0 && len(req.EntityIDs) > action.BulkMax {
			http_utils.HandleError(
				fmt.Errorf("action allows at most %d entities per request, got %d", action.BulkMax, len(req.EntityIDs)),
				w, r, http.StatusBadRequest,
			)
			return
		}

		// Validate params upfront.
		if validationErrs := plugin_system.ValidateActionParams(action, req.Params); len(validationErrs) > 0 {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": validationErrs})
			return
		}

		if action.Async {
			// Async execution: create jobs for each entity ID.
			jobIDs := make([]string, 0, len(req.EntityIDs))
			for _, eid := range req.EntityIDs {
				jobID, err := pm.RunActionAsync(req.Plugin, req.Action, eid, req.Params)
				if err != nil {
					http_utils.HandleError(fmt.Errorf("failed to start async action for entity %d: %w", eid, err), w, r, http.StatusInternalServerError)
					return
				}
				jobIDs = append(jobIDs, jobID)
			}

			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusAccepted)
			if len(jobIDs) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"job_id": jobIDs[0]})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"job_ids": jobIDs})
			}
		} else {
			// Sync execution: run for each entity ID and collect results.
			results := make([]*plugin_system.ActionResult, 0, len(req.EntityIDs))
			for _, eid := range req.EntityIDs {
				if r.Context().Err() != nil {
					http_utils.HandleError(fmt.Errorf("request cancelled"), w, r, http.StatusRequestTimeout)
					return
				}
				result, err := pm.RunAction(req.Plugin, req.Action, eid, req.Params)
				if err != nil {
					http_utils.HandleError(fmt.Errorf("action failed for entity %d: %w", eid, err), w, r, http.StatusInternalServerError)
					return
				}
				results = append(results, result)
			}

			w.Header().Set("Content-Type", constants.JSON)
			if len(results) == 1 {
				_ = json.NewEncoder(w).Encode(results[0])
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
			}
		}
	}
}

// GetActionJobHandler handles GET /v1/jobs/action/job?id=abc
func GetActionJobHandler(ctx PluginActionRunner) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http_utils.HandleError(fmt.Errorf("plugin system is not available"), w, r, http.StatusServiceUnavailable)
			return
		}

		jobID := r.URL.Query().Get("id")
		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("id query parameter is required"), w, r, http.StatusBadRequest)
			return
		}

		job := pm.GetActionJob(jobID)
		if job == nil {
			http_utils.HandleError(fmt.Errorf("action job %q not found", jobID), w, r, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(job)
	}
}
