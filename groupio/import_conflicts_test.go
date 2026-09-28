package groupio

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

// The review's conflict counts predict what the apply does with each resource: one
// whose GUID already exists here is decided by the GUID policy, whatever its hash,
// and only a resource no GUID claims is decided by the resource collision policy.
func TestParseImport_ResourceConflictsCountWhatEachPolicyDecides(t *testing.T) {
	srcCtx := createGUIDIsolatedContext(t, "conflict_counts_src")
	root := mustCreateGroup(t, srcCtx, "ConflictRoot", nil)
	byGUID := mustCreateResource(t, srcCtx, "by-guid.txt", &root.ID, []byte("CONFLICT_BY_GUID"))
	byHash := mustCreateResource(t, srcCtx, "by-hash.txt", &root.ID, []byte("CONFLICT_BY_HASH"))
	mustCreateResource(t, srcCtx, "new.txt", &root.ID, []byte("CONFLICT_NEW"))
	srcCtx.db.First(byGUID, byGUID.ID)
	if byGUID.GUID == nil {
		t.Fatal("expected the source resource to have a GUID")
	}

	var tarBuf bytes.Buffer
	if err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
	}, &tarBuf, func(ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createGUIDIsolatedContext(t, "conflict_counts_dst")
	seed := func(name string, guid *string, hash string) {
		t.Helper()
		location := "/resources/" + name
		if err := afero.WriteFile(dstCtx.fs, location, []byte(name), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		row := &models.Resource{Name: name, GUID: guid, Hash: hash, HashType: "SHA1",
			FileSize: int64(len(name)), Location: location, ResourceCategoryId: testDefaultResourceCategoryID}
		if err := dstCtx.db.Create(row).Error; err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	// The same resource imported before: its GUID and its content both exist here.
	seed("existing-by-guid", byGUID.GUID, byGUID.Hash)
	// Other content under a GUID of its own that happens to hold the same bytes.
	seed("existing-by-hash", nil, byHash.Hash)

	jobID := "test-conflict-counts"
	tarPath := filepath.Join("_imports", jobID+".tar")
	_ = dstCtx.fs.MkdirAll("_imports", 0755)
	if err := afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644); err != nil {
		t.Fatalf("stage archive: %v", err)
	}
	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if plan.Conflicts.ResourceGUIDMatches != 1 {
		t.Errorf("resource GUID matches = %d, want 1", plan.Conflicts.ResourceGUIDMatches)
	}
	if plan.Conflicts.ResourceHashMatches != 1 {
		t.Errorf("resource hash matches = %d, want 1: a resource its GUID already claims is decided by the GUID policy",
			plan.Conflicts.ResourceHashMatches)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.ResourceCollisionPolicy = "skip"
	decisions.GUIDCollisionPolicy = "merge"
	result, err := dstCtx.ApplyImport(context.Background(), jobID, importPlanPath(jobID), decisions, noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.MergedResources != plan.Conflicts.ResourceGUIDMatches {
		t.Errorf("merged = %d, want the plan's %d GUID matches", result.MergedResources, plan.Conflicts.ResourceGUIDMatches)
	}
	if result.SkippedByHash != plan.Conflicts.ResourceHashMatches {
		t.Errorf("skipped by hash = %d, want the plan's %d hash matches", result.SkippedByHash, plan.Conflicts.ResourceHashMatches)
	}
	if result.CreatedResources != 1 {
		t.Errorf("created = %d, want 1 for the resource nothing here matches", result.CreatedResources)
	}
}
