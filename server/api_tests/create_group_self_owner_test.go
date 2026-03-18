package api_tests

import (
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCreateGroupRejectsSelfOwnership verifies that CreateGroup rejects
// self-ownership the same way UpdateGroup does.
//
// In a fresh database the first group gets ID 1. Passing OwnerId=1 would
// produce a self-referencing ownership. CreateGroup must detect this after
// the insert (when the auto-assigned ID is known) and reject it.
func TestCreateGroupRejectsSelfOwnership(t *testing.T) {
	tc := SetupTestEnv(t)

	// In a fresh test DB the next auto-increment is 1.
	// Pass OwnerId=1 to attempt self-ownership.
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupCreator{
		Name:    "Self-Owner",
		OwnerId: 1,
	})

	// CreateGroup should reject self-ownership, matching UpdateGroup's behavior.
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"CreateGroup should reject self-ownership (OwnerId == auto-assigned ID) "+
			"just like UpdateGroup does with 'a group cannot be its own owner'")
}

// TestCreateGroupAcceptsValidOwner verifies that creating a group with
// a valid OwnerId (pointing to an existing different group) still works.
func TestCreateGroupAcceptsValidOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a parent group first
	parentResp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupCreator{
		Name: "Parent Group",
	})
	assert.Equal(t, http.StatusOK, parentResp.Code, "creating parent should succeed")

	// Create a child group owned by the parent
	childResp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupCreator{
		Name:    "Child Group",
		OwnerId: 1, // the parent's ID
	})
	assert.Equal(t, http.StatusOK, childResp.Code,
		"creating a group with a valid OwnerId should succeed")
}
