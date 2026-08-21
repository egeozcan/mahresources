package application_context

import (
	"strings"
	"testing"

	"mahresources/models"
)

// CRUDWriter.Delete used `Select(clause.Associations)`, which tells GORM to
// delete *every* association. For a many-to-many that means the join rows,
// which is what the select was written for. For a has-many it means the child
// rows themselves.
//
// Four of the six models this writer is instantiated for carry a has-many:
// Category.Groups, ResourceCategory.Resources, NoteType.Notes and
// Series.Resources. Deleting a category through the generic writer therefore
// deleted every group in it — exactly what DeleteCategory's own comment
// forbids. None of those Deletes is routed today (only Tag and Query reach
// DeleteHandler), so this is a loaded gun rather than a live defect.
//
// The writer now refuses instead of orphaning. Silently leaving children with a
// dangling foreign key would be a quieter bug than deleting them and no more
// correct: FK constraints are not enforced on every deployment, so nothing
// downstream would catch it.
func TestCRUDWriterDelete_DoesNotCascadeIntoHasManyChildren(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	t.Run("category keeps its groups", func(t *testing.T) {
		category := &models.Category{Name: "crud-delete-category"}
		if err := ctx.db.Create(category).Error; err != nil {
			t.Fatalf("create category: %v", err)
		}
		group := &models.Group{Name: "member-group", CategoryId: &category.ID}
		if err := ctx.db.Create(group).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		_, writer := ctx.CategoryCRUD()
		err := writer.Delete(category.ID)
		if err == nil {
			t.Fatal("deleting a category through the generic writer was allowed; it owns groups")
		}
		if !strings.Contains(err.Error(), "Groups") {
			t.Errorf("refusal is %q, want it to name the Groups association", err)
		}

		var groups int64
		if err := ctx.db.Model(&models.Group{}).Where("id = ?", group.ID).Count(&groups).Error; err != nil {
			t.Fatalf("count groups: %v", err)
		}
		if groups == 0 {
			t.Error("deleting a category through the generic writer deleted the groups in it")
		}
	})

	t.Run("resource category keeps its resources", func(t *testing.T) {
		category := &models.ResourceCategory{Name: "crud-delete-rescat"}
		if err := ctx.db.Create(category).Error; err != nil {
			t.Fatalf("create resource category: %v", err)
		}
		resource := &models.Resource{Name: "member-resource", ResourceCategoryId: category.ID}
		if err := ctx.db.Create(resource).Error; err != nil {
			t.Fatalf("create resource: %v", err)
		}

		_, writer := ctx.ResourceCategoryCRUD()
		if err := writer.Delete(category.ID); err == nil {
			t.Fatal("deleting a resource category through the generic writer was allowed; it owns resources")
		}

		var resources int64
		if err := ctx.db.Model(&models.Resource{}).Where("id = ?", resource.ID).Count(&resources).Error; err != nil {
			t.Fatalf("count resources: %v", err)
		}
		if resources == 0 {
			t.Error("deleting a resource category through the generic writer deleted the resources in it")
		}
	})

	t.Run("note type keeps its notes", func(t *testing.T) {
		noteType := &models.NoteType{Name: "crud-delete-notetype"}
		if err := ctx.db.Create(noteType).Error; err != nil {
			t.Fatalf("create note type: %v", err)
		}
		note := &models.Note{Name: "member-note", NoteTypeId: &noteType.ID}
		if err := ctx.db.Create(note).Error; err != nil {
			t.Fatalf("create note: %v", err)
		}

		_, writer := ctx.NoteTypeCRUD()
		if err := writer.Delete(noteType.ID); err == nil {
			t.Fatal("deleting a note type through the generic writer was allowed; it owns notes")
		}

		var notes int64
		if err := ctx.db.Model(&models.Note{}).Where("id = ?", note.ID).Count(&notes).Error; err != nil {
			t.Fatalf("count notes: %v", err)
		}
		if notes == 0 {
			t.Error("deleting a note type through the generic writer deleted the notes of that type")
		}
	})
}

