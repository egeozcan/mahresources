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

func TestExportImportPreservesEntityPickerSlots(t *testing.T) {
	src := createGUIDIsolatedContext(t, t.Name()+"-src")
	const html = `<strong>[property path="Name"]</strong>`
	const css = `.picker-card{color:blue}`
	category := models.Category{Name: "Picker groups", CustomEntityPickerResult: html, CustomEntityPickerResultCSS: css}
	noteType := models.NoteType{Name: "Picker notes", CustomEntityPickerResult: html, CustomEntityPickerResultCSS: css}
	resourceCategory := models.ResourceCategory{Name: "Picker resources", CustomEntityPickerResult: html, CustomEntityPickerResultCSS: css}
	require.NoError(t, src.db.Create(&category).Error)
	require.NoError(t, src.db.Create(&noteType).Error)
	require.NoError(t, src.db.Create(&resourceCategory).Error)
	root := mustCreateGroup(t, src, "Picker root", nil)
	require.NoError(t, src.db.Model(root).Update("category_id", category.ID).Error)
	note := mustCreateNote(t, src, "Picker note", &root.ID)
	require.NoError(t, src.db.Model(note).Update("note_type_id", noteType.ID).Error)
	resource := mustCreateResource(t, src, "Picker resource", &root.ID, []byte("picker fixture"))
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
	require.NoError(t, afero.WriteFile(dst.fs, "_imports/picker.tar", buf.Bytes(), 0644))
	plan, err := dst.ParseImport(context.Background(), "picker", "_imports/picker.tar")
	require.NoError(t, err)
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "merge"
	_, err = dst.ApplyImport(context.Background(), "picker", decisions, noopSinkAltFS{})
	require.NoError(t, err)
	for _, table := range []string{"categories", "note_types", "resource_categories"} {
		var saved struct{ CustomEntityPickerResult, CustomEntityPickerResultCSS string }
		require.NoError(t, dst.db.Table(table).Where("name LIKE ?", "Picker %").First(&saved).Error)
		require.Equal(t, html, saved.CustomEntityPickerResult, table)
		require.Equal(t, css, saved.CustomEntityPickerResultCSS, table)
	}
}
