package application_context

import "strings"

// isUniqueConstraintError checks whether an error is a database unique constraint
// violation. It supports both SQLite and PostgreSQL error messages.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
