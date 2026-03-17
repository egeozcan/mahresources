package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEditRelationTypeCleansUpBackRelationRecords verifies that when a relation
// type's category is changed, GroupRelation records for the BACK-relation type
// are also cleaned up. Without this, changing a relation type's from/to category
// leaves orphaned back-relation records pointing to groups whose categories no
// longer match the semantic relationship.
//
// Scenario:
//  1. Relation type T (from:A, to:B) with back-relation T2 (from:B, to:A)
//  2. Relation GA->GB of type T auto-creates GB->GA of type T2
//  3. Edit T: change FromCategory from A to C
//  4. GA->GB of type T is deleted (GA has category A, not C)
//  5. BUG: GB->GA of type T2 survives — T2 still says to_category=A and GA has
//     category A, so the record looks "valid" in isolation. But the semantic
//     pair is broken: the forward relation is gone, the back-relation remains.
func TestEditRelationTypeCleansUpBackRelationRecords(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create three categories
	catA := &models.Category{Name: "Cat A"}
	catB := &models.Category{Name: "Cat B"}
	catC := &models.Category{Name: "Cat C"}
	tc.DB.Create(catA)
	tc.DB.Create(catB)
	tc.DB.Create(catC)

	// Create relation type T (from:A, to:B) with back-relation "Reverse T"
	relTypeT := &models.GroupRelationType{
		Name:           "Forward Rel",
		FromCategoryId: &catA.ID,
		ToCategoryId:   &catB.ID,
	}
	tc.DB.Create(relTypeT)

	// Create back-relation type T2 (from:B, to:A) and link them bidirectionally
	relTypeT2 := &models.GroupRelationType{
		Name:           "Reverse Rel",
		FromCategoryId: &catB.ID,
		ToCategoryId:   &catA.ID,
		BackRelationId: &relTypeT.ID,
	}
	tc.DB.Create(relTypeT2)
	relTypeT.BackRelationId = &relTypeT2.ID
	tc.DB.Save(relTypeT)

	// Create groups with the right categories
	groupA := &models.Group{Name: "Group A", CategoryId: &catA.ID}
	groupB := &models.Group{Name: "Group B", CategoryId: &catB.ID}
	tc.DB.Create(groupA)
	tc.DB.Create(groupB)

	// Create the forward relation GA->GB of type T
	forwardRel := &models.GroupRelation{
		FromGroupId:    &groupA.ID,
		ToGroupId:      &groupB.ID,
		RelationTypeId: &relTypeT.ID,
		Name:           "forward",
	}
	tc.DB.Create(forwardRel)

	// Create the back-relation GB->GA of type T2 (simulates what AddRelation does)
	backRel := &models.GroupRelation{
		FromGroupId:    &groupB.ID,
		ToGroupId:      &groupA.ID,
		RelationTypeId: &relTypeT2.ID,
		Name:           "backward",
	}
	tc.DB.Create(backRel)

	// Verify both relations exist
	var countBefore int64
	tc.DB.Model(&models.GroupRelation{}).Count(&countBefore)
	assert.Equal(t, int64(2), countBefore, "Should have 2 relations before edit")

	// Edit relation type T: change FromCategory from A to C.
	// This should invalidate GA->GB of type T because GA has category A, not C.
	// It should ALSO clean up GB->GA of type T2 (the back-relation), because
	// the forward relation it mirrors has been invalidated.
	_, err := tc.AppCtx.EditRelationType(&query_models.RelationshipTypeEditorQuery{
		Id:           relTypeT.ID,
		Name:         "Forward Rel",
		FromCategory: catC.ID,
		ToCategory:   catB.ID,
	})
	assert.NoError(t, err, "EditRelationType should succeed")

	// The forward relation GA->GB of type T should be gone
	var forwardCount int64
	tc.DB.Model(&models.GroupRelation{}).
		Where("relation_type_id = ?", relTypeT.ID).
		Count(&forwardCount)
	assert.Equal(t, int64(0), forwardCount,
		"Forward relation (type T) should be deleted because GA's category (A) "+
			"no longer matches T's new from_category (C)")

	// The back-relation GB->GA of type T2 should ALSO be gone, because the
	// forward relation it mirrors has been invalidated. Keeping the back-relation
	// when the forward is deleted creates an orphaned, semantically inconsistent record.
	var backCount int64
	tc.DB.Model(&models.GroupRelation{}).
		Where("relation_type_id = ?", relTypeT2.ID).
		Count(&backCount)
	assert.Equal(t, int64(0), backCount,
		"Back-relation (type T2) should also be deleted when EditRelationType "+
			"invalidates the corresponding forward relation — otherwise it becomes "+
			"an orphaned record with no matching forward partner")
}
