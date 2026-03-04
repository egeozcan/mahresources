package plugin_system

import (
	"context"
	"fmt"
	"log"
	"slices"

	lua "github.com/yuin/gopher-lua"
)

// ActionResult holds the outcome of executing a plugin action.
type ActionResult struct {
	Success  bool           `json:"success"`
	Message  string         `json:"message,omitempty"`
	Redirect string         `json:"redirect,omitempty"`
	JobID    string         `json:"job_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// FindAction locates a registered action by plugin name and action ID.
// It returns a copy of the action registration, the Lua state that owns the
// handler, and an error if not found.
func (pm *PluginManager) FindAction(pluginName, actionID string) (ActionRegistration, *lua.LState, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	actions, ok := pm.actions[pluginName]
	if !ok {
		return ActionRegistration{}, nil, fmt.Errorf("no plugin %q registered", pluginName)
	}

	var found bool
	var action ActionRegistration
	for _, a := range actions {
		if a.ID == actionID {
			action = a
			found = true
			break
		}
	}
	if !found {
		return ActionRegistration{}, nil, fmt.Errorf("no action %q registered for plugin %q", actionID, pluginName)
	}

	// Find the LState for this plugin by matching pluginName in pm.plugins.
	var state *lua.LState
	for i, p := range pm.plugins {
		if p.Name == pluginName {
			state = pm.states[i]
			break
		}
	}
	if state == nil {
		return ActionRegistration{}, nil, fmt.Errorf("no Lua state found for plugin %q", pluginName)
	}

	return action, state, nil
}

// ValidateActionParams validates the provided params against the action's
// parameter definitions. It checks required fields, select option validity,
// and number range constraints.
func ValidateActionParams(action ActionRegistration, params map[string]any) []ValidationError {
	var errs []ValidationError

	for _, p := range action.Params {
		val, exists := params[p.Name]

		// Required check.
		if p.Required && (!exists || val == nil || val == "") {
			errs = append(errs, ValidationError{
				Field:   p.Name,
				Message: fmt.Sprintf("%s is required", p.Label),
			})
			continue
		}

		// Skip further checks if value is absent or empty.
		if !exists || val == nil || val == "" {
			continue
		}

		switch p.Type {
		case "select":
			strVal, ok := val.(string)
			if !ok {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be a string", p.Label),
				})
				continue
			}
			if !slices.Contains(p.Options, strVal) {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be one of the available options", p.Label),
				})
			}

		case "number":
			var numVal float64
			switch v := val.(type) {
			case float64:
				numVal = v
			case int:
				numVal = float64(v)
			case int64:
				numVal = float64(v)
			default:
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be a number", p.Label),
				})
				continue
			}

			if p.Min != nil && numVal < *p.Min {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be at least %v", p.Label, *p.Min),
				})
			}
			if p.Max != nil && numVal > *p.Max {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be at most %v", p.Label, *p.Max),
				})
			}
		}
	}

	return errs
}

// RunAction executes a registered plugin action synchronously. It locates
// the action, validates params, builds a Lua context table, calls the
// handler, and parses the returned table into an ActionResult.
func (pm *PluginManager) RunAction(pluginName, actionID string, entityID uint, params map[string]any) (*ActionResult, error) {
	if pm.closed.Load() {
		return nil, fmt.Errorf("plugin manager is closed")
	}

	action, L, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return nil, err
	}

	// Validate params.
	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return nil, fmt.Errorf("validation failed: %s: %s", validationErrs[0].Field, validationErrs[0].Message)
	}

	// Build context table: { entity_id = N, params = {...}, settings = {...} }
	ctxData := map[string]any{
		"entity_id": entityID,
	}
	if params != nil {
		ctxData["params"] = params
	} else {
		ctxData["params"] = map[string]any{}
	}

	settings := pm.GetPluginSettings(pluginName)
	if settings != nil {
		ctxData["settings"] = settings
	} else {
		ctxData["settings"] = map[string]any{}
	}

	// Acquire the VM lock.
	mu := pm.VMLock(L)
	mu.Lock()
	defer mu.Unlock()

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
	L.SetContext(timeoutCtx)

	err = L.CallByParam(lua.P{
		Fn:      action.Handler,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		if isAbort, reason := parseAbortError(err); isAbort {
			return &ActionResult{
				Success: false,
				Message: reason,
			}, nil
		}
		log.Printf("[plugin] warning: action %q/%q returned error: %v", pluginName, actionID, err)
		return nil, fmt.Errorf("action handler error: %w", err)
	}

	// Parse the return value.
	ret := L.Get(-1)
	L.Pop(1)

	result := &ActionResult{}
	if retTbl, ok := ret.(*lua.LTable); ok {
		parsed := luaTableToGoMap(retTbl)

		if v, ok := parsed["success"].(bool); ok {
			result.Success = v
		}
		if v, ok := parsed["message"].(string); ok {
			result.Message = v
		}
		if v, ok := parsed["redirect"].(string); ok {
			result.Redirect = v
		}
		if v, ok := parsed["job_id"].(string); ok {
			result.JobID = v
		}
		if v, ok := parsed["data"].(map[string]any); ok {
			result.Data = v
		}
	}

	return result, nil
}
