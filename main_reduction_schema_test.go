package main

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

// legacyReductionShape is resource_reductions as it was before the plan-as-
// tables columns: the table an upgrading database already holds.
type legacyReductionShape struct {
	ID                       uint      `gorm:"primarykey"`
	CreatedAt                time.Time `gorm:"index"`
	UpdatedAt                time.Time
	Name                     string `gorm:"size:1000;not null"`
	CreatedByUserId          *uint  `gorm:"index"`
	Status                   string `gorm:"size:20;not null;index"`
	MatchingMode             string `gorm:"size:20;not null"`
	ExcludeExternalResources bool   `gorm:"not null;default:false"`
	KeepAsVersionIdentical   bool   `gorm:"not null;default:false"`
	KeepAsVersionNear        bool   `gorm:"not null;default:true"`
	WinnerRule               []byte `gorm:"type:json"`
	Extent                   []byte `gorm:"type:json"`
	Plan                     []byte `gorm:"type:json"`
	ComputedAt               *time.Time
	ComputingStartedAt       *time.Time
	ComputeDeadline          *time.Time
	ComputeJobID             string `gorm:"size:64"`
	ComputeError             string `gorm:"size:2000"`
	Version                  uint   `gorm:"not null;default:0"`
}

func (legacyReductionShape) TableName() string { return "resource_reductions" }

// plan_schema's default is the whole migration's crash safety (design D1):
// AutoMigrate backfills it as 0, so a server that dies the instant after
// AutoMigrate leaves every legacy row reading "legacy JSON plan", never "plan in
// tables". Only the migration writes 1.
func assertAutoMigrateLeavesLegacyRowsAtSchemaZero(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&legacyReductionShape{}); err != nil {
		t.Fatalf("create the legacy table: %v", err)
	}
	if err := db.Create(&legacyReductionShape{Name: "legacy", Status: "ready", MatchingMode: "both", Plan: []byte(`{"clusters":[]}`)}).Error; err != nil {
		t.Fatalf("insert the legacy row: %v", err)
	}

	// The startup is killed right here: AutoMigrate ran, nothing after it did.
	if err := migrateModels(db); err != nil {
		t.Fatalf("migrateModels: %v", err)
	}

	var rows []struct {
		Name        string
		PlanSchema  int
		ExtentAll   bool
		SettleLease int64 `gorm:"column:settle_lease_until"`
	}
	if err := db.Raw("SELECT name, plan_schema, extent_all, settle_lease_until FROM resource_reductions").Scan(&rows).Error; err != nil {
		t.Fatalf("read the row back: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want the one legacy row, got %d", len(rows))
	}
	// Scan turns a NULL into 0, so ask the database itself: dropping the column's
	// default would leave NULL here, which `= 0` does not match.
	var legacy int64
	if err := db.Raw("SELECT COUNT(*) FROM resource_reductions WHERE plan_schema = 0").Scan(&legacy).Error; err != nil || legacy != 1 {
		t.Fatalf("rows with plan_schema = 0: %d (err %v), want the 1 legacy row", legacy, err)
	}
	if rows[0].PlanSchema != 0 {
		t.Fatalf("plan_schema = %d after AutoMigrate on a legacy row, want 0", rows[0].PlanSchema)
	}
	if rows[0].ExtentAll || rows[0].SettleLease != 0 {
		t.Fatalf("the new columns did not take their defaults: %+v", rows[0])
	}
}

func TestAutoMigrateLeavesLegacyReductionRowsAtSchemaZero(t *testing.T) {
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, filepath.Join(t.TempDir(), "legacy.db"), "", 0)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	assertAutoMigrateLeavesLegacyRowsAtSchemaZero(t, db)
}

func assertSimilarityDistanceIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&models.Resource{}, &models.ResourceSimilarity{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || ok {
		t.Fatalf("before the build: valid = %v, err = %v; want false", ok, err)
	}
	for pass := 0; pass < 2; pass++ { // idempotent: a second boot is a no-op
		if err := models.EnsureSimilarityDistanceIndexes(db); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || !ok {
			t.Fatalf("pass %d: valid = %v, err = %v; want true", pass, ok, err)
		}
	}
}

func TestSimilarityDistanceIndexesAreBuiltOnSQLite(t *testing.T) {
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, filepath.Join(t.TempDir(), "sim.db"), "", 0)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	assertSimilarityDistanceIndexes(t, db)

	// The neighbour query's order is served by the index, not a sort.
	var plan []struct{ Detail string }
	if err := db.Raw(`EXPLAIN QUERY PLAN SELECT resource_id2 FROM resource_similarities WHERE resource_id1 = 1
		ORDER BY COALESCE(p_distance, hamming_distance), resource_id2 LIMIT 400`).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	var sawIndex, sawSort bool
	for _, row := range plan {
		if row.Detail != "" && contains(row.Detail, "idx_sim_r1_eff_dist") {
			sawIndex = true
		}
		if contains(row.Detail, "TEMP B-TREE") {
			sawSort = true
		}
	}
	if !sawIndex || sawSort {
		t.Fatalf("the neighbour query should read idx_sim_r1_eff_dist with no sort; plan = %+v", plan)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
