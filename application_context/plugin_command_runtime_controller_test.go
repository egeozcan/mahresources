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

func requireGenericCommandQuarantineError(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	require.ErrorIs(t, err, plugin_commands.ErrCommandRuntimeQuarantined)
	require.Contains(t, err.Error(), "automatic recovery")
	require.Contains(t, err.Error(), "/logs")
	for _, value := range forbidden {
		require.NotContains(t, err.Error(), value)
	}
}

func TestPluginCommandControllerLeaseContentionHeals(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	first, err := plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = first.Close()
		_ = ctx.StopPluginCommands()
	})

	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireBackoff = []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}
	attempted := make(chan int, 3)
	var mu sync.Mutex
	var attempts []time.Time
	originalAcquire := cfg.acquireLease
	cfg.acquireLease = func(root string) (*plugin_commands.RuntimeLease, error) {
		mu.Lock()
		attempt := len(attempts)
		attempts = append(attempts, time.Now())
		mu.Unlock()
		lease, acquireErr := originalAcquire(root)
		attempted <- attempt
		return lease, acquireErr
	}
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
	require.Equal(t, 0, <-attempted)
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr, root, "leased by another runtime", "-plugins-disabled")

	select {
	case attempt := <-attempted:
		require.Equal(t, 1, attempt)
	case <-time.After(time.Second):
		t.Fatal("first lease retry did not run")
	}
	require.NoError(t, first.Close())
	select {
	case attempt := <-attempted:
		require.Equal(t, 2, attempt)
	case <-time.After(time.Second):
		t.Fatal("second lease retry did not run")
	}
	require.Eventually(t, func() bool {
		_, err := ctx.pluginCommandActive()
		return err == nil
	}, time.Second, time.Millisecond)

	mu.Lock()
	require.Len(t, attempts, 3)
	firstDelay := attempts[1].Sub(attempts[0])
	secondDelay := attempts[2].Sub(attempts[1])
	mu.Unlock()
	require.GreaterOrEqual(t, firstDelay, 5*time.Millisecond)
	require.Less(t, firstDelay, time.Second)
	require.GreaterOrEqual(t, secondDelay, 10*time.Millisecond)
	require.Less(t, secondDelay, time.Second)

	var logs []models.LogEntry
	require.NoError(t, ctx.db.Where("entity_type = ?", "plugin_command").Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Equal(t, models.LogLevelWarning, logs[0].Level)
	require.Contains(t, logs[0].Message, root)
	require.Contains(t, logs[0].Message, "automatic retry")
	require.Contains(t, logs[0].Message, "-plugins-disabled")
	require.Equal(t, models.LogLevelInfo, logs[1].Level)
	require.Contains(t, logs[1].Message, root)
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
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr, runID, "4242", "kill -KILL")
	_, hostErr := ctx.CommandRuns(plugin_commands.Access{PluginName: "controller"})
	requireGenericCommandQuarantineError(t, hostErr, runID, "4242", "kill -KILL")
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	inspector.set(plugin_commands.GroupDead)
	require.Eventually(t, func() bool {
		_, err := ctx.pluginCommandActive()
		return err == nil
	}, time.Second, 5*time.Millisecond)
	run, _, err := ctx.Run(runID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusInterrupted, run.Status)
}

func TestPluginCommandControllerStopDuringAcquireCannotPublish(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	acquireEntered := make(chan struct{})
	allowAcquire := make(chan struct{})
	cfg := defaultPluginCommandControllerConfig()
	cfg.beforeAcquire = func(int) {
		close(acquireEntered)
		<-allowAcquire
	}
	started := make(chan error, 1)
	go func() { started <- ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg) }()
	<-acquireEntered
	stopped := make(chan error, 1)
	go func() { stopped <- ctx.StopPluginCommands() }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	close(allowAcquire)
	require.NoError(t, <-started)
	require.NoError(t, <-stopped)
	_, err := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, err)
	_, err = ctx.SubmitPluginCommand(plugin_commands.CommandRequest{})
	requireGenericCommandQuarantineError(t, err)
	_, err = ctx.CommandRuns(plugin_commands.Access{})
	requireGenericCommandQuarantineError(t, err)
	lease, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, leaseErr)
	require.NoError(t, lease.Close())
}

func TestPluginCommandControllerStopDuringPublishCannotPublish(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	publishEntered := make(chan struct{})
	allowPublish := make(chan struct{})
	cfg := defaultPluginCommandControllerConfig()
	cfg.beforePublish = func() {
		close(publishEntered)
		<-allowPublish
	}
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	started := make(chan error, 1)
	go func() {
		started <- ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg)
	}()
	<-publishEntered
	stopped := make(chan error, 1)
	go func() { stopped <- ctx.StopPluginCommands() }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	close(allowPublish)
	require.NoError(t, <-started)
	require.NoError(t, <-stopped)
	_, err := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, err)
	_, err = ctx.SubmitPluginCommand(plugin_commands.CommandRequest{})
	requireGenericCommandQuarantineError(t, err)
	_, err = ctx.CommandRuns(plugin_commands.Access{})
	requireGenericCommandQuarantineError(t, err)
	lease, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, leaseErr)
	require.NoError(t, lease.Close())
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
