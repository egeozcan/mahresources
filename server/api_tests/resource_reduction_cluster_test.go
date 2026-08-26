package api_tests

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// computeReduction asks for the clustering and waits for it.
//
// Clustering is never synchronous — even the Identical tier is a GROUP BY over
// however much of the library the Extent reaches — so the tests wait on the row
// the way the page does rather than reaching past the queue.
func computeReduction(t *testing.T, tc *TestContext, id uint) models.ResourceReductionPlan {
	t.Helper()
	owner, restricted := asAdmin()
	current, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.RequestReductionCompute(id, current.Version, owner, restricted, nil)
	require.NoError(t, err)
	return awaitReduction(t, tc, id)
}

// awaitReduction polls until the Reduction leaves `computing`.
//
// A read here can lose to the clustering job's own write and come back
// "database table is locked". That is this fixture's shared cache, not the
// feature: SQLITE_LOCKED is a shared-cache table lock, which does not go through
// busy_timeout and does not exist in the WAL configuration the app actually runs
// under, where a reader never blocks on a writer. The poll retries it; a failure
// that is not contention still fails the test immediately.
func awaitReduction(t *testing.T, tc *TestContext, id uint) models.ResourceReductionPlan {
	t.Helper()
	owner, restricted := asAdmin()
	deadline := time.Now().Add(20 * time.Second)
	for {
		red, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
		if err != nil && strings.Contains(err.Error(), "is locked") && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		require.NoError(t, err)
		if red.Status != models.ReductionStatusComputing {
			require.Equal(t, models.ReductionStatusReady, red.Status, "clustering failed: %s", red.ComputeError)
			plan, err := application_context.DecodeReductionPlan(red.Plan)
			require.NoError(t, err)
			return plan
		}
		if time.Now().After(deadline) {
			t.Fatalf("clustering did not finish: still %s", red.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func clusterFor(plan models.ResourceReductionPlan, resourceID uint) *models.ReductionCluster {
	for _, cluster := range plan.Clusters {
		for _, member := range cluster.Members {
			if member.ResourceID == resourceID {
				return cluster
			}
		}
	}
	return nil
}

func memberIDs(cluster *models.ReductionCluster) []uint {
	ids := make([]uint, 0, len(cluster.Members))
	for _, m := range cluster.Members {
		ids = append(ids, m.ResourceID)
	}
	return ids
}

// addWithHash creates a Resource whose content hash is `hash`.
//
// It cannot be done by uploading the same bytes twice: AddResource deduplicates
// on content hash at create time and hands back the existing Resource. Two rows
// sharing a hash are still an ordinary state of this database — a version upload
// rewrites resources.hash without deduplicating, which is exactly how the
// production case arises — so the fixture writes the column directly rather than
// pretending the create path could produce it.
func addWithHash(t *testing.T, tc *TestContext, name, body, hash string) *models.Resource {
	t.Helper()
	r := addResourceWithBody(t, tc, name, body)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", r.ID).Update("hash", hash).Error)
	r.Hash = hash
	return r
}

// setDimensions gives a Resource a resolution the default Winner Rule can rank.
// AddResource stores no dimensions for a plain text body, and the rule's first
// criterion is pixel count.
func setDimensions(t *testing.T, tc *TestContext, id uint, width, height uint) {
	t.Helper()
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).
		Updates(map[string]any{"width": width, "height": height}).Error)
}

// Resources sharing a content hash form one Cluster, and it arrives checked:
// byte-identity is a fact, so the friction belongs on the other tier.
func TestIdenticalClusterGroupsByContentHash(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	c := addResourceWithBody(t, tc, "c.txt", "different bytes")

	red := createReduction(t, tc, "Identical", []uint{a.ID, b.ID, c.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1, "only the two sharing a hash form a Cluster")
	cluster := plan.Clusters[0]
	assert.Equal(t, models.ReductionTierIdentical, cluster.Tier)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, memberIDs(cluster))
	assert.True(t, cluster.Checked, "an Identical Cluster arrives checked")
	assert.False(t, cluster.Oversized, "the size cap is a Near-Identical rule only")
}

// The empty content hash is a live value in this schema, so a plain GROUP BY
// would collapse every hashless Resource into one Cluster — arriving checked, and
// proposing to delete files with nothing to do with each other.
func TestIdenticalClusteringExcludesTheEmptyHash(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	b := addResourceWithBody(t, tc, "b.txt", "bbb")
	c := addResourceWithBody(t, tc, "c.txt", "ccc")
	for _, id := range []uint{a.ID, b.ID, c.ID} {
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("hash", "").Error)
	}

	red := createReduction(t, tc, "Hashless", []uint{a.ID, b.ID, c.ID})
	plan := computeReduction(t, tc, red.ID)

	assert.Empty(t, plan.Clusters, "hashless Resources are reported in coverage, never clustered")
	assert.Equal(t, 3, plan.Coverage.ExtentSize)
	assert.Equal(t, 0, plan.Coverage.ContentHashed, "and the coverage line says why nothing was found")
}

// The Winner Rule's first criterion decides, and the page is told which one did
// and by how much — the difference between "the rule picked this" and "the rule
// picked this by four times the resolution".
func TestWinnerRuleReportsWhichCriterionDecidedAndByHowMuch(t *testing.T) {
	tc := SetupTestEnv(t)

	small := addWithHash(t, tc, "small.txt", "small body", "shared-hash")
	large := addWithHash(t, tc, "large.txt", "large body", "shared-hash")
	setDimensions(t, tc, small.ID, 100, 100)
	setDimensions(t, tc, large.ID, 400, 100)

	red := createReduction(t, tc, "Resolution", []uint{small.ID, large.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, large.ID, cluster.WinnerID, "the higher resolution wins under the default rule")
	assert.Equal(t, models.WinnerCriterionPixelsDesc, cluster.DecidedBy)
	assert.Equal(t, "4x the pixels", cluster.Margin)
	assert.False(t, cluster.Undecided)
}

// A criterion with no discriminating power falls through to the next, so a rule
// mentioning resolution still behaves sensibly for content types that have none.
func TestACriterionThatCannotDiscriminateFallsThrough(t *testing.T) {
	tc := SetupTestEnv(t)

	// Neither carries dimensions, so pixel count cannot decide. File size can:
	// the same content hash with a different recorded size is contrived, but it
	// is exactly the fall-through the rule promises.
	first := addWithHash(t, tc, "first.txt", "first body", "shared-hash")
	second := addWithHash(t, tc, "second.txt", "second body", "shared-hash")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", first.ID).Update("file_size", 10).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", second.ID).Update("file_size", 999).Error)

	red := createReduction(t, tc, "Fallthrough", []uint{first.ID, second.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, second.ID, cluster.WinnerID)
	assert.Equal(t, models.WinnerCriterionSizeDesc, cluster.DecidedBy, "pixel count could not tell them apart")
}

// A Cluster where every criterion tied says so, rather than presenting a
// tiebreaker of last resort as a decision.
func TestAClusterNoCriterionCouldDecideSaysSo(t *testing.T) {
	tc := SetupTestEnv(t)

	first := addWithHash(t, tc, "first.txt", "first body", "shared-hash")
	second := addWithHash(t, tc, "second.txt", "other body", "shared-hash")
	// No dimensions on either, equal sizes, and one creation instant shared, so
	// all three criteria of the default rule tie.
	sameInstant := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{first.ID, second.ID}).
		Updates(map[string]any{"created_at": sameInstant, "file_size": 10}).Error)

	red := createReduction(t, tc, "Undecided", []uint{first.ID, second.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.True(t, cluster.Undecided, "every criterion tied")
	assert.Equal(t, models.UndecidedCriterion, cluster.DecidedBy)
	assert.Equal(t, first.ID, cluster.WinnerID, "the tiebreaker of last resort is lowest id")
}

// A Reduction over a parent Group covers the subtree the reviewer means.
func TestExtentExpandsThroughGroupDescendants(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	parent, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Holidays"})
	require.NoError(t, err)
	child, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Holidays 2019", OwnerId: parent.ID})
	require.NoError(t, err)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{a.ID, b.ID}).
		Update("owner_id", child.ID).Error)

	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:     "Everything under Holidays",
		GroupIds: []uint{parent.ID},
	}, owner, restricted)
	require.NoError(t, err)

	plan := computeReduction(t, tc, red.ID)
	assert.Equal(t, 2, plan.Coverage.ExtentSize, "a descendant Group's Resources are in the Extent")
	require.Len(t, plan.Clusters, 1)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, memberIDs(plan.Clusters[0]))
}

