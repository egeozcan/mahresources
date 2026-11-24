package application_context

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"mahresources/models"
	"mahresources/models/query_models"
)

// Final-review fixes for the table-backed readers: each test is one finding.

// rowsCapture records the SQL and the row count of every statement.
type rowsCapture struct {
	mu   sync.Mutex
	sqls []string
	rows []int64
}

func (c *rowsCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *rowsCapture) Info(context.Context, string, ...interface{})     {}
func (c *rowsCapture) Warn(context.Context, string, ...interface{})     {}
func (c *rowsCapture) Error(context.Context, string, ...interface{})    {}
func (c *rowsCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, n := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.rows = append(c.rows, n)
	c.mu.Unlock()
}

// ---- 1: the keeps-smaller comparison survives dimensions whose product overflows int64

func testStoreKeepsSmallerOnHugeDimensions(t *testing.T, ctx *MahresourcesContext) {
	huge := uint(4_000_000_000)
	kept := &models.Resource{Name: "kept", Hash: "h", Width: 10, Height: 10}
	a := &models.Resource{Name: "a", Hash: "h", Width: huge, Height: huge}
	b := &models.Resource{Name: "b", Hash: "h", Width: 20, Height: 20}
	for _, res := range []*models.Resource{kept, a, b} {
		require.NoError(t, ctx.db.Create(res).Error)
	}
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "k", Tier: "near", State: "open", WinnerID: kept.ID, KeepsSmaller: true}, Members: memberRows(kept.ID, b.ID, a.ID)}))

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	assert.Equal(t, a.ID, review.Clusters[0].LargestMemberID)
	assert.Equal(t, "10×10", review.Clusters[0].KeptDimensions)
}

func TestStoreKeepsSmallerOnHugeDimensionsSQLite(t *testing.T) {
	testStoreKeepsSmallerOnHugeDimensions(t, newReductionSQLiteContext(t))
}

// ---- 4: member hydration of small Clusters stays within 500 rows a statement -------

func testStoreSmallClusterHydrationIsChunked(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	next := uint(1)
	for i := 0; i < 100; i++ {
		ids := sequentialIDs(next, 12)
		next += 12
		require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: fmt.Sprintf("c%03d", i), Tier: "identical", State: "open", WinnerID: ids[0], Position: int64(i)}, Members: memberRows(ids...)}))
	}
	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	review, _, err := spy.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Sort: "computed", PageSize: 100})
	require.NoError(t, err)
	require.Len(t, review.Clusters, 100)
	for _, v := range review.Clusters {
		assert.Len(t, v.Members, 12, v.ID)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	sawMembers := false
	for i, sql := range capture.sqls {
		if strings.Contains(sql, "FROM `reduction_members`") || strings.Contains(sql, `FROM "reduction_members"`) {
			sawMembers = true
			assert.LessOrEqual(t, capture.rows[i], int64(500), sql)
		}
	}
	assert.True(t, sawMembers)
}

func TestStoreSmallClusterHydrationIsChunkedSQLite(t *testing.T) {
	testStoreSmallClusterHydrationIsChunked(t, newReductionSQLiteContext(t))
}

// ---- 5: a read-only Cluster does not scan its losers for the keeps-smaller figure ---

func testStoreReadOnlyKeepsSmallerDoesNotScan(t *testing.T, ctx *MahresourcesContext) {
	kept := &models.Resource{Name: "kept", Hash: "h", Width: 10, Height: 10}
	big := &models.Resource{Name: "big", Hash: "h", Width: 100, Height: 100}
	require.NoError(t, ctx.db.Create(kept).Error)
	require.NoError(t, ctx.db.Create(big).Error)
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "ro", Tier: "identical", State: "open", WinnerID: kept.ID, KeepsSmaller: true, ReadOnly: true}, Members: memberRows(kept.ID, big.ID)}))

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	review, _, err := spy.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	v := review.Clusters[0]
	assert.True(t, v.KeepsSmaller)
	assert.Zero(t, v.LargestMemberID, "no promote target on a read-only Cluster")
	capture.mu.Lock()
	defer capture.mu.Unlock()
	for _, sql := range capture.sqls {
		assert.NotContains(t, strings.ToLower(sql), "order by r.width", sql)
	}
}

func TestStoreReadOnlyKeepsSmallerDoesNotScanSQLite(t *testing.T) {
	testStoreReadOnlyKeepsSmallerDoesNotScan(t, newReductionSQLiteContext(t))
}

// ---- 7: the list sorts on the full name, not the bounded projection ----------------

func testStoreListSortsOnTheFullName(t *testing.T, ctx *MahresourcesContext) {
	prefix := strings.Repeat("€", 400) // 1,200 bytes, under a 1,000 character column
	first := newReductionRow(t, ctx.db) // older: the tie-break (newest first) would put it second
	second := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", first.ID).Update("name", prefix+"b").Error)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", second.ID).Update("name", prefix+"z").Error)

	rows, _, _, err := ctx.storeListReductions(0, 10, &query_models.ResourceReductionQuery{SortBy: []string{"name asc"}})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, first.ID, rows[0].ID, "b sorts before z past the 1,024th byte")
	rows, _, _, err = ctx.storeListReductions(0, 10, &query_models.ResourceReductionQuery{SortBy: []string{"name desc"}})
	require.NoError(t, err)
	assert.Equal(t, second.ID, rows[0].ID)
}

func TestStoreListSortsOnTheFullNameSQLite(t *testing.T) {
	testStoreListSortsOnTheFullName(t, newReductionSQLiteContext(t))
}
