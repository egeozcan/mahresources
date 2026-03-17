package application_context

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"
)

// TestUpdateGroup_CategoryChange_LeavesInconsistentRelations demonstrates a bug
// where changing a group's category via UpdateGroup does NOT clean up existing
// GroupRelation records that require the old category.
//
// The AddRelation function validates that the from-group's category matches the
// relation type's FromCategoryId and the to-group's category matches the
// ToCategoryId. However, UpdateGroup allows changing a group's category without
// checking or cleaning up existing relations that were valid under the old
// category but are invalid under the new one.
//
// Compare with EditRelationType which DOES clean up GroupRelation rows when the
// relation type's categories change (relation_context.go lines 186-198).
//
// This leads to data inconsistency: the database contains relations that could
// never have been created under the current category assignments, and that
// violate the invariant enforced by AddRelation.
func TestUpdateGroup_CategoryChange_LeavesInconsistentRelations(t *testing.T) {
	ctx := createTestContext(t)

	// Step 1: Create two categories
	catX, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Category X"})
	if err != nil {
		t.Fatalf("Failed to create category X: %v", err)
	}
	catY, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "Category Y"})
	if err != nil {
		t.Fatalf("Failed to create category Y: %v", err)
	}

	// Step 2: Create a relation type that requires FromCategory=catX, ToCategory=catX
	relType, err := ctx.AddRelationType(&query_models.RelationshipTypeEditorQuery{
		Name:         "X-to-X Relation",
		FromCategory: catX.ID,
		ToCategory:   catX.ID,
	})
	if err != nil {
		t.Fatalf("Failed to create relation type: %v", err)
	}

	// Step 3: Create two groups both with category X
	groupA, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       "Group A",
		CategoryId: catX.ID,
	})
	if err != nil {
		t.Fatalf("Failed to create group A: %v", err)
	}
	groupB, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       "Group B",
		CategoryId: catX.ID,
	})
	if err != nil {
		t.Fatalf("Failed to create group B: %v", err)
	}

	// Step 4: Create a relation from A -> B using the relation type
	// This should succeed because both groups have category X
	relation, err := ctx.AddRelation(groupA.ID, groupB.ID, relType.ID, "test relation", "")
	if err != nil {
		t.Fatalf("Failed to create relation: %v", err)
	}

	// Verify the relation exists
	fetchedRelation, err := ctx.GetRelation(relation.ID)
	if err != nil {
		t.Fatalf("Failed to fetch relation: %v", err)
	}
	if fetchedRelation.ID == 0 {
		t.Fatal("Relation was not created")
	}

	// Step 5: Change group A's category from X to Y
	_, err = ctx.UpdateGroup(&query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name:       "Group A",
			CategoryId: catY.ID,
		},
		ID: groupA.ID,
	})
	if err != nil {
		t.Fatalf("Failed to update group A's category: %v", err)
	}

	// Verify group A now has category Y
	updatedA, err := ctx.GetGroup(groupA.ID)
	if err != nil {
		t.Fatalf("Failed to fetch updated group A: %v", err)
	}
	if updatedA.CategoryId == nil || *updatedA.CategoryId != catY.ID {
		t.Fatalf("Group A should have category Y (id=%d), got %v", catY.ID, updatedA.CategoryId)
	}

	// Step 6: The relation from A -> B should have been cleaned up because
	// group A no longer has category X (required by the relation type's FromCategoryId).
	// BUG: The relation still exists, creating an inconsistency.
	var remainingRelation models.GroupRelation
	err = ctx.db.First(&remainingRelation, relation.ID).Error
	if err == nil {
		// The relation still exists - this is the bug!
		// Verify it's truly inconsistent by checking categories
		var fromGroup models.Group
		ctx.db.First(&fromGroup, *remainingRelation.FromGroupId)
		var relationType models.GroupRelationType
		ctx.db.First(&relationType, *remainingRelation.RelationTypeId)

		fromCatID := uint(0)
		if fromGroup.CategoryId != nil {
			fromCatID = *fromGroup.CategoryId
		}
		if fromGroup.CategoryId == nil || *fromGroup.CategoryId != *relationType.FromCategoryId {
			t.Errorf("BUG: UpdateGroup left an inconsistent GroupRelation (id=%d): "+
				"relation type %q requires FromCategoryId=%d but group %q now has CategoryId=%d. "+
				"UpdateGroup should clean up GroupRelation records that become invalid when "+
				"a group's category changes, similar to how EditRelationType cleans up "+
				"relations when a relation type's categories change.",
				remainingRelation.ID,
				relationType.Name, *relationType.FromCategoryId,
				fromGroup.Name, fromCatID)
		}
	}

	// Step 7: Confirm that AddRelation correctly rejects new relations with the wrong category.
	// Create another group with category X to use as a target.
	groupC, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       "Group C",
		CategoryId: catX.ID,
	})
	if err != nil {
		t.Fatalf("Failed to create group C: %v", err)
	}

	// Try to create a new relation A -> C with the same type. This should fail
	// because A now has category Y, not X.
	_, err = ctx.AddRelation(groupA.ID, groupC.ID, relType.ID, "should fail", "")
	if err == nil {
		t.Error("Expected AddRelation to reject relation from group A (now category Y) " +
			"with relation type requiring FromCategory X, but it succeeded")
	}

	// Cleanup (shared in-memory DB)
	ctx.db.Where("from_group_id IN ? OR to_group_id IN ?",
		[]uint{groupA.ID, groupB.ID, groupC.ID},
		[]uint{groupA.ID, groupB.ID, groupC.ID}).Delete(&models.GroupRelation{})
	ctx.db.Delete(&models.GroupRelationType{}, relType.ID)
	ctx.db.Delete(&models.Group{}, []uint{groupA.ID, groupB.ID, groupC.ID})
	ctx.db.Delete(&models.Category{}, []uint{catX.ID, catY.ID})
}
