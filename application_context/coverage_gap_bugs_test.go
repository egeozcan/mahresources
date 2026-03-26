package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createCoverageTestContext creates a test context for coverage-gap tests.
func createCoverageTestContext(t *testing.T, cacheName string) *MahresourcesContext {
	t.Helper()

	dsn := "file:" + cacheName + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.NoteBlock{},
		&models.PluginKV{},
		&models.ResourceVersion{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	config := &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(fs, db, readOnlyDB, config)
}

// =============================================
// R9-B-001 & R9-B-002: Nil pointer dereferences
// (Tested in static_template_context_test.go)
// =============================================

// =============================================
// R9-B-003: DeleteCategory not wrapped in transaction
//
// DeleteCategory performs multiple write operations (clearing group category IDs,
// deleting relation types, deleting relations, etc.) without a transaction.
// If the final Delete fails after prior writes succeeded, the database is
// left in an inconsistent state: groups have lost their category_id but
// the category still exists.
// =============================================

func TestDeleteCategory_WithRelationTypes_Consistency(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_cat_consistency")

	// Create a category
	cat, err := ctx.CreateCategory(&query_models.CategoryCreator{
		Name: "Test Category",
	})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	// Create two groups in that category
	group1, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       "Group A",
		CategoryId: cat.ID,
	})
	if err != nil {
		t.Fatalf("CreateGroup A: %v", err)
	}

	group2, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       "Group B",
		CategoryId: cat.ID,
	})
	if err != nil {
		t.Fatalf("CreateGroup B: %v", err)
	}

	// Create a relation type referencing this category
	catID := cat.ID
	relType := &models.GroupRelationType{
		Name:           "related-to",
		FromCategoryId: &catID,
		ToCategoryId:   &catID,
	}
	if err := ctx.db.Create(relType).Error; err != nil {
		t.Fatalf("Create relation type: %v", err)
	}

	// Create a relation between the groups using this type
	g1ID := group1.ID
	g2ID := group2.ID
	rtID := relType.ID
	relation := &models.GroupRelation{
		FromGroupId:    &g1ID,
		ToGroupId:      &g2ID,
		RelationTypeId: &rtID,
	}
	if err := ctx.db.Create(relation).Error; err != nil {
		t.Fatalf("Create relation: %v", err)
	}

	// Delete the category
	if err := ctx.DeleteCategory(cat.ID); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	// Verify: category should be gone
	var catCheck models.Category
	if err := ctx.db.First(&catCheck, cat.ID).Error; err == nil {
		t.Error("Category should have been deleted")
	}

	// Verify: groups should still exist but category_id should be nil
	var g1, g2 models.Group
	if err := ctx.db.First(&g1, group1.ID).Error; err != nil {
		t.Fatalf("Group A should still exist: %v", err)
	}
	if g1.CategoryId != nil {
		t.Error("Group A category_id should be nil after category deletion")
	}
	if err := ctx.db.First(&g2, group2.ID).Error; err != nil {
		t.Fatalf("Group B should still exist: %v", err)
	}
	if g2.CategoryId != nil {
		t.Error("Group B category_id should be nil after category deletion")
	}

	// Verify: relation type should be gone
	var rtCheck models.GroupRelationType
	if err := ctx.db.First(&rtCheck, relType.ID).Error; err == nil {
		t.Error("Relation type should have been deleted")
	}

	// Verify: relation should be gone
	var relCheck models.GroupRelation
	if err := ctx.db.First(&relCheck, relation.ID).Error; err == nil {
		t.Error("Relation should have been deleted")
	}
}

func TestDeleteCategory_NonExistent_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_cat_nonexistent")

	err := ctx.DeleteCategory(99999)
	if err == nil {
		t.Error("DeleteCategory for non-existent ID should return error")
	}
}

// =============================================
// R9-B-004: maskDSN edge cases
// =============================================

func TestMaskDSN_Empty(t *testing.T) {
	if got := maskDSN(""); got != "" {
		t.Errorf("maskDSN(\"\") = %q, want \"\"", got)
	}
}

func TestMaskDSN_Memory(t *testing.T) {
	if got := maskDSN(":memory:"); got != ":memory:" {
		t.Errorf("maskDSN(\":memory:\") = %q, want \":memory:\"", got)
	}
}

func TestMaskDSN_PostgresWithCredentials(t *testing.T) {
	input := "postgres://admin:secret123@localhost:5432/mydb"
	expected := "postgres://***@localhost:5432/mydb"
	if got := maskDSN(input); got != expected {
		t.Errorf("maskDSN(%q) = %q, want %q", input, got, expected)
	}
}

func TestMaskDSN_SQLitePath(t *testing.T) {
	input := "/path/to/my.db"
	if got := maskDSN(input); got != input {
		t.Errorf("maskDSN(%q) = %q, want %q (file paths should be unchanged)", input, got, input)
	}
}

