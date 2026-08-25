package api_tests

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
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

	// Across two resources the name is the only thing that distinguishes the two
	// panes; "Left" and "Right" restate the side the reader is already looking at.
	t.Run("cross-resource labels name the resources", func(t *testing.T) {
		reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res2.ID, latestV2)
		req := httptest.NewRequest("GET", reqURL, nil)
		ctx := provider(req)

		assert.Equal(t, "Resource One", ctx["label1"])
		assert.Equal(t, "Resource Two", ctx["label2"])
	})

	// The section is always rendered, so an unavailable merge has to say why
	// rather than removing itself from the page.
	t.Run("an unavailable merge carries a reason", func(t *testing.T) {
		sameResource := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res1.ID, latestV1)
		ctx := provider(httptest.NewRequest("GET", sameResource, nil))
		assert.Equal(t, false, ctx["canMerge"])
		assert.Contains(t, ctx["mergeBlockedReason"], "two different resources")

		crossResource := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res2.ID, latestV2)
		ctx = provider(httptest.NewRequest("GET", crossResource, nil))
		assert.Equal(t, true, ctx["canMerge"])
		assert.Equal(t, "", ctx["mergeBlockedReason"])
	})

	// The layout reserves a 400px sidebar for any page that does not opt out,
	// and this page has nothing to put in it.
	t.Run("the layout sidebar is suppressed", func(t *testing.T) {
		reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res2.ID, latestV2)
		ctx := provider(httptest.NewRequest("GET", reqURL, nil))
		assert.Equal(t, true, ctx["hideSidebar"])
	})

	// A constant title left every tab, bookmark and history entry saying the same
	// thing whatever was being compared.
	t.Run("the page title names what is being compared", func(t *testing.T) {
		cross := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res2.ID, latestV2)
		ctx := provider(httptest.NewRequest("GET", cross, nil))
		assert.Equal(t, "Resource One vs Resource Two", ctx["pageTitle"])

		same := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&r2=%d&v2=%d",
			res1.ID, latestV1, res1.ID, latestV1)
		ctx = provider(httptest.NewRequest("GET", same, nil))
		assert.Contains(t, ctx["pageTitle"], "Resource One")
	})

	t.Run("same-resource current vs old labels are Current/vN", func(t *testing.T) {
		// latestV1 is the current version; use v1=latestV1, v2=latestV1 for now
		// but we need an older version — version 1 should exist if latestV1 > 1,
		// otherwise both are the same version
		if latestV1 > 1 {
			reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=%d&v2=1", res1.ID, latestV1)
			req := httptest.NewRequest("GET", reqURL, nil)
			ctx := provider(req)

			assert.Equal(t, "Current", ctx["label1"])
			assert.Equal(t, "v1", ctx["label2"])
		}

		// Reverse: v1=1, v2=latestV1
		if latestV1 > 1 {
			reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=1&v2=%d", res1.ID, latestV1)
			req := httptest.NewRequest("GET", reqURL, nil)
			ctx := provider(req)

			assert.Equal(t, "v1", ctx["label1"])
			assert.Equal(t, "Current", ctx["label2"])
		}
	})

	t.Run("same-resource neither current labels are Newer/Older", func(t *testing.T) {
		// Create additional versions via DB to get 3+ versions on res1
		var lastVersionID uint
		for i := 2; i <= 4; i++ {
			v := models.ResourceVersion{
				ResourceID:    res1.ID,
				VersionNumber: i,
				Hash:          fmt.Sprintf("hash-v%d", i),
				HashType:      "SHA1",
				FileSize:      100,
				ContentType:   "text/plain",
				Location:      fmt.Sprintf("/fake/v%d", i),
				Comment:       fmt.Sprintf("version %d", i),
			}
			assert.NoError(t, tc.DB.Create(&v).Error)
			lastVersionID = v.ID
		}
		// Point CurrentVersionID to v4 so v1 and v2 are both non-current
		assert.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", res1.ID).
			Update("current_version_id", lastVersionID).Error)

		// Re-create provider to pick up the updated resource
		provider = template_context_providers.CompareContextProvider(tc.AppCtx)

		// Now comparing v1 vs v2 — neither is the current (v4 is current)
		reqURL := fmt.Sprintf("/resource/compare?r1=%d&v1=1&v2=2", res1.ID)
		req := httptest.NewRequest("GET", reqURL, nil)
		ctx := provider(req)

		assert.Equal(t, "Older", ctx["label1"])
		assert.Equal(t, "Newer", ctx["label2"])

		// Reverse: v2 vs v1
		reqURL2 := fmt.Sprintf("/resource/compare?r1=%d&v1=2&v2=1", res1.ID)
		req2 := httptest.NewRequest("GET", reqURL2, nil)
		ctx2 := provider(req2)

		assert.Equal(t, "Newer", ctx2["label1"])
		assert.Equal(t, "Older", ctx2["label2"])
	})
}
