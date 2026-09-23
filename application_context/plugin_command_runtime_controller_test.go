package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/jobs"
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

func capturePluginCommandStdout(t *testing.T) func() string {
	t.Helper()
	readStdout, writeStdout, err := os.Pipe()
	require.NoError(t, err)
	originalStdout := os.Stdout
	os.Stdout = writeStdout
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		os.Stdout = originalStdout
		_ = writeStdout.Close()
		_ = readStdout.Close()
	})
	return func() string {
		t.Helper()
		require.False(t, stopped, "stdout capture stopped twice")
		stopped = true
		os.Stdout = originalStdout
		require.NoError(t, writeStdout.Close())
		output, err := io.ReadAll(readStdout)
		require.NoError(t, err)
		require.NoError(t, readStdout.Close())
		return string(output)
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

	stopStdoutCapture := capturePluginCommandStdout(t)
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

	stdout := stopStdoutCapture()
	require.Contains(t, stdout, "leased by another runtime")
	require.Contains(t, stdout, "automatic retry is active")
	require.Contains(t, stdout, "activated for staging root")
	require.Equal(t, 1, strings.Count(stdout, "leased by another runtime"), "lease quarantine stdout warning must be deduplicated")
}

func TestPluginCommandControllerBootIdentityWarningUsesStdout(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	stopStdoutCapture := capturePluginCommandStdout(t)
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "", errors.New("identity provider unavailable") }
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(),
	}, cfg))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	stdout := stopStdoutCapture()
	require.Contains(t, stdout, "boot-session identity is unavailable")
	require.Contains(t, stdout, "identity provider unavailable")
}

type controllerRecoveryInspector struct {
	mu     sync.Mutex
	state  plugin_commands.GroupState
	states map[int]plugin_commands.GroupState
	errs   map[int]error
	seen   []int
}

func (i *controllerRecoveryInspector) InspectGroup(pgid int, _ string) (plugin_commands.GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.seen = append(i.seen, pgid)
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

const controllerHealingPluginName = "controller-healing-host"

func newControllerHealingPluginContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	pluginDir := t.TempDir()
	writeConsentTestPlugin(t, pluginDir, controllerHealingPluginName, `plugin = {
  name = "controller-healing-host", version = "1.0", api_version = 1,
  capabilities = {"commands", "inject"},
  commands = {{name="probe", argv={"missing-controller-probe", "{{value}}"}}}
}
function init()
  mah.inject("page_bottom", function()
    local id, command_err = mah.commands.run("probe", {value="healed"})
    local runs, fs_err = mah.fs.runs()
    if command_err ~= nil or fs_err ~= nil then
      return "command=" .. tostring(command_err) .. "|fs=" .. tostring(fs_err)
    end
    return "ok:" .. tostring(id) .. ":" .. tostring(#runs)
  end)
end
`)
	ctx := createTestContextWithPlugins(t, pluginDir)
	ctx.SetJobService(jobs.NewService())
	migratePluginCommandJobTestModels(t, ctx)
	require.NoError(t, ctx.db.AutoMigrate(
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
	))
	_, err := ctx.EnsurePluginStates()
	require.NoError(t, err)
	require.NoError(t, ctx.SetPluginEnabledWithOptions(
		controllerHealingPluginName, true, PluginEnableOptions{ConfirmCommands: true},
	))
	t.Cleanup(ctx.PluginManager().Close)
	return ctx
}

func renderControllerHealingPlugin(ctx *MahresourcesContext) string {
	return ctx.PluginManager().RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
}

func requireControllerHealingPluginQuarantined(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	output := renderControllerHealingPlugin(ctx)
	require.Contains(t, output, "command=plugin command runtime is unavailable")
	require.Contains(t, output, "fs=plugin command runtime is unavailable")
}

func TestPluginCommandControllerRecoveryQuarantinePublishesBothHostsToLoadedPlugin(t *testing.T) {
	ctx := newControllerHealingPluginContext(t)
	root := t.TempDir()
	createControllerRecoveryRun(t, ctx, "loaded-plugin-blocker", 4341)

	inspector := &controllerRecoveryInspector{state: plugin_commands.GroupAliveUnverified}
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.recoveryInterval = 10 * time.Millisecond
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}, cfg))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })

	require.True(t, ctx.PluginManager().IsEnabled(controllerHealingPluginName))
	requireControllerHealingPluginQuarantined(t, ctx)

	inspector.set(plugin_commands.GroupDead)
	var healedOutput string
	require.Eventually(t, func() bool {
		healedOutput = renderControllerHealingPlugin(ctx)
		return strings.HasPrefix(healedOutput, "ok:")
	}, time.Second, 5*time.Millisecond)
	require.True(t, ctx.PluginManager().IsEnabled(controllerHealingPluginName), "healing must not reload the plugin")

	var submitted models.PluginCommandRun
	require.NoError(t, ctx.db.Where("plugin_name = ?", controllerHealingPluginName).First(&submitted).Error)
	require.Equal(t, "probe", submitted.CommandName)
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

