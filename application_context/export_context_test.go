package application_context

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

// exportCollector mirrors archive/reader_test.go's testCollector but lives
// here because the archive package's test helpers are internal.
type exportCollector struct {
	groups       map[string]*archive.GroupPayload
	notes        map[string]*archive.NotePayload
	resources    map[string]*archive.ResourcePayload
	series       map[string]*archive.SeriesPayload
	blobs        map[string][]byte
	previews     map[string][]byte
	categoryDefs []archive.CategoryDef
	tagDefs      []archive.TagDef
}

func newExportCollector() *exportCollector {
	return &exportCollector{
		groups:    map[string]*archive.GroupPayload{},
		notes:     map[string]*archive.NotePayload{},
		resources: map[string]*archive.ResourcePayload{},
		series:    map[string]*archive.SeriesPayload{},
		blobs:     map[string][]byte{},
		previews:  map[string][]byte{},
	}
}

func (c *exportCollector) OnGroup(p *archive.GroupPayload) error {
	c.groups[p.ExportID] = p
	return nil
}
func (c *exportCollector) OnNote(p *archive.NotePayload) error { c.notes[p.ExportID] = p; return nil }
func (c *exportCollector) OnResource(p *archive.ResourcePayload) error {
	c.resources[p.ExportID] = p
	return nil
}
func (c *exportCollector) OnSeries(p *archive.SeriesPayload) error {
	c.series[p.ExportID] = p
	return nil
}
func (c *exportCollector) OnBlob(hash string, body io.Reader, size int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	c.blobs[hash] = data
	return nil
}
func (c *exportCollector) OnPreview(id string, body io.Reader, size int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	c.previews[id] = data
	return nil
}
func (c *exportCollector) OnCategoryDefs(defs []archive.CategoryDef) error {
	c.categoryDefs = defs
	return nil
}
func (c *exportCollector) OnTagDefs(defs []archive.TagDef) error { c.tagDefs = defs; return nil }

func TestStreamExport_FullFidelityRoundTrip(t *testing.T) {
	ctx := createTestContext(t)

	root := mustCreateGroup(t, ctx, "Root", nil)
	child := mustCreateGroup(t, ctx, "Child", &root.ID)
	mustCreateResource(t, ctx, "img.png", &root.ID, []byte("PNGDATA"))
	mustCreateResource(t, ctx, "doc.pdf", &child.ID, []byte("PDFDATA"))
	mustCreateNote(t, ctx, "Hello", &root.ID)

	req := &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope: archive.ExportScope{
			Subtree: true, OwnedResources: true, OwnedNotes: true,
			RelatedM2M: true, GroupRelations: true,
		},
		Fidelity: archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs: archive.ExportSchemaDefs{
			CategoriesAndTypes: true, Tags: true, GroupRelationTypes: true,
		},
	}

	var buf bytes.Buffer
	report := func(ev ProgressEvent) {}
	if err := ctx.StreamExport(context.Background(), req, &buf, report); err != nil {
		t.Fatalf("StreamExport: %v", err)
	}

	r, err := archive.NewReader(&buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()
	m, err := r.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if m.Counts.Groups != 2 {
		t.Errorf("groups = %d", m.Counts.Groups)
	}
	if m.Counts.Resources != 2 {
		t.Errorf("resources = %d", m.Counts.Resources)
	}
	if m.Counts.Notes != 1 {
		t.Errorf("notes = %d", m.Counts.Notes)
	}
	if m.Counts.Blobs != 2 {
		t.Errorf("blobs = %d", m.Counts.Blobs)
	}

	c := newExportCollector()
	if err := r.Walk(c); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(c.groups) != 2 {
		t.Errorf("walked groups = %d", len(c.groups))
	}
	if len(c.resources) != 2 {
		t.Errorf("walked resources = %d", len(c.resources))
	}
	if len(c.notes) != 1 {
		t.Errorf("walked notes = %d", len(c.notes))
	}
	if len(c.blobs) != 2 {
		t.Errorf("walked blobs = %d", len(c.blobs))
	}
}

