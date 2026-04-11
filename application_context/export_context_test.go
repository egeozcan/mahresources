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
