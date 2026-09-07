package application_context

import (
	"context"
	"encoding/json"
	"testing"

	"gorm.io/gorm"
	"mahresources/mrql"
)

func metadataIndexFixture(t *testing.T, db *gorm.DB) (*gorm.DB, *MetadataIndexer) {
	t.Helper()
	metaType := "JSON"
	if db.Dialector.Name() == "postgres" {
		metaType = "JSONB"
	}
	for _, table := range []string{"resources", "notes", "groups"} {
		if err := db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, meta " + metaType + ")").Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"resource_categories", "categories", "note_types"} {
		if err := db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, metadata_indexes TEXT)").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO " + table + " (id,metadata_indexes) VALUES (1,'')").Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, NewMetadataIndexer(db)
}

func setMetadataIndexes(t *testing.T, db *gorm.DB, indexes ...mrql.MetadataIndex) {
	t.Helper()
	for entity, table := range map[string]string{"resource": "resource_categories", "group": "categories", "note": "note_types"} {
		keys := []mrql.MetadataIndexKey{}
		for _, index := range indexes {
			if index.Entity == entity {
				keys = append(keys, mrql.MetadataIndexKey{Key: index.Key, Kind: index.Kind})
			}
		}
		raw, _ := json.Marshal(keys)
		if err := db.Exec("UPDATE "+table+" SET metadata_indexes = ? WHERE id = 1", string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMetadataIndexerLifecycleSQLite(t *testing.T) {
	db := newTestDB(t)
	settings, manager := metadataIndexFixture(t, db)
	checkMetadataIndexerLifecycle(t, db, settings, manager)
}

func checkMetadataIndexerLifecycle(t *testing.T, db *gorm.DB, settings *gorm.DB, manager *MetadataIndexer) {
	t.Helper()
	numeric := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	text := mrql.MetadataIndex{Entity: "note", Key: "camera.model", Kind: "text"}
	if err := db.Exec("CREATE INDEX operator_index ON resources(id)").Error; err != nil {
		t.Fatal(err)
	}
	setMetadataIndexes(t, settings, numeric, text)
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, err := listManagedMetadataIndexes(db)
	if err != nil {
		t.Fatal(err)
	}
	numericParts, err := numeric.Indexes(db.Dialector.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != len(numericParts)+1 || !indexes[numeric.Name()].Valid || !indexes[text.Name()].Valid {
		t.Fatalf("indexes=%v", indexes)
	}
	if manager.Status().State != "ready" {
		t.Fatalf("status=%+v", manager.Status())
	}
	// The manager reads persisted declarations rather than relying on a process-local
	// cache, and discovers existing indexes after a restart.
	restarted := NewMetadataIndexer(db)
	if err := restarted.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	setMetadataIndexes(t, settings, text)
	if err := restarted.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, err = listManagedMetadataIndexes(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 || !indexes[text.Name()].Valid {
		t.Fatalf("obsolete index not removed: %v", indexes)
	}
	setMetadataIndexes(t, settings)
	if err := restarted.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, err = listManagedMetadataIndexes(db)
	if err != nil || len(indexes) != 0 {
		t.Fatalf("reset: indexes=%v err=%v", indexes, err)
	}
	if !db.Migrator().HasIndex("resources", "operator_index") {
		t.Fatal("removed an unmanaged index")
	}
}

func TestMetadataIndexerInvalidConfigurationPreservesIndexes(t *testing.T) {
	db := newTestDB(t)
	settings, manager := metadataIndexFixture(t, db)
	index := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	setMetadataIndexes(t, settings, index)
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE resource_categories SET metadata_indexes = 'null'").Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("invalid stored configuration accepted")
	}
	if manager.Status().State != "failed" {
		t.Fatal(manager.Status())
	}
	indexes, err := listManagedMetadataIndexes(db)
	if err != nil || !indexes[index.Name()].Valid {
		t.Fatalf("discarded index on invalid config: %v %v", indexes, err)
	}
}

func TestMetadataIndexerFailedBuildRetriesSQLite(t *testing.T) {
	db := newTestDB(t)
	settings, manager := metadataIndexFixture(t, db)
	index := mrql.MetadataIndex{Entity: "resource", Key: "score", Kind: "numeric"}
	if err := db.Exec("INSERT INTO resources(id,meta) VALUES (1,'not json')").Error; err != nil {
		t.Fatal(err)
	}
	setMetadataIndexes(t, settings, index)
	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("expected malformed JSON build failure")
	}
	if manager.Status().State != "failed" {
		t.Fatal(manager.Status())
	}
	if err := db.Exec(`UPDATE resources SET meta = '{"score":10}'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.Status().State != "ready" {
		t.Fatal(manager.Status())
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Reconcile(cancelled); err == nil {
		t.Fatal("ignored cancellation")
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataIndexerSharesCategoryDeclarations(t *testing.T) {
	db := newTestDB(t)
	_, manager := metadataIndexFixture(t, db)
	raw := `[{"key":"score","kind":"numeric"}]`
	if err := db.Exec("UPDATE resource_categories SET metadata_indexes = ?", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO resource_categories(id,metadata_indexes) VALUES (2,?)", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, _ := listManagedMetadataIndexes(db)
	if len(indexes) != 1 {
		t.Fatalf("created duplicate indexes: %v", indexes)
	}
	if err := db.Exec("DELETE FROM resource_categories WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, _ = listManagedMetadataIndexes(db)
	if len(indexes) != 1 {
		t.Fatal("removed an index still used by another category")
	}
	if err := db.Exec("UPDATE resource_categories SET metadata_indexes = '[]' WHERE id = 2").Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	indexes, _ = listManagedMetadataIndexes(db)
	if len(indexes) != 0 {
		t.Fatal("retained an index after its last declaration was removed")
	}
}
