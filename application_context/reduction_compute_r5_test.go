package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// associations_desc counts through the store's one-bind subtree query: a principal
// whose subtree holds more Groups than a statement may bind still clusters.
func testAssociationsRuleOverAHugeSubtree(t *testing.T, ctx *MahresourcesContext) {
	root := &models.Group{Name: "root"}
	require.NoError(t, ctx.db.Create(root).Error)
	require.NoError(t, ctx.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 70000)
		INSERT INTO groups (name, owner_id, created_at, updated_at) SELECT 'g' || i, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM n`, root.ID).Error)
	one := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 10, H: 10, Size: 1, Owner: &root.ID})
	two := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: 20, H: 20, Size: 2, Owner: &root.ID})
	note := &models.Note{Name: "n", OwnerId: &root.ID}
	require.NoError(t, ctx.db.Create(note).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)`, one, note.ID).Error)

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 7, ScopeGroupID: &root.ID})
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true, Rule: []string{models.WinnerCriterionAssociationsDesc}})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := scoped.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)
	require.NoError(t, scoped.runReductionCompute(context.Background(), cur.ID, "", gen, nil))
	var winner uint
	require.NoError(t, ctx.db.Raw(`SELECT winner_id FROM reduction_clusters WHERE reduction_id = ? AND generation = ?`, r.ID, gen).Scan(&winner).Error)
	require.Equal(t, one, winner, "the Resource with a Note wins on associations")
	_ = two
}

// A reviewer's "don't remove" on a member of a Cluster that was later applied and
// merged (a migrated over-cap Cluster keeps such survivors) is still a decision: the
// ejected survivor must not reappear in a fresh Cluster, while its Winner may.
func testEjectedSurvivorOfAMergedClusterStaysOut(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	res := func(name string) uint { return makeResource(t, ctx.db, name, opsRes{Hash: "he", W: 10, H: 10}) }
	w, kept, other := res("w"), res("kept"), res("other")
	c := &storedCluster{Row: models.ReductionClusterRow{Key: "merged", Tier: models.ReductionTierIdentical, State: models.ReductionClusterApplied,
		Position: 1, Merged: true, ReadOnly: false, WinnerID: w}, Members: []models.ReductionMemberRow{opsMember(w, "he"), ejectedMember(kept, "he")}}
	require.NoError(t, ctx.reductionStore().insertCluster(r, opsGen, c))

	streamCompute(t, ctx, r)
	var fresh []uint
	require.NoError(t, ctx.db.Raw(`SELECT m.resource_id FROM reduction_members m JOIN reduction_clusters c ON c.id = m.cluster_id
		WHERE c.reduction_id = ? AND c.key <> 'merged' AND c.generation = (SELECT plan_generation FROM resource_reductions WHERE id = ?)`, r.ID, r.ID).Scan(&fresh).Error)
	require.NotContains(t, fresh, kept, "an ejected survivor of a merged Cluster is not proposed again")
	require.ElementsMatch(t, []uint{w, other}, fresh)
}

// Staging the Resources related to a selected Group probes the relationship table
// once per candidate Resource (correlated), where an IN over the subquery makes
// SQLite materialize the Group's whole relationship list on every page.
func testRelatedGroupStagingProbesPerResource(t *testing.T, ctx *MahresourcesContext) {
	require.NoError(t, models.EnsureSupplementalIndexes(ctx.db))
	g := &models.Group{Name: "G"}
	require.NoError(t, ctx.db.Create(g).Error)
	for i := 0; i < 6; i++ {
		id := mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: uint(10 + i), H: 10, Size: int64(i)})
		require.NoError(t, ctx.db.Exec(`INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)`, g.ID, id).Error)
	}
	rec := installScaleRecorder(t, ctx)
	rec.capture("related-page", `SELECT r\.id FROM resources r WHERE r\.id > \?.*groups_related_resources`, "")
	rec.capture("live-extent", `SELECT r\.id FROM resources r WHERE r\.id IN \(.*groups_related_resources`, "")
	r := mkStreamReduction(t, ctx, streamSel{Groups: []uint{g.ID}, Identical: true})
	streamCompute(t, ctx, r)
	for _, name := range []string{"related-page", "live-extent"} {
		plan := rec.explain(t, ctx, name)
		if isPG(ctx) {
			// Postgres plans the semi-join itself; the table is small in this fixture.
			continue
		}
		require.Regexp(t, `(?m)^\s*SEARCH grr .*resource_id=\?`, plan, "%s probes the relationship table per Resource", name)
		require.NotRegexp(t, `(?m)^\s*SCAN (grr|groups_related_resources)`, plan, name)
		// The only list built is the Group tree's ids, never the relationship rows.
		require.NotRegexp(t, `LIST SUBQUERY \d+\s*\n\s*SEARCH grr`, plan, name)
	}
}

func TestRelatedGroupStagingProbesPerResourceSQLite(t *testing.T) {
	testRelatedGroupStagingProbesPerResource(t, newScaleSQLiteContext(t))
}

