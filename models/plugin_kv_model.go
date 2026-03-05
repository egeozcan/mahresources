package models

import "time"

// PluginKV stores per-plugin key-value data.
type PluginKV struct {
	ID         uint      `gorm:"primarykey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	PluginName string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
	Key        string    `gorm:"uniqueIndex:idx_plugin_kv_key;not null"`
	Value      string    `gorm:"type:text;not null"`
}
