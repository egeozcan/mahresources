package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeriesCreateViaFormData verifies that series creation works with
// form-encoded data, not just JSON. Previously, the generic CreateHandler
// passed a pointer-to-pointer to gorilla/schema which failed.
func TestSeriesCreateViaFormData(t *testing.T) {
	tc := SetupTestEnv(t)

	formData := url.Values{}
	formData.Set("Name", "Form Created Series")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/series/create", formData)
	require.Equal(t, http.StatusOK, resp.Code, "form-encoded series creation should succeed, got: %s", resp.Body.String())

	var series models.Series
	err := json.Unmarshal(resp.Body.Bytes(), &series)
	require.NoError(t, err)

	assert.Equal(t, "Form Created Series", series.Name)
	assert.Greater(t, series.ID, uint(0))
}
