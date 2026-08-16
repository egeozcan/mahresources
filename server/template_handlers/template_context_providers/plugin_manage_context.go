package template_context_providers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_system"
)

type pluginDisplay struct {
	Name        string
	Version     string
	Description string
	Enabled     bool
	HasDocs     bool
	Settings    []plugin_system.SettingDefinition
	Values      map[string]any

	// Legacy is true for a plugin that declares no manifest at all: it keeps
	// the full mah surface, which is why Capabilities below lists everything.
	Legacy     bool
	APIVersion int

	// Capabilities is the effective set (db:write implies db:read), and
	// CapabilityLabels is the human sentence for each one, so an operator reads
	// a described power rather than a slug.
	Capabilities     []string
	CapabilityLabels map[string]string

	// Network is the declared egress allowlist in canonical display form. An
	// empty list means "any public host" — the broadest policy, not the absence
	// of network access.
	Network           []string
	AllowPrivateHosts bool

	Dependencies []string

	// MinAppVersion is informational only: it is parsed and displayed, never
	// enforced.
	MinAppVersion string
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

func PluginManageContextProvider(appCtx PluginManagePageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := StaticTemplateCtx(request)
		ctx["pageTitle"] = "Manage Plugins"

		pm := appCtx.PluginManager()
		if pm == nil {
			ctx["plugins"] = []pluginDisplay{}
			return ctx
		}

		discovered := pm.DiscoveredPlugins()
		states, err := appCtx.GetPluginStates()
		if err != nil {
			log.Printf("[plugin] warning: failed to load plugin states: %v", err)
		}

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
			caps := dp.Manifest.Capabilities().Sorted()
			pd := pluginDisplay{
				Name:              dp.Name,
				Version:           dp.Version,
				Description:       dp.Description,
				HasDocs:           pm.PluginHasDocs(dp.Name),
				Settings:          dp.Settings,
				Values:            make(map[string]any),
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
