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

// TestEditResourceMoveToSeriesUpdatesEffectiveMeta demonstrates that
// EditResource does not recompute the effective Meta when moving a resource
// to a different series via SeriesId.
//
// The design invariant (documented in docs/concepts/series.md) is:
//
//	resource.Meta == mergeMeta(series.Meta, resource.OwnMeta)
//
// Steps to reproduce:
//  1. Create Series A with meta {"author":"Alice"}
//  2. Create Series B with meta {"author":"Alice","lang":"en"}
//  3. Create a resource in Series A with effective meta {"author":"Alice","page":1}
//     OwnMeta = {"page":1}
//  4. EditResource to move the resource to Series B (via SeriesId)
//  5. After the edit, the resource's Meta should be:
//     mergeMeta(seriesB.Meta, computeOwnMeta(oldMeta, seriesB.Meta))
//     = mergeMeta({"author":"Alice","lang":"en"}, {"page":1})
//     = {"author":"Alice","lang":"en","page":1}
//
// BUG: resource.Meta stays {"author":"Alice","page":1} — it is missing
// the "lang":"en" key inherited from the new series.
func TestEditResourceMoveToSeriesUpdatesEffectiveMeta(t *testing.T) {
	tc := SetupTestEnv(t)

	// Migrate Series and ResourceVersion tables (not in default SetupTestEnv)
	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err, "failed to migrate Series/ResourceVersion")

	// Step 1: Create Series A
	seriesA := &models.Series{
		Name: "Series A",
		Slug: "series-a",
		Meta: types.JSON(`{"author":"Alice"}`),
	}
	require.NoError(t, tc.DB.Create(seriesA).Error)

	// Step 2: Create Series B with an extra key
	seriesB := &models.Series{
		Name: "Series B",
		Slug: "series-b",
		Meta: types.JSON(`{"author":"Alice","lang":"en"}`),
	}
	require.NoError(t, tc.DB.Create(seriesB).Error)

	// Step 3: Create a resource in Series A
	// Effective meta: {"author":"Alice","page":1}
	// OwnMeta (delta from Series A): {"page":1}
	resource := &models.Resource{
		Name:     "Test Resource",
		Hash:     "move-series-test-hash",
		HashType: "SHA1",
		Location: "/test/move-series.txt",
		Meta:     types.JSON(`{"author":"Alice","page":1}`),
		OwnMeta:  types.JSON(`{"page":1}`),
		SeriesID: &seriesA.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Sanity check: verify the resource is in Series A
	var beforeEdit models.Resource
	require.NoError(t, tc.DB.First(&beforeEdit, resource.ID).Error)
	require.NotNil(t, beforeEdit.SeriesID)
	assert.Equal(t, seriesA.ID, *beforeEdit.SeriesID)

	// Step 4: Edit the resource to move it to Series B
	edited, err := tc.AppCtx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:     "Test Resource",
			SeriesId: seriesB.ID,
		},
		ID: resource.ID,
	})
	require.NoError(t, err, "EditResource should succeed")
	require.NotNil(t, edited)

	// Verify the resource moved to Series B
	require.NotNil(t, edited.SeriesID, "resource should still be in a series")
	assert.Equal(t, seriesB.ID, *edited.SeriesID,
		"resource should now be in Series B")

	// Step 5: Verify effective Meta includes keys from the new series
	// Re-read from DB to get the persisted value
	var afterEdit models.Resource
	require.NoError(t, tc.DB.First(&afterEdit, resource.ID).Error)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterEdit.Meta, &meta),
		"resource Meta should be valid JSON")

	// OwnMeta should be {"page":1} (page is unique to the resource)
	var ownMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterEdit.OwnMeta, &ownMeta),
		"resource OwnMeta should be valid JSON")
	assert.Equal(t, float64(1), ownMeta["page"],
		"OwnMeta should contain the resource-specific 'page' key")

	// The effective Meta should be the merge of Series B's meta and OwnMeta.
	// Expected: {"author":"Alice","lang":"en","page":1}
	assert.Equal(t, "Alice", meta["author"],
		"effective Meta should have 'author' from the new series")
	assert.Equal(t, float64(1), meta["page"],
		"effective Meta should have 'page' from the resource's OwnMeta")

	// BUG: This assertion fails because EditResource does not recompute
	// resource.Meta = mergeMeta(newSeries.Meta, ownMeta) when moving
	// to a new series. The resource.Meta still has the old effective value
	// {"author":"Alice","page":1} and is missing "lang":"en" from Series B.
	assert.Equal(t, "en", meta["lang"],
		"BUG: effective Meta should include 'lang' inherited from the new series, "+
			"but EditResource does not recompute Meta when changing series")
}
