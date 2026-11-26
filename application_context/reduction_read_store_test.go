package application_context

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// tableReduction is a Reduction row whose plan is in tables, at generation "gen".
func tableReduction(t *testing.T, ctx *MahresourcesContext) *models.ResourceReduction {
	t.Helper()
	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Where("id = ?", r.ID).Updates(map[string]any{"plan_schema": 1, "plan_generation": "gen"}).Error)
	r.PlanSchema = 1
	gen := "gen"
	r.PlanGeneration = &gen
	return r
}

func memberRows(ids ...uint) []models.ReductionMemberRow {
	rows := make([]models.ReductionMemberRow, len(ids))
	for i, id := range ids {
		rows[i] = models.ReductionMemberRow{ResourceID: id, Hash: "h", InExtent: true}
	}
	return rows
}

func sequentialIDs(from uint, n int) []uint {
	ids := make([]uint, n)
	for i := range ids {
		ids[i] = from + uint(i)
	}
	return ids
}

func testStoreReviewRefusesALegacyPlan(t *testing.T, ctx *MahresourcesContext) {
	legacy := newReductionRow(t, ctx.db) // plan_schema 0
	_, _, err := ctx.storeGetReductionReview(legacy.ID, nil, false, 1, nil)
	require.ErrorIs(t, err, ErrReductionUpgrading)

	current := tableReduction(t, ctx)
	got, err := ctx.storeGetReductionProgress([]models.ResourceReduction{*legacy, *current})
	require.NoError(t, err)
	assert.True(t, got.Upgrading[legacy.ID], "a legacy row renders as being upgraded")
	assert.NotContains(t, got.Progress, legacy.ID)
	assert.Contains(t, got.Progress, current.ID, "the rest of the page still counts")

	// A Reduction nobody may see answers as an unknown id does.
	_, _, err = ctx.storeGetReductionReview(987654, nil, false, 1, nil)
	require.ErrorIs(t, err, ErrReductionNotFound)
}

func TestStoreReviewRefusesALegacyPlanSQLite(t *testing.T) {
	testStoreReviewRefusesALegacyPlan(t, newReductionSQLiteContext(t))
}

func testStoreReviewBoundsDisplayText(t *testing.T, ctx *MahresourcesContext) {
	name := strings.Repeat("€", 367) // 1,101 bytes, three per rune: the 1,024th byte cuts one in half
	require.Len(t, name, 1101)
	var ids []uint
	for i := 0; i < 2; i++ {
		res := &models.Resource{Name: name, OriginalName: name, ContentType: "image/png", Hash: "h"}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	short := &models.Resource{Name: "short", Hash: "h"}
	require.NoError(t, ctx.db.Create(short).Error)

	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "c1", Tier: "identical", State: "open", WinnerID: ids[0]}, Members: memberRows(ids[0], ids[1], short.ID)}))
	require.NoError(t, ctx.reductionStore().addExtentResources(r, ids))

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	check := func(label, got string) {
		assert.True(t, utf8.ValidString(got), label+" is valid UTF-8")
		assert.LessOrEqual(t, len(got), reductionDisplayTextBytes, label)
		assert.Equal(t, strings.Repeat("€", 341), got, label+" is cut to a rune boundary")
	}
	check("winner name", review.Clusters[0].Winner.Name)
	check("member name", review.Clusters[0].Members[1].Resource.Name)
	check("original name", review.Clusters[0].Members[1].Resource.OriginalName)
	assert.Equal(t, "short", review.Clusters[0].Members[2].Resource.Name, "a short value is untouched")
	require.Len(t, review.SelectionResources, 2)
	check("selection name", review.SelectionResources[0].Name)

	// Exactly at the bound with plain ASCII is kept whole minus nothing.
	ascii := &models.Resource{Name: strings.Repeat("a", 1024), Hash: "h"}
	require.NoError(t, ctx.db.Create(ascii).Error)
	got, err := ctx.reductionStore().loadNarrowResources([]uint{ascii.ID})
	require.NoError(t, err)
	assert.Len(t, got[ascii.ID].Name, 1024)
	assert.Empty(t, got[ascii.ID].Description, "descriptions are never loaded")
	assert.Nil(t, got[ascii.ID].Meta, "meta is never loaded")
}

