//go:build postgres && json1 && fts5

package jobs

import "testing"

// The execution-principal SQL binds booleans for the deletion marks, which
// PostgreSQL compares as boolean columns and SQLite as integers.
func TestTheExecutionPrincipalSQLAgreesWithExecutionAccessOnPostgres(t *testing.T) {
	testTheExecutionPrincipalSQLAgreesWithExecutionAccess(t, newPGDeps(t))
}
