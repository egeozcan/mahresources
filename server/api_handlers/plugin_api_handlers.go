package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
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
		if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
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
