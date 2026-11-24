package application_context

import (
	"context"
	"errors"
	"mahresources/jobs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// The public entry points on the plan tables (design D3, D8, D12): what each refuses
// and when. The writers' own behaviour is tested beside the store.

func newCutoverContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	return withSettings(newReductionSQLiteContext(t))
}

func legacyRow(t *testing.T, ctx *MahresourcesContext, status string) *models.ResourceReduction {
	t.Helper()
	row := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: status, MatchingMode: "both"}, Plan: []byte(`{"clusters":[]}`)}
	require.NoError(t, ctx.db.Create(row).Error)
	return &row.ResourceReduction
}

func tableRow(t *testing.T, ctx *MahresourcesContext, status string) *models.ResourceReduction {
	t.Helper()
	row := &models.ResourceReduction{Name: "tables", Status: status, MatchingMode: "both", PlanSchema: 1}
	require.NoError(t, ctx.db.Create(row).Error)
	return row
}

func TestEveryEntryPointAnswersABeingUpgradedReductionAsUpgrading(t *testing.T) {
	ctx := newCutoverContext(t)
	require.NoError(t, models.EnsureSimilarityDistanceIndexes(ctx.db))
	r := legacyRow(t, ctx, models.ReductionStatusReady)

	_, err := ctx.GetReductionReview(r.ID, nil, false, 1, nil)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "review")
	_, err = ctx.GetReductionClusterMembers(r.ID, nil, false, "k", "")
	assert.ErrorIs(t, err, ErrReductionUpgrading, "members")
	_, err = ctx.OverrideReductionCluster(&query_models.ReductionOverride{ID: r.ID, Version: r.Version, ClusterID: "k", Action: ReductionActionSkip}, nil, false)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "override")
	_, err = ctx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{ID: r.ID, Version: r.Version, Action: ReductionActionCheck, ClusterIDs: []string{"k"}}, nil, false)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "bulk")
	_, err = ctx.ApplyResourceReduction(&query_models.ReductionApply{ID: r.ID, Version: r.Version}, nil, false)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "apply")
	_, err = ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "compute")
	_, err = ctx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{ID: r.ID, Version: r.Version, Name: "x"}, nil, false)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "settings")
	_, err = ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ID: r.ID, ResourceIds: []uint{1}}, nil, false)
	assert.ErrorIs(t, err, ErrReductionUpgrading, "widen")

	// The list renders it as being upgraded instead of failing the page.
	rows, count, progress, err := ctx.ListResourceReductions(0, 20, &query_models.ResourceReductionQuery{})
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.EqualValues(t, 1, count)
	assert.True(t, progress[r.ID].Upgrading)
}

func TestAComputeIsRefusedUntilTheSimilarityIndexesAreBuilt(t *testing.T) {
	ctx := newCutoverContext(t)
	r := tableRow(t, ctx, models.ReductionStatusDraft)

	_, err := ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	require.ErrorIs(t, err, ErrReductionIndexesBuilding)
	assert.Contains(t, err.Error(), "indexes are still being built")
	assert.Equal(t, models.ReductionStatusDraft, loadReductionRow(t, ctx.db, r.ID).Status, "a refused compute claims nothing")

	require.NoError(t, models.EnsureSimilarityDistanceIndexes(ctx.db))
	_, err = ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		// A shared-cache table lock lost to the worker's own write is not an answer.
		var status string
		if err := ctx.db.Raw(`SELECT status FROM resource_reductions WHERE id = ?`, r.ID).Scan(&status).Error; err != nil {
			return false
		}
		return status == models.ReductionStatusReady
	}, 10*time.Second, 20*time.Millisecond, "the compute lands its plan once the indexes exist")
}

func TestSettingsAndWideningAreRefusedWhileComputing(t *testing.T) {
	ctx := newCutoverContext(t)
	r := tableRow(t, ctx, models.ReductionStatusComputing)

	_, err := ctx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{ID: r.ID, Version: r.Version, Name: "renamed"}, nil, false)
	assert.ErrorIs(t, err, ErrReductionBusy)
	_, err = ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ID: r.ID, ResourceIds: []uint{1}}, nil, false)
	assert.ErrorIs(t, err, ErrReductionBusy)
	assert.Equal(t, "tables", loadReductionRow(t, ctx.db, r.ID).Name)
}

func TestASelectionIsCappedPerRequestAndAtomic(t *testing.T) {
	ctx := newCutoverContext(t)
	ids := make([]uint, reductionSelectionCap+1)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	_, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ResourceIds: ids}, nil, false)
	assert.ErrorIs(t, err, ErrReductionSelectionTooLarge)

	// A selection refused part-way leaves no Reduction behind.
	_, err = ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: 987654}, nil, false)
	require.Error(t, err)
	var n int64
	require.NoError(t, ctx.db.Model(&models.ResourceReduction{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestACreatedReductionHoldsItsSelectionAsRows(t *testing.T) {
	ctx := newCutoverContext(t)
	group := &models.Group{Name: "g"}
	require.NoError(t, ctx.db.Create(group).Error)
	r1 := &models.Resource{Name: "a", OwnerId: &group.ID}
	r2 := &models.Resource{Name: "b", OwnerId: &group.ID}
	require.NoError(t, ctx.db.Create(r1).Error)
	require.NoError(t, ctx.db.Create(r2).Error)

	created, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "sel", ResourceIds: []uint{r1.ID}, GroupIds: []uint{group.ID}, All: true}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, 1, created.PlanSchema, "a new Reduction is a table plan")
	extent, err := ctx.ExtentForTest(created.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{r1.ID}, extent.ResourceIDs)
	assert.Equal(t, []uint{group.ID}, extent.GroupIDs)
	assert.True(t, extent.All)

	widened, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ID: created.ID, OwnerId: group.ID}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, created.Version+1, widened.Version)
	extent, err = ctx.ExtentForTest(created.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{r1.ID, r2.ID}, extent.ResourceIDs)
}

func TestAQueuedComputeWaitsForTheMigrationRatherThanBlocking(t *testing.T) {
	ctx := newCutoverContext(t)
	r := legacyRow(t, ctx, models.ReductionStatusFailed)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}

	previous := reductionUpgradePoll
	reductionUpgradePoll = 5 * time.Millisecond
	t.Cleanup(func() { reductionUpgradePoll = previous })

	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = ctx.db.Exec(`UPDATE resource_reductions SET plan_schema = 1 WHERE id = ?`, r.ID).Error
	}()
	got, err := adapter.awaitUpgrade(context.Background(), jobs.Execution{}, r.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.PlanSchema, "the run proceeds once the migration publishes")

	// A cancelled run stops waiting.
	other := legacyRow(t, ctx, models.ReductionStatusFailed)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.awaitUpgrade(cancelled, jobs.Execution{}, other.ID)
	assert.True(t, errors.Is(err, context.Canceled))
}