// The other direction. Tag is the model whose Delete *is* routed, and its three
// associations are all many-to-many, so the join rows must still be cleared —
// otherwise the fix above would trade a cascade defect for orphaned join rows.
func TestCRUDWriterDelete_StillClearsManyToManyJoinRows(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	tag := &models.Tag{Name: "crud-delete-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	group := &models.Group{Name: "tagged-group"}
	if err := ctx.db.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := ctx.db.Model(group).Association("Tags").Append(tag); err != nil {
		t.Fatalf("tag the group: %v", err)
	}

	var joined int64
	if err := ctx.db.Table("group_tags").Where("tag_id = ?", tag.ID).Count(&joined).Error; err != nil {
		t.Fatalf("count join rows: %v", err)
	}
	if joined == 0 {
		t.Fatal("fixture did not create a join row, so the assertion below would be vacuous")
	}

	_, writer := ctx.TagCRUD()
	if err := writer.Delete(tag.ID); err != nil {
		t.Fatalf("delete tag: %v", err)
	}

	if err := ctx.db.Table("group_tags").Where("tag_id = ?", tag.ID).Count(&joined).Error; err != nil {
		t.Fatalf("re-count join rows: %v", err)
	}
	if joined != 0 {
		t.Error("deleting a tag left its group_tags join rows behind")
	}

	var groups int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", group.ID).Count(&groups).Error; err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if groups == 0 {
		t.Error("deleting a tag deleted the group it was attached to")
	}
}

// The second routed Delete. Query has no associations at all, so it exercises
// the "no joins, nothing owned" path — the one the refusal above must not
// swallow. Both routed deletes (tag, query) are pinned here precisely because
// the refusal is keyed on the model's shape: a model gaining an association
// later would change what its route does, and this is what would say so.
func TestCRUDWriterDelete_RoutedQueryDeleteStillWorks(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	// Migrated here rather than in the shared helper: queries is not in that
	// helper's AutoMigrate list, and every test DB in this tree carries its own.
	if err := ctx.db.AutoMigrate(&models.Query{}); err != nil {
		t.Fatalf("migrate queries: %v", err)
	}

	query := &models.Query{Name: "crud-delete-query", Text: "select 1"}
	if err := ctx.db.Create(query).Error; err != nil {
		t.Fatalf("create query: %v", err)
	}

	_, writer := ctx.QueryCRUD()
	if err := writer.Delete(query.ID); err != nil {
		t.Fatalf("delete query: %v", err)
	}

	var remaining int64
	if err := ctx.db.Model(&models.Query{}).Where("id = ?", query.ID).Count(&remaining).Error; err != nil {
		t.Fatalf("count queries: %v", err)
	}
	if remaining != 0 {
		t.Error("the routed query delete no longer deletes the query")
	}
}

// A belongs-to is not owned data: the foreign key is on the row being deleted,
// so deleting it neither removes nor orphans the row it points at. The first
// version of the refusal above lumped belongs-to in with has-many and would
// have refused a perfectly safe delete; no model the writer serves today has
// one, so only this test says so.
func TestAssociationShapes_BelongsToIsNeitherClearedNorRefused(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	// models.Group has a belongs-to (Category, via CategoryId) alongside its
	// many-to-many Tags, which makes it the one model that separates the two.
	joins, owned, err := associationShapes(ctx.db, &models.Group{})
	if err != nil {
		t.Fatalf("associationShapes: %v", err)
	}

	for _, name := range owned {
		if name == "Category" {
			t.Error("Category is a belongs-to and was classified as owned; a model with one would be refused for no reason")
		}
	}
	for _, name := range joins {
		if name == "Category" {
			t.Error("Category is a belongs-to and was selected for deletion; that would delete the parent row")
		}
	}
}