func TestStoreReviewBoundsDisplayTextSQLite(t *testing.T) {
	testStoreReviewBoundsDisplayText(t, newReductionSQLiteContext(t))
}

func TestTrimDisplayText(t *testing.T) {
	cut := []byte(strings.Repeat("€", 367))[:1024]
	assert.Equal(t, strings.Repeat("€", 341), trimDisplayText(cut))
	assert.Equal(t, "ab", trimDisplayText([]byte("ab")))
	assert.Equal(t, "", trimDisplayText(nil))
	two := []byte(strings.Repeat("a", 1023) + "é")[:1024] // the first byte of a two-byte rune last
	assert.Equal(t, strings.Repeat("a", 1023), trimDisplayText(two))
	whole := []byte(strings.Repeat("a", 1022) + "é")[:1024]
	assert.Equal(t, strings.Repeat("a", 1022)+"é", trimDisplayText(whole), "a rune that ends at the bound is kept")
}

func testStoreReviewPageBudget(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	store := ctx.reductionStore()
	next := uint(1)
	add := func(key string, n int, readOnly bool) {
		ids := sequentialIDs(next, n)
		next += uint(n)
		require.NoError(t, store.insertCluster(r, "gen", &storedCluster{
			Row: models.ReductionClusterRow{Key: key, Tier: "identical", State: "open", WinnerID: ids[0], Position: int64(next), ReadOnly: readOnly}, Members: memberRows(ids...)}))
	}
	for i := 0; i < 10; i++ { // 10 x 200 = the 2,000 budget
		add(fmt.Sprintf("a%02d", i), 200, false)
	}
	for i := 0; i < 5; i++ { // up to 12 always render in full
		add(fmt.Sprintf("b%02d", i), 12, false)
	}
	add("c00", 13, false) // past the budget: its Winner and a cursor
	add("c01", 201, true) // a legacy over-cap Cluster: also past the budget

	q := &query_models.ResourceReductionReviewQuery{Sort: "computed", PageSize: 50}
	review, extras, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, q)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 17)
	byKey := map[string]int{}
	for i, v := range review.Clusters {
		byKey[v.ID] = i
	}
	total := 0
	for _, v := range review.Clusters {
		total += len(v.Members)
	}
	assert.Equal(t, 10*200+5*12+2, total, "the page holds the budget, the small Clusters and two Winners")
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("a%02d", i)
		view := review.Clusters[byKey[key]]
		assert.Len(t, view.Members, 200, key)
		assert.False(t, extras.Clusters[key].MoreMembers, key)
	}
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("b%02d", i)
		assert.Len(t, review.Clusters[byKey[key]].Members, 12, key)
		assert.False(t, extras.Clusters[key].MoreMembers, key)
	}
	for _, key := range []string{"c00", "c01"} {
		view := review.Clusters[byKey[key]]
		require.Len(t, view.Members, 1, key)
		assert.True(t, view.Members[0].IsWinner, key)
		assert.True(t, extras.Clusters[key].MoreMembers, key)
		assert.Equal(t, "", extras.Clusters[key].MembersAfter, key+": the cursor starts at the first member")
	}
	assert.Equal(t, 13, extras.Clusters["c00"].MemberCount)
	assert.Equal(t, 1, extras.Clusters["c00"].MembersShown)
	assert.Equal(t, 12, review.Clusters[byKey["c00"]].LoserCount, "counts come from the stored columns, not the loaded members")

	// The cursor walks a Cluster's members in (position, id) order, 200 at a time.
	var clusterID uint
	require.NoError(t, ctx.db.Raw(`SELECT id FROM reduction_clusters WHERE reduction_id = ? AND "key" = ?`, r.ID, "c01").Scan(&clusterID).Error)
	first, err := store.memberPage(clusterID, startOfMembers(), reductionMemberCap)
	require.NoError(t, err)
	require.Len(t, first, 200)
	cursor, err := parseReductionMemberCursor(reductionMemberCursor{Position: first[199].Position, ID: first[199].ID}.String())
	require.NoError(t, err)
	rest, err := store.memberPage(clusterID, cursor, reductionMemberCap)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Equal(t, 200, rest[0].Position)
	_, err = parseReductionMemberCursor("garbage")
	assert.Error(t, err)
}

