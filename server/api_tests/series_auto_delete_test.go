package api_tests

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestEditResourceToNewSeriesAutoDeletesOldSeries(t *testing.T) {
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
		"resource should have moved to a different series")

	// Step 3: Verify the old series was auto-deleted (it had only this one resource)
	var count int64
	tc.DB.Model(&models.Series{}).Where("id = ?", oldSeriesID).Count(&count)
	assert.Equal(t, int64(0), count,
		fmt.Sprintf("old series (ID=%d) should be auto-deleted after its only resource moved to a new series", oldSeriesID))
}
