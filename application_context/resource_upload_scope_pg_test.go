//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/models"
)

// The reference count runs inside the delete's transaction; on Postgres that is
// a real transaction handle, and dropping the subtree filter must keep it.
func TestDeletingEitherResourceThatSharesAFileKeepsTheOthersFilePG(t *testing.T) {
	runSharedFileDeletionCases(t, func(t *testing.T) *MahresourcesContext {
		db, dsn := pgContainer.CreateTestDBWithDSN(t)
		if err := db.AutoMigrate(
			&models.Resource{}, &models.Group{}, &models.Tag{}, &models.Note{},
			&models.Category{}, &models.ResourceCategory{}, &models.Series{},
			&models.ResourceVersion{}, &models.Preview{}, &models.ImageHash{},
			&models.ResourceSimilarity{}, &models.LogEntry{}, &models.NoteBlock{},
			&models.NoteType{}, &models.GroupRelation{}, &models.GroupRelationType{},
		); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		readOnly, err := sqlx.Connect("pgx", dsn)
		if err != nil {
			t.Fatalf("open read-only handle: %v", err)
		}
		t.Cleanup(func() { readOnly.Close() })
		ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
			DbType: constants.DbTypePosgres,
		})
		defaultRC := &models.ResourceCategory{Name: "Default"}
		defaultRC.ID = 1
		db.FirstOrCreate(defaultRC, 1)
		return ctx
	})
}