func TestMaskDSN_SQLiteFileURI(t *testing.T) {
	input := "file:test.db?_journal_mode=WAL"
	if got := maskDSN(input); got != input {
		t.Errorf("maskDSN(%q) = %q, want %q (SQLite file URIs should be unchanged)", input, got, input)
	}
}

func TestMaskDSN_AtInQueryParam(t *testing.T) {
	// A DSN like "file:test.db?user=foo@bar" has an @ but no scheme://
	// It should NOT be masked since there's no user:pass@host pattern
	input := "file:test.db?user=foo@bar"
	if got := maskDSN(input); got != input {
		t.Errorf("maskDSN(%q) = %q, want %q (@ in query params should not trigger masking)", input, got, input)
	}
}

// =============================================
// R9-B-005: ValidateMeta edge cases
// =============================================

func TestValidateMeta_Empty(t *testing.T) {
	if err := ValidateMeta(""); err != nil {
		t.Errorf("ValidateMeta(\"\") should be nil, got %v", err)
	}
}

func TestValidateMeta_EmptyObject(t *testing.T) {
	if err := ValidateMeta("{}"); err != nil {
		t.Errorf("ValidateMeta(\"{}\") should be nil, got %v", err)
	}
}

func TestValidateMeta_ValidJSON(t *testing.T) {
	if err := ValidateMeta(`{"key":"value"}`); err != nil {
		t.Errorf("ValidateMeta with valid JSON should be nil, got %v", err)
	}
}

func TestValidateMeta_InvalidJSON(t *testing.T) {
	if err := ValidateMeta("{not json}"); err == nil {
		t.Error("ValidateMeta with invalid JSON should return error")
	}
}

func TestValidateMeta_JSONNull(t *testing.T) {
	// "null" is valid JSON per json.Valid, but it's not a valid meta object.
	// Many downstream operations (json_each, json_patch, json_object_keys)
	// expect a JSON object. If ValidateMeta passes "null" through,
	// queries like json_each(meta) will fail at runtime.
	err := ValidateMeta("null")
	if err == nil {
		t.Error("ValidateMeta(\"null\") should return an error because " +
			"downstream operations like json_each() expect a JSON object, " +
			"not a JSON null literal")
	}
}

func TestValidateMeta_JSONArray(t *testing.T) {
	// Arrays are valid JSON but not valid meta objects
	err := ValidateMeta("[1,2,3]")
	if err == nil {
		t.Error("ValidateMeta(\"[1,2,3]\") should return an error because " +
			"meta must be a JSON object, not an array")
	}
}

func TestValidateMeta_JSONString(t *testing.T) {
	// A bare JSON string literal is valid JSON but not a valid meta object
	err := ValidateMeta(`"hello"`)
	if err == nil {
		t.Error("ValidateMeta('\"hello\"') should return an error because " +
			"meta must be a JSON object, not a string literal")
	}
}

func TestValidateMeta_JSONNumber(t *testing.T) {
	// A bare number is valid JSON but not a valid meta object
	err := ValidateMeta("42")
	if err == nil {
		t.Error("ValidateMeta(\"42\") should return an error because " +
			"meta must be a JSON object, not a number")
	}
}

// =============================================
// Coverage gap: DeleteNoteType leaves data inconsistent without transaction
// =============================================

func TestDeleteNoteType_NullsNoteReferences(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_notetype_null")

	// Create a note type
	nt, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{
		Name: "Test Type",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNoteType: %v", err)
	}

	// Create a note with that type
	note, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:       "Typed Note",
			NoteTypeId: nt.ID,
		},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateNote: %v", err)
	}

	// Delete the note type
	if err := ctx.DeleteNoteType(nt.ID); err != nil {
		t.Fatalf("DeleteNoteType: %v", err)
	}

	// Verify note still exists but NoteTypeId is nil
	var noteCheck models.Note
	if err := ctx.db.First(&noteCheck, note.ID).Error; err != nil {
		t.Fatalf("Note should still exist: %v", err)
	}
	if noteCheck.NoteTypeId != nil {
		t.Error("Note's NoteTypeId should be nil after note type deletion")
	}
}

func TestDeleteNoteType_NonExistent_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_notetype_nonexistent")

	err := ctx.DeleteNoteType(99999)
	if err == nil {
		t.Error("DeleteNoteType for non-existent ID should return error")
	}
}

// =============================================
// Coverage gap: DeleteResourceCategory
// =============================================

func TestDeleteResourceCategory_NullsResourceReferences(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_rescat_null")

	// Create a resource category
	rc, err := ctx.CreateResourceCategory(&query_models.ResourceCategoryCreator{
		Name: "Test Resource Category",
	})
	if err != nil {
		t.Fatalf("CreateResourceCategory: %v", err)
	}

	// Create a resource with that category
	res := &models.Resource{
		Name:               "Categorized Resource",
		ResourceCategoryId: &rc.ID,
	}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("Create resource: %v", err)
	}

	// Delete the resource category
	if err := ctx.DeleteResourceCategory(rc.ID); err != nil {
		t.Fatalf("DeleteResourceCategory: %v", err)
	}

	// Verify resource still exists but ResourceCategoryId is nil
	var resCheck models.Resource
	if err := ctx.db.First(&resCheck, res.ID).Error; err != nil {
		t.Fatalf("Resource should still exist: %v", err)
	}
	if resCheck.ResourceCategoryId != nil {
		t.Error("Resource's ResourceCategoryId should be nil after category deletion")
	}
}

