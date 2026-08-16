package plugin_system

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mahresources/models/block_types"

	lua "github.com/yuin/gopher-lua"
)

// Lua execution timeouts.
const (
	// hookLockWait bounds how long a *nested* hook dispatch waits for another
	// plugin's VM lock before skipping the hook. Matched to the hook's own Lua
	// timeout: a hook that cannot start within the time it is allowed to run is
	// not worth waiting for, and waiting unbounded there is what allows a lock
	// cycle between two goroutines to become permanent.
	hookLockWait = 5 * time.Second

	luaExecTimeout     = 5 * time.Second  // hooks, injections, sync calls
	luaPageTimeout     = 30 * time.Second // plugin page handlers
	asyncActionTimeout = 5 * time.Minute  // async actions and start_job
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
//
// pluginName is captured at registration so a skipped re-entrant dispatch can
// name the plugin in its warning. mah.on is only reachable from init(), which
// loadPlugin calls after populating the name, so it is always set here.
type hookEntry struct {
	state      *lua.LState
	fn         *lua.LFunction
	pluginName string
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
	actions      map[string][]ActionRegistration // pluginName -> actions
	apiEndpoints map[string]map[string]*APIEndpoint // pluginName -> "METHOD:path" -> handler
	blockTypes   map[string][]*PluginBlockType      // pluginName -> block types
	displayTypes map[string][]*PluginDisplayType   // pluginName -> display types
	shortcodes   map[string][]*PluginShortcode     // pluginName -> shortcodes
	docs         map[string][]*PluginDoc           // pluginName -> general doc entries
	mu      sync.RWMutex
	vmLocks map[*lua.LState]*sync.Mutex
	dbProvider atomic.Value
	dbWriter   atomic.Value
	// principalBinder binds dbProvider/dbWriter to the principal that triggered
	// a call. Optional; nil falls back to the unbound provider.
	principalBinder atomic.Value
	logger          atomic.Value
	kvStore      atomic.Value
	mrqlExecutor atomic.Value
	closed       atomic.Bool

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
		apiEndpoints:    make(map[string]map[string]*APIEndpoint),
		blockTypes:      make(map[string][]*PluginBlockType),
		displayTypes:    make(map[string][]*PluginDisplayType),
		shortcodes:      make(map[string][]*PluginShortcode),
		docs:            make(map[string][]*PluginDoc),
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
			state:      L,
			fn:         handler,
			pluginName: *pluginNamePtr,
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

	mahMod.RawSetString("block_type", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		pbt, err := parseBlockTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		pbt.State = L

		pm.mu.Lock()
		for _, existing := range pm.blockTypes[*pluginNamePtr] {
			if existing.TypeName == pbt.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate block type %q", pbt.TypeName))
				return 0
			}
		}
		pm.blockTypes[*pluginNamePtr] = append(pm.blockTypes[*pluginNamePtr], pbt)
		pm.mu.Unlock()

		block_types.RegisterBlockType(pbt)
		return 0
	}))

	mahMod.RawSetString("display_type", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		dt, err := parseDisplayTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		dt.State = L

		pm.mu.Lock()
		for _, existing := range pm.displayTypes[*pluginNamePtr] {
			if existing.TypeName == dt.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate display type %q", dt.TypeName))
				return 0
			}
		}
		pm.displayTypes[*pluginNamePtr] = append(pm.displayTypes[*pluginNamePtr], dt)
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("shortcode", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		sc, err := parseShortcodeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		sc.State = L

		pm.mu.Lock()
		for _, existing := range pm.shortcodes[*pluginNamePtr] {
			if existing.TypeName == sc.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate shortcode %q", sc.TypeName))
				return 0
			}
		}
		pm.shortcodes[*pluginNamePtr] = append(pm.shortcodes[*pluginNamePtr], sc)
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("doc", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		doc := &PluginDoc{PluginName: *pluginNamePtr}

		if v := tbl.RawGetString("name"); v == lua.LNil {
			L.ArgError(1, "missing required field 'name'")
			return 0
		} else if str, ok := v.(lua.LString); !ok {
			L.ArgError(1, fmt.Sprintf("'name' must be a string, got %s", v.Type()))
			return 0
		} else {
			raw := string(str)
			if !validShortcodeName.MatchString(raw) {
				L.ArgError(1, fmt.Sprintf("invalid doc name %q: must match [a-z][a-z0-9_-]{0,49}", raw))
				return 0
			}
			doc.Name = raw
		}

		if v := tbl.RawGetString("label"); v == lua.LNil {
			L.ArgError(1, "missing required field 'label'")
			return 0
		} else {
			doc.Label = v.String()
		}

		if v := tbl.RawGetString("description"); v != lua.LNil {
			doc.Description = v.String()
		}
		if v := tbl.RawGetString("category"); v != lua.LNil {
			doc.Category = v.String()
		}
		if v := tbl.RawGetString("attrs"); v != lua.LNil {
			if attrsTbl, ok := v.(*lua.LTable); ok {
				doc.Attrs = parseDocAttrs(attrsTbl)
			}
		}
		if v := tbl.RawGetString("examples"); v != lua.LNil {
			if exTbl, ok := v.(*lua.LTable); ok {
				doc.Examples = parseDocExamples(exTbl)
			}
		}
		if v := tbl.RawGetString("notes"); v != lua.LNil {
			if notesTbl, ok := v.(*lua.LTable); ok {
				notesTbl.ForEach(func(_, val lua.LValue) {
					if s, ok := val.(lua.LString); ok {
						doc.Notes = append(doc.Notes, string(s))
					}
				})
			}
		}

		pm.mu.Lock()
		// Check name uniqueness against other docs.
		for _, existing := range pm.docs[*pluginNamePtr] {
			if existing.Name == doc.Name {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate doc entry %q", doc.Name))
				return 0
			}
		}
		// Check name uniqueness against shortcodes.
		for _, sc := range pm.shortcodes[*pluginNamePtr] {
			if shortcodeName(sc) == doc.Name {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("doc name %q conflicts with shortcode of the same name", doc.Name))
				return 0
			}
		}
		pm.docs[*pluginNamePtr] = append(pm.docs[*pluginNamePtr], doc)
		pm.mu.Unlock()
		return 0
	}))

	mahMod.RawSetString("api", L.NewFunction(func(L *lua.LState) int {
		method := strings.ToUpper(L.CheckString(1))
		path := L.CheckString(2)
		handler := L.CheckFunction(3)

		switch method {
		case "GET", "POST", "PUT", "DELETE":
		default:
			L.ArgError(1, "method must be GET, POST, PUT, or DELETE")
			return 0
		}

		if !validPagePath.MatchString(path) {
			L.ArgError(2, "invalid api path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
			return 0
		}

		timeout := defaultAPITimeout
		if optsTbl := L.OptTable(4, nil); optsTbl != nil {
			if t, ok := optsTbl.RawGetString("timeout").(lua.LNumber); ok {
				d := time.Duration(float64(t)) * time.Second
				if d > maxAPITimeout {
					d = maxAPITimeout
				}
				if d > 0 {
					timeout = d
				}
			}
		}

		name := *pluginNamePtr
		key := method + ":" + path

		pm.mu.Lock()
		if pm.apiEndpoints[name] == nil {
			pm.apiEndpoints[name] = make(map[string]*APIEndpoint)
		}
		pm.apiEndpoints[name][key] = &APIEndpoint{
			state:   L,
			fn:      handler,
			timeout: timeout,
		}
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
			// start_job runs while an entry point holds this VM's lock, so the
			// triggering user is on L's context here. Without an owner the job
			// is nobody's, and jobVisibleToPrincipal hides an ownerless job from
			// every non-admin — including the user who just triggered it. The
			// async *action* path has always set this; start_job never did.
			ownerUserID: ownerFromInvocation(pm.invocationFor(L)),
		}

		pm.actionJobsMu.Lock()
		pm.actionJobs[jobID] = job
		pm.actionJobsMu.Unlock()

		pm.notifyActionJobSubscribers("added", job)

		wg := pm.actionWaitGroup(*pluginNamePtr)
		wg.Add(1)

		go func() {
			defer wg.Done()
			// mainState: start_job is callable from a coroutine, whose LState is
			// not in vmLocks — the worker would fail the job it just created with
			// "plugin is no longer available".
			pm.runStartJobGoroutine(job, mainState(L), fn, jobID)
		}()

		L.Push(lua.LString(jobID))
		return 1
	}))

	mahMod.RawSetString("html_escape", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&#39;")
		L.Push(lua.LString(s))
		return 1
	}))

	// mah.sleep(seconds) - blocks the current Lua VM for the given duration.
	// Bounded to [0, 30] seconds to prevent abuse. Useful for polling external
	// async APIs (e.g. fal.ai queue) from within a sync action handler.
	mahMod.RawSetString("sleep", L.NewFunction(func(L *lua.LState) int {
		secs := L.CheckNumber(1)
		if secs < 0 {
			secs = 0
		}
		if secs > 30 {
			secs = 30
		}
		time.Sleep(time.Duration(float64(secs) * float64(time.Second)))
		return 0
	}))

	pm.registerDbModule(L, mahMod)
	pm.registerHttpModule(L, mahMod)
	pm.registerJsonModule(L, mahMod)
	pm.registerKvModule(L, mahMod, pluginNamePtr)
	pm.registerImageModule(L, mahMod)
	pm.registerUtilModule(L, mahMod)

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

	// Unregister plugin block types from global registry.
	for _, pbt := range pm.blockTypes[name] {
		block_types.UnregisterBlockType(pbt.TypeName)
	}
	delete(pm.blockTypes, name)

	// Remove display types for this plugin.
	delete(pm.displayTypes, name)

	// Remove shortcodes and general docs for this plugin.
	delete(pm.shortcodes, name)
	delete(pm.docs, name)

	// Remove API endpoints for this plugin.
	delete(pm.apiEndpoints, name)

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

	// Close the state only once nothing is executing on it. Waiting on the action
	// WaitGroup above does not cover hooks, injections, shortcodes, block/display
	// renders, pages or API endpoints, all of which call into this state while
	// holding its VM lock. Taking that lock here waits for whichever of them is
	// in flight, and removing the vmLocks entry while still holding it is what
	// lets LockVM tell a caller that queued behind us to back out instead of
	// running against a closed state.
	//
	// pm.mu must not be held while acquiring the VM lock: a caller holding that
	// lock takes pm.mu.RLock inside LockVM, so the reverse order would deadlock.
	if mu := pm.VMLock(targetState); mu != nil {
		mu.Lock()
		pm.mu.Lock()
		delete(pm.vmLocks, targetState)
		pm.mu.Unlock()
		targetState.Close()
		mu.Unlock()
	}

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
	if pages, ok := pm.pages[pluginName]; ok {
		if _, exists := pages[path]; exists {
			pm.mu.RUnlock()
			return true
		}
	}
	pm.mu.RUnlock()

	// Check auto-generated docs pages.
	return pm.HasDocsPage(pluginName, path)
}

