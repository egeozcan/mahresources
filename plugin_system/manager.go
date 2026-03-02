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
	plugins           []PluginInfo
	states            []*lua.LState
	hooks             map[string][]hookEntry
	injections        map[string][]injectionEntry
	pages     map[string]map[string]pageEntry // pluginName -> path -> handler
	menuItems []MenuRegistration
	mu        sync.RWMutex
	// vmLocks is populated during single-threaded initialization (NewPluginManager/loadPlugin)
	// and is read-only afterward, so concurrent reads without locking are safe.
	vmLocks    map[*lua.LState]*sync.Mutex
	dbProvider atomic.Value
	closed     atomic.Bool

	// HTTP async callback support
	httpClient  *http.Client
	httpMu      sync.Mutex
	httpPending []httpCallback
	httpNotify  chan struct{}  // buffered(1), signals new callbacks
	httpStop    chan struct{}  // closed to stop drain goroutine
	httpWg      sync.WaitGroup // tracks in-flight HTTP goroutines
	httpSem     chan struct{}  // concurrency semaphore
}

// NewPluginManager scans dir for subdirectories containing plugin.lua,
// loads each in alphabetical order into an isolated Lua VM, and returns
// the manager. If dir does not exist, an empty manager is returned.
// Must be called from a single goroutine; after it returns the manager
// is safe for concurrent use.
func NewPluginManager(dir string) (*PluginManager, error) {
	pm := &PluginManager{
		hooks:      make(map[string][]hookEntry),
		injections: make(map[string][]injectionEntry),
		pages:      make(map[string]map[string]pageEntry),
		vmLocks:    make(map[*lua.LState]*sync.Mutex),
		httpClient: newHttpClient(),
		httpNotify: make(chan struct{}, 1),
		httpStop:   make(chan struct{}),
		httpSem:    make(chan struct{}, maxConcurrentHttpReqs),
	}

	go pm.drainHttpCallbacks()

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

		if err := pm.loadPlugin(pluginDir, scriptPath); err != nil {
			log.Printf("[plugin] warning: skipping %q: %v", name, err)
		}
	}

	return pm, nil
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

	pm.vmLocks[L] = &sync.Mutex{}

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

	pm.plugins = append(pm.plugins, info)
	pm.states = append(pm.states, L)
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
		log.Printf("[plugin][%s] %s", level, message)
		return 0
	}))

	mahMod.RawSetString("abort", L.NewFunction(func(L *lua.LState) int {
		reason := L.CheckString(1)
		L.RaiseError("PLUGIN_ABORT: %s", reason)
		return 0
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

	pm.registerDbModule(L, mahMod)
	pm.registerHttpModule(L, mahMod)

	L.SetGlobal("mah", mahMod)
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

// VMLock returns the mutex associated with the given Lua state.
func (pm *PluginManager) VMLock(L *lua.LState) *sync.Mutex {
	return pm.vmLocks[L]
}

// Close shuts down all Lua VMs. After Close returns, hooks and injections
// are no-ops.
func (pm *PluginManager) Close() {
	pm.closed.Store(true)
	pm.httpWg.Wait() // wait for in-flight HTTP goroutines to finish
	close(pm.httpStop)
	for _, L := range pm.states {
		L.Close()
	}
	pm.states = nil
	pm.hooks = nil
	pm.injections = nil
	pm.pages = nil
	pm.menuItems = nil
	pm.vmLocks = nil
}
