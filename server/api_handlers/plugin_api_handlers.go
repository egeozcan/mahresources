package api_handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mahresources/constants"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"net/http"
	"strings"

	"mahresources/application_context"
)

type pluginListItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Settings    any    `json:"settings,omitempty"`
	Values      any    `json:"values,omitempty"`
}

func GetPluginsManageHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
			item := pluginListItem{
				Name:        dp.Name,
				Version:     dp.Version,
				Description: dp.Description,
				Settings:    dp.Settings,
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

func GetPluginEnableHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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

func GetPluginDisableHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
func GetPluginPurgeDataHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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

func GetPluginSettingsHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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

// PluginAPIHandler handles JSON API requests to plugin-registered endpoints.
// Routes: GET/POST/PUT/DELETE /v1/plugins/{pluginName}/{path...}
func PluginAPIHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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
			const maxBodySize = 50 << 20 // 50MB
			limited := io.LimitReader(r.Body, maxBodySize)
			bodyBytes, err := io.ReadAll(limited)
			if err == nil {
				body = string(bodyBytes)
			}
		}

		pageCtx := plugin_system.PageContext{
			Path:    r.URL.String(),
			Method:  r.Method,
			Query:   queryMap,
			Params:  make(map[string]any),
			Headers: headerMap,
			Body:    body,
		}

		resp := pm.HandleAPI(pluginName, r.Method, apiPath, pageCtx)

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
