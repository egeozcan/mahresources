package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Bug 2: Bulk operations should return 400 (not 500) for validation errors
// ============================================================================

func TestBulkResourceAddTags_NotFoundReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Status Test Tag"}
	tc.DB.Create(tag)

	// Nonexistent resource IDs should NOT produce 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addTags",
		url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk addTags with nonexistent resource IDs should not return 500")
}

func TestBulkResourceRemoveTags_ValidationReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	// Missing IDs should NOT produce 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/removeTags",
		url.Values{})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk removeTags with no IDs should not return 500")
}

func TestBulkResourceReplaceTags_ValidationReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	// Missing IDs should NOT produce 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/replaceTags",
		url.Values{})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk replaceTags with no IDs should not return 500")
}

func TestBulkResourceAddGroups_NotFoundReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("Status Test Group")

	// Nonexistent resource IDs should NOT produce 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addGroups",
		url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(group.ID)}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk addGroups with nonexistent resource IDs should not return 500")
}

func TestBulkNoteAddTags_NotFoundReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Note Status Tag"}
	tc.DB.Create(tag)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addTags",
		url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk addTags with nonexistent note IDs should not return 500")
}

func TestBulkNoteRemoveTags_ValidationReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/removeTags",
		url.Values{})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk removeTags with no IDs should not return 500")
}

func TestBulkNoteAddGroups_NotFoundReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("Note Status Group")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addGroups",
		url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(group.ID)}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk addGroups with nonexistent note IDs should not return 500")
}

func TestBulkGroupAddTags_NotFoundReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Group Status Tag"}
	tc.DB.Create(tag)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/addTags",
		url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk addTags with nonexistent group IDs should not return 500")
}

func TestBulkGroupRemoveTags_ValidationReturns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/removeTags",
		url.Values{})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"bulk removeTags with no IDs should not return 500")
}

// ============================================================================
// Bug 9: Resource rotate should not return 500 for all errors
// ============================================================================

func TestRotateResource_BadRequest_Returns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	// Rotating a nonexistent resource should not return 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/rotate",
		url.Values{"ID": {"999999"}, "Degrees": {"90"}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"rotate with nonexistent resource should not return 500")
}

// ============================================================================
// Bug 10: setDimensions should not return 500 for missing parameters
// ============================================================================

func TestSetDimensions_BadRequest_Returns4xx(t *testing.T) {
	tc := SetupTestEnv(t)

	// Setting dimensions on a nonexistent resource should not return 500
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/setDimensions",
		url.Values{"ID": {"999999"}, "Width": {"100"}, "Height": {"200"}})
	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"setDimensions with nonexistent resource should not return 500")
}

// ============================================================================
// Bug 11: recalculateDimensions with no IDs should return proper JSON error
// ============================================================================

func TestRecalculateDimensions_NoIDs_ReturnsJSONError(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/recalculateDimensions",
		map[string]any{})

	// Should return 400 with a JSON body, not an empty 200
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"recalculateDimensions with no IDs should return 400")

	// Should have a JSON body
	var body map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "response body should be valid JSON")
}

// ============================================================================
// Bug 12: Merge-with-self should return consistent error messages
// ============================================================================

func TestMergeGroups_ZeroWinnerID_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// winnerId=0 should return 400, not 404 "record not found"
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/merge",
		url.Values{"Winner": {"0"}, "Losers": {"1"}})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"merge groups with winner ID 0 should return 400")
}

func TestMergeGroups_WinnerIsLoser_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := tc.CreateDummyGroup("Self Merge Group")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/merge",
		url.Values{"Winner": {fmt.Sprint(group.ID)}, "Losers": {fmt.Sprint(group.ID)}})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"merge group with itself should return 400")
}

func TestMergeTags_ZeroWinnerID_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tags/merge",
		url.Values{"Winner": {"0"}, "Losers": {"1"}})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"merge tags with winner ID 0 should return 400")
}
