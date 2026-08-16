package application_context

import (
	"encoding/json"
	"fmt"
	"strings"

	"mahresources/models"
	"mahresources/plugin_system"
)

// EnsurePluginStates creates PluginState rows for any discovered plugins
// that don't yet have one. Returns all plugin states.
func (ctx *MahresourcesContext) EnsurePluginStates() ([]models.PluginState, error) {
	if ctx.pluginManager == nil {
		return nil, nil
	}

	discovered := ctx.pluginManager.DiscoveredPlugins()

	if len(discovered) > 0 {
		// De-duplicate discovered names before checking existence.
		discoveredMap := make(map[string]struct{}, len(discovered))
		var uniqueNames []string
		for _, dp := range discovered {
			if _, ok := discoveredMap[dp.Name]; !ok {
				discoveredMap[dp.Name] = struct{}{}
				uniqueNames = append(uniqueNames, dp.Name)
			}
		}

		var existingNames []string
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name IN ?", uniqueNames).
			Pluck("plugin_name", &existingNames).Error; err != nil {
			return nil, err
		}

		existingMap := make(map[string]struct{}, len(existingNames))
		for _, name := range existingNames {
			existingMap[name] = struct{}{}
		}

		var toCreate []models.PluginState
		for _, name := range uniqueNames {
			if _, ok := existingMap[name]; !ok {
				toCreate = append(toCreate, models.PluginState{
					PluginName: name,
					Enabled:    false,
				})
			}
		}

		if len(toCreate) > 0 {
			if err := ctx.db.Create(&toCreate).Error; err != nil {
				return nil, fmt.Errorf("batch creating plugin states: %w", err)
			}
		}
	}

	return ctx.GetPluginStates()
}

// GetPluginStates returns all plugin states from the database.
func (ctx *MahresourcesContext) GetPluginStates() ([]models.PluginState, error) {
	var states []models.PluginState
	if err := ctx.db.Order("plugin_name").Find(&states).Error; err != nil {
		return nil, err
	}
	return states, nil
}

// GetPluginState returns the state for a specific plugin.
func (ctx *MahresourcesContext) GetPluginState(pluginName string) (*models.PluginState, error) {
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// SetPluginEnabled enables or disables a plugin and updates the database.
func (ctx *MahresourcesContext) SetPluginEnabled(pluginName string, enabled bool) error {
	if ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager not initialized")
	}

	if enabled {
		// Check required settings before enabling
		dp := ctx.findDiscoveredPlugin(pluginName)
		if dp == nil {
			return fmt.Errorf("plugin %q not found", pluginName)
		}

		settings, _ := ctx.loadPluginSettingsMap(pluginName)
		missing := plugin_system.CheckRequiredSettings(dp.Settings, settings)
		if len(missing) > 0 {
			return fmt.Errorf("missing required settings: %v", missing)
		}

		// Enabling *is* the consent gesture, so the record is written here and
		// before the load, which enforces it. Written afterwards, the first
		// enable of a plugin that widened its manifest would be refused by the
		// consent it is in the middle of giving.
		//
		// Before the enabled column too, so a consent write that fails leaves
		// the row exactly as it was and there is nothing to revert.
		consent := plugin_system.GrantsFromManifest(dp.Manifest)
		if err := (&pluginConsentStore{ctx: ctx}).RecordConsent(pluginName, consent); err != nil {
			return err
		}

		// Persist DB state first, then enable in memory. If the in-memory
		// step fails we revert the DB so the two stay consistent.
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name = ?", pluginName).
			Update("enabled", true).Error; err != nil {
			return err
		}

		// Load settings into plugin manager memory
		ctx.pluginManager.SetPluginSettings(pluginName, settings)

		if err := ctx.pluginManager.EnablePlugin(pluginName); err != nil {
			// Revert DB state on failure
			_ = ctx.db.Model(&models.PluginState{}).
				Where("plugin_name = ?", pluginName).
				Update("enabled", false).Error
			return err
		}
	} else {
		// Persist DB state first, then disable in memory.
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name = ?", pluginName).
			Update("enabled", false).Error; err != nil {
			return err
		}

		if err := ctx.pluginManager.DisablePlugin(pluginName); err != nil {
			// If the plugin wasn't loaded in memory, the desired state
			// (disabled) is already achieved — don't revert the DB.
			if !ctx.pluginManager.IsEnabled(pluginName) {
				return nil
			}
			// Revert DB state on unexpected failure
			_ = ctx.db.Model(&models.PluginState{}).
				Where("plugin_name = ?", pluginName).
				Update("enabled", true).Error
			return err
		}
	}

	return nil
}

