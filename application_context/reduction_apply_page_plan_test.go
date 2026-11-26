package application_context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The apply loop pages the batch by (position, id). The page must be a seek on
// idx_rc_batch with no sort step: a sort reads the whole remaining suffix for every
// fifty Clusters.
func TestApplyPageSeeksIdxRcBatchWithoutASortSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+applyPageSQL, uint(1), "g", "b", int64(-1)<<62, uint(0), applyReadChunk).Scan(&plan).Error)
	var joined strings.Builder
	for _, p := range plan {
		joined.WriteString(p.Detail + "\n")
	}
	assert.Contains(t, joined.String(), "idx_rc_batch", "served by idx_rc_batch: %s", joined.String())
	assert.NotContains(t, joined.String(), "TEMP B-TREE", "no sort of the remaining suffix: %s", joined.String())
	assert.Regexp(t, `idx_rc_batch \(reduction_id=\? AND generation=\? AND apply_batch=\? AND \(position,id\)>\(\?,\?\)\)|idx_rc_batch \(reduction_id=\? AND generation=\? AND apply_batch=\? AND position>\?\)`, joined.String(), "the cursor bounds the range: %s", joined.String())
}
