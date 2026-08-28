package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// A Cluster is open until something terminal happens to it, and "reviewed" is a
// judgement recorded on an open one. The filter must tell the two apart.
func reviewCluster(tier string, state string, reviewed bool) *models.ReductionCluster {
	return &models.ReductionCluster{
		ID:       tier + "-" + state + "-" + itoaBool(reviewed),
		Tier:     tier,
		State:    state,
		Reviewed: reviewed,
	}
}

func itoaBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// reviewClusterWithMember attaches one member Resource, so reachability can be
// exercised: the filter's first step is deciding which Clusters the caller may
// fully see.
func reviewClusterWithMember(tier, state string, memberID uint) *models.ReductionCluster {
	c := reviewCluster(tier, state, false)
	c.WinnerID = memberID
	c.Members = []*models.ReductionMember{{ResourceID: memberID}}
	return c
}

// The status filter follows the labels the page shows: open, reviewed, skipped,
// applied, stale — where reviewed is an open Cluster that has been acted on.
func TestFilterReviewClustersByStatus(t *testing.T) {
	clusters := []*models.ReductionCluster{
		reviewCluster(models.ReductionTierIdentical, models.ReductionClusterOpen, false),
		reviewCluster(models.ReductionTierIdentical, models.ReductionClusterOpen, true),
		reviewCluster(models.ReductionTierNear, models.ReductionClusterSkipped, false),
		reviewCluster(models.ReductionTierNear, models.ReductionClusterApplied, false),
		reviewCluster(models.ReductionTierIdentical, models.ReductionClusterStale, false),
	}
	visible := map[uint]bool{}

	got := filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterSkipped},
	})
	require.Len(t, got, 1)
	assert.Equal(t, models.ReductionClusterSkipped, got[0].State)

	got = filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{"open"},
	})
	require.Len(t, got, 1)
	assert.False(t, got[0].Reviewed, "\"open\" selects only the untouched open Cluster")

	got = filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{"reviewed"},
	})
	require.Len(t, got, 1)
	assert.True(t, got[0].Reviewed, "\"reviewed\" selects the open Cluster that has been acted on")

	got = filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{"open", models.ReductionClusterSkipped},
	})
	require.Len(t, got, 2, "several statuses are an OR")

	got = filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterApplied, models.ReductionClusterStale},
	})
	require.Len(t, got, 2)
}

func TestFilterReviewClustersByTier(t *testing.T) {
	clusters := []*models.ReductionCluster{
		reviewCluster(models.ReductionTierIdentical, models.ReductionClusterOpen, false),
		reviewCluster(models.ReductionTierNear, models.ReductionClusterOpen, false),
	}
	visible := map[uint]bool{}

	got := filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Tier: []string{models.ReductionTierNear},
	})
	require.Len(t, got, 1)
	assert.Equal(t, models.ReductionTierNear, got[0].Tier)
}

// The attention filter keeps only the Clusters that carry a curation marker in
// the plan: a Loser holding something the Winner does not, or an oversized
// Near-Identical Cluster.
func TestFilterReviewClustersByNeedsAttention(t *testing.T) {
	lossy := reviewCluster(models.ReductionTierNear, models.ReductionClusterOpen, false)
	lossy.Lossy = []string{"description"}
	oversized := reviewCluster(models.ReductionTierNear, models.ReductionClusterOpen, false)
	oversized.Oversized = true
	plain := reviewCluster(models.ReductionTierIdentical, models.ReductionClusterOpen, false)
	clusters := []*models.ReductionCluster{lossy, oversized, plain}
	visible := map[uint]bool{}

	got := filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{NeedsAttention: true})
	require.Len(t, got, 2)
	assert.Equal(t, lossy.ID, got[0].ID)
	assert.Equal(t, oversized.ID, got[1].ID)
}

// Criteria combine: every active one must match. An inactive query never filters.
func TestFilterReviewClustersCombinesAndDefaults(t *testing.T) {
	clusters := []*models.ReductionCluster{
		reviewCluster(models.ReductionTierIdentical, models.ReductionClusterSkipped, false),
		reviewCluster(models.ReductionTierNear, models.ReductionClusterSkipped, false),
	}
	visible := map[uint]bool{}

	got := filterReviewClusters(clusters, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterSkipped},
		Tier:   []string{models.ReductionTierIdentical},
	})
	require.Len(t, got, 1)
	assert.Equal(t, models.ReductionTierIdentical, got[0].Tier)
}

// The filter is the one thing that must not answer questions about a withheld
// Cluster: its tier, state and curation markers are redacted because they
// describe Resources the caller may not know about, so no filter may match on
// them. Reachability is decided before any criterion is consulted, and a
// Cluster that fails it is excluded however well its own fields would match.
func TestFilterReviewClustersNeverMatchesWithheld(t *testing.T) {
	// A hidden Near-Identical, Lossy, open Cluster — fields the redaction
	// strips, and exactly the ones an oracle would fish for.
	hidden := reviewClusterWithMember(models.ReductionTierNear, models.ReductionClusterOpen, 1)
	hidden.Lossy = []string{"description"}
	visibleCluster := reviewClusterWithMember(models.ReductionTierIdentical, models.ReductionClusterOpen, 2)

	// Only Resource 2 is within the caller's reach.
	visible := map[uint]bool{2: true}

	for _, tc := range []struct {
		query          *query_models.ResourceReductionReviewQuery
		visibleMatches bool
	}{
		// The hidden Cluster is in no filtered result — whatever the criterion,
		// the result can never be read as an answer about it. The visible one
		// appears exactly when its own fields satisfy the filter.
		{&query_models.ResourceReductionReviewQuery{Tier: []string{models.ReductionTierNear}}, false},
		{&query_models.ResourceReductionReviewQuery{Status: []string{"open"}}, true},
		{&query_models.ResourceReductionReviewQuery{NeedsAttention: true}, false},
		{&query_models.ResourceReductionReviewQuery{Status: []string{models.ReductionClusterSkipped}}, false},
	} {
		got := filterReviewClusters([]*models.ReductionCluster{hidden, visibleCluster}, visible, tc.query)
		if tc.visibleMatches {
			require.Len(t, got, 1)
			assert.Equal(t, visibleCluster.ID, got[0].ID,
				"the visible Cluster answers a filter its own fields satisfy")
		} else {
			assert.Empty(t, got,
				"no Cluster — visible or withheld — matches a filter its fields do not satisfy")
		}
	}
}

// A Cluster whose only unreachable member is a confirmed-merged Loser is not
// withheld: the reviewer performed that deletion, so the record of it stays
// visible. The same exception the render applies.
func TestFilterReviewClustersMergedLosersAreNotWithheld(t *testing.T) {
	applied := reviewClusterWithMember(models.ReductionTierIdentical, models.ReductionClusterApplied, 7)
	applied.WinnerID = 9
	applied.Merged = true
	applied.Members = []*models.ReductionMember{
		{ResourceID: 9},
		{ResourceID: 7},
	}
	// Resource 7 (the merged-away Loser) is unreachable; the Winner 9 is not.
	visible := map[uint]bool{9: true}

	got := filterReviewClusters([]*models.ReductionCluster{applied}, visible, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterApplied},
	})
	require.Len(t, got, 1, "the confirmed merge keeps the Cluster visible to its reviewer")
}
