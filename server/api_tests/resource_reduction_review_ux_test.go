package api_tests

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

func reviewOf(t *testing.T, tc *TestContext, id uint, page int, q *query_models.ResourceReductionReviewQuery) *contracts.ReductionReview {
	t.Helper()
	owner, restricted := asAdmin()
	review, err := tc.AppCtx.GetReductionReview(id, owner, restricted, page, q)
	require.NoError(t, err)
	return review
}

func clusterIDs(review *contracts.ReductionReview) []string {
	out := make([]string, 0, len(review.Clusters))
	for _, c := range review.Clusters {
		out = append(out, c.ID)
	}
	return out
}

// pairOf makes one Identical Cluster of n members sharing `hash`.
func clusterOf(t *testing.T, tc *TestContext, hash string, n int) []uint {
	t.Helper()
	ids := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		r := addWithHash(t, tc, fmt.Sprintf("%s-%d.txt", hash, i), fmt.Sprintf("%s body %d", hash, i), hash)
		ids = append(ids, r.ID)
	}
	return ids
}

// Most copies first is the default, then the largest total file size, then the plan's
// own order. Ties break by Cluster id so a refresh never reshuffles pages.
func TestReviewSortsByCopiesSizeAndComputedOrder(t *testing.T) {
	tc := SetupTestEnv(t)

	two := clusterOf(t, tc, "hash-two", 2)
	three := clusterOf(t, tc, "hash-three", 3)
	twoToo := clusterOf(t, tc, "hash-two-too", 2)
	// The two-copy Clusters hold the biggest files, the three-copy one the smallest.
	for _, id := range three {
		setFileSize(t, tc, id, 10)
	}
	for _, id := range two {
		setFileSize(t, tc, id, 5000)
	}
	for _, id := range twoToo {
		setFileSize(t, tc, id, 100)
	}

	all := append(append(append([]uint{}, two...), three...), twoToo...)
	red := createReduction(t, tc, "Sorting", all)
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 3)

	idOf := func(members []uint) string { return clusterFor(plan, members[0]).ID }

	byCopies := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{})
	got := clusterIDs(byCopies)
	require.Len(t, got, 3)
	assert.Equal(t, idOf(three), got[0], "the three-copy Cluster comes first by default")
	tail := []string{idOf(two), idOf(twoToo)}
	sort.Strings(tail)
	assert.Equal(t, tail, got[1:], "ties on copy count break by Cluster id")

	explicit := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Sort: "copies"})
	assert.Equal(t, got, clusterIDs(explicit))

	bySize := clusterIDs(reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Sort: "size"}))
	assert.Equal(t, []string{idOf(two), idOf(twoToo), idOf(three)}, bySize,
		"10000 bytes, 200 bytes, 30 bytes")

	var computed []string
	for _, c := range plan.Clusters {
		computed = append(computed, c.ID)
	}
	assert.Equal(t, computed, clusterIDs(reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Sort: "computed"})))

	// An unknown sort is the default rather than an error.
	assert.Equal(t, got, clusterIDs(reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Sort: "nonsense"})))
}

func TestReviewPageSizeIsOneOfTheOffered(t *testing.T) {
	tc := SetupTestEnv(t)
	var all []uint
	for i := 0; i < 23; i++ {
		all = append(all, clusterOf(t, tc, fmt.Sprintf("bulk-%02d", i), 2)...)
	}
	red := createReduction(t, tc, "Many", all)
	computeReduction(t, tc, red.ID)

	for _, c := range []struct{ asked, want, onPage1 int }{
		{0, 20, 20}, {7, 20, 20}, {20, 20, 20}, {50, 50, 23}, {100, 100, 23}, {101, 20, 20},
	} {
		review := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{PageSize: c.asked})
		assert.Equal(t, c.want, review.PageSize, "asked %d", c.asked)
		assert.Len(t, review.Clusters, c.onPage1, "asked %d", c.asked)
		assert.Equal(t, 23, review.ClusterCount)
	}
	page2 := reviewOf(t, tc, red.ID, 2, &query_models.ResourceReductionReviewQuery{})
	assert.Len(t, page2.Clusters, 3)
}

