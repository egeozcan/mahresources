package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"mahresources/models"
	"mahresources/plugin_system"
)

// EnsurePluginStates creates PluginState rows for any discovered plugins
// that don't yet have one. Returns all plugin states.
func (ctx *MahresourcesContext) EnsurePluginStates() ([]models.PluginState, error) {
	if ctx.pluginManager == nil {
		return nil, nil
	}

	discovered := ctx.pluginManager.DiscoveredPlugins()

	if len(discovered) > 0 {
		// De-duplicate discovered names before checking existence.
		discoveredMap := make(map[string]struct{}, len(discovered))
		var uniqueNames []string
		for _, dp := range discovered {
			if _, ok := discoveredMap[dp.Name]; !ok {
				discoveredMap[dp.Name] = struct{}{}
				uniqueNames = append(uniqueNames, dp.Name)
			}
		}

		var existingNames []string
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name IN ?", uniqueNames).
			Pluck("plugin_name", &existingNames).Error; err != nil {
			return nil, err
		}

		existingMap := make(map[string]struct{}, len(existingNames))
		for _, name := range existingNames {
			existingMap[name] = struct{}{}
		}

		var toCreate []models.PluginState
		for _, name := range uniqueNames {
			if _, ok := existingMap[name]; !ok {
				toCreate = append(toCreate, models.PluginState{
					PluginName: name,
					Enabled:    false,
				})
			}
		}

		if len(toCreate) > 0 {
			if err := ctx.db.Create(&toCreate).Error; err != nil {
				return nil, fmt.Errorf("batch creating plugin states: %w", err)
			}
		}
	}

	return ctx.GetPluginStates()
}

// GetPluginStates returns all plugin states from the database.
func (ctx *MahresourcesContext) GetPluginStates() ([]models.PluginState, error) {
	var states []models.PluginState
	if err := ctx.db.Order("plugin_name").Find(&states).Error; err != nil {
		return nil, err
	}
	return states, nil
}

