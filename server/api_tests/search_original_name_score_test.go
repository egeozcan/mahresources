package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchOriginalNameMatchGetsAdequateScore demonstrates that a resource
// whose original_name exactly matches the search term receives a poor
// relevance score.
//
// The LIKE-based search query correctly includes original_name in its
// WHERE clause (via extraLikeCols), so resources ARE found when only
// their original_name matches. However, calculateRelevanceScore only
// examines the name and description fields — it ignores original_name
// entirely. As a result, a resource whose original_name is an exact
// match gets the minimum score of 20 (the "nothing matched" fallback),
// which ranks it below resources that merely contain the search term
// somewhere in their description.
//
// Steps to reproduce:
//  1. Create a resource with a distinctive original_name that does NOT
//     appear in its name or description.
//  2. Create a second resource that mentions the same term only in its
//     description (as a substring).
//  3. Search for the distinctive term.
//  4. The resource with the exact original_name match should score
//     higher than a loose description mention, but it scores equal or
//     lower because calculateRelevanceScore never checks original_name.
func TestSearchOriginalNameMatchGetsAdequateScore(t *testing.T) {
	tc := SetupTestEnv(t)

	// Restrict to 1 connection so SQLite in-memory DB is shared across
	// the concurrent goroutines in GlobalSearch
	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	searchTerm := "IMG_20230915_142355"

	// Resource A: original_name exactly matches, but name and description do not
	resourceA := &models.Resource{
		Name:         "Photo from vacation",
		Description:  "A sunset at the beach",
		OriginalName: searchTerm,
		Hash:         "aaa111",
		HashType:     "SHA1",
		Location:     "/test/a.jpg",
	}
	tc.DB.Create(resourceA)

	// Resource B: name and original_name do NOT match, but description
	// contains the search term as a substring buried in other text.
	resourceB := &models.Resource{
		Name:         "Camera dump notes",
		Description:  "Contains files like " + searchTerm + " and others from that day",
		OriginalName: "dump_notes.txt",
		Hash:         "bbb222",
		HashType:     "SHA1",
		Location:     "/test/b.txt",
	}
	tc.DB.Create(resourceB)

	// Search for the original filename
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: searchTerm,
		Limit: 50,
		Types: []string{"resource"},
	})
	require.NoError(t, err)

	// Both resources should be found
	require.GreaterOrEqual(t, len(result.Results), 2,
		"search should find both resources (one via original_name, one via description)")

	// Find scores for each resource
	var scoreA, scoreB int
	for _, r := range result.Results {
		if r.ID == resourceA.ID {
			scoreA = r.Score
		}
		if r.ID == resourceB.ID {
			scoreB = r.Score
		}
	}

	require.NotZero(t, scoreA, "resource A (original_name match) should appear in results")
	require.NotZero(t, scoreB, "resource B (description match) should appear in results")

	// Resource A has the search term as its exact original_name — this is
	// the strongest possible match for an original_name search. It should
	// score HIGHER than resource B which merely contains the term as a
	// substring in its description.
	//
	// BUG: calculateRelevanceScore only checks name and description.
	// Resource A gets score 20 (nothing matched in name/description).
	// Resource B gets score 40 (description contains the term).
	// So the exact original_name match is ranked BELOW the description mention.
	assert.Greater(t, scoreA, scoreB,
		"BUG: resource with exact original_name match (score=%d) should rank higher than "+
			"resource with substring description match (score=%d), but calculateRelevanceScore "+
			"ignores original_name entirely", scoreA, scoreB)
}
