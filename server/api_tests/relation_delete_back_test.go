package api_tests

import (
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeleteRelationLeavesOrphanedBackRelation demonstrates that deleting
// a GroupRelation whose RelationType has a BackRelationId leaves the
// auto-created back-relation as an orphan in the database.
//
// Steps:
//  1. Create categories, groups, and a bidirectional relation type pair
//     ("parent" with back-relation "child").
//  2. Add a relation A->B of type "parent", which auto-creates B->A of
//     type "child".
//  3. Delete the A->B "parent" relation.
//  4. Verify that the B->A "child" back-relation is also deleted.
//
// Expected: both the forward and back relation are deleted.
// Actual (bug): only the forward relation is deleted; the back-relation
// remains in the database as an orphan.
func TestDeleteRelationLeavesOrphanedBackRelation(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category for both groups
	cat := &models.Category{Name: "BackRelTest Cat"}
	tc.DB.Create(cat)

	// Create two groups in that category
	groupA := &models.Group{Name: "Group A", CategoryId: &cat.ID}
	groupB := &models.Group{Name: "Group B", CategoryId: &cat.ID}
	tc.DB.Create(groupA)
	tc.DB.Create(groupB)

	// Create a bidirectional relation type pair: "parent" <-> "child"
	// First create the forward type
	parentType := &models.GroupRelationType{
		Name:           "parent-of",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(parentType)

	// Then create the back type, linking it to the forward type
	childType := &models.GroupRelationType{
		Name:           "child-of",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
		BackRelationId: &parentType.ID,
	}
	tc.DB.Create(childType)

	// Link the forward type back to the back type
	parentType.BackRelationId = &childType.ID
	tc.DB.Save(parentType)

	// Add a relation A->B of type "parent-of"
	// This should auto-create B->A of type "child-of"
	resp := tc.MakeRequest(http.MethodPost, "/v1/relation", map[string]any{
		"FromGroupId":         groupA.ID,
		"ToGroupId":           groupB.ID,
		"GroupRelationTypeId": parentType.ID,
	})
	require.Equal(t, http.StatusOK, resp.Code, "creating forward relation should succeed")

	// Verify both relations exist
	var forwardRel models.GroupRelation
	err := tc.DB.Where("from_group_id = ? AND to_group_id = ? AND relation_type_id = ?",
		groupA.ID, groupB.ID, parentType.ID).First(&forwardRel).Error
	require.NoError(t, err, "forward relation A->B should exist")

	var backRel models.GroupRelation
	err = tc.DB.Where("from_group_id = ? AND to_group_id = ? AND relation_type_id = ?",
		groupB.ID, groupA.ID, childType.ID).First(&backRel).Error
	require.NoError(t, err, "auto-created back-relation B->A should exist")

	// Count total relations before deletion
	var countBefore int64
	tc.DB.Model(&models.GroupRelation{}).Count(&countBefore)
	require.Equal(t, int64(2), countBefore, "should have exactly 2 relations before deletion")

	// Delete the forward relation A->B
	deleteURL := fmt.Sprintf("/v1/relation/delete?Id=%d", forwardRel.ID)
	deleteResp := tc.MakeRequest(http.MethodPost, deleteURL, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, "deleting forward relation should succeed")

	// Verify the forward relation is gone
	var checkForward models.GroupRelation
	err = tc.DB.First(&checkForward, forwardRel.ID).Error
	assert.Error(t, err, "forward relation should be deleted")

	// THE BUG: the back-relation B->A should also be deleted, but it isn't
	var countAfter int64
	tc.DB.Model(&models.GroupRelation{}).Count(&countAfter)
	assert.Equal(t, int64(0), countAfter,
		"Deleting a relation whose type has a BackRelationId should also delete the "+
			"auto-created back-relation; instead the back-relation is orphaned (count should be 0, got %d)", countAfter)
}
