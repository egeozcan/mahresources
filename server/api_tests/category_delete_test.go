package api_tests

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestDeleteCategoryDoesNotDeleteGroups(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category
	cat := &models.Category{Name: "Deletable Category"}
	assert.NoError(t, tc.DB.Create(cat).Error)

	// Create a group in that category
	group := &models.Group{
		Name:       "Important Group",
		CategoryId: &cat.ID,
	}
	assert.NoError(t, tc.DB.Create(group).Error)
	groupID := group.ID

	// Verify the group has the category
	var check models.Group
	assert.NoError(t, tc.DB.First(&check, groupID).Error)
	assert.NotNil(t, check.CategoryId)
	assert.Equal(t, cat.ID, *check.CategoryId)

	// Delete the category
	err := tc.AppCtx.DeleteCategory(cat.ID)
	assert.NoError(t, err)

	// The group should still exist with CategoryId set to NULL,
	// NOT be cascade-deleted
	var afterDelete models.Group
	err = tc.DB.First(&afterDelete, groupID).Error
	assert.NoError(t, err,
		"group should still exist after its category is deleted — must be SET NULL, not CASCADE")
	assert.Nil(t, afterDelete.CategoryId,
		"CategoryId should be set to NULL after category deletion")
}

func TestDeleteResourceCategoryDoesNotDeleteResources(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category
	rc := &models.ResourceCategory{Name: "Deletable RC"}
	assert.NoError(t, tc.DB.Create(rc).Error)

	// Create a resource with that category
	file := io.NopCloser(bytes.NewReader([]byte("rc-delete-test-content")))
	resource, err := tc.AppCtx.AddResource(file, "rc-test.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:               "RC Test Resource",
			ResourceCategoryId: rc.ID,
		},
	})
	assert.NoError(t, err)
	resourceID := resource.ID

	// Verify the resource has the category
	var check models.Resource
	assert.NoError(t, tc.DB.First(&check, resourceID).Error)
	assert.NotNil(t, check.ResourceCategoryId)
	assert.Equal(t, rc.ID, *check.ResourceCategoryId)

	// Delete the resource category
	err = tc.AppCtx.DeleteResourceCategory(rc.ID)
	assert.NoError(t, err)

	// The resource should still exist with ResourceCategoryId set to NULL
	var afterDelete models.Resource
	err = tc.DB.First(&afterDelete, resourceID).Error
	assert.NoError(t, err,
		"resource should still exist after its resource category is deleted")
	assert.Nil(t, afterDelete.ResourceCategoryId,
		"ResourceCategoryId should be set to NULL after resource category deletion")
}