// GetPluginState returns the state for a specific plugin.
func (ctx *MahresourcesContext) GetPluginState(pluginName string) (*models.PluginState, error) {
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// SetPluginEnabled enables or disables a plugin and updates the database.
// pluginStateMu serializes settle-time reconciliation of the enabled column.
//
// Package-level rather than a field, because a context is cloned per call —
// WithPrincipal and WithTransaction both hand back a copy — so a per-context
// mutex would let two clones reconcile the same plugin at the same moment,
// which is the one thing it exists to prevent. There is one plugin manager per
// process, so one mutex per process is the matching scope.
// settleAttempts and settleRetryDelay bound reconcileEnabledState's retry of a
// failed settling write. Small on purpose: the mutex is held across them, and a
// failure that outlives a few tens of milliseconds is not the transient kind
// this is for.
const (
	settleAttempts   = 3
	settleRetryDelay = 25 * time.Millisecond
)

var pluginStateMu sync.Mutex

// reconcileEnabledState settles the enabled column against what this process
// actually did, and is how both lifecycle branches finish.
//
// It replaces an undo per branch. Undoing looks right and is not: each branch
// writes the column before acting so a crash mid-load leaves the operator's
// intent on disk, and then has to take that write back if the act fails — which
// is a check-then-act with room for another whole operation in between. Every
// local rule for whether to take it back has failed on a real interleaving. An
// unconditional undo discards a decision made later (a failed enable overwriting
// a successful one). A conditional undo keyed on the writer's own value fails
// the opposite way: rollback ownership has to *transfer* between operations —
// the loser of two enables defers to the winner, so it is the winner's failure
// that must undo the loser's write — and no per-writer token can express that.
// The receipt tried and moved the defect from a microsecond window into a
// seconds-wide one. ClaimDownloadHistoryRetry's shape does not carry over here,
// because there the claim's owner and its undoer are the same operation.
//
// So nobody undoes. Whoever finishes last writes what is true. "Loaded" is a
// property of this process's memory, and IsEnabled answers it exactly — except
// during a load, when the plugin is in none of the manager's maps. That single
// blind spot is what EnableInFlight excludes, and an operation that finds one in
// flight writes nothing because the operation running it will settle the row
// when it is done.
//
// The mutex is process-local on purpose, and is not the kind of lock this tree
// avoids: the question being answered — "is this plugin loaded *here*" — is
// per-process in-memory state, and two processes are entitled to differ on it.
// It is taken before pm.mu (via EnableInFlight and IsEnabled) and never the
// reverse; the manager never calls up into the context.
//
// It does span the settling UPDATE, which is the shape that deadlocked the
// scoped-access cache once — a caller inside a transaction holding a connection
// while the lock-holder needs one is, on SQLite, a deadlock. It is safe here for
// a reason worth keeping true: the only callers are SetPluginEnabled's two
// branches, reached from the enable/disable handlers with no transaction open
// and no connection held, and nothing else waits on this mutex. Calling
// SetPluginEnabled from inside a transaction would break that, so do not.
func (ctx *MahresourcesContext) reconcileEnabledState(pluginName string) {
	if ctx.pluginManager == nil {
		return
	}

	// The scoped-access snapshot keys on the enabled column, and by the time this
	// runs the caller has already written it optimistically — so the snapshot is
	// suspect whatever is decided below, including on the paths that settle
	// nothing. Keying this on the settling UPDATE's own RowsAffected was wrong
	// for exactly that reason: a disable of a plugin that was not loaded moves
	// the row true->false in the caller's write, leaves the reconcile with
	// nothing to change, and would have left the snapshot serving "allowed"
	// until its TTL expired. Registered first so it runs after the unlock: it is
	// a store on an atomic, and there is no reason to do it under the mutex.
	defer ctx.InvalidateScopedPluginAccess()

	pluginStateMu.Lock()
	defer pluginStateMu.Unlock()

	// A failed statement here is the one remaining way the row can be left
	// disagreeing with the process, and one direction of it is the failure this
	// whole area exists to prevent: a disable whose plugin is still loaded leaves
	// the row saying off, and the next restart drops the plugin and its
	// schedules. A transient lock or connection error is exactly the kind that
	// succeeds on a second attempt, so try again rather than logging once and
	// calling it settled.
	//
	// Each attempt re-establishes its own precondition instead of reusing the
	// first answer. No other reconcile can be running — that is what the mutex
	// buys — but a load can still start and finish underneath this one, and
	// writing a "loaded" read from before it would settle the row against a
	// process state that no longer exists. Mid-load IsEnabled reads false for a
	// plugin about to publish, which is precisely the answer that must never be
	// written down.
	var err error
	for attempt := 0; attempt < settleAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(settleRetryDelay)
		}
		if ctx.pluginManager.EnableInFlight(pluginName) {
			return
		}
		loaded := ctx.pluginManager.IsEnabled(pluginName)
		err = ctx.db.Model(&models.PluginState{}).
			Where("plugin_name = ? AND enabled <> ?", pluginName, loaded).
			Update("enabled", loaded).Error
		if err == nil {
			return
		}
	}

	// Where an operator actually looks, not only on stdout. Until someone enables
	// or disables this plugin again — which reconciles it, so the state does
	// self-heal — the row cannot be trusted to describe the process.
	ctx.Logger().Warning("system", "plugin", nil, pluginName,
		"could not settle the plugin's enabled state; the stored value may not match "+
			"whether the plugin is actually running", map[string]interface{}{
			"error":    err.Error(),
			"attempts": settleAttempts,
		})
}

