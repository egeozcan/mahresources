package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeGroups_CreatesOwnershipCycle_IndirectOwnership verifies that merging
// a loser group into a winner that is indirectly owned by the loser (through
// an intermediate group) creates an ownership cycle.
//
// Setup:
//   - Group A (winner) is owned by Group C  (A.OwnerId = C)
//   - Group C is owned by Group B (loser)   (C.OwnerId = B)
//   - Group B has no owner                  (B.OwnerId = nil)
//
// Hierarchy: B -> C -> A  (B owns C, C owns A)
//
// When merging B into A, MergeGroups runs:
//
//	UPDATE groups SET owner_id = A WHERE owner_id IN (B)
//
// This changes C.OwnerId from B to A, creating: A -> C -> A (cycle!).
//
// The per-loser fix only checks if the winner was *directly* owned by a loser
// (winner.OwnerId == loser.ID), but here the winner is owned by C, not B.
// The indirect cycle through C goes undetected.
func TestMergeGroups_CreatesOwnershipCycle_IndirectOwnership(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create Group B (root, will be the loser)
	respB := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name": "Group B",
		"Meta": "{}",
	})
	require.Equal(t, http.StatusOK, respB.Code, "creating Group B")
	var groupB models.Group
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &groupB))

	// Create Group C owned by B
	respC := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name":    "Group C",
		"Meta":    "{}",
		"OwnerId": groupB.ID,
	})
	require.Equal(t, http.StatusOK, respC.Code, "creating Group C")
	var groupC models.Group
	require.NoError(t, json.Unmarshal(respC.Body.Bytes(), &groupC))

	// Create Group A (winner) owned by C
	respA := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name":    "Group A",
		"Meta":    "{}",
		"OwnerId": groupC.ID,
	})
	require.Equal(t, http.StatusOK, respA.Code, "creating Group A")
	var groupA models.Group
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &groupA))

	// Verify pre-merge hierarchy: B -> C -> A
	var checkA, checkC models.Group
	tc.DB.First(&checkC, groupC.ID)
	require.NotNil(t, checkC.OwnerId)
	require.Equal(t, groupB.ID, *checkC.OwnerId, "C should be owned by B before merge")

	tc.DB.First(&checkA, groupA.ID)
	require.NotNil(t, checkA.OwnerId)
	require.Equal(t, groupC.ID, *checkA.OwnerId, "A should be owned by C before merge")

	// Merge B (loser) into A (winner)
	respMerge := tc.MakeRequest(http.MethodPost, "/v1/groups/merge", map[string]interface{}{
		"Winner": groupA.ID,
		"Losers": []uint{groupB.ID},
	})
	require.Equal(t, http.StatusOK, respMerge.Code, "merge request should succeed (HTTP level)")

	// After merge, B is deleted. The critical question: does C.OwnerId
	// get set to A, creating a cycle A -> C -> A?
	//
	// Reload the surviving groups to check for cycles.
	var postMergeA, postMergeC models.Group
	tc.DB.First(&postMergeC, groupC.ID)
	tc.DB.First(&postMergeA, groupA.ID)

	// Check for the cycle: if C.OwnerId == A.ID and A.OwnerId == C.ID,
	// we have a cycle A -> C -> A.
	hasCycle := false
	if postMergeC.OwnerId != nil && *postMergeC.OwnerId == groupA.ID {
		if postMergeA.OwnerId != nil && *postMergeA.OwnerId == groupC.ID {
			hasCycle = true
		}
	}

	aOwner := uint(0)
	if postMergeA.OwnerId != nil {
		aOwner = *postMergeA.OwnerId
	}
	cOwner := uint(0)
	if postMergeC.OwnerId != nil {
		cOwner = *postMergeC.OwnerId
	}

	assert.False(t, hasCycle,
		"BUG: MergeGroups created an ownership cycle! "+
			"After merging B (id=%d) into A (id=%d), C.OwnerId was set to A (because C was owned by B), "+
			"but A.OwnerId is still C, creating cycle A -> C -> A. "+
			"MergeGroups should detect and break indirect ownership cycles when transferring children. "+
			"Post-merge state: A.OwnerId=%d, C.OwnerId=%d",
		groupB.ID, groupA.ID, aOwner, cOwner)

	// Additional verification: walk up the ownership chain from A.
	// If there's a cycle, we'll revisit A within a few hops.
	if !hasCycle {
		// Even if the simple 2-node check didn't catch it, verify
		// no cycle exists by walking the chain.
		visited := map[uint]bool{groupA.ID: true}
		current := postMergeA.OwnerId
		for current != nil {
			if visited[*current] {
				t.Errorf("BUG: Ownership cycle detected! Walking up from A (id=%d) revisited group %d. "+
					"MergeGroups must not create ownership cycles when transferring children from loser to winner.",
					groupA.ID, *current)
				break
			}
			visited[*current] = true
			var g models.Group
			if err := tc.DB.First(&g, *current).Error; err != nil {
				break
			}
			current = g.OwnerId
		}
	}
}