// A better copy sitting outside the Extent is shown, and it wins. That is the
// whole reason a Cluster is allowed to reach outside the selection at all.
func TestABetterCopyOutsideTheExtentWins(t *testing.T) {
	tc := SetupTestEnv(t)

	inside := addWithHash(t, tc, "inside.txt", "inside body", "shared-hash")
	outside := addWithHash(t, tc, "outside.txt", "outside body", "shared-hash")
	setDimensions(t, tc, inside.ID, 100, 100)
	setDimensions(t, tc, outside.ID, 400, 400)

	red := createReduction(t, tc, "Reaching out", []uint{inside.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, outside.ID, cluster.WinnerID, "the better copy elsewhere wins")
	assert.ElementsMatch(t, []uint{inside.ID, outside.ID}, memberIDs(cluster))
	assert.NotContains(t, cluster.LoserIDs(), outside.ID, "nothing outside the Extent is ever destroyed")
}

// A worse copy outside the Extent is left out of the Cluster entirely. It may
// never lose, and forcing it to win would merge the reviewer's own Resources into
// something they did not select.
func TestAWorseCopyOutsideTheExtentIsNotInTheCluster(t *testing.T) {
	tc := SetupTestEnv(t)

	insideBig := addWithHash(t, tc, "inside-big.txt", "big body", "shared-hash")
	insideSmall := addWithHash(t, tc, "inside-small.txt", "small body", "shared-hash")
	outside := addWithHash(t, tc, "outside.txt", "outside body", "shared-hash")
	setDimensions(t, tc, insideBig.ID, 400, 400)
	setDimensions(t, tc, insideSmall.ID, 200, 200)
	setDimensions(t, tc, outside.ID, 100, 100)

	red := createReduction(t, tc, "Worse outside", []uint{insideBig.ID, insideSmall.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, insideBig.ID, cluster.WinnerID)
	assert.ElementsMatch(t, []uint{insideBig.ID, insideSmall.ID}, memberIDs(cluster))
	assert.NotContains(t, memberIDs(cluster), outside.ID)
}

// Two out-of-Extent members cannot both win and neither may lose, so the Cluster
// as computed would be unsatisfiable. The best of them wins; the rest are not in
// the Cluster at all.
func TestAClusterHoldsAtMostOneOutOfExtentMember(t *testing.T) {
	tc := SetupTestEnv(t)

	inside := addWithHash(t, tc, "inside.txt", "inside body", "shared-hash")
	outsideBest := addWithHash(t, tc, "outside-best.txt", "best body", "shared-hash")
	outsideAlso := addWithHash(t, tc, "outside-also.txt", "also body", "shared-hash")
	setDimensions(t, tc, inside.ID, 100, 100)
	setDimensions(t, tc, outsideBest.ID, 800, 800)
	setDimensions(t, tc, outsideAlso.ID, 400, 400)

	red := createReduction(t, tc, "Two outside", []uint{inside.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, outsideBest.ID, cluster.WinnerID)
	assert.ElementsMatch(t, []uint{inside.ID, outsideBest.ID}, memberIDs(cluster))
	assert.Equal(t, []uint{inside.ID}, cluster.LoserIDs())
}

// A Loser holding a description the Winner lacks is the curated copy about to be
// thrown away, and merge drops all three of these fields silently.
func TestAClusterIsFlaggedWhenAMergeWouldLoseCuration(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)
	setDimensions(t, tc, loser.ID, 100, 100)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", loser.ID).
		Update("description", "the one somebody actually wrote about").Error)

	red := createReduction(t, tc, "Lossy", []uint{winner.ID, loser.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	assert.Contains(t, plan.Clusters[0].Lossy, "description")
}

// Every member's content hash is recorded at compute time. That snapshot is what
// makes staleness detectable at all: a version upload rewrites resources.hash and
// leaves the similarity pairs untouched, so the pair table cannot report that the
// reviewed bytes are gone.
func TestThePlanRecordsEachMembersHashAtComputeTime(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red := createReduction(t, tc, "Snapshot", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	for _, member := range plan.Clusters[0].Members {
		assert.Equal(t, "shared-hash", member.Hash, "the hash each member held at compute time")
	}
}

// Coverage distinguishes "no repeats found" from "nothing was hashed". A library
// of text files has no perceptual hashes and that is correct, not a fault.
func TestCoverageReportsWhatCouldBeExamined(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	b := addResourceWithBody(t, tc, "b.txt", "bbb")

	red := createReduction(t, tc, "Coverage", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	assert.Equal(t, 2, plan.Coverage.ExtentSize)
	assert.Equal(t, 2, plan.Coverage.ContentHashed)
	assert.Equal(t, 0, plan.Coverage.PerceptualEligible, "text files can never carry a perceptual hash")
	assert.Equal(t, 0, plan.Coverage.PerceptualHashed)
}

// Identical-only is a real mode, not half of the default one: it covers video,
// PDF and audio, which have no perceptual hash at all.
func TestIdenticalOnlyModeSkipsThePerceptualTier(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:         "Cheap sweep",
		ResourceIds:  []uint{a.ID, b.ID},
		MatchingMode: models.MatchingModeIdenticalOnly,
	}, owner, restricted)
	require.NoError(t, err)

	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)
	assert.Equal(t, models.ReductionTierIdentical, plan.Clusters[0].Tier)
}

// A finished plan is refused, not rebased, when the Reduction moved under it.
//
// The plan describes the Extent, the Winner Rule and the decisions as they were
// when the run began. Landing it on a row that has since been widened would
// present a stale proposal as current, with a fresh computed_at — so the page's
// own drift line would report zero and the reviewer would have no way to tell.
func TestAComputeThatLostItsRowIsRefusedRatherThanRebased(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	a := addWithHash(t, tc, "a.txt", "a body", "shared-hash")
	b := addWithHash(t, tc, "b.txt", "b body", "shared-hash")

	red := createReduction(t, tc, "Moved under it", []uint{a.ID, b.ID})
	computeReduction(t, tc, red.ID)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	stalePlan, err := application_context.DecodeReductionPlan(current.Plan)
	require.NoError(t, err)

	// Something widens the Extent while the run is notionally still going.
	c := addWithHash(t, tc, "c.txt", "c body", "shared-hash")
	_, err = tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID:          red.ID,
		ResourceIds: []uint{c.ID},
	}, owner, restricted)
	require.NoError(t, err)

	err = tc.AppCtx.StoreReductionPlanForTest(red.ID, current.ComputeJobID, current.Version, stalePlan)
	assert.ErrorIs(t, err, application_context.ErrReductionStaleCompute)
}

