package models

import "time"

const (
	PluginCommandImportStatusPending     = "pending"
	PluginCommandImportStatusRunning     = "running"
	PluginCommandImportStatusSucceeded   = "succeeded"
	PluginCommandImportStatusFailed      = "failed"
	PluginCommandImportStatusCancelled   = "cancelled"
	PluginCommandImportStatusInterrupted = "interrupted"
)

type PluginCommandImport struct {
	ID               string `gorm:"primaryKey;size:32"`
	RunID            string `gorm:"index;size:32;not null"`
	FileName         string `gorm:"size:255;not null"`
	PluginGeneration uint64
	CreatedByUserId  *uint     `gorm:"index"`
	Status           string    `gorm:"index;size:16;not null"`
	Error            string    `gorm:"type:text"`
	CreatedAt        time.Time `gorm:"index"`
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

type PluginCommandImportMap struct {
	ID         uint   `gorm:"primaryKey"`
	RunID      string `gorm:"uniqueIndex:idx_command_import_file;size:32;not null"`
	FileName   string `gorm:"uniqueIndex:idx_command_import_file;size:255;not null"`
	ImportID   string `gorm:"index;size:32;not null"`
	ResourceID *uint
	Status     string `gorm:"index;size:16;not null"`
	Error      string `gorm:"type:text"`
}
