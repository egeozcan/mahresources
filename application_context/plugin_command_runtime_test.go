package application_context

import (
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/plugin_commands"
)

func installPluginCommandActiveForTest(ctx *MahresourcesContext, dispatcher *plugin_commands.Dispatcher, exchange plugin_commands.Exchange, lease *plugin_commands.RuntimeLease) {
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	controller.owner = ctx
	controller.active.Store(&pluginCommandActiveRuntime{dispatcher: dispatcher, exchange: exchange})
	controller.lease = lease
	controller.state = pluginCommandRuntimeActive
	controller.mu.Unlock()
}

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
	installPluginCommandActiveForTest(ctx, dispatcher, plugin_commands.NewExchange(ctx, settings), nil)
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

func TestPluginCommandHostSetsThumbnailFromExchangeFile(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.Config.AuthEnabled = false
	root, err := ctx.EnsureRootAdmin()
	require.NoError(t, err)
	require.NoError(t, ctx.db.AutoMigrate(&models.Preview{}))

	target := &models.Resource{Name: "video.mp4", ContentType: "video/mp4"}
	require.NoError(t, ctx.db.Create(target).Error)

	settings := commandDisableSettings{root: t.TempDir()}
	now := time.Now().UTC()
	finished := now.Add(time.Second)
	run := testRun("thumbnail-run", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	won, err := ctx.MarkRunRunning(run.ID, now)
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: finished})
	require.NoError(t, err)
	require.True(t, won)

	runDir := filepath.Join(settings.root, "plugin_exchange", run.PluginName, run.ID)
	require.NoError(t, os.MkdirAll(runDir, 0o700))
	cover := makeColorJPEG(t, 32, 20, color.RGBA{B: 255, A: 255})
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "cover.jpg"), cover, 0o600))
	installPluginCommandActiveForTest(ctx, nil, plugin_commands.NewExchange(ctx, settings), nil)

	err = ctx.SetCommandResourceThumbnail(context.Background(), plugin_commands.Access{PluginName: run.PluginName}, run.ID, "cover.jpg", target.ID)
	require.NoError(t, err)

	var preview models.Preview
	require.NoError(t, ctx.db.Where("resource_id = ? AND width = 0 AND height = 0", target.ID).First(&preview).Error)
	require.True(t, colorClose(sampleCenterColor(t, preview.Data), color.RGBA{B: 255, A: 255}, 12))
	_, statErr := os.Stat(filepath.Join(runDir, "cover.jpg"))
	require.NoError(t, statErr, "setting a thumbnail must leave the exchange source intact")

	var stored models.Resource
	require.NoError(t, ctx.db.First(&stored, target.ID).Error)
	require.NotNil(t, stored.CreatedByUserId)
	require.Equal(t, root.ID, *stored.CreatedByUserId)
}

func TestPluginCommandHostThumbnailEnforcesRoleAndScope(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.Preview{}))

	inside := &models.Group{Name: "inside"}
	outside := &models.Group{Name: "outside"}
	require.NoError(t, ctx.db.Create(inside).Error)
	require.NoError(t, ctx.db.Create(outside).Error)
	guest := &models.User{Username: "thumbnail-guest", Role: models.RoleGuest, ScopeGroupId: &inside.ID}
	user := &models.User{Username: "thumbnail-user", Role: models.RoleUser, ScopeGroupId: &inside.ID}
	require.NoError(t, ctx.db.Create(guest).Error)
	require.NoError(t, ctx.db.Create(user).Error)
	insideResource := &models.Resource{Name: "inside.mp4", OwnerId: &inside.ID}
	outsideResource := &models.Resource{Name: "outside.mp4", OwnerId: &outside.ID}
	require.NoError(t, ctx.db.Create(insideResource).Error)
	require.NoError(t, ctx.db.Create(outsideResource).Error)

	settings := commandDisableSettings{root: t.TempDir()}
	now := time.Now().UTC()
	seedRun := func(id string, actor uint) {
		t.Helper()
		finished := now.Add(time.Second)
		run := testRun(id, &actor, false, now)
		require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
		won, err := ctx.MarkRunRunning(run.ID, now)
		require.NoError(t, err)
		require.True(t, won)
		won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: finished})
		require.NoError(t, err)
		require.True(t, won)
		runDir := filepath.Join(settings.root, "plugin_exchange", run.PluginName, run.ID)
		require.NoError(t, os.MkdirAll(runDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(runDir, "cover.jpg"), makeColorJPEG(t, 8, 8, color.RGBA{G: 255, A: 255}), 0o600))
	}
	seedRun("guest-thumbnail", guest.ID)
	seedRun("scoped-thumbnail", user.ID)
	installPluginCommandActiveForTest(ctx, nil, plugin_commands.NewExchange(ctx, settings), nil)

	err := ctx.SetCommandResourceThumbnail(context.Background(), plugin_commands.Access{PluginName: "worker", ActorUserID: &guest.ID}, "guest-thumbnail", "cover.jpg", insideResource.ID)
	require.ErrorIs(t, err, ErrRoleCapability)

	err = ctx.SetCommandResourceThumbnail(context.Background(), plugin_commands.Access{PluginName: "worker", ActorUserID: &user.ID}, "scoped-thumbnail", "cover.jpg", outsideResource.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "outside-scope error = %v", err)
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
	active, err := ctx.pluginCommandActive()
	if err != nil || active.dispatcher == nil || active.exchange == nil {
		cancel()
		t.Fatalf("recovered command host was not published: %v", err)
	}
	clone := ctx.WithPrincipal(nil)
	cloneActive, err := clone.pluginCommandActive()
	if err != nil || cloneActive != active {
		cancel()
		t.Fatalf("request clone did not preserve process-lifetime command host: %v", err)
	}

	cancel()
	require.NoError(t, ctx.StopPluginCommands())
}
