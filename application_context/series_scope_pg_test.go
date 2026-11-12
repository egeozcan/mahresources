//go:build postgres && json1 && fts5

package application_context

import "testing"

// Postgres variants of series_scope_test.go (#91). Postgres runs without FK
// constraints, so a wrongly deleted Series leaves its outside members with a
// dangling series_id that breaks every later edit and delete of them.

func TestScopedEditResourceLeaveKeepsSeriesWithOutsideMembersPostgres(t *testing.T) {
	runScopedEditLeaveKeepsSeriesWithOutsideMembers(t, newSeriesPostgresContext(t))
}

func TestScopedEditResourceLeaveDeletesSeriesWhenSoleMemberPostgres(t *testing.T) {
	runScopedEditLeaveDeletesSeriesWhenSoleMember(t, newSeriesPostgresContext(t))
}

func TestScopedRemoveFromSeriesKeepsSeriesWithOutsideMembersPostgres(t *testing.T) {
	runScopedRemoveFromSeriesKeepsSeriesWithOutsideMembers(t, newSeriesPostgresContext(t))
}

func TestScopedRemoveFromSeriesDeletesSeriesWhenSoleMemberPostgres(t *testing.T) {
	runScopedRemoveFromSeriesDeletesSeriesWhenSoleMember(t, newSeriesPostgresContext(t))
}
