//go:build postgres

package api_tests

import "testing"

func TestMRQLAggregatedValuesRenderAsValuesPostgres(t *testing.T) {
	runMRQLAggregatedValuesRenderAsValues(t, SetupPostgresTestEnv(t))
}
