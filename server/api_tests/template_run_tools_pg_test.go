//go:build postgres

package api_tests

import "testing"

// The readers' SQL (LIMIT n+1 paging, the IN/UNION link queries, the scope
// predicate) on Postgres, through the same checks the SQLite run makes.
func TestToolReadersOnPostgres(t *testing.T) {
	t.Run("readers", func(t *testing.T) { checkToolReaders(t, newToolFixtureOn(t, SetupPostgresTestEnv(t), nil)) })
	t.Run("revision paging", func(t *testing.T) { checkToolRevisionPaging(t, newToolFixtureOn(t, SetupPostgresTestEnv(t), nil)) })
	t.Run("scope text", func(t *testing.T) { checkToolScopeText(t, newToolFixtureOn(t, SetupPostgresTestEnv(t), nil)) })
}
