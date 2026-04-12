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

	// Build default decisions (accept all suggestions).
	// Use "duplicate" collision policy because createTestContext uses
	// file::memory:?cache=shared — all contexts share the same SQLite DB,
	// so the source resource's hash is visible in dstCtx. "duplicate" forces
	// a new resource row even when the hash already exists.
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"

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

	// Verify groups exist in destination (2 source + 2 imported in shared DB)
	var groups []models.Group
	dstCtx.db.Find(&groups)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups in DB (2 source + 2 imported), got %d", len(groups))
	}

	// Verify resource blob on disk (2 total: source + imported duplicate)
	var resources []models.Resource
	dstCtx.db.Find(&resources)
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources in DB (source + imported), got %d", len(resources))
	}
	// Find the imported resource (the one created by apply)
	var imported *models.Resource
	for i := range resources {
		for _, createdID := range result.CreatedResourceIDs {
			if resources[i].ID == createdID {
				imported = &resources[i]
				break
			}
		}
		if imported != nil {
			break
		}
	}
	if imported == nil {
		t.Fatal("could not find imported resource among created IDs")
	}
	if imported.Location == "" {
		t.Fatal("imported resource Location is empty")
	}
	blobFile, err := dstCtx.fs.Open(imported.Location)
	if err != nil {
		t.Fatalf("open blob: %v", err)
	}
	blobData, _ := io.ReadAll(blobFile)
	blobFile.Close()
	if !bytes.Equal(blobData, content) {
		t.Errorf("blob content mismatch: got %q, want %q", blobData, content)
	}

	// Verify tag exists and is named correctly
	var destTags []models.Tag
	dstCtx.db.Find(&destTags)
	foundTag := false
	for _, tag := range destTags {
		if tag.Name == "fiction" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Error("imported tag 'fiction' not found")
	}

	// Verify category was created on destination
	var destCats []models.Category
	dstCtx.db.Find(&destCats)
	found := false
	for _, c := range destCats {
		if c.Name == "Books" {
			found = true
		}
	}
	if !found {
		t.Error("imported category 'Books' not found")
	}

	// Verify note exists (2 total: source + imported in shared DB)
	var notes []models.Note
	dstCtx.db.Find(&notes)
	foundNote := false
	for _, n := range notes {
		if n.Name == "My Note" {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatal("imported note 'My Note' not found")
	}
}

func TestApplyImport_ResourceCollisionSkip(t *testing.T) {
	// Shared-DB: srcCtx and dstCtx use the same SQLite database.
	// Export a resource from srcCtx, then import with "skip" policy.
	// The resource hash already exists in the shared DB, so ApplyImport
	// should skip it rather than creating a duplicate.
	srcCtx := createTestContext(t)

	content := []byte("SHARED CONTENT")
	root := mustCreateGroup(t, srcCtx, "SkipRoot", nil)
	mustCreateResource(t, srcCtx, "shared.txt", &root.ID, content)

	// --- Export ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// --- Destination (shares DB with source) ---
	dstCtx := createTestContext(t)

	// Count resources before import (includes source resource due to shared DB)
	var countBefore int64
	dstCtx.db.Model(&models.Resource{}).Count(&countBefore)

	// Stage the tar for import
	jobID := "test-collision-skip"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	// Parse
	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Apply with "skip" collision policy (the default from buildDefaultDecisions)
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Resource should be skipped because its hash already exists in the shared DB
	if result.SkippedByHash < 1 {
		t.Errorf("expected SkippedByHash >= 1, got %d", result.SkippedByHash)
	}
	if result.CreatedResources != 0 {
		t.Errorf("expected CreatedResources == 0, got %d", result.CreatedResources)
	}

	// Verify no new resources were added
	var countAfter int64
	dstCtx.db.Model(&models.Resource{}).Count(&countAfter)
	if countAfter != countBefore {
		t.Errorf("resource count changed: before=%d, after=%d; expected no change", countBefore, countAfter)
	}
}

func TestValidateForApply_MissingHashAcknowledgement(t *testing.T) {
	// Unit test for the ValidateForApply gate on ManifestOnlyMissingHashes.
	// When the plan reports missing hashes and the user hasn't acknowledged,
	// validation must reject. When acknowledged, it must pass.
	plan := &ImportPlan{
		ManifestOnlyMissingHashes: 5,
	}
	decisions := &ImportDecisions{
		AcknowledgeMissingHashes: false,
		MappingActions:           make(map[string]MappingAction),
		DanglingActions:          make(map[string]DanglingAction),
	}

	err := plan.ValidateForApply(decisions)
	if err == nil {
		t.Fatal("expected error when AcknowledgeMissingHashes is false and plan has missing hashes")
	}

	// Now acknowledge
	decisions.AcknowledgeMissingHashes = true
	err = plan.ValidateForApply(decisions)
	if err != nil {
		t.Fatalf("expected no error when AcknowledgeMissingHashes is true, got: %v", err)
	}
}

func TestApplyImport_SchemaDefsMapToExisting(t *testing.T) {
	// When the source archive includes a category that already exists in the
	// destination (shared DB), ParseImport should suggest "map" and ApplyImport
	// should reuse the existing category rather than creating a new one.
	srcCtx := createTestContext(t)

	// Create a category and a group that uses it
	cat := &models.Category{Name: "SharedCat", Description: "Shared category"}
	if err := srcCtx.db.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	root := mustCreateGroup(t, srcCtx, "CatRoot", nil)
	srcCtx.db.Model(root).Update("category_id", cat.ID)

	// --- Export with schema defs ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// --- Destination (shares DB -- "SharedCat" already exists) ---
	dstCtx := createTestContext(t)

	// Count categories before import
	var catCountBefore int64
	dstCtx.db.Model(&models.Category{}).Count(&catCountBefore)

	// Stage tar
	jobID := "test-schema-map"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	// Parse -- should suggest "map" for SharedCat
	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Verify the category mapping suggests "map"
	foundCatMapping := false
	for _, m := range plan.Mappings.Categories {
		if m.DestinationName == "SharedCat" && m.Suggestion == "map" {
			foundCatMapping = true
			if m.DestinationID == nil {
				t.Fatal("category mapping DestinationID is nil")
			}
			if *m.DestinationID != cat.ID {
				t.Errorf("category mapping DestinationID = %d, want %d", *m.DestinationID, cat.ID)
			}
		}
	}
	if !foundCatMapping {
		t.Fatalf("expected category mapping with suggestion 'map' for SharedCat, mappings: %+v", plan.Mappings.Categories)
	}

	// Apply with default decisions (which accept the "map" suggestion)
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// No new category should be created -- the existing one was reused
	if result.CreatedCategories != 0 {
		t.Errorf("expected CreatedCategories == 0 (mapped to existing), got %d", result.CreatedCategories)
	}

	// Category count should not have changed
	var catCountAfter int64
	dstCtx.db.Model(&models.Category{}).Count(&catCountAfter)
	if catCountAfter != catCountBefore {
		t.Errorf("category count changed: before=%d, after=%d; expected no change", catCountBefore, catCountAfter)
	}

	// Verify the imported group points to the existing category
	var importedGroups []models.Group
	dstCtx.db.Where("name = ?", "CatRoot").Find(&importedGroups)
	// Shared DB: there will be the original + the imported copy
	foundWithCat := false
	for _, g := range importedGroups {
		if g.CategoryId != nil && *g.CategoryId == cat.ID {
			foundWithCat = true
		}
	}
	if !foundWithCat {
		t.Error("no imported group points to the existing SharedCat category")
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