// A single-Resource Extent still finds the better copy sitting outside it. That
// reach is the whole reason a Cluster is allowed outside the Extent at all, and
// "is there already a better copy of this?" is the question a reviewer asks about
// one photograph.
func TestASingleResourceExtentStillReachesOutside(t *testing.T) {
	tc := SetupTestEnv(t)

	only := addImage(t, tc, "only.jpg", 200, 200)
	better := addImage(t, tc, "better.jpg", 900, 900)
	pairThem(t, tc, only, better, 4)

	red := createReduction(t, tc, "One Resource", []uint{only.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, better.ID, cluster.WinnerID)
	assert.Equal(t, []uint{only.ID}, cluster.LoserIDs())
}

// The out-of-Extent rule can move the Winner off the greedy-star seed, and ADR
// 0002's pair-justification holds only while the Winner is the seed. A member with
// no pair to the outsider that displaced it is ejected at cluster time, not left
// for apply to refuse.
func TestAnOutsiderWinnerEjectsMembersItHasNoPairTo(t *testing.T) {
	tc := SetupTestEnv(t)

	seed := addImage(t, tc, "seed.jpg", 400, 400)
	sibling := addImage(t, tc, "sibling.jpg", 200, 200)
	outsider := addImage(t, tc, "outsider.jpg", 900, 900)
	pairThem(t, tc, seed, sibling, 4)
	pairThem(t, tc, seed, outsider, 4)
	// No outsider-to-sibling pair.

	red := createReduction(t, tc, "Outsider wins", []uint{seed.ID, sibling.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, outsider.ID, cluster.WinnerID)
	assert.Equal(t, []uint{seed.ID}, cluster.LoserIDs(),
		"the sibling has no stored pair to the Winner, so it is not proposed for deletion")

	ejected := memberOf(cluster, sibling.ID)
	require.NotNil(t, ejected)
	assert.True(t, ejected.Ejected)
	assert.Equal(t, models.EjectReasonNoPairToWinner, ejected.EjectedReason)
}
