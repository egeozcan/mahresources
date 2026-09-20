package application_context

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"mahresources/models"
	"mahresources/plugin_commands"
)

type pluginCommandRuntimeState uint8

const (
	pluginCommandRuntimeIdle pluginCommandRuntimeState = iota
	pluginCommandRuntimeAcquiring
	pluginCommandRuntimeQuarantined
	pluginCommandRuntimeActive
	pluginCommandRuntimeStopping
)

type pluginCommandActiveRuntime struct {
	dispatcher *plugin_commands.Dispatcher
	exchange   plugin_commands.Exchange
}

type pluginCommandRuntimeController struct {
	owner    *MahresourcesContext
	settings plugin_commands.Settings
	active   atomic.Pointer[pluginCommandActiveRuntime]
	mu       sync.Mutex
	state    pluginCommandRuntimeState
	reason   string
	retryAt  time.Time
	lease    *plugin_commands.RuntimeLease
	pending  *plugin_commands.Dispatcher
	// pendingExchange shares the pending dispatcher's lease manager. It is not
	// published until recovery succeeds.
	pendingExchange plugin_commands.Exchange
	draining        *pluginCommandActiveRuntime
	cancel          context.CancelFunc
	done            chan struct{}
	bootSessionID   string
	config          pluginCommandControllerConfig
}

type pluginCommandControllerConfig struct {
	acquireLease     func(string) (*plugin_commands.RuntimeLease, error)
	bootSessionID    func() (string, error)
	acquireBackoff   []time.Duration
	recoveryInterval time.Duration
	inspector        plugin_commands.ProcessInspector
	beforeAcquire    func(int)
	beforePublish    func()
}

func defaultPluginCommandControllerConfig() pluginCommandControllerConfig {
	return pluginCommandControllerConfig{
		acquireLease:     plugin_commands.AcquireRuntimeLease,
		bootSessionID:    plugin_commands.CurrentBootSessionID,
		acquireBackoff:   []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute, 5 * time.Minute},
		recoveryInterval: 5 * time.Minute,
	}
}

const pluginCommandCallerQuarantineReason = "commands are quarantined until automatic recovery succeeds; see /logs"

func newPluginCommandRuntimeController(owner *MahresourcesContext) *pluginCommandRuntimeController {
	return &pluginCommandRuntimeController{owner: owner}
}

func (ctx *MahresourcesContext) pluginCommandActive() (*pluginCommandActiveRuntime, error) {
	if ctx == nil || ctx.pluginCommandController == nil {
		return nil, &plugin_commands.RuntimeQuarantinedError{Reason: pluginCommandCallerQuarantineReason}
	}
	controller := ctx.pluginCommandController
	if active := controller.active.Load(); active != nil {
		return active, nil
	}
	controller.mu.Lock()
	retryAt := controller.retryAt
	controller.mu.Unlock()
	retryAfter := time.Until(retryAt)
	if retryAt.IsZero() || retryAfter < 0 {
		retryAfter = 0
	}
	return nil, &plugin_commands.RuntimeQuarantinedError{Reason: pluginCommandCallerQuarantineReason, RetryAfterDuration: retryAfter}
}

func (ctx *MahresourcesContext) startPluginCommandsWithConfig(callCtx context.Context, settings plugin_commands.Settings, cfg pluginCommandControllerConfig) error {
	if settings == nil {
		return fmt.Errorf("plugin command settings are required")
	}
	if ctx == nil || ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager is unavailable")
	}
	if cfg.acquireLease == nil {
		cfg.acquireLease = plugin_commands.AcquireRuntimeLease
	}
	if cfg.bootSessionID == nil {
		cfg.bootSessionID = plugin_commands.CurrentBootSessionID
	}
	if len(cfg.acquireBackoff) == 0 {
		cfg.acquireBackoff = defaultPluginCommandControllerConfig().acquireBackoff
	}
	if cfg.recoveryInterval <= 0 {
		cfg.recoveryInterval = 5 * time.Minute
	}
	controller := ctx.pluginCommandController
	if controller == nil {
		controller = newPluginCommandRuntimeController(ctx)
		ctx.pluginCommandController = controller
	}

	controller.mu.Lock()
	if controller.state != pluginCommandRuntimeIdle {
		controller.mu.Unlock()
		return fmt.Errorf("plugin command runtime is already active or draining")
	}
	runCtx, cancel := context.WithCancel(callCtx)
	controller.owner = ctx
	controller.settings = commandSettings{Settings: settings}
	controller.config = cfg
	controller.state = pluginCommandRuntimeAcquiring
	controller.reason = ""
	controller.retryAt = time.Time{}
	controller.cancel = cancel
	controller.done = make(chan struct{})
	done := controller.done
	controller.mu.Unlock()

	bootSessionID, identityErr := cfg.bootSessionID()
	if identityErr != nil {
		message := fmt.Sprintf("plugin command boot-session identity is unavailable; recovery will use process inspection: %v", identityErr)
		log.Printf("[plugin-command] WARNING: %s", message)
		ctx.Logger().Warning(models.LogActionSystem, "plugin_command", nil, settings.StagingRoot(), message, nil)
		bootSessionID = ""
	}
	controller.mu.Lock()
	controller.bootSessionID = bootSessionID
	controller.mu.Unlock()

	kind, err := controller.tryActivate(runCtx, 0)
	switch kind {
	case pluginCommandAttemptActive:
		close(done)
		return nil
	case pluginCommandAttemptLeaseBusy, pluginCommandAttemptRecoveryBlocked:
		go controller.retryLoop(runCtx, done)
		return nil
	case pluginCommandAttemptStopped:
		close(done)
		return nil
	default:
		controller.resetAfterFailedStart()
		close(done)
		return err
	}
}