// Approval is the Checked flag: Identical Clusters arrive approved, Near-Identical
// ones do not.
func TestReviewFiltersByApproval(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "approval-a", 2)
	b := clusterOf(t, tc, "approval-b", 2)
	red := createReduction(t, tc, "Approval", append(append([]uint{}, a...), b...))
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, clusterFor(plan, b[0]).ID, application_context.ReductionActionUncheck, 0)

	approved := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Approval: "approved"})
	require.Len(t, approved.Clusters, 1)
	assert.Equal(t, clusterFor(plan, a[0]).ID, approved.Clusters[0].ID)

	unapproved := reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Approval: "unapproved"})
	require.Len(t, unapproved.Clusters, 1)
	assert.Equal(t, clusterFor(plan, b[0]).ID, unapproved.Clusters[0].ID)

	assert.Len(t, reviewOf(t, tc, red.ID, 1, &query_models.ResourceReductionReviewQuery{Approval: "bogus"}).Clusters, 2)
}

// Per-tab counts cover every status, whatever the current filter is, and only the
// Clusters the caller may fully see: a withheld Cluster is in none of them.
func TestReviewTabCountsAreOverVisibleClustersOnly(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	visibleA := clusterOf(t, tc, "tabs-visible-a", 2)
	visibleB := clusterOf(t, tc, "tabs-visible-b", 2)
	hidden := clusterOf(t, tc, "tabs-hidden", 2)
	for _, id := range append(append([]uint{}, visibleA...), visibleB...) {
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("owner_id", narrow.ID).Error)
	}
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[0]).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[1]).Update("owner_id", wide.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Tabs", ResourceIds: append(append(append([]uint{}, visibleA...), visibleB...), hidden...),
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 3)
	override(t, tc, red.ID, clusterFor(plan, visibleB[0]).ID, application_context.ReductionActionSkip, 0)
	override(t, tc, red.ID, clusterFor(plan, hidden[0]).ID, application_context.ReductionActionSkip, 0)

	adminReview := reviewOf(t, tc, red.ID, 1, nil)
	assert.Equal(t, contracts.ReductionTabCounts{Open: 1, Skipped: 2}, adminReview.TabCounts)

	narrowCtx := scopedTo(t, tc, narrow.ID)
	review, err := narrowCtx.GetReductionReview(red.ID, owner, restricted, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"skipped"}})
	require.NoError(t, err)
	assert.Equal(t, contracts.ReductionTabCounts{Open: 1, Skipped: 1}, review.TabCounts,
		"the hidden skipped Cluster is not counted, and the counts ignore the status filter")
	assert.Equal(t, 1, review.ClusterCount)
}

// A member's card facts: dimensions, size, type and date come from the Resource;
// tags, notes and groups are counted per member.
func TestReviewMembersCarryTheirAssociationCounts(t *testing.T) {
	tc := SetupTestEnv(t)
	members := clusterOf(t, tc, "facts", 2)
	red := createReduction(t, tc, "Facts", members)
	computeReduction(t, tc, red.ID)

	tag := &models.Tag{Name: "facts-tag"}
	require.NoError(t, tc.DB.Create(tag).Error)
	note := &models.Note{Name: "facts-note"}
	require.NoError(t, tc.DB.Create(note).Error)
	group, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "facts-group"})
	require.NoError(t, err)
	require.NoError(t, tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?)", members[0], tag.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", members[0], note.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)", members[0], group.ID).Error)

	review := reviewOf(t, tc, red.ID, 1, nil)
	require.Len(t, review.Clusters, 1)
	counts := map[uint][3]int{}
	for _, m := range review.Clusters[0].Members {
		counts[m.ResourceID] = [3]int{m.TagCount, m.NoteCount, m.GroupCount}
	}
	assert.Equal(t, [3]int{1, 1, 1}, counts[members[0]])
	assert.Equal(t, [3]int{0, 0, 0}, counts[members[1]])
}

// The keeps-smaller warning names the two resolutions it compares and the copy its
// fix button would keep instead.
func TestReviewNamesTheKeepsSmallerDimensions(t *testing.T) {
	tc := SetupTestEnv(t)
	large := addWithHash(t, tc, "large.txt", "large body", "dims-hash")
	small := addWithHash(t, tc, "small.txt", "small body", "dims-hash")
	setDimensions(t, tc, large.ID, 1600, 1066)
	setDimensions(t, tc, small.ID, 800, 533)
	red := createReduction(t, tc, "Dims", []uint{large.ID, small.ID})
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, small.ID)

	view := reviewOf(t, tc, red.ID, 1, nil).Clusters[0]
	assert.True(t, view.KeepsSmaller)
	assert.True(t, view.NeedsAttention)
	assert.Equal(t, "800×533", view.KeptDimensions)
	assert.Equal(t, "1600×1066", view.LargerDimensions)
	assert.Equal(t, large.ID, view.LargestMemberID)
	assert.Equal(t, "you chose it", view.DecidedByLabel)
}

