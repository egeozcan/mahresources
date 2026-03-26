package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchDoesNotTreatUnderscoreAsWildcard(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Create tags: one with underscore, one similar but no underscore
	tc.DB.Create(&models.Tag{Name: "data_point"})
	tc.DB.Create(&models.Tag{Name: "dataXpoint"})

	// Search for the literal underscore name
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "data_point",
		Limit: 50,
		Types: []string{"tag"},
	})
	assert.NoError(t, err)

	// Should find only "data_point", not "dataXpoint"
	// If underscore is treated as SQL LIKE wildcard, both would match
	names := make([]string, 0, len(result.Results))
	for _, r := range result.Results {
		names = append(names, r.Name)
	}
	assert.Equal(t, 1, len(result.Results),
		"search for 'data_point' should match only the literal underscore, not treat _ as wildcard; got: %v", names)
}

func TestSearchTotalReflectsAllResults(t *testing.T) {
	tc := SetupTestEnv(t)

	// Restrict to 1 connection so SQLite in-memory DB is shared across
	// the concurrent goroutines in GlobalSearch
	sqlDB, err := tc.DB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Create 5 tags that all match the search term "zzuniqueterm"
	for i := 1; i <= 5; i++ {
		tag := &models.Tag{Name: fmt.Sprintf("zzuniqueterm_tag_%d", i)}
		tc.DB.Create(tag)
	}

	// First search: limit=2. Populates the cache with all results, returns only 2.
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "zzuniqueterm",
		Limit: 2,
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Results), "should return 2 items (respecting limit)")
	assert.Equal(t, 5, result.Total,
		"total should be 5 (all matching results), not the trimmed count")

	// Second search hits cache path, different limit.
	result2, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "zzuniqueterm",
		Limit: 3,
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result2.Results), "should return 3 items from cache")
	assert.Equal(t, 5, result2.Total,
		"total from cache should be 5 (all cached results), not the trimmed count")
}

func TestSearchSpecialCharactersReturnNoResults(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Create some entities so the DB is not empty
	tc.DB.Create(&models.Tag{Name: "RealTag"})
	tc.DB.Create(&models.Tag{Name: "AnotherTag"})

	// Search queries consisting only of special characters should return
	// zero results. Before the fix, the sanitized term was empty which
	// caused the FTS scope to apply no WHERE clause, returning everything.
	specialQueries := []string{
		"'",
		"''",
		"\"",
		"<>",
		"&&",
		"@#$%",
		"()",
		"!!!",
	}

	for _, q := range specialQueries {
		result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: q,
			Limit: 50,
		})
		assert.NoError(t, err, "query=%q", q)
		assert.Equal(t, 0, result.Total,
			"search for %q (only special chars) should return 0 results, got %d", q, result.Total)
		assert.Equal(t, 0, len(result.Results),
			"search for %q should return empty results slice", q)
	}
}

func TestSearchReturnsEmptyArrayNotNull(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	assert.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: "zzzznonexistentterm12345",
		Limit: 20,
	})
	assert.NoError(t, err)
	assert.NotNil(t, result.Results, "Results should not be nil")
	assert.Equal(t, 0, len(result.Results))

	// Verify JSON marshaling produces [] not null
	data, err := json.Marshal(result)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"results":[]`)
	assert.NotContains(t, string(data), `"results":null`)
}
