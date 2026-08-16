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
	"strings"

	"github.com/gorilla/mux"
)

type pluginListItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Settings    any    `json:"settings,omitempty"`
	Values      any    `json:"values,omitempty"`

	// Legacy is true for a plugin that declares no manifest at all: it keeps
	// the full mah surface, which is why Capabilities below lists everything.
	Legacy     bool `json:"legacy"`
	APIVersion int  `json:"api_version,omitempty"`

	// Capabilities is the effective set (db:write implies db:read), and
	// CapabilityLabels is the human sentence for each one, so an operator reads
	// a described power rather than a slug.
	Capabilities     []string          `json:"capabilities"`
	CapabilityLabels map[string]string `json:"capability_labels"`

	// Network is the declared egress allowlist in canonical display form. An
	// empty list means "any public host" — the broadest policy, not the absence
	// of network access.
	Network           []string `json:"network,omitempty"`
	AllowPrivateHosts bool     `json:"allow_private_hosts"`

	Dependencies []string `json:"dependencies,omitempty"`

	// MinAppVersion is informational only: it is parsed and displayed, never
	// enforced.
	MinAppVersion string `json:"min_app_version,omitempty"`
}

// capabilityLabels maps each capability to its human sentence, skipping any
// that has no label.
func capabilityLabels(caps []string) map[string]string {
	labels := make(map[string]string, len(caps))
	for _, c := range caps {
		if label, ok := plugin_system.CapabilityLabels[c]; ok {
			labels[c] = label
		}
	}
	return labels
}

func GetPluginsManageHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			w.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(w).Encode([]pluginListItem{})
			return
		}

		discovered := pm.DiscoveredPlugins()
		states, _ := ctx.GetPluginStates()

		stateMap := make(map[string]struct {
			enabled  bool
			settings string
		})
		for _, s := range states {
			stateMap[s.PluginName] = struct {
				enabled  bool
				settings string
			}{s.Enabled, s.SettingsJSON}
		}

		var items []pluginListItem
		for _, dp := range discovered {
			caps := dp.Manifest.Capabilities().Sorted()
			item := pluginListItem{
				Name:              dp.Name,
				Version:           dp.Version,
				Description:       dp.Description,
				Settings:          dp.Settings,
				Legacy:            !dp.Manifest.Declared,
				APIVersion:        dp.Manifest.APIVersion,
				Capabilities:      caps,
				CapabilityLabels:  capabilityLabels(caps),
				Network:           dp.Manifest.NetworkDisplay(),
				AllowPrivateHosts: dp.Manifest.AllowPrivateHosts,
				Dependencies:      dp.Manifest.Dependencies,
				MinAppVersion:     dp.Manifest.MinAppVersion,
			}
			if s, ok := stateMap[dp.Name]; ok {
				item.Enabled = s.enabled
				if s.settings != "" {
					var vals map[string]any
					if err := json.Unmarshal([]byte(s.settings), &vals); err == nil {
						item.Values = vals
					}
				}
			}
			items = append(items, item)
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(items)
	}
}

func GetPluginEnableHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(
				fmt.Errorf("missing plugin name"),
				w, r, http.StatusBadRequest,
			)
			return
		}

		if err := ctx.SetPluginEnabled(name, true); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name, "enabled": true})
	}
}

func GetPluginDisableHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(
				fmt.Errorf("missing plugin name"),
				w, r, http.StatusBadRequest,
			)
			return
		}

		if err := ctx.SetPluginEnabled(name, false); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name, "enabled": false})
	}
}

// GetPluginPurgeDataHandler deletes all KV data for a disabled plugin.
func GetPluginPurgeDataHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http_utils.HandleError(fmt.Errorf("missing plugin name"), w, r, http.StatusBadRequest)
			return
		}

		pm := ctx.PluginManager()
		if pm != nil && pm.IsEnabled(name) {
			http_utils.HandleError(fmt.Errorf("cannot purge data for enabled plugin — disable it first"), w, r, http.StatusBadRequest)
			return
		}

		if err := ctx.PluginKVPurge(name); err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name})
	}
}

func GetPluginSettingsHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			name = strings.TrimSpace(r.FormValue("name"))
		}
		if name == "" {
			http_utils.HandleError(
				fmt.Errorf("missing plugin name"),
				w, r, http.StatusBadRequest,
			)
			return
		}

		var values map[string]any
		limitedBody := io.LimitReader(r.Body, 64*1024) // 64KB limit for plugin settings
		if err := json.NewDecoder(limitedBody).Decode(&values); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		validationErrors, err := ctx.SavePluginSettings(name, values)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		if len(validationErrors) > 0 {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": validationErrors})
			return
		}

		if http_utils.RedirectIfHTMLAccepted(w, r, "/plugins/manage") {
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": name})
	}
}

// GetPluginBlockRenderHandler renders a plugin block type's HTML.
func GetPluginBlockRenderHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(w, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		vars := mux.Vars(r)
		pluginName := vars["pluginName"]
		if pluginName == "" {
			http.Error(w, "plugin name required", http.StatusBadRequest)
			return
		}

		blockID := uint(http_utils.GetIntQueryParameter(r, "blockId", 0))
		if blockID == 0 {
			http.Error(w, "blockId required", http.StatusBadRequest)
			return
		}

		mode := r.URL.Query().Get("mode")
		if mode != "view" && mode != "edit" {
			http.Error(w, "mode must be 'view' or 'edit'", http.StatusBadRequest)
			return
		}

		block, err := ctx.GetBlock(blockID)
		if err != nil {
			http.Error(w, "block not found", http.StatusNotFound)
			return
		}

		if !strings.HasPrefix(block.Type, "plugin:"+pluginName+":") {
			http.Error(w, "block type does not belong to this plugin", http.StatusBadRequest)
			return
		}

		note, err := ctx.GetNote(block.NoteID)
		if err != nil {
			http.Error(w, "note not found", http.StatusNotFound)
			return
		}

		var contentMap map[string]any
		if block.Content != nil {
			_ = json.Unmarshal(block.Content, &contentMap)
		}
		if contentMap == nil {
			contentMap = map[string]any{}
		}

		var stateMap map[string]any
		if block.State != nil {
			_ = json.Unmarshal(block.State, &stateMap)
		}
		if stateMap == nil {
			stateMap = map[string]any{}
		}

		var noteTypeID uint
		if note.NoteTypeId != nil {
			noteTypeID = *note.NoteTypeId
		}

		renderCtx := plugin_system.BlockRenderContext{
			Block: plugin_system.BlockRenderData{
				ID:       block.ID,
				Content:  contentMap,
				State:    stateMap,
				Position: block.Position,
			},
			Note: plugin_system.NoteRenderData{
				ID:         note.ID,
				Name:       note.Name,
				NoteTypeID: noteTypeID,
			},
			Settings: pm.GetPluginSettings(pluginName),
		}

		// The request context, so an abandoned render stops instead of holding
		// this plugin's VM lock, and so identical MRQL queries inside the
		// render collapse to one execution.
		html, err := pm.RenderBlock(plugin_system.WithMRQLCache(r.Context()), pluginName, block.Type, mode, renderCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}
}

// maxSafeJSInteger is 2^53-1: the browser carries entity ids as JS numbers, and
// past this they stop being distinguishable from their neighbours.
const maxSafeJSInteger = 1<<53 - 1

// PluginDisplayTypeInfo describes one installed display renderer.
type PluginDisplayTypeInfo struct {
	Type       string `json:"type"`       // full namespaced name, for "x-display"
	Label      string `json:"label"`      // human-readable, declared by the plugin
	PluginName string `json:"pluginName"` // owning plugin
}

