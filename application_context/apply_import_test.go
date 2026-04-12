package application_context

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/download_queue"
	"mahresources/models"
)

// noopSink satisfies download_queue.ProgressSink for tests.
type noopSink struct{}

func (noopSink) SetPhase(string)              {}
func (noopSink) SetPhaseProgress(int64, int64) {}
func (noopSink) UpdateProgress(int64, int64)   {}
func (noopSink) AppendWarning(string)          {}
func (noopSink) SetResultPath(string)          {}

// Compile-time check that noopSink implements ProgressSink.
var _ download_queue.ProgressSink = noopSink{}

func TestApplyImport_FullRoundTrip(t *testing.T) {
	// --- Source instance: seed data ---
	srcCtx := createTestContext(t)

	cat := &models.Category{Name: "Books", Description: "Book category"}
	if err := srcCtx.db.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	tag := &models.Tag{Name: "fiction", Description: "Fiction tag"}
	if err := srcCtx.db.Create(tag).Error; err != nil {
		t.Fatal(err)
	}

	root := mustCreateGroup(t, srcCtx, "Root", nil)
	srcCtx.db.Model(root).Update("category_id", cat.ID)
	srcCtx.db.Model(root).Association("Tags").Append(tag)

	child := mustCreateGroup(t, srcCtx, "Child", &root.ID)

	content := []byte("HELLO WORLD BLOB")
	res := mustCreateResource(t, srcCtx, "hello.txt", &child.ID, content)
	srcCtx.db.Model(res).Association("Tags").Append(tag)

	mustCreateNote(t, srcCtx, "My Note", &child.ID)

	// --- Export ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true, OwnedNotes: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()
	if len(tarBytes) == 0 {
		t.Fatal("export produced empty tar")
	}

	// --- Destination instance ---
	dstCtx := createTestContext(t)

	// Stage the tar file for import
	jobID := "test-apply-001"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	// Parse
	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if plan.Counts.Groups != 2 {
		t.Fatalf("expected 2 groups, got %d", plan.Counts.Groups)
	}

	// Build default decisions (accept all suggestions)
	decisions := buildDefaultDecisions(plan)

	// Apply
	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// --- Verify ---
	if result.CreatedGroups != 2 {
		t.Errorf("expected 2 created groups, got %d", result.CreatedGroups)
	}
	if result.CreatedResources != 1 {
		t.Errorf("expected 1 created resource, got %d", result.CreatedResources)
	}
	if result.CreatedNotes != 1 {
		t.Errorf("expected 1 created note, got %d", result.CreatedNotes)
	}

	// Verify groups exist in destination
	var groups []models.Group
	dstCtx.db.Find(&groups)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups in DB, got %d", len(groups))
	}

	// Verify resource blob on disk
	var resources []models.Resource
	dstCtx.db.Find(&resources)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource in DB, got %d", len(resources))
	}
	r := resources[0]
	if r.Location == "" {
		t.Fatal("resource Location is empty")
	}
	blobFile, err := dstCtx.fs.Open(r.Location)
	if err != nil {
		t.Fatalf("open blob: %v", err)
	}
	blobData, _ := io.ReadAll(blobFile)
	blobFile.Close()
	if !bytes.Equal(blobData, content) {
		t.Errorf("blob content mismatch: got %q, want %q", blobData, content)
	}

	// Verify tag was created on destination and associated
	var destTags []models.Tag
	dstCtx.db.Find(&destTags)
	if len(destTags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(destTags))
	}
	if destTags[0].Name != "fiction" {
		t.Errorf("tag name = %q, want %q", destTags[0].Name, "fiction")
	}

	// Verify category was created on destination
	var destCats []models.Category
	dstCtx.db.Find(&destCats)
	// Destination starts with a default category (ID=1), plus our imported one
	found := false
	for _, c := range destCats {
		if c.Name == "Books" {
			found = true
		}
	}
	if !found {
		t.Error("imported category 'Books' not found")
	}

	// Verify note exists
	var notes []models.Note
	dstCtx.db.Find(&notes)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Name != "My Note" {
		t.Errorf("note name = %q, want %q", notes[0].Name, "My Note")
	}
}

// buildDefaultDecisions creates decisions that accept all plan suggestions.
func buildDefaultDecisions(plan *ImportPlan) *ImportDecisions {
	d := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          make(map[string]MappingAction),
		DanglingActions:         make(map[string]DanglingAction),
	}
	allMappings := [][]MappingEntry{
		plan.Mappings.Categories,
		plan.Mappings.NoteTypes,
		plan.Mappings.ResourceCategories,
		plan.Mappings.Tags,
		plan.Mappings.GroupRelationTypes,
	}
	for _, group := range allMappings {
		for _, entry := range group {
			action := entry.Suggestion
			if action == "" {
				action = "create"
			}
			d.MappingActions[entry.DecisionKey] = MappingAction{
				Include:       true,
				Action:        action,
				DestinationID: entry.DestinationID,
			}
		}
	}
	for _, dr := range plan.DanglingRefs {
		d.DanglingActions[dr.ID] = DanglingAction{Action: "drop"}
	}
	return d
}
