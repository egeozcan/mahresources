package application_context

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/util"
)

// setupSharedCacheTestContext mirrors setupTestContext but uses a shared-cache
// in-memory database. executeCrossEntity queries the three entity tables on
// concurrent pool connections, and with cache=private each extra connection
// would see its own empty database.
func setupSharedCacheTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.NoteBlock{},
	); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	util.AddInitialData(db)

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	return NewMahresourcesContext(nil, db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

// TestMRQLCrossEntityOrderByRandom pins the cross-entity ORDER BY RANDOM() path:
// a query without a `type =` filter runs through executeCrossEntity, whose global
// sorter must not dereference the nil Field of a random order clause. Before the
// fix this panicked as soon as two items needed comparing.
func TestMRQLCrossEntityOrderByRandom(t *testing.T) {
	ctx := setupSharedCacheTestContext(t)

	for i := 0; i < 3; i++ {
		if err := ctx.db.Create(&models.Group{Name: fmt.Sprintf("rndx-group-%d", i)}).Error; err != nil {
			t.Fatalf("seed group: %v", err)
		}
		if err := ctx.db.Create(&models.Note{Name: fmt.Sprintf("rndx-note-%d", i)}).Error; err != nil {
			t.Fatalf("seed note: %v", err)
		}
	}

	res, err := ctx.ExecuteMRQL(context.Background(), `name ~ "rndx-" ORDER BY RANDOM() LIMIT 4`, 0, 0, nil)
	if err != nil {
		t.Fatalf("cross-entity ORDER BY RANDOM() error: %v", err)
	}
	total := len(res.Resources) + len(res.Notes) + len(res.Groups)
	if total == 0 || total > 4 {
		t.Fatalf("expected 1-4 items from random cross-entity sample, got %d", total)
	}
}

// TestMRQLCrossEntityRandomProportional pins the proportional-sampling property
// of cross-entity ORDER BY RANDOM(): a rare entity type must not be
// overrepresented. With 99 notes and 1 group both matching, a true global random
// sample of 10 includes the single group ~10% of the time. The old per-entity
// cap fetched min(count, limit) rows per type, so the pool was 10 notes + 1 group
// and the group appeared ~10/11 ≈ 91% of the time.
func TestMRQLCrossEntityRandomProportional(t *testing.T) {
	ctx := setupSharedCacheTestContext(t)

	for i := 0; i < 99; i++ {
		if err := ctx.db.Create(&models.Note{Name: fmt.Sprintf("prop-note-%d", i)}).Error; err != nil {
			t.Fatalf("seed note: %v", err)
		}
	}
	if err := ctx.db.Create(&models.Group{Name: "prop-group-0"}).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}

	const iterations = 120
	groupHits := 0
	for i := 0; i < iterations; i++ {
		res, err := ctx.ExecuteMRQL(context.Background(), `name ~ "prop-" ORDER BY RANDOM() LIMIT 10`, 0, 0, nil)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		total := len(res.Resources) + len(res.Notes) + len(res.Groups)
		if total != 10 {
			t.Fatalf("iteration %d: expected 10 items, got %d", i, total)
		}
		if len(res.Groups) > 0 {
			groupHits++
		}
	}

	// True proportional rate is ~12/120; the buggy per-entity cap gives ~109/120.
	// A generous ceiling separates the two regimes without flaking.
	if groupHits > 50 {
		t.Fatalf("group overrepresented in random sample: appeared in %d/%d draws (expected ~%d); per-entity cap not corrected",
			groupHits, iterations, iterations*1/100)
	}
	if groupHits == 0 {
		t.Fatalf("group never sampled in %d draws; expected ~%d", iterations, iterations*1/100)
	}
}

// TestMRQLCrossEntityRandomTiebreak covers RANDOM() combined with a real sort
// key (`ORDER BY created, RANDOM()`): the field clauses still sort, the random
// clause only breaks ties, and nothing panics.
func TestMRQLCrossEntityRandomTiebreak(t *testing.T) {
	ctx := setupSharedCacheTestContext(t)

	for i := 0; i < 3; i++ {
		if err := ctx.db.Create(&models.Group{Name: fmt.Sprintf("rndy-group-%d", i)}).Error; err != nil {
			t.Fatalf("seed group: %v", err)
		}
	}

	res, err := ctx.ExecuteMRQL(context.Background(), `name ~ "rndy-" ORDER BY created, RANDOM() LIMIT 3`, 0, 0, nil)
	if err != nil {
		t.Fatalf("cross-entity ORDER BY created, RANDOM() error: %v", err)
	}
	if len(res.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(res.Groups))
	}
}
