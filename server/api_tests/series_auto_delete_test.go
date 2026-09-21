package api_tests

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestEditResourceToNewSeriesPreservesOldSeries(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a resource in "old-series" via upload with SeriesSlug
	fileContent := []byte("series-auto-delete-test-content")
	file := io.NopCloser(bytes.NewReader(fileContent))
	resource, err := tc.AppCtx.AddResource(file, "series-file.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:       "Series Resource",
			Meta:       `{"key":"value"}`,
			SeriesSlug: "old-series",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resource)
	assert.NotNil(t, resource.SeriesID, "resource should be in a series after upload with slug")

	oldSeriesID := *resource.SeriesID

	// Verify the old series exists
	var oldSeries models.Series
	err = tc.DB.First(&oldSeries, oldSeriesID).Error
	assert.NoError(t, err, "old series should exist")

	// Step 2: Edit the resource to move it to a brand-new series via slug
	edited, err := tc.AppCtx.EditResource(&query_models.ResourceEditor{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:       "Series Resource",
			Meta:       `{"key":"value"}`,
			SeriesSlug: "new-series",
		},
		ID: resource.ID,
	})
	assert.NoError(t, err)
	assert.NotNil(t, edited)
	assert.NotNil(t, edited.SeriesID, "resource should be in the new series")
	assert.NotEqual(t, oldSeriesID, *edited.SeriesID,
		"resource should have moved to a different series (old=%d, got=%d)", oldSeriesID, *edited.SeriesID)

	// Verify both the returned associations and persisted foreign key. GORM's
	// belongs-to callbacks write through non-nil pointers, so checking only the
	// in-memory SeriesID can hide a stale association replay during Save/Updates.
	var newSeries models.Series
	require.NoError(t, tc.DB.Where("slug = ?", "new-series").First(&newSeries).Error)
	require.NotNil(t, edited.Series)
	assert.Equal(t, newSeries.ID, edited.Series.ID)
	assert.Equal(t, newSeries.ID, *edited.SeriesID)

	var stored models.Resource
	require.NoError(t, tc.DB.First(&stored, resource.ID).Error)
	require.NotNil(t, stored.SeriesID)
	assert.Equal(t, newSeries.ID, *stored.SeriesID)
	assert.JSONEq(t, string(newSeries.Meta), string(stored.Meta))

	// Step 3: A move preserves its empty source. Deleting it inside the move
	// would race a reciprocal move that already resolved this Series as its
	// destination but is waiting for the canonical Series locks.
	var count int64
	tc.DB.Model(&models.Series{}).Where("id = ?", oldSeriesID).Count(&count)
	assert.Equal(t, int64(1), count,
		fmt.Sprintf("old series (ID=%d) should remain after its only resource moves", oldSeriesID))
}
