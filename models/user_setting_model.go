package models

import "time"

// UserSetting stores per-user key-value UI preferences (e.g. lightbox quick tags,
// the showDescriptions toggle, MRQL query history). It replaces browser localStorage
// so preferences follow a user across browsers/devices and can be backed up server-side.
//
// Modeled on PluginKV: a composite-unique (user_id, key) with the value stored as JSON
// text and upserted via clause.OnConflict. There is intentionally no FK association to
// users (matching PluginKV) to avoid SQLite FK/AutoMigrate churn; rows are cleaned up by
// the user-deletion path rather than a cascade.
type UserSetting struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserId    uint   `gorm:"uniqueIndex:idx_user_setting_key;index;not null"`
	Key       string `gorm:"uniqueIndex:idx_user_setting_key;size:128;not null"`
	Value     string `gorm:"type:text;not null"`
}
