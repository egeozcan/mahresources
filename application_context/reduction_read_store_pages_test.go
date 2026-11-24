package application_context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// A page of a Cluster's members seeks on (cluster_id, position, id): the cursor skips
// every earlier member instead of reading the prefix again, so walking a 200-member
// Cluster page by page is linear, not quadratic.
func testStoreMemberPageSeeksOnTheWholeKey(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	ids := sequentialIDs(1, 30)
	members := memberRows(ids...)
	for i := range members {
		members[i].Position = i / 3 // ties on position, broken by id
	}
	require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "k", Tier: "identical", State: "open", WinnerID: ids[0]}, Members: members}))
	var cluster models.ReductionClusterRow
	require.NoError(t, ctx.db.Where("reduction_id = ?", r.ID).First(&cluster).Error)

	// Walking by 4 returns every member once, in order, across tied positions.
	var seen []uint
	after := startOfMembers()
	for {
		rows, err := store.memberPage(cluster.ID, after, 4)
		require.NoError(t, err)
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			seen = append(seen, row.ResourceID)
		}
		last := rows[len(rows)-1]
		after = reductionMemberCursor{Position: last.Position, ID: last.ID}
	}
	assert.Equal(t, ids, seen)

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	_, err := spy.reductionStore().memberPage(cluster.ID, reductionMemberCursor{Position: 5, ID: 17}, 5)
	require.NoError(t, err)
	capture.mu.Lock()
	sql := capture.sqls[len(capture.sqls)-1]
	capture.mu.Unlock()

	var plan []string
	if ctx.db.Dialector.Name() == "postgres" {
		require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
			for _, off := range []string{"enable_seqscan", "enable_sort", "enable_bitmapscan"} {
				if err := tx.Exec(`SET LOCAL ` + off + ` = off`).Error; err != nil {
					return err
				}
			}
			return tx.Raw("EXPLAIN " + sql).Scan(&plan).Error
		}))
	} else {
		var rows []struct{ Detail string }
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+sql).Scan(&rows).Error)
		for _, row := range rows {
			plan = append(plan, row.Detail)
		}
	}
	text := strings.Join(plan, "\n")
	require.Contains(t, text, "idx_rm_page", fmt.Sprintf("%s\n%s", sql, text))
	for _, bad := range []string{"TEMP B-TREE", "Sort", "Seq Scan", "SCAN reduction_members"} {
		require.NotContains(t, text, bad, "%s\n%s", sql, text)
	}
	if ctx.db.Dialector.Name() == "postgres" {
		// The whole key is the index condition, not a filter over the cluster's prefix.
		assert.Regexp(t, `Index Cond:.*position.*id`, strings.ReplaceAll(text, "\n", " "))
	}
}

func TestStoreMemberPageSeeksOnTheWholeKeySQLite(t *testing.T) {
	testStoreMemberPageSeeksOnTheWholeKey(t, newReductionSQLiteContext(t))
}

// "Keep the larger copy" must name the larger of two copies whose pixel counts differ by
// one even when both exceed 2^63: the comparison is exact, not a REAL's rounding.
func testStoreLargestLoserIsExactPastInt64(t *testing.T, ctx *MahresourcesContext) {
	kept := &models.Resource{Name: "kept", Hash: "h", Width: 10, Height: 10}
	a := &models.Resource{Name: "a", Hash: "h", Width: 3_999_999_999, Height: 4_000_000_001}
	b := &models.Resource{Name: "b", Hash: "h", Width: 4_000_000_000, Height: 4_000_000_000}
	for _, res := range []*models.Resource{kept, a, b} {
		require.NoError(t, ctx.db.Create(res).Error)
	}
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "k", Tier: "near", State: "open", WinnerID: kept.ID, KeepsSmaller: true}, Members: memberRows(kept.ID, a.ID, b.ID)}))

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	assert.Equal(t, b.ID, review.Clusters[0].LargestMemberID, "4,000,000,000 x 4,000,000,000 is one pixel more than 3,999,999,999 x 4,000,000,001")
}

func TestStoreLargestLoserIsExactPastInt64SQLite(t *testing.T) {
	testStoreLargestLoserIsExactPastInt64(t, newReductionSQLiteContext(t))
}

// A Reduction name stored at its full length must not cross into Go on a members request:
// the row is read with the name cut in SQL, as the review read does.
func testStoreMembersRequestReadsTheBoundedName(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	ids := sequentialIDs(1, 3)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "k", Tier: "identical", State: "open", WinnerID: ids[0]}, Members: memberRows(ids...)}))
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Update("name", strings.Repeat("€", 367)).Error)

	capture := &rowsCapture{}
	spy := *ctx
	spy.db = ctx.db.Session(&gorm.Session{Logger: capture})
	_, err := spy.GetReductionClusterMembers(r.ID, nil, false, "k", "")
	require.NoError(t, err)

	capture.mu.Lock()
	defer capture.mu.Unlock()
	saw := false
	for _, sql := range capture.sqls {
		if strings.Contains(sql, "resource_reductions") && strings.Contains(sql, "SELECT") {
			saw = true
			assert.Regexp(t, `substr(ing)?\(`, strings.ToLower(sql), "the name is cut in SQL: %s", sql)
		}
	}
	assert.True(t, saw)
}

func TestStoreMembersRequestReadsTheBoundedNameSQLite(t *testing.T) {
	testStoreMembersRequestReadsTheBoundedName(t, newReductionSQLiteContext(t))
}
