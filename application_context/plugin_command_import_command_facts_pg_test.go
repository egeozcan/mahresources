//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestPluginCommandImportRetrySelectorParityAndQueryCountPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	service := ctx.JobService()
	require.NoError(t, ctx.db.AutoMigrate(&models.PluginCommandRunOutput{}))
	const actorID = uint(9017)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	runID := "pg-retry-selector-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID),
		CreatedAt: time.Now().UTC(),
	}).Error)
	seedPluginCommandRunJob(t, ctx, actorID, "pg-retry-selector-run-job")
	for i := 0; i < 24; i++ {
		fileName := fmt.Sprintf("candidate-%03d.bin", i)
		importID := fmt.Sprintf("pg-retry-selector-%03d", i)
		seedPluginCommandRetryCandidate(t, ctx, actorID, runID, importID, fileName)
		createPluginCommandRetryFile(t, root, "worker", runID, fileName)
	}

	admin := jobs.Access{Administrator: true}
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	assertAdapterSelectorMatchesCommands(t, ctx, admin, jobs.CommandCancel)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "inspect")

	var selectorQueries atomic.Int64
	callbackName := "test:plugin-command-import-retry-selector-count-pg"
	err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "plugin_command_import_command_facts") {
			selectorQueries.Add(1)
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	listAndSummary := func(expected int) int64 {
		t.Helper()
		selectorQueries.Store(0)
		page, err := service.List(ctx.jobDeps(), admin, jobs.Filter{Command: "retry-import"}, jobs.Cursor{}, 200)
		require.NoError(t, err)
		require.Len(t, page.Jobs, expected)
		summary, err := service.Summary(ctx.jobDeps(), admin, jobs.Filter{Command: "retry-import"}, 24*time.Hour)
		require.NoError(t, err)
		require.EqualValues(t, expected, summary.Total)
		return selectorQueries.Load()
	}
	small := listAndSummary(24)
	for i := 24; i < 48; i++ {
		fileName := fmt.Sprintf("candidate-%03d.bin", i)
		importID := fmt.Sprintf("pg-retry-selector-%03d", i)
		seedPluginCommandRetryCandidate(t, ctx, actorID, runID, importID, fileName)
		createPluginCommandRetryFile(t, root, "worker", runID, fileName)
	}
	large := listAndSummary(48)
	require.Equal(t, small, large, "PostgreSQL selector query count must not grow with candidate Jobs")

	require.NoError(t, ctx.db.Model(&models.User{}).Where("id = ?", actorID).Update("role", models.RoleGuest).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
}
