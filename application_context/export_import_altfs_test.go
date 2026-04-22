package application_context

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/archive"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
)

// noopSinkAltFS satisfies download_queue.ProgressSink for the alt-fs round-trip test.
// Named differently from apply_import_test.go's noopSink to avoid redeclaration.
type noopSinkAltFS struct{}

func (noopSinkAltFS) SetPhase(string)              {}
func (noopSinkAltFS) SetPhaseProgress(int64, int64) {}
func (noopSinkAltFS) UpdateProgress(int64, int64)   {}
func (noopSinkAltFS) AppendWarning(string)          {}
func (noopSinkAltFS) SetResultPath(string)          {}

var _ download_queue.ProgressSink = noopSinkAltFS{}

// createContextWithAltFs creates a fresh isolated context that has an alt-fs
// named "archival" backed by an in-memory filesystem.
func createContextWithAltFs(t *testing.T, name string) (*MahresourcesContext, afero.Fs) {
	t.Helper()

	dsn := "file:" + name + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.NoteBlock{},
		&models.PluginKV{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	altFs := afero.NewMemMapFs()

	// Config.AltFileSystems values are strings that are turned into real FS objects
	// in NewMahresourcesContext via storage.CreateStorage. For tests we bypass that:
	// we construct the context normally (no alt-fs in config) and then inject the
	// in-memory afero directly into the context's altFileSystems map.
	config := &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(fs, db, readOnlyDB, config)

	// Register the in-memory alt-fs (bypasses disk path creation).
	ctx.RegisterAltFs("archival", altFs)
	// Also persist the string key in Config so PathName validation works.
	ctx.Config.AltFileSystems = map[string]string{"archival": "/fake/archival"}

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)

	return ctx, altFs
}

// TestExportImport_PreservesStorageLocation verifies BH-023 layer 1:
// exporting a resource that has a non-empty StorageLocation writes
// storage_location to the archive payload, and importing it back restores
// the StorageLocation field on the re-created resource row.
func TestExportImport_PreservesStorageLocation(t *testing.T) {
	// Use t.Name() to get unique DB names across -count=N runs within the same process.
	// --- Source: isolated DB with alt-fs ---
	srcCtx, srcAltFs := createContextWithAltFs(t, t.Name()+"-src")

	// Seed the alt-fs with the blob so the exporter can read it.
	content := []byte("alt-fs content bh023")
	sum := sha1.Sum(content)
	hash := fmt.Sprintf("%x", sum)
	altLoc := "/resources/" + hash

	if err := afero.WriteFile(srcAltFs, altLoc, content, 0644); err != nil {
		t.Fatalf("write alt-fs blob: %v", err)
	}

	// Create group + resource with StorageLocation="archival".
	root := mustCreateGroup(t, srcCtx, "bh023-group", nil)

	storageLoc := "archival"
	res := &models.Resource{
		Name:            "bh023-res",
		ContentType:     "text/plain",
		FileSize:        int64(len(content)),
		Hash:            hash,
		HashType:        "SHA1",
		Location:        altLoc,
		StorageLocation: &storageLoc,
		OwnerId:         &root.ID,
	}
	if err := srcCtx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	// --- Export ---
	var tarBuf bytes.Buffer
	err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope: archive.ExportScope{
			Subtree:        true,
			OwnedResources: true,
		},
		Fidelity: archive.ExportFidelity{
			ResourceBlobs: true,
		},
	}, &tarBuf, nil)
	if err != nil {
		t.Fatalf("StreamExport: %v", err)
	}
	tarBytes := tarBuf.Bytes()
	if len(tarBytes) == 0 {
		t.Fatal("export produced empty tar")
	}

	// --- Destination: fresh DB, no overlap with source ---
	dstCtx := createGUIDIsolatedContext(t, t.Name()+"-dst")

	jobID := "bh023-job"
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := dstCtx.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(dstCtx.fs, tarPath, tarBytes, 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if plan.Counts.Resources != 1 {
		t.Fatalf("expected 1 resource in plan, got %d", plan.Counts.Resources)
	}

	decisions := buildDefaultDecisions(plan)
	decisions.GUIDCollisionPolicy = "merge"

	result, err := dstCtx.ApplyImport(context.Background(), jobID, decisions, noopSinkAltFS{})
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	if result.CreatedResources != 1 {
		t.Fatalf("expected 1 created resource, got %d (warnings: %v)", result.CreatedResources, result.Warnings)
	}

	// --- Assert StorageLocation is preserved ---
	var reimported models.Resource
	if err := dstCtx.db.First(&reimported, result.CreatedResourceIDs[0]).Error; err != nil {
		t.Fatalf("load reimported resource: %v", err)
	}

	if reimported.StorageLocation == nil {
		t.Fatal("BH-023: reimported resource StorageLocation is nil (expected 'archival')")
	}
	if *reimported.StorageLocation != "archival" {
		t.Errorf("BH-023: reimported StorageLocation = %q, want 'archival'", *reimported.StorageLocation)
	}
}
