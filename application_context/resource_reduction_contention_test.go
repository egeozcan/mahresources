package application_context

import (
	"sync/atomic"
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
	require.NoError(t, ctx.db.AutoMigrate(&models.ResourceReduction{}))
	return ctx
}

// commitBeforeEveryReductionWrite has another connection commit immediately before
// every INSERT and UPDATE this context sends to resource_reductions, and counts them.
//
// That is what a busy deployment does on its own: the compute Job's bookkeeping, a
// hash or thumbnail worker, a job migration pass commit between a writer's reads and
// its write. A transaction that read first holds a WAL snapshot that commit has just
// made stale, and promoting it fails at once with SQLITE_BUSY_SNAPSHOT — the busy
// handler is never invoked, so busy_timeout does nothing. It fires on every write, not
// once, because a retry that reads again first meets the same commit again under
// sustained load; only a transaction whose first statement is the write survives it.
//
// The competing write goes through the raw pool, so it runs on another connection and
// does not re-enter this callback.
func commitBeforeEveryReductionWrite(t *testing.T, ctx *MahresourcesContext) *atomic.Int32 {
	t.Helper()
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	var fired atomic.Int32
	competingCommit := func(tx *gorm.DB) {
		if tx.Statement.Table != "resource_reductions" {
			return
		}
		fired.Add(1)
		if _, err := sqlDB.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`); err != nil {
			t.Errorf("competing commit: %v", err)
		}
	}
	const updateName, createName = "test:commit_before_reduction_update", "test:commit_before_reduction_create"
	require.NoError(t, ctx.db.Callback().Update().Before("gorm:update").Register(updateName, competingCommit))
	require.NoError(t, ctx.db.Callback().Create().Before("gorm:create").Register(createName, competingCommit))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Update().Remove(updateName)
		_ = ctx.db.Callback().Create().Remove(createName)
	})
	return &fired
}

func TestWideningAReductionIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newReductionWALContext(t)
	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ResourceIds: []uint{1}}, nil, false)
	require.NoError(t, err)

	fired := commitBeforeEveryReductionWrite(t, ctx)
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

	fired := commitBeforeEveryReductionWrite(t, ctx)
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

	fired := commitBeforeEveryReductionWrite(t, ctx)
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

	fired := commitBeforeEveryReductionWrite(t, ctx)
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
