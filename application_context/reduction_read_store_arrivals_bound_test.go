package application_context

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// arrivalChunkStatements are the statements of the arrivals walk that page the
// resources table (the ones that carry the keyset order).
func arrivalChunkStatements(capture *rowsCapture) []string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	var out []string
	for _, sql := range capture.sqls {
		if strings.Contains(sql, "ORDER BY x.created_at, x.id") {
			out = append(out, sql)
		}
	}
	return out
}

func seedArrivals(t *testing.T, ctx *MahresourcesContext, n int) (*models.ResourceReduction, time.Time) {
	t.Helper()
	group, r := arrivalsGroupReduction(t, ctx)
	since := time.Now().Add(-time.Hour).UTC()
	stamp := since.Format("2006-01-02 15:04:05+00:00")
	for i := 0; i < n; i++ {
		require.NoError(t, ctx.db.Exec("INSERT INTO resources (name, hash, owner_id, created_at, updated_at) VALUES (?, 'h', ?, ?, ?)",
			fmt.Sprintf("a%d", i), group.ID, since.Add(time.Duration(i+1)*time.Second).Format("2006-01-02 15:04:05+00:00"), stamp).Error)
	}
	return r, since
}

// Each chunk after the first seeks straight past the cursor: the instant the walk
// started from is not a second range bound beside it, which let the planner range over
// created_at > since and discard every row before the cursor (quadratic in arrivals).
func TestStoreArrivalsChunksSeekFromTheCursorSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	require.NoError(t, ctx.db.Exec("CREATE INDEX IF NOT EXISTS idx_resources_created_at_id ON resources(created_at, id)").Error)
	reduction, since := seedArrivals(t, ctx, 23)

	old := reductionArrivalChunk
	reductionArrivalChunk = 5
	defer func() { reductionArrivalChunk = old }()
	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	n, _, err := spy.reductionStore().enteredSinceWithin(reduction, since, 1000)
	require.NoError(t, err)
	assert.Equal(t, 23, n)

	chunks := arrivalChunkStatements(capture)
	require.GreaterOrEqual(t, len(chunks), 5)
	plain := regexp.MustCompile(`x\.created_at > `)
	for i, sql := range chunks {
		if i == 0 {
			assert.Regexp(t, plain, sql, "the first chunk starts after the instant")
			continue
		}
		assert.NotRegexp(t, plain, sql, "chunk %d re-states the start instant beside the cursor: %s", i, sql)
		assert.Contains(t, sql, "(x.created_at, x.id) >", sql)
		var plan []struct {
			Detail string `gorm:"column:detail"`
		}
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+sql).Scan(&plan).Error)
		joined := ""
		for _, p := range plan {
			joined += p.Detail + "\n"
		}
		// Either created_at index serves the seek (the single-column one is covering,
		// since id is the rowid); what matters is a SEARCH bounded on created_at.
		assert.Regexp(t, `SEARCH x USING (COVERING )?INDEX idx_resources_created_at(_id)? \(created_at>`, joined, "chunk %d is an index seek: %s", i, joined)
		assert.NotContains(t, joined, "SCAN x", "chunk %d must not scan: %s", i, joined)
	}
}

// The walk is bounded: it examines at most the cap and says it stopped, so a library
// with a million new uploads does not rebuild the Extent for every chunk of them.
func TestStoreArrivalsWalkIsCappedSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	reduction, since := seedArrivals(t, ctx, 23)

	old := reductionArrivalChunk
	reductionArrivalChunk = 5
	defer func() { reductionArrivalChunk = old }()

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	n, capped, err := spy.reductionStore().enteredSinceWithin(reduction, since, 10)
	require.NoError(t, err)
	assert.True(t, capped)
	assert.Equal(t, 10, n, "a capped walk reports what it counted within the cap")
	assert.Len(t, arrivalChunkStatements(capture), 3, "two chunks of five reach the cap of ten, and a one-row probe finds an eleventh")

	n, capped, err = ctx.reductionStore().enteredSinceWithin(reduction, since, 23)
	require.NoError(t, err)
	assert.False(t, capped, "exactly the cap is not over it")
	assert.Equal(t, 23, n)
}
