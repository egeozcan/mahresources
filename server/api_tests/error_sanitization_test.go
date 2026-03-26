package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelationTypeCreate_WithoutCategories_FriendlyError verifies that creating
// a relation type without FromCategory/ToCategory returns a user-friendly error
// message instead of leaking the raw "FOREIGN KEY constraint failed" from SQLite.
func TestRelationTypeCreate_WithoutCategories_FriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Try to create a relation type without categories (FromCategory=0, ToCategory=0)
	resp := tc.MakeRequest(http.MethodPost, "/v1/relationType", map[string]any{
		"Name": "Orphan Relation Type",
	})

	// Should fail — zero-value category IDs point to non-existent rows
	require.NotEqual(t, http.StatusOK, resp.Code,
		"creating a relation type without categories should fail")

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)
	errMsg := body["error"]

	assert.NotEmpty(t, errMsg, "error response should contain an error message")
	assert.False(t, strings.Contains(errMsg, "FOREIGN KEY constraint"),
		"error message should not leak raw DB constraint error; got: %s", errMsg)
	assert.True(t,
		strings.Contains(errMsg, "fromCategory") || strings.Contains(errMsg, "toCategory") ||
			strings.Contains(errMsg, "category") || strings.Contains(errMsg, "required"),
		"error message should mention categories or required fields; got: %s", errMsg)
}

// TestDuplicateCategory_FriendlyError verifies that creating a category with
// a duplicate name returns a friendly error instead of leaking "UNIQUE constraint failed".
func TestDuplicateCategory_FriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create the first category
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name": "Duplicate Cat",
	})
	require.Equal(t, http.StatusOK, resp1.Code, "first category creation should succeed")

	// Try to create a second category with the same name
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name": "Duplicate Cat",
	})
	require.NotEqual(t, http.StatusOK, resp2.Code,
		"duplicate category creation should fail")

	var body map[string]string
	json.Unmarshal(resp2.Body.Bytes(), &body)
	errMsg := body["error"]

	assert.NotEmpty(t, errMsg, "error response should contain an error message")
	assert.False(t, strings.Contains(errMsg, "UNIQUE constraint failed"),
		"error message should not leak raw DB constraint error; got: %s", errMsg)
	assert.True(t,
		strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate"),
		"error message should say entity already exists; got: %s", errMsg)
}

// TestDuplicateResourceCategory_FriendlyError verifies that creating a resource
// category with a duplicate name returns a friendly error.
func TestDuplicateResourceCategory_FriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create the first resource category
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{
		"Name": "Duplicate RC",
	})
	require.Equal(t, http.StatusOK, resp1.Code, "first resource category creation should succeed")

	// Try to create a second resource category with the same name
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{
		"Name": "Duplicate RC",
	})
	require.NotEqual(t, http.StatusOK, resp2.Code,
		"duplicate resource category creation should fail")

	var body map[string]string
	json.Unmarshal(resp2.Body.Bytes(), &body)
	errMsg := body["error"]

	assert.NotEmpty(t, errMsg, "error response should contain an error message")
	assert.False(t, strings.Contains(errMsg, "UNIQUE constraint failed"),
		"error message should not leak raw DB constraint error; got: %s", errMsg)
	assert.True(t,
		strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate"),
		"error message should say entity already exists; got: %s", errMsg)
}

// TestDuplicateQuery_FriendlyError verifies that creating a query with a
// duplicate name returns a friendly error.
func TestDuplicateQuery_FriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create the first query
	resp1 := tc.MakeRequest(http.MethodPost, "/v1/query", map[string]any{
		"Name": "Duplicate Query",
		"Text": "SELECT 1",
	})
	require.Equal(t, http.StatusOK, resp1.Code, "first query creation should succeed")

	// Try to create a second query with the same name
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/query", map[string]any{
		"Name": "Duplicate Query",
		"Text": "SELECT 2",
	})
	require.NotEqual(t, http.StatusOK, resp2.Code,
		"duplicate query creation should fail")

	var body map[string]string
	json.Unmarshal(resp2.Body.Bytes(), &body)
	errMsg := body["error"]

	assert.NotEmpty(t, errMsg, "error response should contain an error message")
	assert.False(t, strings.Contains(errMsg, "UNIQUE constraint failed"),
		"error message should not leak raw DB constraint error; got: %s", errMsg)
	assert.True(t,
		strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate"),
		"error message should say entity already exists; got: %s", errMsg)
}
