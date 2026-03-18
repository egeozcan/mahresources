package api_tests

import (
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTagDeleteViaFactoryCleansUpJunctionTableRows verifies that deleting a
// tag through the generic CRUDWriter.Delete handler (POST /v1/tag/delete)
// removes the corresponding rows from the group_tags junction table.
//
// The generic CRUDWriter.Delete creates a zero-value entity and calls
// db.Select(clause.Associations).Delete(&entity, id). Because the entity's
// primary key field is 0 (not loaded from the database), GORM's association
// cleanup targets tag_id=0 instead of the actual tag ID, leaving orphaned
// junction table rows behind.
func TestTagDeleteViaFactoryCleansUpJunctionTableRows(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag
	tag := &models.Tag{Name: "Doomed Tag"}
	tc.DB.Create(tag)
	assert.NotZero(t, tag.ID)

	// Create a group and associate the tag with it
	group := tc.CreateDummyGroup("Tag Owner Group")
	tc.DB.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (?, ?)", group.ID, tag.ID)

	// Verify the junction row exists
	var countBefore int64
	tc.DB.Table("group_tags").Where("tag_id = ?", tag.ID).Count(&countBefore)
	assert.Equal(t, int64(1), countBefore, "setup: group_tags should have 1 row for the tag")

	// Delete the tag via the factory handler endpoint
	url := fmt.Sprintf("/v1/tag/delete?Id=%d", tag.ID)
	resp := tc.MakeRequest(http.MethodPost, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify the tag itself is gone
	var tagCheck models.Tag
	result := tc.DB.First(&tagCheck, tag.ID)
	assert.Error(t, result.Error, "tag should be deleted from the database")

	// Verify the junction table row is also cleaned up
	var countAfter int64
	tc.DB.Table("group_tags").Where("tag_id = ?", tag.ID).Count(&countAfter)
	assert.Equal(t, int64(0), countAfter,
		"group_tags junction table should have 0 rows for the deleted tag — "+
			"the generic CRUDWriter.Delete uses a zero-value entity whose ID is 0, "+
			"so GORM's Select(clause.Associations) deletes associations WHERE tag_id=0 "+
			"instead of the actual tag ID, leaving orphaned rows behind")
}
