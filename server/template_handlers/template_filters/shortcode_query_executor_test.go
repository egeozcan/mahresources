package template_filters

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/seed"
	"mahresources/shortcodes"
)

func setupExecutorTestContext(t *testing.T) (*application_context.MahresourcesContext, *gorm.DB) {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Group{}, &models.Category{}, &models.NoteType{}, &models.TemplateRevision{}, &models.Preview{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.NoteBlock{},
		&models.SavedMRQLQuery{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seed.AddInitialData(db)
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := application_context.NewMahresourcesContext(nil, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	return ctx, db
}

// Regression: an inline query carrying a scope must splice SCOPE at the
// grammatically correct position (before LIMIT/ORDER BY/etc.), not append it at
// the end where MRQL would reject it. The result must round-trip through Parse.
func TestExecutorLinkScopeInsertedBeforeTrailingClauses(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	scope := &models.Group{Name: "scope-grp"}
	if err := db.Create(scope).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), `type = "resource" ORDER BY name LIMIT 5`, shortcodes.QueryOptions{
		ScopeGroupID: scope.ID,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	want := fmt.Sprintf(`type = "resource" SCOPE %d ORDER BY name LIMIT 5`, scope.ID)
	if res.EffectiveQuery != want {
		t.Fatalf("EffectiveQuery = %q, want %q", res.EffectiveQuery, want)
	}
}

// Regression: a scoped saved query must not link via ?saved=<id> (which opens
// globally) — the executor clears SavedID and bakes the scope into the query
// text so the view-all link reproduces the scoped result set.
func TestExecutorScopedSavedQueryDropsSavedID(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	scope := &models.Group{Name: "saved-scope-grp"}
	if err := db.Create(scope).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := ctx.CreateSavedMRQLQuery("rep", `type = "resource" LIMIT 3`, ""); err != nil {
		t.Fatalf("create saved: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), "", shortcodes.QueryOptions{
		SavedName:    "rep",
		ScopeGroupID: scope.ID,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.SavedID != 0 {
		t.Fatalf("scoped saved query must clear SavedID, got %d", res.SavedID)
	}
	want := fmt.Sprintf(`type = "resource" SCOPE %d LIMIT 3`, scope.ID)
	if res.EffectiveQuery != want {
		t.Fatalf("EffectiveQuery = %q, want %q", res.EffectiveQuery, want)
	}
}

// An unscoped saved query keeps its saved identity (?saved=<id>).
func TestExecutorUnscopedSavedQueryKeepsSavedID(t *testing.T) {
	ctx, _ := setupExecutorTestContext(t)
	saved, err := ctx.CreateSavedMRQLQuery("rep2", `type = "resource"`, "")
	if err != nil {
		t.Fatalf("create saved: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), "", shortcodes.QueryOptions{SavedName: "rep2"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.SavedID != saved.ID {
		t.Fatalf("unscoped saved query should keep SavedID %d, got %d", saved.ID, res.SavedID)
	}
}

// similarityRecorder is the real context with SimilarityDistancesTo counted, so
// a test can tell whether the enrichment step ran at all.
type similarityRecorder struct {
	mrqlShortcodeRunner
	calls int
}

func (r *similarityRecorder) SimilarityDistancesTo(reqCtx context.Context, targetID uint, ids []uint) (map[uint]int, error) {
	r.calls++
	return r.mrqlShortcodeRunner.SimilarityDistancesTo(reqCtx, targetID, ids)
}

func seedExecutorSimilarity(t *testing.T, db *gorm.DB) (target, near, far uint) {
	t.Helper()
	rs := []*models.Resource{{Name: "target"}, {Name: "near"}, {Name: "far"}}
	for _, r := range rs {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed resource: %v", err)
		}
	}
	p1, p2 := uint8(1), uint8(7)
	pairs := []models.ResourceSimilarity{
		{ResourceID1: rs[0].ID, ResourceID2: rs[1].ID, HammingDistance: 9, PDistance: &p1},
		{ResourceID1: rs[0].ID, ResourceID2: rs[2].ID, HammingDistance: 9, PDistance: &p2},
	}
	if err := db.Create(&pairs).Error; err != nil {
		t.Fatalf("seed pairs: %v", err)
	}
	return rs[0].ID, rs[1].ID, rs[2].ID
}

func TestExecutorAttachesSimilarityDistances(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	target, near, far := seedExecutorSimilarity(t, db)
	rec := &similarityRecorder{mrqlShortcodeRunner: ctx}
	q := fmt.Sprintf("type = resource AND SIMILAR TO resource(%d) ORDER BY distance ASC", target)

	res, err := executeMRQLForShortcode(context.Background(), rec, q, shortcodes.QueryOptions{Limit: 8, WantSimilarityDistance: true})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if rec.calls != 1 {
		t.Fatalf("SimilarityDistancesTo calls = %d, want 1", rec.calls)
	}
	if len(res.Items) != 2 || res.Items[0].EntityID != near || res.Items[1].EntityID != far {
		t.Fatalf("items = %+v, want near then far", res.Items)
	}
	for i, want := range []int{1, 7} {
		got := res.Items[i].SimilarityDistance
		if got == nil || *got != want {
			t.Errorf("item %d distance = %v, want %d", i, got, want)
		}
	}
}

func TestExecutorSkipsSimilarityDistancesUnlessAsked(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	target, _, _ := seedExecutorSimilarity(t, db)
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"flag off", fmt.Sprintf("type = resource AND SIMILAR TO resource(%d)", target), false},
		{"no SIMILAR TO", `type = resource AND name = "near"`, true},
		{"two SIMILAR TO", fmt.Sprintf("type = resource AND (SIMILAR TO resource(%d) OR SIMILAR TO resource(%d))", target, target+1), true},
	}
	for _, c := range cases {
		rec := &similarityRecorder{mrqlShortcodeRunner: ctx}
		res, err := executeMRQLForShortcode(context.Background(), rec, c.query, shortcodes.QueryOptions{Limit: 8, WantSimilarityDistance: c.want})
		if err != nil {
			t.Fatalf("%s: exec: %v", c.name, err)
		}
		if rec.calls != 0 {
			t.Errorf("%s: SimilarityDistancesTo calls = %d, want 0", c.name, rec.calls)
		}
		if len(res.Items) == 0 {
			t.Errorf("%s: expected items", c.name)
		}
		for _, it := range res.Items {
			if it.SimilarityDistance != nil {
				t.Errorf("%s: item %d has distance %d", c.name, it.EntityID, *it.SimilarityDistance)
			}
		}
	}
}

func TestSimilarityDistancesToBounds(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	target, near, far := seedExecutorSimilarity(t, db)
	got, err := ctx.SimilarityDistancesTo(context.Background(), target, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty ids: %v, %v", got, err)
	}
	ids := make([]uint, 101)
	if _, err := ctx.SimilarityDistancesTo(context.Background(), target, ids); err == nil {
		t.Fatal("expected an error for more than 100 ids")
	}
	// Both directions of the stored pair resolve.
	got, err = ctx.SimilarityDistancesTo(context.Background(), near, []uint{target, far})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 || got[target] != 1 {
		t.Fatalf("distances from near = %v, want {target: 1}", got)
	}
}
