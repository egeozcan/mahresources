package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Issue #91: leaving a Series auto-deletes it when no member remains. The
// "no member remains" check used to be a Count on resources through the
// caller's handle, which the subtree scope callback filters by owner. A
// group-limited principal therefore saw members outside its subtree as absent
// and deleted a Series those members still belonged to.

type seriesScopeFixture struct {
	scopeGroup   *models.Group
	series       *models.Series
	outsideOwned *models.Resource // member of series, owned outside the subtree
	inside       *models.Resource // owned inside the subtree, not in series
}

func newSeriesScopeFixture(t *testing.T, ctx *MahresourcesContext, withOutsideMember bool) seriesScopeFixture {
	t.Helper()
	scopeGroup := &models.Group{Name: "series-scope-inside"}
	if err := ctx.db.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	outsideGroup := &models.Group{Name: "series-scope-outside"}
	if err := ctx.db.Create(outsideGroup).Error; err != nil {
		t.Fatalf("create outside group: %v", err)
	}
	series := &models.Series{Name: "Shared Series", Slug: "shared-series-scope", Meta: []byte(`{"shared":true}`)}
	if err := ctx.db.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	f := seriesScopeFixture{scopeGroup: scopeGroup, series: series}
	if withOutsideMember {
		f.outsideOwned = &models.Resource{
			Name: "outside member", OwnerId: &outsideGroup.ID, SeriesID: &series.ID,
			Meta: []byte(`{"shared":true}`), OwnMeta: []byte(`{}`),
		}
		if err := ctx.db.Create(f.outsideOwned).Error; err != nil {
			t.Fatalf("create outside member: %v", err)
		}
	}
	f.inside = &models.Resource{Name: "inside resource", OwnerId: &scopeGroup.ID, Meta: []byte(`{}`), OwnMeta: []byte(`{}`)}
	if err := ctx.db.Create(f.inside).Error; err != nil {
		t.Fatalf("create inside resource: %v", err)
	}
	return f
}

func (f seriesScopeFixture) principal(t *testing.T, ctx *MahresourcesContext, role models.Role) *auth.Principal {
	return accountPrincipal(t, ctx, "series-scoped", role, &f.scopeGroup.ID)
}

func (f seriesScopeFixture) editInside(t *testing.T, ctx *MahresourcesContext, seriesID uint) {
	t.Helper()
	if _, err := ctx.EditResource(&query_models.ResourceEditor{
		ID: f.inside.ID,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: f.inside.Name, Meta: `{}`, OwnerId: f.scopeGroup.ID, SeriesId: seriesID,
		},
	}); err != nil {
		t.Fatalf("scoped EditResource(SeriesId=%d): %v", seriesID, err)
	}
}

func seriesExists(t *testing.T, ctx *MahresourcesContext, id uint) bool {
	t.Helper()
	var count int64
	if err := ctx.db.Model(&models.Series{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count series: %v", err)
	}
	return count > 0
}

// assertSeriesSurvivedScopedLeave checks, on the unscoped context, that the
// Series and its out-of-scope member are intact and still writable.
func assertSeriesSurvivedScopedLeave(t *testing.T, ctx *MahresourcesContext, f seriesScopeFixture) {
	t.Helper()
	if !seriesExists(t, ctx, f.series.ID) {
		t.Fatal("a scoped principal's leave deleted a Series that still has a member outside its subtree")
	}
	var outside models.Resource
	if err := ctx.db.First(&outside, f.outsideOwned.ID).Error; err != nil {
		t.Fatalf("reload outside member: %v", err)
	}
	if outside.SeriesID == nil || *outside.SeriesID != f.series.ID {
		t.Fatalf("outside member series_id = %v, want %d", outside.SeriesID, f.series.ID)
	}
	if _, err := ctx.EditResource(&query_models.ResourceEditor{
		ID: outside.ID,
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name: "outside member renamed", Meta: `{}`, OwnerId: *outside.OwnerId, SeriesId: f.series.ID,
		},
	}); err != nil {
		t.Fatalf("unscoped EditResource on outside member: %v", err)
	}
	if err := ctx.DeleteResource(outside.ID); err != nil {
		t.Fatalf("unscoped DeleteResource on outside member: %v", err)
	}
	if seriesExists(t, ctx, f.series.ID) {
		t.Fatal("deleting the last member did not auto-delete the Series")
	}
}

