package plugin_system

import (
	"encoding/json"
	"fmt"
)

// PluginShortcode represents a plugin-registered shortcode.
type PluginShortcode struct {
	PluginName string
	TypeName   string
	Label      string
}

// RenderShortcode renders a plugin-registered shortcode. Stub for now.
func (pm *PluginManager) RenderShortcode(pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string) (string, error) {
	return "", fmt.Errorf("plugin shortcodes not yet implemented")
}

// GetPluginShortcode returns a specific plugin shortcode by full type name, or nil. Stub for now.
func (pm *PluginManager) GetPluginShortcode(fullTypeName string) *PluginShortcode {
	return nil
}
