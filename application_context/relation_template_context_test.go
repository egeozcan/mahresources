package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"
)

// relationsWorld is a small relation graph around one carrier group:
//
//	carrier --Depends on (svc)--> auth, queue
//	carrier --Depends on (team)--> team      (same type name, other categories)
//	carrier --Blocks--> release
//	upstream --Blocks--> carrier             (incoming)
type relationsWorld struct {
	ctx                                         *MahresourcesContext
	svc, teamCat                                *models.Category
	dependsSvc, dependsTeam, blocks             *models.GroupRelationType
	carrier, auth, queue, team, release, upstrm *models.Group
}

func newRelationsWorld(t *testing.T, ctx *MahresourcesContext) *relationsWorld {
	t.Helper()
	w := &relationsWorld{ctx: ctx}
	mk := func(v any) {
		t.Helper()
		if err := ctx.db.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	w.svc = &models.Category{Name: "Service"}
	w.teamCat = &models.Category{Name: "Team"}
	mk(w.svc)
	mk(w.teamCat)
	w.dependsSvc = &models.GroupRelationType{Name: "Depends on", FromCategoryId: &w.svc.ID, ToCategoryId: &w.svc.ID}
	w.dependsTeam = &models.GroupRelationType{Name: "Depends on", FromCategoryId: &w.svc.ID, ToCategoryId: &w.teamCat.ID}
	w.blocks = &models.GroupRelationType{Name: "Blocks"}
	mk(w.dependsSvc)
	mk(w.dependsTeam)
	mk(w.blocks)
	group := func(name string, cat *models.Category) *models.Group {
		g := &models.Group{Name: name, Description: name + " description", Meta: types.JSON(`{"status":"` + name + `-ok"}`)}
		if cat != nil {
			g.CategoryId = &cat.ID
		}
		mk(g)
		return g
	}
	w.carrier = group("carrier", w.svc)
	w.auth = group("auth", w.svc)
	w.queue = group("queue", w.svc)
	w.team = group("team", w.teamCat)
	w.release = group("release", nil)
	w.upstrm = group("upstream", w.svc)
	w.edge(t, w.carrier, w.queue, w.dependsSvc, "q", "")
	w.edge(t, w.carrier, w.auth, w.dependsSvc, "tokens", "signs them")
	w.edge(t, w.carrier, w.team, w.dependsTeam, "", "")
	w.edge(t, w.carrier, w.release, w.blocks, "", "")
	w.edge(t, w.upstrm, w.carrier, w.blocks, "up", "")
	return w
}

func (w *relationsWorld) edge(t *testing.T, from, to *models.Group, typ *models.GroupRelationType, name, description string) *models.GroupRelation {
	t.Helper()
	r := &models.GroupRelation{Name: name, Description: description, FromGroupId: &from.ID, ToGroupId: &to.ID, RelationTypeId: &typ.ID}
	if err := w.ctx.db.Create(r).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	return r
}

func listRelations(t *testing.T, ctx *MahresourcesContext, req shortcodes.RelationsRequest) []shortcodes.GroupRelationEntry {
	t.Helper()
	entries, err := ctx.ListGroupRelationsForTemplate(context.Background(), req)
	if err != nil {
		t.Fatalf("ListGroupRelationsForTemplate(%+v): %v", req, err)
	}
	return entries
}

// relationsSummary renders entries as "Type>GroupName" pairs, in order.
func relationsSummary(entries []shortcodes.GroupRelationEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, e.Type+">"+e.GroupName)
	}
	return strings.Join(parts, ",")
}

func TestRelationsForTemplateListing(t *testing.T) {
	runRelationsForTemplateListing(t, createTestContextWithPlugins(t, t.TempDir()))
}

