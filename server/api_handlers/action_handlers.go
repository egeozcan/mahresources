package api_handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"net/http"
	"strconv"
)

// PluginActionRunner provides access to plugin-action infrastructure. The
// *Visible methods are consulted when the request runs as a group-limited
// principal so an action can only target entities inside that principal's
// subtree (they return true for unrestricted principals).
type PluginActionRunner interface {
	PluginManager() *plugin_system.PluginManager
	ActionEntityRefReader() plugin_system.EntityRefReader
	ActionEntityDataReader() plugin_system.ActionEntityDataReader
	ResourceVisible(id uint) bool
	NoteVisible(id uint) bool
	GroupVisible(id uint) bool
	MaxActionEntities() int
	PluginAllowsScopedPrincipals(pluginName string) bool
}

// actionScopeRestricted reports whether the request principal is group-limited,
// so its plugin actions must be confined to its subtree.
func actionScopeRestricted(p *auth.Principal) bool {
	return p != nil && !p.IsAdmin() && (p.IsScoped() || p.RequiresScope())
}

// actionPluginAccess answers, per plugin, whether this request may run that
// plugin's actions. Both action surfaces ask it: the run path refuses with it,
// and the listing declines to offer what the run path would refuse. An action is
// the most direct way there is to make a plugin's Lua run, so the operator's
// per-plugin decision has to govern it; gating the indirect surfaces (pages,
// shortcodes, slots) while leaving this one open would make the setting mean
// something different from what it says.
//
// The rule itself is auth.PluginActionAccessFor's, not a second copy of it, for
// the reason DownloadHistoryQuery gives: a listing and the mutation it leads to
// cannot be allowed to drift apart. The same predicate also filters the action
// lists the pages render (server/routes.go).
//
// It is the reach rule plus the one thing running an action is that reading a
// page is not: a write. withAuthorization refuses a guest POST
// /v1/jobs/action/run whatever the toggle says, and that refusal is above this
// handler, so a filter built on reach alone would keep offering guests buttons
// that only ever answer 403.
//
// One case diverges: a request carrying no principal is unrestricted here and
// refused there. That is this file's existing treatment of a missing principal,
// set by actionScopeRestricted, which skips the entity-visibility checks for it.
// Refusing the plugin while granting every entity would answer "who is asking?"
// two ways inside one handler. It costs nothing in a deployment: every route
// sits behind withAuthentication, which attaches a principal to every
// non-public path in either auth mode, and neither action path is public, so
// the only callers that reach it are this package's bare handler mounts. That was measured, and TestActionHandlers_BareMountsAreTestsOnly pins
// it: if that guard ever fails, this carve-out is the thing to reconsider.
func actionPluginAccess(ctx PluginActionRunner, r *http.Request) auth.PluginAccess {
	if auth.PrincipalFromContext(r.Context()) == nil {
		return func(string) bool { return true }
	}
	return auth.PluginActionAccessFor(r.Context(), ctx.PluginAllowsScopedPrincipals)
}

