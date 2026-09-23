package application_context

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestPluginCommandDatabaseFenceIsBoundToOneStagingRootAndToken(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	first, err := ctx.acquirePluginCommandDBFence("/staging/a")
	require.NoError(t, err)
	second, err := ctx.acquirePluginCommandDBFence("/staging/a")
	require.NoError(t, err, "same-root takeover is allowed after the caller acquired RuntimeLease")
	require.NotEqual(t, first, second)
	err = ctx.releasePluginCommandDBFence(first)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer owned")
	_, err = ctx.acquirePluginCommandDBFence("/staging/b")
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot)
	require.NoError(t, ctx.releasePluginCommandDBFence(second))
	_, err = ctx.acquirePluginCommandDBFence("/staging/b")
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot,
		"a graceful release must preserve the database-to-staging-root binding")
}

func TestPluginCommandControllerAcquiresStagingLeaseBeforeDatabaseFence(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	settings := testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}
	config := defaultPluginCommandControllerConfig()
	var order []string
	acquireLease := config.acquireLease
	config.acquireLease = func(root string) (*plugin_commands.RuntimeLease, error) {
		order = append(order, "staging")
		return acquireLease(root)
	}
	config.acquireDBFence = func(root string) (string, error) {
		require.Equal(t, "staging", order[len(order)-1])
		order = append(order, "database")
		return ctx.acquirePluginCommandDBFence(root)
	}
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, config))
	require.Equal(t, []string{"staging", "database"}, order)
	require.NoError(t, ctx.StopPluginCommands())
}

func TestPluginCommandStaleFenceTokenCannotStartAcceptedRun(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	record := testRun("stale-command-fence", nil, true, now)
	require.NoError(t, ctx.CreateRun(record, testOutput(record.ID, now)))
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	oldToken := controller.dbFence
	controller.mu.Unlock()
	require.NotEmpty(t, oldToken)
	require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", "rotated-token").Error)
	won, err := ctx.MarkRunRunning(record.ID, now.Add(time.Second))
	require.Error(t, err)
	require.Contains(t, err.Error(), "stale")
	require.False(t, won)
	stored, _, err := ctx.Run(record.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusQueued, stored.Status)
}

func TestPluginCommandCanonicalJobFollowsRunUnderFencedClaim(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	record := testRun("canonical-command-lifecycle", nil, true, now)
	require.NoError(t, ctx.CreateRun(record, testOutput(record.ID, now)))
	stored, _, err := ctx.Run(record.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, record.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	won, err := ctx.MarkRunRunning(record.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	stored, _, err = ctx.Run(record.ID)
	require.NoError(t, err)
	require.Equal(t, execution.ExecutionToken, stored.JobExecutionToken)
	won, err = ctx.FinishRun(record.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded,
		FinishedAt: now.Add(2 * time.Second), OutputTail: "completed"})
	require.NoError(t, err)
	require.True(t, won)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateSucceeded, snapshot.State)
	outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, "command-history", outputs[0].Key)
}

func TestPluginCommandImportIsChildJobAndFinishesWithResourceOutput(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	actor := uint(7)
	run := testRun("import-parent-run", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	storedRun, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	claim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-child", RunID: run.ID,
		FileName: "asset.png", PluginGeneration: 1, CreatedByUserID: &actor, CreatedAt: now})
	require.NoError(t, err)
	require.True(t, claim.Enqueue)
	require.NotEmpty(t, claim.JobID)
	var link models.JobLink
	require.NoError(t, ctx.db.Where("from_job_id = ? AND to_job_id = ?", storedRun.JobID, claim.JobID).First(&link).Error)
	require.Equal(t, string(jobs.LinkParentChild), link.Type)
	execution, claimed, err := ctx.claimPluginCommandJob(claim.JobID, JobKindPluginCommandImport, claim.ImportID)
	require.NoError(t, err)
	require.True(t, claimed)
	won, err := ctx.MarkImportRunning(claim.ImportID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	var storedImport models.PluginCommandImport
	require.NoError(t, ctx.db.First(&storedImport, "id = ?", claim.ImportID).Error)
	require.Equal(t, execution.ExecutionToken, storedImport.JobExecutionToken)
	resourceID := uint(123)
	won, err = ctx.FinishImport(claim.ImportID, plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusSucceeded,
		ResourceID: &resourceID, FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, claim.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateSucceeded, snapshot.State)
	outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, claim.JobID)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, "imported-resource", outputs[0].Key)
}

