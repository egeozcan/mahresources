package application_context

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

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

func TestApplyImport_VersionHistoryRoundTrip(t *testing.T) {
	srcCtx := createTestContext(t)

	root := mustCreateGroup(t, srcCtx, "VerRoot", nil)
	content := []byte("VERSION_CONTENT")
	res := mustCreateResource(t, srcCtx, "versioned.txt", &root.ID, content)

	// Create a ResourceVersion row directly.
	ver := models.ResourceVersion{
		ResourceID:    res.ID,
		VersionNumber: 1,
		Hash:          res.Hash,
		HashType:      "SHA1",
		FileSize:      int64(len(content)),
		ContentType:   "text/plain",
		Location:      res.Location,
	}
	if err := srcCtx.db.Create(&ver).Error; err != nil {
		t.Fatal(err)
	}
	// Point the resource at this version.
	if err := srcCtx.db.Model(res).Update("current_version_id", ver.ID).Error; err != nil {
		t.Fatal(err)
	}

	// --- Export with version fidelity ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true, ResourceVersions: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// --- Destination ---
	dstCtx := createTestContext(t)

	jobID := "test-version-rt"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if result.CreatedResources != 1 {
		t.Fatalf("expected 1 created resource, got %d", result.CreatedResources)
	}
	if result.CreatedVersions < 1 {
		t.Errorf("expected at least 1 created version, got %d", result.CreatedVersions)
	}

	// Find imported resource and preload its Versions.
	importedResID := result.CreatedResourceIDs[0]
	var importedRes models.Resource
	if err := dstCtx.db.Preload("Versions").First(&importedRes, importedResID).Error; err != nil {
		t.Fatalf("load imported resource: %v", err)
	}
	if len(importedRes.Versions) == 0 {
		t.Fatal("imported resource has no ResourceVersion rows")
	}
	if importedRes.CurrentVersionID == nil {
		t.Fatal("imported resource CurrentVersionID is nil")
	}
}

func TestApplyImport_PreviewsRoundTrip(t *testing.T) {
	srcCtx := createTestContext(t)

	root := mustCreateGroup(t, srcCtx, "PrevRoot", nil)
	content := []byte("PREVIEW_RES_CONTENT")
	res := mustCreateResource(t, srcCtx, "withpreview.png", &root.ID, content)

	// Create a Preview row directly.
	preview := models.Preview{
		ResourceId:  &res.ID,
		Data:        []byte("PNG_PREVIEW_DATA"),
		Width:       100,
		Height:      80,
		ContentType: "image/png",
	}
	if err := srcCtx.db.Create(&preview).Error; err != nil {
		t.Fatal(err)
	}

	// --- Export with preview fidelity ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true, ResourcePreviews: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// --- Destination ---
	dstCtx := createTestContext(t)

	jobID := "test-preview-rt"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if result.CreatedResources != 1 {
		t.Fatalf("expected 1 created resource, got %d", result.CreatedResources)
	}
	if result.CreatedPreviews < 1 {
		t.Errorf("expected at least 1 created preview, got %d", result.CreatedPreviews)
	}

	// Find imported resource's previews.
	importedResID := result.CreatedResourceIDs[0]
	var previews []models.Preview
	if err := dstCtx.db.Where("resource_id = ?", importedResID).Find(&previews).Error; err != nil {
		t.Fatalf("query previews: %v", err)
	}
	if len(previews) == 0 {
		t.Fatal("imported resource has no preview rows")
	}
	p := previews[0]
	if p.Width != 100 {
		t.Errorf("preview Width = %d, want 100", p.Width)
	}
	if p.Height != 80 {
		t.Errorf("preview Height = %d, want 80", p.Height)
	}
	if p.ContentType != "image/png" {
		t.Errorf("preview ContentType = %q, want image/png", p.ContentType)
	}
	if len(p.Data) == 0 {
		t.Error("preview Data is empty")
	}
}