// entityVisibleForAction reports whether the target entity is visible to the
// (scoped) context for the action's entity type. Entity types that are not
// subtree-scoped (tags, categories, ...) are always allowed.
func entityVisibleForAction(ctx PluginActionRunner, entity string, id uint) bool {
	switch entity {
	case "resource":
		return ctx.ResourceVisible(id)
	case "note":
		return ctx.NoteVisible(id)
	case "group":
		return ctx.GroupVisible(id)
	default:
		return true
	}
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

		// Offer only what this caller could actually run. A group-limited account
		// is refused a plugin it was not opened to, so listing that plugin's
		// actions hands it a button whose only outcome is a 403. The decision is
		// read on every request, so a grant or a revocation shows up on the next
		// load rather than the next restart.
		//
		// This executes no plugin code and hides nothing an enumeration could
		// not recover elsewhere: it removes a dead control, it is not a
		// containment boundary. The run path is the boundary.
		access := actionPluginAccess(ctx, r)
		actions := pm.GetActions(entity, entityDataPtr)
		// Non-nil even when everything is filtered out: the browser maps over
		// this response, and appending to a nil slice would encode null.
		offered := make([]plugin_system.ActionRegistration, 0, len(actions))
		for _, action := range actions {
			if access(action.PluginName) {
				offered = append(offered, action)
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(offered)
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

		// And the deployment-wide ceiling, which is a different thing from
		// BulkMax: that one is the action author's policy and defaults to
		// unlimited, so it bounds nothing for the actions that never set it.
		// The async branch below creates a goroutine, a job-map entry and an
		// SSE notification per id before any of them runs, and the 1MB body
		// limit still admits on the order of 10^5 ids.
		if max := ctx.MaxActionEntities(); len(req.EntityIDs) > max {
			http_utils.HandleError(
				fmt.Errorf("at most %d entities may be submitted in one action run, got %d", max, len(req.EntityIDs)),
				w, r, http.StatusBadRequest,
			)
			return
		}

		reqPrincipal := auth.PrincipalFromContext(r.Context())
		if !actionPluginAccess(ctx, r)(req.Plugin) {
			http_utils.HandleError(
				fmt.Errorf("this plugin is not available to group-limited accounts"),
				w, r, http.StatusForbidden,
			)
			return
		}

		// RBAC: a group-limited principal may only run an action on entities
		// inside its subtree. The entity-ref params are scoped automatically
		// because the entity-ref reader runs on this request-scoped context, but
		// the primary entity_ids must be checked explicitly.
		if actionScopeRestricted(reqPrincipal) {
			for _, eid := range req.EntityIDs {
				if !entityVisibleForAction(ctx, action.Entity, eid) {
					http_utils.HandleError(
						fmt.Errorf("one or more target entities are outside your scope"),
						w, r, http.StatusForbidden,
					)
					return
				}
			}
		}

		// Drop the values of params show_when hides, once, here at the boundary.
		// The modal already strips them; a direct API caller does not, and a
		// handler is entitled to read a hidden param's absence as absence.
		plugin_system.StripHiddenParams(action, req.Params)

		// Validate params upfront.
		if validationErrs := plugin_system.ValidateActionParams(action, req.Params); len(validationErrs) > 0 {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": validationErrs})
			return
		}

		// DB-backed entity_ref validation happens here exactly once. RunAction and
		// RunActionAsync repeat the cheaper structural ValidateActionParams check
		// for defense-in-depth, but never call ValidateActionEntityRefs, so no
		// redundant DB round-trips occur during bulk fan-out.
		reader := ctx.ActionEntityRefReader()
		if reader == nil {
			http_utils.HandleError(fmt.Errorf("entity ref reader unavailable"), w, r, http.StatusInternalServerError)
			return
		}
		refErrs, err := plugin_system.ValidateActionEntityRefs(reader, action, req.Params)
		if err != nil {
			http_utils.HandleError(fmt.Errorf("entity ref validation: %w", err), w, r, http.StatusInternalServerError)
			return
		}
		if len(refErrs) > 0 {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": refErrs})
			return
		}

		// The action's own Filters decide which entities the UI OFFERS it for.
		// Re-apply them here, over the same predicate, so a direct POST cannot
		// run a PNG-only action on a PDF. One read for the whole batch, beside
		// the entity_ref read above, and skipped entirely for an action that
		// declares no filters. A mismatch — including an entity that came back
		// missing, so deleted or out of scope — vetoes the whole batch.
		if plugin_system.ActionHasEntityFilters(action) {
			filterErrs, err := plugin_system.ValidateActionEntityFilters(ctx.ActionEntityDataReader(), action, req.EntityIDs)
			if err != nil {
				http_utils.HandleError(fmt.Errorf("entity filter validation: %w", err), w, r, http.StatusInternalServerError)
				return
			}
			if len(filterErrs) > 0 {
				w.Header().Set("Content-Type", constants.JSON)
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"errors": filterErrs})
				return
			}
		}

		// The executors resolve the action by id, and an id is not a
		// generation: a disable/edit/re-enable between here and there yields a
		// different registration under the same id, whose filters these
		// entities were never checked against. The fingerprint binds the two.
		expectFilters := plugin_system.ActionFiltersFingerprint(action.Filters)

		if action.Async {
			// Async execution: create jobs for each entity ID, tagged with the
			// submitting user so the job listing/SSE only surface them to that
			// user (and admins).
			owner := principalOwnerID(reqPrincipal)
			jobIDs := make([]string, 0, len(req.EntityIDs))
			for _, eid := range req.EntityIDs {
				jobID, err := pm.RunActionAsyncForOwner(owner, req.Plugin, req.Action, eid, req.Params, expectFilters)
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
				// A cache per entity, not per request. An action is a write:
				// one that queries "unprocessed resources" and then marks the
				// current one processed would serve the next entity in the
				// batch the answer from before its own write, because two
				// entities under one owner group resolve to the same cache key
				// and nothing invalidates the cache. Within a single entity's
				// handler, repeated identical queries still collapse.
				result, err := pm.RunAction(plugin_system.WithMRQLCache(r.Context()), req.Plugin, req.Action, eid, req.Params, expectFilters)
				if err != nil {
					// A single-entity run keeps its status: there is nothing
					// partial about it, and a caller that reads 5xx as "this
					// did not happen" is right.
					if len(req.EntityIDs) == 1 {
						http_utils.HandleError(fmt.Errorf("action failed for entity %d: %w", eid, err), w, r, http.StatusInternalServerError)
						return
					}
					// In a bulk run it is already false. The entities before
					// this one ran, and every mah.db write they made is
					// committed — nothing brackets the batch in a transaction —
					// so answering 500 for the whole request describes none of
					// what happened and leaves the caller unable to tell which
					// half landed. Report it in the entity's own slot and keep
					// going; the result array is positional, which is how the
					// modal maps a failure back to an id.
					results = append(results, &plugin_system.ActionResult{
						Success: false,
						Message: err.Error(),
					})
					continue
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
		if job == nil || !jobVisibleToPrincipal(auth.PrincipalFromContext(r.Context()), job.Owner()) {
			// Non-owners get a 404 (not 403) so job IDs can't be enumerated.
			http_utils.HandleError(fmt.Errorf("action job %q not found", jobID), w, r, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(job)
	}
}
