package application_context

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

// The reduction store's tests run the same bodies on SQLite and Postgres: a body
// takes a context, the SQLite wrapper builds it here and the Postgres wrapper in
// reduction_store_pg_test.go.

// reductionTables are the plan tables the store reads and writes.
var reductionTables = []any{
	&models.ResourceReduction{},
	&models.ReductionClusterRow{},
	&models.ReductionMemberRow{},
	&models.ReductionExtentResource{},
	&models.ReductionExtentGroup{},
	&models.ReductionExtentRow{},
	&models.ReductionSeed{},
	&models.ReductionDeletedID{},
	&models.ResourceCleanupManifest{},
	&models.ReductionMigrationRow{},
	&models.ReductionMigration{},
	// The legacy plan and extent columns the migration reads: no other model names them.
	&legacyReduction{},
}

// newReductionSQLiteContext is a context with the plan tables migrated.
func newReductionSQLiteContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := createTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(reductionTables...))
	return ctx
}

// ---- a random world: a Group tree, Resources, and a plan over them ----------------

type reductionWorld struct {
	ctx *MahresourcesContext
	rng *rand.Rand

	groups    []uint
	resources []uint          // every id used by a member, whether or not the row exists
	exists    map[uint]bool   // the Resource row exists
	owner     map[uint]uint   // owner group, 0 for none
	size      map[uint]int64  // file size of an existing Resource
	children  map[uint][]uint // the Group tree as created (cycles not reflected)
}

// newReductionWorld builds nGroups Groups in a random forest (each hangs off an
// earlier Group or is a root), nResources Resources with a random owner (none
// for some), and removes some Resources and some Groups, so the world has deleted
// members, a deleted scope root and orphaned subtrees. When cycle is set two
// Groups are made each other's parent.
func newReductionWorld(t *testing.T, ctx *MahresourcesContext, rng *rand.Rand, nGroups, nResources int, cycle bool) *reductionWorld {
	t.Helper()
	w := &reductionWorld{ctx: ctx, rng: rng, exists: map[uint]bool{}, owner: map[uint]uint{}, size: map[uint]int64{}, children: map[uint][]uint{}}
	db := ctx.db
	for i := 0; i < nGroups; i++ {
		g := &models.Group{Name: fmt.Sprintf("g%d", i)}
		if len(w.groups) > 0 && rng.Intn(5) > 0 {
			parent := w.groups[rng.Intn(len(w.groups))]
			g.OwnerId = &parent
		}
		require.NoError(t, db.Create(g).Error)
		w.groups = append(w.groups, g.ID)
		if g.OwnerId != nil {
			w.children[*g.OwnerId] = append(w.children[*g.OwnerId], g.ID)
		}
	}
	if cycle && len(w.groups) >= 2 {
		a, b := w.groups[0], w.groups[1]
		require.NoError(t, db.Exec("UPDATE groups SET owner_id = ? WHERE id = ?", b, a).Error)
		require.NoError(t, db.Exec("UPDATE groups SET owner_id = ? WHERE id = ?", a, b).Error)
	}
	for i := 0; i < nResources; i++ {
		res := &models.Resource{Name: fmt.Sprintf("r%d", i), FileSize: int64(rng.Intn(1000)), Hash: fmt.Sprintf("h%d", rng.Intn(6))}
		if len(w.groups) > 0 && rng.Intn(6) > 0 {
			owner := w.groups[rng.Intn(len(w.groups))]
			res.OwnerId = &owner
		}
		require.NoError(t, db.Create(res).Error)
		w.resources = append(w.resources, res.ID)
		w.exists[res.ID] = true
		w.size[res.ID] = res.FileSize
		if res.OwnerId != nil {
			w.owner[res.ID] = *res.OwnerId
		}
	}
	// Delete some Resources and, afterwards, some Groups.
	for _, id := range w.resources {
		if rng.Intn(7) == 0 {
			require.NoError(t, db.Exec("DELETE FROM resources WHERE id = ?", id).Error)
			w.exists[id] = false
			w.size[id] = 0
		}
	}
	if len(w.groups) > 3 && rng.Intn(2) == 0 {
		victim := w.groups[rng.Intn(len(w.groups))]
		require.NoError(t, db.Exec("UPDATE resources SET owner_id = NULL WHERE owner_id = ?", victim).Error)
		require.NoError(t, db.Exec("UPDATE groups SET owner_id = NULL WHERE owner_id = ?", victim).Error)
		require.NoError(t, db.Exec("DELETE FROM groups WHERE id = ?", victim).Error)
		for id, o := range w.owner {
			if o == victim {
				w.owner[id] = 0
			}
		}
	}
	return w
}

