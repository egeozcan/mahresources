package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchCacheInvalidationAfterEntityCreate demonstrates that the search
// cache returns stale empty results after creating an entity that matches a
// previously-cached search term.
//
// The bug is in SearchCache.InvalidateByType: it only removes cache entries
// whose results already contain the specified entity type. When a search
// returns zero results, the entry's type set is empty, so no invalidation
// event will ever remove it. Subsequent searches for the same term keep
// returning the cached empty result even after a matching entity is created.
//
// Repro steps:
//  1. Search for a unique term that matches nothing -> cached with types={}
//  2. Create a tag whose name matches the search term
//  3. InvalidateSearchCacheByType("tag") fires (but does nothing: types={})
//  4. Search again -> cache hit returns stale empty result
func TestSearchCacheInvalidationAfterEntityCreate(t *testing.T) {
	tc := SetupTestEnv(t)

	// Use a single connection so the in-memory SQLite DB is shared across
	// the concurrent goroutines inside GlobalSearch.
	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	const searchTerm = "zzuniquecacheterm"

	// Step 1: Search for a term that doesn't exist yet.
	// This populates the cache with an empty result set (types = {}).
	result1, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: searchTerm,
		Limit: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, len(result1.Results),
		"sanity check: no results should exist yet for %q", searchTerm)

	// Step 2: Create a tag that matches the search term.
	tc.DB.Create(&models.Tag{Name: searchTerm + "_tag"})

	// Step 3: Invalidate the cache for entity type "tag".
	// This is what the application does internally after creating a tag.
	tc.AppCtx.InvalidateSearchCacheByType("tag")

	// Step 4: Search again. If the cache was properly invalidated, the new
	// tag should appear. If the bug is present, the stale empty result is
	// returned from the cache.
	result2, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: searchTerm,
		Limit: 20,
	})
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(result2.Results), 1,
		"BUG: search for %q returned 0 results after creating a matching tag; "+
			"the cache entry with empty type set was not invalidated by "+
			"InvalidateSearchCacheByType(\"tag\"), so the stale empty result "+
			"was served from the cache", searchTerm)
}
