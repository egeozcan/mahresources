package application_context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/contracts"
	"mahresources/models"
)

// A plugin veto can carry a megabyte of reason; the database clips it at 1,024 bytes
// and the response must not hold more than that either (200 outcomes x 1 MiB).
func TestStoreApplyCollectorClipsReasonsAtARuneBoundarySQLite(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	collector := &applyCollector{ctx: ctx, result: &contracts.ReductionApplyResult{}}
	// 3-byte runes: byte 1,024 falls inside one, so a plain byte cut would split it.
	long := strings.Repeat("€", 2000)
	collector.addStale(contracts.ReductionApplyOutcome{ClusterID: "k", Reason: long})
	got := collector.result.Stale[0].Reason
	assert.LessOrEqual(t, len(got), 1024)
	assert.Greater(t, len(got), 1020)
	assert.True(t, strings.HasPrefix(long, got))
	assert.Equal(t, strings.ToValidUTF8(got, "?"), got, "the cut does not split a rune")

	collector.addStale(contracts.ReductionApplyOutcome{ClusterID: "short", Reason: "no"})
	assert.Equal(t, "no", collector.result.Stale[1].Reason)
}

// An outcome names its Winner only while the caller can still see it: the allow-list the
// request started with does not decide that after a merge moved the Winner out of scope.
func TestStoreApplyOutcomeNameFollowsCurrentScopeSQLite(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	mine := &models.Group{Name: "mine"}
	elsewhere := &models.Group{Name: "elsewhere"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(elsewhere).Error)
	winner := makeResource(t, ctx.db, "the-winner", opsRes{Hash: "h", Owner: &mine.ID})

	viewer := ctx.WithPrincipal(accountPrincipal(t, ctx, "viewer", models.RoleUser, &mine.ID))
	collector := &applyCollector{ctx: viewer, result: &contracts.ReductionApplyResult{}}
	collector.addApplied(contracts.ReductionApplyOutcome{ClusterID: "a", WinnerID: winner})
	assert.Equal(t, "the-winner", collector.result.Applied[0].WinnerName, "visible Winner is named")

	require.NoError(t, ctx.db.Exec("UPDATE resources SET owner_id = ? WHERE id = ?", elsewhere.ID, winner).Error)
	collector.addApplied(contracts.ReductionApplyOutcome{ClusterID: "b", WinnerID: winner})
	assert.Empty(t, collector.result.Applied[1].WinnerName, "a Winner now outside the subtree is not named")
}
