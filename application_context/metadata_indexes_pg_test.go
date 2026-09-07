//go:build postgres

package application_context

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/mrql"
)

func TestMetadataIndexerLifecyclePostgres(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.RuntimeSetting{}); err != nil {
		t.Fatal(err)
	}
	settings, manager := metadataIndexFixture(t, db)
	checkMetadataIndexerLifecycle(t, db, settings, manager)
}

func TestMRQLUsesOnlyReadyMetadataIndexesPostgres(t *testing.T) {
	ctx := newPostgresPluginContext(t, nil)
	ctx.metadataIndexer = NewMetadataIndexer(ctx.db)
	category := models.ResourceCategory{Name: "Indexed scores", MetadataIndexes: `[{"key":"score","kind":"numeric"}]`}
	if err := ctx.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.Resource{Name: "Indexed resource", Meta: types.JSON(`{"score":10}`)}
	if err := ctx.db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	query, err := mrql.Parse(`type = resource AND meta.score = 10`)
	if err != nil {
		t.Fatal(err)
	}
	check := func(wantIndexed bool) {
		t.Helper()
		explained, err := ctx.ExplainMRQL(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(explained.Statements) != 1 {
			t.Fatal(explained.Statements)
		}
		if got := strings.Contains(explained.Statements[0].SQL, "length("); got != wantIndexed {
			t.Fatalf("indexed predicate=%v want=%v: %s", got, wantIndexed, explained.Statements[0].SQL)
		}
		result, err := ctx.ExecuteMRQLParsed(t.Context(), query, 0, 0)
		if err != nil || len(result.Resources) != 1 || result.Resources[0].ID != resource.ID {
			t.Fatalf("query changed result: result=%+v err=%v", result, err)
		}
	}
	check(false) // Saving a declaration alone must not slow down the query.
	if err := ctx.metadataIndexer.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(true)
	ready := ctx.metadataIndexer.ReadyIndexes()
	ready[0].Key = "mutated snapshot"
	check(true) // A caller cannot mutate the worker's cached declaration set.
	if err := ctx.db.Model(&category).Update("metadata_indexes", "[]").Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.metadataIndexer.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(false)
}

func TestMetadataIndexerRepairsInvalidPostgresIndex(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.RuntimeSetting{}); err != nil {
		t.Fatal(err)
	}
	settings, manager := metadataIndexFixture(t, db)
	index := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	if err := db.Exec(`INSERT INTO resources(id,meta) VALUES (1,'{"score":10}'),(2,'{"score":10}')`).Error; err != nil {
		t.Fatal(err)
	}
	sql, err := index.CreateSQL("postgres")
	if err != nil {
		t.Fatal(err)
	}
	// Failed concurrent UNIQUE build is a deterministic way to leave the same
	// invalid catalog entry as an interrupted non-unique concurrent build.
	if err := db.Exec(strings.Replace(sql, "CREATE INDEX", "CREATE UNIQUE INDEX", 1)).Error; err == nil {
		t.Fatal("expected unique build to fail")
	}
	before, err := listManagedMetadataIndexes(db)
	if err != nil {
		t.Fatal(err)
	}
	if old, ok := before[index.Name()]; !ok || old.Valid {
		t.Fatalf("fixture did not leave invalid index: %v", before)
	}
	setMetadataIndexes(t, settings, index)
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := listManagedMetadataIndexes(db)
	if err != nil || !after[index.Name()].Valid {
		t.Fatalf("repair failed: %v %v", after, err)
	}
}

func TestMetadataIndexerSerializesPostgresServers(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.RuntimeSetting{}); err != nil {
		t.Fatal(err)
	}
	settings, manager := metadataIndexFixture(t, db)
	setMetadataIndexes(t, settings, mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"})
	if err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SELECT pg_advisory_lock(734639214822)").Error; err != nil {
			return err
		}
		defer conn.Exec("SELECT pg_advisory_unlock(734639214822)")
		if err := manager.Reconcile(context.Background()); err != nil {
			return err
		}
		if manager.Status().State != "pending" {
			t.Fatalf("concurrent reconciler was not deferred: %+v", manager.Status())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.Status().State != "ready" {
		t.Fatal(manager.Status())
	}
}

func TestMetadataIndexerReplacesOldNumericIndexesPostgres(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	_, manager := metadataIndexFixture(t, db)
	index := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	oldName := strings.Replace(index.Name(), "_v2_", "_v1_", 1)
	if err := db.Exec(`CREATE INDEX "` + oldName + `" ON resources(id)`).Error; err != nil {
		t.Fatal(err)
	}
	raw := `[{"key":"score","kind":"numeric"}]`
	if err := db.Exec("UPDATE resource_categories SET metadata_indexes = ?", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO resource_categories(id,metadata_indexes) VALUES (2,?)", raw).Error; err != nil {
		t.Fatal(err)
	}
	parts, err := index.Indexes("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected numeric and oversized-value indexes, got %d", len(parts))
	}
	for _, removeCategory := range []uint{0, 1, 2} {
		if removeCategory != 0 {
			if err := db.Exec("DELETE FROM resource_categories WHERE id = ?", removeCategory).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := manager.Reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		indexes, err := listManagedMetadataIndexes(db)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := indexes[oldName]; ok {
			t.Fatal("obsolete numeric index survived replacement")
		}
		if removeCategory == 2 {
			if len(indexes) != 0 {
				t.Fatal("companion indexes survived their last declaration")
			}
		} else {
			if len(indexes) != len(parts) {
				t.Fatalf("expected %d shared physical indexes, got %v", len(parts), indexes)
			}
			for _, part := range parts {
				if !indexes[part.Name].Valid {
					t.Fatalf("missing valid physical index %s", part.Name)
				}
			}
		}
	}
}

func TestMetadataIndexerDoesNotPublishPartialBuildPostgres(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	_, manager := metadataIndexFixture(t, db)
	index := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	setMetadataIndexes(t, db, index)
	parts, err := index.Indexes("postgres")
	if err != nil || len(parts) != 2 {
		t.Fatalf("numeric indexes: %v %v", parts, err)
	}
	// A conflicting name outside the managed entity tables makes the second
	// concurrent build fail after the first has already committed.
	if err := db.Exec("CREATE TABLE index_conflict (id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE INDEX "` + parts[1].Name + `" ON index_conflict(id)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(t.Context()); err == nil {
		t.Fatal("expected companion index creation failure")
	}
	if len(manager.ReadyIndexes()) != 0 {
		t.Fatal("published incomplete index pair to query translation")
	}
	if err := db.Exec(`DROP INDEX "` + parts[1].Name + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ready := manager.ReadyIndexes(); len(ready) != 1 || ready[0] != index {
		t.Fatalf("complete index pair not published: %v", ready)
	}
}
