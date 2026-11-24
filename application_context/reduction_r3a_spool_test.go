package application_context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func TestMigrationBoundsAClusterScalarInTheSpoolItself(t *testing.T) {
	testMigrationBoundsAClusterScalarInTheSpool(t, newReductionSQLiteContext(t))
}

// A legacy Cluster of 200 members with a 1 MiB veto reason would spool 200 copies of
// the reason (the Cluster's scalars ride on every member row), and the member read
// would load 200 MiB before the stored value was cut. The bound is in the INSERT
// itself, so the spool never holds more than the bounded text per row.
func testMigrationBoundsAClusterScalarInTheSpool(t *testing.T, ctx *MahresourcesContext) {
	const members = 200
	first := seedResources(t, ctx, members, `'rs' || n`, `'sh'`, `NULL`)
	type m = map[string]any
	var ms []any
	for i := 0; i < members; i++ {
		ms = append(ms, m{"resourceId": first + uint(i), "hash": "sh", "inExtent": true})
	}
	reason := strings.Repeat("é", 512*1024) // 1 MiB of two-byte runes
	cluster := m{"id": "c-veto", "tier": "near", "winnerId": first, "state": "stale", "staleReason": reason, "members": ms}
	red := seedLegacyPlan(t, ctx, []any{cluster})

	var maxSpooled int64 = -1
	orig := migrationAfterResourceLock
	t.Cleanup(func() { migrationAfterResourceLock = orig })
	migrationAfterResourceLock = func([]uint) {
		var n int64
		_ = ctx.db.Raw(`SELECT COALESCE(MAX(LENGTH(stale_reason)), 0) FROM reduction_migration_rows`).Scan(&n).Error
		if n > maxSpooled {
			maxSpooled = n
		}
	}
	require.NoError(t, ctx.MigrateReductionPlans())

	require.GreaterOrEqual(t, maxSpooled, int64(0), "the spool was inspected while a Cluster was written")
	require.LessOrEqual(t, maxSpooled, int64(4*models.StaleReasonMaxBytes),
		"every spooled member row carried the whole veto reason")

	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ?", red.ID).First(&row).Error)
	require.LessOrEqual(t, len(row.StaleReason), models.StaleReasonMaxBytes)
	require.True(t, strings.HasPrefix(row.StaleReason, "é"), "the bounded reason is still the start of the original")
	require.Equal(t, members, row.MemberCount)
}