func TestApplyImport_SeriesSlugPreserved(t *testing.T) {
	srcCtx := createTestContext(t)

	// Create a Series.
	series := models.Series{Name: "Volumes", Slug: "test-volumes-slug"}
	if err := srcCtx.db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}

	root := mustCreateGroup(t, srcCtx, "SerRoot", nil)
	content := []byte("SERIES_RES")
	res := mustCreateResource(t, srcCtx, "seriesfile.txt", &root.ID, content)

	// Assign resource to series.
	if err := srcCtx.db.Model(res).Update("series_id", series.ID).Error; err != nil {
		t.Fatal(err)
	}

	// --- Export with series fidelity ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true, ResourceSeries: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// --- Destination (shared DB: series already exists by slug) ---
	dstCtx := createTestContext(t)

	jobID := "test-series-slug-rt"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "duplicate"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Shared DB: the series slug already exists, so it should be reused.
	if result.ReusedSeries < 1 {
		t.Errorf("expected ReusedSeries >= 1, got %d", result.ReusedSeries)
	}

	// Find the imported resource and verify its SeriesID.
	importedResID := result.CreatedResourceIDs[0]
	var importedRes models.Resource
	if err := dstCtx.db.First(&importedRes, importedResID).Error; err != nil {
		t.Fatalf("load imported resource: %v", err)
	}
	if importedRes.SeriesID == nil {
		t.Fatal("imported resource SeriesID is nil")
	}
	if *importedRes.SeriesID != series.ID {
		t.Errorf("imported resource SeriesID = %d, want %d (original)", *importedRes.SeriesID, series.ID)
	}
}

func TestApplyImport_AmbiguousNoteTypeRequiresDecision(t *testing.T) {
	srcCtx := createTestContext(t)

	// Create a NoteType "ApplyTestDiary" and a note using it.
	nt := models.NoteType{Name: "ApplyTestDiary"}
	if err := srcCtx.db.Create(&nt).Error; err != nil {
		t.Fatal(err)
	}
	root := mustCreateGroup(t, srcCtx, "DiaryRoot", nil)
	note := mustCreateNote(t, srcCtx, "My Diary Entry", &root.ID)
	if err := srcCtx.db.Model(note).Update("note_type_id", nt.ID).Error; err != nil {
		t.Fatal(err)
	}

	// --- Export with schema defs ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// In shared DB, create a SECOND NoteType "ApplyTestDiary" to trigger ambiguity.
	nt2 := models.NoteType{Name: "ApplyTestDiary"}
	if err := srcCtx.db.Create(&nt2).Error; err != nil {
		t.Fatal(err)
	}

	// --- Destination ---
	dstCtx := createTestContext(t)

	jobID := "test-ambiguous-nt-apply"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Find the ambiguous NoteType mapping.
	var ambiguousEntry *MappingEntry
	for i := range plan.Mappings.NoteTypes {
		if plan.Mappings.NoteTypes[i].Ambiguous {
			ambiguousEntry = &plan.Mappings.NoteTypes[i]
			break
		}
	}
	if ambiguousEntry == nil {
		t.Fatal("expected an ambiguous NoteType mapping for 'Diary'")
	}

	// Build decisions but manually set the ambiguous entry's action to "" to
	// simulate an incomplete review.
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"
	decisions.MappingActions[ambiguousEntry.DecisionKey] = MappingAction{
		Include: true,
		Action:  "",
	}

	// ValidateForApply should reject because the ambiguous entry has no action.
	if err := plan.ValidateForApply(decisions); err == nil {
		t.Fatal("expected ValidateForApply to reject decisions with empty action on ambiguous entry")
	}

	// Fix: set the ambiguous entry's action to "map" pointing to the first NoteType.
	decisions.MappingActions[ambiguousEntry.DecisionKey] = MappingAction{
		Include:       true,
		Action:        "map",
		DestinationID: &nt.ID,
	}

	// Now validation should pass.
	if err := plan.ValidateForApply(decisions); err != nil {
		t.Fatalf("expected ValidateForApply to pass after fix, got: %v", err)
	}

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Find the imported note and verify its NoteTypeId.
	if len(result.CreatedNoteIDs) == 0 {
		t.Fatal("no notes were created")
	}
	var importedNote models.Note
	if err := dstCtx.db.First(&importedNote, result.CreatedNoteIDs[0]).Error; err != nil {
		t.Fatalf("load imported note: %v", err)
	}
	if importedNote.NoteTypeId == nil {
		t.Fatal("imported note NoteTypeId is nil")
	}
	if *importedNote.NoteTypeId != nt.ID {
		t.Errorf("imported note NoteTypeId = %d, want %d", *importedNote.NoteTypeId, nt.ID)
	}
}

