package application_context

import (
	"testing"

	"mahresources/models"
)

// Bug 3: GetDataStats is missing the Series entity count.
// The EntityCounts struct should have a Series field, and GetDataStats should query it.

func TestGetDataStats_IncludesSeriesCount(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_series_count_test")

	// Create a series
	series := &models.Series{Name: "test-series", Slug: "test-series"}
	if err := ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	if stats.Entities.Series < 1 {
		t.Errorf("expected at least 1 series in entity counts, got %d", stats.Entities.Series)
	}
}

func TestGetDataStats_SeriesCountIsZeroWhenNoSeries(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_series_zero_test")

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	if stats.Entities.Series != 0 {
		t.Errorf("expected 0 series in entity counts when no series exist, got %d", stats.Entities.Series)
	}
}
