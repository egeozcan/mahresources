package application_context

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// ---- fixtures shared by the table-writer tests ---------------------------------

const opsGen = "g"

// newPlanReduction is a Reduction row holding a settled table plan at generation
// opsGen, read back the way a caller loads it.
func newPlanReduction(t *testing.T, db *gorm.DB) *models.ResourceReduction {
	t.Helper()
	r := &models.ResourceReduction{Name: "ops test", Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, db.Create(r).Error)
	gen := opsGen
	require.NoError(t, db.Exec("UPDATE resource_reductions SET plan_schema = 1, plan_generation = ? WHERE id = ?", gen, r.ID).Error)
	return loadReductionRow(t, db, r.ID)
}

// withSettings wires the runtime settings the Near paths read the similarity
// threshold from.
func withSettings(ctx *MahresourcesContext) *MahresourcesContext {
	ctx.SetSettings(NewRuntimeSettings(ctx.db, NewStdlibSettingsLogger(), BuildSpecsExported(), BuildDefaultsFromConfig(ctx.Config)))
	return ctx
}

func loadReductionRow(t *testing.T, db *gorm.DB, id uint) *models.ResourceReduction {
	t.Helper()
	var r models.ResourceReduction
	require.NoError(t, db.Select(reductionColumns).First(&r, id).Error)
	return &r
}

type opsRes struct {
	W, H   uint
	Desc   string
	Series bool
	Cat    uint
	Hash   string
	Owner  *uint
}

func makeResource(t *testing.T, db *gorm.DB, name string, o opsRes) uint {
	t.Helper()
	res := &models.Resource{Name: name, Hash: o.Hash, Width: o.W, Height: o.H, Description: o.Desc, ResourceCategoryId: 1, OwnerId: o.Owner}
	if o.Cat != 0 {
		res.ResourceCategoryId = o.Cat
	}
	if o.Series {
		s := &models.Series{Name: name, Slug: fmt.Sprintf("s-%s-%d", name, time.Now().UnixNano())}
		require.NoError(t, db.Create(s).Error)
		res.SeriesID = &s.ID
	}
	require.NoError(t, db.Create(res).Error)
	return res.ID
}

func makePair(t *testing.T, db *gorm.DB, a, b uint, distance uint8) {
	t.Helper()
	d := distance
	require.NoError(t, db.Create(&models.ResourceSimilarity{ResourceID1: a, ResourceID2: b, HammingDistance: distance, PDistance: &d}).Error)
}

// insertOps writes a Cluster of existing Resources under the Reduction.
func insertOps(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key, tier string, winner uint, members ...models.ReductionMemberRow) *storedCluster {
	t.Helper()
	c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: tier, State: models.ReductionClusterOpen, WinnerID: winner, Checked: tier == models.ReductionTierIdentical}, Members: members}
	require.NoError(t, ctx.reductionStore().insertCluster(r, opsGen, c))
	return c
}

// mutateOps rewrites a stored Cluster through the store's own writer, so its
// derived columns follow.
func mutateOps(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key string, fn func(*storedCluster)) {
	t.Helper()
	c := loadOps(t, ctx, r, key)
	before := copyMembers(c.Members)
	fn(c)
	require.NoError(t, ctx.reductionStore().writeCluster(c, before))
}

func opsMember(id uint, hash string) models.ReductionMemberRow {
	return models.ReductionMemberRow{ResourceID: id, Hash: hash, FileSize: 10, InExtent: true}
}

func ejectedMember(id uint, hash string) models.ReductionMemberRow {
	m := opsMember(id, hash)
	m.Ejected, m.EjectedReason = true, models.EjectReasonManual
	return m
}

func loadOps(t *testing.T, ctx *MahresourcesContext, r *models.ResourceReduction, key string) *storedCluster {
	t.Helper()
	c, err := ctx.reductionStore().loadCluster(r, opsGen, key)
	require.NoError(t, err)
	return c
}

func override(r *models.ResourceReduction, key, action string, resourceID uint) *query_models.ReductionOverride {
	return &query_models.ReductionOverride{ID: r.ID, Version: r.Version, ClusterID: key, Action: action, ResourceID: resourceID}
}

func memberOf(c *storedCluster, id uint) models.ReductionMemberRow {
	for _, m := range c.Members {
		if m.ResourceID == id {
			return m
		}
	}
	return models.ReductionMemberRow{}
}

// ---- directed tests -----------------------------------------------------------

