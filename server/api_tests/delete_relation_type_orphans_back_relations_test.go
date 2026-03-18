package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// TestDeleteRelationTypeOrphansBackRelationGroupRelations verifies that
// DeleteRelationshipType cleans up GroupRelation records belonging to the
// back-relation type when the forward type is deleted.
//
// Scenario:
//  1. Create a bidirectional relation type pair: "parent-of" <-> "child-of".
//  2. Use AddRelation to create A->B (parent-of), which auto-creates B->A (child-of).
//  3. Delete the "parent-of" relation TYPE.
//  4. GroupRelation rows of type "parent-of" are correctly deleted (line 331
//     of relation_context.go).
//  5. BUG: GroupRelation rows of type "child-of" (the back-relation type)
//     survive as orphans. The back-relation type "child-of" itself is
//     intentionally preserved (it becomes a standalone type with
//     BackRelationId=NULL), but its GroupRelation records no longer have a
//     matching forward partner and are semantically stale.
//
// Expected: all GroupRelation records of the back-relation type that were
// auto-created as mirrors of the deleted type's relations should be cleaned up.
// Actual: they survive, leaving orphaned back-relation records.
func TestDeleteRelationTypeOrphansBackRelationGroupRelations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category (same for both groups since it's a self-referential pair)
	cat := &models.Category{Name: "People"}
	require.NoError(t, tc.DB.Create(cat).Error)

	// Create two groups in the same category
	groupA := &models.Group{Name: "Alice", CategoryId: &cat.ID, Meta: []byte(`{}`)}
	groupB := &models.Group{Name: "Bob", CategoryId: &cat.ID, Meta: []byte(`{}`)}
	require.NoError(t, tc.DB.Create(groupA).Error)
	require.NoError(t, tc.DB.Create(groupB).Error)

	// Create a bidirectional relation type pair: "parent-of" <-> "child-of"
	parentType := &models.GroupRelationType{
		Name:           "parent-of",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	require.NoError(t, tc.DB.Create(parentType).Error)

	childType := &models.GroupRelationType{
		Name:           "child-of",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
		BackRelationId: &parentType.ID,
	}
	require.NoError(t, tc.DB.Create(childType).Error)

	parentType.BackRelationId = &childType.ID
	require.NoError(t, tc.DB.Save(parentType).Error)

	// Use AddRelation to create the forward relation + auto-created back-relation
	_, err := tc.AppCtx.AddRelation(groupA.ID, groupB.ID, parentType.ID, "test parent", "")
	require.NoError(t, err, "AddRelation should succeed")

	// Verify both group relations exist: forward (parent-of) and back (child-of)
	var forwardCount, backCount int64
	tc.DB.Model(&models.GroupRelation{}).Where("relation_type_id = ?", parentType.ID).Count(&forwardCount)
	tc.DB.Model(&models.GroupRelation{}).Where("relation_type_id = ?", childType.ID).Count(&backCount)
	require.Equal(t, int64(1), forwardCount, "forward relation should exist")
	require.Equal(t, int64(1), backCount, "auto-created back-relation should exist")

	// Delete the "parent-of" relation TYPE
	err = tc.AppCtx.DeleteRelationshipType(parentType.ID)
	require.NoError(t, err, "DeleteRelationshipType should succeed")

	// Verify the "parent-of" type is gone
	var typeCheck models.GroupRelationType
	assert.Error(t, tc.DB.First(&typeCheck, parentType.ID).Error, "parent-of type should be deleted")

	// Verify the "child-of" type still exists (correct behavior, becomes standalone)
	var childTypeCheck models.GroupRelationType
	require.NoError(t, tc.DB.First(&childTypeCheck, childType.ID).Error, "child-of type should survive")
	assert.Nil(t, childTypeCheck.BackRelationId, "child-of BackRelationId should be cleared to NULL")

	// Verify forward relations (parent-of) are cleaned up
	var forwardAfter int64
	tc.DB.Model(&models.GroupRelation{}).Where("relation_type_id = ?", parentType.ID).Count(&forwardAfter)
	assert.Equal(t, int64(0), forwardAfter, "forward relations of deleted type should be cleaned up")

	// BUG: back-relation GroupRelation records (child-of) should also be cleaned up,
	// because their matching forward partners are gone and they are now orphaned.
	// DeleteRelationshipType only deletes GroupRelations where relation_type_id
	// equals the deleted type's ID, but does NOT delete GroupRelations of the
	// back-relation type.
	var backAfter int64
	tc.DB.Model(&models.GroupRelation{}).Where("relation_type_id = ?", childType.ID).Count(&backAfter)
	assert.Equal(t, int64(0), backAfter,
		"GroupRelation records of the back-relation type (child-of) should be cleaned up "+
			"when the forward type (parent-of) is deleted, because they are orphaned mirrors "+
			"with no corresponding forward relation; instead they survive as stale data")
}