func runRelationsForTemplateListing(t *testing.T, ctx *MahresourcesContext) {
	w := newRelationsWorld(t, ctx)
	out := shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: shortcodes.RelationsDirectionOut, Limit: 20}

	// Type name first, then creation order (id): queue's edge was made before auth's.
	entries := listRelations(t, ctx, out)
	if got, want := relationsSummary(entries), "Blocks>release,Depends on>queue,Depends on>auth,Depends on>team"; got != want {
		t.Fatalf("out = %s, want %s", got, want)
	}
	authEdge := entries[2]
	want := shortcodes.GroupRelationEntry{Direction: "out", Type: "Depends on", Name: "tokens", Description: "signs them",
		GroupID: w.auth.ID, GroupName: "auth", GroupCategory: "Service"}
	if !reflect.DeepEqual(authEdge, want) {
		t.Errorf("auth edge = %+v, want %+v", authEdge, want)
	}
	if entries[0].GroupCategory != "" {
		t.Errorf("a far group with no category has an empty GroupCategory, got %q", entries[0].GroupCategory)
	}
	if entries[3].GroupCategory != "Team" {
		t.Errorf("the far group's own category, got %q", entries[3].GroupCategory)
	}

	// The type filter matches every type of that name, whatever its categories.
	typed := out
	typed.Type = "Depends on"
	if got := relationsSummary(listRelations(t, ctx, typed)); got != "Depends on>queue,Depends on>auth,Depends on>team" {
		t.Errorf("type filter = %s", got)
	}
	typed.Type = "depends on"
	if got := listRelations(t, ctx, typed); len(got) != 0 {
		t.Errorf("the type filter is case-sensitive, got %s", relationsSummary(got))
	}

	limited := out
	limited.Limit = 2
	if got := relationsSummary(listRelations(t, ctx, limited)); got != "Blocks>release,Depends on>queue" {
		t.Errorf("limit = %s", got)
	}

	in := shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: shortcodes.RelationsDirectionIn, Limit: 20}
	incoming := listRelations(t, ctx, in)
	if len(incoming) != 1 || incoming[0].GroupName != "upstream" || incoming[0].Direction != "in" || incoming[0].Name != "up" {
		t.Errorf("in = %+v", incoming)
	}

	if _, err := ctx.ListGroupRelationsForTemplate(context.Background(), shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: "both", Limit: 5}); err == nil {
		t.Error("an unknown direction must be refused")
	}
}

func TestRelationsForTemplateScope(t *testing.T) {
	runRelationsForTemplateScope(t, createTestContextWithPlugins(t, t.TempDir()))
}

