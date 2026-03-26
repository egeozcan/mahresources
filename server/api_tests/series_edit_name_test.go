package api_tests

import (
	"mahresources/models"
	"mahresources/models/types"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeriesEditNameEndpoint verifies that the inline name edit endpoint
// for series works correctly. Previously, the /v1/series/editName route
// was missing, causing the inline-edit UI component to fail with a 404.
func TestSeriesEditNameEndpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	require.NoError(t, tc.DB.AutoMigrate(&models.Series{}))

	series := &models.Series{
		Name: "Original Name",
		Slug: "original-name",
		Meta: types.JSON("{}"),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	formData := url.Values{}
	formData.Set("name", "Updated Name")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/series/editName?id="+
		url.QueryEscape(string(rune('0'+series.ID))), formData)

	// The handler returns 200 with empty body (or redirect for HTML clients)
	assert.Equal(t, http.StatusOK, resp.Code,
		"POST /v1/series/editName should succeed, got: %s", resp.Body.String())

	// Verify the name was actually updated
	var check models.Series
	require.NoError(t, tc.DB.First(&check, series.ID).Error)
	assert.Equal(t, "Updated Name", check.Name, "series name should be updated")
}

// TestSeriesEditNameRejectsEmpty verifies that the editName endpoint
// rejects empty names.
func TestSeriesEditNameRejectsEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	require.NoError(t, tc.DB.AutoMigrate(&models.Series{}))

	series := &models.Series{
		Name: "Non Empty",
		Slug: "non-empty",
		Meta: types.JSON("{}"),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	formData := url.Values{}
	formData.Set("name", "")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/series/editName?id=1", formData)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"POST /v1/series/editName with empty name should fail")
}

// TestSeriesEditNameNotFound verifies that editing a non-existent series
// returns an appropriate error.
func TestSeriesEditNameNotFound(t *testing.T) {
	tc := SetupTestEnv(t)

	require.NoError(t, tc.DB.AutoMigrate(&models.Series{}))

	formData := url.Values{}
	formData.Set("name", "New Name")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/series/editName?id=999", formData)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"POST /v1/series/editName for non-existent series should fail")
}
