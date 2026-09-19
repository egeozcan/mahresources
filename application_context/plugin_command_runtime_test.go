package application_context

import (
	"context"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_commands"
)

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
