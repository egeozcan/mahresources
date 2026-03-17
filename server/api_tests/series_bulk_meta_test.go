package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkAddMetaLostOnSeriesUpdate demonstrates that BulkAddMetaToResources
// does not update OwnMeta for resources in a series. When the series meta is
// subsequently updated, the recomputed effective meta loses the bulk-added keys
// because OwnMeta still contains the old delta.
//
// Steps to reproduce:
//  1. Create a series with meta {"shared":"v1"}
//  2. Create a resource in the series with meta {"shared":"v1","own":"myval"}
//     -> OwnMeta should be {"own":"myval"}
//  3. BulkAddMeta {"bulkKey":"bulkVal"} to the resource
//     -> resource.Meta becomes {"shared":"v1","own":"myval","bulkKey":"bulkVal"}
//     -> BUT resource.OwnMeta is still {"own":"myval"} (not updated!)
//  4. Update series meta to {"shared":"v2"}
//     -> UpdateSeries recomputes: mergeMeta({"shared":"v2"}, {"own":"myval"})
//     -> resource.Meta becomes {"shared":"v2","own":"myval"}
//     -> The "bulkKey" is LOST
func TestBulkAddMetaLostOnSeriesUpdate(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// The test environment's AutoMigrate doesn't include Series and ResourceVersion.
	// Migrate them now.
	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err, "failed to migrate Series/ResourceVersion")

	// Step 1: Create a series with meta
	series := &models.Series{
		Name: "TestSeries",
		Slug: "test-series",
		Meta: types.JSON(`{"shared":"v1"}`),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	// Step 2: Create a resource assigned to the series
	// The resource's effective meta is {"shared":"v1","own":"myval"}
	// Its OwnMeta (the delta from series) should be {"own":"myval"}
	resource := &models.Resource{
		Name:     "Series Resource",
		Hash:     "abc123",
		HashType: "SHA1",
		Location: "/test/file.txt",
		Meta:     types.JSON(`{"shared":"v1","own":"myval"}`),
		OwnMeta:  types.JSON(`{"own":"myval"}`),
		SeriesID: &series.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Step 3: Bulk-add meta to the resource
	bulkQuery := &query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{resource.ID}},
		Meta:      `{"bulkKey":"bulkVal"}`,
	}
	err = tc.AppCtx.BulkAddMetaToResources(bulkQuery)
	require.NoError(t, err, "BulkAddMetaToResources should succeed")

	// Verify the resource's Meta was patched (this should work)
	var afterBulk models.Resource
	require.NoError(t, tc.DB.First(&afterBulk, resource.ID).Error)

	var afterBulkMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterBulk.Meta, &afterBulkMeta))
	assert.Equal(t, "bulkVal", afterBulkMeta["bulkKey"],
		"resource Meta should contain the bulk-added key immediately after BulkAddMeta")

	// Step 4: Update the series meta
	updatedSeries, err := tc.AppCtx.UpdateSeries(&query_models.SeriesEditor{
		ID:   series.ID,
		Name: "TestSeries",
		Meta: `{"shared":"v2"}`,
	})
	require.NoError(t, err, "UpdateSeries should succeed")
	_ = updatedSeries

	// Step 5: Verify the resource's Meta still contains the bulk-added key
	var afterSeriesUpdate models.Resource
	require.NoError(t, tc.DB.First(&afterSeriesUpdate, resource.ID).Error)

	var finalMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterSeriesUpdate.Meta, &finalMeta))

	// The series meta changed from "v1" to "v2", so "shared" should be "v2"
	assert.Equal(t, "v2", finalMeta["shared"],
		"resource should have the updated series value for 'shared'")

	// The resource's own key should still be present
	assert.Equal(t, "myval", finalMeta["own"],
		"resource should still have its own key 'own'")

	// BUG: The bulk-added key "bulkKey" should still be present, but it's LOST
	// because BulkAddMetaToResources patched Meta but not OwnMeta.
	// When UpdateSeries recomputes Meta = mergeMeta(newSeriesMeta, OwnMeta),
	// it uses the stale OwnMeta that doesn't know about "bulkKey".
	assert.Equal(t, "bulkVal", finalMeta["bulkKey"],
		"BUG: bulk-added meta key 'bulkKey' was lost after series meta update because "+
			"BulkAddMetaToResources does not update OwnMeta for resources in a series")
}

// TestBulkAddMetaViaAPILostOnSeriesUpdate demonstrates the same bug through
// the HTTP API layer, confirming it's a user-facing issue.
func TestBulkAddMetaViaAPILostOnSeriesUpdate(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err)

	// Create series and resource
	series := &models.Series{
		Name: "APISeries",
		Slug: "api-series",
		Meta: types.JSON(`{"color":"red"}`),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	resource := &models.Resource{
		Name:     "API Resource",
		Hash:     "def456",
		HashType: "SHA1",
		Location: "/test/api.txt",
		Meta:     types.JSON(`{"color":"red","size":"large"}`),
		OwnMeta:  types.JSON(`{"size":"large"}`),
		SeriesID: &series.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Bulk add meta via API
	resp := tc.MakeRequest(http.MethodPost, "/v1/resources/addMeta", map[string]any{
		"ID":   []uint{resource.ID},
		"Meta": `{"weight":"heavy"}`,
	})
	assert.Equal(t, http.StatusOK, resp.Code,
		"POST /v1/resources/addMeta should succeed")

	// Verify meta was added
	var check models.Resource
	require.NoError(t, tc.DB.First(&check, resource.ID).Error)
	var checkMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(check.Meta, &checkMeta))
	require.Equal(t, "heavy", checkMeta["weight"],
		"setup: bulk meta add should work immediately")

	// Now update the series meta
	_, err = tc.AppCtx.UpdateSeries(&query_models.SeriesEditor{
		ID:   series.ID,
		Name: "APISeries",
		Meta: `{"color":"blue"}`,
	})
	require.NoError(t, err)

	// Check if the bulk-added "weight" key survived the series update
	var finalResource models.Resource
	require.NoError(t, tc.DB.First(&finalResource, resource.ID).Error)

	var finalMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(finalResource.Meta, &finalMeta))

	assert.Equal(t, "blue", finalMeta["color"], "series value should be updated")
	assert.Equal(t, "large", finalMeta["size"], "own value should be preserved")
	assert.Equal(t, "heavy", finalMeta["weight"],
		"BUG: bulk-added meta key 'weight' was lost after series meta update")
}