// GetBlockTypes returns all plugin-registered block types.
func (pm *PluginManager) GetBlockTypes() []*PluginBlockType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var result []*PluginBlockType
	for _, types := range pm.blockTypes {
		result = append(result, types...)
	}
	return result
}

// GetDisplayTypes returns all plugin-registered display types.
//
// The counterpart to GetBlockTypes, and it was missing: display types could
// only be looked up by exact name, so nothing could enumerate them. A schema
// author had to hand-type "x-display": "plugin:<n>:<t>" from a README, and a
// typo degraded silently — which is the likeliest reason the whole display-type
// surface has no users, while block types, which have both an accessor and an
// endpoint feeding a picker, do.
func (pm *PluginManager) GetDisplayTypes() []*PluginDisplayType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var result []*PluginDisplayType
	for _, types := range pm.displayTypes {
		result = append(result, types...)
	}
	return result
}

// GetPluginBlockType returns a specific plugin block type by full name, or nil.
func (pm *PluginManager) GetPluginBlockType(fullTypeName string) *PluginBlockType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, types := range pm.blockTypes {
		for _, pbt := range types {
			if pbt.TypeName == fullTypeName {
				return pbt
			}
		}
	}
	return nil
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

// VMLock returns the mutex associated with the given Lua state, or nil when the
// manager no longer tracks that state (it was disabled or the manager closed).
// Every caller must check for nil before locking.
//
// The read is guarded: DisablePlugin deletes from pm.vmLocks under pm.mu, so an
// unguarded read here is a map read racing a map write, which Go can abort the
// process for. The lock is released before returning, so the caller's own
// mu.Lock() does not nest inside pm.mu and cannot invert the ordering
// DisablePlugin relies on when it drops pm.mu to let in-flight goroutines finish.
func (pm *PluginManager) VMLock(L *lua.LState) *sync.Mutex {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.vmLocks[L]
}

