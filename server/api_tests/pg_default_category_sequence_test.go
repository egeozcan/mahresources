//go:build postgres

package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// TestPostgresSequenceAfterExplicitIDInsert verifies that after inserting a
// resource category with an explicit id = 1, the Postgres sequence is advanced
// so that the next auto-generated ID does not collide.
func TestPostgresSequenceAfterExplicitIDInsert(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// SetupPostgresTestEnv already created the default category at ID 1.
	// Verify it exists.
	var defaultCat models.ResourceCategory
	require.NoError(t, tc.DB.First(&defaultCat, 1).Error)
	assert.Equal(t, "Default", defaultCat.Name)

	// Now create a new category via the normal GORM path (auto-increment).
	// If the sequence wasn't advanced past 1, this will fail with a
	// duplicate key violation.
	newCat := &models.ResourceCategory{Name: "After Default", Description: "Should get ID > 1"}
	err := tc.DB.Create(newCat).Error
	require.NoError(t, err, "auto-increment insert should not collide with explicit id=1")
	assert.Greater(t, newCat.ID, uint(1), "new category should have ID > 1")

	// Create a second one to be sure the sequence keeps advancing
	newCat2 := &models.ResourceCategory{Name: "Third Category"}
	require.NoError(t, tc.DB.Create(newCat2).Error)
	assert.Greater(t, newCat2.ID, newCat.ID, "IDs should keep advancing")
}