func runRelationsForTemplateScope(t *testing.T, ctx *MahresourcesContext) {
	w := newRelationsWorld(t, ctx)
	// A subtree: root owns carrier and auth; queue, team, release and upstream
	// stay outside it.
	root := &models.Group{Name: "root"}
	if err := ctx.db.Create(root).Error; err != nil {
		t.Fatal(err)
	}
	for _, g := range []*models.Group{w.carrier, w.auth} {
		if err := ctx.db.Model(g).Update("owner_id", root.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &root.ID})
	out := shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: shortcodes.RelationsDirectionOut, Limit: 20}
	in := shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: shortcodes.RelationsDirectionIn, Limit: 20}

	// Only the edge whose far group is inside the subtree.
	if got := relationsSummary(listRelations(t, guest, out)); got != "Depends on>auth" {
		t.Errorf("guest out = %s, want only the inside edge", got)
	}
	if got := listRelations(t, guest, in); len(got) != 0 {
		t.Errorf("guest in = %s, want none: upstream is outside", relationsSummary(got))
	}
	// The unconfined view of the same rows, so the checks above are the scope.
	if got := listRelations(t, ctx, out); len(got) != 4 {
		t.Fatalf("control: admin sees %d edges, want 4", len(got))
	}

	// A carrier outside the subtree runs no query at all.
	queries := countQueries(t, ctx.db)
	upstream := shortcodes.RelationsRequest{GroupID: w.upstrm.ID, Direction: shortcodes.RelationsDirectionOut, Limit: 20}
	if got := listRelations(t, guest, upstream); len(got) != 0 {
		t.Errorf("an outside carrier listed %s", relationsSummary(got))
	}
	if n := queries.Load(); n != 0 {
		t.Errorf("an outside carrier ran %d queries, want 0", n)
	}

	// A guest who must be scoped but has no group sees nothing.
	denied := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest})
	if got := listRelations(t, denied, out); len(got) != 0 {
		t.Errorf("a deny-all guest listed %s", relationsSummary(got))
	}

	// A handle bound for MRQL carries no ORM scope: refused before any query.
	queries.Store(0)
	mrqlGuest := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &root.ID})
	if _, err := mrqlGuest.ListGroupRelationsForTemplate(context.Background(), out); !errors.Is(err, shortcodes.ErrRelationsUnavailable) {
		t.Errorf("WithMRQLPrincipal guest: err = %v, want ErrRelationsUnavailable", err)
	}
	if _, err := mrqlGuest.LoadRelatedGroupDetails(context.Background(), []uint{w.auth.ID}); !errors.Is(err, shortcodes.ErrRelationsUnavailable) {
		t.Errorf("WithMRQLPrincipal guest details: err = %v, want ErrRelationsUnavailable", err)
	}
	if n := queries.Load(); n != 0 {
		t.Errorf("a WithMRQLPrincipal guest ran %d queries, want 0", n)
	}
	// An admin bound for MRQL is unconfined and is answered.
	mrqlAdmin := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleAdmin})
	if got := listRelations(t, mrqlAdmin, out); len(got) != 4 {
		t.Errorf("WithMRQLPrincipal admin sees %d edges, want 4", len(got))
	}

	// 30 hidden edges sort ahead of the visible ones but do not fill the limit.
	aType := &models.GroupRelationType{Name: "Aardvark"}
	if err := ctx.db.Create(aType).Error; err != nil {
		t.Fatal(err)
	}
	inside2 := &models.Group{Name: "inside-2", OwnerId: &root.ID}
	if err := ctx.db.Create(inside2).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		hidden := &models.Group{Name: fmt.Sprintf("hidden-%d", i)}
		if err := ctx.db.Create(hidden).Error; err != nil {
			t.Fatal(err)
		}
		w.edge(t, w.carrier, hidden, aType, "", "")
	}
	w.edge(t, w.carrier, inside2, w.blocks, "", "")
	// The allow-list is a snapshot taken at binding, so bind again.
	guest = ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &root.ID})
	limited := out
	limited.Limit = 5
	if got := relationsSummary(listRelations(t, guest, limited)); got != "Blocks>inside-2,Depends on>auth" {
		t.Errorf("guest with hidden edges first = %s", got)
	}
	if got := listRelations(t, ctx, limited); len(got) != 5 || got[0].Type != "Aardvark" {
		t.Errorf("control: admin's first five are the hidden ones, got %s", relationsSummary(got))
	}

	// Details are loaded only for groups the viewer may see.
	details, err := guest.LoadRelatedGroupDetails(context.Background(), []uint{w.auth.ID, w.queue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].ID != w.auth.ID || details[0].Description != "auth description" || metaStatus(t, details[0].Meta) != "auth-ok" {
		t.Errorf("guest details = %+v, want only auth", details)
	}
}

// A confined principal's allow-list can be far larger than either driver's
// bind-parameter limit. Nothing here may bind it: an allow-list IN on any
// statement would make every [relations] of a large subtree an error.
func TestRelationsForTemplateLargeAllowList(t *testing.T) {
	runRelationsForTemplateLargeAllowList(t, createTestContextWithPlugins(t, t.TempDir()), 40000)
}

