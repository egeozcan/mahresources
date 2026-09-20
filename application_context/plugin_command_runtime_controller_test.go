package application_context

import (
	"context"
	"encoding/json"
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
	mu     sync.Mutex
	state  plugin_commands.GroupState
	states map[int]plugin_commands.GroupState
	errs   map[int]error
}

func (i *controllerRecoveryInspector) InspectGroup(pgid int, _ string) (plugin_commands.GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.errs[pgid]; err != nil {
		return plugin_commands.GroupIdentity{}, err
	}
	state := i.state
	if configured, ok := i.states[pgid]; ok {
		state = configured
	}
	return plugin_commands.GroupIdentity{State: state}, nil
}

func (*controllerRecoveryInspector) KillGroup(int) error { return nil }

func (i *controllerRecoveryInspector) set(state plugin_commands.GroupState) {
	i.mu.Lock()
	i.state = state
	i.mu.Unlock()
}

func (i *controllerRecoveryInspector) setGroup(pgid int, state plugin_commands.GroupState) {
	i.mu.Lock()
	if i.states == nil {
		i.states = make(map[int]plugin_commands.GroupState)
	}
	i.states[pgid] = state
	delete(i.errs, pgid)
	i.mu.Unlock()
}

func createControllerRecoveryRun(t *testing.T, ctx *MahresourcesContext, runID string, pgid int) {
	t.Helper()
	now := time.Now().UTC()
	owner := uint(7)
	require.NoError(t, ctx.CreateRun(plugin_commands.RunRecord{
		ID: runID, PluginName: "controller", CommandName: "tool", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: runID, ArgvJSON: `[]`, CreatedAt: now}))
	won, err := ctx.MarkRunRunning(runID, now)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, ctx.SetRunProcessGroup(runID, pgid, "same-boot"))
}

func TestPluginCommandControllerRecoveryQuarantineHeals(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	createControllerRecoveryRun(t, ctx, "blocked-one", 4241)
	createControllerRecoveryRun(t, ctx, "blocked-two", 4242)
	createControllerRecoveryRun(t, ctx, "already-dead", 4243)

	inspector := &controllerRecoveryInspector{states: map[int]plugin_commands.GroupState{
		4241: plugin_commands.GroupAliveUnverified,
		4242: plugin_commands.GroupAliveUnverified,
		4243: plugin_commands.GroupDead,
	}}
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.recoveryInterval = 20 * time.Millisecond
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}, cfg))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	ctx.pluginCommandController.mu.Lock()
	pending := ctx.pluginCommandController.pending
	ctx.pluginCommandController.mu.Unlock()
	require.NotNil(t, pending)
	for _, runID := range []string{"blocked-one", "blocked-two"} {
		run, _, err := ctx.Run(runID)
		require.NoError(t, err)
		require.Equal(t, plugin_commands.RunStatusRunning, run.Status)
	}
	dead, _, err := ctx.Run("already-dead")
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusInterrupted, dead.Status)

	quarantinedCalls := []func() error{
		func() error { _, err := ctx.SubmitPluginCommand(plugin_commands.CommandRequest{}); return err },
		func() error { _, err := ctx.CommandRuns(plugin_commands.Access{}); return err },
		func() error { _, err := ctx.ListCommandFiles(plugin_commands.Access{}, "blocked-one"); return err },
		func() error {
			_, err := ctx.ReadCommandFile(plugin_commands.Access{}, "blocked-one", "out", 1)
			return err
		},
		func() error { _, err := ctx.SubmitCommandImport(plugin_commands.ImportSubmission{}); return err },
		func() error { return ctx.DiscardCommandFile(plugin_commands.Access{}, "blocked-one", "out") },
		func() error { return ctx.DiscardCommandRun(plugin_commands.Access{}, "blocked-one") },
	}
	for _, call := range quarantinedCalls {
		requireGenericCommandQuarantineError(t, call(), "blocked-one", "blocked-two", "4241", "4242", "kill -KILL")
	}
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	inspector.setGroup(4241, plugin_commands.GroupDead)
	inspector.setGroup(4242, plugin_commands.GroupDead)
	require.Eventually(t, func() bool {
		_, err := ctx.pluginCommandActive()
		return err == nil
	}, time.Second, 5*time.Millisecond)
	active, err := ctx.pluginCommandActive()
	require.NoError(t, err)
	require.Same(t, pending, active.dispatcher)
	_, err = ctx.CommandRuns(plugin_commands.Access{PluginName: "controller"})
	require.NoError(t, err)
	for _, runID := range []string{"blocked-one", "blocked-two"} {
		run, _, err := ctx.Run(runID)
		require.NoError(t, err)
		require.Equal(t, plugin_commands.RunStatusInterrupted, run.Status)
	}
	second, leaseErr = plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)
}