// LockVM acquires the VM lock for L and returns it, or returns nil when the
// plugin is gone. A nil return means the caller must not touch L at all: no lock
// is held in that case.
//
// The nil check callers do on VMLock's result is not sufficient on its own,
// because a caller can capture a live mutex and then block on it while
// DisablePlugin closes the state. LState.Close() writes state the in-flight
// L.CallByParam() is reading, so that ordering is a data race and then a nil
// dereference inside gopher-lua. LockVM closes the window by re-checking
// liveness *after* the lock is held: DisablePlugin removes the entry while
// holding this same mutex, so a caller that wins the race sees the entry, and a
// caller that loses it sees the entry gone and backs out.
//
// vmRequestKey carries the caller's own context, undeadlined, inside the
// timeout-wrapped context a VM entry point installs on the LState.
type vmRequestKey struct{}

// vmParentContext is the context a VM entry point should hang its timeout off.
//
// Entry points that serve a request pass r.Context(), so an abandoned request
// cancels the Lua call instead of letting it hold the plugin's VM lock — which
// is exclusive across every other surface of that plugin — for the full
// timeout. It also carries the per-request MRQL cache, which mah.db.mrql_query
// reads off L.Context(). Entry points with no request (hooks, async action
// jobs) pass nil and get Background.
//
// The caller's context is also stashed as a value, so work that must outlive
// the Lua deadline — a sync HTTP call, which is allowed 120s against a 5s Lua
// timeout — can still be cancelled by a client disconnect. See
// vmRequestContext.
func vmParentContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithValue(ctx, vmRequestKey{}, ctx)
}

