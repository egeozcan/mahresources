package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

func member(id uint, hash string, size int64) models.ReductionMemberRow {
	return models.ReductionMemberRow{ResourceID: id, Hash: hash, FileSize: size, InExtent: true}
}

func TestDeriveClusterColumns(t *testing.T) {
	build := func() storedCluster {
		return storedCluster{
			Row: models.ReductionClusterRow{Key: "identical-aa", WinnerID: 10},
			Members: []models.ReductionMemberRow{
				member(10, "h", 100),
				member(11, "h", 40),
				{ResourceID: 12, Hash: "h", FileSize: 7, Ejected: true},
				{ResourceID: 13, Hash: "h", FileSize: 3, Gone: true},
				{ResourceID: 14, Hash: "h", FileSize: 1, Ejected: true, Gone: true},
			},
		}
	}
	c := build()
	deriveClusterColumns(&c)
	assert.Equal(t, 5, c.Row.MemberCount)
	assert.Equal(t, 2, c.Row.LoserCount, "a Loser is a non-ejected member that is not the Winner")
	assert.EqualValues(t, 151, c.Row.TotalSize, "every member's size counts, ejected ones too")
	assert.Equal(t, 1, c.Row.GoneMembers, "an ejected gone member is not counted: it is left untouched either way")
	assert.False(t, c.Row.NeedsAttention)
	assert.Len(t, c.Row.Fingerprint, 64)

	t.Run("needs attention is lossy, oversized or keeps-smaller", func(t *testing.T) {
		for name, mutate := range map[string]func(*models.ReductionClusterRow){
			"lossy":         func(r *models.ReductionClusterRow) { r.Lossy = []byte(`["description"]`) },
			"oversized":     func(r *models.ReductionClusterRow) { r.Oversized = true },
			"keeps smaller": func(r *models.ReductionClusterRow) { r.KeepsSmaller = true },
		} {
			c := build()
			mutate(&c.Row)
			deriveClusterColumns(&c)
			assert.True(t, c.Row.NeedsAttention, name)
		}
		c := build()
		c.Row.Lossy = []byte(`[]`)
		deriveClusterColumns(&c)
		assert.False(t, c.Row.NeedsAttention, "an empty lossy list is no warning")
	})

	t.Run("the fingerprint moves with everything an apply decides about", func(t *testing.T) {
		base := build()
		deriveClusterColumns(&base)
		for name, mutate := range map[string]func(*storedCluster){
			"key":             func(c *storedCluster) { c.Row.Key = "identical-bb" },
			"winner":          func(c *storedCluster) { c.Row.WinnerID = 11 },
			"a Loser hash":    func(c *storedCluster) { c.Members[1].Hash = "other" },
			"winner hash":     func(c *storedCluster) { c.Members[0].Hash = "other" },
			"a Loser ejected": func(c *storedCluster) { c.Members[1].Ejected = true },
			"fixed seed":      func(c *storedCluster) { c.Row.FixedSeed = true },
		} {
			c := build()
			mutate(&c)
			deriveClusterColumns(&c)
			assert.NotEqual(t, base.Row.Fingerprint, c.Row.Fingerprint, name)
		}
		// Not of an ejected member's hash, nor of member order.
		c := build()
		c.Members[2].Hash = "unrelated"
		c.Members[0], c.Members[1] = c.Members[1], c.Members[0]
		deriveClusterColumns(&c)
		assert.Equal(t, base.Row.Fingerprint, c.Row.Fingerprint)
	})
}

func testReductionStoreRoundTrip(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	store := ctx.reductionStore()

	// The insert is chunked: no statement carries more than reductionRowChunk members.
	var mu sync.Mutex
	var batches []int
	const cb = "test:count_member_batches"
	require.NoError(t, ctx.db.Callback().Create().After("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table != "reduction_members" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, tx.Statement.ReflectValue.Len())
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Create().Remove(cb) })

	big := &storedCluster{Row: models.ReductionClusterRow{Key: "near-big", Tier: "near", State: "open", WinnerID: 1, Checked: true}}
	for i := 1; i <= 1200; i++ {
		big.Members = append(big.Members, member(uint(i), "h", 2))
	}
	require.NoError(t, store.insertCluster(r, "g1", big))
	mu.Lock()
	assert.Equal(t, []int{500, 500, 200}, batches, "1,200 member rows in statements of at most 500")
	mu.Unlock()
	require.NotZero(t, big.Row.ID)

	loaded, err := store.loadCluster(r, "g1", "near-big")
	require.NoError(t, err)
	assert.Len(t, loaded.Members, reductionMemberCap, "a Cluster loads its first 200 members")
	for i, m := range loaded.Members {
		assert.Equal(t, i, m.Position, "in position order")
	}
	assert.Equal(t, 1200, loaded.Row.MemberCount, "the derived count is the whole Cluster, not the loaded part")
	assert.EqualValues(t, 2400, loaded.Row.TotalSize)
	assert.Equal(t, 1199, loaded.Row.LoserCount)

	_, err = store.loadCluster(r, "g1", "no-such-key")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = store.loadCluster(r, "other-generation", "near-big")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "a Cluster is read at its own generation only")

	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestReductionStoreRoundTripSQLite(t *testing.T) {
	testReductionStoreRoundTrip(t, newReductionSQLiteContext(t))
}