func TestApplyImport_SchemaDefsOffCreatesMinimal(t *testing.T) {
	// Build a tar manually with schema defs toggled OFF. The group payload
	// references a category name that does NOT exist in the DB, so ParseImport
	// synthesizes a mapping with HasPayload: false and Suggestion: "create".
	// ApplyImport should create a minimal category with only Name set.
	ctx := createTestContext(t)

	catName := "MinimalTestCat_NoPayload"

	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	err = w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1},
		ExportOptions: archive.ExportOptions{
			SchemaDefs: archive.ExportSchemaDefs{
				CategoriesAndTypes: false,
				Tags:               false,
				GroupRelationTypes: false,
			},
		},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "MinGroup", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	err = w.WriteGroup(&archive.GroupPayload{
		ExportID:     "g0001",
		SourceID:     1,
		Name:         "MinGroup",
		CategoryName: catName, // no CategoryRef (schema defs off)
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	jobID := "test-schemadefs-off-apply"
	tarPath := filepath.Join("_imports", jobID+".tar")
	ctx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(ctx.fs, tarPath, buf.Bytes(), 0644)

	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Verify the synthesized category mapping.
	if len(plan.Mappings.Categories) == 0 {
		t.Fatal("expected at least 1 category mapping")
	}
	var targetEntry *MappingEntry
	for i := range plan.Mappings.Categories {
		if plan.Mappings.Categories[i].SourceKey == catName {
			targetEntry = &plan.Mappings.Categories[i]
			break
		}
	}
	if targetEntry == nil {
		t.Fatalf("no category mapping for %s, got: %+v", catName, plan.Mappings.Categories)
	}
	if targetEntry.HasPayload {
		t.Errorf("HasPayload = true, want false (schema defs were off)")
	}
	// No match in DB => suggestion should be "create".
	if targetEntry.Suggestion != "create" {
		t.Errorf("Suggestion = %q, want 'create'", targetEntry.Suggestion)
	}

	// Accept all defaults (which already include "create" for this entry).
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"

	// Count categories before.
	var catCountBefore int64
	ctx.db.Model(&models.Category{}).Count(&catCountBefore)

	result, err := ctx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if result.CreatedCategories < 1 {
		t.Errorf("expected CreatedCategories >= 1, got %d", result.CreatedCategories)
	}

	// Verify category count increased.
	var catCountAfter int64
	ctx.db.Model(&models.Category{}).Count(&catCountAfter)
	if catCountAfter <= catCountBefore {
		t.Errorf("category count did not increase: before=%d, after=%d", catCountBefore, catCountAfter)
	}

	// The newly created category should have only Name set (minimal: no Description,
	// no CustomHeader, etc.) because there was no payload to populate from.
	var created models.Category
	if err := ctx.db.Where("name = ?", catName).First(&created).Error; err != nil {
		t.Fatalf("find created category: %v", err)
	}
	if created.Description != "" {
		t.Errorf("expected empty Description on minimal category, got %q", created.Description)
	}
	if created.CustomHeader != "" {
		t.Errorf("expected empty CustomHeader on minimal category, got %q", created.CustomHeader)
	}

	// Verify the imported group points to the new category.
	var importedGroups []models.Group
	ctx.db.Where("name = ?", "MinGroup").Find(&importedGroups)
	foundWithCat := false
	for _, g := range importedGroups {
		if g.CategoryId != nil && *g.CategoryId == created.ID {
			foundWithCat = true
		}
	}
	if !foundWithCat {
		t.Error("imported group does not point to the newly created minimal category")
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
