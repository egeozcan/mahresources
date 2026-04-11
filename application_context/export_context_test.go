package application_context

import (
	"crypto/sha1"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

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
