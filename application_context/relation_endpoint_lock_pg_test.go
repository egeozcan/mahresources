//go:build postgres && json1 && fts5

package application_context

import (
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

// TestARelationHoldsItsEndpointsUntilItCommitsPG pins the Postgres half of
// AddRelation's category check. Under READ COMMITTED a check inside the
// transaction still sees whatever commits the instant after it, so the endpoint
// rows are locked when they are checked, and a category change issued between
// the check and the commit has to wait for the relation to land. (UpdateGroup
// updates the group row before its edge cleanup, so in the product that wait is
// what lets the cleanup see the new edge; this test asserts only the wait.)
// Remove the lock clause and the change commits at once.
//
// The change is a raw UPDATE injected on its own connection, at the back
// relation's insert, which comes after the check.
func TestARelationHoldsItsEndpointsUntilItCommitsPG(t *testing.T) {
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Group{}, &models.GroupRelationType{}, &models.GroupRelation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})

	from, to, elsewhere := &models.Category{Name: "From"}, &models.Category{Name: "To"}, &models.Category{Name: "Elsewhere"}
	for _, category := range []*models.Category{from, to, elsewhere} {
		if err := db.Create(category).Error; err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	fromGroup := &models.Group{Name: "from", CategoryId: &from.ID}
	toGroup := &models.Group{Name: "to", CategoryId: &to.ID}
	for _, group := range []*models.Group{fromGroup, toGroup} {
		if err := db.Create(group).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	back := &models.GroupRelationType{Name: "is linked from", FromCategoryId: &to.ID, ToCategoryId: &from.ID}
	if err := db.Create(back).Error; err != nil {
		t.Fatalf("create back type: %v", err)
	}
	relationType := &models.GroupRelationType{Name: "links to", FromCategoryId: &from.ID, ToCategoryId: &to.ID, BackRelationId: &back.ID}
	if err := db.Create(relationType).Error; err != nil {
		t.Fatalf("create type: %v", err)
	}

	changer, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open changer connection: %v", err)
	}
	t.Cleanup(func() { changer.Close() })

	var inserts int
	var once sync.Once
	var changeBlocked bool
	var changeErr error
	changed := make(chan struct{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:recategorize_after_check", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "group_relations" {
			return
		}
		inserts++
		if inserts < 2 {
			return
		}
		once.Do(func() {
			go func() {
				defer close(changed)
				_, changeErr = changer.Exec("UPDATE groups SET category_id = $1 WHERE id = $2", elsewhere.ID, fromGroup.ID)
			}()
			select {
			case <-changed:
			case <-time.After(3 * time.Second):
				// Waiting on the row lock, which is the check holding.
				changeBlocked = true
			}
		})
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:recategorize_after_check") })

	if _, err := ctx.AddRelation(fromGroup.ID, toGroup.ID, relationType.ID, "linked", ""); err != nil {
		t.Fatalf("add relation: %v", err)
	}
	if inserts < 2 {
		t.Fatal("the back relation was never inserted, so the change was never injected and nothing below means anything")
	}
	<-changed
	if changeErr != nil {
		t.Fatalf("the injected category change failed instead of waiting: %v", changeErr)
	}
	if !changeBlocked {
		t.Fatal("a category change committed between the relation's check and its commit: the endpoint rows were not locked")
	}
}
