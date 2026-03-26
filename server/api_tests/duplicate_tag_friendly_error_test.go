package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateTagCreationReturnsFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag via API (first time should succeed)
	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	assert.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	// Create the same tag again (should fail with friendly error)
	resp = tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	assert.True(t, resp.Code >= 400, "duplicate tag creation should fail, got %d", resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	errMsg, ok := body["error"].(string)
	require.True(t, ok, "error should be a string, got %v", body)

	// Error should NOT contain raw UNIQUE constraint message
	assert.NotContains(t, errMsg, "UNIQUE constraint failed",
		"raw DB constraint error should not leak to user")
	assert.NotContains(t, errMsg, "tags.name",
		"raw DB table/column name should not leak to user")

	// Error should contain a user-friendly message
	assert.Contains(t, errMsg, "already exists",
		"error message should explain that the tag already exists")
}

func TestDuplicateTagCreationViaFormReturnsFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create first tag via form
	formData := url.Values{"Name": {"form-dup-tag"}}
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	assert.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	// Create duplicate tag via form
	resp = tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	assert.True(t, resp.Code >= 400, "duplicate tag creation should fail, got %d", resp.Code)

	// Body should NOT contain raw constraint error
	bodyStr := resp.Body.String()
	assert.NotContains(t, bodyStr, "UNIQUE constraint failed",
		"raw DB constraint error should not leak to user")
}
