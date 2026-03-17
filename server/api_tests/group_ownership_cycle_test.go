package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGroupOwnershipCycle_DirectCycleAllowed verifies that UpdateGroup allows
// creating a two-group ownership cycle (A owns B, then B set to own A).
// This is a bug: the application only rejects direct self-ownership (A owns A)
// but not indirect cycles (A->B->A), which corrupts the ownership hierarchy.
func TestGroupOwnershipCycle_DirectCycleAllowed(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create Group A
	respA := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name": "Group A",
		"Meta": "{}",
	})
	require.Equal(t, http.StatusOK, respA.Code, "creating Group A should succeed")

	var groupA models.Group
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &groupA))
	require.NotZero(t, groupA.ID, "Group A must have a valid ID")

	// Create Group B owned by Group A
	respB := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name":    "Group B",
		"Meta":    "{}",
		"OwnerId": groupA.ID,
	})
	require.Equal(t, http.StatusOK, respB.Code, "creating Group B (owned by A) should succeed")

	var groupB models.Group
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &groupB))
	require.NotZero(t, groupB.ID, "Group B must have a valid ID")

	// Verify B is owned by A
	var checkB models.Group
	tc.DB.First(&checkB, groupB.ID)
	require.NotNil(t, checkB.OwnerId)
	require.Equal(t, groupA.ID, *checkB.OwnerId, "Group B should be owned by Group A")

	// Now try to set Group A's owner to Group B — this creates a cycle: A->B->A
	respCycle := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"ID":      groupA.ID,
		"Name":    "Group A",
		"Meta":    "{}",
		"OwnerId": groupB.ID,
	})

	// BUG: The application should reject this with an error because it creates
	// an ownership cycle (A owns B, B owns A). Instead, it succeeds silently,
	// creating a corrupted hierarchy where FindParentsOfGroup enters an
	// infinite loop (bounded only by the arbitrary level < 20 limit).
	assert.NotEqual(t, http.StatusOK, respCycle.Code,
		"Setting Group A's owner to Group B should fail because B is already owned by A (cycle: A->B->A)")
}

// TestGroupOwnershipCycle_ThreeGroupChain verifies that a three-group cycle
// (A->B->C->A) is also detected and rejected.
func TestGroupOwnershipCycle_ThreeGroupChain(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create Group A
	respA := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name": "Group A",
		"Meta": "{}",
	})
	require.Equal(t, http.StatusOK, respA.Code)
	var groupA models.Group
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &groupA))

	// Create Group B owned by A
	respB := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name":    "Group B",
		"Meta":    "{}",
		"OwnerId": groupA.ID,
	})
	require.Equal(t, http.StatusOK, respB.Code)
	var groupB models.Group
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &groupB))

	// Create Group C owned by B
	respC := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name":    "Group C",
		"Meta":    "{}",
		"OwnerId": groupB.ID,
	})
	require.Equal(t, http.StatusOK, respC.Code)
	var groupC models.Group
	require.NoError(t, json.Unmarshal(respC.Body.Bytes(), &groupC))

	// Now try to set Group A's owner to Group C — this creates A->B->C->A
	respCycle := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"ID":      groupA.ID,
		"Name":    "Group A",
		"Meta":    "{}",
		"OwnerId": groupC.ID,
	})

	// BUG: Same as above but with a longer chain. The application should
	// walk up the proposed owner's ancestry and check for the target group.
	assert.NotEqual(t, http.StatusOK, respCycle.Code,
		"Setting Group A's owner to Group C should fail because C->B->A creates a cycle back to A")
}