// principals is one of each kind a Reduction is read by: the unconfined ones, a
// group-limited user at several Groups (including a deleted one), and a guest with
// no scope group, which fails closed.
func (w *reductionWorld) principals() map[string]*auth.Principal {
	out := map[string]*auth.Principal{
		"system": auth.SystemPrincipal(),
		"admin":  {Role: models.RoleAdmin, UserID: 1},
		"user":   {Role: models.RoleUser, UserID: 2},
		"deny":   {Role: models.RoleGuest, UserID: 3},
	}
	for i := 0; i < 3 && len(w.groups) > 0; i++ {
		scope := w.groups[w.rng.Intn(len(w.groups))]
		out[fmt.Sprintf("scoped-%d", i)] = &auth.Principal{Role: models.RoleUser, UserID: uint(10 + i), ScopeGroupID: &scope}
	}
	gone := uint(987654)
	out["deleted-root"] = &auth.Principal{Role: models.RoleGuest, UserID: 20, ScopeGroupID: &gone}
	return out
}

// randomPlan builds a plan over the world's Resources: every Resource is in at
// most one Cluster (the invariant availability gives a real plan, and the one the
// oracle's id-keyed visibility map relies on), with every state, tier and flag.
func (w *reductionWorld) randomPlan(t *testing.T) []*models.ReductionCluster {
	ids := append([]uint(nil), w.resources...)
	w.rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	// Resource ids that never had a row, so a member can name a Resource that is
	// simply not there.
	for i := 0; i < 4; i++ {
		ids = append(ids, 500000+uint(w.rng.Intn(1000))+uint(i)*1000)
	}
	var clusters []*models.ReductionCluster
	for n := 0; len(ids) > 0; n++ {
		size := 1 + w.rng.Intn(5)
		if size > len(ids) {
			size = len(ids)
		}
		members := ids[:size]
		ids = ids[size:]
		c := &models.ReductionCluster{
			ID:   fmt.Sprintf("cl-%04d-%x", n, w.rng.Intn(1<<20)),
			Tier: []string{models.ReductionTierIdentical, models.ReductionTierNear}[w.rng.Intn(2)],
		}
		c.State = []string{models.ReductionClusterOpen, models.ReductionClusterOpen, models.ReductionClusterOpen,
			models.ReductionClusterSkipped, models.ReductionClusterApplied, models.ReductionClusterStale}[w.rng.Intn(6)]
		c.Checked = w.rng.Intn(2) == 0
		c.Reviewed = w.rng.Intn(3) == 0
		c.Oversized = w.rng.Intn(6) == 0
		c.KeepsSmaller = w.rng.Intn(6) == 0
		if w.rng.Intn(5) == 0 {
			c.Lossy = []string{"description"}
		}
		for i, id := range members {
			m := &models.ReductionMember{ResourceID: id, Hash: fmt.Sprintf("hash-%d", w.rng.Intn(4)), InExtent: true}
			if i > 0 && w.rng.Intn(5) == 0 {
				m.Ejected = true
				m.EjectedReason = models.EjectReasonManual
			}
			if c.Tier == models.ReductionTierNear && i > 0 {
				d := uint8(w.rng.Intn(10))
				m.Distance = &d
			}
			c.Members = append(c.Members, m)
		}
		c.WinnerID = members[0] // never ejected: i > 0 only
		if c.State == models.ReductionClusterApplied {
			c.Merged = w.rng.Intn(3) > 0
			c.ClaimID = fmt.Sprintf("claim-%d", n)
			if c.Merged {
				c.LoserOwners = map[uint]uint{}
				for _, m := range c.Members {
					if !m.IsLoser(c.WinnerID) {
						continue
					}
					// A merged Loser is gone; its recorded owner is anything: none,
					// a live Group, a deleted one.
					switch w.rng.Intn(4) {
					case 0: // owner unknown
					case 1:
						c.LoserOwners[m.ResourceID] = 0
					default:
						if len(w.groups) > 0 {
							c.LoserOwners[m.ResourceID] = w.groups[w.rng.Intn(len(w.groups))]
						}
					}
					if w.exists[m.ResourceID] && w.rng.Intn(2) == 0 {
						require.NoError(t, w.ctx.db.Exec("DELETE FROM resources WHERE id = ?", m.ResourceID).Error)
						w.exists[m.ResourceID] = false
						w.size[m.ResourceID] = 0
					}
				}
			}
		}
		clusters = append(clusters, c)
	}
	return clusters
}

