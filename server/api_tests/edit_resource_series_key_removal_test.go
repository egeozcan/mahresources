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

// TestEditResource_RemoveSeriesKey_SurvivesSeriesUpdate demonstrates that
// when a resource in a series has its Meta edited to remove a key provided
// by the series, the removal is persisted as an explicit null override in
// OwnMeta so that a subsequent UpdateSeries call does NOT reintroduce the
// removed key.
//
// Steps:
//  1. Create Series with meta {"author":"alice","year":2024}
//  2. Create resource in the series (creator). OwnMeta={}, effective Meta=series meta.
//  3. Edit resource Meta to {"year":2024} — intentionally dropping "author".
//  4. Call UpdateSeries (changing series name only, meta unchanged).
//  5. Re-read the resource. Its Meta should still be {"year":2024} — NOT
//     {"author":"alice","year":2024}.
//
// BUG: computeOwnMeta only tracks keys present in the resource's meta.
// Keys absent from the resource but present in the series are not
// recorded as null overrides, so they silently reappear on the next
// mergeMeta recomputation triggered by UpdateSeries.
func TestEditResource_RemoveSeriesKey_SurvivesSeriesUpdate(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Migrate Series and ResourceVersion tables (not in default SetupTestEnv)
	err := tc.DB.AutoMigrate(&models.Series{}, &models.ResourceVersion{})
	require.NoError(t, err, "failed to migrate Series/ResourceVersion")

	// Step 1: Create series with two-key meta
	series := &models.Series{
		Name: "My Series",
		Slug: "my-series",
		Meta: types.JSON(`{"author":"alice","year":2024}`),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	// Step 2: Create resource in the series (simulates the series creator)
	// OwnMeta is empty — all meta is inherited from the series.
	resource := &models.Resource{
		Name:     "Episode 1",
		Hash:     "series-key-removal-test",
		HashType: "SHA1",
		Location: "/test/ep1.txt",
		Meta:     types.JSON(`{"author":"alice","year":2024}`),
		OwnMeta:  types.JSON(`{}`),
		SeriesID: &series.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Sanity: verify initial effective meta
	var before models.Resource
	require.NoError(t, tc.DB.First(&before, resource.ID).Error)
	var beforeMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(before.Meta, &beforeMeta))
	assert.Equal(t, "alice", beforeMeta["author"], "initial meta has author")
	assert.Equal(t, float64(2024), beforeMeta["year"], "initial meta has year")

	// Step 3: Edit the resource to remove "author" — keep only "year"
	_, editErr := tc.AppCtx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:     "Episode 1",
			Meta:     `{"year":2024}`,
			SeriesId: series.ID,
		},
		ID: resource.ID,
	})
	require.NoError(t, editErr, "EditResource should succeed")

	// Verify that right after the edit, Meta is correct
	var afterEdit models.Resource
	require.NoError(t, tc.DB.First(&afterEdit, resource.ID).Error)
	var editedMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterEdit.Meta, &editedMeta))
	assert.Nil(t, editedMeta["author"],
		"After edit, Meta should NOT contain 'author' (user removed it)")
	assert.Equal(t, float64(2024), editedMeta["year"],
		"After edit, Meta should still contain 'year'")

	// Step 4: Trigger a series meta recomputation by updating the series meta.
	// UpdateSeries recomputes effective Meta for all resources when meta changes
	// via mergeMeta(series.Meta, resource.OwnMeta).
	// We add a harmless new key to the series so meta changes and triggers recompute.
	resp := tc.MakeRequest(http.MethodPost, "/v1/series", map[string]interface{}{
		"ID":   series.ID,
		"Name": "My Series",
		"Meta": `{"author":"alice","year":2024,"publisher":"acme"}`,
	})
	require.Equal(t, http.StatusOK, resp.Code,
		"UpdateSeries should succeed; body: %s", resp.Body.String())

	// Step 5: Re-read the resource and verify the removed key stays removed.
	var afterSeriesUpdate models.Resource
	require.NoError(t, tc.DB.First(&afterSeriesUpdate, resource.ID).Error)

	var finalMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(afterSeriesUpdate.Meta, &finalMeta))

	assert.Equal(t, float64(2024), finalMeta["year"],
		"year should still be present")

	// The new series key "publisher" should be inherited (user never removed it).
	assert.Equal(t, "acme", finalMeta["publisher"],
		"publisher should be inherited from the updated series meta")

	// This is the key assertion: 'author' must NOT reappear.
	// If computeOwnMeta recorded a null override for 'author' in OwnMeta,
	// mergeMeta would delete the key and the assertion passes.
	// BUG: computeOwnMeta produces {} instead of {"author":null}, so
	// mergeMeta(series, {}) reintroduces "author" from the series.
	assert.Nil(t, finalMeta["author"],
		"BUG: 'author' was explicitly removed by the user but reappears "+
			"after UpdateSeries because computeOwnMeta does not record "+
			"null overrides for missing series keys")
}
