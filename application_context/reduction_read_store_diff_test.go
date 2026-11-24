package application_context

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Differential: the JSON reader (getReductionReview over a decoded plan) is the
// oracle for storeGetReductionReview over the same plan written as rows, for
// randomized plans and principals, on SQLite and Postgres. The documented
// differences are not compared: the apply token's definition, and a Cluster's
// total_size (compute-time sizes), which only the size sort could expose, and the
// oracle's own sort reads the live sizes, which the harness keeps equal.

type readTwin struct {
	oldID, newID uint
	clusters     []*models.ReductionCluster
	// lastEvidence is whether the table twin's last-apply record can account for
	// its counts: no counts, or a batch id that exactly that many Clusters carry
	// (design D10). Computed from the rows, independently of the reader. The JSON
	// reader never checks this for a viewer nothing is withheld from, so for one the
	// table reader differs from it exactly where the evidence is missing.
	lastEvidence bool
}

// makeReadTwin writes one random world's plan twice: as a legacy JSON plan on one
// Reduction and as tables on another, with the same extent, coverage, settings
// and last apply.
func makeReadTwin(t *testing.T, w *reductionWorld, rng *rand.Rand) *readTwin {
	ctx := w.ctx
	// Dimensions, times, related Groups and Changed members: what the page shows
	// and the drift figure counts.
	for _, id := range w.resources {
		if w.exists[id] {
			require.NoError(t, ctx.db.Exec("UPDATE resources SET width = ?, height = ?, created_at = ? WHERE id = ?",
				100*(1+rng.Intn(4)), 100*(1+rng.Intn(4)), time.Now().Add(-time.Duration(rng.Intn(180))*time.Minute), id).Error)
		}
	}
	for i := 0; i < 12 && len(w.groups) > 0 && len(w.resources) > 0; i++ {
		_ = ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)",
			w.groups[rng.Intn(len(w.groups))], w.resources[rng.Intn(len(w.resources))]).Error
	}
	clusters := w.randomPlan(t)
	for _, c := range clusters {
		for _, m := range c.Members {
			if rng.Intn(9) == 0 {
				c.ChangedMembers = append(c.ChangedMembers, m.ResourceID)
			}
		}
	}

	// The extent.
	var extent models.ResourceReductionExtent
	extent.All = rng.Intn(5) == 0
	if !extent.All {
		for i := 0; i < rng.Intn(30); i++ {
			extent.ResourceIDs = append(extent.ResourceIDs, w.resources[rng.Intn(len(w.resources))])
		}
		extent.ResourceIDs = dedupeUints(extent.ResourceIDs)
		for i := 0; i < rng.Intn(4) && len(w.groups) > 0; i++ {
			extent.GroupIDs = append(extent.GroupIDs, w.groups[rng.Intn(len(w.groups))])
		}
		extent.GroupIDs = dedupeUints(extent.GroupIDs)
		if rng.Intn(4) == 0 {
			extent.GroupIDs = append(extent.GroupIDs, 777777) // a Group that no longer exists
		}
	}

	computedAt := time.Now().Add(-time.Duration(60+rng.Intn(60)) * time.Minute)
	coverage := models.ReductionCoverage{ExtentSize: 5 + rng.Intn(50), ContentHashed: rng.Intn(40), PerceptualHashed: rng.Intn(30), PerceptualEligible: rng.Intn(30)}
	settings := &models.ReductionSettings{MatchingMode: models.MatchingModeBothTiers, WinnerRule: models.DefaultWinnerRule()}
	switch rng.Intn(4) {
	case 0:
		settings.MatchingMode = models.MatchingModeIdenticalOnly
	case 1:
		settings.WinnerRule = []string{models.WinnerCriterionSizeDesc}
	}

	last := &models.ReductionLastApply{StartedAt: time.Now().Add(-time.Minute), RequestID: "req"}
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
		last.Applied = rng.Intn(3)
	}
	if len(last.ClusterIDs) > 0 {
		last.Applied = len(last.ClusterIDs)
	}
	running := rng.Intn(2) == 0
	if !running {
		done := time.Now()
		last.FinishedAt = &done
	}
	hasLast := rng.Intn(5) > 0

	// The legacy Reduction.
	oldRow := newReductionRow(t, ctx.db)
	oldRow.Status = models.ReductionStatusReady
	plan := models.ResourceReductionPlan{Clusters: clonePlanClusters(t, clusters), Coverage: coverage, Settings: settings}
	if hasLast {
		plan.LastApply = last
	}
	extentJSON, err := json.Marshal(extent)
	require.NoError(t, err)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET extent = ?, computed_at = ?, status = 'ready' WHERE id = ?", string(extentJSON), computedAt, oldRow.ID).Error)
	require.NoError(t, ctx.overwriteLegacyPlanForTest(oldRow.ID, plan))

	// The table Reduction.
	newRow := newReductionRow(t, ctx.db)
	const gen = "gen"
	admin := ctx.reductionStore()
	for i, c := range clusters {
		sc := w.storedFromPlan(c, int64(i))
		for k := range sc.Members {
			sc.Members[k].Changed = containsUint(c.ChangedMembers, sc.Members[k].ResourceID)
		}
		require.NoError(t, admin.insertCluster(newRow, gen, sc))
	}
	if len(extent.ResourceIDs) > 0 {
		require.NoError(t, admin.addExtentResources(newRow, extent.ResourceIDs))
	}
	if len(extent.GroupIDs) > 0 {
		require.NoError(t, admin.addExtentGroups(newRow, extent.GroupIDs))
	}
	coverageJSON, _ := json.Marshal(coverage)
	settingsJSON, _ := json.Marshal(settings)
	updates := map[string]any{"plan_schema": 1, "plan_generation": gen, "extent_all": extent.All, "status": "ready",
		"computed_at": computedAt, "coverage": string(coverageJSON), "plan_settings": string(settingsJSON)}
	if hasLast {
		raw, _ := json.Marshal(last)
		var rec map[string]any
		require.NoError(t, json.Unmarshal(raw, &rec))
		delete(rec, "clusterIds")
		batch := ""
		if len(last.ClusterIDs) > 0 {
			batch = "batch-1"
			rec["batchId"] = batch
			for _, id := range last.ClusterIDs {
				require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET outcome_batch = ? WHERE reduction_id = ? AND "key" = ?`, batch, newRow.ID, id).Error)
			}
		}
		raw, _ = json.Marshal(rec)
		updates["last_apply"] = string(raw)
	}
	if running && hasLast {
		updates["apply_lease_until"] = time.Now().Add(time.Hour).UnixNano()
	}
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", newRow.ID).Updates(updates).Error)
	evidence := true
	if hasLast {
		counted := last.Applied + last.Stale
		if rec, ok := updates["last_apply"].(string); ok {
			var parsed struct {
				BatchID string `json:"batchId"`
			}
			require.NoError(t, json.Unmarshal([]byte(rec), &parsed))
			var stamped int64
			if parsed.BatchID != "" {
				require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND outcome_batch = ?`, newRow.ID, parsed.BatchID).Scan(&stamped).Error)
			}
			evidence = (parsed.BatchID == "" && counted == 0) || (parsed.BatchID != "" && int(stamped) == counted)
		}
	}
	return &readTwin{oldID: oldRow.ID, newID: newRow.ID, clusters: clusters, lastEvidence: evidence}
}