func testOpsOverrideActions(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h"})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h"})
	c3 := makeResource(t, ctx.db, "c", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "k1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"), opsMember(c3, "h"))

	step := func(action string, id uint) *models.ResourceReduction {
		t.Helper()
		next, err := ctx.storeOverrideCluster(r, override(r, "k1", action, id))
		require.NoError(t, err, action)
		require.Equal(t, r.Version+1, next.Version, "every decision is one version")
		r = next
		return next
	}

	step(ReductionActionUncheck, 0)
	c := loadOps(t, ctx, r, "k1")
	require.False(t, c.Row.Checked)
	require.True(t, c.Row.Reviewed)

	step(ReductionActionCheck, 0)
	require.True(t, loadOps(t, ctx, r, "k1").Row.Checked)

	step(ReductionActionEject, b)
	c = loadOps(t, ctx, r, "k1")
	require.True(t, memberOf(c, b).Ejected)
	require.Equal(t, models.EjectReasonManual, memberOf(c, b).EjectedReason)
	require.Equal(t, 1, c.Row.LoserCount)

	step(ReductionActionEject, c3)
	c = loadOps(t, ctx, r, "k1")
	require.Zero(t, c.Row.LoserCount)
	require.False(t, c.Row.Checked, "nothing left to remove: no longer approved")

	_, err := ctx.storeOverrideCluster(r, override(r, "k1", ReductionActionCheck, 0))
	require.ErrorIs(t, err, ErrReductionNothingToRemove)

	before := c.Row.RestoreSeq
	step(ReductionActionRestore, c3)
	c = loadOps(t, ctx, r, "k1")
	require.False(t, memberOf(c, c3).Ejected)
	require.Equal(t, before+1, c.Row.RestoreSeq, "a restore adds a Loser, so it bumps restore_seq")

	step(ReductionActionPromote, c3)
	c = loadOps(t, ctx, r, "k1")
	require.Equal(t, c3, c.Row.WinnerID)
	require.Equal(t, models.ManualCriterion, c.Row.DecidedBy)
	require.Equal(t, before+2, c.Row.RestoreSeq)
	require.False(t, c.Row.Checked, "a different kept copy is a different proposal")

	step(ReductionActionSkip, 0)
	require.Equal(t, models.ReductionClusterSkipped, loadOps(t, ctx, r, "k1").Row.State)
	step(ReductionActionReopen, 0)
	require.Equal(t, models.ReductionClusterOpen, loadOps(t, ctx, r, "k1").Row.State)

	// errors keep today's values
	for _, tc := range []struct {
		o   *query_models.ReductionOverride
		err error
	}{
		{override(r, "nope", ReductionActionCheck, 0), ErrReductionClusterNotFound},
		{override(r, "k1", ReductionActionEject, 424242), ErrReductionMemberNotFound},
		{override(r, "k1", ReductionActionEject, c3), ErrReductionEjectWinner},
		{override(r, "k1", ReductionActionPromote, 424242), ErrReductionMemberNotFound},
	} {
		_, err := ctx.storeOverrideCluster(r, tc.o)
		require.ErrorIs(t, err, tc.err)
	}
	stale := override(r, "k1", ReductionActionCheck, 0)
	stale.Version = r.Version - 1
	_, err = ctx.storeOverrideCluster(r, stale)
	require.ErrorIs(t, err, ErrReductionConflict, "a page loaded before the last decision is refused, not merged")
	_, err = ctx.storeOverrideCluster(r, &query_models.ReductionOverride{ID: r.ID, Version: r.Version, ClusterID: "k1", Action: "frobnicate"})
	require.Error(t, err)
	require.Equal(t, r.Version, loadReductionRow(t, ctx.db, r.ID).Version, "a refused decision does not bump the version")

	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsOverrideActionsSQLite(t *testing.T) {
	testOpsOverrideActions(t, withSettings(newReductionSQLiteContext(t)))
}

