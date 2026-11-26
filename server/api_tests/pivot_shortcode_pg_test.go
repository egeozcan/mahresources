//go:build postgres

package api_tests

import "testing"

// The SQLite checks on Postgres: ->> text keys and the boolean label, the
// numeric SUM cast, the group alias in ORDER BY, and the principal-scoped
// executor with visible joins.

func TestPivotShortcode_LinkCountsMatchCells_Postgres(t *testing.T) {
	checkPivotLinkCountsMatchCells(t, setupPostgresAuthEnv(t), true)
}

func TestPivotShortcode_SumSemantics_Postgres(t *testing.T) {
	checkPivotSumSemantics(t, setupPostgresAuthEnv(t), true)
}

func TestPivotShortcode_OneStatement_Postgres(t *testing.T) {
	checkPivotRunsOneStatement(t, setupPostgresAuthEnv(t))
}

func TestPivotShortcode_ScopedPrincipalCannotSeeOutside_Postgres(t *testing.T) {
	checkPivotScopedPrincipal(t, setupPostgresAuthEnv(t))
}
