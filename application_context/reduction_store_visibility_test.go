package application_context

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The store decides visibility with one subtree CTE; the scope callback decides it
// with a flattened allow-list. For every Resource, under every kind of principal,
// the two must agree: a one-member Cluster is withheld exactly when the scoped
// handle cannot read that Resource.
func testReductionVisibilityMatchesScopeCallback(t *testing.T, newCtx func(*testing.T) *MahresourcesContext, seeds []int64) {
	for _, seed := range seeds {
		ctx := newCtx(t)
		rng := rand.New(rand.NewSource(seed))
		w := newReductionWorld(t, ctx, rng, 12+rng.Intn(10), 40, seed%2 == 0)
		r := newReductionRow(t, ctx.db)
		gen := fmt.Sprintf("g%d", seed)
		for i, id := range w.resources {
			c := &storedCluster{Row: models.ReductionClusterRow{Key: fmt.Sprintf("one-%d", id), Tier: "identical", State: "open", WinnerID: id, Position: int64(i)},
				Members: []models.ReductionMemberRow{{ResourceID: id, Hash: "h", Gone: !w.exists[id]}}}
			require.NoError(t, ctx.reductionStore().insertCluster(r, gen, c))
		}
		for name, p := range w.principals() {
			pctx := ctx.WithPrincipal(p)
			var readable []uint
			require.NoError(t, pctx.db.Model(&models.Resource{}).Pluck("id", &readable).Error)
			want := map[string]bool{}
			for _, id := range readable {
				want[fmt.Sprintf("one-%d", id)] = true
			}
			if !pctx.isScopedPrincipal() {
				// Unconfined: nothing is withheld (a deleted member reads as stale).
				for _, id := range w.resources {
					want[fmt.Sprintf("one-%d", id)] = true
				}
			}
			rows, total, err := pctx.reductionStore().pageClusters(r, gen, &query_models.ResourceReductionReviewQuery{Status: []string{"all"}}, 0, 1000)
			require.NoError(t, err)
			got := map[string]bool{}
			for _, k := range clusterRowKeys(rows) {
				got[k] = true
			}
			assert.Equal(t, len(want), total, "seed %d principal %s", seed, name)
			assert.Equal(t, want, got, "seed %d principal %s", seed, name)
		}
	}
}

func TestReductionVisibilityMatchesScopeCallbackSQLite(t *testing.T) {
	testReductionVisibilityMatchesScopeCallback(t, newReductionSQLiteContext, []int64{1, 2, 3, 4, 5, 6})
}

// Step 0 of the design (D2): a GORM subquery handed to a statement as a `?` argument
// runs through the scope callback, a string fragment naming the same table does not.
// The store therefore names `resources` in raw SQL and scopes it itself, and this
// negative control shows the string form is the one that leaks.
func testReductionSubqueryArgGetsScopeCallback(t *testing.T, ctx *MahresourcesContext) {
	scope := &models.Group{Name: "scope"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(scope).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	in := &models.Resource{Name: "in", OwnerId: &scope.ID}
	out := &models.Resource{Name: "out", OwnerId: &other.ID}
	require.NoError(t, ctx.db.Create(in).Error)
	require.NoError(t, ctx.db.Create(out).Error)
	r := newReductionRow(t, ctx.db)
	c := &storedCluster{Row: models.ReductionClusterRow{Key: "k", WinnerID: in.ID},
		Members: []models.ReductionMemberRow{{ResourceID: in.ID}, {ResourceID: out.ID}}}
	require.NoError(t, ctx.reductionStore().insertCluster(r, "g", c))

	pctx := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &scope.ID})
	var viaSubquery, viaFragment int64
	require.NoError(t, pctx.db.Model(&models.ReductionMemberRow{}).
		Where("resource_id IN (?)", pctx.db.Model(&models.Resource{}).Select("id")).Count(&viaSubquery).Error)
	require.NoError(t, pctx.db.Model(&models.ReductionMemberRow{}).
		Where("resource_id IN (SELECT id FROM resources)").Count(&viaFragment).Error)
	assert.EqualValues(t, 1, viaSubquery, "the subquery argument was scoped by the callback")
	assert.EqualValues(t, 2, viaFragment, "the string fragment was not: it sees the out-of-subtree Resource")
}

func TestReductionSubqueryArgGetsScopeCallbackSQLite(t *testing.T) {
	testReductionSubqueryArgGetsScopeCallback(t, newReductionSQLiteContext(t))
}