// Edits are refused while computing or while the row is a legacy plan, on read-only
// history, on an applied Cluster, and on a Cluster that reads as stale for a
// deleted copy (check) as stale rather than unknown.
func testOpsOverrideRefusals(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h"})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "k1", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	ro := insertOps(t, ctx, r, "ro", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	require.NoError(t, ctx.db.Exec("UPDATE reduction_clusters SET read_only = ? , state = 'stale' WHERE id = ?", true, ro.Row.ID).Error)
	ap := insertOps(t, ctx, r, "ap", models.ReductionTierIdentical, a, opsMember(a, "h"), opsMember(b, "h"))
	require.NoError(t, ctx.db.Exec("UPDATE reduction_clusters SET state = 'applied', claim_id = 'x' WHERE id = ?", ap.Row.ID).Error)

	_, err := ctx.storeOverrideCluster(r, override(r, "ro", ReductionActionReopen, 0))
	require.ErrorIs(t, err, ErrReductionClusterReadOnly)
	_, err = ctx.storeOverrideCluster(r, override(r, "ap", ReductionActionUncheck, 0))
	require.ErrorIs(t, err, ErrReductionClusterSettled)

	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'computing', compute_deadline = ? WHERE id = ?", time.Now().Add(time.Hour), r.ID).Error)
	_, err = ctx.storeOverrideCluster(loadReductionRow(t, ctx.db, r.ID), override(r, "k1", ReductionActionUncheck, 0))
	require.ErrorIs(t, err, ErrReductionBusy)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'ready', plan_schema = 0 WHERE id = ?", r.ID).Error)
	_, err = ctx.storeOverrideCluster(loadReductionRow(t, ctx.db, r.ID), override(r, "k1", ReductionActionUncheck, 0))
	require.ErrorIs(t, err, ErrReductionBusy, "a legacy plan is not edited through the tables")
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET plan_schema = 1 WHERE id = ?", r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)

	// owner nulled between load and write: the CAS is owner-qualified
	owner := uint(77)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = ? WHERE id = ?", owner, r.ID).Error)
	loaded := loadReductionRow(t, ctx.db, r.ID)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = NULL WHERE id = ?", r.ID).Error)
	_, err = ctx.storeOverrideCluster(loaded, override(loaded, "k1", ReductionActionUncheck, 0))
	require.ErrorIs(t, err, ErrReductionConflict)

	// a deleted copy: reads stale, so check is refused as stale, and eject is how it is fixed
	r = loadReductionRow(t, ctx.db, r.ID)
	require.NoError(t, ctx.db.Exec("UPDATE reduction_members SET gone = ? WHERE cluster_id = (SELECT id FROM reduction_clusters WHERE \"key\" = 'k1') AND resource_id = ?", true, b).Error)
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET gone_members = 1 WHERE "key" = 'k1'`).Error)
	_, err = ctx.storeOverrideCluster(r, override(r, "k1", ReductionActionCheck, 0))
	require.ErrorIs(t, err, ErrReductionClusterStale)
	next, err := ctx.storeOverrideCluster(r, override(r, "k1", ReductionActionEject, b))
	require.NoError(t, err)
	c := loadOps(t, ctx, next, "k1")
	require.Zero(t, c.Row.GoneMembers, "the ejected copy no longer makes it stale")
	require.Equal(t, models.ReductionClusterOpen, c.Row.State)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsOverrideRefusalsSQLite(t *testing.T) {
	testOpsOverrideRefusals(t, withSettings(newReductionSQLiteContext(t)))
}

// Near promotion re-justifies against the new Winner with named-pair probes only,
// restore re-reads one pair, and an outsider is demoted, not left a Loser.
func testOpsOverrideNear(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	w := makeResource(t, ctx.db, "w", opsRes{W: 10, H: 10, Hash: "w"})
	x := makeResource(t, ctx.db, "x", opsRes{W: 20, H: 20, Hash: "x"})
	y := makeResource(t, ctx.db, "y", opsRes{W: 20, H: 20, Hash: "y"})
	out := makeResource(t, ctx.db, "out", opsRes{W: 30, H: 30, Hash: "o"})
	// a Winner with a crowd of unrelated pairs: they must never be read
	for i := 0; i < 50; i++ {
		other := makeResource(t, ctx.db, fmt.Sprintf("crowd%d", i), opsRes{Hash: "c"})
		makePair(t, ctx.db, x, other, 3)
	}
	makePair(t, ctx.db, w, x, 4)
	makePair(t, ctx.db, x, y, 6)
	// w and y are NOT paired; out is not paired to anyone.
	d := 4
	xm := opsMember(x, "x")
	xm.Distance = &d
	ym := opsMember(y, "y")
	ym.Distance = &d
	outsider := opsMember(out, "o")
	outsider.InExtent = false
	insertOps(t, ctx, r, "near", models.ReductionTierNear, out, opsMember(w, "w"), xm, ym, outsider)
	// make the Winner the outsider, as buildCluster does
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET checked = ? WHERE "key" = 'near'`, false).Error)

	var mu sync.Mutex
	var pairSQL []string
	record := func(db *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		pairSQL = append(pairSQL, db.Statement.SQL.String())
	}
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register("ops:record-row", record))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register("ops:record-query", record))
	defer ctx.db.Callback().Row().Remove("ops:record-row")
	defer ctx.db.Callback().Query().Remove("ops:record-query")

	next, err := ctx.storeOverrideCluster(r, override(r, "near", ReductionActionPromote, w))
	require.NoError(t, err)
	c := loadOps(t, ctx, next, "near")
	require.Equal(t, w, c.Row.WinnerID)
	require.True(t, memberOf(c, out).Ejected)
	require.Equal(t, models.EjectReasonOutsiderDemoted, memberOf(c, out).EjectedReason, "an outsider is demoted, not relabelled")
	require.False(t, memberOf(c, x).Ejected)
	require.Equal(t, 4, *memberOf(c, x).Distance)
	require.True(t, memberOf(c, y).Ejected, "no stored pair between y and the new Winner")
	require.Equal(t, models.EjectReasonNoPairToWinner, memberOf(c, y).EjectedReason)
	require.Nil(t, memberOf(c, w).Distance)

	mu.Lock()
	probed := 0
	for _, q := range pairSQL {
		if strings.Contains(q, "resource_similarities") {
			probed++
			require.Contains(t, q, " IN ", "a pair read names the members it probes: %s", q)
		}
	}
	require.NotZero(t, probed, "the pair reads were observed")
	mu.Unlock()

	// y has no pair to w, so it cannot be put back; the outsider never can
	r = next
	_, err = ctx.storeOverrideCluster(r, override(r, "near", ReductionActionRestore, y))
	require.ErrorIs(t, err, ErrReductionRestoreUnpaired)
	_, err = ctx.storeOverrideCluster(r, override(r, "near", ReductionActionRestore, out))
	require.ErrorIs(t, err, ErrReductionRestoreOutsider)

	// promoting x: y is paired to x, so its ejection lapses; w stays, paired to x
	next, err = ctx.storeOverrideCluster(r, override(r, "near", ReductionActionPromote, x))
	require.NoError(t, err)
	c = loadOps(t, ctx, next, "near")
	require.False(t, memberOf(c, y).Ejected)
	require.Equal(t, 6, *memberOf(c, y).Distance)
	require.False(t, memberOf(c, w).Ejected)
	require.Equal(t, 4, *memberOf(c, w).Distance)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsOverrideNearSQLite(t *testing.T) {
	testOpsOverrideNear(t, withSettings(newReductionSQLiteContext(t)))
}

