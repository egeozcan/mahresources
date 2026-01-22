package query_models

// LogEntryQuery defines the query parameters for filtering log entries.
type LogEntryQuery struct {
	Level         string   // Filter by log level (info, warning, error)
	Action        string   // Filter by action (create, update, delete, system)
	EntityType    string   // Filter by entity type
	EntityID      uint     // Filter by entity ID
	Message       string   // LIKE search on message
	RequestPath   string   // LIKE search on request path
	CreatedBefore string   // Filter logs created before this time
	CreatedAfter  string   // Filter logs created after this time
	SortBy        []string // Sort columns
}

// EntityHistoryQuery defines parameters for getting history of a specific entity.
type EntityHistoryQuery struct {
	EntityType string // The type of entity
	EntityID   uint   // The ID of the entity
}
