package api_tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models/query_models"
)

// A "Show more members" page that was requested for one render of a card must not
// be answered for another. The request names the Reduction version and the kept copy
// the card showed; a page for a Reduction or a Cluster that has moved on is a conflict,
// and an approval that names a kept copy the Cluster no longer keeps is one too.
func TestMembersPageAndApprovalRefuseACardThatMovedOn(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	hub := addImage(t, tc, "hub.jpg", 4000, 4000)
	ids := []uint{hub.ID}
	for i := 0; i < 14; i++ {
		neighbour := addImage(t, tc, fmt.Sprintf("near-%d.jpg", i), 100, 100)
		pairThem(t, tc, hub, neighbour, 5)
		ids = append(ids, neighbour.ID)
	}
	red := createReduction(t, tc, "Race", ids)
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	require.True(t, cluster.Oversized)
	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)

	version := current.Version
	winner := cluster.WinnerID
	page := func(v, w *uint) error {
		_, err := tc.AppCtx.GetReductionClusterMembersChecked(red.ID, owner, restricted, cluster.ID, "",
			contracts.ReductionMembersExpect{Version: v, WinnerID: w})
		return err
	}
	stale := version + 1
	other := winner + 1000
	assert.ErrorIs(t, page(&stale, nil), application_context.ErrReductionConflict, "a page for another version")
	assert.ErrorIs(t, page(nil, &other), application_context.ErrReductionConflict, "a page for another kept copy")
	require.NoError(t, page(&version, &winner), "the render the card was built from")
	require.NoError(t, page(nil, nil), "callers that say nothing are unchanged")

	seen := uint(len(cluster.Members))
	approve := func(w *uint) error {
		_, err := tc.AppCtx.OverrideReductionCluster(&query_models.ReductionOverride{
			ID: red.ID, Version: version, ClusterID: cluster.ID,
			Action: application_context.ReductionActionCheck, AcknowledgeOversized: true, MembersSeen: &seen, WinnerID: w,
		}, owner, restricted)
		return err
	}
	assert.ErrorIs(t, approve(&other), application_context.ErrReductionConflict, "an approval of a different kept copy")
	require.NoError(t, approve(&winner))
}