func runRelationsForTemplateLargeAllowList(t *testing.T, ctx *MahresourcesContext, size int) {
	w := newRelationsWorld(t, ctx)
	// Synthetic IDs far above the fixture's, plus every group but queue.
	allowed := make([]uint, 0, size+4)
	for i := 0; i < size; i++ {
		allowed = append(allowed, uint(1_000_000+i))
	}
	allowed = append(allowed, w.carrier.ID, w.auth.ID, w.team.ID, w.release.ID)
	big := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &w.carrier.ID})
	big.db = big.db.WithContext(context.WithValue(big.db.Statement.Context, scopeCtxKey{}, &scopeFilter{allowed: allowed}))

	entries := listRelations(t, big, shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: shortcodes.RelationsDirectionOut, Limit: 20})
	if got := relationsSummary(entries); got != "Blocks>release,Depends on>auth,Depends on>team" {
		t.Fatalf("large allow-list = %s", got)
	}
	details, err := big.LoadRelatedGroupDetails(context.Background(), []uint{w.auth.ID, w.team.ID, w.queue.ID})
	if err != nil {
		t.Fatalf("details under a large allow-list: %v", err)
	}
	if len(details) != 2 {
		t.Errorf("details = %+v, want auth and team", details)
	}
}

func TestRelatedGroupDetails(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	w := newRelationsWorld(t, ctx)
	deleted := uint(987654)
	details, err := ctx.LoadRelatedGroupDetails(context.Background(), []uint{w.auth.ID, w.team.ID, deleted})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[uint]models.Group{}
	for _, g := range details {
		byID[g.ID] = g
	}
	if len(byID) != 2 || byID[w.team.ID].Description != "team description" || metaStatus(t, byID[w.auth.ID].Meta) != "auth-ok" {
		t.Errorf("details = %+v", details)
	}
	if byID[w.auth.ID].Name != "" {
		t.Errorf("only id, description and meta are read, got name %q", byID[w.auth.ID].Name)
	}
	if _, err := ctx.LoadRelatedGroupDetails(context.Background(), make([]uint, shortcodes.MaxRelationsLimit+1)); err == nil {
		t.Error("more IDs than a [relations] can list must be refused")
	}
}

// metaStatus decodes Meta rather than comparing bytes: jsonb normalises them.
func metaStatus(t *testing.T, meta types.JSON) string {
	t.Helper()
	var m struct{ Status string }
	if err := json.Unmarshal(meta, &m); err != nil {
		t.Fatalf("decode meta %q: %v", meta, err)
	}
	return m.Status
}

// One statement per call, whatever the direction, filter or scope.
func TestRelationsForTemplateRunsOneStatement(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	w := newRelationsWorld(t, ctx)
	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &w.carrier.ID})
	queries := countQueries(t, ctx.db)
	for _, c := range []*MahresourcesContext{ctx, guest} {
		for _, dir := range []string{shortcodes.RelationsDirectionOut, shortcodes.RelationsDirectionIn} {
			queries.Store(0)
			listRelations(t, c, shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: dir, Type: "Blocks", Limit: 20})
			if n := queries.Load(); n != 1 {
				t.Errorf("%s ran %d statements, want 1", dir, n)
			}
		}
	}
	queries.Store(0)
	if _, err := ctx.LoadRelatedGroupDetails(context.Background(), []uint{w.auth.ID, w.team.ID}); err != nil {
		t.Fatal(err)
	}
	if n := queries.Load(); n != 1 {
		t.Errorf("details ran %d statements, want 1", n)
	}
}

func TestRelationsForTemplateHonoursTheDeadline(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	w := newRelationsWorld(t, ctx)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := ctx.ListGroupRelationsForTemplate(expired, shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: "out", Limit: 5})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