func TestPluginCommandCrashRecoveryDoesNotNeedFormerDispatcherReport(t *testing.T) {
	const pgid = 6141
	for _, test := range []struct {
		name           string
		inspectorError error
	}{
		{name: "process inspection proves no worker remains"},
		{name: "uninspectable worker stays blocked until proof arrives", inspectorError: errors.New("process table unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newPluginCommandStoreTestContext(t)
			service := jobs.NewService()
			ctx.SetJobService(service)
			root := t.TempDir()
			settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}

			// Leave a previous controller's dispatcher with its report channel open,
			// as it would be after a crash. The replacement gets a new context and has
			// no reference through which it could ask this dispatcher to quiesce.
			oldLease, err := plugin_commands.AcquireRuntimeLease(root)
			require.NoError(t, err)
			oldFence, err := ctx.acquirePluginCommandDBFence(root)
			require.NoError(t, err)
			oldDispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
				Store: ctx, Jobs: lifecycleAsyncJobs{}, Executor: &lifecycleStubbornExecutor{},
				Settings: settings,
			})
			oldController := ctx.pluginCommandController
			oldController.mu.Lock()
			oldController.settings = commandSettings{Settings: settings}
			oldController.config = defaultPluginCommandControllerConfig()
			oldController.lease = oldLease
			oldController.dbFence = oldFence
			oldController.state = pluginCommandRuntimeActive
			oldController.active.Store(&pluginCommandActiveRuntime{dispatcher: oldDispatcher})
			oldController.mu.Unlock()
			require.False(t, oldDispatcher.RuntimeLeaseReleasable(), "the crashed owner's report must remain unavailable")

			now := time.Now().UTC()
			run := testRun("crash-recovery-command", uintPtr(7), false, now)
			require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
			stored, _, err := ctx.Run(run.ID)
			require.NoError(t, err)
			execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, run.ID)
			require.NoError(t, err)
			require.True(t, claimed)
			won, err := ctx.MarkRunRunning(run.ID, now.Add(time.Second))
			require.NoError(t, err)
			require.True(t, won)
			require.NoError(t, ctx.SetRunProcessGroup(run.ID, pgid, "former-boot"))
			var running models.Job
			require.NoError(t, ctx.db.First(&running, "id = ?", stored.JobID).Error)
			require.Equal(t, string(jobs.StateRunning), running.State)
			require.Equal(t, execution.ExecutionToken, running.ExecutionToken)

			// Releasing the OS lease models process death. The stale database token
			// is intentionally left behind; the replacement CAS-rotates it after it
			// takes the same staging root.
			require.NoError(t, oldLease.Close())
			recoveryCtx := NewMahresourcesContext(ctx.fs, ctx.db, ctx.readOnlyDB, ctx.Config)
			recoveryService := jobs.NewService()
			recoveryCtx.SetJobService(recoveryService)
			if manager := recoveryCtx.PluginManager(); manager != nil {
				t.Cleanup(manager.Close)
			}
			inspector := &controllerRecoveryInspector{state: plugin_commands.GroupDead}
			if test.inspectorError != nil {
				inspector.errs = map[int]error{pgid: test.inspectorError}
			}
			cfg := defaultPluginCommandControllerConfig()
			cfg.bootSessionID = func() (string, error) { return "former-boot", nil }
			cfg.inspector = inspector
			cfg.recoveryInterval = 5 * time.Millisecond
			require.NoError(t, recoveryCtx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
			t.Cleanup(func() {
				if err := recoveryCtx.StopPluginCommands(); err != nil {
					// A failing assertion may leave startup recovery quarantined. Test
					// teardown can release its temporary resources after cancelling the
					// retry loop; production keeps both fences in this case.
					controller := recoveryCtx.pluginCommandController
					controller.mu.Lock()
					lease, token := controller.lease, controller.dbFence
					controller.lease, controller.dbFence = nil, ""
					controller.mu.Unlock()
					_ = recoveryCtx.releasePluginCommandDBFence(token)
					if lease != nil {
						_ = lease.Close()
					}
				}
			})
			if test.inspectorError == nil {
				var source models.PluginCommandRun
				require.NoError(t, recoveryCtx.db.First(&source, "id = ?", run.ID).Error)
				require.Equal(t, plugin_commands.RunStatusInterrupted, source.Status)
				var recovered models.Job
				require.NoError(t, recoveryCtx.db.First(&recovered, "id = ?", stored.JobID).Error)
				require.Equal(t, string(jobs.StateInterrupted), recovered.State)
				var claim models.JobClaim
				require.NoError(t, recoveryCtx.db.First(&claim, "job_id = ?", stored.JobID).Error)
				require.Equal(t, models.JobClaimStateReleased, claim.State)
				var capacityCount int64
				require.NoError(t, recoveryCtx.db.Model(&models.JobCapacityLease{}).Where("job_id = ?", stored.JobID).Count(&capacityCount).Error)
				require.Zero(t, capacityCount)
				inspector.mu.Lock()
				require.Equal(t, []int{pgid}, inspector.seen, "the replacement must prove the process group dead before it releases ownership")
				inspector.mu.Unlock()
				return
			}

			// Each failed inspection keeps the canonical Job, claim, and admitted
			// slot quarantined. The source stays running and no replacement runtime
			// is published while process death remains uncertain.
			recoveryCtx.pluginCommandController.mu.Lock()
			pending := recoveryCtx.pluginCommandController.pending
			recoveryCtx.pluginCommandController.mu.Unlock()
			require.NotNil(t, pending)
			require.Nil(t, recoveryCtx.pluginCommandController.active.Load())
			assertQuarantined := func() {
				t.Helper()
				var blocked models.Job
				require.NoError(t, recoveryCtx.db.First(&blocked, "id = ?", stored.JobID).Error)
				require.Equal(t, string(jobs.StateBlocked), blocked.State)
				require.Equal(t, execution.ExecutionToken, blocked.ExecutionToken)
				var source models.PluginCommandRun
				require.NoError(t, recoveryCtx.db.First(&source, "id = ?", run.ID).Error)
				require.Equal(t, plugin_commands.RunStatusRunning, source.Status)
				var claim models.JobClaim
				require.NoError(t, recoveryCtx.db.First(&claim, "job_id = ?", stored.JobID).Error)
				require.Equal(t, models.JobClaimStateQuarantined, claim.State)
				require.Equal(t, execution.ExecutionToken, claim.ExecutionToken)
				var capacityCount int64
				require.NoError(t, recoveryCtx.db.Model(&models.JobCapacityLease{}).Where("job_id = ?", stored.JobID).Count(&capacityCount).Error)
				require.EqualValues(t, 1, capacityCount)
			}
			assertQuarantined()
			time.Sleep(50 * time.Millisecond)
			assertQuarantined()
			recoveryCtx.pluginCommandController.mu.Lock()
			state := recoveryCtx.pluginCommandController.state
			recoveryCtx.pluginCommandController.mu.Unlock()
			require.Equal(t, pluginCommandRuntimeQuarantined, state)

			// Give the test a safe shutdown boundary. Recovery cannot publish or
			// free capacity until a later process inspection supplies positive proof.
			inspector.setGroup(pgid, plugin_commands.GroupDead)
			require.Eventually(t, func() bool {
				_, err := recoveryCtx.pluginCommandActive()
				return err == nil
			}, time.Second, 5*time.Millisecond)
			var recoveredSource models.PluginCommandRun
			require.NoError(t, recoveryCtx.db.First(&recoveredSource, "id = ?", run.ID).Error)
			require.Equal(t, plugin_commands.RunStatusInterrupted, recoveredSource.Status)
			var recovered models.Job
			require.NoError(t, recoveryCtx.db.First(&recovered, "id = ?", stored.JobID).Error)
			require.Equal(t, string(jobs.StateInterrupted), recovered.State)
			require.Empty(t, recovered.ExecutionToken, "proven-dead recovery must clear the original token only when it terminalizes")
			var recoveredClaim models.JobClaim
			require.NoError(t, recoveryCtx.db.First(&recoveredClaim, "job_id = ?", stored.JobID).Error)
			require.Equal(t, models.JobClaimStateReleased, recoveredClaim.State)
			var recoveredCapacityCount int64
			require.NoError(t, recoveryCtx.db.Model(&models.JobCapacityLease{}).Where("job_id = ?", stored.JobID).Count(&recoveredCapacityCount).Error)
			require.Zero(t, recoveredCapacityCount)
		})
	}
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
	stopStdoutCapture := capturePluginCommandStdout(t)
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

	stdout := stopStdoutCapture()
	for _, value := range []string{
		"runtime recovery is quarantined", "log-blocked-one", "5251",
		"automatic recovery failed and remains quarantined", "activated for staging root",
	} {
		require.Contains(t, stdout, value)
	}
	require.Equal(t, 1, strings.Count(stdout, "automatic recovery failed and remains quarantined"), "retry-failure stdout warning must be deduplicated")
	require.Equal(t, 1, strings.Count(stdout, "activated for staging root"), "healing stdout message must be one-shot")
}

