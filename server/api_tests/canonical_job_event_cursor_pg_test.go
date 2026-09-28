//go:build postgres

package api_tests

import "testing"

func TestCanonicalJobSSEReadsPastEventsItsViewerCannotSeeOnPostgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	testCanonicalJobSSEReadsPastEventsItsViewerCannotSee(t, tc)
}
