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
	// createTestContext uses file::memory:?cache=shared — all contexts share the same SQLite DB,
	// so the source resource's GUID and hash are both visible in dstCtx.
	//
	// GUIDCollisionPolicy: all entities (groups, notes, resources) have GUIDs.
	// "replace" updates existing rows with incoming data, keeping the same DB row.
	// Groups and notes fire GUID collision and are replaced in-place.
	// The resource is also replaced in-place, so CreatedResources stays 0 but the
	// resource row is updated and its blob is re-written.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"

	// Apply
	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// With GUIDCollisionPolicy=replace, all entities are replaced in-place (not newly created).
	// CreatedResources stays 0 since no new rows are inserted.
	if result.CreatedResources != 0 {
		t.Errorf("expected 0 created resources (replaced in-place), got %d", result.CreatedResources)
	}

	// Verify groups exist in destination (2 source groups; GUID replace means no new rows)
	var groups []models.Group
	dstCtx.db.Find(&groups)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups in DB (source only, GUID replace), got %d", len(groups))
	}

	// Verify resource still exists (1 row: replaced in-place)
	var resources []models.Resource
	dstCtx.db.Find(&resources)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource in DB (source replaced in-place), got %d", len(resources))
	}
	// The existing resource row should have blob on disk
	existing := resources[0]
	if existing.Location == "" {
		t.Fatal("resource Location is empty")
	}
	blobFile, err := dstCtx.fs.Open(existing.Location)
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

	// Verify note exists in the shared DB (created as part of srcCtx, GUID-skipped on import)
	var notes []models.Note
	dstCtx.db.Find(&notes)
	foundNote := false
	for _, n := range notes {
		if n.Name == "My Note" {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatal("note 'My Note' not found in DB")
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

	// Apply with "skip" collision policy. With GUID collision taking precedence over
	// hash collision, GUIDCollisionPolicy=skip ensures the resource is skipped via
	// the GUID path (no new row, no merge). ResourceCollisionPolicy=skip handles
	// resources that have hashes but no GUID in the archive.
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"
	decisions.GUIDCollisionPolicy = "skip"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Resource should be skipped (either via GUID collision or hash collision).
	// CreatedResources must be 0 and the total count must not change.
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

	// GUIDCollisionPolicy=replace: the resource exists in the shared DB with the same GUID,
	// so it is replaced in-place. Existing versions/previews are deleted and incoming ones created.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// CreatedResources is 0 (replaced in-place), but versions should be created.
	if result.CreatedVersions < 1 {
		t.Errorf("expected at least 1 created version, got %d", result.CreatedVersions)
	}

	// Find the replaced resource via result.CreatedResourceIDs (tracked by replaceResource).
	if len(result.CreatedResourceIDs) == 0 {
		t.Fatal("expected at least one entry in CreatedResourceIDs (replaced resource)")
	}
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

	// GUIDCollisionPolicy=replace: the resource exists in the shared DB with the same GUID,
	// so it is replaced in-place. Existing previews are deleted and incoming ones created.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// CreatedResources is 0 (replaced in-place), but previews should be created.
	if result.CreatedPreviews < 1 {
		t.Errorf("expected at least 1 created preview, got %d", result.CreatedPreviews)
	}

	// Find the replaced resource via result.CreatedResourceIDs (tracked by replaceResource).
	if len(result.CreatedResourceIDs) == 0 {
		t.Fatal("expected at least one entry in CreatedResourceIDs (replaced resource)")
	}
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

	// GUIDCollisionPolicy=replace: the resource exists in the shared DB with the same GUID,
	// so it is replaced in-place. The incoming SeriesRef should be wired.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Shared DB: the series slug already exists, so it should be reused.
	if result.ReusedSeries < 1 {
		t.Errorf("expected ReusedSeries >= 1, got %d", result.ReusedSeries)
	}

	// The resource was replaced in-place; find it via CreatedResourceIDs.
	if len(result.CreatedResourceIDs) == 0 {
		t.Fatal("expected at least one entry in CreatedResourceIDs (replaced resource)")
	}
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
	// Build an archive manually that contains a NoteTypeDef with NO GUID.
	// Without a GUID, the resolver falls back to name-based lookup, which can
	// find multiple matches and trigger the Ambiguous flag.
	ctx := createTestContext(t)

	// Create TWO NoteTypes with the same name "AmbigDiary" to seed ambiguity.
	// NoteType name is not uniquely indexed, so duplicates are allowed.
	nt := models.NoteType{Name: "AmbigDiary"}
	if err := ctx.db.Create(&nt).Error; err != nil {
		t.Fatal(err)
	}
	nt2 := models.NoteType{Name: "AmbigDiary"}
	if err := ctx.db.Create(&nt2).Error; err != nil {
		t.Fatal(err)
	}

	root := mustCreateGroup(t, ctx, "DiaryRoot", nil)
	note := mustCreateNote(t, ctx, "My Diary Entry", &root.ID)
	if err := ctx.db.Model(note).Update("note_type_id", nt.ID).Error; err != nil {
		t.Fatal(err)
	}

	// Build the archive manually so the NoteTypeDef has no GUID.
	// This ensures name-based resolution is used and triggers ambiguity.
	var tarBuf bytes.Buffer
	w, err := archive.NewWriter(&tarBuf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// Write manifest first (required by archive format).
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1, Notes: 1},
		ExportOptions: archive.ExportOptions{
			SchemaDefs: archive.ExportSchemaDefs{CategoriesAndTypes: true},
		},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: root.Name, SourceID: root.ID, Path: "groups/g0001.json"},
			},
			Notes: []archive.NoteEntry{
				{ExportID: "n0001", Name: note.Name, SourceID: note.ID, Owner: "g0001", Path: "notes/n0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Write a NoteTypeDef without a GUID so name-based resolution is used.
	noteTypeDef := archive.NoteTypeDef{
		ExportID: "nt0001",
		SourceID: nt.ID,
		// GUID intentionally omitted to force name-based resolution.
		Name: "AmbigDiary",
	}
	if err := w.WriteNoteTypeDefs([]archive.NoteTypeDef{noteTypeDef}); err != nil {
		t.Fatalf("WriteNoteTypeDefs: %v", err)
	}

	// Write a minimal group payload.
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  root.ID,
		Name:      root.Name,
		Tags:      []archive.TagRef{},
		Meta:      map[string]any{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}

	// Write a note that references the NoteType def.
	if err := w.WriteNote(&archive.NotePayload{
		ExportID:    "n0001",
		SourceID:    note.ID,
		Name:        note.Name,
		OwnerRef:    "g0001",
		NoteTypeRef: "nt0001",
		Tags:        []archive.TagRef{},
		Meta:        map[string]any{},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	jobID := "test-ambiguous-nt-apply"
	tarPath := filepath.Join("_imports", jobID+".tar")
	ctx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(ctx.fs, tarPath, tarBytes, 0644)

	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
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
		t.Fatal("expected an ambiguous NoteType mapping for 'AmbigDiary'")
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

	result, err := ctx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Find the imported note and verify its NoteTypeId.
	if len(result.CreatedNoteIDs) == 0 {
		t.Fatal("no notes were created")
	}
	var importedNote models.Note
	if err := ctx.db.First(&importedNote, result.CreatedNoteIDs[0]).Error; err != nil {
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

func TestApplyImport_ShellGroupCreate(t *testing.T) {
	// GroupA (root) has a RelatedResource owned by GroupB.
	// Export GroupA with RelatedDepth=1 => GroupB becomes a shell group.
	// Import with default decisions => shell group is created.
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	res := mustCreateResource(t, srcCtx, "external.txt", &groupB.ID, []byte("EXT"))
	mustLinkRelatedResource(t, srcCtx, groupA.ID, res.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		RelatedDepth: 1,
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	dstCtx := createTestContext(t)

	jobID := "test-shell-create"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// GUIDCollisionPolicy=replace: GroupA, GroupB, and the resource all exist in the shared DB.
	// "replace" updates them in-place (no new rows) while preserving idMap wiring so the
	// replaced resource remains owned by GroupB.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// With GUID replace, no new groups are created — the existing GroupA and GroupB are replaced in-place.
	if result.CreatedGroups != 0 {
		t.Errorf("expected CreatedGroups=0 (GUID replace), got %d", result.CreatedGroups)
	}

	// Verify the resource (replaced in-place) is owned by GroupB
	var importedRes models.Resource
	if err := dstCtx.db.Where("name = ?", "external.txt").First(&importedRes).Error; err != nil {
		t.Fatalf("find resource: %v", err)
	}
	if importedRes.OwnerId == nil {
		t.Fatal("resource OwnerId is nil")
	}
	var ownerGroup models.Group
	dstCtx.db.Where("id = ?", *importedRes.OwnerId).First(&ownerGroup)
	if ownerGroup.Name != "GroupB" {
		t.Errorf("resource owner name = %q, want %q", ownerGroup.Name, "GroupB")
	}
}

func TestApplyImport_ShellGroupMapToExisting(t *testing.T) {
	// GroupA (root) has a RelatedResource owned by GroupB.
	// Export GroupA with RelatedDepth=1 => GroupB becomes a shell group.
	// Map the shell to an existing targetGroup in the destination.
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	res := mustCreateResource(t, srcCtx, "external.txt", &groupB.ID, []byte("MAPEXT"))
	mustLinkRelatedResource(t, srcCtx, groupA.ID, res.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{OwnedResources: true, RelatedM2M: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		RelatedDepth: 1,
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	dstCtx := createTestContext(t)

	// Create a target group to map the shell to
	targetGroup := mustCreateGroup(t, dstCtx, "TargetGroup", nil)

	jobID := "test-shell-map"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Find the shell group export ID in the plan
	var shellExportID string
	var findShell func(items []ImportPlanItem)
	findShell = func(items []ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				shellExportID = item.ExportID
				return
			}
			findShell(item.Children)
		}
	}
	findShell(plan.Items)
	if shellExportID == "" {
		t.Fatal("no shell group found in plan")
	}

	// GUIDCollisionPolicy=replace: the resource exists in the shared DB with the same GUID.
	// "replace" updates it in-place and tracks it in CreatedResourceIDs.
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"
	decisions.ShellGroupActions[shellExportID] = ShellGroupAction{
		Action:        "map_to_existing",
		DestinationID: &targetGroup.ID,
	}

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if result.MappedShellGroups != 1 {
		t.Errorf("expected MappedShellGroups=1, got %d", result.MappedShellGroups)
	}
	if result.CreatedShellGroups != 0 {
		t.Errorf("expected CreatedShellGroups=0 (mapped, not created), got %d", result.CreatedShellGroups)
	}

	// Verify the imported resource's owner is the target group
	if len(result.CreatedResourceIDs) == 0 {
		t.Fatal("no resources created")
	}
	var importedRes models.Resource
	if err := dstCtx.db.First(&importedRes, result.CreatedResourceIDs[0]).Error; err != nil {
		t.Fatalf("load imported resource: %v", err)
	}
	if importedRes.OwnerId == nil {
		t.Fatal("imported resource OwnerId is nil")
	}
	if *importedRes.OwnerId != targetGroup.ID {
		t.Errorf("imported resource OwnerId=%d, want %d (targetGroup)", *importedRes.OwnerId, targetGroup.ID)
	}
}

func TestApplyImport_ShellGroupMap_DuplicateGroupRelation(t *testing.T) {
	// A -> (related group) -> B, B -> (typed relation) -> C
	// Export A with depth 2 and GroupRelations scope so B and C are both shells
	// and the B->C typed relation is in-scope (non-dangling).
	// Then map both shells to pre-existing targets that already have the same
	// typed relation. Apply must succeed (not fail on duplicate).
	srcCtx := createTestContext(t)

	groupA := mustCreateGroup(t, srcCtx, "GroupA", nil)
	groupB := mustCreateGroup(t, srcCtx, "GroupB", nil)
	groupC := mustCreateGroup(t, srcCtx, "GroupC", nil)
	mustLinkRelatedGroup(t, srcCtx, groupA.ID, groupB.ID)

	grt := mustCreateGroupRelationType(t, srcCtx, "TestRelType")
	mustCreateGroupRelation(t, srcCtx, groupB.ID, groupC.ID, grt.ID)

	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{groupA.ID},
		Scope:        archive.ExportScope{RelatedM2M: true, GroupRelations: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{GroupRelationTypes: true},
		RelatedDepth: 2,
	}, &tarBuf, func(ev ProgressEvent) {})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	tarBytes := tarBuf.Bytes()

	// Destination: create targetB, targetC, same GRT, and pre-existing relation
	dstCtx := createTestContext(t)
	targetB := mustCreateGroup(t, dstCtx, "TargetB", nil)
	targetC := mustCreateGroup(t, dstCtx, "TargetC", nil)
	// GRT already exists in shared DB, so reuse grt.ID
	mustCreateGroupRelation(t, dstCtx, targetB.ID, targetC.ID, grt.ID)

	jobID := "test-shell-dup-rel"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644)

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Find shell group export IDs and map them to target groups
	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"

	shellToTarget := map[string]uint{} // name -> target ID
	var findShells func(items []ImportPlanItem)
	findShells = func(items []ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				if item.Name == "GroupB" {
					shellToTarget[item.ExportID] = targetB.ID
				} else if item.Name == "GroupC" {
					shellToTarget[item.ExportID] = targetC.ID
				}
			}
			findShells(item.Children)
		}
	}
	findShells(plan.Items)

	if len(shellToTarget) != 2 {
		t.Fatalf("expected 2 shell groups, found %d", len(shellToTarget))
	}

	for exportID, targetID := range shellToTarget {
		id := targetID // capture
		decisions.ShellGroupActions[exportID] = ShellGroupAction{
			Action:        "map_to_existing",
			DestinationID: &id,
		}
	}

	// Apply must succeed despite the duplicate GroupRelation
	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply should succeed despite duplicate relation: %v", err)
	}

	if result.MappedShellGroups != 2 {
		t.Errorf("expected MappedShellGroups=2, got %d", result.MappedShellGroups)
	}
}

// buildDefaultDecisions creates decisions that accept all plan suggestions.
func buildDefaultDecisions(plan *ImportPlan) *ImportDecisions {
	d := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          make(map[string]MappingAction),
		DanglingActions:         make(map[string]DanglingAction),
		ShellGroupActions:       make(map[string]ShellGroupAction),
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
