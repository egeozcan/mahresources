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

// paramVisible evaluates a param's show_when against the submitted values.
//
// This mirrors isParamVisible in src/components/pluginActionModal.js, which is
// the specification: every key must match (AND), an array-valued expectation
// means "one of", and anything else is compared for equality. Numbers are
// compared numerically because a value that arrived as JSON and one that came
// from a Lua registration are both float64 but a caller may send an int.
//
// A controller that is itself absent from the submitted map makes the dependent
// param invisible. That is the one place server and client can disagree — the
// client evaluates against its live form state, where a hidden controller still
// holds a value — and it errs toward not validating rather than toward
// rejecting an input the user was never shown.
func paramVisible(p ActionParam, params map[string]any) bool {
	if len(p.ShowWhen) == 0 {
		return true
	}
	for key, expected := range p.ShowWhen {
		actual, exists := params[key]
		if !exists {
			return false
		}
		if options, ok := expected.([]any); ok {
			matched := false
			for _, option := range options {
				if paramValuesEqual(option, actual) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
			continue
		}
		if !paramValuesEqual(expected, actual) {
			return false
		}
	}
	return true
}

// paramValuesEqual compares two submitted-value-shaped values, treating all
// numeric types as comparable so an int from JSON matches a float64 from Lua.
//
// Only scalars can match. A submitted value that is a JSON object or array is
// never equal to anything — and must not reach ==, which panics on an
// uncomparable operand rather than returning false.
func paramValuesEqual(expected, actual any) bool {
	if en, ok := paramNumeric(expected); ok {
		an, ok := paramNumeric(actual)
		return ok && en == an
	}
	switch e := expected.(type) {
	case string:
		a, ok := actual.(string)
		return ok && e == a
	case bool:
		a, ok := actual.(bool)
		return ok && e == a
	case nil:
		return actual == nil
	default:
		// Registration rejects non-scalar expectations, so this is unreachable
		// for a loaded plugin; answer false rather than assume.
		return false
	}
}

func paramNumeric(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	default:
		return 0, false
	}
}

// StripHiddenParams removes the values of params their show_when hides.
//
// Called once, at the request boundary, before the params are handed on. The
// browser already strips them, but a direct API caller does not, so a handler
// that assumed show_when implied absence was wrong for exactly that caller.
//
// Deliberately separate from ValidateActionParams, which runs a second time
// inside RunAction as defense-in-depth: a validator that also deleted would see
// a different map on its second pass, and a param chained onto one it had just
// dropped would be discarded on the way through.
func StripHiddenParams(action ActionRegistration, params map[string]any) {
	if len(params) == 0 {
		return
	}
	hidden := make([]string, 0, len(action.Params))
	for _, p := range action.Params {
		if !paramVisible(p, params) {
			hidden = append(hidden, p.Name)
		}
	}
	// Collected first: deleting inside the loop would change the answer for
	// params evaluated after it.
	for _, name := range hidden {
		delete(params, name)
	}
}

// ValidateActionParams validates the provided params against the action's
// parameter definitions. It checks required fields, select option validity,
// and number range constraints.
//
// Visibility is resolved first, from the submitted values: a param its
// show_when hides is not validated, because the user was never shown it. This
// function does not mutate params — see StripHiddenParams.
func ValidateActionParams(action ActionRegistration, params map[string]any) []ValidationError {
	var errs []ValidationError

	for _, p := range action.Params {
		if !paramVisible(p, params) {
			continue
		}

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
		case "boolean":
			// Everything except false and nil is truthy in Lua, so an
			// unvalidated `[]` posted for a confirmation flag arrives as a
			// table and reads as "yes" to a handler gating a delete on it.
			if _, ok := val.(bool); !ok {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be true or false", p.Label),
				})
			}

		case "text", "textarea", "hidden":
			if _, ok := val.(string); !ok {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s must be a string", p.Label),
				})
			}

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

		case "entity_ref":
			if !p.Multi {
				// Expect single number (or null).
				var id float64
				switch v := val.(type) {
				case float64:
					id = v
				default:
					errs = append(errs, ValidationError{
						Field:   p.Name,
						Message: fmt.Sprintf("%s: expected single ID number", p.Label),
					})
					continue
				}
				// Reject non-integers (e.g. 1.5), zero, and negatives by round-tripping through uint.
				if id <= 0 || id != float64(uint(id)) {
					errs = append(errs, ValidationError{
						Field:   p.Name,
						Message: fmt.Sprintf("%s: ID must be a positive integer", p.Label),
					})
				}
				continue
			}
			// Multi: expect []any of float64.
			arr, ok := val.([]any)
			if !ok {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s: expected array of IDs", p.Label),
				})
				continue
			}
			// Required check for multi: the top-level required check above
			// treats any present slice as "exists", so an empty array would
			// pass even when Required=true and Min is unset.
			if p.Required && len(arr) == 0 {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s is required", p.Label),
				})
				continue
			}
			if p.Min != nil && float64(len(arr)) < *p.Min {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s: must have at least %v entries", p.Label, *p.Min),
				})
			}
			// Max=0 is treated as "unlimited" per spec; a real cap requires Max>0.
			if p.Max != nil && *p.Max > 0 && float64(len(arr)) > *p.Max {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s: must have at most %v entries", p.Label, *p.Max),
				})
			}
			for _, v := range arr {
				n, ok := v.(float64)
				// Reject non-integers (e.g. 1.5), zero, and negatives by round-tripping through uint.
				if !ok || n <= 0 || n != float64(uint(n)) {
					errs = append(errs, ValidationError{
						Field:   p.Name,
						Message: fmt.Sprintf("%s: each ID must be a positive integer", p.Label),
					})
					break
				}
			}
		}
	}

	return errs
}