func (ctx *MahresourcesContext) SetPluginEnabled(pluginName string, enabled bool) error {
	if ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager not initialized")
	}

	if enabled {
		// Check required settings before enabling
		dp := ctx.findDiscoveredPlugin(pluginName)
		if dp == nil {
			return fmt.Errorf("plugin %q not found", pluginName)
		}

		settings, _ := ctx.loadPluginSettingsMap(pluginName)
		missing := plugin_system.CheckRequiredSettings(dp.Settings, settings)
		if len(missing) > 0 {
			return fmt.Errorf("missing required settings: %v", missing)
		}

		// Enabling *is* the consent gesture, so the record is written here and
		// before the load, which enforces it. Written afterwards, the first
		// enable of a plugin that widened its manifest would be refused by the
		// consent it is in the middle of giving.
		//
		// Before the enabled column too, so a consent write that fails leaves
		// the row exactly as it was and there is nothing to revert.
		consent := plugin_system.GrantsFromManifest(dp.Manifest)
		if err := (&pluginConsentStore{ctx: ctx}).RecordConsent(pluginName, consent); err != nil {
			return err
		}

		// Persist DB state first, then enable in memory: a crash mid-load leaves
		// the operator's intent on disk, and the next boot retries it — except in
		// the one window reconcileEnabledState documents, where a second,
		// concurrent operation can settle over this write before the manager has
		// registered the load. Nothing
		// takes this write back — reconcileEnabledState settles the column
		// against what the process actually ended up with, whatever happened in
		// between and whoever else was doing it at the same time.
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name = ?", pluginName).
			Update("enabled", true).Error; err != nil {
			return err
		}
		defer ctx.reconcileEnabledState(pluginName)

		// Load settings into plugin manager memory
		ctx.pluginManager.SetPluginSettings(pluginName, settings)

		if err := ctx.pluginManager.EnablePlugin(pluginName); err != nil {
			// Report the refusal and let the deferred reconcile decide the row.
			// Both sentinels that used to steer that decision — "already
			// enabled" and ErrEnableInProgress — asked the same question the
			// reconcile answers directly and later: is this plugin running now?
			// They remain what the *caller* is told, which is a separate thing.
			return err
		}

		// Record what the plugin declared, against whoever is asking. This is the
		// only point where both exist: init() runs with its Lua context removed,
		// so mah.schedule cannot see the operator, and after this call returns the
		// request context is gone. A failure here is logged rather than returned —
		// the plugin is enabled and working, and unwinding that to report a
		// bookkeeping error would be the worse outcome.
		ctx.syncSchedulesFor(pluginName)
	} else {
		// Persist DB state first, then disable in memory. As above, nothing
		// takes this write back; the deferred reconcile settles it.
		if err := ctx.db.Model(&models.PluginState{}).
			Where("plugin_name = ?", pluginName).
			Update("enabled", false).Error; err != nil {
			return err
		}
		defer ctx.reconcileEnabledState(pluginName)

		if err := ctx.pluginManager.DisablePlugin(pluginName); err != nil {
			// What the caller is told, only — the row is the reconcile's.
			//
			// "I could not act" must not be reported as success, and it is the
			// one refusal the loaded state cannot identify: an enable is in
			// flight, so the plugin is not in the manager's map yet, which is
			// indistinguishable from a plugin that was never loaded. Answering
			// it with the idempotent branch below reported {"ok":true} while the
			// load went on to publish.
			if errors.Is(err, plugin_system.ErrLoadInProgress) {
				return err
			}
			// Not loaded and no load in flight: the caller asked for a state the
			// process is already in, which is success rather than a failure to
			// report.
			if !ctx.pluginManager.IsEnabled(pluginName) {
				return nil
			}
			return err
		}
	}

	// No invalidation here: every path that reaches it has written the column and
	// therefore has a reconcile deferred, which is the one place that drops the
	// snapshot. Two call sites for one rule is how the early-return path lost it.
	return nil
}

// SavePluginSettings validates and saves settings for a plugin.
func (ctx *MahresourcesContext) SavePluginSettings(pluginName string, values map[string]any) ([]plugin_system.ValidationError, error) {
	if ctx.pluginManager == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return nil, fmt.Errorf("plugin %q not found", pluginName)
	}

	// Validate
	if errs := plugin_system.ValidateSettings(dp.Settings, values); len(errs) > 0 {
		return errs, nil
	}

	// Filter to declared keys only
	declared := make(map[string]struct{}, len(dp.Settings))
	for _, s := range dp.Settings {
		declared[s.Name] = struct{}{}
	}
	filtered := make(map[string]any, len(declared))
	for k, v := range values {
		if _, ok := declared[k]; ok {
			filtered[k] = v
		}
	}
	values = filtered

	// Serialize to JSON
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshaling settings: %w", err)
	}

	// Save to DB
	if err := ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("settings_json", string(jsonBytes)).Error; err != nil {
		return nil, err
	}

	// Update in-memory settings if plugin is enabled
	if ctx.pluginManager.IsEnabled(pluginName) {
		ctx.pluginManager.SetPluginSettings(pluginName, values)
	}

	return nil, nil
}

