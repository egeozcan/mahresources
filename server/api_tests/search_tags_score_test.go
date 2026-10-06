package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchTagMatchGetsAdequateScore(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	searchTerm := "IMG_20230915_142355"

	tag := &models.Tag{
		Name: searchTerm,
	}
	tc.DB.Create(tag)

	// Resource A: description contains term, but ALSO has a tag matching exactly.
	resourceA := &models.Resource{
		Name:        "Some Random Resource",
		Description: "A generic description with " + searchTerm + " inside",
		Hash:        "aaa111",
		HashType:    "SHA1",
		Location:    "/test/a.jpg",
	}
	tc.DB.Create(resourceA)
	tc.DB.Model(resourceA).Association("Tags").Append(tag)

	// Resource B: description contains term, but NO tag matching.
	resourceB := &models.Resource{
		Name:        "A completely different thing",
		Description: "Has the word " + searchTerm + " buried inside",
		Hash:        "bbb222",
		HashType:    "SHA1",
		Location:    "/test/b.jpg",
	}
	tc.DB.Create(resourceB)

	// Search
	result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{
		Query: searchTerm,
		Limit: 50,
		Types: []string{"resource"},
	})
	require.NoError(t, err)

	// Find scores for each resource
	var scoreA, scoreB int
	foundA := false
	foundB := false
	for _, r := range result.Results {
		if r.ID == resourceA.ID {
			scoreA = r.Score
			foundA = true
		}
		if r.ID == resourceB.ID {
			scoreB = r.Score
			foundB = true
		}
	}

	require.True(t, foundA, "resource A (tag match) should appear in results")
	require.True(t, foundB, "resource B (description match) should appear in results")

	assert.Greater(t, scoreA, scoreB,
		"BUG: resource with exact tag match (score=%d) should rank higher than "+
			"resource with substring description match (score=%d)", scoreA, scoreB)
}