func assertSameReview(t *testing.T, label string, want, got *contracts.ReductionReview, twin *readTwin, scoped bool) {
	t.Helper()
	assert.Equal(t, want.Status, got.Status, label+" status")
	assert.Equal(t, want.Coverage, got.Coverage, label+" coverage")
	assert.Equal(t, want.CoverageTrusted, got.CoverageTrusted, label+" coverage trusted")
	assert.Equal(t, want.ClusterCount, got.ClusterCount, label+" cluster count")
	assert.Equal(t, want.CheckedCount, got.CheckedCount, label+" checked")
	assert.Equal(t, want.CheckedLoserCount, got.CheckedLoserCount, label+" checked losers")
	assert.Equal(t, want.Page, got.Page, label)
	assert.Equal(t, want.PageSize, got.PageSize, label)
	assert.Equal(t, want.SelectedResources, got.SelectedResources, label+" selected resources")
	assert.Equal(t, want.SelectedGroups, got.SelectedGroups, label+" selected groups")
	assert.Equal(t, want.ExtentSize, got.ExtentSize, label+" extent size")
	assert.Equal(t, want.EnteredSinceCompute, got.EnteredSinceCompute, label+" entered since compute")
	assert.Equal(t, want.ExtentAll, got.ExtentAll, label)
	assert.Equal(t, want.SelectionGroups, got.SelectionGroups, label+" selection groups")
	assert.Equal(t, want.SelectionResources, got.SelectionResources, label+" selection resources")
	assert.Equal(t, want.MoreGroups, got.MoreGroups, label)
	assert.Equal(t, want.MoreResources, got.MoreResources, label)
	assert.Equal(t, want.TabCounts, got.TabCounts, label+" tabs")
	assert.Equal(t, want.ApplyFigures, got.ApplyFigures, label+" apply figures")
	assert.Equal(t, want.Bulk, got.Bulk, label+" bulk")
	assert.Equal(t, want.SettleableClaims, got.SettleableClaims, label+" settleable")
	assert.Equal(t, want.SettingsChanged, got.SettingsChanged, label+" settings changed")
	assert.NotEmpty(t, got.ApplyToken, label+" token")

	require.Equal(t, want.LastApply == nil, got.LastApply == nil, label+" last apply presence")
	if want.LastApply != nil {
		w, g := *want.LastApply, *got.LastApply
		if !scoped && !twin.lastEvidence {
			// Nothing is withheld from this viewer, so the JSON reader shows the counts
			// as recorded; the table reader shows them only with evidence (D10).
			assert.False(t, w.CountsHidden, label)
			assert.True(t, g.CountsHidden, label+" counts without evidence are unavailable to an administrator too")
			assert.Zero(t, g.Applied+g.Removed+g.Stale, label)
			w.Applied, w.Removed, w.Stale, w.CountsHidden = 0, 0, 0, true
		} else if !scoped {
			assert.False(t, g.CountsHidden, label+" counts with evidence are shown")
		}
		assert.WithinDuration(t, w.StartedAt, g.StartedAt, time.Millisecond, label)
		assert.Equal(t, w.FinishedAt == nil, g.FinishedAt == nil, label)
		w.StartedAt, g.StartedAt, w.FinishedAt, g.FinishedAt = time.Time{}, time.Time{}, nil, nil
		assert.Equal(t, w, g, label+" last apply")
	}

	require.Equal(t, len(want.Clusters), len(got.Clusters), label+" page length")
	for i := range want.Clusters {
		assertSameClusterView(t, fmt.Sprintf("%s cluster %d (%s)", label, i, want.Clusters[i].ID), want.Clusters[i], got.Clusters[i])
	}
}

