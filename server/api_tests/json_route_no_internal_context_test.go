package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonRouteErrorDoesNotLeakAdminMenu(t *testing.T) {
	tc := SetupTestEnv(t)

	// Request a non-existent note via .json route to trigger an error response
	resp := tc.MakeRequest(http.MethodGet, "/note.json?id=99999", nil)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	// The response must NOT contain internal template context fields
	assert.NotContains(t, body, "adminMenu", "adminMenu should not leak in JSON response")
	assert.NotContains(t, body, "menu", "menu should not leak in JSON response")
	assert.NotContains(t, body, "assetVersion", "assetVersion should not leak in JSON response")
	assert.NotContains(t, body, "title", "title should not leak in JSON response")
	assert.NotContains(t, body, "queryValues", "queryValues should not leak in JSON response")
	assert.NotContains(t, body, "url", "url should not leak in JSON response")

	// The response SHOULD contain the error message
	assert.Contains(t, body, "errorMessage", "errorMessage should be present in error response")
}

func TestJsonRouteSuccessDoesNotLeakInternalFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag directly in DB
	tag := &models.Tag{Name: "test-tag-json-leak"}
	tc.DB.Create(tag)

	// Request it via .json route
	resp := tc.MakeRequest(http.MethodGet, "/tags.json", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	// The response must NOT contain internal template context fields
	assert.NotContains(t, body, "adminMenu", "adminMenu should not leak in JSON response")
	assert.NotContains(t, body, "menu", "menu should not leak in JSON response")
	assert.NotContains(t, body, "assetVersion", "assetVersion should not leak in JSON response")
	assert.NotContains(t, body, "queryValues", "queryValues should not leak in JSON response")
}

func TestJsonRouteDoesNotLeakPluginFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Request notes list via .json route
	resp := tc.MakeRequest(http.MethodGet, "/notes.json", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	// The response must NOT contain plugin-related internal fields
	assert.NotContains(t, body, "hasPluginManager", "hasPluginManager should not leak in JSON response")
	assert.NotContains(t, body, "pluginDetailActions", "pluginDetailActions should not leak in JSON response")
	assert.NotContains(t, body, "pluginCardActions", "pluginCardActions should not leak in JSON response")
	assert.NotContains(t, body, "pluginBulkActions", "pluginBulkActions should not leak in JSON response")
}
