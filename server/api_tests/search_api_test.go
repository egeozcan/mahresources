package api_tests

import (
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