// The Identical tier pages a hash's holders by id: the (hash, id) key serves both the
// equality and the order, so no sort sits between the index and the LIMIT.
func testHolderCursorIsServedByTheHashIDIndex(t *testing.T, ctx *MahresourcesContext) {
	require.NoError(t, models.EnsureSupplementalIndexes(ctx.db))
	for i := 0; i < 6; i++ {
		mkStreamRes(t, ctx.db, streamRes{Hash: "dup", W: uint(10 + i), H: 10, Size: int64(i)})
	}
	rec := installScaleRecorder(t, ctx)
	rec.capture("holders", `FROM resources r WHERE r\.hash = \? AND r\.id > \?`, "")
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	streamCompute(t, ctx, r)
	rec.mu.Lock()
	stmt, ok := rec.captured["holders"]
	rec.mu.Unlock()
	require.True(t, ok, "the holder statement was sent")

	var lines []string
	if isPG(ctx) {
		require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
			for _, off := range []string{"enable_seqscan", "enable_sort", "enable_bitmapscan"} {
				if err := tx.Exec(`SET LOCAL ` + off + ` = off`).Error; err != nil {
					return err
				}
			}
			return tx.Raw("EXPLAIN "+stmt.SQL, stmt.Vars...).Scan(&lines).Error
		}))
	} else {
		var rows []struct{ Detail string }
		require.NoError(t, ctx.db.Raw("EXPLAIN QUERY PLAN "+stmt.SQL, stmt.Vars...).Scan(&rows).Error)
		for _, row := range rows {
			lines = append(lines, row.Detail)
		}
	}
	plan := strings.Join(lines, "\n")
	t.Logf("holder plan:\n%s", plan)
	want := "idx_resources_hash"
	if isPG(ctx) {
		want = "idx__resources__hash_id"
	}
	require.Contains(t, plan, want)
	for _, bad := range []string{"TEMP B-TREE", "Sort", "Seq Scan", "SCAN r"} {
		require.NotContains(t, plan, bad)
	}
}

func TestHolderCursorIsServedByTheHashIDIndexSQLite(t *testing.T) {
	testHolderCursorIsServedByTheHashIDIndex(t, newScaleSQLiteContext(t))
}

// The orphan sweep continues each chunk after the last key it deleted: the live
// rows in front of the orphans, and the orphans already gone, are not read again.
func testOrphanSweepContinuesFromTheLastDeletedKey(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	const orphan = 99999
	for i := 1; i <= 3; i++ {
		require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_extent_resources (reduction_id, resource_id) VALUES (?, ?)`, r.ID, i).Error)
	}
	require.NoError(t, ctx.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 1300)
		INSERT INTO reduction_extent_resources (reduction_id, resource_id) SELECT ?, i FROM n`, orphan).Error)

	var selects [][]any
	name := "r5:orphan-sweep"
	record := func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "SELECT x.reduction_id AS reduction_id") {
			selects = append(selects, append([]any(nil), tx.Statement.Vars...))
		}
	}
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, record))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name, record))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Query().Remove(name)
		_ = ctx.db.Callback().Row().Remove(name)
	})

	require.NoError(t, ctx.reductionStore().sweepOrphans("reduction_extent_resources", "resource_id"))
	require.EqualValues(t, 0, planRows(t, ctx, "reduction_extent_resources", "reduction_id = ?", orphan))
	require.EqualValues(t, 3, planRows(t, ctx, "reduction_extent_resources", "reduction_id = ?", r.ID), "live rows stay")
	require.GreaterOrEqual(t, len(selects), 3)
	flat := func(vars []any) string { return fmt.Sprint(vars...) }
	for i, vars := range selects[1:] {
		if i == len(selects)-2 {
			continue // the last read finds nothing, wherever it starts
		}
		require.Contains(t, flat(vars), fmt.Sprint(orphan), "read %d starts after the last deleted key", i+1)
	}
}

// A runner cancelled after it listed the legacy Reductions spools none of them: the
// spool honours the runner's context as the migration step does.
func testSpoolHonoursACancelledRunner(t *testing.T, ctx *MahresourcesContext) {
	red := legacyRow(t, ctx, models.ReductionStatusReady)
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	keep, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = keep.Close() })

	stop, cancel := context.WithCancel(context.Background())
	defer cancel()
	spoolAfterList = cancel
	t.Cleanup(func() { spoolAfterList = nil })

	require.Error(t, ctx.migrateReductionPlansUntil(stop, true))
	require.EqualValues(t, 0, planRows(t, ctx, "reduction_migrations", "reduction_id = ?", red.ID), "a cancelled runner spooled a Reduction")
}

func TestSpoolHonoursACancelledRunnerSQLite(t *testing.T) {
	testSpoolHonoursACancelledRunner(t, newCutoverContext(t))
}

func TestOrphanSweepContinuesFromTheLastDeletedKeySQLite(t *testing.T) {
	testOrphanSweepContinuesFromTheLastDeletedKey(t, withSettings(newReductionSQLiteContext(t)))
}

func TestEjectedSurvivorOfAMergedClusterStaysOutSQLite(t *testing.T) {
	testEjectedSurvivorOfAMergedClusterStaysOut(t, withSettings(newReductionSQLiteContext(t)))
}

func TestAssociationsRuleOverAHugeSubtreeSQLite(t *testing.T) {
	testAssociationsRuleOverAHugeSubtree(t, withSettings(newReductionSQLiteContext(t)))
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