// storedFromPlan writes a plan cluster as rows: sizes and gone flags come from the
// world as it is now, the owner recorded at merge from LoserOwners.
func (w *reductionWorld) storedFromPlan(c *models.ReductionCluster, position int64) *storedCluster {
	sc := &storedCluster{Row: models.ReductionClusterRow{
		Key: c.ID, Position: position, Tier: c.Tier, State: c.State, WinnerID: c.WinnerID,
		Checked: c.Checked, Reviewed: c.Reviewed, Oversized: c.Oversized, DecidedBy: c.DecidedBy,
		Margin: c.Margin, Undecided: c.Undecided, KeepsSmaller: c.KeepsSmaller,
		StaleReason: c.StaleReason, Merged: c.Merged, ClaimID: c.ClaimID,
	}}
	if len(c.Lossy) > 0 {
		raw := `["` + c.Lossy[0] + `"]`
		sc.Row.Lossy = []byte(raw)
	}
	for _, m := range c.Members {
		sc.Members = append(sc.Members, models.ReductionMemberRow{
			ResourceID: m.ResourceID, Hash: m.Hash, FileSize: w.size[m.ResourceID], InExtent: m.InExtent,
			Ejected: m.Ejected, EjectedReason: m.EjectedReason, OwnerAtMerge: c.LoserOwners[m.ResourceID],
			Gone: !w.exists[m.ResourceID],
		})
		if m.Distance != nil {
			d := int(*m.Distance)
			sc.Members[len(sc.Members)-1].Distance = &d
		}
	}
	return sc
}

// newReduction creates the Reduction row the store's methods take.
func newReductionRow(t *testing.T, db *gorm.DB) *models.ResourceReduction {
	t.Helper()
	r := &models.ResourceReduction{Name: "store test", Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, db.Create(r).Error)
	return r
}

// derivedCheckT is what assertDerivedColumnsConsistent needs of a test, so its own
// negative control can run it against a recorder.
type derivedCheckT interface {
	require.TestingT
	Helper()
}

// assertDerivedColumnsConsistent recomputes every Cluster's derived columns from
// its member rows and requires the stored ones to match. Every store, override,
// bulk, apply, merge, hook, migration and compute test ends with it.
func assertDerivedColumnsConsistent(t derivedCheckT, db *gorm.DB, reductionID uint) {
	t.Helper()
	var rows []models.ReductionClusterRow
	require.NoError(t, db.Where("reduction_id = ?", reductionID).Order("id").Find(&rows).Error)
	for _, row := range rows {
		var members []models.ReductionMemberRow
		require.NoError(t, db.Where("cluster_id = ?", row.ID).Order("position, id").Find(&members).Error)
		want := storedCluster{Row: row, Members: members}
		deriveClusterColumns(&want)
		got := row
		require.Equal(t, want.Row.MemberCount, got.MemberCount, "member_count of %s", row.Key)
		require.Equal(t, want.Row.LoserCount, got.LoserCount, "loser_count of %s", row.Key)
		require.Equal(t, want.Row.NeedsAttention, got.NeedsAttention, "needs_attention of %s", row.Key)
		require.Equal(t, want.Row.Fingerprint, got.Fingerprint, "fingerprint of %s", row.Key)
		require.Equal(t, want.Row.TotalSize, got.TotalSize, "total_size of %s", row.Key)
		require.Equal(t, want.Row.GoneMembers, got.GoneMembers, "gone_members of %s", row.Key)
	}
}

func clusterRowKeys(rows []models.ReductionClusterRow) []string {
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.Key
	}
	return keys
}

func planKeys(clusters []*models.ReductionCluster) []string {
	keys := make([]string, len(clusters))
	for i, c := range clusters {
		keys[i] = c.ID
	}
	return keys
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
