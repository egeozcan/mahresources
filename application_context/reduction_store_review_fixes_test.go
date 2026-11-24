package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A guest with no scope root sees nothing: not even a Cluster that has no
// members, which an EXISTS-over-members rule would let through.
func testReductionDenyWithholdsEveryCluster(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	empty := models.ReductionClusterRow{ReductionID: r.ID, Generation: "g", Key: "empty", Tier: "identical", State: "open", Checked: true}
	require.NoError(t, ctx.db.Create(&empty).Error)

	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, UserID: 3})
	rows, total, err := guest.reductionStore().pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{Status: []string{"all"}}, 0, 100)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
	tabs, err := guest.reductionStore().tabCounts(r, "g")
	require.NoError(t, err)
	require.Zero(t, tabs.Open)

	// the store never writes a Cluster without members
	require.Error(t, ctx.reductionStore().insertCluster(r, "g", &storedCluster{Row: models.ReductionClusterRow{Key: "none"}}))
}

func TestReductionDenyWithholdsEveryClusterSQLite(t *testing.T) {
	testReductionDenyWithholdsEveryCluster(t, newReductionSQLiteContext(t))
}

// The token is over the eligible Clusters' fingerprints in byte order of key,
// across several chunks, inserted out of order; the expected digest is computed
// here from the definition, not through the store's helpers.
func testReductionTokenOrdersByKeyAcrossChunks(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	store := ctx.reductionStore()
	n := eligibleChunk*2 + 37
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%05d", i*7%n+i) // distinct, not in insertion order
	}
	seen := map[string]bool{}
	for _, k := range keys {
		require.False(t, seen[k])
		seen[k] = true
	}
	rand.New(rand.NewSource(1)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	var members []models.ReductionMemberRow
	_ = members
	for i, k := range keys {
		w, l := uint(1000000+2*i), uint(1000001+2*i)
		c := &storedCluster{Row: models.ReductionClusterRow{Key: k, Tier: "identical", State: "open", Checked: true, WinnerID: w, Position: int64(i)},
			Members: []models.ReductionMemberRow{{ResourceID: w, Hash: "h"}, {ResourceID: l, Hash: "h"}}}
		require.NoError(t, store.insertCluster(r, "g", c))
	}
	var rows []struct{ Key, Fingerprint string }
	require.NoError(t, ctx.db.Raw(`SELECT "key", fingerprint FROM reduction_clusters WHERE reduction_id = ?`, r.ID).Scan(&rows).Error)
	sort.Slice(rows, func(i, j int) bool { return strings.Compare(rows[i].Key, rows[j].Key) < 0 })
	h := sha256.New()
	for _, row := range rows {
		h.Write([]byte(row.Fingerprint + "\n"))
	}
	want := hex.EncodeToString(h.Sum(nil))[:32]

	token, clusters, losers, err := store.applyEligibility(r, "g")
	require.NoError(t, err)
	require.Equal(t, n, clusters)
	require.Equal(t, n, losers)
	require.Equal(t, want, token)
}

func TestReductionTokenOrdersByKeyAcrossChunksSQLite(t *testing.T) {
	testReductionTokenOrdersByKeyAcrossChunks(t, newReductionSQLiteContext(t))
}
