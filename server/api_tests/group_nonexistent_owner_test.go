package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestUpdateGroup_NonExistentOwner verifies that UpdateGroup rejects an OwnerId
// pointing to a group that does not exist in the database.
//
// BUG: The cycle detection walk in UpdateGroup (group_crud_context.go) starts
// by querying the proposed OwnerId. If that group doesn't exist, the query
// fails and the loop breaks with "ancestor not found, no cycle". The code
// then proceeds to set OwnerId to the non-existent ID without any further
// validation. This creates a dangling foreign key reference.
//
// The FK constraint *might* catch this in production (SQLite with PRAGMA
// foreign_keys = ON), but:
//   - The test harness doesn't enable FK constraints (no custom driver)
//   - UpdateGroup doesn't call EnsureForeignKeysActive unlike DeleteGroup
//   - The codebase documents that FK constraints can be unreliable inside
//     SQLite transactions (see comments in DeleteGroup)
//
// The cycle detection should explicitly verify that the proposed owner exists
// before accepting it, rather than relying on the FK constraint as a safety net.
func TestUpdateGroup_NonExistentOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"Name": "Test Group",
		"Meta": "{}",
	})
	require.Equal(t, http.StatusOK, resp.Code, "creating group should succeed")

	var group models.Group
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &group))
	require.NotZero(t, group.ID)

	// Pick an OwnerId that definitely doesn't exist
	nonExistentID := uint(999999)

	// Verify it really doesn't exist
	var count int64
	tc.DB.Model(&models.Group{}).Where("id = ?", nonExistentID).Count(&count)
	require.Zero(t, count, "sanity check: group 999999 should not exist")

	// Try to set the group's owner to the non-existent group
	updateResp := tc.MakeRequest(http.MethodPost, "/v1/group", map[string]interface{}{
		"ID":      group.ID,
		"Name":    "Test Group",
		"Meta":    "{}",
		"OwnerId": nonExistentID,
	})

	// The API should reject this with an error — you cannot set an owner
	// that doesn't exist. The cycle detection should not silently pass
	// when the proposed owner is not found in the database.
	assert.NotEqual(t, http.StatusOK, updateResp.Code,
		"UpdateGroup should reject OwnerId pointing to non-existent group %d, "+
			"but the cycle detection silently passes because 'ancestor not found' "+
			"is treated the same as 'no cycle detected'", nonExistentID)

	// Verify the group's owner was NOT set in the database
	var check models.Group
	tc.DB.First(&check, group.ID)
	assert.Nil(t, check.OwnerId,
		"Group's OwnerId should remain NULL after rejected update, "+
			"but it was set to non-existent group %d", nonExistentID)
}

// TestUpdateGroup_NonExistentOwner_ViaAppContext tests the same bug at the
// application context layer directly, bypassing the HTTP handler.
func TestUpdateGroup_NonExistentOwner_ViaAppContext(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group via direct DB insertion
	group := tc.CreateDummyGroup("Direct Test Group")
	require.NotZero(t, group.ID)

	nonExistentID := uint(888888)

	// Verify it doesn't exist
	var count int64
	tc.DB.Model(&models.Group{}).Where("id = ?", nonExistentID).Count(&count)
	require.Zero(t, count)

	// Call UpdateGroup directly with a non-existent OwnerId
	_, err := tc.AppCtx.UpdateGroup(&query_models.GroupEditor{
		ID: group.ID,
		GroupCreator: query_models.GroupCreator{
			Name:    "Direct Test Group",
			Meta:    "{}",
			OwnerId: nonExistentID,
		},
	})

	// UpdateGroup should return an error for a non-existent owner
	assert.Error(t, err,
		"UpdateGroup should return error when OwnerId %d does not exist, "+
			"but the cycle detection walk treats 'not found' as 'no cycle'", nonExistentID)

	// Verify the DB state wasn't corrupted
	var check models.Group
	tc.DB.First(&check, group.ID)
	assert.Nil(t, check.OwnerId,
		"Group's OwnerId should remain NULL — got dangling reference to %d", nonExistentID)
}