func TestPluginCommandImportClaimPersistsTokenAndRetryLineageAtomically(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	actor := uint(9)
	run := testRun("import-retry-parent", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	storedRun, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	_, claimedRun, err := ctx.claimPluginCommandJob(storedRun.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimedRun)
	won, err := ctx.MarkRunRunning(run.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	first, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-first", RunID: run.ID,
		FileName: "asset.bin", FieldsJSON: `{"name":"asset"}`, PluginGeneration: 1, CreatedByUserID: &actor, CreatedAt: now})
	require.NoError(t, err)
	firstExecution, claimed, err := ctx.claimPluginCommandJob(first.JobID, JobKindPluginCommandImport, first.ImportID)
	require.NoError(t, err)
	require.True(t, claimed)
	var beforeStart models.PluginCommandImport
	require.NoError(t, ctx.db.First(&beforeStart, "id = ?", first.ImportID).Error)
	require.Equal(t, plugin_commands.ImportStatusPending, beforeStart.Status)
	require.Equal(t, firstExecution.ExecutionToken, beforeStart.JobExecutionToken,
		"a crash before MarkImportRunning must leave a recoverable queued source with its canonical token")
	won, err = ctx.MarkImportRunning(first.ImportID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishImport(first.ImportID, plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusFailed,
		Error: "import failed", FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	commands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, first.JobID)
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.True(t, keys["inspect"])
	require.True(t, keys["retry-import"], "retry requires the durable failed source, fields and successful parent run")
	second, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-second", RunID: run.ID,
		FileName: "asset.bin", FieldsJSON: `{"name":"asset"}`, PluginGeneration: 2, CreatedByUserID: &actor, CreatedAt: now.Add(3 * time.Second)})
	require.NoError(t, err)
	require.True(t, second.Enqueue)
	require.NotEqual(t, first.JobID, second.JobID)
	var retry models.JobLink
	require.NoError(t, ctx.db.Where("from_job_id = ? AND to_job_id = ? AND type = ?", second.JobID, first.JobID, string(jobs.LinkRetryOf)).First(&retry).Error)
	var prior, successor models.PluginCommandImport
	require.NoError(t, ctx.db.First(&prior, "id = ?", first.ImportID).Error)
	require.NoError(t, ctx.db.First(&successor, "id = ?", second.ImportID).Error)
	require.Equal(t, first.JobID, prior.JobID)
	require.Equal(t, second.JobID, successor.JobID)
}

func TestPluginCommandRunClaimPersistsTokenBeforeRunnerStart(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("claim-before-fork", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	stored, _, err = ctx.Run(run.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusQueued, stored.Status)
	require.Equal(t, execution.ExecutionToken, stored.JobExecutionToken,
		"a crash before MarkRunRunning must leave a recoverable queued source with its canonical token")
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.True(t, keys[jobs.CommandCancel])
	require.True(t, keys["inspect"])
	require.False(t, keys[jobs.CommandRetry])
	require.False(t, keys[jobs.CommandRepeat])
}

func TestPluginCommandQueuedCanonicalCancelAlsoCancelsSource(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("canonical-queued-cancel", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	_, err = service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: stored.JobID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-queued-command",
		ExpectedVersion: uint64(snapshot.Version), Actor: jobs.Access{Administrator: true},
	})
	require.NoError(t, err)
	stored, _, err = ctx.Run(run.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusCancelled, stored.Status)
	require.True(t, stored.CancelRequested)
	snapshot, err = service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateCancelled, snapshot.State)
}

func TestPluginCommandKindsAreAdminOnlyAcrossJobReadSeams(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	now := time.Now().UTC()
	actor := uint(42)
	run := testRun("admin-only-command", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	command, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	importClaim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "admin-only-import", RunID: run.ID,
		FileName: "file.bin", FieldsJSON: `{"name":"file"}`, CreatedByUserID: &actor, CreatedAt: now})
	require.NoError(t, err)
	viewer := jobs.Access{UserID: actor}
	admin := jobs.Access{Administrator: true}
	for _, jobID := range []string{command.JobID, importClaim.JobID} {
		if _, err := service.Get(ctx.jobDeps(), viewer, jobID); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Get(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Timeline(ctx.jobDeps(), viewer, jobID, 0, 20); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Timeline(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Outputs(ctx.jobDeps(), viewer, jobID); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Outputs(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Get(ctx.jobDeps(), admin, jobID); err != nil {
			t.Fatalf("administrator Get(%s): %v", jobID, err)
		}
	}
	page, err := service.List(ctx.jobDeps(), viewer, jobs.Filter{}, jobs.Cursor{}, 20)
	require.NoError(t, err)
	require.Empty(t, page.Jobs)
	adminPage, err := service.List(ctx.jobDeps(), admin, jobs.Filter{}, jobs.Cursor{}, 20)
	require.NoError(t, err)
	require.Len(t, adminPage.Jobs, 2)
	viewerSummary, err := service.Summary(ctx.jobDeps(), viewer, jobs.Filter{}, time.Hour)
	require.NoError(t, err)
	require.Zero(t, viewerSummary.Total)
	adminSummary, err := service.Summary(ctx.jobDeps(), admin, jobs.Filter{}, time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 2, adminSummary.Total)
}

func TestPluginCommandHeartbeatKeepsLongRunningCanonicalClaimAlive(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("long-running-command", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, ctx.db.Model(&models.JobClaim{}).Where("job_id = ?", execution.JobID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Minute)).Error)
	require.NoError(t, ctx.heartbeatPluginCommandJob(execution))
	var claim models.JobClaim
	require.NoError(t, ctx.db.First(&claim, "job_id = ?", execution.JobID).Error)
	require.Equal(t, models.JobClaimStateHeld, claim.State)
	require.True(t, claim.LeaseExpiresAt.After(time.Now().Add(time.Minute)), "lease expires at %s", claim.LeaseExpiresAt)
	// The same callback cannot keep a replacement controller's claim alive.
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	oldToken := controller.dbFence
	controller.mu.Unlock()
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.JobRuntimeFence{}).Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", "new-owner").Error
	}))
	require.Error(t, ctx.heartbeatPluginCommandJob(execution))
	require.NotEmpty(t, oldToken)
}