func assertSameResource(t *testing.T, label string, want, got *models.Resource) {
	t.Helper()
	require.Equal(t, want == nil, got == nil, label+" presence")
	if want == nil {
		return
	}
	assert.Equal(t, want.ID, got.ID, label)
	assert.Equal(t, want.Name, got.Name, label)
	assert.Equal(t, want.OriginalName, got.OriginalName, label)
	assert.Equal(t, want.Hash, got.Hash, label)
	assert.Equal(t, want.ContentType, got.ContentType, label)
	assert.Equal(t, want.FileSize, got.FileSize, label)
	assert.Equal(t, want.Width, got.Width, label)
	assert.Equal(t, want.Height, got.Height, label)
	assert.Equal(t, want.OwnerId, got.OwnerId, label)
	assert.WithinDuration(t, want.CreatedAt, got.CreatedAt, time.Millisecond, label)
}

func assertSameClusterView(t *testing.T, label string, want, got contracts.ReductionClusterView) {
	t.Helper()
	w, g := want.ReductionCluster, got.ReductionCluster
	assert.Equal(t, w.ID, g.ID, label)
	assert.Equal(t, w.Tier, g.Tier, label)
	assert.Equal(t, w.WinnerID, g.WinnerID, label)
	assert.Equal(t, w.State, g.State, label+" state")
	assert.Equal(t, w.Checked, g.Checked, label+" checked")
	assert.Equal(t, w.Reviewed, g.Reviewed, label)
	assert.Equal(t, w.Oversized, g.Oversized, label)
	assert.Equal(t, w.KeepsSmaller, g.KeepsSmaller, label)
	assert.Equal(t, w.Lossy, g.Lossy, label)
	assert.Equal(t, w.DecidedBy, g.DecidedBy, label)
	assert.Equal(t, w.Margin, g.Margin, label)
	assert.Equal(t, w.Undecided, g.Undecided, label)
	assert.Equal(t, w.StaleReason, g.StaleReason, label)
	assert.Equal(t, w.Merged, g.Merged, label)
	assert.Equal(t, w.ClaimID, g.ClaimID, label)
	assert.ElementsMatch(t, w.ChangedMembers, g.ChangedMembers, label)

	assert.Equal(t, want.DecidedByLabel, got.DecidedByLabel, label)
	assert.Equal(t, want.StateLabel, got.StateLabel, label+" state label")
	assert.Equal(t, want.LoserCount, got.LoserCount, label+" losers")
	assert.Equal(t, want.MergedCount, got.MergedCount, label)
	assert.Equal(t, want.LeftOutCount, got.LeftOutCount, label)
	assert.Equal(t, want.WinnerChanged, got.WinnerChanged, label+" winner changed")
	assert.Equal(t, want.Withheld, got.Withheld, label)
	assert.Equal(t, want.Position, got.Position, label)
	assert.Equal(t, want.NeedsAttention, got.NeedsAttention, label)
	assert.Equal(t, want.KeptDimensions, got.KeptDimensions, label)
	assert.Equal(t, want.LargerDimensions, got.LargerDimensions, label)
	assert.Equal(t, want.LargestMemberID, got.LargestMemberID, label)
	assertSameResource(t, label+" winner", want.Winner, got.Winner)

	require.Equal(t, len(want.Members), len(got.Members), label+" members")
	for i := range want.Members {
		wm, gm := want.Members[i], got.Members[i]
		ml := fmt.Sprintf("%s member %d", label, i)
		assert.Equal(t, *wm.ReductionMember, *gm.ReductionMember, ml)
		assertSameResource(t, ml, wm.Resource, gm.Resource)
		assert.Equal(t, wm.IsWinner, gm.IsWinner, ml)
		assert.Equal(t, wm.IsLoser, gm.IsLoser, ml)
		assert.Equal(t, wm.Changed, gm.Changed, ml)
		assert.Equal(t, wm.DistanceLabel, gm.DistanceLabel, ml)
		assert.Equal(t, wm.TagCount, gm.TagCount, ml)
		assert.Equal(t, wm.NoteCount, gm.NoteCount, ml)
		assert.Equal(t, wm.GroupCount, gm.GroupCount, ml)
	}
}