func runScopedEditLeaveKeepsSeriesWithOutsideMembers(t *testing.T, ctx *MahresourcesContext) {
	f := newSeriesScopeFixture(t, ctx, true)
	scoped := ctx.WithPrincipal(f.principal(t, ctx, models.RoleUser))

	f.editInside(t, scoped, f.series.ID) // join
	f.editInside(t, scoped, 0)           // leave

	assertSeriesSurvivedScopedLeave(t, ctx, f)
}

func runScopedEditLeaveDeletesSeriesWhenSoleMember(t *testing.T, ctx *MahresourcesContext) {
	f := newSeriesScopeFixture(t, ctx, false)
	scoped := ctx.WithPrincipal(f.principal(t, ctx, models.RoleUser))

	f.editInside(t, scoped, f.series.ID)
	f.editInside(t, scoped, 0)

	if seriesExists(t, ctx, f.series.ID) {
		t.Fatal("the sole member leaving did not auto-delete the Series")
	}
}

// RemoveResourceFromSeries is an editor operation, and stored editors cannot be
// scoped (normalizeScopeGroup), so the principal is a synthetic scoped editor,
// as in relation_scope_test.go: it measures the scope rule alone.
func runScopedRemoveFromSeriesKeepsSeriesWithOutsideMembers(t *testing.T, ctx *MahresourcesContext) {
	f := newSeriesScopeFixture(t, ctx, true)
	if err := ctx.db.Model(&models.Resource{}).Where("id = ?", f.inside.ID).Update("series_id", f.series.ID).Error; err != nil {
		t.Fatalf("join inside resource: %v", err)
	}
	scoped := ctx.WithPrincipal(f.principal(t, ctx, models.RoleEditor))

	if err := scoped.RemoveResourceFromSeries(f.inside.ID); err != nil {
		t.Fatalf("scoped RemoveResourceFromSeries: %v", err)
	}

	assertSeriesSurvivedScopedLeave(t, ctx, f)
}

func runScopedRemoveFromSeriesDeletesSeriesWhenSoleMember(t *testing.T, ctx *MahresourcesContext) {
	f := newSeriesScopeFixture(t, ctx, false)
	if err := ctx.db.Model(&models.Resource{}).Where("id = ?", f.inside.ID).Update("series_id", f.series.ID).Error; err != nil {
		t.Fatalf("join inside resource: %v", err)
	}
	scoped := ctx.WithPrincipal(f.principal(t, ctx, models.RoleEditor))

	if err := scoped.RemoveResourceFromSeries(f.inside.ID); err != nil {
		t.Fatalf("scoped RemoveResourceFromSeries: %v", err)
	}
	if seriesExists(t, ctx, f.series.ID) {
		t.Fatal("the sole member leaving did not auto-delete the Series")
	}
}

func TestScopedEditResourceLeaveKeepsSeriesWithOutsideMembers(t *testing.T) {
	runScopedEditLeaveKeepsSeriesWithOutsideMembers(t, createTestContext(t))
}

func TestScopedEditResourceLeaveDeletesSeriesWhenSoleMember(t *testing.T) {
	runScopedEditLeaveDeletesSeriesWhenSoleMember(t, createTestContext(t))
}

func TestScopedRemoveFromSeriesKeepsSeriesWithOutsideMembers(t *testing.T) {
	runScopedRemoveFromSeriesKeepsSeriesWithOutsideMembers(t, createTestContext(t))
}

func TestScopedRemoveFromSeriesDeletesSeriesWhenSoleMember(t *testing.T) {
	runScopedRemoveFromSeriesDeletesSeriesWhenSoleMember(t, createTestContext(t))
}
