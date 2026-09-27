package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// newReductionWALContext is a production-shaped SQLite context (WAL, busy_timeout,
// a real file) with the Reduction table added.
func newReductionWALContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newWALTestContext(t, 0)
	require.NoError(t, ctx.db.AutoMigrate(&models.ResourceReduction{}))
	return ctx
}

func TestWideningAReductionIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newReductionWALContext(t)
	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ResourceIds: []uint{1}}, nil, false)
	require.NoError(t, err)

	fired := commitBeforeEveryWriteTo(t, ctx, "resource_reductions")
	widened, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID: created.ID, ResourceIds: []uint{2},
	}, nil, false)
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the widening wrote nothing, so the interleave never happened")

	extent, err := DecodeReductionExtent(widened.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{1, 2}, extent.ResourceIDs)
	assert.Equal(t, created.Version+1, widened.Version)
}

func TestCreatingAReductionFromAGroupIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newReductionWALContext(t)
	owner := &models.Group{Name: "owner"}
	require.NoError(t, ctx.db.Create(owner).Error)
	resource := &models.Resource{Name: "owned", OwnerId: &owner.ID}
	require.NoError(t, ctx.db.Create(resource).Error)

	fired := commitBeforeEveryWriteTo(t, ctx, "resource_reductions")
	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: owner.ID}, nil, false)
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the create wrote nothing, so the interleave never happened")

	extent, err := DecodeReductionExtent(created.Extent)
	require.NoError(t, err)
	assert.Equal(t, []uint{resource.ID}, extent.ResourceIDs)
}

func TestEditingReductionSettingsIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newReductionWALContext(t)
	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ResourceIds: []uint{1}}, nil, false)
	require.NoError(t, err)

	fired := commitBeforeEveryWriteTo(t, ctx, "resource_reductions")
	edited, err := ctx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID: created.ID, Version: created.Version, Name: "renamed",
	}, nil, false)
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the edit wrote nothing, so the interleave never happened")
	assert.Equal(t, "renamed", edited.Name)
}

func TestAReductionOverrideIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newReductionWALContext(t)
	for _, name := range []string{"winner", "loser"} {
		require.NoError(t, ctx.db.Create(&models.Resource{Name: name}).Error)
	}
	reduction := &models.ResourceReduction{
		Name:   "override under load",
		Status: models.ReductionStatusReady,
		Extent: []byte(`{"resourceIds":[1,2]}`),
		Plan: []byte(`{"clusters":[{"id":"c1","tier":"identical","winnerId":1,"state":"open",` +
			`"members":[{"resourceId":1,"inExtent":true},{"resourceId":2,"inExtent":true}]}]}`),
	}
	require.NoError(t, ctx.db.Create(reduction).Error)

	fired := commitBeforeEveryWriteTo(t, ctx, "resource_reductions")
	updated, err := ctx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID: reduction.ID, Version: reduction.Version, ClusterID: "c1", Action: ReductionActionSkip,
	}, nil, false)
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the override wrote nothing, so the interleave never happened")

	plan, err := DecodeReductionPlan(updated.Plan)
	require.NoError(t, err)
	require.Len(t, plan.Clusters, 1)
	assert.Equal(t, models.ReductionClusterSkipped, plan.Clusters[0].State)
}