func TestPluginCommandControllerRejectsMalformedDurableRunsBeforePublication(t *testing.T) {
	positivePGID, zeroPGID, negativePGID := 7, 0, -7
	tests := []struct {
		name          string
		status        string
		pgid          *int
		bootSessionID string
		want          string
	}{
		{name: "unknown status", status: "orphaned", want: "unknown status"},
		{name: "queued with process group", status: plugin_commands.RunStatusQueued, pgid: &positivePGID, want: "invalid process identity"},
		{name: "queued with boot identity", status: plugin_commands.RunStatusQueued, bootSessionID: "same-boot", want: "invalid process identity"},
		{name: "queued with both identity fields", status: plugin_commands.RunStatusQueued, pgid: &positivePGID, bootSessionID: "same-boot", want: "invalid process identity"},
		{name: "running crash window with boot identity", status: plugin_commands.RunStatusRunning, bootSessionID: "same-boot", want: "invalid process identity"},
		{name: "running zero process group", status: plugin_commands.RunStatusRunning, pgid: &zeroPGID, want: "invalid process identity"},
		{name: "running zero process group with boot identity", status: plugin_commands.RunStatusRunning, pgid: &zeroPGID, bootSessionID: "same-boot", want: "invalid process identity"},
		{name: "running negative process group", status: plugin_commands.RunStatusRunning, pgid: &negativePGID, want: "invalid process identity"},
		{name: "running negative process group with boot identity", status: plugin_commands.RunStatusRunning, pgid: &negativePGID, bootSessionID: "same-boot", want: "invalid process identity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := newPluginCommandStoreTestContext(t)
			now := time.Now().UTC()
			require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
				ID: "valid-queued-run", PluginName: "controller", CommandName: "tool",
				ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedAt: now,
			}).Error)
			require.NoError(t, ctx.db.Create(&models.PluginCommandRunOutput{
				RunID: "valid-queued-run", ArgvJSON: `[]`, CreatedAt: now,
			}).Error)
			require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
				ID: "malformed-durable-run", PluginName: "controller", CommandName: "tool",
				ParamsJSON: `{}`, Status: test.status, ProcessGroupID: test.pgid,
				BootSessionID: test.bootSessionID, CreatedAt: now.Add(time.Second),
			}).Error)
			require.NoError(t, ctx.db.Create(&models.PluginCommandRunOutput{
				RunID: "malformed-durable-run", ArgvJSON: `[]`, CreatedAt: now.Add(time.Second),
			}).Error)

			cfg := defaultPluginCommandControllerConfig()
			cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
			cfg.inspector = &controllerRecoveryInspector{state: plugin_commands.GroupDead}
			err := ctx.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
				root: t.TempDir(), commandPath: t.TempDir(),
			}, cfg)
			require.ErrorContains(t, err, test.want)
			var blocked *plugin_commands.RecoveryBlockedError
			require.False(t, errors.As(err, &blocked), "malformed durable state must be fatal, not quarantine")
			active, activeErr := ctx.pluginCommandActive()
			require.Nil(t, active, "malformed durable state must not publish the runtime")
			require.ErrorIs(t, activeErr, plugin_commands.ErrCommandRuntimeQuarantined)
			ctx.pluginCommandController.mu.Lock()
			state := ctx.pluginCommandController.state
			ctx.pluginCommandController.mu.Unlock()
			require.Equal(t, pluginCommandRuntimeIdle, state, "fatal consistency errors must not enter quarantine")

			var valid, malformed models.PluginCommandRun
			require.NoError(t, ctx.db.First(&valid, "id = ?", "valid-queued-run").Error)
			require.Equal(t, plugin_commands.RunStatusQueued, valid.Status, "validation must precede recovery terminalization")
			require.Nil(t, valid.FinishedAt)
			require.NoError(t, ctx.db.First(&malformed, "id = ?", "malformed-durable-run").Error)
			require.Equal(t, test.status, malformed.Status)
			require.Equal(t, test.pgid, malformed.ProcessGroupID)
			require.Equal(t, test.bootSessionID, malformed.BootSessionID)
			require.Nil(t, malformed.FinishedAt)
			var quarantineLogs int64
			require.NoError(t, ctx.db.Model(&models.LogEntry{}).
				Where("entity_type = ?", "plugin_command").Count(&quarantineLogs).Error)
			require.Zero(t, quarantineLogs, "ordinary fatal consistency errors must not be logged as quarantine")
		})
	}
}

