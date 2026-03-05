package plugin_system

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
)

var validPagePath = regexp.MustCompile(`^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)

// PluginInfo holds metadata about a loaded plugin.
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	Dir         string
}

// DiscoveredPlugin holds metadata about a discovered (but not necessarily loaded) plugin.
type DiscoveredPlugin struct {
	Name        string
	Version     string
	Description string
	Dir         string
	Settings    []SettingDefinition
}

// hookEntry stores a Lua hook handler and its parent VM.
type hookEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// injectionEntry stores a Lua injection renderer and its parent VM.
type injectionEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// pageEntry stores a Lua page handler and its parent VM.
type pageEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// PageRegistration represents a plugin-contributed page.
type PageRegistration struct {
	PluginName string
	Path       string
}

// MenuRegistration represents a plugin-contributed menu item.
type MenuRegistration struct {
	PluginName string
	Label      string
	FullPath   string
}

// PluginManager loads and manages Lua plugins.
type PluginManager struct {
	plugins    []PluginInfo
	states     []*lua.LState
	hooks      map[string][]hookEntry
	injections map[string][]injectionEntry
	pages      map[string]map[string]pageEntry // pluginName -> path -> handler
	menuItems  []MenuRegistration
	actions    map[string][]ActionRegistration // pluginName -> actions
	mu         sync.RWMutex
	vmLocks    map[*lua.LState]*sync.Mutex
	dbProvider atomic.Value
	dbWriter   atomic.Value
	logger     atomic.Value
	kvStore    atomic.Value
	closed     atomic.Bool

	// Discovery-phase data (immutable after construction).
	discovered     []DiscoveredPlugin
	pluginSettings map[string]map[string]any // pluginName -> key -> value
	enabling       sync.Map                  // pluginName -> struct{}, prevents concurrent EnablePlugin

	// Async action job support
	actionJobs      map[string]*ActionJob
	actionJobsMu    sync.RWMutex
	actionSemaphore chan struct{} // buffered(maxConcurrentActions)
	actionSubs      map[chan ActionJobEvent]struct{}
	actionSubsMu    sync.RWMutex
	actionInFlight  map[string]*sync.WaitGroup // pluginName -> in-flight async action count

	// HTTP async callback support
	httpClient  *http.Client
	httpMu      sync.Mutex
	httpPending []httpCallback
	httpNotify  chan struct{}   // buffered(1), signals new callbacks
	done    chan struct{}   // closed to stop background goroutines (HTTP drain, job cleanup)
	httpWg      sync.WaitGroup // tracks in-flight HTTP goroutines
	httpSem     chan struct{}   // concurrency semaphore
}

// NewPluginManager scans dir for subdirectories containing plugin.lua,
// discovers each plugin's metadata and settings (without calling init()),
// and returns the manager. Plugins must be explicitly enabled via
// EnablePlugin to create Lua VMs and register hooks/injections/pages.
// If dir does not exist, an empty manager is returned.
func NewPluginManager(dir string) (*PluginManager, error) {
	pm := &PluginManager{
		hooks:           make(map[string][]hookEntry),
		injections:      make(map[string][]injectionEntry),
		pages:           make(map[string]map[string]pageEntry),
		actions:         make(map[string][]ActionRegistration),
		vmLocks:         make(map[*lua.LState]*sync.Mutex),
		pluginSettings:  make(map[string]map[string]any),
		actionJobs:      make(map[string]*ActionJob),
		actionSemaphore: make(chan struct{}, maxConcurrentActions),
		actionSubs:      make(map[chan ActionJobEvent]struct{}),
		actionInFlight:  make(map[string]*sync.WaitGroup),
		httpClient:      newHttpClient(),
		httpNotify:      make(chan struct{}, 1),
		done:        make(chan struct{}),
		httpSem:         make(chan struct{}, maxConcurrentHttpReqs),
	}

	go pm.drainHttpCallbacks()

	// Start action job cleanup ticker.
	go func() {
		ticker := time.NewTicker(actionJobCleanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pm.cleanupOldActionJobs()
			case <-pm.done:
				return
			}
		}
	}()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return pm, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	// Collect subdirectory names that contain plugin.lua, then sort.
	var pluginDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		entryPath := filepath.Join(dir, entry.Name(), "plugin.lua")
		if _, err := os.Stat(entryPath); err == nil {
			pluginDirs = append(pluginDirs, entry.Name())
		}
	}
	sort.Strings(pluginDirs)

	for _, name := range pluginDirs {
		pluginDir := filepath.Join(dir, name)
		scriptPath := filepath.Join(pluginDir, "plugin.lua")
		dp, err := pm.discoverPlugin(pluginDir, scriptPath)
		if err != nil {
			log.Printf("[plugin] warning: skipping %q: %v", name, err)
			continue
		}
		pm.discovered = append(pm.discovered, dp)
	}

	return pm, nil
}

// discoverPlugin creates a temporary Lua VM, executes plugin.lua (top-level
// code only, NOT init()), reads metadata and settings, then closes the VM.
func (pm *PluginManager) discoverPlugin(pluginDir, scriptPath string) (DiscoveredPlugin, error) {
	code, err := os.ReadFile(scriptPath)
	if err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("reading plugin.lua: %w", err)
	}

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	if err := L.DoString(string(code)); err != nil {
		return DiscoveredPlugin{}, fmt.Errorf("parsing plugin.lua: %w", err)
	}

	dp := DiscoveredPlugin{Dir: pluginDir}
	pluginTable := L.GetGlobal("plugin")
	if tbl, ok := pluginTable.(*lua.LTable); ok {
		if v := tbl.RawGetString("name"); v != lua.LNil {
			dp.Name = v.String()
		}
		if v := tbl.RawGetString("version"); v != lua.LNil {
			dp.Version = v.String()
		}
		if v := tbl.RawGetString("description"); v != lua.LNil {
			dp.Description = v.String()
		}
	}

	dp.Settings = extractSettingsFromState(L)
	return dp, nil
}

// loadPlugin creates a Lua VM, registers the mah module, executes plugin.lua,
// reads metadata, and calls init() if present.
func (pm *PluginManager) loadPlugin(pluginDir, scriptPath string) error {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	// Open only safe libraries (excludes os, io, debug, package)
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
		{lua.CoroutineLibName, lua.OpenCoroutine},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	// Remove dangerous base functions
	for _, name := range []string{"dofile", "loadfile", "load"} {
		L.SetGlobal(name, lua.LNil)
	}

	pm.mu.Lock()
	pm.vmLocks[L] = &sync.Mutex{}
	pm.mu.Unlock()

	// pluginName is populated after DoFile reads the plugin table, but before
	// init() is called. Closures in registerMahModule capture the pointer so
	// they see the final value when invoked during init().
	var pluginName string

	// Register the mah module.
	pm.registerMahModule(L, &pluginName)

	// Execute plugin.lua.
	if err := L.DoFile(scriptPath); err != nil {
		L.Close()
		return fmt.Errorf("executing plugin.lua: %w", err)
	}

	// Read plugin metadata from the global `plugin` table.
	info := PluginInfo{Dir: pluginDir}
	pluginTable := L.GetGlobal("plugin")
	if tbl, ok := pluginTable.(*lua.LTable); ok {
		if v := tbl.RawGetString("name"); v != lua.LNil {
			info.Name = v.String()
		}
		if v := tbl.RawGetString("version"); v != lua.LNil {
			info.Version = v.String()
		}
		if v := tbl.RawGetString("description"); v != lua.LNil {
			info.Description = v.String()
		}
	}

	pluginName = info.Name

	// Call init() if it exists.
	initFn := L.GetGlobal("init")
	if initFn != lua.LNil {
		if err := L.CallByParam(lua.P{
			Fn:      initFn,
			NRet:    0,
			Protect: true,
		}); err != nil {
			L.Close()
			return fmt.Errorf("calling init(): %w", err)
		}
	}

	pm.mu.Lock()
	pm.plugins = append(pm.plugins, info)
	pm.states = append(pm.states, L)
	pm.mu.Unlock()

	return nil
}

// registerMahModule sets up the mah.on, mah.inject, mah.log, mah.page, mah.menu,
// and mah.abort functions in the given Lua state. pluginNamePtr is populated by
// loadPlugin after reading the plugin table, before init() is called.
func (pm *PluginManager) registerMahModule(L *lua.LState, pluginNamePtr *string) {
	mahMod := L.NewTable()

	mahMod.RawSetString("on", L.NewFunction(func(L *lua.LState) int {
		eventName := L.CheckString(1)
		handler := L.CheckFunction(2)

		pm.mu.Lock()
		pm.hooks[eventName] = append(pm.hooks[eventName], hookEntry{
			state: L,
			fn:    handler,
		})
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("inject", L.NewFunction(func(L *lua.LState) int {
		slotName := L.CheckString(1)
		renderFn := L.CheckFunction(2)

		pm.mu.Lock()
		pm.injections[slotName] = append(pm.injections[slotName], injectionEntry{
			state: L,
			fn:    renderFn,
		})
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("log", L.NewFunction(func(L *lua.LState) int {
		level := L.CheckString(1)
		message := L.CheckString(2)

		var details map[string]any
		if detailsTbl := L.OptTable(3, nil); detailsTbl != nil {
			details = luaTableToGoMap(detailsTbl)
		}

		if pl := pm.getPluginLogger(); pl != nil {
			pl.PluginLog(*pluginNamePtr, level, message, details)
		} else {
			log.Printf("[plugin][%s] %s", level, message)
		}
		return 0
	}))

	mahMod.RawSetString("abort", L.NewFunction(func(L *lua.LState) int {
		reason := L.CheckString(1)
		L.RaiseError("PLUGIN_ABORT: %s", reason)
		return 0
	}))

	mahMod.RawSetString("get_setting", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		name := *pluginNamePtr

		pm.mu.RLock()
		settings := pm.pluginSettings[name]
		pm.mu.RUnlock()

		if settings == nil {
			L.Push(lua.LNil)
			return 1
		}

		val, ok := settings[key]
		if !ok || val == nil {
			L.Push(lua.LNil)
			return 1
		}

		switch v := val.(type) {
		case string:
			L.Push(lua.LString(v))
		case float64:
			L.Push(lua.LNumber(v))
		case bool:
			L.Push(lua.LBool(v))
		default:
			L.Push(lua.LString(fmt.Sprintf("%v", v)))
		}
		return 1
	}))

	mahMod.RawSetString("page", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		handler := L.CheckFunction(2)

		if !validPagePath.MatchString(path) {
			L.ArgError(1, "invalid page path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
			return 0
		}

		name := *pluginNamePtr
		pm.mu.Lock()
		if pm.pages[name] == nil {
			pm.pages[name] = make(map[string]pageEntry)
		}
		pm.pages[name][path] = pageEntry{state: L, fn: handler}
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("menu", L.NewFunction(func(L *lua.LState) int {
		label := L.CheckString(1)
		path := L.CheckString(2)

		if !validPagePath.MatchString(path) {
			L.ArgError(2, "invalid menu path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
			return 0
		}

		name := *pluginNamePtr
		fullPath := "/plugins/" + name + "/" + path

		pm.mu.Lock()
		pm.menuItems = append(pm.menuItems, MenuRegistration{
			PluginName: name,
			Label:      label,
			FullPath:   fullPath,
		})
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("action", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		action, err := parseActionTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		pm.mu.Lock()
		for _, existing := range pm.actions[*pluginNamePtr] {
			if existing.ID == action.ID {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate action id %q", action.ID))
				return 0
			}
		}
		pm.actions[*pluginNamePtr] = append(pm.actions[*pluginNamePtr], *action)
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("job_progress", L.NewFunction(func(L *lua.LState) int {
		jobID := L.CheckString(1)
		percent := L.CheckInt(2)
		message := L.CheckString(3)

		if percent < 0 {
			percent = 0
		} else if percent > 100 {
			percent = 100
		}

		pm.actionJobsMu.RLock()
		job, ok := pm.actionJobs[jobID]
		pm.actionJobsMu.RUnlock()

		if !ok {
			L.ArgError(1, "unknown job_id")
			return 0
		}

		job.mu.Lock()
		job.Progress = percent
		job.Message = message
		shouldNotify := time.Since(job.lastNotified) >= 200*time.Millisecond || percent >= 100
		if shouldNotify {
			job.lastNotified = time.Now()
		}
		job.mu.Unlock()

		if shouldNotify {
			pm.notifyActionJobSubscribers("updated", job)
		}
		return 0
	}))

	mahMod.RawSetString("job_complete", L.NewFunction(func(L *lua.LState) int {
		jobID := L.CheckString(1)
		resultTbl := L.OptTable(2, nil)

		pm.actionJobsMu.RLock()
		job, ok := pm.actionJobs[jobID]
		pm.actionJobsMu.RUnlock()

		if !ok {
			L.ArgError(1, "unknown job_id")
			return 0
		}

		job.mu.Lock()
		job.Status = "completed"
		job.Progress = 100

		if resultTbl != nil {
			parsed := luaTableToGoMap(resultTbl)
			if msg, hasMsg := parsed["message"].(string); hasMsg {
				job.Message = msg
			} else {
				job.Message = "Completed"
			}
			job.Result = parsed
		} else {
			job.Message = "Completed"
		}
		job.mu.Unlock()

		pm.notifyActionJobSubscribers("updated", job)
		return 0
	}))

	mahMod.RawSetString("job_fail", L.NewFunction(func(L *lua.LState) int {
		jobID := L.CheckString(1)
		errMsg := L.CheckString(2)

		pm.actionJobsMu.RLock()
		job, ok := pm.actionJobs[jobID]
		pm.actionJobsMu.RUnlock()

		if !ok {
			L.ArgError(1, "unknown job_id")
			return 0
		}

		job.mu.Lock()
		job.Status = "failed"
		job.Message = errMsg
		job.mu.Unlock()

		pm.notifyActionJobSubscribers("updated", job)
		return 0
	}))

	// mah.start_job(label, fn) — create an async job and run fn(job_id) in a background goroutine.
	// Returns the job ID immediately. The callback receives the job_id as its argument and can use
	// mah.job_progress, mah.job_complete, mah.job_fail to report status.
	mahMod.RawSetString("start_job", L.NewFunction(func(L *lua.LState) int {
		label := L.CheckString(1)
		fn := L.CheckFunction(2)

		jobID := generateActionJobID()
		job := &ActionJob{
			ID:         jobID,
			Source:     "plugin",
			PluginName: *pluginNamePtr,
			ActionID:   "start_job",
			Label:      label,
			EntityType: "custom",
			Status:     "pending",
			Progress:   0,
			Message:    "Waiting to start...",
			CreatedAt:  time.Now(),
		}

		pm.actionJobsMu.Lock()
		pm.actionJobs[jobID] = job
		pm.actionJobsMu.Unlock()

		pm.notifyActionJobSubscribers("added", job)

		wg := pm.actionWaitGroup(*pluginNamePtr)
		wg.Add(1)

		go func() {
			defer wg.Done()
			pm.runStartJobGoroutine(job, L, fn, jobID)
		}()

		L.Push(lua.LString(jobID))
		return 1
	}))

	pm.registerDbModule(L, mahMod)
	pm.registerHttpModule(L, mahMod)
	pm.registerJsonModule(L, mahMod)
	pm.registerKvModule(L, mahMod, pluginNamePtr)

	L.SetGlobal("mah", mahMod)
}

// DiscoveredPlugins returns a copy of all discovered plugin metadata.
func (pm *PluginManager) DiscoveredPlugins() []DiscoveredPlugin {
	result := make([]DiscoveredPlugin, len(pm.discovered))
	copy(result, pm.discovered)
	return result
}

// GetDiscoveredPlugin returns a pointer to a discovered plugin by name,
// or nil if not found. The discovered list is immutable after construction.
func (pm *PluginManager) GetDiscoveredPlugin(name string) *DiscoveredPlugin {
	for i := range pm.discovered {
		if pm.discovered[i].Name == name {
			return &pm.discovered[i]
		}
	}
	return nil
}

// EnablePlugin activates a discovered plugin by creating a Lua VM and calling init().
// The discovered list is immutable after construction, so no lock is needed to read it.
// loadPlugin handles its own locking for hook/injection/page/menu registration.
func (pm *PluginManager) EnablePlugin(name string) error {
	// Prevent concurrent enable attempts for the same plugin.
	if _, loaded := pm.enabling.LoadOrStore(name, struct{}{}); loaded {
		return fmt.Errorf("plugin %q is already being enabled", name)
	}
	defer pm.enabling.Delete(name)

	pm.mu.RLock()
	for _, p := range pm.plugins {
		if p.Name == name {
			pm.mu.RUnlock()
			return fmt.Errorf("plugin %q is already enabled", name)
		}
	}
	pm.mu.RUnlock()

	// Find in discovered (immutable after construction, no lock needed).
	var dp *DiscoveredPlugin
	for i := range pm.discovered {
		if pm.discovered[i].Name == name {
			dp = &pm.discovered[i]
			break
		}
	}
	if dp == nil {
		return fmt.Errorf("plugin %q not found", name)
	}

	scriptPath := filepath.Join(dp.Dir, "plugin.lua")
	if err := pm.loadPlugin(dp.Dir, scriptPath); err != nil {
		return fmt.Errorf("loading plugin %q: %w", name, err)
	}

	return nil
}

// DisablePlugin deactivates a running plugin: removes all hooks, injections,
// pages, menu items, and closes the Lua VM.
func (pm *PluginManager) DisablePlugin(name string) error {
	pm.mu.Lock()

	var targetState *lua.LState
	var pluginIdx int = -1
	for i, p := range pm.plugins {
		if p.Name == name {
			targetState = pm.states[i]
			pluginIdx = i
			break
		}
	}
	if targetState == nil {
		pm.mu.Unlock()
		return fmt.Errorf("plugin %q is not enabled", name)
	}

	// Remove hooks belonging to this state.
	for event, entries := range pm.hooks {
		var filtered []hookEntry
		for _, e := range entries {
			if e.state != targetState {
				filtered = append(filtered, e)
			}
		}
		pm.hooks[event] = filtered
	}

	// Remove injections belonging to this state.
	for slot, entries := range pm.injections {
		var filtered []injectionEntry
		for _, e := range entries {
			if e.state != targetState {
				filtered = append(filtered, e)
			}
		}
		pm.injections[slot] = filtered
	}

	// Remove pages for this plugin.
	delete(pm.pages, name)

	// Remove menu items for this plugin.
	var filteredMenus []MenuRegistration
	for _, m := range pm.menuItems {
		if m.PluginName != name {
			filteredMenus = append(filteredMenus, m)
		}
	}
	pm.menuItems = filteredMenus

	// Remove actions for this plugin.
	delete(pm.actions, name)

	// Remove from active lists.
	pm.plugins = append(pm.plugins[:pluginIdx], pm.plugins[pluginIdx+1:]...)
	pm.states = append(pm.states[:pluginIdx], pm.states[pluginIdx+1:]...)

	// Remove in-memory settings.
	delete(pm.pluginSettings, name)

	// Grab the in-flight WaitGroup before releasing the lock.
	pm.actionJobsMu.Lock()
	wg := pm.actionInFlight[name]
	delete(pm.actionInFlight, name)
	pm.actionJobsMu.Unlock()

	// Release pm.mu so in-flight goroutines can finish (they need VMLock).
	pm.mu.Unlock()

	if wg != nil {
		wg.Wait()
	}

	// Re-acquire to close state safely, then release.
	pm.mu.Lock()
	delete(pm.vmLocks, targetState)
	targetState.Close()
	pm.mu.Unlock()

	return nil
}

// IsEnabled returns whether a plugin is currently active.
func (pm *PluginManager) IsEnabled(name string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.plugins {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Plugins returns a copy of the loaded plugin info list.
func (pm *PluginManager) Plugins() []PluginInfo {
	result := make([]PluginInfo, len(pm.plugins))
	copy(result, pm.plugins)
	return result
}

// GetHooks returns a copy of the hook entries registered for the given event name.
func (pm *PluginManager) GetHooks(event string) []hookEntry {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	src := pm.hooks[event]
	dst := make([]hookEntry, len(src))
	copy(dst, src)
	return dst
}

// GetInjections returns a copy of the injection entries registered for the given slot name.
func (pm *PluginManager) GetInjections(slot string) []injectionEntry {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	src := pm.injections[slot]
	dst := make([]injectionEntry, len(src))
	copy(dst, src)
	return dst
}

// GetPages returns a flat list of all registered page paths (for diagnostics).
func (pm *PluginManager) GetPages() []PageRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var result []PageRegistration
	for pluginName, pages := range pm.pages {
		for path := range pages {
			result = append(result, PageRegistration{PluginName: pluginName, Path: path})
		}
	}
	return result
}

// HasPage checks if a plugin has registered a page at the given path.
func (pm *PluginManager) HasPage(pluginName, path string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pages, ok := pm.pages[pluginName]; ok {
		_, exists := pages[path]
		return exists
	}
	return false
}

// GetMenuItems returns a copy of all registered menu items.
func (pm *PluginManager) GetMenuItems() []MenuRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]MenuRegistration, len(pm.menuItems))
	copy(result, pm.menuItems)
	return result
}

// SetPluginSettings stores settings for a plugin in memory.
func (pm *PluginManager) SetPluginSettings(pluginName string, settings map[string]any) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pluginSettings[pluginName] = settings
}

// GetPluginSettings returns a shallow copy of the in-memory settings for a plugin.
func (pm *PluginManager) GetPluginSettings(pluginName string) map[string]any {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	orig := pm.pluginSettings[pluginName]
	if orig == nil {
		return nil
	}
	result := make(map[string]any, len(orig))
	for k, v := range orig {
		result[k] = v
	}
	return result
}

// VMLock returns the mutex associated with the given Lua state.
func (pm *PluginManager) VMLock(L *lua.LState) *sync.Mutex {
	return pm.vmLocks[L]
}

// Close shuts down all Lua VMs. After Close returns, hooks and injections
// are no-ops.
func (pm *PluginManager) Close() {
	pm.closed.Store(true)
	pm.httpWg.Wait() // wait for in-flight HTTP goroutines to finish
	close(pm.done)
	for _, L := range pm.states {
		L.Close()
	}
	pm.states = nil
	pm.hooks = nil
	pm.injections = nil
	pm.pages = nil
	pm.menuItems = nil
	pm.actions = nil
	pm.vmLocks = nil
}
