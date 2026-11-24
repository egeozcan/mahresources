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
