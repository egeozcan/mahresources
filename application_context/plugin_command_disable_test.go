package application_context

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_commands"
)

type commandDisableSettings struct{ root string }

func (s commandDisableSettings) StagingRoot() string            { return s.root }
func (commandDisableSettings) PendingPerPluginLimit() int       { return 100 }
func (commandDisableSettings) PerRunQuota() int64               { return 1 << 30 }
func (commandDisableSettings) GlobalStagingQuota() int64        { return 1 << 31 }
func (commandDisableSettings) ExchangeRetention() time.Duration { return time.Hour }
func (commandDisableSettings) OutputRetention() time.Duration   { return time.Hour }
func (commandDisableSettings) CommandPath() string              { return "/bin" }

type commandDisableJobs struct{}

func (commandDisableJobs) SubmitCommandJob(plugin_commands.RunJobSpec, func(string) error, func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	return "", errors.New("unexpected command dispatch")
}
func (commandDisableJobs) SubmitImportJob(plugin_commands.ImportJobSpec, func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	return "", errors.New("unexpected import dispatch")
}

type commandDisableExecutor struct{}

func (commandDisableExecutor) Execute(context.Context, plugin_commands.QueuedRun) plugin_commands.Outcome {
	return plugin_commands.Outcome{Status: plugin_commands.RunStatusFailed, Error: "unexpected execution"}
}

type failingCommandDisableStore struct {
	plugin_commands.Store
	err error
}

type blockingCommandDisableStore struct {
	plugin_commands.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingCommandDisableStore) NonterminalRuns() ([]plugin_commands.RunRecord, error) {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	return s.Store.NonterminalRuns()
}

func (s failingCommandDisableStore) RequestRunCancel(string, string) error { return s.err }

func commandDisableContext(t *testing.T, store func(*MahresourcesContext) plugin_commands.Store) (*MahresourcesContext, string) {
	t.Helper()
	pluginDir := t.TempDir()
	const pluginName = "command-owner"
	writeConsentTestPlugin(t, pluginDir, pluginName, `plugin = { name = "command-owner", version = "1.0", api_version = 1, capabilities = {} }
function init() end
`)
	ctx := createTestContextWithPlugins(t, pluginDir)
	if err := ctx.db.AutoMigrate(
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetPluginEnabled(pluginName, true); err != nil {
		t.Fatal(err)
	}
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: "disable-run", PluginName: pluginName, CommandName: "tool", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, ActorlessAtSubmission: true, CreatedAt: time.Now().UTC(),
	}, plugin_commands.RunOutput{RunID: "disable-run", ArgvJSON: `[]`, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: store(ctx), Jobs: commandDisableJobs{}, Executor: commandDisableExecutor{},
		Settings: commandDisableSettings{root: t.TempDir()},
	})
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx.SetPluginCommandDispatcher(dispatcher)
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(stopCtx)
		ctx.PluginManager().Close()
	})
	return ctx, pluginName
}

func TestSetPluginDisabledRevokesDurableCommandAfterVMRemoval(t *testing.T) {
	ctx, pluginName := commandDisableContext(t, func(ctx *MahresourcesContext) plugin_commands.Store { return ctx })
	if err := ctx.SetPluginEnabled(pluginName, false); err != nil {
		t.Fatal(err)
	}
	if ctx.PluginManager().IsEnabled(pluginName) {
		t.Fatal("plugin VM remained enabled")
	}
	run, _, err := ctx.Run("disable-run")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != plugin_commands.RunStatusCancelled || run.Error != "plugin disabled" || !run.CancelRequested {
		t.Fatalf("run = %+v", run)
	}
}

func TestSetPluginDisabledClosesAdmissionBeforeDurableDrain(t *testing.T) {
	pluginDir := t.TempDir()
	const pluginName = "command-race"
	writeConsentTestPlugin(t, pluginDir, pluginName, `plugin = {
  name = "command-race", version = "1.0", api_version = 1,
  capabilities = {"commands", "inject"},
  commands = {{name="convert", argv={"tool", "{{input}}"}}}
}
function init()
  mah.inject("page_bottom", function()
    local id, err = mah.commands.run("convert", {input="late"})
    return id or err
  end)
end
`)
	ctx := createTestContextWithPlugins(t, pluginDir)
	if err := ctx.db.AutoMigrate(
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetPluginEnabledWithOptions(pluginName, true, PluginEnableOptions{ConfirmCommands: true}); err != nil {
		t.Fatal(err)
	}

	settings := commandDisableSettings{root: t.TempDir()}
	store := &blockingCommandDisableStore{Store: ctx, entered: make(chan struct{}), release: make(chan struct{})}
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: store, Jobs: commandDisableJobs{}, Executor: commandDisableExecutor{}, Settings: settings,
	})
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx.SetPluginCommandDispatcher(dispatcher)
	ctx.pluginCommandExchange = plugin_commands.NewExchange(ctx, settings)
	ctx.PluginManager().SetCommandSubmitter(ctx)
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(stopCtx)
		ctx.PluginManager().Close()
	})

	disabled := make(chan error, 1)
	go func() { disabled <- ctx.SetPluginEnabled(pluginName, false) }()
	select {
	case <-store.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("disable did not reach the durable drain")
	}

	got := ctx.PluginManager().RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	if !strings.Contains(got, "generation is no longer active") {
		close(store.release)
		<-disabled
		t.Fatalf("command admission remained open during disable drain: %q", got)
	}
	var count int64
	if err := ctx.db.Model(&models.PluginCommandRun{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		close(store.release)
		<-disabled
		t.Fatalf("late submission created %d durable runs", count)
	}
	close(store.release)
	if err := <-disabled; err != nil {
		t.Fatal(err)
	}
}

func TestSetPluginDisabledReturnsCommandRevocationFailure(t *testing.T) {
	injected := errors.New("cancel store unavailable")
	ctx, pluginName := commandDisableContext(t, func(ctx *MahresourcesContext) plugin_commands.Store {
		return failingCommandDisableStore{Store: ctx, err: injected}
	})
	err := ctx.SetPluginEnabled(pluginName, false)
	if !errors.Is(err, injected) {
		t.Fatalf("disable error = %v, want %v", err, injected)
	}
	if ctx.PluginManager().IsEnabled(pluginName) {
		t.Fatal("plugin VM remained enabled after command-revocation error")
	}
	run, _, readErr := ctx.Run("disable-run")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if run.Status != plugin_commands.RunStatusQueued {
		t.Fatalf("failed revocation unexpectedly changed run: %+v", run)
	}
}
