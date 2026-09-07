//go:build postgres

package api_tests

import "testing"

func TestSavedSearchCRUDPostgres(t *testing.T) {
	testSavedSearchCRUD(t, SetupPostgresTestEnv(t))
}