// Every statement shape, as a confined principal, with an out-of-subtree twin of
// each Cluster: the twin never appears in any result or count.
func testReductionConfinedShapesNeverShowTheTwin(t *testing.T, ctx *MahresourcesContext) {
	scope := &models.Group{Name: "scope"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(scope).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	r := newReductionRow(t, ctx.db)
	owned := func(owner uint, name string) uint {
		res := &models.Resource{Name: name, OwnerId: &owner}
		require.NoError(t, ctx.db.Create(res).Error)
		return res.ID
	}
	admin := ctx.reductionStore()
	pos := int64(0)
	for _, owner := range []struct {
		prefix string
		group  uint
	}{{"mine", scope.ID}, {"twin", other.ID}} {
		for _, shape := range []struct {
			state  string
			merged bool
		}{{"open", false}, {"applied", false}, {"applied", true}} {
			a, b := owned(owner.group, "a"), owned(owner.group, "b")
			c := &storedCluster{Row: models.ReductionClusterRow{
				Key: fmt.Sprintf("%s-%s-%v", owner.prefix, shape.state, shape.merged), Tier: "identical", State: shape.state,
				Merged: shape.merged, Checked: true, WinnerID: a, Position: pos, OutcomeBatch: "batch-1"},
				Members: []models.ReductionMemberRow{{ResourceID: a, Hash: "h"}, {ResourceID: b, Hash: "h"}}}
			pos++
			require.NoError(t, admin.insertCluster(r, "g", c))
		}
	}
	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &scope.ID})
	s := p.reductionStore()

	rows, total, err := s.pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{Status: []string{"all"}}, 0, 100)
	require.NoError(t, err)
	// merged, unconfirmed applied clusters are filtered out by status "all"? No: all is no filter.
	for _, k := range clusterRowKeys(rows) {
		assert.NotContains(t, k, "twin")
	}
	assert.Equal(t, 3, total)
	tabs, err := s.tabCounts(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 1, tabs.Open)
	assert.Equal(t, 1, tabs.Applied)
	assert.Equal(t, 1, tabs.Merging, "only the viewer's own claim is counted")
	figures, err := s.figures(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 1, figures.Checked)
	_, clusters, _, err := s.applyEligibility(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 1, clusters)
	n, err := s.settleableClaims(r, "g", time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	visible, err := s.lastApplyCountsVisible(r, "g", "batch-1", 6)
	require.NoError(t, err)
	assert.False(t, visible, "counts that include a twin's outcomes are hidden")
	_, err = s.loadCluster(r, "g", "twin-open-false")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "a twin is not found, as an unknown key is not")
	_, err = s.loadCluster(r, "g", "mine-open-false")
	assert.NoError(t, err)
}

func TestReductionConfinedShapesNeverShowTheTwinSQLite(t *testing.T) {
	testReductionConfinedShapesNeverShowTheTwin(t, newReductionSQLiteContext(t))
}

// A 20,000-Group subtree: every statement shape answers correctly with one bind
// for the subtree, where an allow-list would carry 20,000.
func testReductionTwentyThousandGroupSubtree(t *testing.T, ctx *MahresourcesContext) {
	root := &models.Group{Name: "root"}
	require.NoError(t, ctx.db.Create(root).Error)
	kids := make([]models.Group, 20000)
	for i := range kids {
		kids[i] = models.Group{Name: fmt.Sprintf("k%d", i), OwnerId: &root.ID}
	}
	require.NoError(t, ctx.db.CreateInBatches(kids, 1000).Error)
	outside := &models.Group{Name: "outside"}
	require.NoError(t, ctx.db.Create(outside).Error)
	r := newReductionRow(t, ctx.db)
	mk := func(key string, owner uint, i int64) {
		a := &models.Resource{Name: key + "a", OwnerId: &owner}
		b := &models.Resource{Name: key + "b", OwnerId: &owner}
		require.NoError(t, ctx.db.Create(a).Error)
		require.NoError(t, ctx.db.Create(b).Error)
		c := &storedCluster{Row: models.ReductionClusterRow{Key: key, Tier: "identical", State: "open", Checked: true, WinnerID: a.ID, Position: i},
			Members: []models.ReductionMemberRow{{ResourceID: a.ID, Hash: "h"}, {ResourceID: b.ID, Hash: "h"}}}
		require.NoError(t, ctx.reductionStore().insertCluster(r, "g", c))
	}
	mk("deep", kids[19999].ID, 0)
	mk("shallow", kids[0].ID, 1)
	mk("away", outside.ID, 2)

	var mu sync.Mutex
	maxBinds := 0
	record := func(tx *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		if n := len(tx.Statement.Vars); n > maxBinds {
			maxBinds = n
		}
	}
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register("test:binds_row", record))
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register("test:binds_query", record))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Row().Remove("test:binds_row")
		_ = ctx.db.Callback().Query().Remove("test:binds_query")
	})

	p := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, UserID: 5, ScopeGroupID: &root.ID})
	s := p.reductionStore()
	rows, total, err := s.pageClusters(r, "g", &query_models.ResourceReductionReviewQuery{}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.ElementsMatch(t, []string{"deep", "shallow"}, clusterRowKeys(rows))
	tabs, err := s.tabCounts(r, "g")
	require.NoError(t, err)
	assert.Equal(t, 2, tabs.Open)
	_, err = s.figures(r, "g")
	require.NoError(t, err)
	_, _, _, err = s.applyEligibility(r, "g")
	require.NoError(t, err)
	_, err = s.settleableClaims(r, "g", time.Now())
	require.NoError(t, err)
	_, err = s.loadCluster(r, "g", "deep")
	require.NoError(t, err)
	assert.LessOrEqual(t, maxBinds, 12, "the subtree is bound once, not flattened into an IN list")
	assert.NotZero(t, maxBinds, "the bind counter saw the store's statements")
}

func TestReductionTwentyThousandGroupSubtreeSQLite(t *testing.T) {
	testReductionTwentyThousandGroupSubtree(t, newReductionSQLiteContext(t))
}
