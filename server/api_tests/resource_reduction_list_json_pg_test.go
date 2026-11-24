//go:build postgres

package api_tests

import "testing"

func TestReductionListJSONCarriesProgressAndUpgradingPG(t *testing.T) {
	testReductionListJSONCarriesProgressAndUpgrading(t, SetupPostgresTestEnv(t))
}
