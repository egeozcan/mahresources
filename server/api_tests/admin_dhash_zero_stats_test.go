//go:build json1 && fts5

package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// BH-037: GetExpensiveStats exposes a DhashZeroCount so the admin overview
// can render a drill-down link only when there are solid-colour resources
// polluting similarity matches.
func TestGetExpensiveStats_DhashZeroCount(t *testing.T) {
	tc := SetupTestEnv(t)

	// GetExpensiveStats fans out to multiple goroutines; pin the in-memory
	// SQLite to one connection so all goroutines see the same database.
	sqlDB, err := tc.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Two zero-DHash hashes (one int, one legacy string), one non-zero.
	res1 := &models.Resource{Name: "a.png"}
	res2 := &models.Resource{Name: "b.png"}
	res3 := &models.Resource{Name: "c.png"}
	require.NoError(t, tc.DB.Create(res1).Error)
	require.NoError(t, tc.DB.Create(res2).Error)
	require.NoError(t, tc.DB.Create(res3).Error)

	zero := int64(0)
	notZero := int64(42)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &res1.ID, DHashInt: &zero}).Error)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &res2.ID, DHash: "0000000000000000"}).Error)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &res3.ID, DHashInt: &notZero}).Error)

	stats, err := tc.AppCtx.GetExpensiveStats()
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.Similarity.DhashZeroCount,
		"DhashZeroCount should count both the int-zero row and the legacy hex-zero row")
	assert.Equal(t, int64(3), stats.Similarity.TotalHashes,
		"TotalHashes should include all three rows regardless of DHash value")
}
