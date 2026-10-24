//go:build postgres

package api_tests

import "testing"

func TestMRQLJoinsStayInsideTheSubtreePostgres(t *testing.T) {
	runMRQLJoinsStayInsideTheSubtree(t, SetupPostgresTestEnv(t))
}
