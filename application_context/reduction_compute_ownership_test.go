package application_context

import (
	"context"
	"fmt"
	"mahresources/jobs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// ---- failed computes leave nothing behind (D4) -----------------------------------------

// stageGeneration writes one row into every table a compute stages into.
func stageGeneration(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, gen string, resourceID uint) {
	t.Helper()
	require.NoError(t, ctx.reductionStore().insertCluster(r, gen, &storedCluster{
		Row:     models.ReductionClusterRow{Key: "k-" + gen, Tier: models.ReductionTierIdentical, State: models.ReductionClusterOpen},
		Members: []models.ReductionMemberRow{opsMember(resourceID, "h")},
	}))
	require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_extent (reduction_id, generation, resource_id, hash) VALUES (?, ?, ?, 'h')`, r.ID, gen, resourceID).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_seeds (reduction_id, generation, resource_id, key_prefix, sort_key) VALUES (?, ?, ?, ?, ?)`,
		r.ID, gen, resourceID, []byte{1}, []byte{1}).Error)
}

func generationRows(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, gen string) int64 {
	t.Helper()
	var total int64
	for _, table := range []string{"reduction_clusters", "reduction_members", "reduction_extent", "reduction_seeds"} {
		total += planRows(t, ctx, table, "reduction_id = ? AND generation = ?", r.ID, gen)
	}
	return total
}

func testStoreFailedComputesLeaveNoOrphans(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	res := makeResource(t, ctx.db, "orphan-res", opsRes{Hash: "h"})
	var gens []string
	for i := 0; i < 2; i++ {
		r = loadReductionRow(t, ctx.db, r.ID)
		jobID, gen, _ := computing(t, ctx, r)
		stageGeneration(t, ctx, r, gen, res)
		require.NotZero(t, generationRows(t, ctx, r, gen), "staged")
		require.NoError(t, ctx.storeFailCompute(r.ID, jobID, gen, fmt.Errorf("boom %d", i)))
		gens = append(gens, gen)
	}
	for _, gen := range gens {
		require.Zero(t, generationRows(t, ctx, r, gen), "a failed compute removes what it staged, without waiting for another sweep")
	}

	// A crashed worker never reached storeFailCompute: its released generation is
	// still removed when the next compute starts.
	r = loadReductionRow(t, ctx.db, r.ID)
	stageGeneration(t, ctx, r, "crashed-generation", res)
	require.NotZero(t, generationRows(t, ctx, r, "crashed-generation"))
	streamCompute(t, ctx, r)
	require.Zero(t, generationRows(t, ctx, r, "crashed-generation"), "the sweep runs at compute start")
}

func TestStoreFailedComputesLeaveNoOrphansSQLite(t *testing.T) {
	testStoreFailedComputesLeaveNoOrphans(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- the claim and the landing carry the owner predicate (D2) ---------------------------

func testStoreComputeClaimIsOwnerQualified(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = ? WHERE id = ?", 77, r.ID).Error)
	loaded := loadReductionRow(t, ctx.db, r.ID)
	require.NotNil(t, loaded.CreatedByUserId)

	// the owner is deleted after the caller loaded the row: no version bump
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = NULL WHERE id = ?", r.ID).Error)
	_, _, err := ctx.beginReductionComputeStoreAs(loaded.ID, loaded.Version, loaded.CreatedByUserId)
	require.ErrorIs(t, err, ErrReductionConflict)
	require.NotEqual(t, models.ReductionStatusComputing, loadReductionRow(t, ctx.db, r.ID).Status, "nothing was claimed")

	// the landing is qualified the same way
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = ? WHERE id = ?", 77, r.ID).Error)
	loaded = loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStoreAs(loaded.ID, loaded.Version, loaded.CreatedByUserId)
	require.NoError(t, err)
	require.NoError(t, ctx.storeAdmitComputeLease(r.ID, gen))
	running := loadReductionRow(t, ctx.db, r.ID)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = NULL WHERE id = ?", r.ID).Error)
	err = ctx.finishLanding(loaded, "", gen, running.Version, models.ReductionCoverage{}, nil)
	require.ErrorIs(t, err, ErrReductionConflict)
	require.Equal(t, opsGen, *loadReductionRow(t, ctx.db, r.ID).PlanGeneration, "the plan stands")
}

func TestStoreComputeClaimIsOwnerQualifiedSQLite(t *testing.T) {
	testStoreComputeClaimIsOwnerQualified(t, withSettings(newReductionSQLiteContext(t)))
}

func TestAwaitUpgradeGivesUpAtItsDeadline(t *testing.T) {
	ctx := newCutoverContext(t)
	r := legacyRow(t, ctx, models.ReductionStatusFailed)
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}
	prevPoll, prevWait := reductionUpgradePoll, reductionUpgradeMaxWait
	reductionUpgradePoll, reductionUpgradeMaxWait = 5*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { reductionUpgradePoll, reductionUpgradeMaxWait = prevPoll, prevWait })

	// The outer timeout only bounds a regression; the deadline under test is far shorter.
	bound, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	_, err := adapter.awaitUpgrade(bound, jobs.Execution{}, r.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration")
	assert.Less(t, time.Since(start), 5*time.Second)
}