// direction="in" reads the to_group_id index, which EnsureSupplementalIndexes
// creates; out reads unique_rel by its leading column.
func TestRelationsForTemplateUsesAnIndexPerDirection(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	if err := models.EnsureSupplementalIndexes(ctx.db); err != nil {
		t.Fatal(err)
	}
	for dir, index := range map[string]string{"in": "idx__group_relations__to_group_id", "out": "unique_rel"} {
		req := shortcodes.RelationsRequest{GroupID: 7, Direction: dir, Type: "x"}
		for _, scope := range []*scopeFilter{nil, {allowed: []uint{7, 8, 9}}} {
			var rows []relationTemplateRow
			sql := ctx.db.ToSQL(func(tx *gorm.DB) *gorm.DB {
				return relationTemplateQuery(tx, req, 20, scope).Find(&rows)
			})
			plan := sqliteQueryPlan(t, ctx.db, sql)
			if !strings.Contains(plan, "SEARCH group_relations USING INDEX "+index) && !strings.Contains(plan, "SEARCH group_relations USING COVERING INDEX "+index) {
				t.Errorf("%s does not search %s (scoped=%v):\n%s\n%s", dir, index, scope != nil, sql, plan)
			}
		}
	}
}

func TestRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges(t *testing.T) {
	runRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges(t, createTestContextWithPlugins(t, t.TempDir()))
}

func runRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	w := newRelationsWorld(t, ctx)
	if err := ctx.db.Model(w.auth).Update("owner_id", w.carrier.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Baseline, then 500 new hidden edges that sort before the visible edge
	// in both directions. Query the same bound scope each time.
	w.edge(t, w.auth, w.carrier, w.dependsSvc, "", "")
	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &w.carrier.ID})
	requests := []shortcodes.RelationsRequest{
		{GroupID: w.carrier.ID, Direction: "out", Limit: 1},
		{GroupID: w.carrier.ID, Direction: "in", Limit: 1},
	}
	for _, req := range requests {
		if got := relationsSummary(listRelations(t, guest, req)); got != "Depends on>auth" {
			t.Fatalf("baseline %s = %q", req.Direction, got)
		}
	}
	hidden := make([]models.Group, 500)
	for i := range hidden {
		hidden[i].Name = fmt.Sprintf("hidden-first-%d", i)
	}
	if err := ctx.db.CreateInBatches(hidden, 100).Error; err != nil {
		t.Fatal(err)
	}
	edges := make([]models.GroupRelation, 0, 2*len(hidden))
	for i := range hidden {
		id := &hidden[i].ID
		edges = append(edges,
			models.GroupRelation{FromGroupId: &w.carrier.ID, ToGroupId: id, RelationTypeId: &w.blocks.ID},
			models.GroupRelation{FromGroupId: id, ToGroupId: &w.carrier.ID, RelationTypeId: &w.blocks.ID})
	}
	if err := ctx.db.CreateInBatches(edges, 100).Error; err != nil {
		t.Fatal(err)
	}
	for _, req := range requests {
		if got := relationsSummary(listRelations(t, guest, req)); got != "Depends on>auth" {
			t.Errorf("with hidden edges %s = %q, want baseline", req.Direction, got)
		}
	}
	// A live subtree re-read would admit this newly moved group and let its
	// earlier edge displace the result for the existing request snapshot.
	if err := ctx.db.Model(&hidden[0]).Update("owner_id", w.carrier.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, req := range requests {
		if got := relationsSummary(listRelations(t, guest, req)); got != "Depends on>auth" {
			t.Errorf("after reparent %s = %q, want original snapshot", req.Direction, got)
		}
	}
}

func TestVisibleGroupIDsReusesBoundScopeMembership(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	allowed := make([]uint, 40000)
	for i := range allowed {
		allowed[i] = uint(i + 1)
	}
	ctx.db = ctx.db.WithContext(context.WithValue(ctx.db.Statement.Context, scopeCtxKey{}, &scopeFilter{allowed: allowed}))
	// AllocsPerRun warms the first lookup before measuring repeated lookups.
	// Only the result map should allocate on subsequent checks.
	ids := []uint{1, 40000, 40001}
	if n := testing.AllocsPerRun(10, func() { _ = ctx.visibleGroupIDs(ids) }); n > 2 {
		t.Errorf("visibleGroupIDs allocates %.0f times per lookup after binding, want at most 2", n)
	}
	if got := ctx.visibleGroupIDs(ids); len(got) != 2 || !got[1] || !got[40000] {
		t.Errorf("visibleGroupIDs = %v", got)
	}
}