// SavePluginSettings validates and saves settings for a plugin.
func (ctx *MahresourcesContext) SavePluginSettings(pluginName string, values map[string]any) ([]plugin_system.ValidationError, error) {
	if ctx.pluginManager == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return nil, fmt.Errorf("plugin %q not found", pluginName)
	}

	// Validate
	if errs := plugin_system.ValidateSettings(dp.Settings, values); len(errs) > 0 {
		return errs, nil
	}

	// Filter to declared keys only
	declared := make(map[string]struct{}, len(dp.Settings))
	for _, s := range dp.Settings {
		declared[s.Name] = struct{}{}
	}
	filtered := make(map[string]any, len(declared))
	for k, v := range values {
		if _, ok := declared[k]; ok {
			filtered[k] = v
		}
	}
	values = filtered

	// Serialize to JSON
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshaling settings: %w", err)
	}

	// Save to DB
	if err := ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("settings_json", string(jsonBytes)).Error; err != nil {
		return nil, err
	}

	// Update in-memory settings if plugin is enabled
	if ctx.pluginManager.IsEnabled(pluginName) {
		ctx.pluginManager.SetPluginSettings(pluginName, values)
	}

	return nil, nil
}

// ActivateEnabledPlugins enables all plugins marked as enabled in the database.
//
// It is a repeated pass, not one walk of the states. A plugin's dependencies
// have to be loaded before it is, and the states arrive in plugin_name order, so
// a single walk starts "aaa" — which depends on "zzz" — before anything could
// have satisfied it. Each round enables everything whose declared dependencies
// this run has already loaded; rounds stop as soon as one loads nothing, and
// whatever is still waiting is reported once, naming what it waits for.
//
// Per-plugin behaviour is unchanged: settings that fail to load are a warning
// and the plugin starts with its defaults, and a plugin that fails to enable is
// a warning and the run continues. A failed plugin is not retried — it would log
// the same warning every round — and is never counted as loaded, so anything
// depending on it stays behind and is named in the final warning.
func (ctx *MahresourcesContext) ActivateEnabledPlugins() {
	if ctx.pluginManager == nil {
		return
	}

	states, err := ctx.GetPluginStates()
	if err != nil {
		ctx.Logger().Error("system", "plugin", nil, "", "failed to load plugin states at startup", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	var remaining []string
	enabled := make(map[string]struct{}, len(states))
	for _, state := range states {
		if state.Enabled {
			remaining = append(remaining, state.PluginName)
			enabled[state.PluginName] = struct{}{}
		}
	}

	loaded := make(map[string]struct{}, len(remaining))
	failed := make(map[string]struct{})

	for len(remaining) > 0 {
		var waiting []string
		progressed := false

		for _, name := range remaining {
			if len(ctx.unmetDependencies(name, loaded)) > 0 {
				waiting = append(waiting, name)
				continue
			}
			if ctx.enablePluginAtStartup(name) {
				loaded[name] = struct{}{}
				progressed = true
				continue
			}
			failed[name] = struct{}{}
		}

		remaining = waiting
		// Nothing loaded this round, so no unmet dependency can have become met:
		// another round would make exactly the same decisions.
		if !progressed {
			break
		}
	}

	if len(remaining) > 0 {
		ctx.warnAboutStalledPlugins(remaining, loaded, enabled, failed)
	}
}

// enablePluginAtStartup loads one plugin's settings and enables it, reporting
// whether it is now running.
func (ctx *MahresourcesContext) enablePluginAtStartup(pluginName string) bool {
	settings, err := ctx.loadPluginSettingsMap(pluginName)
	if err != nil {
		ctx.Logger().Warning("system", "plugin", nil, pluginName, "failed to load settings at startup", map[string]interface{}{
			"error": err.Error(),
		})
	}
	ctx.pluginManager.SetPluginSettings(pluginName, settings)

	if err := ctx.pluginManager.EnablePlugin(pluginName); err != nil {
		ctx.Logger().Warning("system", "plugin", nil, pluginName, "failed to enable plugin at startup", map[string]interface{}{
			"error": err.Error(),
		})
		return false
	}
	return true
}

// unmetDependencies returns the plugin's declared dependencies that this run has
// not loaded yet.
//
// A plugin with no discovered manifest waits for nothing: EnablePlugin refuses
// it by name, which is the message an operator needs, and holding it back would
// replace that with a vaguer one about dependencies.
func (ctx *MahresourcesContext) unmetDependencies(pluginName string, loaded map[string]struct{}) []string {
	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return nil
	}

	var unmet []string
	for _, dep := range dp.Manifest.Dependencies {
		if _, ok := loaded[dep]; !ok {
			unmet = append(unmet, dep)
		}
	}
	return unmet
}

// warnAboutStalledPlugins logs the single entry that closes an activation run:
// which enabled plugins never started, and which dependency each is waiting for.
//
// One entry rather than one per plugin, because a cycle stalls every plugin in
// it and the useful fact is the shape of the whole set, not each member of it.
func (ctx *MahresourcesContext) warnAboutStalledPlugins(stalled []string, loaded, enabled, failed map[string]struct{}) {
	waitingSet := make(map[string]struct{}, len(stalled))
	for _, name := range stalled {
		waitingSet[name] = struct{}{}
	}

	lines := make([]string, 0, len(stalled))
	for _, name := range stalled {
		var reasons []string
		for _, dep := range ctx.unmetDependencies(name, loaded) {
			reasons = append(reasons, dep+" ("+ctx.stalledDependencyReason(dep, waitingSet, enabled, failed)+")")
		}
		lines = append(lines, name+" waits for "+strings.Join(reasons, ", "))
	}

	ctx.Logger().Warning("system", "plugin", nil, "", "enabled plugins were not started because their dependencies never loaded: "+strings.Join(lines, "; "),
		map[string]interface{}{
			"plugins": stalled,
			"waiting": lines,
		})
}

// stalledDependencyReason says why one dependency never became available, so the
// warning tells an operator what to fix rather than only what broke.
func (ctx *MahresourcesContext) stalledDependencyReason(dep string, waiting, enabled, failed map[string]struct{}) string {
	if ctx.findDiscoveredPlugin(dep) == nil {
		return "no such plugin"
	}
	if _, ok := failed[dep]; ok {
		return "it failed to load"
	}
	if _, ok := waiting[dep]; ok {
		// A stall travels: the plugin at the head of the chain is the one to
		// fix, and this is what points at it. Not a cycle — the manager drops
		// the members of one at discovery, so a cycle never reaches this run.
		return "also waiting for a dependency of its own"
	}
	if _, ok := enabled[dep]; !ok {
		return "not enabled"
	}
	return "not loaded"
}

func (ctx *MahresourcesContext) findDiscoveredPlugin(name string) *plugin_system.DiscoveredPlugin {
	return ctx.pluginManager.GetDiscoveredPlugin(name)
}

func (ctx *MahresourcesContext) loadPluginSettingsMap(pluginName string) (map[string]any, error) {
	state, err := ctx.GetPluginState(pluginName)
	if err != nil || state.SettingsJSON == "" {
		return ctx.applyPluginDefaults(pluginName, make(map[string]any)), err
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(state.SettingsJSON), &settings); err != nil {
		return ctx.applyPluginDefaults(pluginName, make(map[string]any)), err
	}
	return ctx.applyPluginDefaults(pluginName, settings), nil
}

// applyPluginDefaults merges declared default values from the plugin's
// setting definitions into the settings map for any keys not already present.
func (ctx *MahresourcesContext) applyPluginDefaults(pluginName string, settings map[string]any) map[string]any {
	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return settings
	}
	return plugin_system.ApplyDefaults(dp.Settings, settings)
}
