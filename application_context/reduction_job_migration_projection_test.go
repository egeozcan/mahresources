package application_context

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// The job-migration readiness check, the verified-source scrub and the restore
// verification all load resource_reductions. On a library whose legacy rows still
// carry a multi-megabyte plan they must name reductionColumns too, or startup
// receives every legacy plan before the plan migration clears it.
func TestJobMigrationReductionLoadsNeverSelectTheLegacyPlan(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	require.NoError(t, ctx.db.AutoMigrate(reductionTables...))
	big := []byte(`{"clusters":[],"pad":"` + strings.Repeat("x", 3<<20) + `"}`)
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: big, Extent: big}
	require.NoError(t, ctx.db.Create(legacy).Error)
	now := time.Now()
	id := strconv.FormatUint(uint64(legacy.ID), 10)
	mapping := models.JobSourceMapping{SourceKind: jobMigrationReduction, SourceID: id, JobID: "job-x", SourceHash: hashReductionExecution(legacy.ResourceReduction),
		Status: models.JobSourceMappingVerified, Origin: "test", CopiedAt: now, VerifiedAt: &now}
	require.NoError(t, ctx.db.Create(&mapping).Error)

	var mu sync.Mutex
	var statements []string
	record := func(tx *gorm.DB) {
		if sql := tx.Statement.SQL.String(); strings.Contains(sql, "resource_reductions") {
			mu.Lock()
			statements = append(statements, sql)
			mu.Unlock()
		}
	}
	const name = "test:record_job_migration_reduction_sql"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, record))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name, record))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name); _ = ctx.db.Callback().Row().Remove(name) })

	_, _, _ = retiredSourceNeedsScrub(ctx.db, jobMigrationReduction, id)
	_, _, _ = retiredSourceMatchesScrub(ctx.db, mapping)
	_ = ctx.db.Transaction(func(tx *gorm.DB) error {
		_, _, _ = verifyRestoredMigrationSource(ctx, tx, jobMigrationReduction, mapping)
		return nil
	})
	_, _, _ = ctx.scrubReductionMappingsBatch("", 10, now)
	_, _, _ = ctx.verifyReductionsBatch("", 10, now, false)
	_, _ = ctx.GetJobMigrationReadiness()

	legacyColumn := regexp.MustCompile("(?i)[\\s,.`\"(](plan|extent)[\\s,`\"),]")
	require.NotEmpty(t, statements)
	for _, sql := range statements {
		assert.False(t, strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT *"), "an unprojected load of resource_reductions: %s", sql)
		assert.NotRegexp(t, legacyColumn, strings.SplitN(sql, " FROM ", 2)[0], "selects the legacy plan: %s", sql)
	}
}