// A full Near star keeps its seed: promote is refused, and a restore never makes
// the seed a Loser. A restore re-reads the member's existence.
func testOpsOverrideFullStarAndGone(t *testing.T, ctx *MahresourcesContext) {
	r := newPlanReduction(t, ctx.db)
	seed := makeResource(t, ctx.db, "seed", opsRes{Hash: "s"})
	n1 := makeResource(t, ctx.db, "n1", opsRes{Hash: "1"})
	makePair(t, ctx.db, seed, n1, 3)
	insertOps(t, ctx, r, "full", models.ReductionTierNear, seed, opsMember(seed, "s"), opsMember(n1, "1"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET fixed_seed = ?, seed_resource_id = ? WHERE "key" = 'full'`, true, seed).Error)
	mutateOps(t, ctx, r, "full", func(*storedCluster) {})

	_, err := ctx.storeOverrideCluster(r, override(r, "full", ReductionActionPromote, n1))
	require.ErrorIs(t, err, ErrReductionClusterFixedSeed)

	// a seed that is not the Winner (as an old row could be) is never put back as a Loser
	insertOps(t, ctx, r, "odd", models.ReductionTierNear, n1, opsMember(n1, "1"), ejectedMember(seed, "s"))
	require.NoError(t, ctx.db.Exec(`UPDATE reduction_clusters SET fixed_seed = ?, seed_resource_id = ? WHERE "key" = 'odd'`, true, seed).Error)
	mutateOps(t, ctx, r, "odd", func(*storedCluster) {})
	_, err = ctx.storeOverrideCluster(r, override(r, "odd", ReductionActionRestore, seed))
	require.ErrorIs(t, err, ErrReductionClusterFixedSeed)

	// restore of a member deleted while ejected: the member is marked gone and the Cluster reads stale
	del := makeResource(t, ctx.db, "del", opsRes{Hash: "h"})
	keep := makeResource(t, ctx.db, "keep", opsRes{Hash: "h"})
	insertOps(t, ctx, r, "g1", models.ReductionTierIdentical, keep, opsMember(keep, "h"), ejectedMember(del, "h"))
	require.NoError(t, ctx.db.Exec("DELETE FROM resources WHERE id = ?", del).Error)
	next, err := ctx.storeOverrideCluster(r, override(r, "g1", ReductionActionRestore, del))
	require.NoError(t, err)
	c := loadOps(t, ctx, next, "g1")
	require.True(t, memberOf(c, del).Gone)
	require.Equal(t, 1, c.Row.GoneMembers)
	assertDerivedColumnsConsistent(t, ctx.db, r.ID)
}

func TestOpsOverrideFullStarAndGoneSQLite(t *testing.T) {
	testOpsOverrideFullStarAndGone(t, withSettings(newReductionSQLiteContext(t)))
}

// ---- the differential: today's applyOverride is the oracle -----------------------

// toLegacy is the Cluster as today's JSON plan holds it.
func toLegacy(c *storedCluster) *models.ReductionCluster {
	out := &models.ReductionCluster{
		ID: c.Row.Key, Tier: c.Row.Tier, WinnerID: c.Row.WinnerID, DecidedBy: c.Row.DecidedBy, Margin: c.Row.Margin,
		Undecided: c.Row.Undecided, KeepsSmaller: c.Row.KeepsSmaller, State: c.Row.State, Checked: c.Row.Checked,
		Reviewed: c.Row.Reviewed, Oversized: c.Row.Oversized, StaleReason: c.Row.StaleReason,
	}
	if lossyFieldCount(c.Row.Lossy) > 0 {
		_ = json.Unmarshal(c.Row.Lossy, &out.Lossy)
	}
	for _, m := range c.Members {
		lm := &models.ReductionMember{ResourceID: m.ResourceID, Hash: m.Hash, InExtent: m.InExtent, Ejected: m.Ejected, EjectedReason: m.EjectedReason}
		if m.Distance != nil {
			d := uint8(*m.Distance)
			lm.Distance = &d
		}
		out.Members = append(out.Members, lm)
		if m.Changed {
			out.ChangedMembers = append(out.ChangedMembers, m.ResourceID)
		}
	}
	return out
}

func testOpsOverrideMatchesLegacy(t *testing.T, ctx *MahresourcesContext, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for trial := 0; trial < 40; trial++ {
		r := newPlanReduction(t, ctx.db)
		n := 2 + rng.Intn(4)
		ids := make([]uint, n)
		for i := range ids {
			desc := []string{"", "  ", "text"}[rng.Intn(3)]
			ids[i] = makeResource(t, ctx.db, fmt.Sprintf("t%d-%d", trial, i), opsRes{
				W: uint(rng.Intn(3) * 10), H: uint(1 + rng.Intn(3)*10), Desc: desc, Series: rng.Intn(4) == 0, Cat: uint(1 + rng.Intn(2)), Hash: "h",
			})
		}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if rng.Intn(3) > 0 {
					makePair(t, ctx.db, ids[i], ids[j], uint8(rng.Intn(14)))
				}
			}
		}
		tier := []string{models.ReductionTierIdentical, models.ReductionTierNear}[rng.Intn(2)]
		winner := ids[rng.Intn(n)]
		var members []models.ReductionMemberRow
		for _, id := range ids {
			m := opsMember(id, "h")
			if id != winner && rng.Intn(3) == 0 {
				m.Ejected, m.EjectedReason = true, []string{models.EjectReasonManual, models.EjectReasonNoPairToWinner}[rng.Intn(2)]
			}
			if id != winner && rng.Intn(5) == 0 {
				m.InExtent = false
				m.Ejected = rng.Intn(2) == 0
				if m.Ejected {
					m.EjectedReason = models.EjectReasonOutsiderDemoted
				}
			}
			if tier == models.ReductionTierNear && id != winner {
				d := rng.Intn(10)
				m.Distance = &d
			}
			members = append(members, m)
		}
		stored := insertOps(t, ctx, r, fmt.Sprintf("c%d", trial), tier, winner, members...)
		state := []string{models.ReductionClusterOpen, models.ReductionClusterOpen, models.ReductionClusterSkipped, models.ReductionClusterStale}[rng.Intn(4)]
		reason := ""
		if state == models.ReductionClusterStale {
			reason = []string{staleReasonHashChanged, staleReasonDeleted}[rng.Intn(2)]
		}
		require.NoError(t, ctx.db.Exec("UPDATE reduction_clusters SET state = ?, stale_reason = ?, checked = ?, oversized = ?, claim_id = '' WHERE id = ?",
			state, reason, rng.Intn(2) == 0, rng.Intn(4) == 0, stored.Row.ID).Error)
		mutateOps(t, ctx, r, stored.Row.Key, func(*storedCluster) {})
		before := loadOps(t, ctx, r, stored.Row.Key)
		if state == models.ReductionClusterStale && rng.Intn(2) == 0 {
			require.NoError(t, ctx.db.Exec("UPDATE reduction_members SET changed = ? WHERE cluster_id = ? AND resource_id = ?", true, stored.Row.ID, winner).Error)
			before = loadOps(t, ctx, r, stored.Row.Key)
		}

		actions := []string{ReductionActionPromote, ReductionActionEject, ReductionActionRestore, ReductionActionSkip,
			ReductionActionReopen, ReductionActionCheck, ReductionActionUncheck}
		action := actions[rng.Intn(len(actions))]
		target := ids[rng.Intn(n)]
		o := override(r, stored.Row.Key, action, target)
		o.AcknowledgeOversized = rng.Intn(2) == 0

		legacy := toLegacy(before)
		legacyErr := ctx.applyOverride(legacy, o, storedWinnerRule(r))
		_, tableErr := ctx.storeOverrideCluster(r, o)

		label := fmt.Sprintf("seed %d trial %d %s tier=%s state=%s", seed, trial, action, tier, state)
		if legacyErr != nil {
			require.Error(t, tableErr, label)
			require.Equal(t, legacyErr.Error(), tableErr.Error(), label)
			assertDerivedColumnsConsistent(t, ctx.db, r.ID)
			continue
		}
		require.NoError(t, tableErr, label)
		after := loadOps(t, ctx, r, stored.Row.Key)
		require.Equal(t, legacy.State, after.Row.State, label)
		require.Equal(t, legacy.StaleReason, after.Row.StaleReason, label)
		require.Equal(t, legacy.Checked, after.Row.Checked, label)
		require.Equal(t, legacy.Reviewed, after.Row.Reviewed, label)
		require.Equal(t, legacy.WinnerID, after.Row.WinnerID, label)
		require.Equal(t, legacy.DecidedBy, after.Row.DecidedBy, label)
		require.Equal(t, legacy.Margin, after.Row.Margin, label)
		require.Equal(t, legacy.Undecided, after.Row.Undecided, label)
		require.Equal(t, legacy.KeepsSmaller, after.Row.KeepsSmaller, label)
		var gotLossy []string
		if lossyFieldCount(after.Row.Lossy) > 0 {
			require.NoError(t, json.Unmarshal(after.Row.Lossy, &gotLossy))
		}
		sort.Strings(gotLossy)
		wantLossy := append([]string(nil), legacy.Lossy...)
		sort.Strings(wantLossy)
		require.Equal(t, wantLossy, gotLossy, label)
		var gotChanged []uint
		for _, m := range after.Members {
			lm := legacy.Members[0]
			for _, cand := range legacy.Members {
				if cand.ResourceID == m.ResourceID {
					lm = cand
				}
			}
			require.Equal(t, lm.Ejected, m.Ejected, "%s ejected of %d", label, m.ResourceID)
			require.Equal(t, lm.EjectedReason, m.EjectedReason, "%s reason of %d", label, m.ResourceID)
			if lm.Distance == nil {
				require.Nil(t, m.Distance, "%s distance of %d", label, m.ResourceID)
			} else {
				require.NotNil(t, m.Distance, "%s distance of %d", label, m.ResourceID)
				require.Equal(t, int(*lm.Distance), *m.Distance, "%s distance of %d", label, m.ResourceID)
			}
			if m.Changed {
				gotChanged = append(gotChanged, m.ResourceID)
			}
		}
		require.Equal(t, len(legacy.ChangedMembers), len(gotChanged), label)
		assertDerivedColumnsConsistent(t, ctx.db, r.ID)
	}
}

func TestOpsOverrideMatchesLegacySQLite(t *testing.T) {
	for _, seed := range []int64{1, 2, 3} {
		testOpsOverrideMatchesLegacy(t, withSettings(newReductionSQLiteContext(t)), seed)
	}
}