type pluginCommandAttemptKind uint8

const (
	pluginCommandAttemptFatal pluginCommandAttemptKind = iota
	pluginCommandAttemptLeaseBusy
	pluginCommandAttemptRecoveryBlocked
	pluginCommandAttemptActive
	pluginCommandAttemptStopped
)

func (c *pluginCommandRuntimeController) tryActivate(ctx context.Context, attempt int) (pluginCommandAttemptKind, error) {
	c.mu.Lock()
	if c.state == pluginCommandRuntimeStopping {
		c.mu.Unlock()
		return pluginCommandAttemptStopped, nil
	}
	pending := c.pending
	c.mu.Unlock()

	if pending == nil {
		if c.config.beforeAcquire != nil {
			c.config.beforeAcquire(attempt)
		}
		lease, err := c.config.acquireLease(c.settings.StagingRoot())
		if err != nil {
			if errors.Is(err, plugin_commands.ErrRuntimeLeaseBusy) {
				message := fmt.Sprintf("plugin command runtime is quarantined because staging root %q is leased by another runtime; automatic retry is active; restart with -plugins-disabled to keep plugin commands offline", c.settings.StagingRoot())
				c.enterQuarantine(pluginCommandRuntimeAcquiring, message, c.acquireDelay(attempt))
				return pluginCommandAttemptLeaseBusy, err
			}
			return pluginCommandAttemptFatal, fmt.Errorf("acquire plugin command runtime lease: %w; restart with -plugins-disabled to keep plugin commands offline", err)
		}
		c.mu.Lock()
		if c.state == pluginCommandRuntimeStopping {
			c.mu.Unlock()
			_ = lease.Close()
			return pluginCommandAttemptStopped, nil
		}
		c.lease = lease
		c.mu.Unlock()

		usage := plugin_commands.NewStagingUsageCache()
		leases := plugin_commands.NewLeaseManager()
		executor := plugin_commands.NewExecutor(plugin_commands.RunnerDependencies{
			Store: c.owner, BootSessionID: c.bootSessionID, Settings: c.settings,
			Inspector: c.config.inspector, Usage: usage, Logf: log.Printf,
		})
		dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
			Store: c.owner, BootSessionID: c.bootSessionID,
			Jobs: commandLiveJobs{manager: c.owner.downloadManager}, Executor: executor,
			Settings: c.settings, Inspector: c.config.inspector, Usage: usage,
			Leases: leases, Logf: log.Printf,
		})
		dispatcher.SetImporter(c.owner)
		exchange := plugin_commands.NewExchangeWithLeases(c.owner, c.settings, leases)
		c.mu.Lock()
		if c.state == pluginCommandRuntimeStopping {
			c.mu.Unlock()
			_ = lease.Close()
			return pluginCommandAttemptStopped, nil
		}
		c.pending = dispatcher
		c.pendingExchange = exchange
		pending = dispatcher
		c.mu.Unlock()
	}

	if err := pending.Recover(ctx); err != nil {
		var blocked *plugin_commands.RecoveryBlockedError
		if errors.As(err, &blocked) {
			message := fmt.Sprintf("plugin command runtime recovery is quarantined: %v; automatic retry is active; after deciding a named process group is abandoned an operator may terminate it with kill -KILL -- -<pgid>; restart with -plugins-disabled to keep plugin commands offline", blocked)
			c.enterQuarantine(pluginCommandRuntimeQuarantined, message, c.config.recoveryInterval)
			return pluginCommandAttemptRecoveryBlocked, err
		}
		return pluginCommandAttemptFatal, fmt.Errorf("recover plugin commands: %w", err)
	}
	if err := pending.Start(ctx); err != nil {
		return pluginCommandAttemptFatal, fmt.Errorf("start plugin commands: %w", err)
	}
	if c.config.beforePublish != nil {
		c.config.beforePublish()
	}

	c.mu.Lock()
	if c.state == pluginCommandRuntimeStopping {
		c.mu.Unlock()
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = pending.Stop(stopCtx)
		cancel()
		c.mu.Lock()
		c.pending = nil
		c.pendingExchange = nil
		c.mu.Unlock()
		return pluginCommandAttemptStopped, nil
	}
	active := &pluginCommandActiveRuntime{dispatcher: pending, exchange: c.pendingExchange}
	c.active.Store(active)
	c.pending = nil
	c.pendingExchange = nil
	c.state = pluginCommandRuntimeActive
	c.reason = ""
	c.retryAt = time.Time{}
	c.mu.Unlock()

	// The application snapshot is authoritative and is published first. The
	// plugin manager stores this context as a dynamic per-call provider.
	c.owner.pluginManager.SetCommandSubmitter(c.owner)
	c.owner.pluginManager.SetExchangeMediator(c.owner)
	c.logActivation()
	return pluginCommandAttemptActive, nil
}