// vmRequestContext recovers the caller's context from a context installed by
// vmParentContext: cancelled when the request is, but carrying none of the Lua
// deadline. A blocking call that legitimately outlives the Lua timeout hangs
// its own, longer deadline off this rather than off Background, so a client
// that goes away stops it instead of leaving it holding the VM lock.
//
// Falls back to Background, which is the correct answer for a hook or an async
// job: there is no request to be cancelled by.
func vmRequestContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if v, ok := ctx.Value(vmRequestKey{}).(context.Context); ok && v != nil {
		return v
	}
	return context.Background()
}

// Lock ordering is mu then pm.mu, matching DisablePlugin. Nothing may take a VM
// lock while already holding pm.mu.
func (pm *PluginManager) LockVM(L *lua.LState) *sync.Mutex {
	mu := pm.VMLock(L)
	if mu == nil {
		return nil
	}
	mu.Lock()
	pm.mu.RLock()
	_, live := pm.vmLocks[L]
	pm.mu.RUnlock()
	if !live {
		mu.Unlock()
		return nil
	}
	return mu
}

// vmLockPollInterval is how often TryLockVMWithin retries. Only reached under
// genuine contention, and only on the nested path, so a short poll is cheaper
// than the machinery a timed mutex would need.
const vmLockPollInterval = 2 * time.Millisecond

