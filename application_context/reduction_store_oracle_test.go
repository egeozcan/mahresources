package application_context

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// Differential oracle (design D14): today's Go functions over a decoded plan are
// the oracle, and the store's SQL answers over the same plan written as rows must
// equal them, for randomized plans and principals, on SQLite and Postgres.

func clonePlanClusters(t *testing.T, in []*models.ReductionCluster) []*models.ReductionCluster {
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	var out []*models.ReductionCluster
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func randomReviewQuery(rng *rand.Rand) *query_models.ResourceReductionReviewQuery {
	q := &query_models.ResourceReductionReviewQuery{}
	statuses := [][]string{nil, {"open"}, {"reviewed"}, {"skipped", "applied"}, {"stale"}, {"all"}, {"bogus"}, {"open", "stale"}, {"applied"}}
	q.Status = statuses[rng.Intn(len(statuses))]
	tiers := [][]string{nil, {"identical"}, {"near"}, {"identical", "near"}}
	q.Tier = tiers[rng.Intn(len(tiers))]
	q.NeedsAttention = rng.Intn(3) == 0
	q.Approval = []string{"", "approved", "unapproved", "other"}[rng.Intn(4)]
	q.Sort = []string{"", "copies", "size", "computed"}[rng.Intn(4)]
	return q
}

func testReductionStoreMatchesOracle(t *testing.T, newCtx func(*testing.T) *MahresourcesContext, seeds []int64) {
	for _, seed := range seeds {
		rng := rand.New(rand.NewSource(seed))
		ctx := newCtx(t)
		w := newReductionWorld(t, ctx, rng, 10+rng.Intn(8), 60, seed%3 == 0)
		clusters := w.randomPlan(t)

		r := newReductionRow(t, ctx.db)
		const gen = "gen"
		admin := ctx.reductionStore()
		for i, c := range clusters {
			require.NoError(t, admin.insertCluster(r, gen, w.storedFromPlan(c, int64(i))))
		}

		// The last apply record: counts about a subset of Clusters, sometimes naming
		// a Cluster that is not in the plan, sometimes predating the ids.
		last := &models.ReductionLastApply{StartedAt: time.Now(), RequestID: "req"}
		for _, c := range clusters {
			if rng.Intn(5) == 0 {
				last.ClusterIDs = append(last.ClusterIDs, c.ID)
			}
		}
		switch rng.Intn(4) {
		case 0:
			last.ClusterIDs = append(last.ClusterIDs, "not-in-the-plan")
		case 1:
			last.ClusterIDs = nil
			last.Applied = rng.Intn(3) // counts without ids
		}
		if len(last.ClusterIDs) > 0 {
			last.Applied = len(last.ClusterIDs)
		}
		running := rng.Intn(2) == 0
		if !running {
			done := time.Now()
			last.FinishedAt = &done
		}
		for _, id := range last.ClusterIDs {
			require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET outcome_batch = 'batch-1' WHERE reduction_id = ? AND "key" = ?`, r.ID, id).Error)
		}
		if running {
			r.ApplyLeaseUntil = time.Now().Add(time.Hour).UnixNano()
		}
		batch := ""
		if len(last.ClusterIDs) > 0 {
			batch = "batch-1"
		}

		for name, p := range w.principals() {
			label := fmt.Sprintf("seed %d principal %s", seed, name)
			pctx := ctx.WithPrincipal(p)
			store := pctx.reductionStore()

			plan := models.ResourceReductionPlan{Clusters: clonePlanClusters(t, clusters), LastApply: last}
			visible, err := pctx.reviewReach(&plan, reductionMemberIDs(plan.Clusters))
			require.NoError(t, err)

			// Tab counts.
			tabs, err := store.tabCounts(r, gen)
			require.NoError(t, err)
			assert.Equal(t, countReviewTabs(plan.Clusters, visible), tabs, label+" tabs")

			// Filtered, sorted, paged lists.
			for i := 0; i < 14; i++ {
				q := randomReviewQuery(rng)
				filtered := filterReviewClusters(plan.Clusters, visible, q)
				sorted, err := pctx.sortReviewClusters(filtered, visible, q)
				require.NoError(t, err)
				rows, total, err := store.pageClusters(r, gen, q, 0, 10000)
				require.NoError(t, err)
				require.Equal(t, len(sorted), total, label+" total for %+v", q)
				require.Equal(t, planKeys(sorted), clusterRowKeys(rows), label+" order for %+v", q)
				// A middle page is the same slice of the same order.
				page, _, err := store.pageClusters(r, gen, q, 3, 5)
				require.NoError(t, err)
				lo, hi := min(3, len(sorted)), min(8, len(sorted))
				require.Equal(t, planKeys(sorted[lo:hi]), clusterRowKeys(page), label+" page for %+v", q)
			}

			// Figures, and the set the apply token is over.
			wantFigures, eligible := reviewFiguresOf(r, plan.Clusters, visible)
			got, err := store.figures(r, gen)
			require.NoError(t, err)
			assert.Equal(t, wantFigures.checked, got.Checked, label)
			assert.Equal(t, wantFigures.checkedLosers, got.CheckedLosers, label)
			assert.Equal(t, wantFigures.bulk, got.Bulk, label)
			assert.Equal(t, wantFigures.apply.Identical.Clusters, got.Apply.Identical.Clusters, label)
			assert.Equal(t, wantFigures.apply.Identical.Losers, got.Apply.Identical.Losers, label)
			assert.Equal(t, wantFigures.apply.Near.Clusters, got.Apply.Near.Clusters, label)
			assert.Equal(t, wantFigures.apply.Near.Losers, got.Apply.Near.Losers, label)
			assert.Equal(t, wantFigures.apply.KeepsSmaller, got.Apply.KeepsSmaller, label)

			sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
			var wantFingerprints []string
			wantLosers := 0
			for _, c := range eligible {
				sc := w.storedFromPlan(c, 0)
				deriveClusterColumns(sc)
				wantFingerprints = append(wantFingerprints, sc.Row.Fingerprint)
				wantLosers += len(c.LoserIDs())
			}
			token, nClusters, nLosers, err := store.applyEligibility(r, gen)
			require.NoError(t, err)
			assert.Equal(t, len(eligible), nClusters, label+" eligible")
			assert.Equal(t, wantLosers, nLosers, label+" eligible losers")
			assert.Equal(t, applyTokenOf(wantFingerprints), token, label+" token")

			// Settle-only and the last apply's counts.
			wantSettle := settleableClaims(&plan, visible)
			gotSettle, err := store.settleableClaims(r, gen, time.Now())
			require.NoError(t, err)
			assert.Equal(t, wantSettle, gotSettle, label+" settleable")

			wantVisible := lastApplyCountsVisible(last, plan.Clusters, visible)
			gotVisible, err := store.lastApplyCountsVisible(r, gen, batch, last.Applied+last.Stale)
			require.NoError(t, err)
			assert.Equal(t, wantVisible, gotVisible, label+" last apply")
		}
		assertDerivedColumnsConsistent(t, ctx.db, r.ID)
	}
}

func TestReductionStoreMatchesOracleSQLite(t *testing.T) {
	testReductionStoreMatchesOracle(t, newReductionSQLiteContext, []int64{1, 2, 3, 4, 5, 6, 7, 8})
}