// GetPluginDisplayTypesHandler lists every registered plugin display type, so a
// schema author can pick one instead of hand-typing the magic string that
// "x-display" needs. It mirrors GetBlockTypesHandler, which is what the block
// picker consumes.
//
// Deliberately routed under /v1/plugin/ (singular): it enumerates registrations
// and executes no Lua, so it must not land under the /v1/plugins/ prefix that
// isPluginCodePath denies to group-confined principals outright.
func GetPluginDisplayTypesHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		result := []PluginDisplayTypeInfo{}
		if pm := ctx.PluginManager(); pm != nil {
			for _, dt := range pm.GetDisplayTypes() {
				result = append(result, PluginDisplayTypeInfo{
					Type:       dt.TypeName,
					Label:      dt.Label,
					PluginName: dt.PluginName,
				})
			}
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// GetPluginDisplayRenderHandler renders a plugin display type's HTML.
func GetPluginDisplayRenderHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(w, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		vars := mux.Vars(r)
		pluginName := vars["pluginName"]
		if pluginName == "" {
			http.Error(w, "plugin name required", http.StatusBadRequest)
			return
		}

		var req struct {
			Type       string         `json:"type"`
			Value      any            `json:"value"`
			Schema     map[string]any `json:"schema"`
			FieldPath  string         `json:"field_path"`
			FieldLabel string         `json:"field_label"`
			EntityType string         `json:"entity_type"`
			EntityID   uint           `json:"entity_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Type == "" {
			http.Error(w, "type is required", http.StatusBadRequest)
			return
		}
		// The browser carries this id as a JS number, which stops representing
		// consecutive integers past 2^53-1. Beyond that the value cannot be
		// trusted to be the row that was asked for, and a renderer building a
		// link from it would send the reader somewhere else.
		if req.EntityID > maxSafeJSInteger {
			http.Error(w, "entity_id is out of range", http.StatusBadRequest)
			return
		}

		fullTypeName := "plugin:" + pluginName + ":" + req.Type

		renderCtx := plugin_system.DisplayRenderContext{
			Value:      req.Value,
			Schema:     req.Schema,
			FieldPath:  req.FieldPath,
			FieldLabel: req.FieldLabel,
			EntityType: req.EntityType,
			EntityID:   req.EntityID,
			Settings:   pm.GetPluginSettings(pluginName),
		}

		htmlStr, err := pm.RenderDisplay(plugin_system.WithMRQLCache(r.Context()), pluginName, fullTypeName, renderCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlStr))
	}
}

// pluginAPIMaxBodySize is the maximum request body size for plugin API endpoints.
const pluginAPIMaxBodySize = 1 << 20 // 1MB

// PluginAPIHandler handles JSON API requests to plugin-registered endpoints.
// Routes: GET/POST/PUT/DELETE /v1/plugins/{pluginName}/{path...}
func PluginAPIHandler(ctx PluginAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reject unsupported HTTP methods early
		switch r.Method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
		default:
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
			return
		}

		// Reject requests with declared Content-Length exceeding the limit
		if r.ContentLength > pluginAPIMaxBodySize {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "request body too large"})
			return
		}

		pm := ctx.PluginManager()
		if pm == nil {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "plugin system not available"})
			return
		}

		// Parse /v1/plugins/{pluginName}/{path...}
		trimmed := strings.TrimPrefix(r.URL.Path, "/v1/plugins/")
		parts := strings.SplitN(trimmed, "/", 2)
		pluginName := ""
		apiPath := ""
		if len(parts) >= 1 {
			pluginName = parts[0]
		}
		if len(parts) >= 2 {
			apiPath = parts[1]
		}

		if pluginName == "" || pluginName == "manage" {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "plugin not found"})
			return
		}

		// Build query map
		queryMap := make(map[string]any)
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				queryMap[k] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				queryMap[k] = items
			}
		}

		// Build headers map
		headerMap := make(map[string]any)
		for k, v := range r.Header {
			if len(v) == 1 {
				headerMap[strings.ToLower(k)] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				headerMap[strings.ToLower(k)] = items
			}
		}

		// Read body
		var body string
		if r.Body != nil {
			limited := io.LimitReader(r.Body, pluginAPIMaxBodySize+1)
			bodyBytes, err := io.ReadAll(limited)
			if err != nil {
				w.Header().Set("Content-Type", constants.JSON)
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to read request body"})
				return
			}
			if int64(len(bodyBytes)) > pluginAPIMaxBodySize {
				w.Header().Set("Content-Type", constants.JSON)
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "request body too large"})
				return
			}
			body = string(bodyBytes)
		}

		pageCtx := plugin_system.PageContext{
			Path:      r.URL.String(),
			Method:    r.Method,
			Query:     queryMap,
			Params:    make(map[string]any),
			Headers:   headerMap,
			Body:      body,
			Principal: auth.DescribeContext(r.Context()),
		}

		resp := pm.HandleAPI(plugin_system.WithMRQLCache(r.Context()), pluginName, r.Method, apiPath, pageCtx)

		w.Header().Set("Content-Type", constants.JSON)
		if resp.Error != "" {
			w.WriteHeader(resp.StatusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": resp.Error})
			return
		}

		w.WriteHeader(resp.StatusCode)
		if resp.Body != nil {
			_ = json.NewEncoder(w).Encode(resp.Body)
		}
	}
}
