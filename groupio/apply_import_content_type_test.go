package groupio

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

func TestSanitizeImportedContentType(t *testing.T) {
	for _, ok := range []string{"", "image/png", "text/plain; charset=utf-8"} {
		if got, valid := sanitizeImportedContentType(ok); got != ok || !valid {
			t.Errorf("%q: got %q,%v; want unchanged,true", ok, got, valid)
		}
	}
	for _, bad := range []string{"image/x'-alert`1`-'", "image/png');window.__xss=1;('", `a/b"c`, "not a type"} {
		if got, valid := sanitizeImportedContentType(bad); got != "application/octet-stream" || valid {
			t.Errorf("%q: got %q,%v; want octet-stream,false", bad, got, valid)
		}
	}
}

func TestApplyImport_RejectsUnsafeContentType(t *testing.T) {
	srcCtx := createTestContext(t)
	root := mustCreateGroup(t, srcCtx, "CTRoot", nil)
	content := []byte("CT_CONTENT")
	res := mustCreateResource(t, srcCtx, "ct.txt", &root.ID, content)
	evil := "image/x'-alert`1`-'"
	ver := models.ResourceVersion{ResourceID: res.ID, VersionNumber: 1, Hash: res.Hash, HashType: "SHA1",
		FileSize: int64(len(content)), ContentType: evil, Location: res.Location}
	if err := srcCtx.db.Create(&ver).Error; err != nil {
		t.Fatal(err)
	}
	if err := srcCtx.db.Model(res).Updates(map[string]any{"current_version_id": ver.ID, "content_type": evil}).Error; err != nil {
		t.Fatal(err)
	}

	var tarBuf bytes.Buffer
	if err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true, ResourceVersions: true},
	}, &tarBuf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createTestContext(t)
	jobID := "test-evil-ct"
	tarPath := filepath.Join("_imports", jobID+".tar")
	dstCtx.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)
	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "replace"
	result, err := dstCtx.ApplyImport(context.Background(), jobID, importPlanPath(jobID), decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.CreatedResourceIDs) == 0 {
		t.Fatal("no replaced resource")
	}
	var got models.Resource
	if err := dstCtx.db.Preload("Versions").First(&got, result.CreatedResourceIDs[0]).Error; err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "application/octet-stream" {
		t.Errorf("resource content type = %q", got.ContentType)
	}
	if len(got.Versions) == 0 {
		t.Fatal("no versions")
	}
	for _, v := range got.Versions {
		if v.ContentType != "application/octet-stream" {
			t.Errorf("version content type = %q", v.ContentType)
		}
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "content type") {
			found = true
		}
	}
	if !found {
		t.Errorf("no content type warning in %v", result.Warnings)
	}
}