func TestPluginCommandControllerRecoveryQuarantineLogsRetryFailureOnceAndHealing(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	createControllerRecoveryRun(t, ctx, "log-blocked-one", 5251)
	createControllerRecoveryRun(t, ctx, "log-blocked-two", 5252)
	inspector := &controllerRecoveryInspector{
		states: map[int]plugin_commands.GroupState{5251: plugin_commands.GroupAliveUnverified},
		errs:   map[int]error{5252: errors.New("ownership probe denied")},
	}
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.recoveryInterval = 10 * time.Millisecond
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}, cfg))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	var initial models.LogEntry
	require.NoError(t, ctx.db.Where("entity_type = ?", "plugin_command").First(&initial).Error)
	require.Equal(t, models.LogLevelWarning, initial.Level)
	require.Equal(t, models.LogActionSystem, initial.Action)
	for _, value := range []string{"log-blocked-one", "5251", "log-blocked-two", "5252", "ownership probe denied", "/logs", "kill -KILL -- -<pgid>", "-plugins-disabled"} {
		require.Contains(t, initial.Message, value)
	}
	var details struct {
		Blockers []struct {
			RunID          string `json:"run_id"`
			ProcessGroupID int    `json:"process_group_id"`
			Reason         string `json:"reason"`
		} `json:"blockers"`
	}
	require.NoError(t, json.Unmarshal(initial.Details, &details))
	require.Len(t, details.Blockers, 2)
	require.Equal(t, "log-blocked-one", details.Blockers[0].RunID)
	require.Equal(t, 5251, details.Blockers[0].ProcessGroupID)
	require.Contains(t, details.Blockers[0].Reason, "ownership could not be verified")
	require.Equal(t, "log-blocked-two", details.Blockers[1].RunID)
	require.Equal(t, 5252, details.Blockers[1].ProcessGroupID)
	require.Contains(t, details.Blockers[1].Reason, "ownership probe denied")

	require.NoError(t, ctx.db.Migrator().DropTable(&models.PluginCommandImport{}))
	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, ctx.db.Model(&models.LogEntry{}).
			Where("entity_type = ? AND level = ?", "plugin_command", models.LogLevelWarning).Count(&count).Error)
		return count == 2
	}, time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	var warningCount int64
	require.NoError(t, ctx.db.Model(&models.LogEntry{}).
		Where("entity_type = ? AND level = ?", "plugin_command", models.LogLevelWarning).Count(&warningCount).Error)
	require.Equal(t, int64(2), warningCount, "identical retry failures must be deduplicated")
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr)
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	require.NoError(t, ctx.db.AutoMigrate(&models.PluginCommandImport{}))
	inspector.setGroup(5251, plugin_commands.GroupDead)
	inspector.setGroup(5252, plugin_commands.GroupDead)
	require.Eventually(t, func() bool {
		var infoCount int64
		require.NoError(t, ctx.db.Model(&models.LogEntry{}).
			Where("entity_type = ? AND level = ?", "plugin_command", models.LogLevelInfo).Count(&infoCount).Error)
		return infoCount == 1
	}, time.Second, 5*time.Millisecond)
	_, activeErr = ctx.pluginCommandActive()
	require.NoError(t, activeErr)
	time.Sleep(50 * time.Millisecond)
	var infoCount int64
	require.NoError(t, ctx.db.Model(&models.LogEntry{}).
		Where("entity_type = ? AND level = ?", "plugin_command", models.LogLevelInfo).Count(&infoCount).Error)
	require.Equal(t, int64(1), infoCount, "healing must emit exactly one information log")
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

