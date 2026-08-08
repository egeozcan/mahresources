//go:build postgres

package api_tests

import "testing"

func TestScopedGroupDeletePostgresPreservesScopeConfinement(t *testing.T) {
	assertScopedGroupDeleteConflict(t, SetupPostgresTestEnv(t), "postgres-scope-delete")
}
