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

	// GrantsJSON records what an operator consented to when they enabled this
	// plugin: the capability set, network allowlist and private-host flag, as
	// written by plugin_system.Grants.Marshal.
	//
	// Deliberately nullable and with no default. An empty value is NOT "consented
	// to nothing" — it is "no record", which is how every row that predates this
	// column reads, and those are grandfathered on first load. The distinction is
	// made in plugin_system.ParseGrants, not here.
	GrantsJSON string `gorm:"type:text"`

	// AllowScopedPrincipals lets group-limited users and guests reach this
	// plugin's own surfaces: its pages, endpoints, shortcodes, injected slots
	// and rendered blocks. Off by default, and off is what every existing row
	// reads as, so installing this changes nothing until an operator decides
	// plugin by plugin.
	//
	// It does not describe what the plugin may then do. A confined caller's
	// mah.db calls stay bound to that caller's own subtree and role — this only
	// says whether such a caller may knock on the door.
	AllowScopedPrincipals bool `gorm:"default:false"`
}
