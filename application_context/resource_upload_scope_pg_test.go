//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/models"
)

func TestDeletingEitherResourceThatSharesAFileKeepsTheOthersFilePG(t *testing.T) {
	runSharedFileDeletionCases(t, newSharedFilePGContext)
}

// Postgres commits concurrent deletes independently, so a count taken inside
// each delete's transaction still sees the row the other is deleting. Deleting
// the last two rows over one file that way kept the file with nothing pointing
// at it. SQLite cannot interleave them: its writers serialize.
func TestInterleavedDeletesOfTheLastRowsOverAFileRemoveItPG(t *testing.T) {
	const body = "one file behind two rows deleted at once"
	ctx := newSharedFilePGContext(t)
	outside := createGroupNamed(t, ctx, "interleaved-outside", nil)
	inside := createGroupNamed(t, ctx, "interleaved-inside", nil)
	first := uploadAs(t, ctx, body, "first.txt", outside.ID)
	second := uploadAs(t, ctx.WithPrincipal(scopedPrincipalFor(inside)), body, "second.txt", inside.ID)
	if first.Location != second.Location {
		t.Fatalf("the two rows do not share a file: %s and %s", first.Location, second.Location)
	}

	// The second delete runs to completion, on its own connection, while the
	// first has deleted its row and not yet committed.
	deleteResourceBeforeCommit = func() {
		deleteResourceBeforeCommit = nil
		if err := ctx.DeleteResource(second.ID); err != nil {
			t.Errorf("the interleaved delete: %v", err)
		}
	}
	t.Cleanup(func() { deleteResourceBeforeCommit = nil })
	if err := ctx.DeleteResource(first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	exists, err := afero.Exists(ctx.fs, first.GetCleanLocation())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if exists {
		t.Fatalf("the file outlived both rows that referenced it")
	}
}

func newSharedFilePGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
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
}
