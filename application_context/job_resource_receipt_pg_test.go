//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"
)

func TestPostgresReceiptCascadesUnderProductionMigrationSettings(t *testing.T) {
	_, dsn := pgContainer.CreateTestDBWithDSN(t)
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open PostgreSQL database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Job{}, &models.Resource{}, &models.JobResourceReceipt{},
		&models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{}, &models.JobOutput{},
		&models.JobReplayEnvelope{}, &models.JobClaim{}, &models.JobCapacityLease{},
		&models.JobPreference{}, &models.JobPinGuard{}, &models.JobCommandRequest{}, &models.JobLegacyHandle{}, &models.JobImportCommandFact{},
	); err != nil {
		t.Fatalf("migrate receipt tables with production settings: %v", err)
	}
	var migrations sync.WaitGroup
	migrationErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		migrations.Add(1)
		go func() {
			defer migrations.Done()
			migrationErrors <- models.EnsureJobResourceReceiptConstraints(db)
		}()
	}
	migrations.Wait()
	close(migrationErrors)
	for err := range migrationErrors {
		if err != nil {
			t.Fatalf("install receipt cascade constraints concurrently: %v", err)
		}
	}
	jobID := "00000000-0000-7000-8000-000000000001"
	job := &models.Job{
		ID: jobID, Kind: "remote-download", KindVersion: 1, State: "succeeded", Origin: "api",
		VisibilityClass: "owner", ExecutionPrincipal: "host", ReplayClass: "replayable",
		Version: 1, AcceptedAt: time.Now().UTC(),
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create Job: %v", err)
	}
	resource := &models.Resource{Name: "receipt-cascade", Hash: "receipt-hash", ResourceCategoryId: 1}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("create Resource: %v", err)
	}
	if err := db.Create(&models.JobResourceReceipt{JobID: jobID, ResourceID: resource.ID, Hash: resource.Hash}).Error; err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	if err := db.Delete(&models.Resource{}, resource.ID).Error; err != nil {
		t.Fatalf("delete Resource: %v", err)
	}
	var receipts int64
	if err := db.Model(&models.JobResourceReceipt{}).Where("job_id = ?", jobID).Count(&receipts).Error; err != nil {
		t.Fatalf("count receipts after Resource deletion: %v", err)
	}
	if receipts != 0 {
		t.Fatalf("Resource deletion left %d receipt rows", receipts)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("read-only PostgreSQL handle: %v", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})
	decision, err := (&downloadJobAdapter{ctx: ctx, kind: JobKindRemoteDownload}).Reconcile(context.Background(), jobs.ReconcileRequest{
		Snapshot: jobs.Snapshot{ID: jobID}, Claimant: goneRuntimeIdentityForTest(),
	})
	if err != nil || decision != jobs.ReconcileQueue {
		t.Fatalf("reconcile after Resource deletion = %q, %v; want queue without deferral", decision, err)
	}

	secondJobID := "00000000-0000-7000-8000-000000000002"
	job.ID = secondJobID
	finishedAt := time.Now().UTC().Add(-48 * time.Hour)
	expiresAt := time.Now().UTC().Add(-24 * time.Hour)
	job.FinishedAt = &finishedAt
	job.ExpiresAt = &expiresAt
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create second Job: %v", err)
	}
	resource = &models.Resource{Name: "job-receipt-cascade", Hash: "job-receipt-hash", ResourceCategoryId: 1}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("create second Resource: %v", err)
	}
	if err := db.Create(&models.JobResourceReceipt{JobID: secondJobID, ResourceID: resource.ID, Hash: resource.Hash}).Error; err != nil {
		t.Fatalf("create second receipt: %v", err)
	}
	result, err := jobs.NewService().Sweep(jobs.Deps{DB: db}, jobs.RetentionPolicy{History: time.Second}, jobs.SweepCursor{}, 32)
	if err != nil {
		t.Fatalf("sweep expired Job: %v", err)
	}
	if result.Pruned != 1 {
		t.Fatalf("retention pruned %d Jobs, want 1", result.Pruned)
	}
	if err := db.Model(&models.JobResourceReceipt{}).Where("job_id = ?", secondJobID).Count(&receipts).Error; err != nil {
		t.Fatalf("count receipts after Job deletion: %v", err)
	}
	if receipts != 0 {
		t.Fatalf("Job deletion left %d receipt rows", receipts)
	}
}

func TestPostgresReceiptMigrationRemovesLegacyOrphansAndPreservesValidRows(t *testing.T) {
	_, dsn := pgContainer.CreateTestDBWithDSN(t)
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open PostgreSQL database: %v", err)
	}
	if err := db.AutoMigrate(&models.Job{}, &models.Resource{}, &models.JobResourceReceipt{}); err != nil {
		t.Fatalf("migrate receipt tables with production settings: %v", err)
	}
	job := &models.Job{
		ID: "00000000-0000-7000-8000-000000000011", Kind: "remote-download", KindVersion: 1,
		State: "succeeded", Origin: "api", VisibilityClass: "owner", ExecutionPrincipal: "host",
		ReplayClass: "replayable", Version: 1, AcceptedAt: time.Now().UTC(),
	}
	missingResourceJob := *job
	missingResourceJob.ID = "00000000-0000-7000-8000-000000000012"
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create valid Job: %v", err)
	}
	if err := db.Create(&missingResourceJob).Error; err != nil {
		t.Fatalf("create Job for missing-Resource orphan: %v", err)
	}
	resource := &models.Resource{Name: "legacy-receipt", Hash: "legacy-hash", ResourceCategoryId: 1}
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("create Resource: %v", err)
	}
	if err := db.Create(&models.JobResourceReceipt{JobID: job.ID, ResourceID: resource.ID, Hash: resource.Hash}).Error; err != nil {
		t.Fatalf("create valid receipt: %v", err)
	}
	missingJobID := "00000000-0000-7000-8000-000000000013"
	if err := db.Create(&models.JobResourceReceipt{JobID: missingJobID, ResourceID: resource.ID, Hash: resource.Hash}).Error; err != nil {
		t.Fatalf("seed missing-Job receipt: %v", err)
	}
	if err := db.Create(&models.JobResourceReceipt{JobID: missingResourceJob.ID, ResourceID: 987654321, Hash: "missing-resource-hash"}).Error; err != nil {
		t.Fatalf("seed missing-Resource receipt: %v", err)
	}
	if err := models.EnsureJobResourceReceiptConstraints(db); err != nil {
		t.Fatalf("upgrade legacy receipt table: %v", err)
	}
	if err := models.EnsureJobResourceReceiptConstraints(db); err != nil {
		t.Fatalf("repeat upgraded receipt migration: %v", err)
	}
	var count int64
	if err := db.Model(&models.JobResourceReceipt{}).Count(&count).Error; err != nil {
		t.Fatalf("count migrated receipts: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration retained %d receipt rows, want only the valid receipt", count)
	}
	var receipt models.JobResourceReceipt
	if err := db.First(&receipt, "job_id = ?", job.ID).Error; err != nil {
		t.Fatalf("read valid receipt after cleanup: %v", err)
	}
}