// ValidateActionEntityRefs performs DB-backed validation of entity_ref params.
// Returns ([]ValidationError, nil) for user-correctable problems (missing or
// filter-rejected IDs); returns (nil, error) when the reader/DB fails.
//
// Caller (typically GetActionRunHandler) must run ValidateActionParams first
// to confirm structural correctness; this function assumes shapes are valid.
func ValidateActionEntityRefs(reader EntityRefReader, action ActionRegistration, params map[string]any) ([]ValidationError, error) {
	var errs []ValidationError

	for _, p := range action.Params {
		if p.Type != "entity_ref" {
			continue
		}
		val, exists := params[p.Name]
		if !exists || val == nil {
			continue
		}

		// Coerce IDs into []uint.
		var ids []uint
		if p.Multi {
			arr, ok := val.([]any)
			if !ok {
				continue // structural validator would have caught this
			}
			for _, v := range arr {
				if n, ok := v.(float64); ok && n > 0 {
					ids = append(ids, uint(n))
				}
			}
		} else {
			if n, ok := val.(float64); ok && n > 0 {
				ids = []uint{uint(n)}
			}
		}
		if len(ids) == 0 {
			continue
		}

		// Resolve effective filter.
		filter := action.Filters
		if p.Filters != nil {
			filter = *p.Filters
		}

		// Dispatch to the reader.
		var matched []uint
		var err error
		switch p.Entity {
		case "resource":
			matched, err = reader.ResourcesMatching(ids, filter)
		case "note":
			matched, err = reader.NotesMatching(ids, filter)
		case "group":
			matched, err = reader.GroupsMatching(ids, filter)
		default:
			return nil, fmt.Errorf("validating entity refs for param %q: unknown entity type %q", p.Name, p.Entity)
		}
		if err != nil {
			return nil, fmt.Errorf("validating entity refs for param %q: %w", p.Name, err)
		}

		// Compute set difference: requested - matched.
		matchedSet := make(map[uint]bool, len(matched))
		for _, id := range matched {
			matchedSet[id] = true
		}
		for _, id := range ids {
			if !matchedSet[id] {
				errs = append(errs, ValidationError{
					Field:   p.Name,
					Message: fmt.Sprintf("%s: ID %d not found or does not match filter", p.Label, id),
				})
			}
		}
	}

	return errs, nil
}

// RunAction executes a registered plugin action synchronously. It locates
// the action, validates params, builds a Lua context table, calls the
// handler, and parses the returned table into an ActionResult.
//
// ctx is the caller's request context. Deriving the Lua deadline from it means
// an abandoned request stops the handler instead of holding the plugin's VM
// lock for the full timeout, and puts the per-request MRQL cache within reach
// of mah.db.mrql_query. The async path has no request and builds its own.
func (pm *PluginManager) RunAction(ctx context.Context, pluginName, actionID string, entityID uint, params map[string]any) (*ActionResult, error) {
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

	// Acquire the VM lock. A nil return means the plugin was disabled between the
	// registry lookup and here, so its state is closing and must not be touched.
	mu := pm.LockVM(L)
	if mu == nil {
		return nil, fmt.Errorf("plugin %q is no longer available", action.PluginName)
	}
	defer mu.Unlock()

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(vmParentContext(ctx), luaExecTimeout)
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
