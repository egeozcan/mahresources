package models

import "time"

// RuntimeSetting stores a runtime override for one configuration key.
// Absence of a row means "no override; use the boot-time default."
type RuntimeSetting struct {
	Key       string    `gorm:"primaryKey;size:100" json:"key"`
	ValueJSON string    `gorm:"type:text;not null" json:"valueJson"`
	Reason    string    `gorm:"type:text" json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}
