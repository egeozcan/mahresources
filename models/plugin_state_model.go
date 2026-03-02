package models

import "time"

// PluginState persists a plugin's enabled/disabled status and settings.
type PluginState struct {
	ID           uint      `gorm:"primarykey"`
	CreatedAt    time.Time `gorm:"index"`
	UpdatedAt    time.Time `gorm:"index"`
	PluginName   string    `gorm:"uniqueIndex:idx_plugin_name"`
	Enabled      bool      `gorm:"default:false"`
	SettingsJSON string    `gorm:"type:text"`
}
