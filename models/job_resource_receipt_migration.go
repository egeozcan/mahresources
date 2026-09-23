package models

import (
	"fmt"

	"gorm.io/gorm"
)

// EnsureJobResourceReceiptConstraints installs the two acyclic receipt
// cascades on PostgreSQL. Production deliberately disables GORM's automatic
// foreign-key migration because other application models contain cycles; these
// receipt references have no reverse dependency and are safe to add explicitly.
func EnsureJobResourceReceiptConstraints(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// Serialize concurrent application starts: the absent-check and ALTER
		// TABLE must be one migration decision across all server processes.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x4D524A4F42524350)).Error; err != nil {
			return fmt.Errorf("models: lock JobResourceReceipt constraint migration: %w", err)
		}
		if err := pruneOrphanJobResourceReceipts(tx); err != nil {
			return err
		}
		for _, name := range []string{"Job", "Resource"} {
			if tx.Migrator().HasConstraint(&JobResourceReceipt{}, name) {
				continue
			}
			if err := tx.Migrator().CreateConstraint(&JobResourceReceipt{}, name); err != nil {
				return fmt.Errorf("models: create JobResourceReceipt %s cascade: %w", name, err)
			}
		}
		return nil
	})
}

// pruneOrphanJobResourceReceipts removes legacy rows left by releases that had
// no PostgreSQL cascades. One set-based DELETE keeps the upgrade idempotent and
// lets PostgreSQL choose indexed anti-join plans without materializing rows in Go.
func pruneOrphanJobResourceReceipts(tx *gorm.DB) error {
	result := tx.Exec(`
DELETE FROM job_resource_receipts AS receipt
WHERE NOT EXISTS (SELECT 1 FROM jobs AS job WHERE job.id = receipt.job_id)
   OR NOT EXISTS (SELECT 1 FROM resources AS resource WHERE resource.id = receipt.resource_id)`)
	if result.Error != nil {
		return fmt.Errorf("models: prune orphan JobResourceReceipt rows: %w", result.Error)
	}
	return nil
}
