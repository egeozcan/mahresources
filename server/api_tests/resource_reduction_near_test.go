package api_tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// addImage creates a Resource that looks perceptually hashable to the clustering:
// a raster content type and an image_hashes row.
//
// The bytes are not really a JPEG, and deliberately: nothing in this path decodes
// them. The clustering reads the stored pair table, which is the point of the
// design — it trusts the precomputed pairs and reports its coverage rather than
// hashing anything itself.
func addImage(t *testing.T, tc *TestContext, name string, width, height uint) *models.Resource {
	t.Helper()
	r := addResourceWithBody(t, tc, name, "bytes of "+name)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", r.ID).
		Updates(map[string]any{"content_type": "image/jpeg", "width": width, "height": height}).Error)
	id := r.ID
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &id, Status: models.HashStatusOK}).Error)
	r.ContentType = "image/jpeg"
	r.Width, r.Height = width, height
	return r
}

// pairThem stores a perceptual pair, lower id first, as the hash worker does.
func pairThem(t *testing.T, tc *TestContext, a, b *models.Resource, distance uint8) {
	t.Helper()
	first, second := a.ID, b.ID
	if first > second {
		first, second = second, first
	}
	d := distance
	require.NoError(t, tc.DB.Create(&models.ResourceSimilarity{
		ResourceID1:     first,
		ResourceID2:     second,
		HammingDistance: distance,
		PDistance:       &d,
	}).Error)
}

// The Near-Identical tier clusters from the stored pair table, and its Clusters
// arrive unchecked: perceptual similarity is a guess, so the friction sits where
// the risk is.
func TestNearIdenticalClustersFromStoredPairs(t *testing.T) {
	tc := SetupTestEnv(t)

	best := addImage(t, tc, "best.jpg", 800, 800)
	worse := addImage(t, tc, "worse.jpg", 400, 400)
	unrelated := addImage(t, tc, "unrelated.jpg", 600, 600)
	pairThem(t, tc, best, worse, 3)

	red := createReduction(t, tc, "Near", []uint{best.ID, worse.ID, unrelated.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Equal(t, models.ReductionTierNear, cluster.Tier)
	assert.Equal(t, best.ID, cluster.WinnerID, "the seed order is the Winner Rule")
	assert.ElementsMatch(t, []uint{best.ID, worse.ID}, memberIDs(cluster))
	assert.False(t, cluster.Checked, "a Near-Identical Cluster arrives unchecked")

	assert.Equal(t, 3, plan.Coverage.PerceptualEligible)
	assert.Equal(t, 3, plan.Coverage.PerceptualHashed)
}

// Perceptual similarity is not transitive. A within threshold of B and B within
// threshold of C says nothing about A and C, so a Cluster must never be assembled
// by walking the edge list — every proposed deletion has to rest on a stored pair
// between that exact Loser and that exact Winner.
func TestNearIdenticalNeverChainsThroughATransitiveNeighbour(t *testing.T) {
	tc := SetupTestEnv(t)

	hub := addImage(t, tc, "hub.jpg", 800, 800)
	left := addImage(t, tc, "left.jpg", 400, 400)
	right := addImage(t, tc, "right.jpg", 200, 200)
	pairThem(t, tc, hub, left, 4)
	pairThem(t, tc, left, right, 4)
	// No hub-to-right pair: at the read threshold these two are not similar.

	red := createReduction(t, tc, "Chain", []uint{hub.ID, left.ID, right.ID})
	plan := computeReduction(t, tc, red.ID)

	cluster := clusterFor(plan, hub.ID)
	require.NotNil(t, cluster)
	assert.Equal(t, hub.ID, cluster.WinnerID)
	assert.NotContains(t, memberIDs(cluster), right.ID,
		"a transitive neighbour is not a member, however connected the edge list is")
	for _, member := range cluster.Members {
		if member.ResourceID == cluster.WinnerID {
			continue
		}
		require.NotNil(t, member.Distance, "every Loser carries its stored distance to this Winner")
	}
}

// Byte-identical images decode to identical pixels and are therefore also stored
// as distance-zero pairs, so the same Resources would otherwise appear in two
// Clusters with different defaults and possibly different Winners. When both
// apply, take the fact.
func TestTheTwoTiersAreDisjointAndIdenticalWins(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addImage(t, tc, "a.jpg", 800, 800)
	b := addImage(t, tc, "b.jpg", 400, 400)
	for _, id := range []uint{a.ID, b.ID} {
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("hash", "shared-hash").Error)
	}
	pairThem(t, tc, a, b, 0)

	red := createReduction(t, tc, "Both tiers", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1, "a Resource belongs to at most one Cluster per Reduction")
	assert.Equal(t, models.ReductionTierIdentical, plan.Clusters[0].Tier)
}

// A hub image genuinely within threshold of hundreds of others is one click from
// destroying all of them, and the review is this feature's only safety mechanism.
// So an oversized Near-Identical Cluster arrives unchecked and has to be expanded
// before it can be acted on.
func TestAnOversizedNearIdenticalClusterIsFlagged(t *testing.T) {
	tc := SetupTestEnv(t)

	hub := addImage(t, tc, "hub.jpg", 4000, 4000)
	ids := []uint{hub.ID}
	for i := 0; i < 14; i++ {
		neighbour := addImage(t, tc, fmt.Sprintf("near-%d.jpg", i), 100, 100)
		pairThem(t, tc, hub, neighbour, 5)
		ids = append(ids, neighbour.ID)
	}

	red := createReduction(t, tc, "Oversized", ids)
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.True(t, cluster.Oversized)
	assert.False(t, cluster.Checked)
}

// The same size is unremarkable on the Identical tier: fifty copies of one file
// are fifty copies of one file, and byte-identity is a fact rather than a guess.
func TestALargeIdenticalClusterIsNotTreatedAsSuspicious(t *testing.T) {
	tc := SetupTestEnv(t)

	var ids []uint
	for i := 0; i < 20; i++ {
		r := addWithHash(t, tc, fmt.Sprintf("copy-%d.txt", i), fmt.Sprintf("body %d", i), "shared-hash")
		ids = append(ids, r.ID)
	}

	red := createReduction(t, tc, "Numerous", ids)
	plan := computeReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.Len(t, cluster.Members, 20)
	assert.False(t, cluster.Oversized)
	assert.True(t, cluster.Checked)
}

// A pair beyond the read threshold is not a match. The net cannot be widened past
// what the pair table holds, and the clustering asks the same question the rest of
// the app does.
func TestAPairBeyondTheThresholdIsNotClustered(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addImage(t, tc, "a.jpg", 800, 800)
	b := addImage(t, tc, "b.jpg", 400, 400)
	pairThem(t, tc, a, b, 11)

	red := createReduction(t, tc, "Too far", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)

	assert.Empty(t, plan.Clusters)
}
