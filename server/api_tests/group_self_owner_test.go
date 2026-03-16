package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestUpdateGroupRejectsSelfOwnership(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group
	createResp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupCreator{
		Name: "Self-Owner Candidate",
	})
	assert.Equal(t, http.StatusOK, createResp.Code)

	var group models.Group
	err := json.Unmarshal(createResp.Body.Bytes(), &group)
	assert.NoError(t, err)
	assert.NotZero(t, group.ID)

	// Try to set the group as its own owner — this should fail
	updateResp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupEditor{
		ID: group.ID,
		GroupCreator: query_models.GroupCreator{
			Name:    "Self-Owner Candidate",
			OwnerId: group.ID, // self-reference!
		},
	})

	// The API should reject self-ownership with an error
	assert.NotEqual(t, http.StatusOK, updateResp.Code,
		"UpdateGroup should reject setting a group as its own owner")

	// Verify the group still has no owner in the DB
	var check models.Group
	tc.DB.First(&check, group.ID)
	assert.Nil(t, check.OwnerId,
		"Group should not have itself as owner after rejected update")
}
