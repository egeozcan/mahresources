package api_tests

import (
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddRelationDuplicateReturnsErrorNotRedirect verifies that creating the
// exact same relation (same from_group, to_group, relation_type) twice returns
// a non-2xx error response, not a silent redirect (303 See Other) that masks
// the UNIQUE constraint failure.
//
// Bug: The AddRelation handler checks the Accept header (not Content-Type)
// to decide between a redirect and a JSON error. When a JSON-bodied request
// arrives without an explicit Accept header, the handler responds with
// 303 See Other (an HTML-form redirect), hiding the database error from API
// clients. Every other write handler in the codebase returns a proper error
// status via http_utils.HandleError.
func TestAddRelationDuplicateReturnsErrorNotRedirect(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "Dup Test Cat"}
	tc.DB.Create(cat)

	groupA := &models.Group{Name: "Dup From", CategoryId: &cat.ID}
	groupB := &models.Group{Name: "Dup To", CategoryId: &cat.ID}
	tc.DB.Create(groupA)
	tc.DB.Create(groupB)

	relType := &models.GroupRelationType{
		Name:           "dup-test-type",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	payload := map[string]any{
		"FromGroupId":         groupA.ID,
		"ToGroupId":           groupB.ID,
		"GroupRelationTypeId": relType.ID,
	}

	// First creation should succeed
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/relation", payload)
	require.Equal(t, http.StatusOK, resp1.Code, "first relation creation should succeed")

	// Second creation with the same (from, to, type) should return an error.
	// MakeRequest sends Content-Type: application/json (since body is non-nil)
	// which signals that this is a JSON API client.
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/relation", payload)

	// The response must NOT be 200 OK (duplicate silently accepted) or
	// 303 See Other (HTML redirect masking the error). It should be a
	// 4xx error indicating the relation already exists.
	assert.NotEqual(t, http.StatusOK, resp2.Code,
		"duplicate relation should not succeed silently")
	assert.NotEqual(t, http.StatusSeeOther, resp2.Code,
		"JSON API request should not receive an HTML redirect — "+
			"handler should use Content-Type (not Accept) to detect API clients, "+
			"or respond with JSON error for all non-browser requests")

	// Verify only one relation was actually created (no data corruption)
	var count int64
	tc.DB.Model(&models.GroupRelation{}).
		Where("from_group_id = ? AND to_group_id = ? AND relation_type_id = ?",
			groupA.ID, groupB.ID, relType.ID).
		Count(&count)
	assert.Equal(t, int64(1), count,
		"database should contain exactly one relation, not a duplicate")
}
