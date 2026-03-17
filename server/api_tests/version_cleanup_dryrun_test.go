package api_tests

import (
	"mahresources/models"
	"mahresources/models/query_models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupVersionsDryRunOverReportsWhenNoCurrentVersion demonstrates that
// CleanupVersions dry-run reports more deletable versions than can actually be
// deleted when a resource has no CurrentVersionID set.
//
// Root cause:
// CleanupVersions builds a query for versions eligible for deletion and, in
// dry-run mode, returns all matching IDs.  When CurrentVersionID is nil (e.g.
// for un-migrated resources) and KeepLast is 0, the query matches ALL versions.
// However, the actual deletion path calls DeleteVersion for each match, and
// DeleteVersion refuses to delete the last remaining version
// ("cannot delete last version — delete the resource instead").  The dry-run
// path does not apply this guard, so it reports N deletable versions while
// only N-1 can actually be deleted.
//
// Impact:
// API consumers that use dry-run counts to display "X versions will be cleaned
// up" show a higher number than what the subsequent non-dry-run call achieves.
// More importantly, callers that rely on dry-run == actual (e.g. to verify all
// old versions are gone) will see a silent discrepancy.
func TestCleanupVersionsDryRunOverReportsWhenNoCurrentVersion(t *testing.T) {
	tc := SetupTestEnv(t)

	// Migrate version tables
	err := tc.DB.AutoMigrate(&models.ResourceVersion{})
	require.NoError(t, err, "failed to migrate ResourceVersion")

	// Create a resource WITHOUT setting CurrentVersionID (simulates legacy/unmigrated resource)
	resource := &models.Resource{
		Name:     "Legacy Resource",
		Hash:     "aaa111",
		HashType: "SHA1",
		Location: "/test/legacy.txt",
		Meta:     []byte("{}"),
		// CurrentVersionID intentionally left nil
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Create 3 versions for this resource (no CurrentVersionID on the resource)
	for i := 1; i <= 3; i++ {
		v := &models.ResourceVersion{
			ResourceID:    resource.ID,
			VersionNumber: i,
			Hash:          "aaa111",
			HashType:      "SHA1",
			FileSize:      100,
			ContentType:   "text/plain",
			Location:      "/test/legacy.txt",
			Comment:       "test version",
		}
		require.NoError(t, tc.DB.Create(v).Error)
	}

	// Verify we have 3 versions
	var versionCount int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", resource.ID).Count(&versionCount)
	require.Equal(t, int64(3), versionCount, "setup: should have 3 versions")

	// Step 1: Dry-run cleanup with no KeepLast — should report how many will be deleted
	dryRunQuery := &query_models.VersionCleanupQuery{
		ResourceID: resource.ID,
		KeepLast:   0,
		DryRun:     true,
	}

	dryRunIDs, err := tc.AppCtx.CleanupVersions(dryRunQuery)
	require.NoError(t, err, "dry-run should not error")

	// Step 2: Actual cleanup with the same parameters
	actualQuery := &query_models.VersionCleanupQuery{
		ResourceID: resource.ID,
		KeepLast:   0,
		DryRun:     false,
	}

	actualIDs, err := tc.AppCtx.CleanupVersions(actualQuery)
	require.NoError(t, err, "actual cleanup should not error")

	// Step 3: The dry-run count and actual count SHOULD match.
	// BUG: dry-run reports 3 (all versions) but actual can only delete 2
	// because DeleteVersion refuses to delete the last remaining version.
	assert.Equal(t, len(actualIDs), len(dryRunIDs),
		"BUG: dry-run reported %d deletable versions but only %d were actually deleted; "+
			"CleanupVersions dry-run does not account for the DeleteVersion guard "+
			"that prevents deleting the last version when CurrentVersionID is nil",
		len(dryRunIDs), len(actualIDs))
}
