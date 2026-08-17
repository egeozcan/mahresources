package application_context

import (
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The group and note write paths must be callable from inside a transaction a
// caller has already opened.
//
// They could not be until this file was written. GORM's Begin switches on
// Statement.ConnPool; inside a transaction that pool is a *sql.Tx, which
// satisfies neither TxBeginner nor ConnPoolBeginner, so Begin returns
// ErrInvalidTransaction — and CreateGroup never checked tx.Error, so the failure
// surfaced as an opaque error from the first statement rather than as itself.
// db.Transaction takes the other branch: on a pool that is already a
// transaction it issues a SAVEPOINT, which is what makes these paths nest.
//
// mah.db.transaction is the caller that needs this. Without it, the three most
// ordinary things a plugin writes — a group, a note, an update to either —
// would be exactly the ones it could not put in a transaction.

// errPluginTransactionRollbackTest is returned to force a rollback; nothing in
// production produces it, so a test that sees it knows the rollback was the
// test's own doing rather than a real failure.
var errPluginTransactionRollbackTest = errors.New("nested write test: roll back")

func createNestedWriteContext(t *testing.T, name string) *MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.ResourceVersion{}, &models.Note{},
		&models.Tag{}, &models.Group{}, &models.Category{}, &models.NoteType{},
		&models.Preview{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ImageHash{}, &models.ResourceSimilarity{}, &models.LogEntry{},
		&models.ResourceCategory{}, &models.Series{}, &models.NoteBlock{}, &models.PluginKV{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	config := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	sqlDB, _ := db.DB()
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), config)
}

func TestGroupAndNoteWritesNestInsideATransaction(t *testing.T) {
	ctx := createNestedWriteContext(t, "nested_writes")

	var groupID, noteID uint

	if err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		group, err := txCtx.CreateGroup(&query_models.GroupCreator{Name: "nested-group"})
		if err != nil {
			return err
		}
		groupID = group.ID

		if _, err := txCtx.UpdateGroup(&query_models.GroupEditor{
			GroupCreator: query_models.GroupCreator{Name: "nested-group-renamed"},
			ID:           group.ID,
		}); err != nil {
			return err
		}

		note, err := txCtx.CreateOrUpdateNote(&query_models.NoteEditor{
			NoteCreator: query_models.NoteCreator{Name: "nested-note", OwnerId: group.ID},
		})
		if err != nil {
			return err
		}
		noteID = note.ID

		if _, err := txCtx.CreateOrUpdateNote(&query_models.NoteEditor{
			NoteCreator: query_models.NoteCreator{Name: "nested-note-renamed", OwnerId: group.ID},
			ID:          note.ID,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("group and note writes inside a transaction: %v", err)
	}

	var name string
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", groupID).Pluck("name", &name).Error; err != nil {
		t.Fatalf("read committed group: %v", err)
	}
	if name != "nested-group-renamed" {
		t.Errorf("group name after commit = %q, want %q", name, "nested-group-renamed")
	}
	if err := ctx.db.Model(&models.Note{}).Where("id = ?", noteID).Pluck("name", &name).Error; err != nil {
		t.Fatalf("read committed note: %v", err)
	}
	if name != "nested-note-renamed" {
		t.Errorf("note name after commit = %q, want %q", name, "nested-note-renamed")
	}
}

// A failure inside the nested path must roll back to the caller's transaction,
// not commit half of it. The savepoint is what makes the group vanish here: if
// CreateGroup still committed on its own, the row would survive the outer
// rollback.
func TestNestedGroupWriteRollsBackWithTheOuterTransaction(t *testing.T) {
	ctx := createNestedWriteContext(t, "nested_rollback")

	sentinel := errPluginTransactionRollbackTest
	var createdID uint

	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		group, err := txCtx.CreateGroup(&query_models.GroupCreator{Name: "doomed-group"})
		if err != nil {
			return err
		}
		createdID = group.ID
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("WithTransaction err = %v, want the sentinel", err)
	}
	if createdID == 0 {
		t.Fatal("CreateGroup did not run inside the transaction")
	}

	var count int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", createdID).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("group %d survived the outer rollback: CreateGroup committed on its own", createdID)
	}
}
