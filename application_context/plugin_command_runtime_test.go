package application_context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestManagedCommandOutcomeSeparatesGenericAndDurableStatus(t *testing.T) {
	unknown := managedCommandOutcome(plugin_commands.Outcome{
		Status: plugin_commands.RunStatusFailed,
		Error:  "durable write and reread failed",
	})
	require.Equal(t, "failed", string(unknown.Status))
	require.Empty(t, unknown.AuthoritativeStatus)

	confirmed := managedCommandOutcome(plugin_commands.Outcome{
		Status:              plugin_commands.RunStatusFailed,
		AuthoritativeStatus: plugin_commands.RunStatusRunning,
		Error:               "terminal write failed",
	})
	require.Equal(t, "failed", string(confirmed.Status))
	require.Equal(t, plugin_commands.RunStatusRunning, confirmed.AuthoritativeStatus)
}

func TestPluginCommandHostResolvesAuthOffRootBeforeImportClaim(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.Config.AuthEnabled = false
	root, err := ctx.EnsureRootAdmin()
	require.NoError(t, err)

	settings := commandDisableSettings{root: t.TempDir()}
	now := time.Now().UTC()
	finished := now.Add(time.Second)
	run := testRun("auth-off-import-run", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	won, err := ctx.MarkRunRunning(run.ID, now)
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusSucceeded, FinishedAt: finished,
	})
	require.NoError(t, err)
	require.True(t, won)
	run.Status = plugin_commands.RunStatusSucceeded
	run.FinishedAt = &finished
	runDir := filepath.Join(settings.root, "plugin_exchange", run.PluginName, run.ID)
	require.NoError(t, os.MkdirAll(runDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "out.bin"), []byte("auth-off payload"), 0o600))

	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: ctx, Jobs: commandDisableJobs{}, Executor: commandDisableExecutor{},
		Settings: settings, Leases: plugin_commands.NewLeaseManager(),
	})
	dispatcher.SetImporter(ctx)
	require.NoError(t, dispatcher.Start(context.Background()))
	ctx.pluginCommandDispatcher = dispatcher
	ctx.pluginCommandExchange = plugin_commands.NewExchange(ctx, settings)
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(stopCtx)
	})

	result, err := ctx.SubmitCommandImport(plugin_commands.ImportSubmission{
		Access: plugin_commands.Access{PluginName: run.PluginName},
		RunID:  run.ID, Name: "out.bin", PluginGeneration: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.ImportID)

	var claim models.PluginCommandImport
	require.NoError(t, ctx.db.Where("id = ?", result.ImportID).First(&claim).Error)
	require.NotNil(t, claim.CreatedByUserId)
	require.Equal(t, root.ID, *claim.CreatedByUserId)
}

func TestPluginCommandHostPublishesOnlyAfterRecoveryAndSurvivesContextClone(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	t.Cleanup(ctx.PluginManager().Close)
	if err := ctx.db.AutoMigrate(&models.PluginCommandRun{}, &models.PluginCommandRunOutput{}, &models.PluginCommandImport{}, &models.PluginCommandImportMap{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.SubmitPluginCommand(plugin_commands.CommandRequest{}); err == nil {
		t.Fatal("command host was available before recovery")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	settings := commandDisableSettings{root: t.TempDir()}
	if err := ctx.StartPluginCommands(runCtx, settings); err != nil {
		cancel()
		t.Fatal(err)
	}
	if ctx.pluginCommandDispatcher == nil || ctx.pluginCommandExchange == nil {
		cancel()
		t.Fatal("recovered command host was not published")
	}
	clone := ctx.WithPrincipal(nil)
	if clone.pluginCommandDispatcher != ctx.pluginCommandDispatcher || clone.pluginCommandExchange != ctx.pluginCommandExchange {
		cancel()
		t.Fatal("request clone did not preserve process-lifetime command host")
	}

	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	_ = ctx.pluginCommandDispatcher.Stop(stopCtx)
}
