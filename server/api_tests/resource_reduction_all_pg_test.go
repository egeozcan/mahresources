//go:build postgres

package api_tests

import "testing"

// The whole-library walk is id-keyset pages through the scoped handle; the scope
// predicate is SQL, so it is proved on Postgres too.
func TestAWholeLibraryReductionStopsAtTheReviewersSubtreePG(t *testing.T) {
	assertWholeLibraryStopsAtTheSubtree(t, SetupPostgresTestEnv(t))
}
