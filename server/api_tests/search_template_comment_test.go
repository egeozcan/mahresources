//go:build json1 && fts5

package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A search result's description summarizes the entity's Description, so a
// template comment in it is removed as it is on the detail page. Searching
// twice also covers the cached response.
func TestGlobalSearchDescriptionDropsTemplateComments(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.DB.Create(&models.Note{Name: "commentsearchnote", Description: "shown [comment]hidden[/comment]tail"})

	for range 2 {
		result, err := tc.AppCtx.GlobalSearch(&query_models.GlobalSearchQuery{Query: "commentsearchnote", Limit: 10, Types: []string{"note"}})
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		assert.Equal(t, "shown tail", result.Results[0].Description)
	}
}
