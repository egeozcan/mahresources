package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// The sweep must examine work proportional to the STALE rows, not to the live ones.
// Its old shape asked a correlated ownership question of every row of every
// generation it discovered, and every live row answered "owned", so a Reduction
// with 100k live members cost the sweep a scan of all of them (1.9M VM steps on
// SQLite) before it found nothing to delete. Ownership is a property of the
// (reduction, generation) pair, so it is asked once per pair and a protected pair's
// rows are never read at all. The statements the sweep sends are recorded: none may
// read a row of the live generation, and none may carry the per-row ownership
// subquery.
func testStoreSweepExaminesOnlyStale(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	for i := 0; i < 40; i++ {
		key := fmt.Sprintf("live%03d", i)
		seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
			opsMember(makeResource(t, ctx.db, key+"-a", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, key+"-b", opsRes{Hash: "h"}), "h"))
	}
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("stale%d", i)
		seedAt(t, ctx, r, "stale", models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
			opsMember(makeResource(t, ctx.db, key+"-a", opsRes{Hash: "h"}), "h"), opsMember(makeResource(t, ctx.db, key+"-b", opsRes{Hash: "h"}), "h"))
	}

	type stmt struct {
		sql  string
		vars []any
	}
	var mu sync.Mutex
	var seen []stmt
	record := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "reduction_members") || strings.Contains(sql, "reduction_clusters") {
			mu.Lock()
			seen = append(seen, stmt{sql, append([]any(nil), tx.Statement.Vars...)})
			mu.Unlock()
		}
	}
	const name = "test:record_sweep_sql"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, record))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name, record))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name); _ = ctx.db.Callback().Row().Remove(name) })

	require.NoError(t, ctx.sweepReductionGenerations())

	mu.Lock()
	swept := append([]stmt(nil), seen...)
	mu.Unlock()
	require.EqualValues(t, 0, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = 'stale'", r.ID))
	require.EqualValues(t, 40, planRows(t, ctx, "reduction_clusters", "reduction_id = ? AND generation = ?", r.ID, opsGen))
	require.NotEmpty(t, swept)
	for _, s := range swept {
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s.sql)), "SELECT") {
			continue
		}
		require.NotContains(t, s.sql, "resource_reductions", "a per-row ownership subquery: %s", s.sql)
		reads := false
		for _, v := range s.vars {
			reads = reads || fmt.Sprint(v) == opsGen
		}
		// discovery seeks PAST a generation (one row, LIMIT 1); nothing may read the live generation's rows
		if reads {
			require.True(t, strings.HasSuffix(strings.TrimSpace(s.sql), "LIMIT 1"), "a statement reads the live generation's rows: %s", s.sql)
		}
	}
}

func TestStoreSweepExaminesOnlyStale(t *testing.T) {
	testStoreSweepExaminesOnlyStale(t, withSettings(newReductionSQLiteContext(t)))
}
