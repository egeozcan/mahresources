package application_context

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// With a million Resources selected explicitly, each arrivals membership test must
// probe reduction_extent_resources by (reduction_id, resource_id) for the arrival, not
// materialise the whole explicit selection per chunk.
func TestStoreArrivalsMembershipProbesTheExplicitSelectionSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	reduction, since := seedArrivals(t, ctx, 6)
	picked := makeResource(t, ctx.db, "picked", opsRes{Hash: "h"})
	require.NoError(t, ctx.db.Create(&models.ReductionExtentResource{ReductionID: reduction.ID, ResourceID: picked}).Error)

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	_, _, err := spy.reductionStore().enteredSinceWithin(reduction, since, 1000)
	require.NoError(t, err)

	var membership string
	capture.mu.Lock()
	for _, sql := range capture.sqls {
		if strings.Contains(sql, "ext_tree") && strings.Contains(sql, "SELECT COUNT(*) FROM resources x") {
			membership = sql
			break
		}
	}
	capture.mu.Unlock()
	require.NotEmpty(t, membership, "the walk ran a membership statement")

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+membership).Scan(&plan).Error)
	joined := ""
	for _, p := range plan {
		joined += p.Detail + "\n"
	}
	assert.NotRegexp(t, `reduction_extent_resources USING (COVERING )?INDEX \w+ \(reduction_id=\?\)\n`, joined, "the explicit selection is not walked as a whole: %s", joined)
	assert.Regexp(t, `SEARCH \w+ USING (COVERING )?INDEX \w+ \(reduction_id=\? AND resource_id=\?\)`, joined, "a correlated probe on (reduction_id, resource_id): %s", joined)
}
