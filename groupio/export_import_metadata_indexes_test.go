package groupio

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"mahresources/archive"
	"mahresources/models"
)

func TestExportImportPreservesCategoryMetadataIndexes(t *testing.T) {
	src := createGUIDIsolatedContext(t, t.Name()+"-src")
	keys := `[{"key":"score","kind":"numeric"},{"key":"camera.model","kind":"text"}]`
	category := models.Category{Name: "Indexed groups", MetadataIndexes: keys}
	noteType := models.NoteType{Name: "Indexed notes", MetadataIndexes: keys}
	resourceCategory := models.ResourceCategory{Name: "Indexed resources", MetadataIndexes: keys}
	require.NoError(t, src.db.Create(&category).Error)
	require.NoError(t, src.db.Create(&noteType).Error)
	require.NoError(t, src.db.Create(&resourceCategory).Error)
	root := mustCreateGroup(t, src, "Indexed root", nil)
	require.NoError(t, src.db.Model(root).Update("category_id", category.ID).Error)
	note := mustCreateNote(t, src, "Indexed note", &root.ID)
	require.NoError(t, src.db.Model(note).Update("note_type_id", noteType.ID).Error)
	resource := mustCreateResource(t, src, "Indexed resource", &root.ID, []byte("metadata index fixture"))
	require.NoError(t, src.db.Model(resource).Update("resource_category_id", resourceCategory.ID).Error)

	var buf bytes.Buffer
	require.NoError(t, src.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &buf, nil))
	dst := createGUIDIsolatedContext(t, t.Name()+"-dst")
	require.NoError(t, dst.fs.MkdirAll("_imports", 0755))
	require.NoError(t, afero.WriteFile(dst.fs, "_imports/indexes.tar", buf.Bytes(), 0644))
	plan, err := dst.ParseImport(context.Background(), "indexes", "_imports/indexes.tar")
	require.NoError(t, err)
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "merge"
	_, err = dst.ApplyImport(context.Background(), "indexes", decisions, noopSinkAltFS{})
	require.NoError(t, err)

	for _, table := range []string{"categories", "note_types", "resource_categories"} {
		var saved struct{ MetadataIndexes string }
		require.NoError(t, dst.db.Table(table).Where("name LIKE ?", "Indexed %").First(&saved).Error)
		require.JSONEq(t, keys, saved.MetadataIndexes, table)
	}
}
