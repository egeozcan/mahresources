package application_context

import (
	"fmt"
	"strings"
)

// isUniqueConstraintError checks whether the given error represents a database
// unique-constraint violation. It works for both SQLite and PostgreSQL.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SQLite: "UNIQUE constraint failed: ..."
	// PostgreSQL: "duplicate key value violates unique constraint ..."
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}

// isForeignKeyError checks whether the given error represents a database
// foreign-key constraint violation. It works for both SQLite and PostgreSQL.
func isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SQLite: "FOREIGN KEY constraint failed"
	// PostgreSQL: "violates foreign key constraint"
	return strings.Contains(msg, "FOREIGN KEY constraint failed") ||
		strings.Contains(msg, "violates foreign key constraint")
}

// friendlyUniqueError wraps a unique-constraint error with a user-readable message.
func friendlyUniqueError(entityName string, err error) error {
	if isUniqueConstraintError(err) {
		return fmt.Errorf("a %s with that name already exists", entityName)
	}
	return err
}
