package application_context

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

// A chunk of the carry-over whose transaction lost to lock contention is retried
// from the same cursor: the copy must hold every carried member afterwards.
func testCarryOverRetriesTheFailedChunk(t *testing.T, ctx *MahresourcesContext, phase string) {
	r := applyReduction(t, ctx)
	want := map[string][2]uint{}
	for _, key := range []string{"a", "b", "c"} {
		w, l := makeResource(t, ctx.db, key+"w", opsRes{Hash: "h"}), makeResource(t, ctx.db, key+"l", opsRes{Hash: "h"})
		want[key] = [2]uint{w, l}
		seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: key, Tier: models.ReductionTierIdentical, State: models.ReductionClusterSkipped, Position: int64(len(want)), WinnerID: w},
			opsMember(w, "h"), opsMember(l, "h"))
	}
	r = loadReductionRow(t, ctx.db, r.ID)
	jobID, gen, version := computing(t, ctx, r)

	calls := 0
	carryOverChunkHook = func(tx *gorm.DB, p string) error {
		if p != phase {
			return nil
		}
		calls++
		if calls == 1 {
			return errors.New("database is locked")
		}
		return nil
	}
	t.Cleanup(func() { carryOverChunkHook = nil })

	require.NoError(t, ctx.storeLandComputedPlan(r, jobID, gen, version, nil, models.ReductionCoverage{}, nil))
	require.Equal(t, 2, calls, "the failed chunk ran again")
	after := clusterRowsOf(t, ctx, r, gen)
	require.Len(t, after, 3)
	for key, ids := range want {
		var got []uint
		require.NoError(t, ctx.db.Raw(`SELECT resource_id FROM reduction_members WHERE cluster_id = ? ORDER BY position`, after[key].ID).Scan(&got).Error)
		require.Equal(t, []uint{ids[0], ids[1]}, got, "the retried chunk of %s carried its members", key)
	}
}

func testCarryVerificationRefusesAnIncompleteCopy(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	w, l := makeResource(t, ctx.db, "w", opsRes{Hash: "h"}), makeResource(t, ctx.db, "l", opsRes{Hash: "h"})
	seedAt(t, ctx, r, opsGen, models.ReductionClusterRow{Key: "k", Tier: models.ReductionTierIdentical, State: models.ReductionClusterSkipped, Position: 1, WinnerID: w},
		opsMember(w, "h"), opsMember(l, "h"))
	r = loadReductionRow(t, ctx.db, r.ID)
	_, gen, _ := computing(t, ctx, r)
	s := ctx.reductionStore()
	require.NoError(t, s.carryOver(r.ID, gen))
	require.NoError(t, s.verifyCarried(r.ID, opsGen, gen))
	require.NoError(t, ctx.db.Exec(`DELETE FROM reduction_members WHERE generation = ? AND resource_id = ?`, gen, l).Error)
	require.ErrorIs(t, s.verifyCarried(r.ID, opsGen, gen), errCarryIncomplete)
}

// A cancel that arrives while the landing waits for the Reduction row must not
// publish: the landing re-checks the worker's context once it holds the row.
func testLandingRechecksCancelUnderTheRowLock(t *testing.T, ctx *MahresourcesContext) {
	for i := 0; i < 2; i++ {
		mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 10, H: 10, Size: int64(100 + i)})
	}
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)

	// A cancelled statement discards its connection, and a shared in-memory database
	// dies with its last one: hold one open for the assertions.
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	keep, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = keep.Close() })

	jobCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	landAfterLockHook = cancel
	t.Cleanup(func() { landAfterLockHook = nil })

	err = ctx.runReductionComputeOwned(jobCtx, cur.ID, "", gen, nil, nil, false)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	row := loadReductionRow(t, ctx.db, cur.ID)
	require.Nil(t, row.PlanGeneration, "a cancelled compute publishes nothing")
	require.NotEqual(t, models.ReductionStatusReady, row.Status)
}

func TestLandingRechecksCancelUnderTheRowLockSQLite(t *testing.T) {
	testLandingRechecksCancelUnderTheRowLock(t, withSettings(newReductionSQLiteContext(t)))
}

// An outside-Extent member (an external duplicate the principal could see when the
// Cluster was built) that is moved out of the principal's subtree before the Cluster
// is inserted must not be inserted: the insertion re-check demands visibility of
// every member, not only an unchanged Extent classification.
func testInsertionRefusesAMemberMovedOutOfTheSubtree(t *testing.T, ctx *MahresourcesContext) {
	a, b := &models.Group{Name: "A"}, &models.Group{Name: "B"}
	require.NoError(t, ctx.db.Create(a).Error)
	require.NoError(t, ctx.db.Create(b).Error)
	inExtent := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 10, H: 10, Size: 1, Owner: &a.ID})
	external := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 30, H: 30, Size: 2, Owner: &a.ID})
	external2 := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 20, H: 20, Size: 3, Owner: &a.ID})

	moved := false
	computeBeforeInsertHook = func(string, int) {
		if !moved {
			moved = true
			require.NoError(t, ctx.db.Exec("UPDATE resources SET owner_id = ? WHERE id = ?", b.ID, external).Error)
		}
	}
	t.Cleanup(func() { computeBeforeInsertHook = nil })

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 7, ScopeGroupID: &a.ID})
	r := mkStreamReduction(t, ctx, streamSel{Resources: []uint{inExtent}, Identical: true})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := scoped.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, scoped.runReductionCompute(context.Background(), cur.ID, "", gen, nil))
	require.True(t, moved)
	var members []uint
	require.NoError(t, ctx.db.Raw(`SELECT resource_id FROM reduction_members WHERE reduction_id = ? AND generation = ?`, r.ID, gen).Scan(&members).Error)
	require.NotContains(t, members, external, "a Resource moved out of the subtree is not proposed")
	require.ElementsMatch(t, []uint{inExtent, external2}, members)
}

func TestInsertionRefusesAMemberMovedOutOfTheSubtreeSQLite(t *testing.T) {
	testInsertionRefusesAMemberMovedOutOfTheSubtree(t, withSettings(newReductionSQLiteContext(t)))
}

func TestCarryVerificationRefusesAnIncompleteCopySQLite(t *testing.T) {
	testCarryVerificationRefusesAnIncompleteCopy(t, withSettings(newReductionSQLiteContext(t)))
}

func TestCarryOverRetriesTheFailedMemberChunkSQLite(t *testing.T) {
	testCarryOverRetriesTheFailedChunk(t, withSettings(newReductionSQLiteContext(t)), "members")
}
func TestCarryOverRetriesTheFailedClusterChunkSQLite(t *testing.T) {
	testCarryOverRetriesTheFailedChunk(t, withSettings(newReductionSQLiteContext(t)), "clusters")
}
