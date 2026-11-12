//go:build postgres

package api_tests

import "testing"

// The confinement predicates are raw SQL with a recursive CTE inside a
// correlated EXISTS, so they run on Postgres too.
func TestListFiltersDoNotReadOutsideTheSubtreePostgres(t *testing.T) {
	runListFiltersDoNotReadOutsideTheSubtree(t, SetupPostgresTestEnv(t))
}

func TestRelationTypeGroupFiltersHideInvisibleGroupsPostgres(t *testing.T) {
	runRelationTypeGroupFiltersHideInvisibleGroups(t, SetupPostgresTestEnv(t))
}
