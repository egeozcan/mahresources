package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

func TestDeleteRelationTypeDoesNotDeleteBackRelationType(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories for the relation types
	catA := &models.Category{Name: "Cat A"}
	catB := &models.Category{Name: "Cat B"}
	assert.NoError(t, tc.DB.Create(catA).Error)
	assert.NoError(t, tc.DB.Create(catB).Error)

	// Create two relation types that are each other's back relation
	// e.g., "Parent Of" <-> "Child Of"
	parentOf := &models.GroupRelationType{
		Name:           "Parent Of",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	assert.NoError(t, tc.DB.Create(parentOf).Error)

	childOf := &models.GroupRelationType{
		Name:           "Child Of",
		FromCategoryId: &catB.ID,
		ToCategoryId:   &catA.ID,
		BackRelationId: &parentOf.ID,
	}
	assert.NoError(t, tc.DB.Create(childOf).Error)

	// Link them: parentOf.BackRelationId = childOf.ID
	parentOf.BackRelationId = &childOf.ID
	assert.NoError(t, tc.DB.Save(parentOf).Error)

	// Verify both exist
	var checkParent, checkChild models.GroupRelationType
	assert.NoError(t, tc.DB.First(&checkParent, parentOf.ID).Error)
	assert.NoError(t, tc.DB.First(&checkChild, childOf.ID).Error)
	assert.NotNil(t, checkParent.BackRelationId)
	assert.NotNil(t, checkChild.BackRelationId)

	// Delete "Parent Of"
	err := tc.AppCtx.DeleteRelationshipType(parentOf.ID)
	assert.NoError(t, err)

	// "Child Of" should still exist with BackRelationId set to NULL,
	// NOT be cascade-deleted
	var afterDelete models.GroupRelationType
	err = tc.DB.First(&afterDelete, childOf.ID).Error
	assert.NoError(t, err,
		"back relation type should still exist after its paired type is deleted — must be SET NULL, not CASCADE")
	assert.Nil(t, afterDelete.BackRelationId,
		"BackRelationId should be set to NULL after paired type is deleted")
}
