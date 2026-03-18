package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateSeriesRejectsWhitespaceOnlyName demonstrates that UpdateSeries
// does not validate the Name field the same way that series creation does.
//
// Root cause:
// buildSeries (used for creation) calls strings.TrimSpace(name) and rejects
// the result if it's empty. UpdateSeries simply checks `editor.Name != ""`
// and assigns it directly without trimming. This means:
//   - Create with " " -> trimmed to "" -> rejected with "series name must be non-empty"
//   - Update with " " -> not empty -> accepted, series name is now " "
//
// Impact:
// A series can have a whitespace-only name, which looks invisible in the UI,
// causes confusing search results, and violates the invariant established by
// the creation path.
func TestUpdateSeriesRejectsWhitespaceOnlyName(t *testing.T) {
	tc := SetupTestEnv(t)

	// Migrate Series table
	require.NoError(t, tc.DB.AutoMigrate(&models.Series{}))

	// Step 1: Create a series with a valid name
	series := &models.Series{
		Name: "My Valid Series",
		Slug: "my-valid-series",
		Meta: []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(series).Error)

	// Step 2: Verify creation rejects whitespace-only names
	_, seriesWriter := tc.AppCtx.SeriesCRUD()
	_, createErr := seriesWriter.Create(&query_models.SeriesCreator{Name: "   "})
	require.Error(t, createErr, "setup: creating a series with whitespace-only name should fail")
	assert.Contains(t, createErr.Error(), "non-empty",
		"setup: create should reject whitespace-only name with 'non-empty' error")

	// Step 3: Update the series name to whitespace-only
	updated, err := tc.AppCtx.UpdateSeries(&query_models.SeriesEditor{
		ID:   series.ID,
		Name: "   ",
	})

	// BUG: UpdateSeries should reject whitespace-only names (matching
	// the creation validation), but it accepts them silently.
	if err == nil {
		// If the update succeeded, verify the name is actually whitespace
		var check models.Series
		tc.DB.First(&check, series.ID)

		assert.Fail(t,
			"BUG: UpdateSeries accepted a whitespace-only name",
			"The series name is now %q (len=%d). UpdateSeries should reject "+
				"whitespace-only names the same way buildSeries does during creation. "+
				"Returned series name: %q", check.Name, len(check.Name), updated.Name)
	}
	// If err != nil, the test passes (UpdateSeries correctly rejected the name)
}
