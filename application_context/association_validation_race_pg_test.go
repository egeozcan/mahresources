//go:build postgres && json1 && fts5

package application_context

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestDeleteRacingAValidatedGroupIsRefusedPG is the placement guard for the
// association validation in the upload path, and it only exists on Postgres.
//
// SQLite cannot produce the interleaving at all: one writer at a time means a
// delete issued while the upload transaction is open simply waits for it. Under
// Postgres READ COMMITTED it commits immediately and becomes visible, and
// `Association.Append` upserts its target — so the association write recreates
// the group as a blank stub carrying nothing but the id, with no foreign key to
// object because by then the row exists again.
//
// ValidateAndLockAssociationIDs takes SELECT ... FOR UPDATE on the rows it
// validated, so the deleter blocks until the upload's transaction ends. Remove
// that clause and this test resurrects a blank group.
//
// The delete is injected rather than raced for, on its own connection so it does
// not simply join the transaction under test.
func TestDeleteRacingAValidatedGroupIsRefusedPG(t *testing.T) {
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.Group{}, &models.Tag{}, &models.Note{},
		&models.Category{}, &models.ResourceCategory{}, &models.Series{},
		&models.ResourceVersion{}, &models.Preview{},
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

	owner := &models.Group{Name: "race-owner"}
	if err := db.Create(owner).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}
	victim := &models.Group{Name: "race-victim"}
	if err := db.Create(victim).Error; err != nil {
		t.Fatalf("create victim group: %v", err)
	}
	victimID := victim.ID

	payload := []byte("racing the validation of an association id")
	if _, err := ctx.AddResource(newBytesFile(payload), "original.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "original", OwnerId: owner.ID},
	}); err != nil {
		t.Fatalf("seed upload: %v", err)
	}

	deleter, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open deleter connection: %v", err)
	}
	t.Cleanup(func() { deleter.Close() })

	// Fires on the group upsert Append performs: after validation accepted the
	// id, before the association row lands.
	var once sync.Once
	var callbackFired, deleteBlocked bool
	var deleteErr error
	if err := db.Callback().Create().Before("gorm:create").Register(
		"test:delete_validated_group",
		func(tx *gorm.DB) {
			if tx.Statement == nil || tx.Statement.Table != "groups" {
				return
			}
			once.Do(func() {
				callbackFired = true
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, deleteErr = deleter.Exec("DELETE FROM groups WHERE id = $1", victimID)
				}()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					// Waiting on the row lock, which is the fix working.
					deleteBlocked = true
				}
			})
		},
	); err != nil {
		t.Fatalf("register delete callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove("test:delete_validated_group")
	})

	_, uploadErr := ctx.AddResource(newBytesFile(payload), "duplicate.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:    "duplicate",
			OwnerId: owner.ID,
			Groups:  []uint{victimID},
		},
	})

	// Controls first. Without these the interesting assertion below passes for
	// any number of uninteresting reasons: a callback that stopped matching the
	// groups table, an upload that failed before it ever reached the append, or
	// a delete that errored instead of racing.
	if !callbackFired {
		t.Fatal("the delete was never injected, so no race was observed and the result below means nothing")
	}
	var exists *ResourceExistsError
	if uploadErr != nil && !errors.As(uploadErr, &exists) {
		t.Fatalf("the duplicate upload should have reached the association append, got: %v", uploadErr)
	}
	if !deleteBlocked && deleteErr != nil {
		t.Fatalf("the injected delete neither blocked nor succeeded: %v", deleteErr)
	}

	var revived models.Group
	if lookupErr := db.First(&revived, victimID).Error; lookupErr == nil && revived.Name == "" {
		t.Fatalf("group %d was resurrected as a blank stub by Association.Append (delete blocked: %v)", victimID, deleteBlocked)
	}
}