func TestPluginCommandControllerStopDuringHealingRetainsLeaseUntilDispatcherQuiesces(t *testing.T) {
	ctx := newControllerHealingPluginContext(t)
	root := t.TempDir()
	settings := testPluginCommandSettings{root: root, commandPath: t.TempDir()}
	createControllerRecoveryRun(t, ctx, "stop-healing-blocker", 5351)

	inspector := &controllerRecoveryInspector{state: plugin_commands.GroupAliveUnverified}
	executor := &lifecycleStubbornExecutor{started: make(chan struct{}), release: make(chan struct{})}
	publishEntered := make(chan struct{})
	allowPublish := make(chan struct{})
	publishErr := make(chan error, 1)
	var pending *plugin_commands.Dispatcher
	cfg := defaultPluginCommandControllerConfig()
	cfg.bootSessionID = func() (string, error) { return "same-boot", nil }
	cfg.inspector = inspector
	cfg.recoveryInterval = 10 * time.Millisecond
	cfg.buildRuntime = func() (*pluginCommandActiveRuntime, error) {
		leases := plugin_commands.NewLeaseManager()
		pending = plugin_commands.NewDispatcher(plugin_commands.Dependencies{
			Store: ctx, BootSessionID: "same-boot", Jobs: lifecycleAsyncJobs{}, Executor: executor,
			Settings: settings, Inspector: inspector, Leases: leases,
		})
		return &pluginCommandActiveRuntime{
			dispatcher: pending,
			exchange:   plugin_commands.NewExchangeWithLeases(ctx, settings, leases),
		}, nil
	}
	cfg.beforePublish = func() {
		owner := uint(19)
		_, err := pending.Submit(plugin_commands.CommandRequest{
			PluginName: controllerHealingPluginName, ActorUserID: &owner,
			Declaration: plugin_commands.Declaration{Name: "held", Argv: []string{"held"}, Timeout: time.Minute},
		})
		if err == nil {
			select {
			case <-executor.started:
			case <-time.After(time.Second):
				err = errors.New("healing dispatcher worker did not start")
			}
		}
		publishErr <- err
		close(publishEntered)
		<-allowPublish
	}

	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
	requireControllerHealingPluginQuarantined(t, ctx)
	inspector.set(plugin_commands.GroupDead)
	select {
	case <-publishEntered:
	case <-time.After(time.Second):
		close(executor.release)
		t.Fatal("recovery retry did not reach the publication barrier")
	}
	require.NoError(t, <-publishErr)

	stopped := make(chan error, 1)
	go func() { stopped <- ctx.StopPluginCommands() }()
	require.Eventually(t, func() bool {
		ctx.pluginCommandController.mu.Lock()
		defer ctx.pluginCommandController.mu.Unlock()
		return ctx.pluginCommandController.state == pluginCommandRuntimeStopping
	}, time.Second, time.Millisecond)
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)
	select {
	case err := <-stopped:
		close(executor.release)
		t.Fatalf("stop returned before the healing retry left publication: %v", err)
	default:
	}

	close(allowPublish)
	select {
	case err := <-stopped:
		close(executor.release)
		t.Fatalf("stop returned before the healing dispatcher became quiescent: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	second, leaseErr = plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)

	close(executor.release)
	require.NoError(t, <-stopped)
	_, activeErr := ctx.pluginCommandActive()
	requireGenericCommandQuarantineError(t, activeErr)
	requireControllerHealingPluginQuarantined(t, ctx)
	second, leaseErr = plugin_commands.AcquireRuntimeLease(root)
	require.NoError(t, leaseErr)
	require.NoError(t, second.Close())
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
	ctx.SetJobService(jobs.NewService())
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
	root := t.TempDir()
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
			root: root, commandPath: t.TempDir(),
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
	require.Error(t, <-stopped, "an unresolved recovery dispatcher cannot release either fence")
	second, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	require.ErrorIs(t, leaseErr, plugin_commands.ErrRuntimeLeaseBusy)
	require.Nil(t, second)
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
