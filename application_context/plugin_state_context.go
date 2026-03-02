package application_context

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/plugin_system"
)

// EnsurePluginStates creates PluginState rows for any discovered plugins
// that don't yet have one. Returns all plugin states.
func (ctx *MahresourcesContext) EnsurePluginStates() ([]models.PluginState, error) {
	if ctx.pluginManager == nil {
		return nil, nil
	}

	for _, dp := range ctx.pluginManager.DiscoveredPlugins() {
		var count int64
		ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", dp.Name).Count(&count)
		if count == 0 {
			state := models.PluginState{
				PluginName: dp.Name,
				Enabled:    false,
			}
			if err := ctx.db.Create(&state).Error; err != nil {
				return nil, fmt.Errorf("creating plugin state for %q: %w", dp.Name, err)
			}
		}
	}

	var states []models.PluginState
	if err := ctx.db.Order("plugin_name").Find(&states).Error; err != nil {
		return nil, err
	}
	return states, nil
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

	for _, state := range states {
		if !state.Enabled {
			continue
		}

		settings, err := ctx.loadPluginSettingsMap(state.PluginName)
		if err != nil {
			ctx.Logger().Warning("system", "plugin", nil, state.PluginName, "failed to load settings at startup", map[string]interface{}{
				"error": err.Error(),
			})
		}
		ctx.pluginManager.SetPluginSettings(state.PluginName, settings)

		if err := ctx.pluginManager.EnablePlugin(state.PluginName); err != nil {
			ctx.Logger().Warning("system", "plugin", nil, state.PluginName, "failed to enable plugin at startup", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
}

func (ctx *MahresourcesContext) findDiscoveredPlugin(name string) *plugin_system.DiscoveredPlugin {
	return ctx.pluginManager.GetDiscoveredPlugin(name)
}

func (ctx *MahresourcesContext) loadPluginSettingsMap(pluginName string) (map[string]any, error) {
	state, err := ctx.GetPluginState(pluginName)
	if err != nil || state.SettingsJSON == "" {
		return make(map[string]any), err
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(state.SettingsJSON), &settings); err != nil {
		return make(map[string]any), err
	}
	return settings, nil
}
