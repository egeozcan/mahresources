package application_context

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// (4) A stale reason is stored cut to 1,024 bytes at a rune boundary, and the review
// reads select a bounded projection of it, so a plugin veto reason of 1 MiB (or a row
// written before the bound) never reaches a review page.
func testReductionStaleReasonIsBounded(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	a := &models.Resource{Name: "a"}
	b := &models.Resource{Name: "b"}
	require.NoError(t, ctx.db.Create(a).Error)
	require.NoError(t, ctx.db.Create(b).Error)
	s := ctx.reductionStore()

	// Three-byte runes, so byte 1,024 falls inside one (1024 = 341*3 + 1).
	huge := strings.Repeat("€", 400000)
	c := &storedCluster{Row: models.ReductionClusterRow{Key: "veto", Tier: "identical", State: "stale", WinnerID: a.ID, StaleReason: huge},
		Members: []models.ReductionMemberRow{{ResourceID: a.ID, Hash: "h"}, {ResourceID: b.ID, Hash: "h"}}}
	require.NoError(t, s.insertCluster(r, "g", c))

	var stored string
	require.NoError(t, ctx.db.Raw(`SELECT stale_reason FROM reduction_clusters WHERE reduction_id = ? AND "key" = ?`, r.ID, "veto").Scan(&stored).Error)
	require.LessOrEqual(t, len(stored), 1024, "stored reason is bounded")
	require.True(t, utf8.ValidString(stored), "cut at a rune boundary")
	require.Equal(t, 341*3, len(stored))

	// A row written before the bound still reads bounded.
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET stale_reason = ? WHERE reduction_id = ? AND "key" = ?`, huge, r.ID, "veto").Error)
	rows, _, err := s.pageClusters(r, "g", nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.LessOrEqual(t, len(rows[0].StaleReason), 4*1024, "review page read is bounded")
	require.Less(t, len(rows[0].StaleReason), len(huge))
	loaded, err := s.loadCluster(r, "g", "veto")
	require.NoError(t, err)
	require.Less(t, len(loaded.Row.StaleReason), len(huge))
	require.NotEmpty(t, loaded.Row.ID)
}

func TestReductionStaleReasonIsBoundedSQLite(t *testing.T) {
	testReductionStaleReasonIsBounded(t, newReductionSQLiteContext(t))
}

// The projection must name every column of the model.
func TestReviewClusterColumnsCoverTheModel(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	stmt := ctx.db.Session(&gorm.Session{}).Statement
	require.NoError(t, stmt.Parse(&models.ReductionClusterRow{}))
	var want []string
	for _, f := range stmt.Schema.Fields {
		if f.DBName != "" {
			want = append(want, f.DBName)
		}
	}
	var got []string
	bounded := strings.Replace(reviewClusterColumns, "SUBSTR(c.stale_reason, 1, 1024) AS stale_reason", "c.stale_reason", 1)
	require.NotEqual(t, bounded, reviewClusterColumns, "the stale reason is read bounded")
	for _, part := range strings.Split(bounded, ",") {
		got = append(got, strings.Trim(strings.TrimPrefix(strings.TrimSpace(part), "c."), `"`))
	}
	require.ElementsMatch(t, want, got)
}
