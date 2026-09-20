package application_context

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestPluginCommandControllerIsSharedByClones(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(),
	}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	require.Same(t, ctx.pluginCommandController, ctx.WithPrincipal(nil).pluginCommandController)
	require.NoError(t, ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		require.Same(t, ctx.pluginCommandController, txCtx.pluginCommandController)
		return nil
	}))
}

func TestPluginCommandControllerLeaseContentionHeals(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	first, err := plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, err)

	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireBackoff = []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}
	var mu sync.Mutex
	var attempts []time.Time
	originalAcquire := cfg.acquireLease
	cfg.acquireLease = func(root string) (*plugin_commands.RuntimeLease, error) {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()
		return originalAcquire(root)
	}
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
	_, activeErr := ctx.pluginCommandActive()
	require.ErrorIs(t, activeErr, plugin_commands.ErrCommandRuntimeQuarantined)
	require.NoError(t, first.Close())
	require.Eventually(t, func() bool {
		_, err := ctx.pluginCommandActive()
		return err == nil
	}, time.Second, 5*time.Millisecond)
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	mu.Lock()
	require.GreaterOrEqual(t, len(attempts), 2)
	mu.Unlock()

	var logs []models.LogEntry
	require.NoError(t, ctx.db.Where("entity_type = ?", "plugin_command").Order("id asc").Find(&logs).Error)
	require.GreaterOrEqual(t, len(logs), 2)
	require.Equal(t, models.LogLevelWarning, logs[0].Level)
	require.Contains(t, logs[0].Message, root)
	require.Contains(t, logs[0].Message, "automatic retry")
	require.Contains(t, logs[0].Message, "-plugins-disabled")
	require.Equal(t, models.LogLevelInfo, logs[len(logs)-1].Level)
}

type controllerRecoveryInspector struct {
	mu    sync.Mutex
	state plugin_commands.GroupState
}

func (i *controllerRecoveryInspector) InspectGroup(int, string) (plugin_commands.GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return plugin_commands.GroupIdentity{State: i.state}, nil
}

func (*controllerRecoveryInspector) KillGroup(int) error { return nil }

func (i *controllerRecoveryInspector) set(state plugin_commands.GroupState) {
	i.mu.Lock()
	i.state = state
	i.mu.Unlock()
}

func TestPluginCommandControllerRecoveryQuarantineHeals(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	now := time.Now().UTC()
	owner := uint(7)
	runID := "controller-recovery-blocked"
	require.NoError(t, ctx.CreateRun(plugin_commands.RunRecord{
		ID: runID, PluginName: "controller", CommandName: "tool", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: runID, ArgvJSON: `[]`, CreatedAt: now}))
	won, err := ctx.MarkRunRunning(runID, now)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, ctx.SetRunProcessGroup(runID, 4242, "same-boot"))

	inspector := &controllerRecoveryInspector{state: plugin_commands.GroupAliveUnverified}
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.recoveryInterval = 5 * time.Millisecond
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}, cfg))
	_, activeErr := ctx.pluginCommandActive()
	require.ErrorIs(t, activeErr, plugin_commands.ErrCommandRuntimeQuarantined)
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	inspector.set(plugin_commands.GroupDead)
	require.Eventually(t, func() bool {
		_, err := ctx.pluginCommandActive()
		return err == nil
	}, time.Second, 5*time.Millisecond)
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	run, _, err := ctx.Run(runID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusInterrupted, run.Status)
}

func TestPluginCommandControllerStopBeforeActivation(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	first, err := plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, err)

	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireBackoff = []time.Duration{5 * time.Millisecond}
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
	require.NoError(t, ctx.StopPluginCommands())
	require.NoError(t, first.Close())
	time.Sleep(25 * time.Millisecond)
	_, err = ctx.pluginCommandActive()
	require.ErrorIs(t, err, plugin_commands.ErrCommandRuntimeQuarantined)
}

func TestPluginCommandControllerNonBusyLeaseErrorIsFatal(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireLease = func(string) (*plugin_commands.RuntimeLease, error) {
		return nil, errors.New("permission denied")
	}
	err := ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(),
	}, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "-plugins-disabled")
}
