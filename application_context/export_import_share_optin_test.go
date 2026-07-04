package application_context

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

// A note type's ApplyTemplatesToShares opt-in (whether its CustomHeader/CustomCSS
// render on public share pages) must survive the export → import round-trip, just
// like the other note-type template fields. Regression guard for the review
// finding that the flag was silently dropped by the archive path.
func TestExportImport_PreservesNoteTypeShareOptIn(t *testing.T) {
	srcCtx := createGUIDIsolatedContext(t, t.Name()+"-src")

	nt := &models.NoteType{
		Name:                   "ShareOptInType",
		ApplyTemplatesToShares: true,
		CustomHeader:           `<div>[property path="Name"]</div>`,
		CustomCSS:              ".x{color:red}",
	}
	if err := srcCtx.db.Create(nt).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}

	root := mustCreateGroup(t, srcCtx, "share-optin-root", nil)
	note := &models.Note{Name: "share-optin-note", NoteTypeId: &nt.ID, OwnerId: &root.ID}
	if err := srcCtx.db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("StreamExport: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	dstCtx := createGUIDIsolatedContext(t, t.Name()+"-dst")
	jobID := "share-optin-job"
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := dstCtx.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "merge"
	if _, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSinkAltFS{}); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	var reimported models.NoteType
	if err := dstCtx.db.Where("name = ?", "ShareOptInType").First(&reimported).Error; err != nil {
		t.Fatalf("load reimported note type: %v", err)
	}
	if !reimported.ApplyTemplatesToShares {
		t.Error("ApplyTemplatesToShares was dropped through export/import")
	}
	if reimported.CustomHeader == "" {
		t.Error("CustomHeader was dropped through export/import")
	}
}
