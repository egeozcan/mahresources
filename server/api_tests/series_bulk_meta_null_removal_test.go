package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkAddMetaNullRemoval_RestoredBySeriesUpdate demonstrates that
// BulkAddMetaToResources with a null-valued key (which removes the key via
// json_patch) breaks the series Meta invariant for series-inherited keys.
//
// The invariant is: resource.Meta == merge(series.Meta, resource.OwnMeta)
//
// json_patch(target, '{"key": null}') removes "key" from target. When the
// target is resource.Meta, the key is removed correctly. But when the target
// is resource.OwnMeta and the key was INHERITED from the series (i.e. it
// doesn't exist in OwnMeta at all), json_patch('{}', '{"key":null}') is a
// no-op -- OwnMeta stays '{}'. The invariant breaks:
//
//	resource.Meta = {"year":2024}          (author removed)
//	merge(series, OwnMeta) = {"author":"alice","year":2024}  (author still inherited)
//
// The removed key silently reappears whenever the series meta is recomputed
// (UpdateSeries, DeleteSeries, or any operation that calls mergeMeta).
func TestBulkAddMetaNullRemoval_RestoredBySeriesUpdate(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err, "migrate Series/ResourceVersion")

	// Step 1: Create a series with two meta keys.
	series := &models.Series{
		Name: "NullRemovalSeries",
		Slug: "null-removal-series",
		Meta: types.JSON(`{"author":"alice","year":2024}`),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	// Step 2: Create a resource in the series. OwnMeta is empty because the
	// resource inherits everything from the series.
	resource := &models.Resource{
		Name:     "NullRemoval Resource",
		Hash:     "nullrm001",
		HashType: "SHA1",
		Location: "/test/nullrm.txt",
		Meta:     types.JSON(`{"author":"alice","year":2024}`),
		OwnMeta:  types.JSON(`{}`),
		SeriesID: &series.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Sanity check: invariant holds before the bulk operation.
	var pre models.Resource
	require.NoError(t, tc.DB.First(&pre, resource.ID).Error)
	var preMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(pre.Meta, &preMeta))
	require.Equal(t, "alice", preMeta["author"], "setup: resource should inherit author from series")

	// Step 3: Bulk-remove the "author" key using json_patch null semantics.
	// json_patch(meta, '{"author":null}') removes "author" from meta.
	err = tc.AppCtx.BulkAddMetaToResources(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{resource.ID}},
		Meta:      `{"author":null}`,
	})
	require.NoError(t, err, "BulkAddMetaToResources with null should succeed")

	// Step 4: Immediately after, Meta should no longer have "author".
	var afterBulk models.Resource
	require.NoError(t, tc.DB.First(&afterBulk, resource.ID).Error)
	var afterBulkMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterBulk.Meta, &afterBulkMeta))
	require.Nil(t, afterBulkMeta["author"],
		"immediate check: resource Meta should not have 'author' after null-removal")
	require.Equal(t, float64(2024), afterBulkMeta["year"],
		"immediate check: 'year' should still be present")

	// Step 5: Update the series meta to a different value. This triggers
	// recomputation: Meta = mergeMeta(series.Meta, OwnMeta).
	// Changing "year" from 2024 to 2025 forces metaChanged=true.
	_, err = tc.AppCtx.UpdateSeries(&query_models.SeriesEditor{
		ID:   series.ID,
		Name: "NullRemovalSeries",
		Meta: `{"author":"alice","year":2025}`,
	})
	require.NoError(t, err, "UpdateSeries should succeed")

	// Step 6: After recomputation, "author" should still be absent from Meta
	// because the user explicitly removed it via BulkAddMeta.
	var afterUpdate models.Resource
	require.NoError(t, tc.DB.First(&afterUpdate, resource.ID).Error)
	var afterUpdateMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterUpdate.Meta, &afterUpdateMeta))

	assert.Nil(t, afterUpdateMeta["author"],
		"BUG: series-inherited key 'author' reappeared after UpdateSeries even though "+
			"the user explicitly removed it via BulkAddMetaToResources with null. "+
			"json_patch on OwnMeta is a no-op when the key only exists in the series base, "+
			"so the removal is lost on recomputation.")
	assert.Equal(t, float64(2025), afterUpdateMeta["year"],
		"'year' should reflect the updated series value")
}

// TestBulkAddMetaNullRemoval_RestoredByDeleteSeries demonstrates the same
// invariant violation but through DeleteSeries: the series meta is merged
// back into the resource, resurrecting the key the user removed.
func TestBulkAddMetaNullRemoval_RestoredByDeleteSeries(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err, "migrate Series/ResourceVersion")

	series := &models.Series{
		Name: "NullDeleteSeries",
		Slug: "null-delete-series",
		Meta: types.JSON(`{"color":"red","size":"large"}`),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	resource := &models.Resource{
		Name:     "NullDelete Resource",
		Hash:     "nulldel001",
		HashType: "SHA1",
		Location: "/test/nulldel.txt",
		Meta:     types.JSON(`{"color":"red","size":"large"}`),
		OwnMeta:  types.JSON(`{}`),
		SeriesID: &series.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Remove "color" from this resource via bulk null-patch.
	err = tc.AppCtx.BulkAddMetaToResources(&query_models.BulkEditMetaQuery{
		BulkQuery: query_models.BulkQuery{ID: []uint{resource.ID}},
		Meta:      `{"color":null}`,
	})
	require.NoError(t, err)

	// Verify "color" is gone immediately.
	var afterBulk models.Resource
	require.NoError(t, tc.DB.First(&afterBulk, resource.ID).Error)
	var afterBulkMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterBulk.Meta, &afterBulkMeta))
	require.Nil(t, afterBulkMeta["color"], "setup: color should be removed from Meta")

	// Now delete the series. DeleteSeries merges series meta back into each
	// resource: Meta = mergeMeta(series.Meta, resource.OwnMeta).
	err = tc.AppCtx.DeleteSeries(series.ID)
	require.NoError(t, err, "DeleteSeries should succeed")

	// After the series is deleted, the resource should be detached and its
	// Meta should reflect the state the user intended: no "color" key.
	var afterDelete models.Resource
	require.NoError(t, tc.DB.First(&afterDelete, resource.ID).Error)
	var afterDeleteMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterDelete.Meta, &afterDeleteMeta))

	assert.Nil(t, afterDelete.SeriesID,
		"resource should be detached from series after DeleteSeries")
	assert.Nil(t, afterDeleteMeta["color"],
		"BUG: deleted-series key 'color' was resurrected into resource Meta by "+
			"DeleteSeries. The user had explicitly removed it via BulkAddMetaToResources "+
			"with null, but OwnMeta did not record the removal so mergeMeta brought it back.")
	assert.Equal(t, "large", afterDeleteMeta["size"],
		"'size' should be preserved after series deletion")
}
