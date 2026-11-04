package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scores depend on tag names, so renaming a tag must drop cached resource
// results; otherwise the old (tag-less) ranking is served until the TTL.
func TestSearchTagRenameInvalidatesCachedResourceScores(t *testing.T) {
	tc := SetupTestEnv(t)

	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	const term = "zztagrenameterm"
	tag := &models.Tag{Name: "placeholder"}
	require.NoError(t, tc.DB.Create(tag).Error)
	res := &models.Resource{
		Name: "plain", Description: "mentions " + term, Hash: "ccc333",
		HashType: "SHA1", Location: "/test/c.jpg",
	}
	require.NoError(t, tc.DB.Create(res).Error)
	require.NoError(t, tc.DB.Model(res).Association("Tags").Append(tag))

	scoreOf := func() int {
		out, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{Query: term, Limit: 50})
		require.NoError(t, err)
		for _, r := range out.Results {
			if r.Type == "resource" && r.ID == res.ID {
				return r.Score
			}
		}
		t.Fatalf("resource not found in results for %q", term)
		return 0
	}

	require.Equal(t, 40, scoreOf(), "description-only match before the rename")

	_, err = tc.AppCtx.UpdateTag(&query_models.TagCreator{ID: tag.ID, Name: term})
	require.NoError(t, err)

	assert.Equal(t, 90, scoreOf(), "exact tag match after the rename, not the cached score")
}
