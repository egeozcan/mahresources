package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/shortcodes"
)

func newAdjacentSQLiteContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.Category{}, &models.TemplateRevision{}, &models.ResourceCategory{}, &models.NoteType{},
		&models.Series{}, &models.Group{}, &models.Resource{}, &models.Note{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := models.EnsureResourceListIndexes(db); err != nil {
		t.Fatalf("resource list indexes: %v", err)
	}
	if err := models.EnsureSiblingNavIndexes(db); err != nil {
		t.Fatalf("sibling navigation indexes: %v", err)
	}
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{DbType: constants.DbTypeSqlite})
	cleanupMahresourcesTestContext(t, ctx)
	return ctx
}

// adjacentWorld creates rows with explicit creation times, so ties and order
// are under the test's control.
type adjacentWorld struct {
	t    *testing.T
	ctx  *MahresourcesContext
	base time.Time
}

func newAdjacentWorld(t *testing.T, ctx *MahresourcesContext) *adjacentWorld {
	return &adjacentWorld{t: t, ctx: ctx, base: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (w *adjacentWorld) at(minute int) time.Time {
	return w.base.Add(time.Duration(minute) * time.Minute)
}

func (w *adjacentWorld) create(v any) {
	w.t.Helper()
	if err := w.ctx.db.Create(v).Error; err != nil {
		w.t.Fatalf("create %T: %v", v, err)
	}
}

func (w *adjacentWorld) group(name string, owner, category *uint, minute int) *models.Group {
	g := &models.Group{Name: name, OwnerId: owner, CategoryId: category, CreatedAt: w.at(minute)}
	w.create(g)
	return g
}

func (w *adjacentWorld) note(name string, owner, noteType *uint, minute int) *models.Note {
	n := &models.Note{Name: name, OwnerId: owner, NoteTypeId: noteType, CreatedAt: w.at(minute)}
	w.create(n)
	return n
}

func (w *adjacentWorld) resource(name string, owner *uint, category uint, series *uint, minute int) *models.Resource {
	r := &models.Resource{Name: name, OwnerId: owner, ResourceCategoryId: category, SeriesID: series, CreatedAt: w.at(minute)}
	w.create(r)
	return r
}

func adjacentNames(t *testing.T, ctx *MahresourcesContext, entityType string, id uint, series bool) (string, string) {
	t.Helper()
	res, err := ctx.AdjacentEntities(context.Background(), shortcodes.AdjacentQuery{EntityType: entityType, EntityID: id, Series: series})
	if err != nil {
		t.Fatalf("AdjacentEntities(%s %d): %v", entityType, id, err)
	}
	name := func(l *shortcodes.AdjacentLink) string {
		if l == nil {
			return ""
		}
		return l.Name
	}
	return name(res.Prev), name(res.Next)
}

func expectAdjacent(t *testing.T, ctx *MahresourcesContext, entityType string, id uint, series bool, wantPrev, wantNext string) {
	t.Helper()
	prev, next := adjacentNames(t, ctx, entityType, id, series)
	if prev != wantPrev || next != wantNext {
		t.Errorf("%s %d (series=%v): prev=%q next=%q, want prev=%q next=%q", entityType, id, series, prev, next, wantPrev, wantNext)
	}
}

func uptr(v uint) *uint { return &v }

func countQueries(t *testing.T, db *gorm.DB) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	name := "test:count_adjacent_queries_" + strings.ReplaceAll(t.Name(), "/", "_")
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(*gorm.DB) { n.Add(1) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
	return &n
}

func TestAdjacentOrderingTiesAndPartitions(t *testing.T) {
	runAdjacentOrderingTiesAndPartitions(t, newAdjacentSQLiteContext(t))
}

func runAdjacentOrderingTiesAndPartitions(t *testing.T, ctx *MahresourcesContext) {
	w := newAdjacentWorld(t, ctx)
	owner := w.group("owner", nil, nil, 0)
	other := w.group("other-owner", nil, nil, 0)
	typeA := &models.NoteType{Name: "A"}
	typeB := &models.NoteType{Name: "B"}
	w.create(typeA)
	w.create(typeB)

	// n2 and n3 share a creation time; the id breaks the tie.
	n1 := w.note("n1", &owner.ID, &typeA.ID, 1)
	w.note("other-type", &owner.ID, &typeB.ID, 2)
	n2 := w.note("n2", &owner.ID, &typeA.ID, 3)
	n3 := w.note("n3", &owner.ID, &typeA.ID, 3)
	w.note("other-owner", &other.ID, &typeA.ID, 3)
	n4 := w.note("n4", &owner.ID, &typeA.ID, 5)

	expectAdjacent(t, ctx, "note", n1.ID, false, "", "n2")
	expectAdjacent(t, ctx, "note", n2.ID, false, "n1", "n3")
	expectAdjacent(t, ctx, "note", n3.ID, false, "n2", "n4")
	expectAdjacent(t, ctx, "note", n4.ID, false, "n3", "")

	// Walking forward from the first visits every sibling exactly once.
	seen := []string{}
	id := n1.ID
	for i := 0; i < 10; i++ {
		res, err := ctx.AdjacentEntities(context.Background(), shortcodes.AdjacentQuery{EntityType: "note", EntityID: id})
		if err != nil {
			t.Fatal(err)
		}
		if res.Next == nil {
			break
		}
		seen = append(seen, res.Next.Name)
		id = res.Next.ID
	}
	if got := strings.Join(seen, ","); got != "n2,n3,n4" {
		t.Errorf("walk = %s, want n2,n3,n4", got)
	}

	// Ownerless rows pair with each other and not with owned ones; a NULL type
	// or category pairs with other NULLs.
	u1 := w.note("u1", nil, nil, 10)
	w.note("owned-untyped", &owner.ID, nil, 11)
	u2 := w.note("u2", nil, nil, 12)
	expectAdjacent(t, ctx, "note", u1.ID, false, "", "u2")
	expectAdjacent(t, ctx, "note", u2.ID, false, "u1", "")

	cat := &models.Category{Name: "cat"}
	w.create(cat)
	// Top-level groups: owner and other were created at minute 0 with no category.
	g1 := w.group("g1", nil, &cat.ID, 20)
	w.group("g-child", &owner.ID, &cat.ID, 21)
	g2 := w.group("g2", nil, &cat.ID, 22)
	expectAdjacent(t, ctx, "group", g1.ID, false, "", "g2")
	expectAdjacent(t, ctx, "group", g2.ID, false, "g1", "")
	expectAdjacent(t, ctx, "group", owner.ID, false, "", "other-owner")

	rc := &models.ResourceCategory{Name: "rc"}
	w.create(rc)
	r1 := w.resource("r1", nil, rc.ID, nil, 30)
	w.resource("r-owned", &owner.ID, rc.ID, nil, 31)
	r2 := w.resource("r2", nil, rc.ID, nil, 32)
	expectAdjacent(t, ctx, "resource", r1.ID, false, "", "r2")
	expectAdjacent(t, ctx, "resource", r2.ID, false, "r1", "")

	// A missing entity resolves to nothing.
	expectAdjacent(t, ctx, "note", 999999, false, "", "")
}

func TestAdjacentSeriesMode(t *testing.T) {
	runAdjacentSeriesMode(t, newAdjacentSQLiteContext(t))
}

func runAdjacentSeriesMode(t *testing.T, ctx *MahresourcesContext) {
	w := newAdjacentWorld(t, ctx)
	a := w.group("a", nil, nil, 0)
	b := w.group("b", nil, nil, 0)
	rcA := &models.ResourceCategory{Name: "rcA"}
	rcB := &models.ResourceCategory{Name: "rcB"}
	w.create(rcA)
	w.create(rcB)
	series := &models.Series{Name: "book", Slug: "book"}
	w.create(series)

	p1 := w.resource("p1", &a.ID, rcA.ID, &series.ID, 1)
	w.resource("not-in-series", &a.ID, rcA.ID, nil, 2)
	p2 := w.resource("p2", &b.ID, rcB.ID, &series.ID, 3)
	p3 := w.resource("p3", &a.ID, rcA.ID, &series.ID, 4)
	loose := w.resource("loose", &a.ID, rcA.ID, nil, 5)

	expectAdjacent(t, ctx, "resource", p1.ID, true, "", "p2")
	expectAdjacent(t, ctx, "resource", p2.ID, true, "p1", "p3")
	expectAdjacent(t, ctx, "resource", p3.ID, true, "p2", "")
	// Default mode on the same row ignores the series.
	expectAdjacent(t, ctx, "resource", p3.ID, false, "not-in-series", "loose")
	expectAdjacent(t, ctx, "resource", loose.ID, true, "", "")
}

func TestAdjacentScope(t *testing.T) {
	runAdjacentScope(t, newAdjacentSQLiteContext(t))
}

func runAdjacentScope(t *testing.T, ctx *MahresourcesContext) {
	w := newAdjacentWorld(t, ctx)
	top := w.group("top", nil, nil, 0)
	s := w.group("S", &top.ID, nil, 1)
	w.group("S-sibling", &top.ID, nil, 2)
	child := w.group("S-child", &s.ID, nil, 3)
	outside := w.group("outside", nil, nil, 4)
	w.group("S-child-2", &s.ID, nil, 5)
	rc := &models.ResourceCategory{Name: "rc"}
	w.create(rc)
	series := &models.Series{Name: "mixed", Slug: "mixed"}
	w.create(series)

	n1 := w.note("in-1", &child.ID, nil, 10)
	w.note("in-2", &child.ID, nil, 11)
	nOut := w.note("out-1", &outside.ID, nil, 12)
	w.note("out-2", &outside.ID, nil, 13)
	r1 := w.resource("s-1", &child.ID, rc.ID, &series.ID, 20)
	w.resource("s-out", &outside.ID, rc.ID, &series.ID, 21)
	w.resource("s-3", &s.ID, rc.ID, &series.ID, 22)
	unowned := w.resource("unowned", nil, rc.ID, nil, 23)
	w.resource("unowned-2", nil, rc.ID, nil, 24)

	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest, ScopeGroupID: &s.ID})
	expectAdjacent(t, guest, "note", n1.ID, false, "", "in-2")
	expectAdjacent(t, guest, "group", s.ID, false, "", "")
	expectAdjacent(t, guest, "group", child.ID, false, "", "S-child-2")
	expectAdjacent(t, guest, "resource", r1.ID, true, "", "s-3")
	expectAdjacent(t, guest, "resource", unowned.ID, false, "", "")
	// The unconfined view of the same rows, so the checks above are the scope.
	expectAdjacent(t, ctx, "group", s.ID, false, "", "S-sibling")
	expectAdjacent(t, ctx, "resource", r1.ID, true, "", "s-out")

	// A guest who must be scoped but has no group sees nothing.
	unscoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest})
	expectAdjacent(t, unscoped, "note", n1.ID, false, "", "")

	// An entity outside the subtree costs exactly one query: the key subquery,
	// which is not scoped, is never reached.
	queries := countQueries(t, ctx.db)
	expectAdjacent(t, guest, "note", nOut.ID, false, "", "")
	if got := queries.Load(); got != 1 {
		t.Errorf("an out-of-subtree entity ran %d queries, want 1", got)
	}

	// The subtree root's siblings are all outside the subtree, however many
	// its parent has: one query for the root itself, and no seek.
	queries.Store(0)
	expectAdjacent(t, guest, "group", s.ID, false, "", "")
	if got := queries.Load(); got != 1 {
		t.Errorf("the subtree root ran %d queries, want 1", got)
	}

	// A handle bound for MRQL carries no ORM scope; the guard refuses it before
	// any query.
	queries.Store(0)
	mrqlGuest := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &s.ID})
	expectAdjacent(t, mrqlGuest, "note", nOut.ID, false, "", "")
	expectAdjacent(t, mrqlGuest, "note", n1.ID, false, "", "")
	if got := queries.Load(); got != 0 {
		t.Errorf("a WithMRQLPrincipal guest ran %d queries, want 0", got)
	}

	// The deadline rebase keeps the scope: a bounded read by the guest still
	// cannot reach the outside note's siblings.
	bounded, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	res, err := guest.AdjacentEntities(bounded, shortcodes.AdjacentQuery{EntityType: "note", EntityID: nOut.ID})
	if err != nil || res.Prev != nil || res.Next != nil {
		t.Errorf("bounded guest read of an outside note = %+v, %v; want empty", res, err)
	}
	res, err = guest.AdjacentEntities(bounded, shortcodes.AdjacentQuery{EntityType: "note", EntityID: n1.ID})
	if err != nil || res.Next == nil || res.Next.Name != "in-2" {
		t.Errorf("bounded guest read of an inside note = %+v, %v; want next in-2", res, err)
	}
}

