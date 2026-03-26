package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNonNumericIdReturnsUserFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Request a note with a non-numeric ID via .json route
	resp := tc.MakeRequest(http.MethodGet, "/note.json?id=abc", nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	errMsg, ok := body["errorMessage"].(string)
	require.True(t, ok, "errorMessage should be a string")

	// Error should NOT contain raw schema error
	assert.NotContains(t, errMsg, "schema: error converting",
		"raw schema error should not leak to user")

	// Error should contain a user-friendly message
	assert.Contains(t, errMsg, "invalid value",
		"error message should be user-friendly")
}

func TestNonNumericIdApiReturnsUserFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Request notes with a non-numeric OwnerId via API route
	resp := tc.MakeRequest(http.MethodGet, "/v1/notes?OwnerId=abc", nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	errMsg, ok := body["error"].(string)
	require.True(t, ok, "error should be a string")

	// Error should NOT contain raw schema error
	assert.NotContains(t, errMsg, "schema: error converting",
		"raw schema error should not leak to user")

	// Error should contain a user-friendly message
	assert.Contains(t, errMsg, "invalid value",
		"error message should be user-friendly")
}

func TestNonNumericFilterParamReturnsFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Request groups with a non-numeric tag filter via .json route
	resp := tc.MakeRequest(http.MethodGet, "/groups.json?Tags=abc", nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	errMsg, ok := body["errorMessage"].(string)
	require.True(t, ok, "errorMessage should be a string")

	// Error should NOT contain raw schema error
	assert.NotContains(t, errMsg, "schema: error converting",
		"raw schema error should not leak to user")
}
