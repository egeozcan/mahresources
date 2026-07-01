//go:build postgres

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFTSPostgresHyphenatedNumberNameFindableByOwnName is a regression test for
// a Postgres FTS defect: a row whose name contained a hyphenated number+alnum
// token could not be found by searching its own exact name.
//
// Postgres' English text-search parser reads the hyphen+digit run as a
// signed-integer lexeme (e.g. "2024-3q" -> lexeme "-3" plus "q") while global
// search collapsed the hyphen to a space and queried "...3q:*", which never
// matched the stored search_vector. Measured against real Postgres this missed
// ~27% of hyphen+digit tokens — realistic values such as dates, order IDs,
// SKUs and batch numbers. The fix (fts/postgres.go BuildSearchScope) derives the
// prefix tsquery from the raw term's OWN to_tsvector lexemes so the query
// tokenizes identically to the stored vector.
func TestFTSPostgresHyphenatedNumberNameFindableByOwnName(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	err := tc.AppCtx.InitFTS()
	require.NoError(t, err, "Postgres FTS must initialize")

	// Every name here deterministically failed the old split+':*' query (the
	// token after the hyphen starts with a digit) and must now be found.
	names := []string{
		"Invoice 2024-3q",
		"Order 5-7alpha",
		"Batch 900-1x",
		"SKU 12-8ab",
		"Report 2024-07",
	}
	for _, name := range names {
		require.NoError(t, tc.DB.Create(&models.Note{Name: name}).Error)
	}

	// Default (prefix) search: each note must be findable by its exact name.
	// Types is set, so the process-wide result cache is bypassed and each query
	// exercises the real FTS path.
	for _, name := range names {
		res, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: name,
			Limit: 50,
			Types: []string{"note"},
		})
		require.NoError(t, err)
		require.True(t, ftsResultsContain(res.Results, name),
			"prefix search for %q must find the note with that exact name; got %v",
			name, ftsResultNames(res.Results))
	}

	// Explicit exact mode ("=name") exercises the same tokenization fix.
	exactName := "Invoice 2024-3q"
	res, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "=" + exactName,
		Limit: 50,
		Types: []string{"note"},
	})
	require.NoError(t, err)
	require.True(t, ftsResultsContain(res.Results, exactName),
		"exact search for %q must find its note; got %v", exactName, ftsResultNames(res.Results))
}

func ftsResultsContain(results []query_models.SearchResultItem, name string) bool {
	for _, r := range results {
		if r.Name == name {
			return true
		}
	}
	return false
}

func ftsResultNames(results []query_models.SearchResultItem) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Name)
	}
	return out
}