// ActivateEnabledPlugins enables all plugins marked as enabled in the database.
//
// It is a repeated pass, not one walk of the states. A plugin's dependencies
// have to be loaded before it is, and the states arrive in plugin_name order, so
// a single walk starts "aaa" — which depends on "zzz" — before anything could
// have satisfied it. Each round enables everything whose declared dependencies
// this run has already loaded; rounds stop as soon as one loads nothing, and
// whatever is still waiting is reported once, naming what it waits for.
//
// Per-plugin behaviour is unchanged: settings that fail to load are a warning
// and the plugin starts with its defaults, and a plugin that fails to enable is
// a warning and the run continues. A failed plugin is not retried — it would log
// the same warning every round — and is never counted as loaded, so anything
// depending on it stays behind and is named in the final warning.
func (ctx *MahresourcesContext) ActivateEnabledPlugins() {
	if ctx.pluginManager == nil {
		return
	}

	states, err := ctx.GetPluginStates()
	if err != nil {
		ctx.Logger().Error("system", "plugin", nil, "", "failed to load plugin states at startup", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	var remaining []string
	enabled := make(map[string]struct{}, len(states))
	for _, state := range states {
		if state.Enabled {
			remaining = append(remaining, state.PluginName)
			enabled[state.PluginName] = struct{}{}
		}
	}

	loaded := make(map[string]struct{}, len(remaining))
	failed := make(map[string]struct{})

	for len(remaining) > 0 {
		var waiting []string
		progressed := false

		for _, name := range remaining {
			if len(ctx.unmetDependencies(name, loaded)) > 0 {
				waiting = append(waiting, name)
				continue
			}
			if ctx.enablePluginAtStartup(name) {
				loaded[name] = struct{}{}
				progressed = true
				continue
			}
			failed[name] = struct{}{}
		}

		remaining = waiting
		// Nothing loaded this round, so no unmet dependency can have become met:
		// another round would make exactly the same decisions.
		if !progressed {
			break
		}
	}

	if len(remaining) > 0 {
		ctx.warnAboutStalledPlugins(remaining, loaded, enabled, failed)
	}
}

// enablePluginAtStartup loads one plugin's settings and enables it, reporting
// whether it is now running.
func (ctx *MahresourcesContext) enablePluginAtStartup(pluginName string) bool {
	settings, err := ctx.loadPluginSettingsMap(pluginName)
	if err != nil {
		ctx.Logger().Warning("system", "plugin", nil, pluginName, "failed to load settings at startup", map[string]interface{}{
			"error": err.Error(),
		})
	}
	ctx.pluginManager.SetPluginSettings(pluginName, settings)

	if err := ctx.pluginManager.EnablePlugin(pluginName); err != nil {
		ctx.Logger().Warning("system", "plugin", nil, pluginName, "failed to enable plugin at startup", map[string]interface{}{
			"error": err.Error(),
		})
		return false
	}

	// Startup carries no principal, so this creates rows for anything newly
	// declared and leaves the owner of everything else alone. A schedule first
	// seen here is unowned and therefore inert until an operator enables the
	// plugin themselves, which is the fail-closed direction: nobody chose it, so
	// nothing runs as anybody.
	ctx.syncSchedulesFor(pluginName)
	return true
}

// syncSchedulesFor records a plugin's declared schedules, logging rather than
// returning a failure: the plugin is loaded and working either way, and the
// caller's job was to enable it.
func (ctx *MahresourcesContext) syncSchedulesFor(pluginName string) {
	if ctx.pluginManager == nil {
		return
	}
	regs := ctx.pluginManager.DeclaredSchedules(pluginName)
	if len(regs) == 0 {
		return
	}
	if err := ctx.SyncPluginSchedules(pluginName, regs); err != nil {
		ctx.Logger().Warning("system", "plugin", nil, pluginName, "failed to record plugin schedules", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

// unmetDependencies returns the plugin's declared dependencies that this run has
// not loaded yet.
//
// A plugin with no discovered manifest waits for nothing: EnablePlugin refuses
// it by name, which is the message an operator needs, and holding it back would
// replace that with a vaguer one about dependencies.
func (ctx *MahresourcesContext) unmetDependencies(pluginName string, loaded map[string]struct{}) []string {
	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return nil
	}

	var unmet []string
	for _, dep := range dp.Manifest.Dependencies {
		if _, ok := loaded[dep]; !ok {
			unmet = append(unmet, dep)
		}
	}
	return unmet
}

// warnAboutStalledPlugins logs the single entry that closes an activation run:
// which enabled plugins never started, and which dependency each is waiting for.
//
// One entry rather than one per plugin, because a cycle stalls every plugin in
// it and the useful fact is the shape of the whole set, not each member of it.
func (ctx *MahresourcesContext) warnAboutStalledPlugins(stalled []string, loaded, enabled, failed map[string]struct{}) {
	waitingSet := make(map[string]struct{}, len(stalled))
	for _, name := range stalled {
		waitingSet[name] = struct{}{}
	}

	lines := make([]string, 0, len(stalled))
	for _, name := range stalled {
		var reasons []string
		for _, dep := range ctx.unmetDependencies(name, loaded) {
			reasons = append(reasons, dep+" ("+ctx.stalledDependencyReason(dep, waitingSet, enabled, failed)+")")
		}
		lines = append(lines, name+" waits for "+strings.Join(reasons, ", "))
	}

	ctx.Logger().Warning("system", "plugin", nil, "", "enabled plugins were not started because their dependencies never loaded: "+strings.Join(lines, "; "),
		map[string]interface{}{
			"plugins": stalled,
			"waiting": lines,
		})
}

// stalledDependencyReason says why one dependency never became available, so the
// warning tells an operator what to fix rather than only what broke.
func (ctx *MahresourcesContext) stalledDependencyReason(dep string, waiting, enabled, failed map[string]struct{}) string {
	if ctx.findDiscoveredPlugin(dep) == nil {
		return "no such plugin"
	}
	if _, ok := failed[dep]; ok {
		return "it failed to load"
	}
	if _, ok := waiting[dep]; ok {
		// A stall travels: the plugin at the head of the chain is the one to
		// fix, and this is what points at it. Not a cycle — the manager drops
		// the members of one at discovery, so a cycle never reaches this run.
		return "also waiting for a dependency of its own"
	}
	if _, ok := enabled[dep]; !ok {
		return "not enabled"
	}
	return "not loaded"
}

func (ctx *MahresourcesContext) findDiscoveredPlugin(name string) *plugin_system.DiscoveredPlugin {
	return ctx.pluginManager.GetDiscoveredPlugin(name)
}

func (ctx *MahresourcesContext) loadPluginSettingsMap(pluginName string) (map[string]any, error) {
	state, err := ctx.GetPluginState(pluginName)
	if err != nil || state.SettingsJSON == "" {
		return ctx.applyPluginDefaults(pluginName, make(map[string]any)), err
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(state.SettingsJSON), &settings); err != nil {
		return ctx.applyPluginDefaults(pluginName, make(map[string]any)), err
	}
	return ctx.applyPluginDefaults(pluginName, settings), nil
}

// applyPluginDefaults merges declared default values from the plugin's
// setting definitions into the settings map for any keys not already present.
func (ctx *MahresourcesContext) applyPluginDefaults(pluginName string, settings map[string]any) map[string]any {
	dp := ctx.findDiscoveredPlugin(pluginName)
	if dp == nil {
		return settings
	}
	return plugin_system.ApplyDefaults(dp.Settings, settings)
}