func TestPluginCommandControllerStopDuringPrePublicationDrainRetainsLease(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	executor := &lifecycleStubbornExecutor{started: make(chan struct{}), release: make(chan struct{})}
	publishEntered := make(chan struct{})
	allowPublish := make(chan struct{})
	cfg := defaultPluginCommandControllerConfig()
	cfg.buildRuntime = func() (*pluginCommandActiveRuntime, error) {
		return &pluginCommandActiveRuntime{dispatcher: plugin_commands.NewDispatcher(plugin_commands.Dependencies{
			Store: ctx, Jobs: lifecycleAsyncJobs{}, Executor: executor, Settings: settings,
		})}, nil
	}
	cfg.beforePublish = func() {
		close(publishEntered)
		<-allowPublish
	}

	started := make(chan error, 1)
	go func() { started <- ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg) }()
	<-publishEntered
	ctx.pluginCommandController.mu.Lock()
	pending := ctx.pluginCommandController.pending
	ctx.pluginCommandController.mu.Unlock()
	require.NotNil(t, pending)
	owner := uint(19)
	_, err := pending.Submit(plugin_commands.CommandRequest{
		PluginName: "controller", ActorUserID: &owner,
		Declaration: plugin_commands.Declaration{Name: "blocked", Argv: []string{"blocked"}, Timeout: time.Minute},
	})
	require.NoError(t, err)
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		close(executor.release)
		t.Fatal("pre-publication worker did not start")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- ctx.stopPluginCommandsWithin(20 * time.Millisecond) }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	close(allowPublish)
	require.NoError(t, <-started)
	require.ErrorIs(t, <-stopped, context.DeadlineExceeded)
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	close(executor.release)
	require.Eventually(t, pending.RuntimeLeaseReleasable, time.Second, time.Millisecond)
	require.NoError(t, ctx.stopPluginCommandsWithin(20*time.Millisecond))
	second, leaseErr = plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, leaseErr)
	require.NoError(t, second.Close())
}

func TestPluginCommandControllerStopDuringBusyAcquireStaysStopping(t *testing.T) {
	testPluginCommandControllerStopDuringAcquireError(t, plugin_commands.ErrRuntimeLeaseBusy)
}

func TestPluginCommandControllerStopDuringNonBusyAcquireStaysStopping(t *testing.T) {
	testPluginCommandControllerStopDuringAcquireError(t, errors.New("permission denied"))
}

func testPluginCommandControllerStopDuringAcquireError(t *testing.T, acquireErr error) {
	t.Helper()
	ctx := newPluginCommandStoreTestContext(t)
	acquireEntered := make(chan struct{})
	allowAcquire := make(chan struct{})
	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireLease = func(string) (*plugin_commands.RuntimeLease, error) {
		close(acquireEntered)
		<-allowAcquire
		return nil, acquireErr
	}
	started := make(chan error, 1)
	go func() {
		started <- ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
			root: t.TempDir(), commandPath: t.TempDir(),
		}, cfg)
	}()
	<-acquireEntered
	stopped := make(chan error, 1)
	go func() { stopped <- ctx.stopPluginCommandsWithin(20 * time.Millisecond) }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	close(allowAcquire)
	require.NoError(t, <-started)
	require.NoError(t, <-stopped)
	ctx.pluginCommandController.mu.Lock()
	state := ctx.pluginCommandController.state
	ctx.pluginCommandController.mu.Unlock()
	require.Equal(t, pluginCommandRuntimeStopping, state)
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr)
	var logs []models.LogEntry
	require.NoError(t, ctx.db.Where("entity_type = ?", "plugin_command").Find(&logs).Error)
	require.Empty(t, logs)
}

func TestPluginCommandControllerStopAfterRecoveryBlockerStaysStopping(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	runID := "controller-stop-recovery-blocked"
	require.NoError(t, ctx.CreateRun(plugin_commands.RunRecord{
		ID: runID, PluginName: "controller", CommandName: "tool", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: runID, ArgvJSON: `[]`, CreatedAt: now}))
	won, err := ctx.MarkRunRunning(runID, now)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, ctx.SetRunProcessGroup(runID, 4242, "same-boot"))

	inspector := &controllerRecoveryInspector{state: plugin_commands.GroupAliveUnverified}
	blockerReady := make(chan struct{})
	allowQuarantine := make(chan struct{})
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.beforeQuarantine = func(pluginCommandRuntimeState) {
		close(blockerReady)
		<-allowQuarantine
	}
	started := make(chan error, 1)
	go func() {
		started <- ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
			root: t.TempDir(), commandPath: t.TempDir(),
		}, cfg)
	}()
	<-blockerReady
	stopped := make(chan error, 1)
	go func() { stopped <- ctx.stopPluginCommandsWithin(20 * time.Millisecond) }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	close(allowQuarantine)
	require.NoError(t, <-started)
	require.NoError(t, <-stopped)
	ctx.pluginCommandController.mu.Lock()
	state := ctx.pluginCommandController.state
	ctx.pluginCommandController.mu.Unlock()
	require.Equal(t, pluginCommandRuntimeStopping, state)
	var logs []models.LogEntry
	require.NoError(t, ctx.db.Where("entity_type = ?", "plugin_command").Find(&logs).Error)
	require.Empty(t, logs)
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr)
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
