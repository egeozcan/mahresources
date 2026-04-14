package api_handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

func (n *noopImportSink) SetPhase(string)              {}
func (n *noopImportSink) SetPhaseProgress(int64, int64) {}
func (n *noopImportSink) UpdateProgress(int64, int64)   {}
func (n *noopImportSink) AppendWarning(s string)        { n.warnings = append(n.warnings, s) }
func (n *noopImportSink) SetResultPath(string)          {}

var _ download_queue.ProgressSink = (*noopImportSink)(nil)

// TestBuildImportApplyRunFn_RestoresPlanOnFailure verifies that when
// ApplyImport fails, the runFn renames the consumed plan file back to
// .plan.json so the user can POST /apply again without re-uploading.
// Failure is deterministic: we don't stage a tar, so Phase 1 errors out
// immediately with "open tar".
func TestBuildImportApplyRunFn_RestoresPlanOnFailure(t *testing.T) {
	dsn := "file:import_restore_fail?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.ResourceCategory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	fs := afero.NewMemMapFs()
	ctx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	db.FirstOrCreate(&models.ResourceCategory{Name: "Default"}, 1)

	jobID := "restore-fail-test"
	planPath := filepath.Join("_imports", jobID+".plan.json")
	consumedPath := filepath.Join("_imports", jobID+".plan.applied.json")

	// Simulate handler state after it consumed the plan but before runFn runs.
	plan := &application_context.ImportPlan{JobID: jobID}
	planBytes, _ := json.Marshal(plan)
	if err := fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(fs, consumedPath, planBytes, 0644); err != nil {
		t.Fatalf("write consumed plan: %v", err)
	}
	// Intentionally no tar — ApplyImport's Phase 1 will fail.

	sink := &noopImportSink{}
	runFn := buildImportApplyRunFn(ctx, jobID, consumedPath, &application_context.ImportDecisions{
		MappingActions:  map[string]application_context.MappingAction{},
		DanglingActions: map[string]application_context.DanglingAction{},
	})

	if err := runFn(context.Background(), nil, sink); err == nil {
		t.Fatal("expected runFn to return an error when tar is missing")
	}

	// Plan must be back at .plan.json so a retry can succeed.
	if exists, _ := afero.Exists(fs, planPath); !exists {
		t.Errorf("plan was not restored: %s does not exist after failure", planPath)
	}
	if exists, _ := afero.Exists(fs, consumedPath); exists {
		t.Errorf("consumed plan still exists at %s after restoration", consumedPath)
	}
}

// TestBuildImportApplyRunFn_DoesNotRestoreLegacyPlan verifies that when
// the archive has groups/notes WITHOUT GUIDs (pre-GUID legacy format),
// a failed apply does NOT restore the plan — retrying would duplicate
// those rows since group/note names aren't uniquely indexed. The user
// must re-upload the archive to get a clean parse.
func TestBuildImportApplyRunFn_DoesNotRestoreLegacyPlan(t *testing.T) {
	dsn := "file:import_legacy_no_restore?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.ResourceCategory{}, &models.Group{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	fs := afero.NewMemMapFs()
	ctx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	db.FirstOrCreate(&models.ResourceCategory{Name: "Default"}, 1)

	jobID := "legacy-no-restore"
	planPath := filepath.Join("_imports", jobID+".plan.json")
	consumedPath := filepath.Join("_imports", jobID+".plan.applied.json")
	tarPath := filepath.Join("_imports", jobID+".tar")

	// Build a legacy-style archive: a group with NO GUID on the payload,
	// and a tar that parses fine but fails mid-Phase-2 because we kill the
	// context just after collection (the easiest reproducible failure mode
	// that still lets collection complete and sets RetrySafe correctly).
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
				{ExportID: "g0001", Name: "LegacyGroup", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  1,
		Name:      "LegacyGroup",
		// GUID intentionally empty — simulates pre-GUID archive.
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	if err := fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(fs, tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	// Parse writes .plan.json; simulate handler's rename.
	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := fs.Rename(planPath, consumedPath); err != nil {
		t.Fatalf("rename plan to consumed: %v", err)
	}

	// Cancel the context before apply runs so ApplyImport fails AFTER
	// collection (which records RetrySafe=false for this legacy archive).
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	sink := &noopImportSink{}
	decisions := &application_context.ImportDecisions{
		MappingActions:  map[string]application_context.MappingAction{},
		DanglingActions: map[string]application_context.DanglingAction{},
	}
	for _, entry := range plan.Mappings.Categories {
		decisions.MappingActions[entry.DecisionKey] = application_context.MappingAction{
			Include: true, Action: "create", DestinationID: entry.DestinationID,
		}
	}

	runFn := buildImportApplyRunFn(ctx, jobID, consumedPath, decisions)
	if err := runFn(cancelledCtx, nil, sink); err == nil {
		t.Fatal("expected runFn to return an error (cancelled context)")
	}

	// Plan must NOT have been restored — the archive is not retry-safe.
	if exists, _ := afero.Exists(fs, planPath); exists {
		t.Errorf("plan was restored at %s, but legacy archive is not retry-safe", planPath)
	}
	if exists, _ := afero.Exists(fs, consumedPath); !exists {
		t.Errorf("consumed plan disappeared at %s (expected to remain in place)", consumedPath)
	}
}
