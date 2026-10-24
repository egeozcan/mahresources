//go:build postgres

package api_tests

import "testing"

func TestMRQLConfinedSeesTheWholeSubtreePostgres(t *testing.T) {
	runMRQLConfinedSeesTheWholeSubtree(t, SetupPostgresTestEnv(t))
}

func TestMRQLConfinedRankIgnoresHiddenDocumentsPostgres(t *testing.T) {
	runMRQLConfinedRankIgnoresHiddenDocuments(t, SetupPostgresTestEnv(t))
}