func TestStreamExport_BlobMissingRecordsWarning(t *testing.T) {
	ctx := createTestContext(t)

	root := mustCreateGroup(t, ctx, "Root", nil)
	r := mustCreateResource(t, ctx, "img.png", &root.ID, []byte("PNGDATA"))

	// Delete the file from the filesystem behind the resource's back.
	if err := ctx.fs.Remove(r.Location); err != nil {
		t.Fatalf("remove blob: %v", err)
	}

	req := &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
	}

	var buf bytes.Buffer
	// Capture reporter events so we can verify warnings flow through.
	var liveWarnings []string
	var sawBytes bool
	report := func(ev ProgressEvent) {
		if ev.Warning != "" {
			liveWarnings = append(liveWarnings, ev.Warning)
		}
		if ev.BytesWritten > 0 {
			sawBytes = true
		}
	}
	if err := ctx.StreamExport(context.Background(), req, &buf, report); err != nil {
		t.Fatalf("StreamExport: %v", err)
	}

	rdr, _ := archive.NewReader(&buf)
	defer rdr.Close()
	m, _ := rdr.ReadManifest()
	if len(m.Warnings) == 0 {
		t.Fatalf("expected at least one warning in manifest, got none")
	}
	if len(liveWarnings) == 0 {
		t.Fatalf("expected at least one warning via reporter, got none")
	}
	if !sawBytes {
		t.Fatalf("expected reporter to see BytesWritten > 0")
	}

	// C2: assert that the walked resource payload has BlobMissing == true and
	// an empty BlobRef (we must not promise a blob that isn't there).
	c := newExportCollector()
	if err := rdr.Walk(c); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(c.resources) != 1 {
		t.Fatalf("expected 1 resource in archive, got %d", len(c.resources))
	}
	for _, rp := range c.resources {
		if !rp.BlobMissing {
			t.Errorf("expected BlobMissing == true on resource payload, got false")
		}
		if rp.BlobRef != "" {
			t.Errorf("expected empty BlobRef when blob is missing, got %q", rp.BlobRef)
		}
	}
}

func TestEstimateExport_CountsGroupsResourcesNotes(t *testing.T) {
	ctx := createTestContext(t)

	root := mustCreateGroup(t, ctx, "Root", nil)
	child := mustCreateGroup(t, ctx, "Child", &root.ID)
	mustCreateResource(t, ctx, "img.jpg", &root.ID, []byte("PNGDATA"))
	mustCreateResource(t, ctx, "doc.pdf", &child.ID, []byte("PDFDATA"))
	mustCreateNote(t, ctx, "Note 1", &root.ID)

	req := ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope: archive.ExportScope{
			Subtree:        true,
			OwnedResources: true,
			OwnedNotes:     true,
			RelatedM2M:     true,
			GroupRelations: true,
		},
		Fidelity: archive.ExportFidelity{
			ResourceBlobs: true,
		},
		SchemaDefs: archive.ExportSchemaDefs{
			CategoriesAndTypes: true,
			Tags:               true,
			GroupRelationTypes: true,
		},
	}

	est, err := ctx.EstimateExport(&req)
	if err != nil {
		t.Fatalf("EstimateExport: %v", err)
	}
	if est.Counts.Groups != 2 {
		t.Errorf("groups = %d, want 2", est.Counts.Groups)
	}
	if est.Counts.Resources != 2 {
		t.Errorf("resources = %d, want 2", est.Counts.Resources)
	}
	if est.Counts.Notes != 1 {
		t.Errorf("notes = %d, want 1", est.Counts.Notes)
	}
}

func mustCreateGroup(t *testing.T, ctx *MahresourcesContext, name string, ownerID *uint) *models.Group {
	t.Helper()
	g := models.Group{Name: name, OwnerId: ownerID}
	if err := ctx.db.Create(&g).Error; err != nil {
		t.Fatalf("create group %q: %v", name, err)
	}
	t.Cleanup(func() {
		ctx.db.Unscoped().Delete(&models.Group{}, g.ID)
	})
	return &g
}

func mustCreateResource(t *testing.T, ctx *MahresourcesContext, name string, ownerID *uint, content []byte) *models.Resource {
	t.Helper()
	sum := sha1.Sum(content)
	hash := fmt.Sprintf("%x", sum)
	location := "/resources/" + hash
	if err := afero.WriteFile(ctx.fs, location, content, 0644); err != nil {
		t.Fatalf("write blob %q: %v", name, err)
	}
	r := models.Resource{
		Name:               name,
		OwnerId:            ownerID,
		Hash:               hash,
		HashType:           "SHA1",
		FileSize:           int64(len(content)),
		Location:           location,
		ResourceCategoryId: ctx.DefaultResourceCategoryID,
	}
	if err := ctx.db.Create(&r).Error; err != nil {
		t.Fatalf("create resource %q: %v", name, err)
	}
	t.Cleanup(func() {
		ctx.db.Unscoped().Delete(&models.Resource{}, r.ID)
		_ = ctx.fs.Remove(location)
	})
	return &r
}

func mustCreateNote(t *testing.T, ctx *MahresourcesContext, name string, ownerID *uint) *models.Note {
	t.Helper()
	n := models.Note{Name: name, OwnerId: ownerID}
	if err := ctx.db.Create(&n).Error; err != nil {
		t.Fatalf("create note %q: %v", name, err)
	}
	t.Cleanup(func() {
		ctx.db.Unscoped().Delete(&models.Note{}, n.ID)
	})
	return &n
}

