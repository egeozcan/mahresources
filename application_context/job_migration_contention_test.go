package application_context

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// TestADualPublishedMappingIsNotRefusedByACommitAfterItsRead pins the mapping
// refresh's write order on SQLite. It reads the existing mapping and then saves the
// new revision in one transaction; if that transaction read first, another connection
// committing in between (the compute Job's own bookkeeping lands at the same moment)
// failed the save at once with SQLITE_BUSY_SNAPSHOT, and the refresh was logged as a
// warning and left for the next startup migration to repair. Covered both for a
// mapping that does not exist yet and for one that does.
func TestADualPublishedMappingIsNotRefusedByACommitAfterItsRead(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	require.NoError(t, ctx.db.AutoMigrate(&models.JobSourceMapping{}))
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)

	for _, existing := range []bool{false, true} {
		sourceID := map[bool]string{false: "new", true: "existing"}[existing]
		if existing {
			require.NoError(t, ctx.recordDualPublishedSource(jobMigrationReduction, sourceID, "job-0", "hash-0", false, time.Now()))
		}

		var once sync.Once
		competing := make(chan error, 1)
		var waited bool
		name := "test:commit_after_mapping_read_" + sourceID
		require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table != "job_source_mappings" {
				return
			}
			once.Do(func() {
				finished := make(chan struct{})
				go func() {
					_, err := sqlDB.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`)
					competing <- err
					close(finished)
				}()
				select {
				case <-finished:
				case <-time.After(300 * time.Millisecond):
					// Waiting on the refresh's writer lock.
					waited = true
				}
			})
		}))

		require.NoError(t, ctx.recordDualPublishedSource(jobMigrationReduction, sourceID, "job-1", "hash-1", false, time.Now()), sourceID)
		require.NoError(t, <-competing, sourceID)
		assert.True(t, waited, "%s: the competing commit landed inside the refresh", sourceID)
		require.NoError(t, ctx.db.Callback().Query().Remove(name))

		var mapping models.JobSourceMapping
		require.NoError(t, ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, sourceID).First(&mapping).Error)
		assert.Equal(t, "job-1", mapping.JobID, sourceID)
	}
}
