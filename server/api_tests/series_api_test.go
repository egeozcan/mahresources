package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeriesListSortByMetaKey(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two series with different meta values
	tc.DB.Create(&models.Series{Name: "Zebra Series", Slug: "zebra", Meta: []byte(`{"priority":"2"}`)})
	tc.DB.Create(&models.Series{Name: "Alpha Series", Slug: "alpha", Meta: []byte(`{"priority":"1"}`)})

	// Sort by meta key — this should work on SQLite (converted to json_extract)
	resp := tc.MakeRequest(http.MethodGet, "/v1/seriesList?SortBy=meta->>'priority'", nil)
	assert.Equal(t, http.StatusOK, resp.Code,
		"Sorting series by meta->>'key' should not cause a SQL error on SQLite")

	var series []models.Series
	err := json.Unmarshal(resp.Body.Bytes(), &series)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(series), 2, "should return both series")
}