func TestStoreReviewPageBudgetSQLite(t *testing.T) {
	testStoreReviewPageBudget(t, newReductionSQLiteContext(t))
}

func testStoreReviewWinnerBeyondTheFirstMembers(t *testing.T, ctx *MahresourcesContext) {
	r := tableReduction(t, ctx)
	ids := sequentialIDs(1, 250)
	winner := ids[240] // a promotion can make any member the Winner
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "big", Tier: "identical", State: "open", WinnerID: winner, ReadOnly: true, Truncated: 7}, Members: memberRows(ids...)}))

	review, extras, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	view := review.Clusters[0]
	require.Len(t, view.Members, 201, "the first 200 and the Winner on its own")
	assert.True(t, view.Members[0].IsWinner)
	assert.Equal(t, winner, view.Members[0].ResourceID)
	assert.Equal(t, 249, view.LoserCount)
	assert.True(t, extras.Clusters["big"].MoreMembers)
	assert.NotEmpty(t, extras.Clusters["big"].MembersAfter)
	assert.Equal(t, 7, extras.Clusters["big"].Truncated, "an unscoped viewer is told copies were left out")
}

func TestStoreReviewWinnerBeyondTheFirstMembersSQLite(t *testing.T) {
	testStoreReviewWinnerBeyondTheFirstMembers(t, newReductionSQLiteContext(t))
}

func testStoreReviewHidesTruncationFromAScopedViewer(t *testing.T, ctx *MahresourcesContext) {
	home := &models.Group{Name: "home"}
	require.NoError(t, ctx.db.Create(home).Error)
	var ids []uint
	for i := 0; i < 3; i++ {
		res := &models.Resource{Name: fmt.Sprintf("m%d", i), Hash: "h", OwnerId: &home.ID}
		require.NoError(t, ctx.db.Create(res).Error)
		ids = append(ids, res.ID)
	}
	r := tableReduction(t, ctx)
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "star", Tier: "near", State: "open", WinnerID: ids[0], Truncated: 5, FixedSeed: true, SeedResourceID: ids[0], DecidedBy: "seed"}, Members: memberRows(ids...)}))

	admin, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, admin.Clusters, 1)
	assert.Equal(t, "kept: the copy the others were matched to", admin.Clusters[0].DecidedByLabel, "a full star's seed is justified as the copy the others were matched to")

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &home.ID})
	scopedReview, extras, err := scoped.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Len(t, scopedReview.Clusters, 1, "the scoped viewer sees the Cluster, so a zero below is a hidden count and not a missing entry")
	scopedStar, present := extras.Clusters["star"]
	require.True(t, present, "the Cluster has an entry in the extras")
	assert.Equal(t, 3, scopedStar.MemberCount)
	assert.Zero(t, scopedStar.Truncated, "omitted copies may lie outside a scoped viewer's scope")
	_, extras, err = ctx.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	require.Contains(t, extras.Clusters, "star")
	assert.Equal(t, 5, extras.Clusters["star"].Truncated)

	// A scoped viewer whose scope misses a member sees no Cluster, no stub and no count.
	elsewhere := &models.Group{Name: "elsewhere"}
	require.NoError(t, ctx.db.Create(elsewhere).Error)
	other := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 6, ScopeGroupID: &elsewhere.ID})
	review, _, err := other.storeGetReductionReview(r.ID, nil, false, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, review.Clusters)
	assert.Zero(t, review.ClusterCount)
}

func TestStoreReviewHidesTruncationFromAScopedViewerSQLite(t *testing.T) {
	testStoreReviewHidesTruncationFromAScopedViewer(t, newReductionSQLiteContext(t))
}
