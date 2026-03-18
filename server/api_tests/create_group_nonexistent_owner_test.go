package api_tests

import (
	"fmt"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCreateGroupRejectsNonExistentOwner verifies that CreateGroup returns an
// error when OwnerId references a group that does not exist.
//
// Bug: CreateGroup runs inside a SQLite transaction where PRAGMA foreign_keys
// is a no-op, so the FK constraint on owner_id never fires.  The function
// itself does not validate that the referenced owner group exists, which means
// a non-existent OwnerId is silently accepted, producing a dangling foreign
// key reference.  Subsequent GetGroup calls will return the group with a nil
// Owner even though OwnerId is set — an inconsistent state.
func TestCreateGroupRejectsNonExistentOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	// Try to create a group whose OwnerId points to a group that does not exist.
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", query_models.GroupCreator{
		Name:    "Orphan Child",
		OwnerId: 99999, // no group with this ID exists
	})

	// The request should fail because the owner does not exist.
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"CreateGroup should reject an OwnerId that references a non-existent group")
}

// TestCreateGroupNonExistentOwnerCausesInconsistentState demonstrates the
// downstream consequence of the missing validation: a group is persisted with
// OwnerId pointing to a non-existent group.  When re-fetched, OwnerId is set
// but Owner is nil — an impossible state under correct FK enforcement.
func TestCreateGroupNonExistentOwnerCausesInconsistentState(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group with a non-existent owner via the business-logic layer.
	group, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{
		Name:    "Dangling Owner",
		OwnerId: 99999,
	})

	// If CreateGroup correctly rejects the bad OwnerId, the bug is fixed.
	if err != nil {
		return
	}

	// Fetch the created group back with full preloading.
	fetched, fetchErr := tc.AppCtx.GetGroup(group.ID)
	assert.NoError(t, fetchErr)

	// The group was saved with OwnerId = 99999 ...
	assert.NotNil(t, fetched.OwnerId,
		"OwnerId should be set on the persisted group")
	assert.Equal(t, uint(99999), *fetched.OwnerId,
		"OwnerId should match the value that was passed")

	// ... but the Owner preload finds nothing because group 99999 does not exist.
	// This is the inconsistency: OwnerId is set yet Owner is nil.
	assert.NotNil(t, fetched.Owner,
		"Owner should not be nil when OwnerId is set — "+
			"a dangling foreign key was persisted because CreateGroup "+
			"does not validate the existence of the referenced owner group")

	// Also verify through the HTTP API that the dangling reference is visible.
	url := fmt.Sprintf("/v1/group?id=%d", group.ID)
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)
}