func TestDeleteResourceCategory_NonExistent_ReturnsError(t *testing.T) {
	ctx := createCoverageTestContext(t, "delete_rescat_nonexistent")

	err := ctx.DeleteResourceCategory(99999)
	if err == nil {
		t.Error("DeleteResourceCategory for non-existent ID should return error")
	}
}

// =============================================
// Coverage gap: QueueForHashing / QueueForThumbnailing
// =============================================

func TestQueueForHashing_NilQueue(t *testing.T) {
	ctx := createCoverageTestContext(t, "queue_hash_nil")
	// hashQueue is nil by default
	if queued := ctx.QueueForHashing(1); queued {
		t.Error("QueueForHashing should return false when queue is nil")
	}
}

func TestQueueForHashing_WithQueue(t *testing.T) {
	ctx := createCoverageTestContext(t, "queue_hash_active")
	ch := make(chan uint, 10)
	ctx.SetHashQueue(ch)

	if !ctx.QueueForHashing(42) {
		t.Error("QueueForHashing should return true when queue has capacity")
	}
	got := <-ch
	if got != 42 {
		t.Errorf("Expected 42 on queue, got %d", got)
	}
}

func TestQueueForHashing_FullQueue(t *testing.T) {
	ctx := createCoverageTestContext(t, "queue_hash_full")
	ch := make(chan uint) // unbuffered = immediately full
	ctx.SetHashQueue(ch)

	if queued := ctx.QueueForHashing(1); queued {
		t.Error("QueueForHashing should return false when queue is full")
	}
}

func TestQueueForThumbnailing_NilQueue(t *testing.T) {
	ctx := createCoverageTestContext(t, "queue_thumb_nil")
	if queued := ctx.QueueForThumbnailing(1); queued {
		t.Error("QueueForThumbnailing should return false when queue is nil")
	}
}

func TestQueueForThumbnailing_WithQueue(t *testing.T) {
	ctx := createCoverageTestContext(t, "queue_thumb_active")
	ch := make(chan uint, 10)
	ctx.SetThumbnailQueue(ch)

	if !ctx.QueueForThumbnailing(42) {
		t.Error("QueueForThumbnailing should return true when queue has capacity")
	}
}

// =============================================
// Coverage gap: EnsureForeignKeysActive
// =============================================

func TestEnsureForeignKeysActive_NonSQLite(t *testing.T) {
	ctx := createCoverageTestContext(t, "fk_non_sqlite")
	ctx.Config.DbType = "POSTGRES"
	// Should be a no-op (not panic)
	ctx.EnsureForeignKeysActive(nil)
}

func TestEnsureForeignKeysActive_NilDB(t *testing.T) {
	ctx := createCoverageTestContext(t, "fk_nil_db")
	// Should use ctx.db when passed nil
	ctx.EnsureForeignKeysActive(nil)
	// No panic = pass
}

// =============================================
// Coverage gap: IsReadOnlyDBEnforced
// =============================================

func TestIsReadOnlyDBEnforced_NoReadOnlyDB(t *testing.T) {
	ctx := createCoverageTestContext(t, "readonly_none")
	ctx.readOnlyDB = nil
	if ctx.IsReadOnlyDBEnforced() {
		t.Error("Should return false when readOnlyDB is nil")
	}
}

func TestIsReadOnlyDBEnforced_SQLiteWithModeRO(t *testing.T) {
	ctx := createCoverageTestContext(t, "readonly_ro")
	ctx.Config.DbReadOnlyDsn = "file:test.db?mode=ro"
	if !ctx.IsReadOnlyDBEnforced() {
		t.Error("Should return true when DSN contains mode=ro")
	}
}

func TestIsReadOnlyDBEnforced_PostgresSeparateDSN(t *testing.T) {
	ctx := createCoverageTestContext(t, "readonly_pg")
	ctx.Config.DbType = constants.DbTypePosgres
	ctx.Config.DbDsn = "postgres://host/db1"
	ctx.Config.DbReadOnlyDsn = "postgres://host/db2"
	if !ctx.IsReadOnlyDBEnforced() {
		t.Error("Should return true when Postgres has separate DSNs")
	}
}

func TestIsReadOnlyDBEnforced_PostgresSameDSN(t *testing.T) {
	ctx := createCoverageTestContext(t, "readonly_pg_same")
	ctx.Config.DbType = constants.DbTypePosgres
	ctx.Config.DbDsn = "postgres://host/db1"
	ctx.Config.DbReadOnlyDsn = "postgres://host/db1"
	if ctx.IsReadOnlyDBEnforced() {
		t.Error("Should return false when Postgres has same DSN for read/write")
	}
}