// TryLockVMWithin is LockVM bounded by wait. It returns (nil, false) when the
// plugin is gone — the caller must not touch L — and (nil, true) when the plugin
// is alive but its lock could not be taken in time.
//
// This exists to break lock cycles *between* goroutines, which the invocation
// chain cannot see because a chain is per-call-stack. Two plugins that each hook
// an entity the other writes can arrive at each other's mutex from opposite
// directions: goroutine A holds P and waits for Q while B holds Q and waits for
// P. Both waits are unbounded and the Lua deadline cannot preempt a block inside
// a Go call, so that is permanent — and it is permanent on the code this
// replaces too.
//
// Only the nested case needs bounding, and only the nested case gets it (see
// RunAfterHooks): a dispatch that holds no VM lock cannot be a participant in
// such a cycle, so it keeps waiting as long as it takes, exactly as before.
func (pm *PluginManager) TryLockVMWithin(L *lua.LState, wait time.Duration) (*sync.Mutex, bool) {
	mu := pm.VMLock(L)
	if mu == nil {
		return nil, false
	}
	deadline := time.Now().Add(wait)
	for {
		if mu.TryLock() {
			pm.mu.RLock()
			_, live := pm.vmLocks[L]
			pm.mu.RUnlock()
			if !live {
				mu.Unlock()
				return nil, false
			}
			return mu, false
		}
		if time.Now().After(deadline) {
			// Distinguish "busy" from "gone" here too, not just on the
			// acquiring path. DisablePlugin unregisters hooks before it deletes
			// the vmLocks entry, so a dispatcher working from a hook snapshot
			// taken just before that can sit here waiting on a plugin that is
			// being torn down. Reporting it as contention would fail a caller's
			// write over a hook that no longer exists; a disabled plugin is
			// always a safe skip.
			pm.mu.RLock()
			_, live := pm.vmLocks[L]
			pm.mu.RUnlock()
			return nil, live
		}
		time.Sleep(vmLockPollInterval)
	}
}

// Close shuts down all Lua VMs. After Close returns, hooks and injections
// are no-ops.
//
// Close now blocks on in-flight plugin work, because it acquires each VM lock
// before closing that state. Shutdown latency is therefore bounded by whatever
// is running: up to asyncActionTimeout (5m) for a wedged async action, since
// nothing here waits on actionInFlight the way it waits on httpWg. That is
// deliberate for now (waiting is strictly better than the use-after-close crash
// it replaces), but if shutdown needs a hard ceiling it should get one of its
// own, in the shape DownloadManager.ShutdownDrainTimeout already uses.
func (pm *PluginManager) Close() {
	pm.closed.Store(true)
	pm.httpWg.Wait() // wait for in-flight HTTP goroutines to finish
	close(pm.done)

	// Same lifecycle DisablePlugin uses, for the same reason: pm.closed is
	// checked on the way in, so a render or async action that passed that check
	// can still be inside L.CallByParam here. Closing under it is a data race and
	// then a nil dereference inside gopher-lua. Take each VM lock (which waits for
	// whatever is running), drop the vmLocks entry while holding it so anyone
	// queued behind us backs out of LockVM, then close.
	//
	// pm.mu is not held across the VM lock: LockVM takes pm.mu.RLock while holding
	// the VM lock, so the reverse order would deadlock.
	pm.mu.RLock()
	states := make([]*lua.LState, len(pm.states))
	copy(states, pm.states)
	pm.mu.RUnlock()

	for _, L := range states {
		mu := pm.VMLock(L)
		if mu == nil {
			continue
		}
		mu.Lock()
		pm.mu.Lock()
		delete(pm.vmLocks, L)
		pm.mu.Unlock()
		L.Close()
		mu.Unlock()
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.states = nil
	pm.hooks = nil
	pm.injections = nil
	pm.pages = nil
	pm.menuItems = nil
	pm.actions = nil
	pm.blockTypes = nil
	pm.displayTypes = nil
	pm.shortcodes = nil
	pm.apiEndpoints = nil
	pm.vmLocks = nil
}
