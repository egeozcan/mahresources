//go:build postgres

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFTSPostgresPrefixLongerThanStemMatches is a regression test for a
// Postgres FTS defect: search_vector holds English stems, so a typed prefix
// longer than the stored stem never matched. "Mentionab" missed "Mentionable"
// (stored as 'mention') and "runni" missed "Running" (stored as 'run'), which
// broke typeahead (global search, @-mentions) part way through a word. Prefix
// search now also matches an unstemmed vector.
func TestFTSPostgresPrefixLongerThanStemMatches(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	require.NoError(t, tc.AppCtx.InitFTS(), "Postgres FTS must initialize")

	require.NoError(t, tc.DB.Create(&models.Note{Name: "Mentionable widget"}).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Running shoes"}).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Plain title", Description: "notes about generators"}).Error)

	cases := []struct{ query, want string }{
		{"Mentionab", "Mentionable widget"},
		{"runni", "Running shoes"},
		{"generato", "Plain title"},
		// The stemmed match still applies: "runs" stems to 'run'.
		{"runs", "Running shoes"},
		{"Mentionable", "Mentionable widget"},
	}
	for _, c := range cases {
		// Types is set, so the process-wide result cache is bypassed.
		res, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: c.query,
			Limit: 50,
			Types: []string{"note"},
		})
		require.NoError(t, err)
		require.True(t, ftsResultsContain(res.Results, c.want),
			"prefix search for %q must find %q; got %v", c.query, c.want, ftsResultNames(res.Results))
	}
}
