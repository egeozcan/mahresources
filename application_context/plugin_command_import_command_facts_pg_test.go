//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
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

func TestPluginCommandImportRetryFactsFollowReplayPurgeAndRetiredEnvelopePostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	service := ctx.JobService()
	require.NoError(t, ctx.db.AutoMigrate(&models.PluginCommandRunOutput{}))
	const actorID = uint(9023)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	runID := "pg-retry-fact-retirement-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID), CreatedAt: time.Now().UTC(),
	}).Error)
	group := models.Group{Name: "pg-retry-fact-retirement-group"}
	require.NoError(t, ctx.db.Create(&group).Error)
	acceptedFields, err := json.Marshal(plugin_commands.ResourceFields{SeriesID: 314, GroupIDs: []uint{group.ID}})
	require.NoError(t, err)
	forgottenJobID, forgottenImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "pgrr-forget", "forget.bin", string(acceptedFields),
	)
	expiredJobID, expiredImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "pgrr-expire", "expire.bin", string(acceptedFields),
	)
	retainedJobID, retainedImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "pgrr-retain", "retained.bin", string(acceptedFields),
	)
	createPluginCommandRetryFile(t, root, "worker", runID, "forget.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "expire.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "retained.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	require.NoError(t, ctx.db.Model(&models.JobWriterEpoch{}).
		Where("id = ?", models.JobWriterEpochRowID).
		Update("minimum_epoch", models.JobWriterEpochRetiredPlaintext).Error)
	staleFields := `{"series_id":999,"group_ids":[]}`
	require.NoError(t, ctx.db.Model(&models.PluginCommandImport{}).
		Where("id IN ?", []string{forgottenImportID, expiredImportID, retainedImportID}).
		Update("fields_json", staleFields).Error)
	require.NoError(t, ctx.StopPluginCommands())
	ctx.pluginCommandController = nil
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	var canonicalFact models.PluginCommandImportCommandFact
	require.NoError(t, ctx.db.Where("job_id = ?", forgottenJobID).First(&canonicalFact).Error)
	require.Equal(t, uint(314), canonicalFact.SeriesID)
	require.Equal(t, 1, canonicalFact.GroupCount)
	var canonicalGroup models.PluginCommandImportCommandFactGroup
	require.NoError(t, ctx.db.Where("import_id = ?", forgottenImportID).First(&canonicalGroup).Error)
	require.Equal(t, group.ID, canonicalGroup.GroupID)

	admin := jobs.Access{Administrator: true}
	_, err = service.ForgetReplay(ctx.jobDeps(), admin, forgottenJobID)
	require.NoError(t, err)
	assertPluginCommandImportFactsAbsent(t, ctx, forgottenJobID, forgottenImportID)

	require.NoError(t, ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", expiredJobID).
		Update("expires_at", time.Now().UTC().Add(-time.Second)).Error)
	purged, err := service.PurgeExpiredReplay(ctx.jobDeps(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, purged)
	assertPluginCommandImportFactsAbsent(t, ctx, expiredJobID, expiredImportID)

	now := time.Now().UTC()
	finishedAt, jobDeadline, replayDeadline := now.Add(-48*time.Hour), now.Add(-24*time.Hour), now.Add(24*time.Hour)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", retainedJobID).
		Updates(map[string]any{"finished_at": finishedAt, "expires_at": jobDeadline}).Error)
	require.NoError(t, ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", retainedJobID).
		Update("expires_at", replayDeadline).Error)
	sweep, err := service.Sweep(ctx.jobDeps(), jobs.RetentionPolicy{}, jobs.SweepCursor{}, 100)
	require.NoError(t, err)
	require.Equal(t, 1, sweep.Pruned)
	assertPluginCommandImportFactsAbsent(t, ctx, retainedJobID, retainedImportID)

	require.NoError(t, ctx.StopPluginCommands())
	ctx.pluginCommandController = nil
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	assertPluginCommandImportFactsAbsent(t, ctx, forgottenJobID, forgottenImportID)
	assertPluginCommandImportFactsAbsent(t, ctx, expiredJobID, expiredImportID)
	assertPluginCommandImportFactsAbsent(t, ctx, retainedJobID, retainedImportID)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
}
