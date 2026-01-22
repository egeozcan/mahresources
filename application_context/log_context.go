package application_context

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// Logger provides a convenient interface for logging operations.
// It wraps the MahresourcesContext and provides convenience methods.
type Logger struct {
	ctx *MahresourcesContext
	// HTTP request context (optional, for capturing request details)
	request *http.Request
}

// Logger returns a Logger instance for the current context.
// If a request context has been set via SetRequestContext, it will be used.
func (ctx *MahresourcesContext) Logger() *Logger {
	return &Logger{ctx: ctx, request: ctx.currentRequest}
}

// LogFromRequest returns a Logger that captures HTTP request details.
func (ctx *MahresourcesContext) LogFromRequest(r *http.Request) *Logger {
	return &Logger{ctx: ctx, request: r}
}

// Info logs an informational message.
func (l *Logger) Info(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{}) {
	l.log(models.LogLevelInfo, action, entityType, entityID, entityName, message, details)
}

// Warning logs a warning message.
func (l *Logger) Warning(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{}) {
	l.log(models.LogLevelWarning, action, entityType, entityID, entityName, message, details)
}

// Error logs an error message.
func (l *Logger) Error(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{}) {
	l.log(models.LogLevelError, action, entityType, entityID, entityName, message, details)
}

// log creates and persists a log entry.
// Errors are printed to stdout but never propagate to break main operations.
func (l *Logger) log(level, action, entityType string, entityID *uint, entityName, message string, details map[string]interface{}) {
	entry := models.LogEntry{
		CreatedAt:  time.Now(),
		Level:      level,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		EntityName: truncateString(entityName, 255),
		Message:    truncateString(message, 1000),
	}

	// Convert details to JSON if provided
	if details != nil {
		jsonBytes, err := json.Marshal(details)
		if err != nil {
			fmt.Printf("Logger: failed to marshal details: %v\n", err)
		} else {
			entry.Details = types.JSON(jsonBytes)
		}
	}

	// Capture HTTP request details if available
	if l.request != nil {
		entry.RequestPath = truncateString(l.request.URL.Path, 500)
		entry.UserAgent = truncateString(l.request.UserAgent(), 500)
		entry.IPAddress = getClientIP(l.request)
	}

	// Save to database (fire-and-forget, errors logged to stdout)
	if err := l.ctx.db.Create(&entry).Error; err != nil {
		fmt.Printf("Logger: failed to write log entry: %v\n", err)
	}
}

// GetLogEntries retrieves log entries with filtering and pagination.
func (ctx *MahresourcesContext) GetLogEntries(offset, maxResults int, query *query_models.LogEntryQuery) (*[]models.LogEntry, error) {
	var logs []models.LogEntry
	return &logs, ctx.db.Scopes(database_scopes.LogEntryQuery(query, false)).
		Limit(maxResults).
		Offset(offset).
		Find(&logs).Error
}

// GetLogEntriesCount returns the total count of log entries matching the query.
func (ctx *MahresourcesContext) GetLogEntriesCount(query *query_models.LogEntryQuery) (int64, error) {
	var count int64
	return count, ctx.db.Model(&models.LogEntry{}).
		Scopes(database_scopes.LogEntryQuery(query, true)).
		Count(&count).Error
}

// GetLogEntry retrieves a single log entry by ID.
func (ctx *MahresourcesContext) GetLogEntry(id uint) (*models.LogEntry, error) {
	var log models.LogEntry
	return &log, ctx.db.First(&log, id).Error
}

// GetEntityHistory retrieves log entries for a specific entity with pagination.
func (ctx *MahresourcesContext) GetEntityHistory(entityType string, entityID uint, offset, limit int) (*[]models.LogEntry, error) {
	var logs []models.LogEntry
	query := &query_models.EntityHistoryQuery{
		EntityType: entityType,
		EntityID:   entityID,
	}
	db := ctx.db.Scopes(database_scopes.EntityHistoryQuery(query))
	if limit > 0 {
		db = db.Limit(limit).Offset(offset)
	}
	return &logs, db.Find(&logs).Error
}

// GetEntityHistoryCount returns the total count of log entries for a specific entity.
func (ctx *MahresourcesContext) GetEntityHistoryCount(entityType string, entityID uint) (int64, error) {
	var count int64
	query := &query_models.EntityHistoryQuery{
		EntityType: entityType,
		EntityID:   entityID,
	}
	return count, ctx.db.Model(&models.LogEntry{}).Scopes(database_scopes.EntityHistoryQuery(query)).Count(&count).Error
}

// CleanupOldLogs deletes log entries older than the specified number of days.
// Returns the number of deleted entries.
func (ctx *MahresourcesContext) CleanupOldLogs(daysToKeep int) (int64, error) {
	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	result := ctx.db.Where("created_at < ?", cutoffTime).Delete(&models.LogEntry{})
	return result.RowsAffected, result.Error
}

// Helper functions

// truncateString truncates a string to the specified max length in runes (not bytes).
// This safely handles multi-byte UTF-8 characters without splitting them.
func truncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs (client, proxy1, proxy2, ...)
		// Take only the first one (the original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = strings.TrimSpace(xff[:idx])
		}
		return truncateString(xff, 45)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return truncateString(xri, 45)
	}

	// Fall back to RemoteAddr, stripping the port if present
	ip := r.RemoteAddr
	// Handle IPv6 addresses like [::1]:8080
	if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]"); idx != -1 {
			ip = ip[1:idx] // Extract IPv6 address without brackets
		}
	} else if idx := strings.LastIndex(ip, ":"); idx != -1 {
		// Handle IPv4 addresses like 192.168.1.1:8080
		ip = ip[:idx]
	}
	return truncateString(ip, 45)
}