// eligibleFingerprints are the stored fingerprints of the Clusters an apply may act
// on for this principal, in key order: open, nothing gone, not read-only history,
// checked with a copy to remove, unclaimed, and fully visible. The first conditions
// are applied here in Go to plain rows; only visibility uses the store's predicate.
func eligibleFingerprints(t *testing.T, pctx *MahresourcesContext, reductionID uint) []string {
	t.Helper()
	store := pctx.reductionStore()
	prefix, args := store.with()
	var rows []models.ReductionClusterRow
	require.NoError(t, store.db.Raw(prefix+`SELECT c.* FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = 'gen' AND NOT `+store.withheld("c")+` ORDER BY c."key"`,
		append(args, reductionID)...).Scan(&rows).Error)
	var out []string
	for _, row := range rows {
		if row.State == models.ReductionClusterOpen && row.GoneMembers == 0 && !row.ReadOnly && row.Checked && row.LoserCount > 0 && row.ClaimID == "" {
			out = append(out, row.Fingerprint)
		}
	}
	return out
}

func testReductionReadStoreMatchesJSONReader(t *testing.T, newCtx func(*testing.T) *MahresourcesContext, seeds []int64) {
	compared, scopedCompared, withLast, withDims, nonEmptyTokens := 0, 0, 0, 0, 0
	for _, seed := range seeds {
		rng := rand.New(rand.NewSource(seed))
		ctx := newCtx(t)
		w := newReductionWorld(t, ctx, rng, 10+rng.Intn(8), 70, seed%3 == 0)
		twin := makeReadTwin(t, w, rng)

		principals := w.principals()
		// The earliest Groups are the forest's roots, whose subtrees are the large ones.
		for i := 0; i < 3 && i < len(w.groups); i++ {
			root := w.groups[i]
			principals[fmt.Sprintf("scoped-root-%d", i)] = &auth.Principal{Role: models.RoleUser, UserID: uint(40 + i), ScopeGroupID: &root}
		}
		for name, p := range principals {
			pctx := ctx.WithPrincipal(p)
			for i := 0; i < 6; i++ {
				q := randomReviewQuery(rng)
				q.PageSize = []int{0, 20, 50}[rng.Intn(3)]
				page := 1 + rng.Intn(2)
				label := fmt.Sprintf("seed %d principal %s query %+v page %d", seed, name, q, page)
				want, err := pctx.getReductionReview(twin.oldID, nil, false, page, q)
				require.NoError(t, err, label)
				got, _, err := pctx.storeGetReductionReview(twin.newID, nil, false, page, q)
				require.NoError(t, err, label)
				assertSameReview(t, label, want, got, twin, pctx.isScopedPrincipal())
				// The apply token is the digest of the eligible Clusters' stored
				// fingerprints in key order, read here by a plain SELECT and
				// filtered in Go, so a token that ignored the eligible set fails.
				fps := eligibleFingerprints(t, pctx, twin.newID)
				assert.Equal(t, applyTokenOf(fps), got.ApplyToken, label+" apply token")
				if len(fps) > 0 {
					nonEmptyTokens++
					assert.NotEqual(t, applyTokenOf(nil), got.ApplyToken, label+" a nonempty eligible set digests to something")
				}
				compared += len(want.Clusters)
				if ctx.WithPrincipal(p).isScopedPrincipal() {
					scopedCompared += len(want.Clusters)
				}
				if want.LastApply != nil {
					withLast++
				}
				for _, v := range want.Clusters {
					if v.KeptDimensions != "" {
						withDims++
					}
				}
			}

			// The list's progress.
			oldRow, err := ctx.GetResourceReduction(twin.oldID, nil, false)
			require.NoError(t, err)
			wantProgress, err := pctx.legacyReductionProgress([]models.ResourceReduction{*oldRow})
			require.NoError(t, err)
			newRow, err := ctx.GetResourceReduction(twin.newID, nil, false)
			require.NoError(t, err)
			got, err := pctx.storeGetReductionProgress([]models.ResourceReduction{*newRow})
			require.NoError(t, err)
			assert.Equal(t, wantProgress[twin.oldID], got.Progress[twin.newID], fmt.Sprintf("seed %d principal %s progress", seed, name))
			assert.Empty(t, got.Upgrading)
		}
		assertDerivedColumnsConsistent(t, ctx.db, twin.newID)
	}
	// The comparison has to have had something to compare.
	require.Greater(t, compared, 20*len(seeds), "Cluster views compared")
	require.Greater(t, scopedCompared, 2*len(seeds), "Cluster views compared for a scoped principal")
	require.Greater(t, withLast, len(seeds))
	require.Greater(t, withDims, 0, "keeps-smaller dimensions compared")
	require.Greater(t, nonEmptyTokens, 0, "an apply token over a nonempty eligible set compared")
	t.Logf("compared %d Cluster views (%d scoped), %d last-apply records, %d keeps-smaller", compared, scopedCompared, withLast, withDims)
}

func TestReductionReadStoreMatchesJSONReaderSQLite(t *testing.T) {
	testReductionReadStoreMatchesJSONReader(t, newReductionSQLiteContext, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
}

var _ = query_models.ResourceReductionReviewQuery{}