func mustLinkRelatedGroup(t *testing.T, ctx *MahresourcesContext, fromID, toID uint) {
	t.Helper()
	var from, to models.Group
	if err := ctx.db.First(&from, fromID).Error; err != nil {
		t.Fatalf("load from group: %v", err)
	}
	if err := ctx.db.First(&to, toID).Error; err != nil {
		t.Fatalf("load to group: %v", err)
	}
	if err := ctx.db.Model(&from).Association("RelatedGroups").Append(&to); err != nil {
		t.Fatalf("append related: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Model(&from).Association("RelatedGroups").Delete(&to)
	})
}

func mustCreateGroupRelationType(t *testing.T, ctx *MahresourcesContext, name string) *models.GroupRelationType {
	t.Helper()
	rt := models.GroupRelationType{Name: name}
	if err := ctx.db.Create(&rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	t.Cleanup(func() {
		ctx.db.Unscoped().Delete(&models.GroupRelationType{}, rt.ID)
	})
	return &rt
}

func mustCreateGroupRelation(t *testing.T, ctx *MahresourcesContext, fromID, toID, typeID uint) *models.GroupRelation {
	t.Helper()
	rel := models.GroupRelation{
		FromGroupId:    &fromID,
		ToGroupId:      &toID,
		RelationTypeId: &typeID,
	}
	if err := ctx.db.Create(&rel).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	t.Cleanup(func() {
		ctx.db.Unscoped().Delete(&models.GroupRelation{}, rel.ID)
	})
	return &rel
}

func TestBuildExportPlan_DetectsDanglingRelatedGroup(t *testing.T) {
	ctx := createTestContext(t)

	inScope := mustCreateGroup(t, ctx, "InScope", nil)
	outOfScope := mustCreateGroup(t, ctx, "OutOfScope", nil)

	// Add an m2m RelatedGroups link from inScope -> outOfScope.
	mustLinkRelatedGroup(t, ctx, inScope.ID, outOfScope.ID)

	req := &ExportRequest{
		RootGroupIDs: []uint{inScope.ID},
		Scope: archive.ExportScope{
			Subtree:        true,
			OwnedResources: true,
			OwnedNotes:     true,
			RelatedM2M:     true,
			GroupRelations: true,
		},
	}

	plan, err := ctx.buildExportPlan(req)
	if err != nil {
		t.Fatalf("buildExportPlan: %v", err)
	}

	if len(plan.dangling) != 1 {
		t.Fatalf("dangling = %d, want 1: %+v", len(plan.dangling), plan.dangling)
	}
	d := plan.dangling[0]
	if d.Kind != archive.DanglingKindRelatedGroup {
		t.Errorf("kind = %q, want %q", d.Kind, archive.DanglingKindRelatedGroup)
	}
	if d.ToStub.SourceID != outOfScope.ID || d.ToStub.Name != "OutOfScope" {
		t.Errorf("stub = %+v", d.ToStub)
	}
}

func TestBuildExportPlan_DetectsDanglingGroupRelation(t *testing.T) {
	ctx := createTestContext(t)

	inScope := mustCreateGroup(t, ctx, "InScope", nil)
	outOfScope := mustCreateGroup(t, ctx, "OutOfScope", nil)

	relType := mustCreateGroupRelationType(t, ctx, "DerivedFrom")
	mustCreateGroupRelation(t, ctx, inScope.ID, outOfScope.ID, relType.ID)

	plan, err := ctx.buildExportPlan(&ExportRequest{
		RootGroupIDs: []uint{inScope.ID},
		Scope:        archive.ExportScope{Subtree: true, GroupRelations: true},
	})
	if err != nil {
		t.Fatalf("buildExportPlan: %v", err)
	}

	found := false
	for _, d := range plan.dangling {
		if d.Kind == archive.DanglingKindGroupRelation && d.RelationTypeName == "DerivedFrom" && d.ToStub.SourceID == outOfScope.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DerivedFrom dangling ref, got %+v", plan.dangling)
	}
}

func TestBuildExportPlan_CollectsLargeSubtreeWithoutTruncation(t *testing.T) {
	// Regression test for the GetGroupTreeDown(100, 5000) cap bug.
	// Seeds >50 direct children under one parent and verifies the plan
	// includes all of them.
	ctx := createTestContext(t)
	root := mustCreateGroup(t, ctx, "BigRoot", nil)
	const N = 120
	for i := 0; i < N; i++ {
		mustCreateGroup(t, ctx, fmt.Sprintf("child%03d", i), &root.ID)
	}

	plan, err := ctx.buildExportPlan(&ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true},
	})
	if err != nil {
		t.Fatalf("buildExportPlan: %v", err)
	}
	// Expect: 1 root + N children
	if got, want := len(plan.groupIDs), N+1; got != want {
		t.Fatalf("plan.groupIDs = %d, want %d", got, want)
	}
}
