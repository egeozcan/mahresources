package application_context

import (
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// isUniqueConstraintError checks whether the given error represents a database
// unique-constraint violation. It works for both SQLite and PostgreSQL.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
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
	return strings.Contains(msg, "FOREIGN KEY constraint failed") ||
		strings.Contains(msg, "violates foreign key constraint")
}

// isLockContentionError checks whether the given error means the statement gave
// up waiting for a lock rather than failing on its own merits. SQLite surfaces
// its exhausted busy_timeout as "database is locked"; PostgreSQL reports an
// expired lock_timeout as SQLSTATE 55P03. Matching on the message follows the
// same convention as the constraint helpers above and keeps pgx an indirect
// dependency.
func isLockContentionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "55P03")
}

// isDeadlockError reports a database that aborted this transaction to break a
// deadlock, rather than one that failed on its own merits. Postgres reports
// SQLSTATE 40P01; SQLite has a single writer and cannot deadlock between
// transactions.
//
// Deliberately separate from isLockContentionError rather than folded into it:
// that helper decides, among other things, whether a failed login is answered
// with 503 Retry-After, and widening it would change an unrelated contract.
func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "deadlock detected") || strings.Contains(msg, "40P01")
}

// friendlyUniqueError wraps a unique-constraint error with a user-readable message.
func friendlyUniqueError(entityName string, err error) error {
	if isUniqueConstraintError(err) {
		return fmt.Errorf("a %s with that name already exists", entityName)
	}
	return err
}

// friendlyUniqueNameError is friendlyUniqueError for the paths that know which
// name collided, so the message can quote it. Matches the wording the tag
// create path already uses, so renaming onto a taken name reads the same as
// creating one.
func friendlyUniqueNameError(entityName, name string, err error) error {
	if isUniqueConstraintError(err) {
		return fmt.Errorf("a %s named %q already exists", entityName, name)
	}
	return err
}

// modelLabel derives a human-readable singular name for a model from its GORM
// schema — "NoteType" becomes "note type". The generic EntityWriter serves
// every entity from one code path, so it has no label to pass down.
func modelLabel(db *gorm.DB, model any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil || stmt.Schema == nil {
		return "entity"
	}
	return spaceCamelCase(stmt.Schema.Name)
}

// spaceCamelCase lowercases a Go identifier and inserts spaces at word
// boundaries, keeping acronym runs together: "SavedMRQLQuery" -> "saved mrql query".
func spaceCamelCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			startsWord := !unicode.IsUpper(runes[i-1])
			endsAcronym := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if startsWord || endsAcronym {
				b.WriteRune(' ')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
