//go:build postgres

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFTSPostgresPrefixLongerThanStemMatches is a regression test for a
// Postgres FTS defect: search_vector holds English stems, so a typed prefix
// longer than the stored stem never matched. "Mentionab" missed "Mentionable"
// (stored as 'mention') and "runni" missed "Running" (stored as 'run'), which
// broke typeahead (global search, @-mentions) part way through a word. Prefix
// search now also matches each word unstemmed.
func TestFTSPostgresPrefixLongerThanStemMatches(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// Rows written before FTS setup, as on an upgraded database.
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Mentionable widget"}).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Running shoes"}).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Plain title", Description: "notes about generators"}).Error)

	require.NoError(t, tc.AppCtx.InitFTS(), "Postgres FTS must initialize")

	require.NoError(t, tc.DB.Create(&models.Note{Name: "Running Mentionable"}).Error)
	require.NoError(t, tc.DB.Create(&models.Note{Name: "Theory notes"}).Error)

	search := func(query string) []query_models.SearchResultItem {
		t.Helper()
		// Types is set, so the process-wide result cache is bypassed.
		res, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: query,
			Limit: 50,
			Types: []string{"note"},
		})
		require.NoError(t, err)
		return res.Results
	}

	for _, c := range []struct{ query, want string }{
		{"Mentionab", "Mentionable widget"},
		{"runni", "Running shoes"},
		{"generato", "Plain title"},
		// The stemmed match still applies: "runs" stems to 'run'.
		{"runs", "Running shoes"},
		{"Mentionable", "Mentionable widget"},
		// Each word may match either way.
		{"runs mentionab", "Running Mentionable"},
		// A stopword constrains nothing beside other words...
		{"the mentionab", "Mentionable widget"},
		// ...and is matched as a prefix when it is the whole query.
		{"the", "Theory notes"},
	} {
		results := search(c.query)
		require.True(t, ftsResultsContain(results, c.want),
			"prefix search for %q must find %q; got %v", c.query, c.want, ftsResultNames(results))
	}

	// Every word must match: "runs mentionab" is not "runs" alone.
	require.False(t, ftsResultsContain(search("runs mentionab"), "Running shoes"))

	// The unstemmed index is built in the background and ends up valid.
	require.Eventually(t, func() bool {
		var valid int64
		tc.DB.Raw(`
			SELECT COUNT(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
			WHERE c.relname = 'idx_notes_fts_simple' AND i.indisvalid
		`).Scan(&valid)
		return valid == 1
	}, 10*time.Second, 50*time.Millisecond)
}
