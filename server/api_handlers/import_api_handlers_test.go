package api_handlers

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/archive"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
)

// noopImportSink satisfies download_queue.ProgressSink for the apply runFn
// under test. It records warnings so the test can inspect them.
type noopImportSink struct {
	warnings []string
}

func (n *noopImportSink) SetPhase(string)               {}
func (n *noopImportSink) SetPhaseProgress(int64, int64) {}
func (n *noopImportSink) UpdateProgress(int64, int64)   {}
func (n *noopImportSink) AppendWarning(s string)        { n.warnings = append(n.warnings, s) }
func (n *noopImportSink) SetResultPath(string)          {}

var _ download_queue.ProgressSink = (*noopImportSink)(nil)

// TestApplyImport_NoGUIDResourceMarksRetryUnsafe verifies that an archive
// with GUIDs on its groups/notes but missing a GUID on a resource is
// still flagged retry-unsafe — replay would fall into the hash-collision
// branch of applyOneResource, either silently dropping M2M wiring (skip
// policy) or inserting a duplicate resource row (non-skip policies).
func TestApplyImport_NoGUIDResourceMarksRetryUnsafe(t *testing.T) {
	dsn := "file:noguid_resource_unsafe?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ResourceCategory{}, &models.Group{}, &models.Resource{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	fs := afero.NewMemMapFs()
	ctx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	db.FirstOrCreate(&models.ResourceCategory{Name: "Default"}, 1)

	// Group has a GUID; the resource payload intentionally omits one.
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1, Resources: 1},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "MixedGroup", SourceID: 1, Path: "groups/g0001.json"},
			},
			Resources: []archive.ResourceEntry{
				{ExportID: "r0001", Name: "legacy-res", SourceID: 1, Owner: "g0001", Path: "resources/r0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  1,
		Name:      "MixedGroup",
		GUID:      "019d8a00-0000-7000-8000-000000000010",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.WriteResource(&archive.ResourcePayload{
		ExportID:    "r0001",
		SourceID:    1,
		Name:        "legacy-res",
		Hash:        "deadbeef",
		HashType:    "SHA1",
		OwnerRef:    "g0001",
		BlobMissing: true,
		// GUID intentionally omitted — simulates a pre-GUID resource.
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteResource: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	jobID := "mixed-guid-resource"
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(fs, tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	if _, err := ctx.ParseImport(context.Background(), jobID, tarPath); err != nil {
		t.Fatalf("parse: %v", err)
	}
	decisions := &application_context.ImportDecisions{
		GUIDCollisionPolicy:      "merge", // not skip — isolate the resource-GUID signal
		AcknowledgeMissingHashes: true,
		MappingActions:           map[string]application_context.MappingAction{},
		DanglingActions:          map[string]application_context.DanglingAction{},
	}

	result, err := ctx.ApplyImport(context.Background(), jobID, filepath.Join("_imports", jobID+".plan.json"), decisions, &noopImportSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RetrySafe {
		t.Error("expected RetrySafe=false when the archive has a resource without a GUID")
	}
}

// TestApplyImport_SkipPolicyMarksResultRetryUnsafe verifies that choosing
// GUIDCollisionPolicy=skip at apply time flips result.RetrySafe to false,
// even when the archive itself carries GUIDs on every entity. Replaying
// such an apply would hit the skip branches for rows the first run
// created and silently drop their archive M2M wiring.
func TestApplyImport_SkipPolicyMarksResultRetryUnsafe(t *testing.T) {
	dsn := "file:skip_policy_unsafe?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ResourceCategory{}, &models.Category{}, &models.Group{},
		&models.Tag{}, &models.NoteType{}, &models.Note{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	fs := afero.NewMemMapFs()
	ctx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	db.FirstOrCreate(&models.ResourceCategory{Name: "Default"}, 1)

	// A modern archive — every entity carries a GUID.
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "SkipGroup", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  1,
		Name:      "SkipGroup",
		GUID:      "019d8a00-0000-7000-8000-000000000001",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	jobID := "skip-retry-unsafe"
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(fs, tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	decisions := &application_context.ImportDecisions{
		GUIDCollisionPolicy: "skip",
		MappingActions:      map[string]application_context.MappingAction{},
		DanglingActions:     map[string]application_context.DanglingAction{},
	}
	for _, entry := range plan.Mappings.Categories {
		decisions.MappingActions[entry.DecisionKey] = application_context.MappingAction{
			Include: true, Action: "create", DestinationID: entry.DestinationID,
		}
	}

	result, err := ctx.ApplyImport(context.Background(), jobID, filepath.Join("_imports", jobID+".plan.json"), decisions, &noopImportSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RetrySafe {
		t.Error("expected RetrySafe=false when GUIDCollisionPolicy=skip (modern archive, but skip drops M2M on retry)")
	}
}
