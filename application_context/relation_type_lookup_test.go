package application_context

import (
	"errors"
	"testing"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/query_models"
)

// AddRelationType looks for an existing reverse type before inserting one, and
// adopts it when it finds one. That adoption is deliberate — it is how a pair
// created separately gets linked, and inserting a duplicate instead would be
// worse — but the lookup used `if err == nil { ... }`, so every error was
// treated as "not found". A transient read failure therefore fell through to
// the insert and produced a second, unlinked reverse type, with nothing
// reported to the caller.
//
// The failure is injected with a GORM callback rather than raced, because a
// read error is not reachable from the public API: the callback makes the
// interleave deterministic instead of hoping for it.
func TestAddRelationType_ReverseLookupFailureIsReported(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	from := &models.Category{Name: "lookup-from"}
	to := &models.Category{Name: "lookup-to"}
	if err := ctx.db.Create(from).Error; err != nil {
		t.Fatalf("create from category: %v", err)
	}
	if err := ctx.db.Create(to).Error; err != nil {
		t.Fatalf("create to category: %v", err)
	}

	boom := errors.New("injected reverse-type lookup failure")
	const callbackName = "test:fail_relation_type_lookup"
	if err := ctx.db.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "group_relation_types" {
			db.AddError(boom)
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	defer func() {
		if err := ctx.db.Callback().Query().Remove(callbackName); err != nil {
			t.Fatalf("remove callback: %v", err)
		}
	}()

	_, err := ctx.AddRelationType(&query_models.RelationshipTypeEditorQuery{
		Name:         "lookup-forward",
		ReverseName:  "lookup-back",
		FromCategory: from.ID,
		ToCategory:   to.ID,
	})
	if err == nil {
		t.Fatal("a failing reverse-type lookup was reported as success")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error is %v, want the injected lookup failure", err)
	}
}

// The other direction, so the switch above cannot be written as a blanket
// refusal: when the reverse type genuinely does not exist, it is still created
// and linked, and when it does exist it is adopted rather than duplicated.
func TestAddRelationType_AdoptsAnExistingReverseTypeWithoutDuplicating(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	from := &models.Category{Name: "adopt-from"}
	to := &models.Category{Name: "adopt-to"}
	if err := ctx.db.Create(from).Error; err != nil {
		t.Fatalf("create from category: %v", err)
	}
	if err := ctx.db.Create(to).Error; err != nil {
		t.Fatalf("create to category: %v", err)
	}

	// A reverse type that already exists, unlinked.
	existing := &models.GroupRelationType{
		Name:           "adopt-back",
		FromCategoryId: &to.ID,
		ToCategoryId:   &from.ID,
	}
	if err := ctx.db.Create(existing).Error; err != nil {
		t.Fatalf("create existing reverse type: %v", err)
	}

	created, err := ctx.AddRelationType(&query_models.RelationshipTypeEditorQuery{
		Name:         "adopt-forward",
		ReverseName:  "adopt-back",
		FromCategory: from.ID,
		ToCategory:   to.ID,
	})
	if err != nil {
		t.Fatalf("add relation type: %v", err)
	}

	var backCount int64
	if err := ctx.db.Model(&models.GroupRelationType{}).Where("name = ?", "adopt-back").Count(&backCount).Error; err != nil {
		t.Fatalf("count reverse types: %v", err)
	}
	if backCount != 1 {
		t.Errorf("there are %d reverse types named adopt-back, want 1: the existing one was duplicated", backCount)
	}

	var adopted models.GroupRelationType
	if err := ctx.db.First(&adopted, existing.ID).Error; err != nil {
		t.Fatalf("re-read the existing reverse type: %v", err)
	}
	if adopted.BackRelationId == nil || *adopted.BackRelationId != created.ID {
		t.Errorf("existing reverse type points at %v, want %d: it was not adopted", adopted.BackRelationId, created.ID)
	}
	if created.BackRelationId == nil || *created.BackRelationId != existing.ID {
		t.Errorf("created type points at %v, want %d", created.BackRelationId, existing.ID)
	}
}
