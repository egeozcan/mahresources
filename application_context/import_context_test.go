package application_context

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

func buildTestImportTar(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// Write manifest
	err = w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts: archive.Counts{
			Groups:    2,
			Notes:     1,
			Resources: 1,
		},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "Root Group", SourceID: 1, Path: "groups/g0001.json"},
				{ExportID: "g0002", Name: "Child Group", SourceID: 2, Path: "groups/g0002.json"},
			},
			Notes: []archive.NoteEntry{
				{ExportID: "n0001", Name: "Test Note", SourceID: 1, Owner: "g0002", Path: "notes/n0001.json"},
			},
			Resources: []archive.ResourceEntry{
				{ExportID: "r0001", Name: "Test Resource", SourceID: 1, Owner: "g0002", Hash: "abc123", Path: "resources/r0001.json"},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Write category defs
	err = w.WriteCategoryDefs([]archive.CategoryDef{
		{ExportID: "c0001", SourceID: 1, Name: "TestCat", Description: "A test category"},
	})
	if err != nil {
		t.Fatalf("WriteCategoryDefs: %v", err)
	}

	// Write tag defs
	err = w.WriteTagDefs([]archive.TagDef{
		{ExportID: "t0001", SourceID: 1, Name: "TestTag", Description: "A test tag"},
	})
	if err != nil {
		t.Fatalf("WriteTagDefs: %v", err)
	}

	// Write groups
	err = w.WriteGroup(&archive.GroupPayload{
		ExportID:     "g0001",
		SourceID:     1,
		Name:         "Root Group",
		CategoryName: "TestCat",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("WriteGroup root: %v", err)
	}

	err = w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0002",
		SourceID:  2,
		Name:      "Child Group",
		OwnerRef:  "g0001",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("WriteGroup child: %v", err)
	}

	// Write note
	err = w.WriteNote(&archive.NotePayload{
		ExportID:  "n0001",
		SourceID:  1,
		Name:      "Test Note",
		OwnerRef:  "g0002",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	// Write resource
	err = w.WriteResource(&archive.ResourcePayload{
		ExportID:  "r0001",
		SourceID:  1,
		Name:      "Test Resource",
		Hash:      "abc123",
		HashType:  "SHA1",
		FileSize:  100,
		OwnerRef:  "g0002",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("WriteResource: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	return buf.Bytes()
}

func TestParseImport_BasicPlan(t *testing.T) {
	ctx := createTestContext(t)

	// Build and write the tar to the filesystem
	tarData := buildTestImportTar(t)
	tarPath := "_imports/test-job.tar"
	if err := ctx.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(ctx.fs, tarPath, tarData, 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	plan, err := ctx.ParseImport("test-job", tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}

	// Check basic plan fields
	if plan.JobID != "test-job" {
		t.Errorf("JobID = %q, want 'test-job'", plan.JobID)
	}
	if plan.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", plan.SchemaVersion)
	}

	// Check counts
	if plan.Counts.Groups != 2 {
		t.Errorf("Counts.Groups = %d, want 2", plan.Counts.Groups)
	}
	if plan.Counts.Notes != 1 {
		t.Errorf("Counts.Notes = %d, want 1", plan.Counts.Notes)
	}
	if plan.Counts.Resources != 1 {
		t.Errorf("Counts.Resources = %d, want 1", plan.Counts.Resources)
	}

	// Check tree structure: 1 root with 1 child
	if len(plan.Items) != 1 {
		t.Fatalf("Items (roots) = %d, want 1", len(plan.Items))
	}
	root := plan.Items[0]
	if root.ExportID != "g0001" {
		t.Errorf("root ExportID = %q, want 'g0001'", root.ExportID)
	}
	if root.Kind != "group" {
		t.Errorf("root Kind = %q, want 'group'", root.Kind)
	}
	if root.Name != "Root Group" {
		t.Errorf("root Name = %q, want 'Root Group'", root.Name)
	}
	if root.OwnerRef != "" {
		t.Errorf("root OwnerRef = %q, want ''", root.OwnerRef)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root.Children = %d, want 1", len(root.Children))
	}
	child := root.Children[0]
	if child.ExportID != "g0002" {
		t.Errorf("child ExportID = %q, want 'g0002'", child.ExportID)
	}
	if child.OwnerRef != "g0001" {
		t.Errorf("child OwnerRef = %q, want 'g0001'", child.OwnerRef)
	}

	// Check resource and note counts on child
	if child.ResourceCount != 1 {
		t.Errorf("child ResourceCount = %d, want 1", child.ResourceCount)
	}
	if child.NoteCount != 1 {
		t.Errorf("child NoteCount = %d, want 1", child.NoteCount)
	}

	// Check rolled-up descendant counts on root
	if root.DescendantResourceCount != 1 {
		t.Errorf("root DescendantResourceCount = %d, want 1", root.DescendantResourceCount)
	}
	if root.DescendantNoteCount != 1 {
		t.Errorf("root DescendantNoteCount = %d, want 1", root.DescendantNoteCount)
	}

	// Category mapping should suggest "create" since DB is empty
	if len(plan.Mappings.Categories) != 1 {
		t.Fatalf("Categories mappings = %d, want 1", len(plan.Mappings.Categories))
	}
	catMapping := plan.Mappings.Categories[0]
	if catMapping.Suggestion != "create" {
		t.Errorf("category suggestion = %q, want 'create'", catMapping.Suggestion)
	}
	if catMapping.DestinationID != nil {
		t.Errorf("category DestinationID = %v, want nil", catMapping.DestinationID)
	}

	// Tag mapping should suggest "create" since DB is empty
	if len(plan.Mappings.Tags) != 1 {
		t.Fatalf("Tags mappings = %d, want 1", len(plan.Mappings.Tags))
	}
	tagMapping := plan.Mappings.Tags[0]
	if tagMapping.Suggestion != "create" {
		t.Errorf("tag suggestion = %q, want 'create'", tagMapping.Suggestion)
	}

	// Verify plan was persisted
	loaded, err := ctx.LoadImportPlan("test-job")
	if err != nil {
		t.Fatalf("LoadImportPlan: %v", err)
	}
	if loaded.JobID != "test-job" {
		t.Errorf("loaded JobID = %q", loaded.JobID)
	}
	if loaded.Counts.Groups != 2 {
		t.Errorf("loaded Counts.Groups = %d", loaded.Counts.Groups)
	}
}

func TestParseImport_NameBasedMapping_ExistingCategory(t *testing.T) {
	ctx := createTestContext(t)

	// Seed a category with the same name as in the tar
	cat := models.Category{Name: "TestCat"}
	if err := ctx.db.Create(&cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	// Build and write the tar to the filesystem
	tarData := buildTestImportTar(t)
	tarPath := "_imports/test-job-cat.tar"
	if err := ctx.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(ctx.fs, tarPath, tarData, 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	plan, err := ctx.ParseImport("test-job-cat", tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}

	// Category mapping should suggest "map" with DestinationID pointing to existing category
	if len(plan.Mappings.Categories) != 1 {
		t.Fatalf("Categories mappings = %d, want 1", len(plan.Mappings.Categories))
	}
	catMapping := plan.Mappings.Categories[0]
	if catMapping.Suggestion != "map" {
		t.Errorf("category suggestion = %q, want 'map'", catMapping.Suggestion)
	}
	if catMapping.DestinationID == nil {
		t.Fatalf("category DestinationID is nil, want non-nil")
	}
	if *catMapping.DestinationID != cat.ID {
		t.Errorf("category DestinationID = %d, want %d", *catMapping.DestinationID, cat.ID)
	}
	if catMapping.DestinationName != "TestCat" {
		t.Errorf("category DestinationName = %q, want 'TestCat'", catMapping.DestinationName)
	}
}