// Settings the plan was computed with are recorded, and a later difference is
// reported; a plan from before they were recorded reports nothing.
func TestReviewReportsSettingsChangedSinceCompute(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	members := clusterOf(t, tc, "settings", 2)
	red := createReduction(t, tc, "Settings", members)
	plan := computeReduction(t, tc, red.ID)

	require.NotNil(t, plan.Settings, "compute records what it ran with")
	assert.Equal(t, models.MatchingModeBothTiers, plan.Settings.MatchingMode)
	assert.Equal(t, models.DefaultWinnerRule(), plan.Settings.WinnerRule)
	assert.False(t, plan.Settings.ExcludeExternal)
	assert.False(t, reviewOf(t, tc, red.ID, 1, nil).SettingsChanged)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	// The keep-as-version flags act at apply, so changing them is not a change.
	flag := true
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: red.ID, Version: current.Version, KeepAsVersionIdentical: &flag,
	}, owner, restricted)
	require.NoError(t, err)
	assert.False(t, reviewOf(t, tc, red.ID, 1, nil).SettingsChanged)

	current, err = tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: red.ID, Version: current.Version, WinnerRule: []string{models.WinnerCriterionSizeAsc},
	}, owner, restricted)
	require.NoError(t, err)
	assert.True(t, reviewOf(t, tc, red.ID, 1, nil).SettingsChanged)

	current, err = tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: red.ID, Version: current.Version, WinnerRule: models.DefaultWinnerRule(), MatchingMode: models.MatchingModeIdenticalOnly,
	}, owner, restricted)
	require.NoError(t, err)
	assert.True(t, reviewOf(t, tc, red.ID, 1, nil).SettingsChanged, "a different matching mode is a change too")

	current, err = tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	plan.Settings = nil
	require.NoError(t, tc.AppCtx.OverwritePlanForTest(red.ID, current.Version, plan))
	assert.False(t, reviewOf(t, tc, red.ID, 1, nil).SettingsChanged, "an old plan has nothing to compare")
}

// The apply dialog's figures are per tier over the approved, visible Clusters.
func TestReviewApplyFigures(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	twoCopies := clusterOf(t, tc, "fig-two", 2)
	threeCopies := clusterOf(t, tc, "fig-three", 3)
	skipped := clusterOf(t, tc, "fig-skipped", 2)
	near1 := addImage(t, tc, "near1.jpg", 800, 600)
	near2 := addImage(t, tc, "near2.jpg", 400, 300)
	pairThem(t, tc, near1, near2, 1)

	ids := append(append(append([]uint{near1.ID, near2.ID}, twoCopies...), threeCopies...), skipped...)
	red := createReduction(t, tc, "Figures", ids)
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, clusterFor(plan, skipped[0]).ID, application_context.ReductionActionSkip, 0)
	nearCluster := clusterFor(plan, near1.ID)
	override(t, tc, red.ID, nearCluster.ID, application_context.ReductionActionCheck, 0)
	// Keeping the smaller near copy by hand is the keeps-smaller case.
	override(t, tc, red.ID, nearCluster.ID, application_context.ReductionActionPromote, near2.ID)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	flag := true
	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: red.ID, Version: current.Version, KeepAsVersionIdentical: &flag,
	}, owner, restricted)
	require.NoError(t, err)

	figures := reviewOf(t, tc, red.ID, 1, nil).ApplyFigures
	assert.Equal(t, contracts.ReductionTierFigures{Clusters: 2, Losers: 3, KeepAsVersion: true}, figures.Identical)
	assert.Equal(t, contracts.ReductionTierFigures{Clusters: 1, Losers: 1, KeepAsVersion: true}, figures.Near,
		"the Near flag defaults on")
	assert.Equal(t, 1, figures.KeepsSmaller)

	bulk := reviewOf(t, tc, red.ID, 1, nil).Bulk
	assert.Equal(t, 2, bulk.IdenticalApprovable, "the skipped Cluster is not open")
	assert.Equal(t, 2, bulk.IdenticalOpen)
}
