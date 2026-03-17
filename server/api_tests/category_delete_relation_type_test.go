package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"mahresources/models"
)

// TestDeleteCategoryCascadesRelationTypes verifies that deleting a category also
// removes GroupRelationType records that reference it via FromCategoryId or
// ToCategoryId. The GORM model declares OnDelete:CASCADE for both FK columns,
// but SQLite's PRAGMA foreign_keys is unreliable inside transactions, so the
// application must handle this explicitly — just as it does for other entities.
//
// BUG: DeleteCategory only clears CategoryId on groups; it does NOT cascade-delete
// (or clean up) GroupRelationType records whose FromCategoryId/ToCategoryId point
// to the deleted category. This leaves dangling FK references that break
// AddRelation (which validates category matching).
func TestDeleteCategoryCascadesRelationTypes(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	catA := &models.Category{Name: "Category A"}
	catB := &models.Category{Name: "Category B"}
	assert.NoError(t, tc.DB.Create(catA).Error)
	assert.NoError(t, tc.DB.Create(catB).Error)

	// Create a relation type that uses catA as FromCategory and catB as ToCategory
	relType := &models.GroupRelationType{
		Name:           "Relates To",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	assert.NoError(t, tc.DB.Create(relType).Error)
	relTypeID := relType.ID

	// Verify the relation type exists and has the correct categories
	var check models.GroupRelationType
	assert.NoError(t, tc.DB.First(&check, relTypeID).Error)
	assert.NotNil(t, check.FromCategoryId)
	assert.Equal(t, catA.ID, *check.FromCategoryId)

	// Delete Category A (which is the FromCategory of the relation type)
	err := tc.AppCtx.DeleteCategory(catA.ID)
	assert.NoError(t, err)

	// The GroupRelationType should be cascade-deleted (per the OnDelete:CASCADE constraint),
	// or at minimum its FromCategoryId should be cleaned up.
	// Since the GORM model specifies OnDelete:CASCADE, the relation type should not exist.
	var afterDelete models.GroupRelationType
	err = tc.DB.First(&afterDelete, relTypeID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"GroupRelationType should be cascade-deleted when its FromCategory is deleted, "+
			"but it still exists with a dangling FK reference to the deleted category")
}

// TestDeleteCategoryCascadesRelationTypes_ToCategory is the same test but for
// the ToCategoryId FK column.
func TestDeleteCategoryCascadesRelationTypes_ToCategory(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	catA := &models.Category{Name: "Category A"}
	catB := &models.Category{Name: "Category B"}
	assert.NoError(t, tc.DB.Create(catA).Error)
	assert.NoError(t, tc.DB.Create(catB).Error)

	// Create a relation type that uses catA as FromCategory and catB as ToCategory
	relType := &models.GroupRelationType{
		Name:           "Relates To",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	assert.NoError(t, tc.DB.Create(relType).Error)
	relTypeID := relType.ID

	// Delete Category B (which is the ToCategory of the relation type)
	err := tc.AppCtx.DeleteCategory(catB.ID)
	assert.NoError(t, err)

	// The GroupRelationType should be cascade-deleted
	var afterDelete models.GroupRelationType
	err = tc.DB.First(&afterDelete, relTypeID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"GroupRelationType should be cascade-deleted when its ToCategory is deleted, "+
			"but it still exists with a dangling FK reference to the deleted category")
}

// TestDeleteCategoryCascadesRelationTypesAndRelations verifies the full cascade:
// deleting a category should also delete relation types that use it, AND the
// group relations that use those relation types.
func TestDeleteCategoryCascadesRelationTypesAndRelations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	catA := &models.Category{Name: "Category A"}
	catB := &models.Category{Name: "Category B"}
	assert.NoError(t, tc.DB.Create(catA).Error)
	assert.NoError(t, tc.DB.Create(catB).Error)

	// Create a relation type
	relType := &models.GroupRelationType{
		Name:           "Relates To",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	assert.NoError(t, tc.DB.Create(relType).Error)
	relTypeID := relType.ID

	// Create two groups in the appropriate categories
	groupA := &models.Group{Name: "Group A", CategoryId: &catA.ID}
	groupB := &models.Group{Name: "Group B", CategoryId: &catB.ID}
	assert.NoError(t, tc.DB.Create(groupA).Error)
	assert.NoError(t, tc.DB.Create(groupB).Error)

	// Create a relation using that type
	relation := &models.GroupRelation{
		FromGroupId:    &groupA.ID,
		ToGroupId:      &groupB.ID,
		RelationTypeId: &relType.ID,
		Name:           "Test Relation",
	}
	assert.NoError(t, tc.DB.Create(relation).Error)
	relationID := relation.ID

	// Delete Category A
	err := tc.AppCtx.DeleteCategory(catA.ID)
	assert.NoError(t, err)

	// The relation type should be gone
	var afterDeleteType models.GroupRelationType
	err = tc.DB.First(&afterDeleteType, relTypeID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"GroupRelationType should be cascade-deleted when its FromCategory is deleted")

	// The relation itself should also be gone (it depends on the relation type)
	var afterDeleteRelation models.GroupRelation
	err = tc.DB.First(&afterDeleteRelation, relationID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"GroupRelation should be cascade-deleted when its RelationType is deleted")
}
