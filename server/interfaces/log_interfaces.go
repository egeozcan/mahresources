package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

// LogEntryReader provides read operations for log entries.
type LogEntryReader interface {
	GetLogEntries(offset, maxResults int, query *query_models.LogEntryQuery) (*[]models.LogEntry, error)
	GetLogEntriesCount(query *query_models.LogEntryQuery) (int64, error)
	GetLogEntry(id uint) (*models.LogEntry, error)
	GetEntityHistory(entityType string, entityID uint, offset, limit int) (*[]models.LogEntry, error)
	GetEntityHistoryCount(entityType string, entityID uint) (int64, error)
}
