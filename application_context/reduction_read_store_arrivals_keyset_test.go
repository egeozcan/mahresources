package application_context

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/models"
)

// countArrivals runs enteredSince with a tiny chunk and fails the test, rather than
// hanging it, when the walk does not finish.
func countArrivals(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, since time.Time, chunk int) (int, error) {
	t.Helper()
	old := reductionArrivalChunk
	reductionArrivalChunk = chunk
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := ctx.reductionStore().enteredSince(r, since)
		done <- result{n, err}
	}()
	select {
	case res := <-done:
		reductionArrivalChunk = old
		return res.n, res.err
	case <-time.After(20 * time.Second):
		t.Fatal("the arrivals walk did not finish: a chunk did not advance")
		return 0, nil
	}
}

func arrivalsGroupReduction(t *testing.T, ctx *MahresourcesContext) (*models.Group, *models.ResourceReduction) {
	t.Helper()
	group := &models.Group{Name: "G"}
	require.NoError(t, ctx.db.Create(group).Error)
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: r.ID, GroupID: group.ID}).Error)
	return group, r
}

// SQLite stores a time as text, and compares text. A cursor scanned into a time
// and bound back is rewritten in UTC, so an arrival stored as 10:00+01:00 came back as
// 09:00+00:00, which sorts before the stored value: the same chunk forever.
func TestStoreArrivalsWalkSurvivesOffsetTimestampsSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	group, r := arrivalsGroupReduction(t, ctx)
	for i := 0; i < 5; i++ {
		res := models.Resource{Name: fmt.Sprintf("r%d", i), Hash: "h", OwnerId: &group.ID}
		require.NoError(t, ctx.db.Create(&res).Error)
		stored := fmt.Sprintf("2031-01-01 10:0%d:00+01:00", i)
		require.NoError(t, ctx.db.Exec("UPDATE resources SET created_at = ? WHERE id = ?", stored, res.ID).Error)
	}
	// The server opens SQLite with _loc=UTC, which is what rewrites a scanned time.
	utc, err := gorm.Open(sqlite.Open(ctx.db.Dialector.(*sqlite.Dialector).DSN+"&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	viaUTC := *ctx
	viaUTC.db = utc
	n, err := countArrivals(t, &viaUTC, r, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), 1)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
}

// Many arrivals share one created_at: a cursor on created_at alone would reprocess the
// tied rows; the id tiebreak must carry the walk across chunk boundaries.
func testStoreArrivalsWalkAcrossTies(t *testing.T, ctx *MahresourcesContext) {
	group, r := arrivalsGroupReduction(t, ctx)
	tied := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	rows := make([]models.Resource, 0, 7)
	for i := 0; i < 7; i++ {
		rows = append(rows, models.Resource{Name: "tie", Hash: "h", OwnerId: &group.ID, CreatedAt: tied})
	}
	require.NoError(t, ctx.db.CreateInBatches(&rows, 4).Error)
	n, err := countArrivals(t, ctx, r, tied.Add(-time.Hour), 2)
	require.NoError(t, err)
	assert.Equal(t, 7, n)
}

func TestStoreArrivalsWalkAcrossTiesSQLite(t *testing.T) {
	testStoreArrivalsWalkAcrossTies(t, newReductionSQLiteContext(t))
}

// The membership test of a chunk probes groups_related_resources per arrival, never
// scans it.
func testStoreArrivalMembershipIsCorrelated(t *testing.T, ctx *MahresourcesContext) {
	group, r := arrivalsGroupReduction(t, ctx)
	related := models.Resource{Name: "related", Hash: "h"}
	require.NoError(t, ctx.db.Create(&related).Error)
	require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", group.ID, related.ID).Error)

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	n, err := spy.reductionStore().enteredSince(r, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	capture.mu.Lock()
	sqls := append([]string{}, capture.sqls...)
	capture.mu.Unlock()
	checked := 0
	for _, sql := range sqls {
		if !strings.Contains(sql, "groups_related_resources") {
			continue
		}
		checked++
		assert.Contains(t, sql, "EXISTS", "membership is a correlated probe: %s", sql)
		assert.Contains(t, sql, "grr.resource_id = x.id", sql)
		if ctx.db.Dialector.Name() == "postgres" {
			var plan []string
			require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
					return err
				}
				return tx.Raw("EXPLAIN " + sql).Scan(&plan).Error
			}))
			for _, line := range plan {
				assert.NotContains(t, line, "Seq Scan on groups_related_resources", sql)
			}
		} else {
			var rows []struct {
				Detail string `gorm:"column:detail"`
			}
			require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+sql).Scan(&rows).Error)
			for _, row := range rows {
				assert.NotRegexp(t, `^SCAN (grr|groups_related_resources)`, row.Detail, "full scan of the relationship table: %s", sql)
			}
		}
	}
	assert.Greater(t, checked, 0, "no statement tested the relationship table")
}

func TestStoreArrivalMembershipIsCorrelatedSQLite(t *testing.T) {
	testStoreArrivalMembershipIsCorrelated(t, newReductionSQLiteContext(t))
}
