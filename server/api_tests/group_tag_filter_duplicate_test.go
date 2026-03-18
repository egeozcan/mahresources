package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGroupTagFilterWithDuplicateTagIDs demonstrates that passing duplicate
// tag IDs in the Tags filter causes the query to return zero results even
// when matching groups exist.
//
// Root cause:
// GroupQuery builds a subquery that counts `count(distinct tag_id)` where
// `tag_id IN ?` and compares the result with `len(query.Tags)`. When the
// input contains duplicates like [1, 1], SQL `IN (1, 1)` is equivalent to
// `IN (1)`, so `count(distinct tag_id)` returns 1. But `len(query.Tags)`
// is 2. The condition `1 = 2` is never true, so no groups match.
//
// This can happen in practice when a user double-clicks a tag filter in the
// UI or when the CLI sends repeated `--tag` flags with the same value.
//
// Expected: groups with tag 1 should still be found.
// Actual (bug): no groups are returned.
func TestGroupTagFilterWithDuplicateTagIDs(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag
	tag := &models.Tag{Name: "Duplicate Filter Tag"}
	require.NoError(t, tc.DB.Create(tag).Error)

	// Create a group and associate it with the tag
	group := tc.CreateDummyGroup("Tagged Group")
	require.NoError(t, tc.DB.Exec(
		"INSERT INTO group_tags (group_id, tag_id) VALUES (?, ?)",
		group.ID, tag.ID,
	).Error)

	// Verify the tag association exists
	var tagCount int64
	tc.DB.Table("group_tags").Where("group_id = ? AND tag_id = ?", group.ID, tag.ID).Count(&tagCount)
	require.Equal(t, int64(1), tagCount, "setup: group should have the tag")

	// Query with a single tag ID — should find the group
	t.Run("single tag ID finds group", func(t *testing.T) {
		url := fmt.Sprintf("/v1/groups?Tags=%d", tag.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		json.Unmarshal(resp.Body.Bytes(), &groups)
		assert.Equal(t, 1, len(groups), "single tag ID should find the group")
	})

	// Query with the same tag ID duplicated — should still find the group
	t.Run("duplicate tag ID still finds group", func(t *testing.T) {
		url := fmt.Sprintf("/v1/groups?Tags=%d&Tags=%d", tag.ID, tag.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var groups []models.Group
		json.Unmarshal(resp.Body.Bytes(), &groups)
		assert.Equal(t, 1, len(groups),
			"BUG: duplicate tag IDs in filter should be deduplicated; "+
				"group_scope.go compares count(distinct tag_id) with len(query.Tags), "+
				"but len includes duplicates so the condition can never be satisfied")
	})
}
