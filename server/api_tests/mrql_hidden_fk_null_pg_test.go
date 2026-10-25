//go:build postgres

package api_tests

import "testing"

func TestMRQLHiddenParentBehavesLikeNullPostgres(t *testing.T) {
	runMRQLHiddenParentBehavesLikeNull(t, SetupPostgresTestEnv(t))
}