// The consistency helper has to be able to fail: a derived column changed behind
// the store's back is caught.
func TestAssertDerivedColumnsConsistentCatchesDrift(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	r := newReductionRow(t, ctx.db)
	c := &storedCluster{Row: models.ReductionClusterRow{Key: "k", WinnerID: 1}, Members: []models.ReductionMemberRow{member(1, "h", 5), member(2, "h", 5)}}
	require.NoError(t, ctx.reductionStore().insertCluster(r, "g", c))
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)

	for column, value := range map[string]any{"loser_count": 7, "total_size": 1, "gone_members": 1, "needs_attention": true, "fingerprint": strings.Repeat("0", 64), "member_count": 9} {
		require.NoError(t, ctx.db.Exec(fmt.Sprintf("UPDATE reduction_clusters SET %s = ? WHERE id = ?", column), value, c.Row.ID).Error)
		rec := &recordingT{}
		func() {
			defer func() { _ = recover() }()
			assertDerivedColumnsConsistent(rec, ctx.db, r.ID)
		}()
		assert.True(t, rec.failed, "a drifted %s must be reported", column)
		require.NoError(t, ctx.db.Exec("UPDATE reduction_clusters SET member_count = 2, loser_count = 1, total_size = 10, gone_members = 0, needs_attention = ?, fingerprint = ? WHERE id = ?",
			false, c.Row.Fingerprint, c.Row.ID).Error)
	}
}

type recordingT struct{ failed bool }

func (r *recordingT) Errorf(string, ...any) { r.failed = true }
func (r *recordingT) FailNow()              { r.failed = true; panic("fail now") }
func (r *recordingT) Helper()               {}

func testReductionStoreReads(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	store := ctx.reductionStore()
	add := func(key, tier, state string, checked bool, copies int, size int64, position int64) {
		c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: tier, State: state, Checked: checked, Position: position, WinnerID: uint(position*100 + 1), Merged: state == "applied"}}
		for i := 0; i < copies; i++ {
			c.Members = append(c.Members, member(uint(position*100+1+int64(i)), "h", size))
		}
		require.NoError(t, store.insertCluster(r, "g", c))
	}
	add("a-open-ident", "identical", "open", true, 3, 10, 1)
	add("b-open-near", "near", "open", false, 2, 90, 2)
	add("c-skipped", "identical", "skipped", false, 5, 1, 3)
	add("d-applied", "near", "applied", false, 2, 5, 4)
	add("e-stale", "identical", "stale", true, 4, 2, 5)
	add("f-open-ident", "identical", "open", false, 3, 30, 6)

	rows, total, err := store.pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{Status: []string{"open"}}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, []string{"a-open-ident", "f-open-ident", "b-open-near"}, clusterRowKeys(rows), "most copies first, then key")

	rows, _, err = store.pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{Status: []string{"open"}, Sort: "size"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"b-open-near", "f-open-ident", "a-open-ident"}, clusterRowKeys(rows), "largest total size first: 180, 90, 30")

	rows, total, err = store.pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{Status: []string{"all"}, Sort: "computed"}, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 6, total, "the total is the filter's, not the page's")
	assert.Equal(t, []string{"c-skipped", "d-applied"}, clusterRowKeys(rows), "plan order, paged by offset")

	tabs, err := store.tabCounts(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 3, tabs.Open)
	assert.Equal(t, 1, tabs.Skipped)
	assert.Equal(t, 1, tabs.Stale)
	assert.Equal(t, 1, tabs.Applied)

	figures, err := store.figures(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 1, figures.Checked)
	assert.Equal(t, 2, figures.CheckedLosers)
	assert.Equal(t, 1, figures.Apply.Identical.Clusters)
	assert.Equal(t, 2, figures.Bulk.IdenticalOpen)
	assert.Equal(t, 1, figures.Bulk.IdenticalApprovable)

	token, clusters, losers, err := store.applyEligibility(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 1, clusters)
	assert.Equal(t, 2, losers)
	assert.Len(t, token, 32)
	// The token binds the proposal: a Loser's hash changing moves it.
	require.NoError(t, ctx.db.Exec("UPDATE reduction_members SET hash = 'changed' WHERE resource_id = 102").Error)
	var row models.ReductionClusterRow
	require.NoError(t, ctx.db.Where(`"key" = ?`, "a-open-ident").First(&row).Error)
	c, err := store.loadCluster(r, "g", "a-open-ident")
	require.NoError(t, err)
	deriveClusterColumns(c)
	require.NoError(t, ctx.db.Model(&models.ReductionClusterRow{}).Where("id = ?", row.ID).Update("fingerprint", c.Row.Fingerprint).Error)
	token2, _, _, err := store.applyEligibility(r, "g")
	require.NoError(t, err)
	assert.NotEqual(t, token, token2)

	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestReductionStoreReadsSQLite(t *testing.T) {
	testReductionStoreReads(t, newReductionSQLiteContext(t))
}
