package models

import (
	"mahresources/models/types"
	"time"
)

// LogEntry represents a single log entry for tracking CRUD operations and system events.
type LogEntry struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `gorm:"index:idx_log_created_at" json:"createdAt"`
	Level       string         `gorm:"index:idx_log_level;size:20" json:"level"`       // info, warning, error
	Action      string         `gorm:"index:idx_log_action;size:20" json:"action"`     // create, update, delete, system
	EntityType  string         `gorm:"index:idx_log_entity_type;size:50" json:"entityType"`
	EntityID    *uint          `gorm:"index:idx_log_entity_id" json:"entityId"`
	EntityName  string         `gorm:"size:255" json:"entityName"`
	Message     string         `gorm:"size:1000" json:"message"`
	Details     types.JSON `gorm:"type:json" json:"details,omitempty"`
	RequestPath string         `gorm:"size:500" json:"requestPath,omitempty"`
	UserAgent   string         `gorm:"size:500" json:"userAgent,omitempty"`
	IPAddress   string         `gorm:"size:45" json:"ipAddress,omitempty"`
}

// Log levels
const (
	LogLevelInfo    = "info"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
)

// Log actions
const (
	LogActionCreate = "create"
	LogActionUpdate = "update"
	LogActionDelete = "delete"
	LogActionSystem = "system"
)