func (c *pluginCommandRuntimeController) acquireDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(c.config.acquireBackoff) {
		return c.config.acquireBackoff[len(c.config.acquireBackoff)-1]
	}
	return c.config.acquireBackoff[attempt]
}

func (c *pluginCommandRuntimeController) enterQuarantine(state pluginCommandRuntimeState, reason string, delay time.Duration) {
	c.mu.Lock()
	changed := c.state != state || c.reason != reason
	c.state = state
	c.reason = reason
	c.retryAt = time.Now().Add(delay)
	c.mu.Unlock()
	if changed {
		log.Printf("[plugin-command] WARNING: %s", reason)
		c.owner.Logger().Warning(models.LogActionSystem, "plugin_command", nil, c.settings.StagingRoot(), reason, nil)
	}
}

func (c *pluginCommandRuntimeController) retryLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	acquireAttempt := 0
	for {
		c.mu.Lock()
		if c.state == pluginCommandRuntimeStopping {
			c.mu.Unlock()
			return
		}
		state := c.state
		delay := c.config.recoveryInterval
		if state == pluginCommandRuntimeAcquiring {
			delay = c.acquireDelay(acquireAttempt)
		}
		c.retryAt = time.Now().Add(delay)
		c.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}

		kind, err := c.tryActivate(ctx, acquireAttempt+1)
		switch kind {
		case pluginCommandAttemptActive, pluginCommandAttemptStopped:
			return
		case pluginCommandAttemptLeaseBusy:
			acquireAttempt++
		case pluginCommandAttemptRecoveryBlocked:
			// Recovery uses a flat cadence while retaining the lease.
		case pluginCommandAttemptFatal:
			if ctx.Err() != nil {
				return
			}
			c.mu.Lock()
			hasLease := c.lease != nil
			c.mu.Unlock()
			nextState := pluginCommandRuntimeAcquiring
			nextDelay := c.acquireDelay(acquireAttempt)
			if hasLease {
				nextState = pluginCommandRuntimeQuarantined
				nextDelay = c.config.recoveryInterval
			}
			message := fmt.Sprintf("plugin command runtime automatic recovery failed and remains quarantined: %v", err)
			c.enterQuarantine(nextState, message, nextDelay)
			if !hasLease {
				acquireAttempt++
			}
		}
	}
}

func (c *pluginCommandRuntimeController) resetAfterFailedStart() {
	c.mu.Lock()
	lease := c.lease
	c.lease = nil
	c.pending = nil
	c.pendingExchange = nil
	c.draining = nil
	c.settings = nil
	c.cancel = nil
	c.done = nil
	c.reason = ""
	c.retryAt = time.Time{}
	c.state = pluginCommandRuntimeIdle
	c.mu.Unlock()
	if lease != nil {
		_ = lease.Close()
	}
}

func (c *pluginCommandRuntimeController) logActivation() {
	message := fmt.Sprintf("plugin command runtime activated for staging root %q after safe recovery", c.settings.StagingRoot())
	log.Printf("[plugin-command] %s", message)
	c.owner.Logger().Info(models.LogActionSystem, "plugin_command", nil, c.settings.StagingRoot(), message, nil)
}
