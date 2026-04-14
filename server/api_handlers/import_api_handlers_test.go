package api_handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/application_context"
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
