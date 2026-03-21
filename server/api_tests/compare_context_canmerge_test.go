package api_tests

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models/query_models"
	template_context_providers "mahresources/server/template_handlers/template_context_providers"
)

func TestCompareContextProvider_CanMerge(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two distinct resources
	file1 := io.NopCloser(bytes.NewReader([]byte("resource-one-content")))
	res1, err := tc.AppCtx.AddResource(file1, "res1.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Resource One"},
	})
	assert.NoError(t, err)

	file2 := io.NopCloser(bytes.NewReader([]byte("resource-two-content")))
	res2, err := tc.AppCtx.AddResource(file2, "res2.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Resource Two"},
	})
	assert.NoError(t, err)

	// Fetch their latest version numbers
	versions1, err := tc.AppCtx.GetVersions(res1.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, versions1, "res1 should have at least one version")

	versions2, err := tc.AppCtx.GetVersions(res2.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, versions2, "res2 should have at least one version")

	latestV1 := versions1[0].VersionNumber
	latestV2 := versions2[0].VersionNumber

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)

	t.Run("canMerge is true for cross-resource comparison at latest versions", func(t *testing.T) {
		reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res2.ID, latestV2)
		req := httptest.NewRequest("GET", reqURL, nil)
		ctx := provider(req)

		canMerge, ok := ctx["canMerge"]
		assert.True(t, ok, "canMerge key should be present in context")
		assert.Equal(t, true, canMerge, "canMerge should be true when comparing different resources at their latest versions")
	})

	t.Run("canMerge is false for same-resource comparison", func(t *testing.T) {
		reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res1.ID, latestV1)
		req := httptest.NewRequest("GET", reqURL, nil)
		ctx := provider(req)

		canMerge, ok := ctx["canMerge"]
		assert.True(t, ok, "canMerge key should be present in context")
		assert.Equal(t, false, canMerge, "canMerge should be false when comparing same resource")
	})
}
