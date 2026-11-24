package application_context

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// newReductionWALContext is a production-shaped SQLite context (WAL, busy_timeout,
// a real file) with the Reduction table added.
func newReductionWALContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newWALTestContext(t, 0)
	require.NoError(t, ctx.db.AutoMigrate(reductionTables...))
	return withSettings(ctx)
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

	extent, err := ctx.ExtentForTest(widened.ID)
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

	extent, err := ctx.ExtentForTest(created.ID)
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
	reduction := &models.ResourceReduction{Name: "override under load", Status: models.ReductionStatusReady, PlanSchema: 1}
	require.NoError(t, ctx.db.Create(reduction).Error)
	require.NoError(t, ctx.OverwritePlanForTest(reduction.ID, reduction.Version, models.ResourceReductionPlan{Clusters: []*models.ReductionCluster{{
		ID: "c1", Tier: "identical", WinnerID: 1, State: models.ReductionClusterOpen,
		Members: []*models.ReductionMember{{ResourceID: 1, InExtent: true}, {ResourceID: 2, InExtent: true}},
	}}}))
	reduction = loadReductionRow(t, ctx.db, reduction.ID)

	fired := commitBeforeEveryWriteTo(t, ctx, "resource_reductions")
	updated, err := ctx.OverrideReductionCluster(&query_models.ReductionOverride{
		ID: reduction.ID, Version: reduction.Version, ClusterID: "c1", Action: ReductionActionSkip,
	}, nil, false)
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the override wrote nothing, so the interleave never happened")

	require.NotZero(t, updated.Version)
	plan, err := ctx.PlanForTest(reduction.ID)
	require.NoError(t, err)
	require.Len(t, plan.Clusters, 1)
	assert.Equal(t, models.ReductionClusterSkipped, plan.Clusters[0].State)
}

// TestAGroupSelectionIsReadFromOneSnapshot pins that a Reduction created from a group
// subtree reads that subtree's Resources as of one moment. The owners are queried in
// chunks, and outside any transaction each chunk would see whatever had committed
// since the last: a Resource moved from a group in a later chunk into one in an
// earlier chunk, after the earlier chunk was read, is in the subtree the whole time
// and in neither answer.
func TestAGroupSelectionIsReadFromOneSnapshot(t *testing.T) {
	ctx := newReductionWALContext(t)
	root := &models.Group{Name: "root"}
	require.NoError(t, ctx.db.Create(root).Error)
	children := make([]models.Group, idChunk+1)
	for i := range children {
		children[i] = models.Group{Name: fmt.Sprintf("child %d", i), OwnerId: &root.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(children, 200).Error)

	owners, err := ctx.collectSubtreeGroupIDs(root.ID)
	require.NoError(t, err)
	require.Greater(t, len(owners), idChunk, "the subtree has to span two chunks")
	earlier, later := owners[1], owners[len(owners)-1]
	moved := &models.Resource{Name: "moved", OwnerId: &later}
	require.NoError(t, ctx.db.Create(moved).Error)

	// A competing writer moves the Resource from the later chunk of Groups into the
	// earlier one while the selection is part-way through. It is started between the
	// first Group's count and its inserts, and the selection's own transaction holds
	// the writer lock, so the move lands only after the selection has read the whole
	// subtree as of one moment: the Resource is selected once, and not lost.
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	var once sync.Once
	moveDone := make(chan error, 1)
	selectionAfterPreflight = func(*gorm.DB) {
		once.Do(func() {
			go func() {
				_, err := sqlDB.Exec(`UPDATE resources SET owner_id = ? WHERE id = ?`, earlier, moved.ID)
				moveDone <- err
			}()
		})
	}
	t.Cleanup(func() { selectionAfterPreflight = nil })

	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		OwnerId: root.ID, IncludeDescendants: true,
	}, nil, false)
	require.NoError(t, err)
	require.NoError(t, <-moveDone, "the competing move")
	extent, err := ctx.ExtentForTest(created.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{moved.ID}, extent.ResourceIDs)
}
