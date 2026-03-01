package plugin_system

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	lua "github.com/yuin/gopher-lua"
)

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

// PluginManager loads and manages Lua plugins.
type PluginManager struct {
	plugins    []PluginInfo
	states     []*lua.LState
	hooks      map[string][]hookEntry
	injections map[string][]injectionEntry
	mu sync.RWMutex
	// vmLocks is populated during single-threaded initialization (NewPluginManager/loadPlugin)
	// and is read-only afterward, so concurrent reads without locking are safe.
	vmLocks    map[*lua.LState]*sync.Mutex
	dbProvider atomic.Value
	closed     atomic.Bool

	// HTTP async callback support
	httpClient  *http.Client
	httpMu      sync.Mutex
	httpPending []httpCallback
	httpNotify  chan struct{} // buffered(1), signals new callbacks
	httpStop    chan struct{} // closed to stop drain goroutine
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
		vmLocks:    make(map[*lua.LState]*sync.Mutex),
		httpClient: newHttpClient(),
		httpNotify: make(chan struct{}, 1),
		httpStop:   make(chan struct{}),
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

	// Register the mah module.
	pm.registerMahModule(L)

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

// registerMahModule sets up the mah.on, mah.inject, mah.log, and mah.abort
// functions in the given Lua state.
func (pm *PluginManager) registerMahModule(L *lua.LState) {
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

// VMLock returns the mutex associated with the given Lua state.
func (pm *PluginManager) VMLock(L *lua.LState) *sync.Mutex {
	return pm.vmLocks[L]
}

// Close shuts down all Lua VMs. After Close returns, hooks and injections
// are no-ops.
func (pm *PluginManager) Close() {
	pm.closed.Store(true)
	close(pm.httpStop)
	for _, L := range pm.states {
		L.Close()
	}
	pm.states = nil
	pm.hooks = nil
	pm.injections = nil
	pm.vmLocks = nil
}
