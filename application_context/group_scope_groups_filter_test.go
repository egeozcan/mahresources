package application_context

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"
)

// TestGroupQuery_GroupsFilter_OwnedByOneGroupShouldNotMatchWhenFilteringByMultiple
// demonstrates a bug in GroupQuery's Groups filter: when filtering by multiple
// groups, a child group owned by only ONE of the filter groups is incorrectly
// included in the results.
//
// The Groups filter should require a group to be associated with ALL specified
// groups (same AND semantics as the resource and note Groups filters), but the
// current implementation uses OR between "related to ALL" and "owned by ANY",
// so ownership by any single filter group is enough to match.
//
// For example, filtering by Groups=[A, B] should only return groups associated
// with BOTH A and B. But a group owned by A (with no relation to B) is
// incorrectly included because the query uses:
//
//	(related_count = len(groups)) OR (owner_id IN groups)
//
// instead of the AND-based counting used by the resource and note scopes.
func TestGroupQuery_GroupsFilter_OwnedByOneGroupShouldNotMatchWhenFilteringByMultiple(t *testing.T) {
	ctx := createTestContext(t)

	// Create filter groups A and B
	groupA := &models.Group{Name: "Filter Group A"}
	groupB := &models.Group{Name: "Filter Group B"}
	ctx.db.Create(groupA)
	ctx.db.Create(groupB)

	// Create a child group owned by A but NOT related to or owned by B
	childOfA := &models.Group{Name: "Child of A Only", OwnerId: &groupA.ID}
	ctx.db.Create(childOfA)

	// Create a group related to both A and B (via group_related_groups)
	relatedToBoth := &models.Group{Name: "Related to Both"}
	ctx.db.Create(relatedToBoth)
	// Add relations: A -> relatedToBoth, B -> relatedToBoth
	ctx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)", groupA.ID, relatedToBoth.ID)
	ctx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)", groupB.ID, relatedToBoth.ID)

	// Query with Groups filter = [A, B]
	// Expected: only "Related to Both" should match (associated with BOTH A and B)
	// Bug: "Child of A Only" also matches because owner_id IN [A, B] triggers the OR branch
	query := &query_models.GroupQuery{
		Groups: []uint{groupA.ID, groupB.ID},
	}

	groups, err := ctx.GetGroups(0, 100, query)
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	count, err := ctx.GetGroupsCount(query)
	if err != nil {
		t.Fatalf("GetGroupsCount failed: %v", err)
	}

	// We expect exactly 1 result: "Related to Both"
	// The count should match the list length
	if int64(len(groups)) != count {
		t.Errorf("List/Count mismatch: GetGroups returned %d groups but GetGroupsCount returned %d",
			len(groups), count)
	}

	// Check that only the correctly matching group is returned
	for _, g := range groups {
		if g.Name == "Child of A Only" {
			t.Errorf("BUG: Group %q (id=%d) was returned by Groups filter [A, B], "+
				"but it is only owned by group A and has no association with group B. "+
				"The Groups filter should require association with ALL specified groups "+
				"(AND semantics), matching the behavior of resource and note Groups filters. "+
				"Instead, the OR between 'related to ALL' and 'owned by ANY' causes groups "+
				"owned by a single filter group to be incorrectly included.",
				g.Name, g.ID)
		}
	}

	if len(groups) != 1 {
		t.Errorf("Expected exactly 1 group matching Groups=[A, B] filter, got %d", len(groups))
		for _, g := range groups {
			t.Logf("  - %q (id=%d, owner_id=%v)", g.Name, g.ID, g.OwnerId)
		}
	}

	// Clean up: shared in-memory DB means these groups would pollute other tests
	ctx.db.Exec("DELETE FROM group_related_groups WHERE group_id IN ? OR related_group_id IN ?",
		[]uint{groupA.ID, groupB.ID}, []uint{groupA.ID, groupB.ID, relatedToBoth.ID, childOfA.ID})
	ctx.db.Delete(&models.Group{}, []uint{childOfA.ID, relatedToBoth.ID, groupB.ID, groupA.ID})
}
