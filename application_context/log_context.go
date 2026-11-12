package application_context

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
		entry.IPAddress = l.ctx.ClientIP(l.request)
	}

	// Save to database (fire-and-forget, errors logged to stdout)
	if err := l.ctx.db.Create(&entry).Error; err != nil {
		fmt.Printf("Logger: failed to write log entry: %v\n", err)
	}
}

// slowQueryLogBufferSize is the queue size between the database's slow-query
// logger and the goroutine that persists entries to the application log. When
// the buffer is full, entries are dropped: a slow database is exactly when
// slow queries fire most, and logging must never block or amplify that load.
const slowQueryLogBufferSize = 64

// slowQuerySQLDetailLimit caps the SQL text stored in LogEntry.Details.
const slowQuerySQLDetailLimit = 4000

// StartSlowQueryLogSink connects the database's slow-query logger to the
// application log: queries slower than the configured threshold are stored as
// warning LogEntry rows (entity type "sql"), reviewable at /logs. Inserts run
// asynchronously on a silenced session so they are never traced themselves.
func (ctx *MahresourcesContext) StartSlowQueryLogSink(slowLogger *models.SlowQueryLogger) {
	entries := make(chan models.SlowQueryEntry, slowQueryLogBufferSize)

	go func() {
		silentDb := ctx.db.Session(&gorm.Session{Logger: gormlogger.Discard, NewDB: true})
		for e := range entries {
			rows := fmt.Sprintf("%d", e.Rows)
			if e.Rows < 0 {
				rows = "?"
			}
			entry := models.LogEntry{
				CreatedAt:  time.Now(),
				Level:      models.LogLevelWarning,
				Action:     models.LogActionSystem,
				EntityType: "sql",
				Message:    truncateString(fmt.Sprintf("SLOW SQL (took %v, %s rows): %s", e.Elapsed.Round(time.Millisecond), rows, e.SQL), 1000),
			}
			details := map[string]interface{}{
				"sql":        truncateString(e.SQL, slowQuerySQLDetailLimit),
				"durationMs": e.Elapsed.Milliseconds(),
				"rows":       e.Rows,
			}
			if e.Source != "" {
				details["source"] = e.Source
			}
			if jsonBytes, err := json.Marshal(details); err == nil {
				entry.Details = types.JSON(jsonBytes)
			}
			if err := silentDb.Create(&entry).Error; err != nil {
				fmt.Printf("Logger: failed to write slow-query log entry: %v\n", err)
			}
		}
	}()

	slowLogger.SetSink(func(e models.SlowQueryEntry) {
		select {
		case entries <- e:
		default: // buffer full: drop rather than block the query path
		}
	})
}

// GetLogEntries retrieves log entries with filtering and pagination.
func (ctx *MahresourcesContext) GetLogEntries(offset, maxResults int, query *query_models.LogEntryQuery) ([]models.LogEntry, error) {
	var logs []models.LogEntry
	return logs, ctx.db.Scopes(database_scopes.LogEntryQuery(query, false)).
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
func (ctx *MahresourcesContext) GetEntityHistory(entityType string, entityID uint, offset, limit int) ([]models.LogEntry, error) {
	var logs []models.LogEntry
	query := &query_models.EntityHistoryQuery{
		EntityType: entityType,
		EntityID:   entityID,
	}
	db := ctx.db.Scopes(database_scopes.EntityHistoryQuery(query))
	if limit > 0 {
		db = db.Limit(limit).Offset(offset)
	}
	return logs, db.Find(&logs).Error
}

// GetEntityHistoryCount returns the total count of log entries for a specific entity.
func (ctx *MahresourcesContext) GetEntityHistoryCount(entityType string, entityID uint) (int64, error) {
	var count int64
	query := &query_models.EntityHistoryQuery{
		EntityType: entityType,
		EntityID:   entityID,
	}
	return count, ctx.db.Model(&models.LogEntry{}).Scopes(database_scopes.EntityHistoryQuery(query, true)).Count(&count).Error
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

// ClientIP returns the request's client IP, truncated to LogEntry.IPAddress's
// 45-character column limit. X-Forwarded-For (leftmost entry) is honored only
// when trustProxy is set, i.e. the server is behind a trusted reverse proxy;
// otherwise a directly connected client could forge it. Falls back to the host
// part of RemoteAddr.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			if idx := strings.Index(xff, ","); idx != -1 {
				xff = strings.TrimSpace(xff[:idx])
			}
			return truncateString(xff, 45)
		}
	}
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	} else if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]"); idx != -1 {
			ip = ip[1:idx]
		}
	} else if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return truncateString(ip, 45)
}

// ClientIP is ClientIP with this context's -trust-proxy-headers setting. A nil
// Config (bare test contexts) means trust off.
func (ctx *MahresourcesContext) ClientIP(r *http.Request) string {
	return ClientIP(r, ctx.Config != nil && ctx.Config.TrustProxyHeaders)
}
