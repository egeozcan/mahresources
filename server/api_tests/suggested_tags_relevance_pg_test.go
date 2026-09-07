//go:build postgres

package api_tests

import "testing"

func TestSuggestedTags_Postgres_Relevance(t *testing.T) {
	assertSuggestedTagRelevance(t, SetupPostgresTestEnv(t))
}