func TestAdjacentHonoursTheDeadline(t *testing.T) {
	ctx := newAdjacentSQLiteContext(t)
	w := newAdjacentWorld(t, ctx)
	n := w.note("n", nil, nil, 0)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := ctx.AdjacentEntities(expired, shortcodes.AdjacentQuery{EntityType: "note", EntityID: n.ID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestAdjacentRejectsAnUnknownType(t *testing.T) {
	ctx := newAdjacentSQLiteContext(t)
	if _, err := ctx.AdjacentEntities(context.Background(), shortcodes.AdjacentQuery{EntityType: "tag", EntityID: 1}); err == nil {
		t.Fatal("an unknown entity type must be refused")
	}
}

// Each seek reads the composite index in order: a range seek after the
// equality prefix, with no sort.
func TestAdjacentSeeksUseTheSiblingIndexes(t *testing.T) {
	ctx := newAdjacentSQLiteContext(t)
	db := ctx.db
	for entityType, spec := range adjacentSpecs {
		for _, owned := range []bool{true, false} {
			for _, next := range []bool{true, false} {
				cur := adjacentRow{ID: 5, Category: uptr(2)}
				if owned {
					cur.OwnerId = uptr(3)
				}
				var dest []adjacentLinkRow
				sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
					return adjacentSeek(tx, spec, cur, false, next).Find(&dest)
				})
				plan := sqliteQueryPlan(t, db, sql)
				want := "idx_" + spec.table + "_adjacent_created"
				if !strings.Contains(plan, "SEARCH "+spec.table+" USING INDEX "+want) &&
					!strings.Contains(plan, "SEARCH "+spec.table+" USING COVERING INDEX "+want) {
					t.Errorf("%s owned=%v next=%v: seek does not search %s:\n%s\n%s", entityType, owned, next, want, sql, plan)
				}
				if strings.Contains(plan, "TEMP B-TREE") {
					t.Errorf("%s owned=%v next=%v: seek sorts:\n%s", entityType, owned, next, plan)
				}
			}
		}
	}
}

func sqliteQueryPlan(t *testing.T, db *gorm.DB, query string) string {
	t.Helper()
	rows, err := db.Raw("EXPLAIN QUERY PLAN " + query).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail + "\n")
	}
	return plan.String()
}
