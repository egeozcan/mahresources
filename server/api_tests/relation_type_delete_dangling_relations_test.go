package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

// TestDeleteRelationTypeLeavesDanglingGroupRelations verifies that
// DeleteRelationshipType properly cleans up GroupRelation records that
// reference the deleted type.
//
// On SQLite, FK ON DELETE CASCADE constraints do not fire reliably
// inside transactions. Other delete functions (e.g., DeleteCategory)
// explicitly delete dependent GroupRelation rows before removing the
// type, but DeleteRelationshipType does not — leaving orphaned
// group_relations rows with an invalid relation_type_id.
func TestDeleteRelationTypeLeavesDanglingGroupRelations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	catA := &models.Category{Name: "Cat A"}
	catB := &models.Category{Name: "Cat B"}
	assert.NoError(t, tc.DB.Create(catA).Error)
	assert.NoError(t, tc.DB.Create(catB).Error)

	// Create two groups, one per category
	groupA := &models.Group{Name: "Group A", CategoryId: &catA.ID, Meta: []byte(`{}`)}
	groupB := &models.Group{Name: "Group B", CategoryId: &catB.ID, Meta: []byte(`{}`)}
	assert.NoError(t, tc.DB.Create(groupA).Error)
	assert.NoError(t, tc.DB.Create(groupB).Error)

	// Create a relation type A→B
	relType := &models.GroupRelationType{
		Name:           "Relates To",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	assert.NoError(t, tc.DB.Create(relType).Error)

	// Create a GroupRelation using that type
	relation := &models.GroupRelation{
		FromGroupId:    &groupA.ID,
		ToGroupId:      &groupB.ID,
		RelationTypeId: &relType.ID,
		Name:           "test relation",
	}
	assert.NoError(t, tc.DB.Create(relation).Error)

	// Verify the relation exists
	var countBefore int64
	tc.DB.Model(&models.GroupRelation{}).Where("relation_type_id = ?", relType.ID).Count(&countBefore)
	assert.Equal(t, int64(1), countBefore, "relation should exist before deleting the type")

	// Delete the relation type
	err := tc.AppCtx.DeleteRelationshipType(relType.ID)
	assert.NoError(t, err, "DeleteRelationshipType should succeed")

	// Verify the relation type is gone
	var typeCheck models.GroupRelationType
	assert.Error(t, tc.DB.First(&typeCheck, relType.ID).Error, "relation type should be deleted")

	// BUG: GroupRelation rows referencing the deleted type should also be gone.
	// Because SQLite FK cascades are unreliable and DeleteRelationshipType does
	// not explicitly remove them (unlike DeleteCategory which does), these rows
	// survive as orphans.
	var countAfter int64
	tc.DB.Model(&models.GroupRelation{}).Where("id = ?", relation.ID).Count(&countAfter)
	assert.Equal(t, int64(0), countAfter,
		"GroupRelation rows referencing the deleted relation type should be cleaned up, "+
			"but they survive because DeleteRelationshipType does not explicitly delete them "+
			"and SQLite FK cascades do not fire reliably")
}
