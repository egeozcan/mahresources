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

// Each carrier gets its own values, so a copy wired for one carrier only (or
// reading another carrier's field) fails here even though the source-level
// wiring guard is satisfied by any one matching assignment in the file.
func TestExportImportPreservesPanelSummarySlots(t *testing.T) {
	src := createGUIDIsolatedContext(t, t.Name()+"-src")
	want := map[string][2]string{
		"categories":          {`<em>group [property path="Name"]</em>`, `.ps-group{color:red}`},
		"note_types":          {`<em>note [property path="Name"]</em>`, `.ps-note{color:green}`},
		"resource_categories": {`<em>resource [property path="Name"]</em>`, `.ps-resource{color:blue}`},
	}
	category := models.Category{Name: "Panel groups", CustomPanelSummary: want["categories"][0], CustomPanelSummaryCSS: want["categories"][1]}
	noteType := models.NoteType{Name: "Panel notes", CustomPanelSummary: want["note_types"][0], CustomPanelSummaryCSS: want["note_types"][1]}
	resourceCategory := models.ResourceCategory{Name: "Panel resources", CustomPanelSummary: want["resource_categories"][0], CustomPanelSummaryCSS: want["resource_categories"][1]}
	require.NoError(t, src.db.Create(&category).Error)
	require.NoError(t, src.db.Create(&noteType).Error)
	require.NoError(t, src.db.Create(&resourceCategory).Error)
	root := mustCreateGroup(t, src, "Panel root", nil)
	require.NoError(t, src.db.Model(root).Update("category_id", category.ID).Error)
	note := mustCreateNote(t, src, "Panel note", &root.ID)
	require.NoError(t, src.db.Model(note).Update("note_type_id", noteType.ID).Error)
	resource := mustCreateResource(t, src, "Panel resource", &root.ID, []byte("panel summary fixture"))
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
	require.NoError(t, afero.WriteFile(dst.fs, "_imports/panel.tar", buf.Bytes(), 0644))
	plan, err := dst.ParseImport(context.Background(), "panel", "_imports/panel.tar")
	require.NoError(t, err)
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "merge"
	_, err = dst.ApplyImport(context.Background(), "panel", importPlanPath("panel"), decisions, noopSinkAltFS{})
	require.NoError(t, err)
	for table, values := range want {
		var saved struct{ CustomPanelSummary, CustomPanelSummaryCSS string }
		require.NoError(t, dst.db.Table(table).Where("name LIKE ?", "Panel %").First(&saved).Error)
		require.Equal(t, values[0], saved.CustomPanelSummary, table)
		require.Equal(t, values[1], saved.CustomPanelSummaryCSS, table)
	}
}
