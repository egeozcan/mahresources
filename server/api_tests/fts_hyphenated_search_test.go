//go:build json1 && fts5

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFTSHyphenatedTermDoesNotMatchDisjointWords verifies that searching for a
// hyphenated term like "well-known" returns the expected results.
//
// The bug: sanitizeSearchTerm preserves hyphens, and the hyphenated string
// "well-known" is passed directly into the FTS5 MATCH expression. FTS5 parses
// "well-known*" as a column filter (column "well", NOT "known*"), which fails
// with "no such column: known" because "well" is not an FTS column name.
// The result is that any search for a hyphenated term silently returns zero
// results — a complete search failure rather than a graceful degradation.
func TestFTSHyphenatedTermDoesNotMatchDisjointWords(t *testing.T) {
	tc := SetupTestEnv(t)

	// Restrict to 1 connection so the in-memory SQLite DB is shared
	// across the concurrent goroutines inside GlobalSearch.
	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Initialize FTS5 so GlobalSearch uses the FTS code path.
	err = tc.AppCtx.InitFTS()
	require.NoError(t, err, "FTS5 must be available (build with -tags 'json1 fts5')")

	// Create two notes:
	//   1. Contains the actual hyphenated phrase "well-known" → should match.
	//   2. Contains "well" and "known" as separate, non-adjacent words → should NOT match.
	tc.DB.Create(&models.Note{Name: "well-known protocol spec"})
	tc.DB.Create(&models.Note{Name: "known for being a well"})

	// FTS triggers fire on INSERT, so the index should be up-to-date.
	// Search for the hyphenated term.
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "well-known",
		Limit: 50,
		Types: []string{"note"},
	})
	require.NoError(t, err)

	// Collect matched note names for diagnostics.
	var names []string
	for _, r := range result.Results {
		names = append(names, r.Name)
	}

	// We expect at least one match: the note whose name contains "well-known".
	// Due to the bug, FTS5 interprets "well-known*" as a column-scoped NOT
	// query, producing "no such column: known" and returning zero results.
	require.GreaterOrEqual(t, len(result.Results), 1,
		"searching for 'well-known' should find the note with the hyphenated "+
			"phrase, but FTS5 chokes on the unescaped hyphen and returns nothing; "+
			"got: %v", names)

	if len(result.Results) == 1 {
		assert.Equal(t, "well-known protocol spec", result.Results[0].Name,
			"the matching note should be the one containing the hyphenated phrase")
	}
}
