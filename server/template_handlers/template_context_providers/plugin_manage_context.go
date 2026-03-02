package template_context_providers

import (
	"encoding/json"
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/plugin_system"
)

type pluginDisplay struct {
	Name        string
	Version     string
	Description string
	Enabled     bool
	Settings    []plugin_system.SettingDefinition
	Values      map[string]any
}

func PluginManageContextProvider(appCtx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := staticTemplateCtx(request)
		ctx["pageTitle"] = "Manage Plugins"

		pm := appCtx.PluginManager()
		if pm == nil {
			ctx["plugins"] = []pluginDisplay{}
			return ctx
		}

		discovered := pm.DiscoveredPlugins()
		states, _ := appCtx.GetPluginStates()

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

		var plugins []pluginDisplay
		for _, dp := range discovered {
			pd := pluginDisplay{
				Name:        dp.Name,
				Version:     dp.Version,
				Description: dp.Description,
				Settings:    dp.Settings,
				Values:      make(map[string]any),
			}
			if s, ok := stateMap[dp.Name]; ok {
				pd.Enabled = s.enabled
				if s.settings != "" {
					json.Unmarshal([]byte(s.settings), &pd.Values)
				}
			}
			plugins = append(plugins, pd)
		}

		ctx["plugins"] = plugins
		return ctx
	}
}
