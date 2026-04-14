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

// TestShouldRestorePlan covers the four branches of the restoration gate:
// nil result (pre-Phase-1 failure), non-nil but no mutations (pre-write
// Phase 2 abort), mutations on a retry-safe archive, and mutations on a
// retry-unsafe archive (legacy pre-GUID or GUIDCollisionPolicy=skip).
func TestShouldRestorePlan(t *testing.T) {
	cases := []struct {
		name   string
		result *application_context.ImportApplyResult
		want   bool
	}{
		{"nil result", nil, true},
		{"no mutations, retry-unsafe",
			&application_context.ImportApplyResult{RetrySafe: false},
			true},
		{"mutations, retry-safe",
			&application_context.ImportApplyResult{CreatedCategories: 1, RetrySafe: true},
			true},
		{"mutations, retry-unsafe",
			&application_context.ImportApplyResult{CreatedCategories: 1, RetrySafe: false},
			false},
		{"mutations via group IDs, retry-unsafe",
			&application_context.ImportApplyResult{CreatedGroupIDs: []uint{1}, RetrySafe: false},
			false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRestorePlan(tc.result); got != tc.want {
				t.Errorf("shouldRestorePlan = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildImportApplyRunFn_DoesNotRestoreLegacyPlan verifies that when a
// legacy pre-GUID archive fails AFTER at least one DB mutation, the
// consumed plan is not restored. (Failure before any mutation is covered
// by the RestoresPlanOnFailure and ShouldRestorePlan tests and IS expected
// to restore the plan — DB state is unchanged then, so replay is safe.)
//
// We simulate "mutation happened" via shouldRestorePlan's logic; the
// handler-level integration for this legacy-with-mutations scenario is
// covered by the CLI/E2E import tests, which exercise real archives.
func TestBuildImportApplyRunFn_LegacyWithMutationsNotRestored(t *testing.T) {
	r := &application_context.ImportApplyResult{CreatedGroups: 1, RetrySafe: false}
	if shouldRestorePlan(r) {
		t.Error("expected shouldRestorePlan=false for legacy archive with committed mutations")
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

	result, err := ctx.ApplyImport(context.Background(), jobID, decisions, &noopImportSink{})
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

// legacyRestorePlanDropped is a placeholder to keep parity with the older
// TestBuildImportApplyRunFn_DoesNotRestoreLegacyPlan below, which used a
// pre-cancelled context to trigger failure before Phase 2 mutations. With
// the P2 fix, that path now correctly restores the plan (no mutations
// means replay is safe regardless of RetrySafe), so the old assertion is
// obsolete. Retained here as a named sentinel so the test list documents
// the behavior change.
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

	// Pre-cancelled context fails at the first inter-phase checkpoint
	// before any schema def / group is written. result has no mutations,
	// so the plan is safe to restore even though the archive is legacy.
	// Replay against an unchanged DB cannot duplicate anything.
	if exists, _ := afero.Exists(fs, planPath); !exists {
		t.Errorf("plan was not restored at %s even though no DB writes happened", planPath)
	}
}
