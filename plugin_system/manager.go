package plugin_system

import (
	"bytes"
	"context"
	"errors"
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

	// pluginHeaderTimeout bounds the top-level code executed to read a
	// plugin's manifest. That read happens at boot for every plugin directory,
	// enabled or not, so an unbounded one lets a single runaway plugin.lua hold
	// the whole server's startup.
	pluginHeaderTimeout = 5 * time.Second

	// pluginLoadTimeout bounds a plugin's own load: its top-level code and its
	// init(). Both hold the plugin's VM lock and the enable request, so an
	// unbounded one holds a request and a core until the process dies.
	pluginLoadTimeout = 30 * time.Second

	// maxPluginSourceSize caps a plugin.lua. The file is read whole, twice per
	// load, at boot, for every plugin directory — including ones nobody
	// enabled. The largest bundled plugin is under 100 KB.
	maxPluginSourceSize = 4 << 20

	// retireDrainTimeout bounds how long a teardown waits for a plugin's
	// in-flight async work before giving up on closing its VM.
	//
	// A failing init() that already started a job would otherwise block the
	// enable request for the job's full 5-minute allowance, with the plugin's
	// name still claimed in `enabling` so a retry is refused. Past the bound the
	// state is left open — leaking one LState until the process exits — rather
	// than closed underneath a worker that is still executing on it. Its
	// registrations are already gone, so nothing can reach it.
	retireDrainTimeout = 5 * time.Second

	luaExecTimeout     = 5 * time.Second  // hooks, injections, sync calls
	luaPageTimeout     = 30 * time.Second // plugin page handlers
	asyncActionTimeout = 5 * time.Minute  // async actions and start_job
)

var validPagePath = regexp.MustCompile(`^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)

// validPluginName is the shortcode grammar: a plugin's name prefixes every
// shortcode it registers ([plugin:<name>:<code>]), so a name outside it
// produces shortcodes that cannot be written.
var validPluginName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)

// PluginInfo holds metadata about a loaded plugin.
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	Dir         string
	Manifest    Manifest
}

// DiscoveredPlugin holds metadata about a discovered (but not necessarily loaded) plugin.
type DiscoveredPlugin struct {
	Name        string
	Version     string
	Description string
	Dir         string
	Settings    []SettingDefinition
	Manifest    Manifest
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
	// plugin is carried on the entry rather than resolved from the state at
	// render time, because RenderSlot has to decide per plugin whether a
	// group-limited caller may see this injection at all, once per slot per
	// page — and mapping a state back to a name is a walk under the manager
	// lock.
	plugin string
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

	// state is the VM that registered this entry; see ActionRegistration.
	state *lua.LState
}

// PluginManager loads and manages Lua plugins.
type PluginManager struct {
	plugins      []PluginInfo
	states       []*lua.LState
	hooks        map[string][]hookEntry
	injections   map[string][]injectionEntry
	pages        map[string]map[string]pageEntry // pluginName -> path -> handler
	menuItems    []MenuRegistration
	actions      map[string][]ActionRegistration    // pluginName -> actions
	apiEndpoints map[string]map[string]*APIEndpoint // pluginName -> "METHOD:path" -> handler
	blockTypes   map[string][]*PluginBlockType      // pluginName -> block types
	displayTypes map[string][]*PluginDisplayType    // pluginName -> display types
	shortcodes   map[string][]*PluginShortcode      // pluginName -> shortcodes
	docs         map[string][]*PluginDoc            // pluginName -> general doc entries
	mu           sync.RWMutex
	vmLocks      map[*lua.LState]*vmMutex
	dbProvider   atomic.Value
	dbWriter     atomic.Value
	// principalBinder binds dbProvider/dbWriter to the principal that triggered
	// a call. Optional; nil falls back to the unbound provider.
	principalBinder atomic.Value
	logger          atomic.Value
	kvStore         atomic.Value
	mrqlExecutor    atomic.Value
	// consent holds the persistent ConsentStore. Unset until wiring, which is
	// why fallbackConsent exists: an unwired manager must still enforce, not
	// silently skip the check.
	consent         atomic.Value
	fallbackConsent *memoryConsentStore

	// egressClients holds one http.Client per distinct network policy. Keyed on
	// the policy fingerprint, not the plugin: see httpClientFor.
	egressMu      sync.RWMutex
	egressClients map[string]*http.Client
	closed        atomic.Bool

	// unknownDispatchWarned holds the hook events and injection slots already
	// reported as outside the catalogue, so one host typo costs one log line
	// rather than one per write. See reportUnknownDispatch.
	unknownDispatchWarned sync.Map

	// loadWg tracks loads in progress. A loading VM is in vmLocks but not yet in
	// states, so a Close that only walks states would leave it open — and the
	// load would then publish into the maps Close had just niled.
	loadWg sync.WaitGroup

	// closeDone guards the one-shot channel close in Close.
	closeDone sync.Once

	// loading tracks in-flight loads by name, so a disable can wait for one
	// instead of racing it.
	//
	// A loading plugin is not in pm.plugins, so DisablePlugin could not see it,
	// answered "not enabled", and the caller persisted enabled=false — after
	// which the load published and the plugin ran on with every granted host
	// function while the database said it was off.
	//
	// The first attempt at this recorded the disable and had the publish step
	// abandon. That needed three separate maps to agree across four call sites,
	// and they did not: a second enable could clear the marker before failing,
	// publication could win the race against recording it, and a wedged init()
	// left the plugin both undisableable and un-enablable. Waiting is one
	// mechanism instead of three, and it answers truthfully — including when it
	// cannot answer at all.
	loading map[string]chan struct{} // pluginName -> closed when the load ends

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

	// HTTP async callback support.
	//
	// Pending callbacks are keyed by the VM that has to run them, not held in
	// one list, because one list is one queue and a queue moves at the speed of
	// its slowest entry. A plugin inside a 120-second synchronous call would
	// otherwise stall every other plugin's callbacks behind its own.
	// httpDraining names the VMs that already have a worker, so a VM keeps
	// exactly one and its callbacks stay in the order they were queued.
	httpMu       sync.Mutex
	httpPending  map[*lua.LState][]httpCallback
	httpDraining map[*lua.LState]bool
	httpNotify   chan struct{}  // buffered(1), signals new callbacks
	done         chan struct{}  // closed to stop background goroutines (HTTP drain, job cleanup)
	httpWg       sync.WaitGroup // tracks in-flight HTTP goroutines
	httpSem      chan struct{}  // concurrency semaphore
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
		vmLocks:         make(map[*lua.LState]*vmMutex),
		pluginSettings:  make(map[string]map[string]any),
		actionJobs:      make(map[string]*ActionJob),
		actionSemaphore: make(chan struct{}, maxConcurrentActions),
		actionSubs:      make(map[chan ActionJobEvent]struct{}),
		actionInFlight:  make(map[string]*sync.WaitGroup),
		loading:         make(map[string]chan struct{}),
		fallbackConsent: newMemoryConsentStore(),
		httpPending:     make(map[*lua.LState][]httpCallback),
		httpDraining:    make(map[*lua.LState]bool),
		httpNotify:      make(chan struct{}, 1),
		done:            make(chan struct{}),
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

	// A plugin's name — not its directory — is the key that its enabled state,
	// its settings, its KV namespace, its job ownership and (in due course) its
	// consent all hang off.
	//
	// So a contested name cannot be awarded to anybody. Electing one claimant
	// (the first by directory name, say) is an identity takeover: a directory
	// called "a-thing" that declares the name of a plugin in "z-thing" would
	// inherit that plugin's persisted enabled flag, its stored settings — API
	// keys included, and mah.get_setting needs no capability — and its KV
	// namespace. Both are dropped, and the operator is told which directories
	// to look at.
	claimed := make(map[string]string, len(pluginDirs))
	contested := make(map[string]bool)
	var discovered []DiscoveredPlugin
	for _, name := range pluginDirs {
		pluginDir := filepath.Join(dir, name)
		scriptPath := filepath.Join(pluginDir, "plugin.lua")
		dp, err := pm.discoverPlugin(pluginDir, scriptPath)
		if err != nil {
			log.Printf("[plugin] warning: skipping %q: %v", name, err)
			continue
		}
		if first, taken := claimed[dp.Name]; taken {
			log.Printf("[plugin] warning: skipping both %q and %q: they both call themselves %q, and a "+
				"plugin's name is what its settings, stored data and enabled state belong to",
				first, name, dp.Name)
			contested[dp.Name] = true
			continue
		}
		claimed[dp.Name] = name
		discovered = append(discovered, dp)
	}
	surviving := make([]DiscoveredPlugin, 0, len(discovered))
	for _, dp := range discovered {
		if contested[dp.Name] {
			continue
		}
		surviving = append(surviving, dp)
	}

	// A dependency is satisfied only by an *enabled* plugin, so a cycle can
	// never be satisfied by anything: each member is waiting for another member
	// that is waiting for it. Left in the discovered set it would be a plugin
	// the UI offers and every enable refuses, with a message that names a
	// plugin the operator can see is installed. Rejecting it at discovery makes
	// it a packaging error, which is what it is.
	//
	// Only the members of the cycle are dropped. A plugin that merely *depends*
	// on one survives and is refused at enable, naming the dependency it cannot
	// get — a better diagnosis than vanishing from the list.
	inCycle := pluginsOnADependencyCycle(surviving)
	for _, dp := range surviving {
		if inCycle[dp.Name] {
			log.Printf("[plugin] warning: skipping %q: its dependencies form a cycle (%s), and a "+
				"dependency is only satisfied by an enabled plugin, so no member of a cycle can ever load",
				dp.Name, strings.Join(dp.Manifest.Dependencies, ", "))
			continue
		}
		pm.discovered = append(pm.discovered, dp)
	}

	return pm, nil
}

// pluginsOnADependencyCycle returns the names that sit on a dependency cycle.
//
// Two prunings, because "can reach a cycle" and "is on a cycle" are different
// questions and only the second should cost a plugin its place in the list:
//
//  1. Repeatedly drop nodes with no outgoing edge. What remains is everything
//     that can reach a cycle.
//  2. Within that, repeatedly drop nodes with no incoming edge. What remains is
//     also reachable *from* a cycle — and a node both on a path to a cycle and
//     on a path from one is on the cycle itself.
//
// Edges to names that were never discovered are ignored: an unmet dependency is
// a different failure, diagnosed at enable.
func pluginsOnADependencyCycle(plugins []DiscoveredPlugin) map[string]bool {
	deps := make(map[string][]string, len(plugins))
	present := make(map[string]bool, len(plugins))
	for _, dp := range plugins {
		present[dp.Name] = true
	}
	for _, dp := range plugins {
		for _, dep := range dp.Manifest.Dependencies {
			if present[dep] {
				deps[dp.Name] = append(deps[dp.Name], dep)
			}
		}
	}

	remaining := make(map[string]bool, len(plugins))
	for name := range present {
		remaining[name] = true
	}

	// (1) drop anything that depends on nothing still standing.
	for {
		dropped := false
		for name := range remaining {
			live := false
			for _, dep := range deps[name] {
				if remaining[dep] {
					live = true
					break
				}
			}
			if !live {
				delete(remaining, name)
				dropped = true
			}
		}
		if !dropped {
			break
		}
	}

	// (2) drop anything nothing still standing depends on.
	for {
		dropped := false
		for name := range remaining {
			needed := false
			for other := range remaining {
				for _, dep := range deps[other] {
					if dep == name {
						needed = true
						break
					}
				}
				if needed {
					break
				}
			}
			if !needed {
				delete(remaining, name)
				dropped = true
			}
		}
		if !dropped {
			break
		}
	}

	return remaining
}

// ErrDependencyNotEnabled is returned when a plugin names a dependency that is
// installed but not currently enabled.
var ErrDependencyNotEnabled = errors.New("a dependency is not enabled")

// ErrDependencyInUse is returned when disabling a plugin that another enabled
// plugin depends on.
var ErrDependencyInUse = errors.New("another enabled plugin depends on this one")

// checkDependencies enforces the only honest semantic available without a Lua
// module loader: the named plugin must be enabled right now.
func (pm *PluginManager) checkDependencies(dp DiscoveredPlugin) error {
	for _, dep := range dp.Manifest.Dependencies {
		if pm.GetDiscoveredPlugin(dep) == nil {
			return fmt.Errorf("plugin %q depends on %q, which is not installed", dp.Name, dep)
		}
		if !pm.IsEnabled(dep) {
			return fmt.Errorf("%w: plugin %q depends on %q; enable %q first",
				ErrDependencyNotEnabled, dp.Name, dep, dep)
		}
	}
	return nil
}

// dependentsOf returns the enabled plugins that declare a dependency on name.
func (pm *PluginManager) dependentsOf(name string) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var out []string
	for _, p := range pm.plugins {
		for _, dep := range p.Manifest.Dependencies {
			if dep == name {
				out = append(out, p.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// unsafeBaseFunctions are removed from every VM a plugin's code runs in.
//
// dofile and loadfile open an arbitrary path; load and loadstring compile
// arbitrary source. A plugin already runs arbitrary Lua, so in the load VM
// these are tidiness — but the *header* VM executes top-level code at boot for
// every plugin directory, enabled or not, and there loadfile is an
// arbitrary-path open by a plugin nobody turned on.
var unsafeBaseFunctions = []string{"dofile", "loadfile", "load", "loadstring"}

// openPluginLibraries opens the safe standard libraries (excluding os, io,
// debug and package) in a plugin VM.
//
// The manifest read and the real run must open exactly the same set, or the
// difference is a discriminator: a plugin could declare one manifest when
// `coroutine` is present and another when it is not, the same way it once could
// with `mah`.
func openPluginLibraries(L *lua.LState) {
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
}

func removeUnsafeBaseFunctions(L *lua.LState) {
	for _, name := range unsafeBaseFunctions {
		L.SetGlobal(name, lua.LNil)
	}
}

// readPluginSource reads a plugin.lua, refusing one that is implausibly large.
func readPluginSource(scriptPath string) ([]byte, error) {
	info, err := os.Stat(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("reading plugin.lua: %w", err)
	}
	if info.Size() > maxPluginSourceSize {
		return nil, fmt.Errorf("plugin.lua is %d bytes, over the %d-byte limit", info.Size(), maxPluginSourceSize)
	}
	code, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("reading plugin.lua: %w", err)
	}
	if len(code) > maxPluginSourceSize {
		return nil, fmt.Errorf("plugin.lua is %d bytes, over the %d-byte limit", len(code), maxPluginSourceSize)
	}
	return code, nil
}

// pluginHeader is everything readable from a plugin.lua without giving it the
// mah table: its identity, its manifest and its settings definitions.
type pluginHeader struct {
	Name        string
	Version     string
	Description string
	Manifest    Manifest
	Settings    []SettingDefinition
}

// readPluginHeader executes a plugin.lua's top-level code in a throwaway VM
// that has no mah table, and reads what the plugin declares about itself.
//
// It takes the source rather than a path because the caller must read the file
// exactly once: loading parses the manifest and then runs the same code, and
// two reads would leave a window in which the grants belong to a version of the
// file that is no longer the one being executed.
func readPluginHeader(code []byte, chunkName string) (pluginHeader, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	openPluginLibraries(L)
	removeUnsafeBaseFunctions(L)

	ctx, cancel := context.WithTimeout(context.Background(), pluginHeaderTimeout)
	defer cancel()
	L.SetContext(ctx)

	fn, err := L.Load(bytes.NewReader(code), chunkName)
	if err != nil {
		return pluginHeader{}, fmt.Errorf("parsing plugin.lua: %w", err)
	}
	L.Push(fn)
	err = L.PCall(0, lua.MultRet, nil)

	// Same reason as the load: everything below reads this VM from Go through
	// _G, which honours metamethods, and none of it is inside a PCall. Here it
	// matters more — this runs at boot for every plugin directory, enabled or
	// not, so `setmetatable(_G, {__index = function() error("boom") end})` in a
	// file nobody turned on would panic the process during startup, and
	// disabling the plugin would not help because discovery does not consult
	// enablement.
	L.G.Global.Metatable = lua.LNil

	if err != nil {
		if ctx.Err() != nil {
			return pluginHeader{}, fmt.Errorf("plugin.lua did not finish its top-level code within %s: "+
				"registration belongs in init(), and top-level code must return promptly", pluginHeaderTimeout)
		}
		// The one error a plugin author hits by accident, worth naming: mah is
		// deliberately absent here, so a top-level mah call lands as a nil index.
		if strings.Contains(err.Error(), "attempt to index a non-table object") ||
			strings.Contains(err.Error(), "attempt to call a non-function object") {
			return pluginHeader{}, fmt.Errorf("executing plugin.lua's top-level code: %w "+
				"(note: mah is only available inside init() and the handlers it registers)", err)
		}
		return pluginHeader{}, fmt.Errorf("parsing plugin.lua: %w", err)
	}

	h, err := readPluginIdentity(L)
	if err != nil {
		return pluginHeader{}, err
	}
	h.Settings = extractSettingsFromState(L)
	return h, nil
}

// readPluginIdentity reads name, version, description and manifest off a VM's
// `plugin` global.
func readPluginIdentity(L *lua.LState) (pluginHeader, error) {
	var h pluginHeader
	pluginTable := L.GetGlobal("plugin")
	tbl, ok := pluginTable.(*lua.LTable)
	if !ok {
		// A plugin with no identity cannot be enabled, named in a warning, or
		// consented to. It used to load anonymously and register under the
		// empty name.
		return pluginHeader{}, fmt.Errorf("plugin.lua must define a `plugin` table")
	}
	if v := tbl.RawGetString("name"); v != lua.LNil {
		h.Name = v.String()
	}
	if v := tbl.RawGetString("version"); v != lua.LNil {
		h.Version = v.String()
	}
	if v := tbl.RawGetString("description"); v != lua.LNil {
		h.Description = v.String()
	}
	if strings.TrimSpace(h.Name) == "" {
		return pluginHeader{}, fmt.Errorf("plugin.lua must define plugin.name")
	}
	// The name is a map key, a URL segment in every menu href, and the prefix
	// of every shortcode this plugin registers. It was never checked, so a
	// plugin called "My Plugin" registered shortcodes no author could invoke
	// (the shortcode grammar would never match) and put a space in a URL.
	if !validPluginName.MatchString(h.Name) {
		return pluginHeader{}, fmt.Errorf("plugin name %q is not usable: it must match %s, because it is "+
			"a URL segment and the prefix of every shortcode the plugin registers", h.Name, validPluginName)
	}
	// A malformed manifest is a hard failure, not a silent legacy fallback: a
	// typo'd capability that granted nothing would surface as "attempt to call
	// a nil value" deep inside init(), and a typo'd host as a request that
	// mysteriously never reaches anything.
	manifest, err := ParseManifest(tbl)
	if err != nil {
		return pluginHeader{}, fmt.Errorf("reading manifest: %w", err)
	}
	h.Manifest = manifest
	return h, nil
}

// discoverPlugin reads a plugin's metadata and settings without running init().
func (pm *PluginManager) discoverPlugin(pluginDir, scriptPath string) (DiscoveredPlugin, error) {
	code, err := readPluginSource(scriptPath)
	if err != nil {
		return DiscoveredPlugin{}, err
	}
	h, err := readPluginHeader(code, scriptPath)
	if err != nil {
		return DiscoveredPlugin{}, err
	}
	return DiscoveredPlugin{
		Name:        h.Name,
		Version:     h.Version,
		Description: h.Description,
		Dir:         pluginDir,
		Settings:    h.Settings,
		Manifest:    h.Manifest,
	}, nil
}

// loadPlugin creates a Lua VM, runs plugin.lua's top-level code, installs the
// granted subset of the mah module, and calls init().
//
// mah is installed *after* the top-level code, not before. That ordering is the
// whole safety argument:
//
//   - The manifest is read by executing the same bytes in a throwaway VM that
//     has no mah. If the real VM had mah during its top-level run, the file
//     could tell the two apart — `if mah then ... else ... end` — and declare a
//     narrow manifest to a human reading the source while the read that decides
//     the grants took the other branch. Running both without mah makes the two
//     environments identical, so the declaration cannot depend on which run it
//     is in.
//   - Top-level code then cannot call mah at all, so it cannot commit side
//     effects before the manifest it declared has been checked. Refusing a load
//     afterwards can roll back registrations; it cannot roll back a created
//     tag.
//
// Registration therefore belongs in init(), which is what the documentation has
// always said and what the manifest read has always enforced by accident.
func (pm *PluginManager) loadPlugin(dp DiscoveredPlugin) error {
	pluginDir := dp.Dir
	scriptPath := filepath.Join(pluginDir, "plugin.lua")

	code, err := readPluginSource(scriptPath)
	if err != nil {
		return err
	}
	header, err := readPluginHeader(code, scriptPath)
	if err != nil {
		return err
	}
	if header.Name != dp.Name {
		// The operator enabled a name. Loading a file that now calls itself
		// something else would register it under an identity nobody chose.
		return fmt.Errorf("plugin.lua now declares the name %q, not %q; restart the server to re-read the plugin directory",
			header.Name, dp.Name)
	}
	if !header.Manifest.Equal(dp.Manifest) {
		// The same argument as the name. The discovered list is what the manage
		// UI renders and what an operator is looking at when they enable
		// something; loading a different capability list than the one on screen
		// makes that display a lie for the rest of the process's life.
		return fmt.Errorf("plugin.lua declares different capabilities than it did at startup; " +
			"restart the server to re-read the plugin directory")
	}
	if header.Manifest.APIVersion > PluginAPIVersion {
		return fmt.Errorf("plugin declares api_version %d, but this server implements %d",
			header.Manifest.APIVersion, PluginAPIVersion)
	}

	if header.Manifest.AllowPrivateHosts {
		// Worth a line on its own every load: it is the one declaration that
		// lets a plugin reach the machine the server runs on.
		log.Printf("[plugin] %s may reach private network addresses named in its allowlist (%s)",
			header.Name, strings.Join(header.Manifest.NetworkDisplay(), ", "))
	}
	if header.Manifest.MinAppVersion != "" {
		// Recorded and shown, never enforced: there is no application version
		// constant to compare against, and inventing one nobody maintains
		// would be worse than saying so.
		log.Printf("[plugin] %s declares min_app_version %q, which this server does not check",
			header.Name, header.Manifest.MinAppVersion)
	}

	grants := header.Manifest.Capabilities()
	logGrants(header.Name, header.Manifest, grants)

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	openPluginLibraries(L)
	removeUnsafeBaseFunctions(L)

	// The load holds this VM's lock from here until the plugin is published.
	//
	// init() runs Lua on L, and mah.start_job hands the same L to a worker
	// goroutine that takes this lock before touching it. Without holding it,
	// that worker finds the lock free and runs concurrently with init() —
	// two goroutines inside one gopher-lua state, which corrupts its stack.
	vmLock := newVMMutex()
	pm.mu.Lock()
	if pm.closed.Load() {
		// Registering under pm.mu, with the closed check inside it, is what
		// makes this exclusive with Close: either Close sees this load in
		// loadWg and waits for it, or this load sees closed and never starts.
		pm.mu.Unlock()
		L.Close()
		return fmt.Errorf("plugin manager is shutting down")
	}
	pm.vmLocks[L] = vmLock
	pm.loadWg.Add(1)
	loadDone := make(chan struct{})
	pm.loading[header.Name] = loadDone
	pm.mu.Unlock()
	defer func() {
		pm.mu.Lock()
		if pm.loading[header.Name] == loadDone {
			delete(pm.loading, header.Name)
		}
		pm.mu.Unlock()
		close(loadDone)
		pm.loadWg.Done()
	}()
	vmLock.Lock()

	// abandon revokes and unregisters *while still holding the VM lock*, then
	// releases it and tears down.
	//
	// The order matters: a request can already be queued on this lock. Released
	// first, that request acquires it, finds the liveness entry still present,
	// and executes a page or endpoint belonging to a plugin whose load just
	// failed. Revoking first means it re-checks, finds the state gone, and
	// backs out — which is what LockVM's post-acquire check is for.
	abandon := func() {
		pm.mu.Lock()
		mu, owned := pm.revokeLocked(L)
		pm.unregisterPluginLocked(header.Name, L)
		pm.mu.Unlock()
		vmLock.Unlock()
		if owned {
			pm.finishTeardown(header.Name, L, mu)
		}
	}

	// The top-level run is bounded. The header read has its own deadline, but
	// this one holds the VM lock too, so a plugin that never returns holds the
	// enable request and a core with it.
	//
	// The deadline covers only the top-level code, and is dropped before
	// init(). gopher-lua copies the parent's context into a coroutine when it
	// is created and never refreshes it, so a context still set across init()
	// would be inherited by any coroutine created there and then cancelled out
	// from under it when the load finished — see
	// TestInvocation_CoroutineWriteUsesTheCurrentRequestActor, which pins
	// exactly that shape. init() is therefore unbounded, as it has always been:
	// a plugin that hangs there hangs its own enable request, and an operator
	// who installed the file could have hung the process a dozen other ways.
	loadCtx, cancelLoad := context.WithTimeout(context.Background(), pluginLoadTimeout)
	L.SetContext(loadCtx)

	// Execute plugin.lua — the same bytes the manifest was read from, in the
	// same mah-less environment the manifest was read in.
	fn, err := L.Load(bytes.NewReader(code), scriptPath)
	if err != nil {
		cancelLoad()
		abandon()
		return fmt.Errorf("executing plugin.lua: %w", err)
	}
	L.Push(fn)
	err = L.PCall(0, lua.MultRet, nil)

	// Everything below reaches into this VM from Go — reading `plugin`,
	// installing `mah`, fetching `init` — and those go through _G, which
	// honours metamethods. A plugin that leaves a metatable on _G would have
	// __index/__newindex run as Lua *outside* any PCall and, in a moment, with
	// no deadline: an error there becomes a panic out of EnablePlugin, and a
	// loop there hangs the load with the VM lock held. Nothing legitimate needs
	// a metatable on the globals table.
	L.G.Global.Metatable = lua.LNil

	L.RemoveContext()
	cancelLoad()
	if err != nil {
		abandon()
		return fmt.Errorf("executing plugin.lua: %w", err)
	}

	// Both runs saw the same bytes in the same environment, so a disagreement
	// now means the declaration is not a function of the file — it varies run
	// to run. Nothing may be granted on that basis.
	live, err := readPluginIdentity(L)
	if err != nil || !live.Manifest.Equal(header.Manifest) ||
		live.Name != header.Name || live.Version != header.Version || live.Description != header.Description {
		abandon()
		return fmt.Errorf("plugin.lua declares something different each time it runs; " +
			"its name, version, description and manifest must be the same on every execution")
	}

	// Consent is checked here, against header.Manifest — the load-time
	// declaration, now proved equal to both the discovered copy and to what
	// this very VM just declared. Checking DiscoveredPlugin.Manifest instead
	// would verify one manifest and install the grants of another.
	//
	// It is the last thing before registerMahModule, which is the only place a
	// mah key is ever set, and init() at the bottom of this function is the
	// first Lua that could observe one. So a refusal here has granted nothing.
	if err := pm.enforceConsent(header.Name, header.Manifest); err != nil {
		abandon()
		return err
	}

	// Only now does the plugin get its host functions, and only the granted
	// ones. Closures in registerMahModule capture the name pointer.
	pluginName := header.Name
	pm.registerMahModule(L, &pluginName, grants, header.Manifest.NetworkPolicy())

	info := PluginInfo{
		Name:        header.Name,
		Version:     header.Version,
		Description: header.Description,
		Dir:         pluginDir,
		Manifest:    header.Manifest,
	}

	// Call init() if it exists.
	//
	// Unbounded, for the coroutine reason above — so if it hangs, say which
	// plugin is hanging. Boot enables plugins before the listener is bound, and
	// the symptom is otherwise a server that never starts and never explains.
	stuck := time.AfterFunc(pluginLoadTimeout, func() {
		log.Printf("[plugin] warning: %s has been inside init() for over %s and is holding its load "+
			"(and, at startup, the server's start-up) — it will not be interrupted",
			header.Name, pluginLoadTimeout)
	})
	defer stuck.Stop()

	initFn := L.GetGlobal("init")
	if initFn != lua.LNil {
		if err := L.CallByParam(lua.P{
			Fn:      initFn,
			NRet:    0,
			Protect: true,
		}); err != nil {
			// Registrations made before the error must go with it. Left
			// behind, they name a state that is about to be closed, and the
			// next render of one calls into a closed LState — a segfault
			// inside gopher-lua, from a plugin that merely failed to load.
			abandon()
			return fmt.Errorf("calling init(): %w", err)
		}
	}

	pm.mu.Lock()
	if pm.closed.Load() {
		// The manager shut down while this plugin was loading. Publishing now
		// would leave a plugin nobody can disable and a VM nobody closes.
		pm.mu.Unlock()
		abandon()
		return fmt.Errorf("plugin manager is shutting down")
	}
	pm.plugins = append(pm.plugins, info)
	pm.states = append(pm.states, L)
	pm.mu.Unlock()

	// Published: anything that queued behind this lock may now run.
	vmLock.Unlock()

	return nil
}

// jobOwnedBy looks up a job and confirms it belongs to the calling plugin.
//
// The reporters take a job id and find it in one process-wide map, so without
// this a plugin could complete or fail another plugin's work — including work
// it never had any capability to create.
func (pm *PluginManager) jobOwnedBy(jobID, pluginName string) (*ActionJob, bool) {
	pm.actionJobsMu.RLock()
	job, ok := pm.actionJobs[jobID]
	pm.actionJobsMu.RUnlock()
	if !ok || job.PluginName != pluginName {
		return nil, false
	}
	return job, true
}

// logGrants records what a plugin was given, and what it was not.
//
// A plugin with no manifest gets everything, which is the one case an operator
// most needs told: it is the state the manifest exists to replace, and it is
// invisible in the code. For a plugin that does declare one, the withheld
// modules are named because an ungranted key is simply absent — which keeps
// `if mah.kv then` working — so the only other symptom is "attempt to index a
// nil value" somewhere inside the plugin.
func logGrants(name string, manifest Manifest, grants CapabilitySet) {
	if !manifest.Declared {
		log.Printf("[plugin] %s: no manifest — running with full access to every mah module. "+
			"Add `api_version = %d` and a `capabilities` list to limit it; unmanifested plugins "+
			"stop being accepted at the next api_version bump.", name, PluginAPIVersion)
		return
	}
	// The job_* reporters come with either actions or jobs, so naming them on
	// the withheld line for one while the other is granted tells the operator a
	// function is absent when it is installed.
	reportersInstalled := grants.Has(CapActions) || grants.Has(CapJobs)
	var withheld []string
	for _, cap := range AllCapabilities {
		if grants.Has(cap) {
			continue
		}
		surface := CapabilitySurfaces[cap]
		if (cap == CapJobs || cap == CapActions) && reportersInstalled {
			surface = CapabilitySurfacesWithoutReporters[cap]
		}
		withheld = append(withheld, fmt.Sprintf("%s (capability %q)", surface, cap))
	}
	if len(withheld) > 0 {
		log.Printf("[plugin] %s: not installed: %s", name, strings.Join(withheld, "; "))
	}
}

// registerMahModule sets up the mah.on, mah.inject, mah.log, mah.page, mah.menu,
// and mah.abort functions in the given Lua state. pluginNamePtr is populated by
// loadPlugin after reading the plugin table, before init() is called.
//
// grants decides what is installed. An ungranted key is never set, so there is
// nothing to guard at the call sites and nothing for a plugin to reach. Keep
// every capability decision in this function (and registerDbModule's read/write
// split) — internal/arch/plugin_capability_gate_test.go fails the build when a
// registration appears without one.
func (pm *PluginManager) registerMahModule(L *lua.LState, pluginNamePtr *string, grants CapabilitySet, egress NetworkPolicy) {
	mahMod := L.NewTable()

	// setIf installs a root-level mah function only when its capability is
	// granted. Capability "" means always installed: json, util, log,
	// html_escape, sleep, abort, doc and get_setting read or write nothing
	// outside the plugin itself.
	setIf := func(capability, name string, fn lua.LGFunction) {
		if capability != "" && !grants.Has(capability) {
			return
		}
		mahMod.RawSetString(name, L.NewFunction(fn))
	}

	// setIfAny installs a function that more than one capability implies. The
	// job reporters are the case: an async action is handed a job_id and is
	// expected to report on it, so "actions" has to reach them, while creating
	// work of its own stays behind "jobs".
	setIfAny := func(capabilities []string, name string, fn lua.LGFunction) {
		for _, c := range capabilities {
			if grants.Has(c) {
				mahMod.RawSetString(name, L.NewFunction(fn))
				return
			}
		}
	}

	setIf(CapHooks, "on", func(L *lua.LState) int {
		eventName := L.CheckString(1)
		handler := L.CheckFunction(2)

		// Refuse a name nothing dispatches, before anything is stored and
		// before the liveness gate below. That is where mah.page checks its
		// path, and raising here is all or nothing: a failing init() goes
		// through loadPlugin's abandon(), which revokes the VM and sweeps every
		// registration made before the error. So there is no half-loaded plugin
		// to weigh against saying so loudly.
		//
		// The message carries the catalogue itself rather than a description of
		// it. The author's next question is "then what is it called?", and a
		// list built from the catalogue cannot come to describe something else.
		if !IsHookEvent(eventName) {
			L.ArgError(1, fmt.Sprintf("unknown event %q: nothing dispatches it, so this hook could never fire. Events: %s",
				eventName, strings.Join(AllHookEvents, ", ")))
			return 0
		}

		// mainState, not L: a registration made from inside a coroutine would
		// otherwise be stamped with the coroutine's state, which no dispatch
		// and no teardown ever matches — so it could never fire and could never
		// be removed.
		owner := mainState(L)

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		pm.hooks[eventName] = append(pm.hooks[eventName], hookEntry{
			state:      owner,
			fn:         handler,
			pluginName: *pluginNamePtr,
		})
		pm.mu.Unlock()
		return 0
	})

	setIf(CapInject, "inject", func(L *lua.LState) int {
		slotName := L.CheckString(1)
		renderFn := L.CheckFunction(2)

		// Slot names live only as string literals in the templates, so a
		// misspelled one is a renderer nothing ever calls. Refused like an
		// event, and for the same reasons.
		if !IsInjectionSlot(slotName) {
			L.ArgError(1, fmt.Sprintf("unknown slot %q: no template renders it, so this injection could never run. Slots: %s",
				slotName, strings.Join(AllInjectionSlots, ", ")))
			return 0
		}

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		pm.injections[slotName] = append(pm.injections[slotName], injectionEntry{
			state:  mainState(L),
			fn:     renderFn,
			plugin: *pluginNamePtr,
		})
		pm.mu.Unlock()
		return 0
	})

	setIf("", "log", func(L *lua.LState) int {
		level := L.CheckString(1)
		message := L.CheckString(2)

		var details map[string]any
		if detailsTbl := L.OptTable(3, nil); detailsTbl != nil {
			details = luaTableToGoMap(detailsTbl)
		}

		if pl := pm.loggerFor(L); pl != nil {
			pl.PluginLog(*pluginNamePtr, level, message, details)
		} else {
			log.Printf("[plugin][%s] %s", level, message)
		}
		return 0
	})

	setIf("", "abort", func(L *lua.LState) int {
		reason := L.CheckString(1)
		L.RaiseError("PLUGIN_ABORT: %s", reason)
		return 0
	})

	setIf("", "get_setting", func(L *lua.LState) int {
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
	})

	setIf(CapPages, "page", func(L *lua.LState) int {
		path := L.CheckString(1)
		handler := L.CheckFunction(2)

		if !validPagePath.MatchString(path) {
			L.ArgError(1, "invalid page path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
			return 0
		}

		name := *pluginNamePtr
		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		if pm.pages[name] == nil {
			pm.pages[name] = make(map[string]pageEntry)
		}
		pm.pages[name][path] = pageEntry{state: mainState(L), fn: handler}
		pm.mu.Unlock()
		return 0
	})

	setIf(CapPages, "menu", func(L *lua.LState) int {
		label := L.CheckString(1)
		path := L.CheckString(2)

		if !validPagePath.MatchString(path) {
			L.ArgError(2, "invalid menu path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
			return 0
		}

		name := *pluginNamePtr
		fullPath := "/plugins/" + name + "/" + path

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		pm.menuItems = append(pm.menuItems, MenuRegistration{
			PluginName: name,
			Label:      label,
			FullPath:   fullPath,
			state:      mainState(L),
		})
		pm.mu.Unlock()
		return 0
	})

	setIf(CapActions, "action", func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		action, err := parseActionTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		for _, existing := range pm.actions[*pluginNamePtr] {
			if existing.ID == action.ID {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate action id %q", action.ID))
				return 0
			}
		}
		action.state = mainState(L)
		pm.actions[*pluginNamePtr] = append(pm.actions[*pluginNamePtr], *action)
		pm.mu.Unlock()
		return 0
	})

	setIf(CapRender, "block_type", func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		pbt, err := parseBlockTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		pbt.State = mainState(L)

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		for _, existing := range pm.blockTypes[*pluginNamePtr] {
			if existing.TypeName == pbt.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate block type %q", pbt.TypeName))
				return 0
			}
		}
		pm.blockTypes[*pluginNamePtr] = append(pm.blockTypes[*pluginNamePtr], pbt)
		// Under pm.mu, with the local record: a teardown landing between the
		// two would remove the record and then watch this add the global entry
		// it can no longer find to remove.
		block_types.RegisterBlockType(pbt)
		pm.mu.Unlock()
		return 0
	})

	setIf(CapRender, "display_type", func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		dt, err := parseDisplayTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		dt.State = mainState(L)

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
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
	})

	setIf(CapRender, "shortcode", func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		sc, err := parseShortcodeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		sc.State = mainState(L)

		pm.mu.Lock()
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
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
	})

	setIf("", "doc", func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		doc := &PluginDoc{PluginName: *pluginNamePtr, State: mainState(L)}

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
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
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
	})

	setIf(CapAPI, "api", func(L *lua.LState) int {
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
		if !pm.stateMayRegisterLocked(L) {
			pm.mu.Unlock()
			return 0
		}
		if pm.apiEndpoints[name] == nil {
			pm.apiEndpoints[name] = make(map[string]*APIEndpoint)
		}
		pm.apiEndpoints[name][key] = &APIEndpoint{
			state:   mainState(L),
			fn:      handler,
			timeout: timeout,
		}
		pm.mu.Unlock()
		return 0
	})

	setIfAny([]string{CapActions, CapJobs}, "job_progress", func(L *lua.LState) int {
		jobID := L.CheckString(1)
		percent := L.CheckInt(2)
		message := L.CheckString(3)

		if percent < 0 {
			percent = 0
		} else if percent > 100 {
			percent = 100
		}

		job, ok := pm.jobOwnedBy(jobID, *pluginNamePtr)
		if !ok {
			// Same message whether the job does not exist or belongs to
			// somebody else, so ids stay unguessable.
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
	})

	setIfAny([]string{CapActions, CapJobs}, "job_complete", func(L *lua.LState) int {
		jobID := L.CheckString(1)
		resultTbl := L.OptTable(2, nil)

		job, ok := pm.jobOwnedBy(jobID, *pluginNamePtr)
		if !ok {
			// Same message whether the job does not exist or belongs to
			// somebody else, so ids stay unguessable.
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
	})

	setIfAny([]string{CapActions, CapJobs}, "job_fail", func(L *lua.LState) int {
		jobID := L.CheckString(1)
		errMsg := L.CheckString(2)

		job, ok := pm.jobOwnedBy(jobID, *pluginNamePtr)
		if !ok {
			// Same message whether the job does not exist or belongs to
			// somebody else, so ids stay unguessable.
			L.ArgError(1, "unknown job_id")
			return 0
		}

		job.mu.Lock()
		job.Status = "failed"
		job.Message = errMsg
		job.mu.Unlock()

		pm.notifyActionJobSubscribers("updated", job)
		return 0
	})

	// mah.start_job(label, fn) — create an async job and run fn(job_id) in a background goroutine.
	// Returns the job ID immediately. The callback receives the job_id as its argument and can use
	// mah.job_progress, mah.job_complete, mah.job_fail to report status.
	setIf(CapJobs, "start_job", func(L *lua.LState) int {
		label := L.CheckString(1)
		fn := L.CheckFunction(2)

		// A revoked plugin may not start new work. Without this the job is
		// created, immediately failed by the worker (its VM is gone), and
		// re-creates the in-flight WaitGroup that teardown had just deleted —
		// leaving an entry nothing ever removes.
		if !pm.stateIsLive(L) {
			L.RaiseError("plugin has been disabled")
			return 0
		}

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
	})

	setIf("", "html_escape", func(L *lua.LState) int {
		s := L.CheckString(1)
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&#39;")
		// Square brackets too, because a plugin's output is not plain HTML: it
		// goes back through the shortcode processor, so `[` is a metacharacter
		// of the output context exactly as `<` is. Without them a plugin that
		// prints a meta value or an entity field hands whoever wrote that text
		// a shortcode on the page that printed it, expanded under the reader's
		// scope rather than the writer's. Quoting the value is no defence: an
		// attribute span is unescaped only after the pattern has matched it, so
		// the escaping above is undone again inside a bracket.
		//
		// Entities rather than removal, so escaped text still reads as it was
		// written: the browser renders these as the characters themselves, in
		// text and in attribute values alike, while shortcodePattern needs a
		// literal `[` and no longer matches. Appended last, so the `&` each one
		// introduces is not escaped again.
		s = strings.ReplaceAll(s, "[", "&#91;")
		s = strings.ReplaceAll(s, "]", "&#93;")
		L.Push(lua.LString(s))
		return 1
	})

	// mah.sleep(seconds) - blocks the current Lua VM for the given duration.
	// Bounded to [0, 30] seconds to prevent abuse. Useful for polling external
	// async APIs (e.g. fal.ai queue) from within a sync action handler.
	setIf("", "sleep", func(L *lua.LState) int {
		// Raised rather than returned: sleep has no return value to carry a
		// refusal, and returning 0 silently would make a plugin that polls an
		// external API inside a transaction look like it worked while it held
		// the write lock for up to 30 seconds.
		if pm.inTransaction(L) {
			L.RaiseError("%s", refusedInTransaction("mah.sleep", whyItWaits))
			return 0
		}
		secs := L.CheckNumber(1)
		if secs < 0 {
			secs = 0
		}
		if secs > 30 {
			secs = 30
		}
		time.Sleep(time.Duration(float64(secs) * float64(time.Second)))
		return 0
	})

	// Sub-modules. registerDbModule splits internally, because db:read and
	// db:write share one table; the rest are whole-module grants.
	pm.registerDbModule(L, mahMod, grants, egress)
	if grants.Has(CapHTTP) {
		pm.registerHttpModule(L, mahMod, egress)
	}
	if grants.Has(CapKV) {
		pm.registerKvModule(L, mahMod, pluginNamePtr)
	}
	if grants.Has(CapImage) {
		pm.registerImageModule(L, mahMod)
	}
	// Always installed: neither reads nor writes anything outside the plugin.
	pm.registerJsonModule(L, mahMod)
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
	if pm.closed.Load() {
		return fmt.Errorf("plugin manager is shutting down")
	}

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

	// Checked before the load, not inside it: a dependency is about the plugin
	// set, not about this plugin's own code, and refusing here means no VM is
	// ever built for a plugin that cannot run.
	if err := pm.checkDependencies(*dp); err != nil {
		return err
	}

	if err := pm.loadPlugin(*dp); err != nil {
		return fmt.Errorf("loading plugin %q: %w", name, err)
	}

	return nil
}

// DisablePlugin deactivates a running plugin: removes all hooks, injections,
// pages, menu items, and closes the Lua VM.
func (pm *PluginManager) DisablePlugin(name string) error {
	// Refuse, never cascade. Disabling one plugin must not silently disable
	// another: the operator asked about this plugin, and a dependent going dark
	// as a side effect is a change they did not make and will not look for.
	//
	// Only the public entry point checks. Close tears every plugin down through
	// finishTeardown directly, and a shutdown that refused to stop a plugin
	// because something depended on it would never finish.
	if dependents := pm.dependentsOf(name); len(dependents) > 0 {
		return fmt.Errorf("%w: %s still depends on %q; disable %s first",
			ErrDependencyInUse, strings.Join(dependents, ", "), name, strings.Join(dependents, " and "))
	}
	return pm.disablePlugin(name, false)
}

// disablePlugin is DisablePlugin, with waited recording whether it has already
// waited out an in-flight load of this name.
func (pm *PluginManager) disablePlugin(name string, waited bool) error {
	pm.mu.Lock()

	// Close nils plugins and states together; a disable that arrives after it
	// used to find a name in one and index the other.
	if pm.closed.Load() {
		pm.mu.Unlock()
		return fmt.Errorf("plugin manager is shutting down")
	}

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
		// Not enabled *yet* is a different answer from not enabled. A load in
		// flight will publish in a moment, and disabling "nothing" would report
		// success while the plugin came up behind it.
		if done, loading := pm.loading[name]; loading && !waited {
			pm.mu.Unlock()
			select {
			case <-done:
				// Published or failed; either way the ordinary path can now see
				// the truth. One retry only — a second wait would mean another
				// enable started, which is a new decision, not this one.
				return pm.disablePlugin(name, true)
			case <-time.After(retireDrainTimeout):
				return fmt.Errorf("plugin %q is still loading after %s and cannot be disabled yet; "+
					"its init() has not returned", name, retireDrainTimeout)
			}
		}
		pm.mu.Unlock()
		return fmt.Errorf("plugin %q is not enabled", name)
	}

	// Revoke in the same critical section as the unregister. Doing it later —
	// inside the teardown, after pm.mu has been released — leaves a window in
	// which the plugin is unregistered but still live, so an invocation already
	// running can register again, and a concurrent re-enable's registrations
	// can be overwritten by the generation being disposed of.
	mu, owned := pm.revokeLocked(targetState)
	pm.unregisterPluginLocked(name, targetState)

	// Remove from active lists.
	pm.plugins = append(pm.plugins[:pluginIdx], pm.plugins[pluginIdx+1:]...)
	pm.states = append(pm.states[:pluginIdx], pm.states[pluginIdx+1:]...)

	// Remove in-memory settings. Only a deliberate disable does this: a failed
	// load must leave the operator's saved settings alone.
	delete(pm.pluginSettings, name)

	// Release pm.mu so in-flight goroutines can finish (they need VMLock).
	pm.mu.Unlock()

	if owned {
		pm.finishTeardown(name, targetState, mu)
	}

	return nil
}

// unregisterPluginLocked removes every registration a plugin made. pm.mu must
// be held.
//
// Hooks and injections are keyed by event and slot, so they are matched by
// state; everything else is filed under the plugin's name. Both are needed:
// the same plugin owns registrations of both shapes.
func (pm *PluginManager) unregisterPluginLocked(name string, state *lua.LState) {
	// Every removal matches the *state*, never the name alone.
	//
	// A name can be held by more than one VM over time: a teardown can still be
	// finishing while the operator re-enables the plugin, so a name-keyed
	// delete would take the live generation's pages and actions with it — and
	// a sweep that skipped name-keyed removal whenever a replacement existed
	// would leave the dead generation's registrations in place instead, which
	// is the same bug facing the other way. Matching on the state is the only
	// rule that is right in both orderings.
	for event, entries := range pm.hooks {
		var filtered []hookEntry
		for _, e := range entries {
			if e.state != state {
				filtered = append(filtered, e)
			}
		}
		pm.hooks[event] = filtered
	}

	for slot, entries := range pm.injections {
		var filtered []injectionEntry
		for _, e := range entries {
			if e.state != state {
				filtered = append(filtered, e)
			}
		}
		pm.injections[slot] = filtered
	}

	for path, entry := range pm.pages[name] {
		if entry.state == state {
			delete(pm.pages[name], path)
		}
	}
	if len(pm.pages[name]) == 0 {
		delete(pm.pages, name)
	}

	var filteredMenus []MenuRegistration
	for _, m := range pm.menuItems {
		if m.state != state {
			filteredMenus = append(filteredMenus, m)
		}
	}
	pm.menuItems = filteredMenus

	var filteredActions []ActionRegistration
	for _, a := range pm.actions[name] {
		if a.state != state {
			filteredActions = append(filteredActions, a)
		}
	}
	if len(filteredActions) == 0 {
		delete(pm.actions, name)
	} else {
		pm.actions[name] = filteredActions
	}

	// Block types also live in a process-global registry.
	var filteredBlocks []*PluginBlockType
	for _, pbt := range pm.blockTypes[name] {
		if pbt.State == state {
			block_types.UnregisterBlockType(pbt.TypeName)
		} else {
			filteredBlocks = append(filteredBlocks, pbt)
		}
	}
	if len(filteredBlocks) == 0 {
		delete(pm.blockTypes, name)
	} else {
		pm.blockTypes[name] = filteredBlocks
	}

	var filteredDisplays []*PluginDisplayType
	for _, dt := range pm.displayTypes[name] {
		if dt.State != state {
			filteredDisplays = append(filteredDisplays, dt)
		}
	}
	if len(filteredDisplays) == 0 {
		delete(pm.displayTypes, name)
	} else {
		pm.displayTypes[name] = filteredDisplays
	}

	var filteredShortcodes []*PluginShortcode
	for _, sc := range pm.shortcodes[name] {
		if sc.State != state {
			filteredShortcodes = append(filteredShortcodes, sc)
		}
	}
	if len(filteredShortcodes) == 0 {
		delete(pm.shortcodes, name)
	} else {
		pm.shortcodes[name] = filteredShortcodes
	}

	var filteredDocs []*PluginDoc
	for _, d := range pm.docs[name] {
		if d.State != state {
			filteredDocs = append(filteredDocs, d)
		}
	}
	if len(filteredDocs) == 0 {
		delete(pm.docs, name)
	} else {
		pm.docs[name] = filteredDocs
	}

	for key, ep := range pm.apiEndpoints[name] {
		if ep.state == state {
			delete(pm.apiEndpoints[name], key)
		}
	}
	if len(pm.apiEndpoints[name]) == 0 {
		delete(pm.apiEndpoints, name)
	}
}

// stateIsLive reports whether a VM has not been revoked. Takes pm.mu itself, so
// callers must not hold it.
func (pm *PluginManager) stateIsLive(L *lua.LState) bool {
	if pm.closed.Load() {
		return false
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, live := pm.vmLocks[mainState(L)]
	return live
}

// stateMayRegisterLocked reports whether a VM is still entitled to register
// things. pm.mu must be held.
//
// A revoked plugin's worker keeps running until it finishes, and it holds a
// fully-installed mah table the whole time — so without this it can re-register
// after its teardown swept, and even overwrite a *replacement* generation's
// page by writing the same path. The vmLocks entry is the same liveness token
// dispatch uses, so registration and dispatch agree on when a VM is gone.
func (pm *PluginManager) stateMayRegisterLocked(L *lua.LState) bool {
	if pm.closed.Load() {
		return false
	}
	_, live := pm.vmLocks[mainState(L)]
	return live
}

func (pm *PluginManager) revokeLocked(state *lua.LState) (*vmMutex, bool) {
	mu, owned := pm.vmLocks[state]
	delete(pm.vmLocks, state)
	return mu, owned
}

// finishTeardown waits for a revoked plugin's in-flight async work, closes its
// VM under mu, and sweeps whatever that work registered on the way out.
//
// The caller must already have revoked the state.
func (pm *PluginManager) finishTeardown(name string, state *lua.LState, mu *vmMutex) {
	pm.actionJobsMu.Lock()
	wg := pm.actionInFlight[name]
	delete(pm.actionInFlight, name)
	pm.actionJobsMu.Unlock()

	// A worker that was already inside when the state was revoked still holds a
	// fully-installed mah table, so it can register right up until it stops.
	// stateMayRegisterLocked refuses those now, but the sweep is what removes
	// anything that landed before the revocation.
	sweep := func() {
		pm.mu.Lock()
		pm.unregisterPluginLocked(name, state)
		pm.mu.Unlock()
	}

	closeUnderLock := func() {
		if mu != nil {
			mu.Lock()
			state.Close()
			mu.Unlock()
			return
		}
		state.Close()
	}

	if wg != nil && !waitWithin(wg, retireDrainTimeout) {
		// The caller is unblocked — an enable request must not wait out a job's
		// whole allowance — but the plugin is already revoked, so the only
		// thing still outstanding is closing the VM.
		log.Printf("[plugin] warning: %s still has async work running after %s; "+
			"closing its VM once that work stops", name, retireDrainTimeout)
		go func() {
			wg.Wait()
			closeUnderLock()
			sweep()
		}()
		return
	}

	closeUnderLock()
	sweep()
}

// waitWithin waits for wg, reporting false if it did not finish in time.
func waitWithin(wg *sync.WaitGroup, limit time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

// closeState closes a plugin's LState exactly once, and only if this caller
// owns the teardown.
//
// The vmLocks entry is the ownership token. LockVM acquires the lock and then
// re-checks that the entry is still there, so of two teardowns racing, the
// first deletes the entry and closes, and the second is told the state is
// already gone. A caller that finds no entry must NOT close: an absent entry
// does not mean nobody is inside — it means somebody else has already taken
// ownership and may still be executing on it. Closing a live LState is a data
// race and then a nil dereference inside gopher-lua.
//
// Taking the lock also waits for whatever is in flight: hooks, injections,
// shortcodes, block and display renders, pages and API endpoints all call into
// the state while holding it.
//
// pm.mu must NOT be held: LockVM takes pm.mu.RLock while holding the VM lock,
// so the reverse order would deadlock.
func closeState(pm *PluginManager, state *lua.LState) {
	// The claim every teardown path uses: remove the entry under pm.mu — which
	// is both the revocation and the ownership claim — and close only if this
	// caller is the one that removed it.
	//
	// One protocol, shared with DisablePlugin and with a failed load: revoke
	// under pm.mu — which is both the revocation and the claim — and close only
	// if this caller is the one that removed the entry. Earlier versions of
	// these paths claimed in opposite orders (lock-then-delete here,
	// delete-then-lock there), which interleaves into a double close, and each
	// grew its own copy until the live disable path had drifted away from the
	// rule entirely.
	pm.mu.Lock()
	mu, owned := pm.revokeLocked(state)
	pm.mu.Unlock()
	if !owned {
		return
	}

	if mu != nil {
		mu.Lock()
		state.Close()
		mu.Unlock()
		return
	}
	state.Close()
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
	pm.mu.RLock()
	defer pm.mu.RUnlock()
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
func (pm *PluginManager) VMLock(L *lua.LState) *vmMutex {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.vmLocks[L]
}

// stillRegistered reports whether L is still in vmLocks, which is the liveness
// question every acquisition re-asks once it holds the lock.
func (pm *PluginManager) stillRegistered(L *lua.LState) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, live := pm.vmLocks[L]
	return live
}

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

// LockVM acquires the VM lock for L and returns it, or returns nil when the
// plugin is gone. A nil return means the caller must not touch L at all: no lock
// is held in that case.
//
// Lock ordering is mu then pm.mu, matching DisablePlugin. Nothing may take a VM
// lock while already holding pm.mu.
//
// (This doc comment spent some time attached to vmRequestKey, several
// declarations below, where it documented nothing.)
func (pm *PluginManager) LockVM(L *lua.LState) *vmMutex {
	// Background never ends, so this is the same unbounded wait it always was.
	mu, _ := pm.LockVMWithContext(context.Background(), L)
	return mu
}

// LockVMWithContext is LockVM for a caller that can be abandoned: it stops
// waiting once ctx ends.
//
// Three outcomes, and the third is the one this exists for. (mu, nil) is
// acquired. (nil, nil) is "the plugin is gone", which every caller already
// handles and must, because touching L after that is a data race and then a nil
// dereference inside gopher-lua. (nil, err) is "your own request ended while you
// were queued behind somebody else's call into this plugin", which is not the
// plugin's fault and must not be reported as though it were missing.
//
// Nothing here gives any caller a deadline it did not have. A caller whose
// context never ends waits exactly as long as before; what changes is only that
// a client who has already gone away stops holding a goroutine in the queue
// behind a call that mah.http allows 120 seconds and a remote fetch 30 minutes.
func (pm *PluginManager) LockVMWithContext(ctx context.Context, L *lua.LState) (*vmMutex, error) {
	mu := pm.VMLock(L)
	if mu == nil {
		return nil, nil
	}
	if !mu.LockWithin(ctx, 0) {
		return nil, errVMWaitAbandoned
	}
	// The nil check on VMLock's result above is not sufficient on its own,
	// because a caller can capture a live mutex and then block on it while
	// DisablePlugin closes the state. LState.Close() writes state the in-flight
	// L.CallByParam() is reading, so that ordering is a data race and then a nil
	// dereference inside gopher-lua. The window closes here, by re-checking
	// liveness *after* the lock is held: DisablePlugin removes the entry while
	// holding this very mutex, so a caller that wins the race sees the entry and
	// one that loses it backs out.
	if !pm.stillRegistered(L) {
		mu.Unlock()
		return nil, nil
	}
	return mu, nil
}

// TryLockVMWithin is LockVM bounded by two things at once: wait, and ctx. It
// returns (nil, false) when the plugin is gone — the caller must not touch L —
// and (nil, true) when the plugin is alive but its lock was not taken before
// either bound ended.
//
// ctx is the caller's own, and a caller that has none passes Background, which
// leaves wait as the only bound and the behaviour exactly what it was. It is a
// second bound rather than a replacement for wait, because the two answer
// different questions: wait breaks a deadlock between two goroutines, ctx stops
// a wait nobody is left to receive the answer of.
//
// The deadlock is why this exists at all, and the invocation chain cannot see
// it, because a chain is per-call-stack. Two plugins that each hook an entity
// the other writes can arrive at each other's mutex from opposite directions:
// goroutine A holds P and waits for Q while B holds Q and waits for P. Both
// waits are unbounded and the Lua deadline cannot preempt a block inside a Go
// call, so that is permanent — and it is permanent on the code this replaces
// too.
//
// Only the nested case needs bounding, and only the nested case gets it (see
// RunAfterHooks): a dispatch that holds no VM lock cannot be a participant in
// such a cycle, so it does not come here at all and waits for as long as its
// own caller does.
func (pm *PluginManager) TryLockVMWithin(ctx context.Context, L *lua.LState, wait time.Duration) (*vmMutex, bool) {
	mu := pm.VMLock(L)
	if mu == nil {
		return nil, false
	}
	// A non-positive wait is handled here rather than passed down, because the
	// two functions read a zero the opposite way round. LockWithin takes it as
	// "no deadline of my own", which is what LockVM wants; this is the function
	// whose entire job is to *bound* the wait, so an unbounded zero would
	// reinstate the cross-goroutine cycle it exists to break — and it would do
	// it for the argument a caller would most naturally pass to mean "do not
	// block". The loop this replaced gave a zero wait one attempt and gave up.
	acquired := false
	if wait <= 0 {
		// ctx plays no part here, and cannot: this branch does not wait, so
		// there is nothing for a cancellation to shorten.
		acquired = mu.TryLock()
	} else {
		acquired = mu.LockWithin(ctx, wait)
	}
	if !acquired {
		// Distinguish "busy" from "gone" here too, not just on the acquiring
		// path. DisablePlugin revokes the vmLocks entry only once it holds the
		// VM lock, so a dispatcher working from a hook snapshot taken before
		// that can sit here waiting on a plugin that is being torn down.
		// Reporting it as contention would fail a caller's write over a hook
		// that no longer exists; a disabled plugin is always a safe skip.
		return nil, pm.stillRegistered(L)
	}
	if !pm.stillRegistered(L) {
		mu.Unlock()
		return nil, false
	}
	return mu, false
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
	// Under pm.mu so it is exclusive with a load registering itself: a load
	// that got in first is in loadWg and waited for below; one that arrives
	// after sees closed and stops before creating anything.
	pm.mu.Lock()
	pm.closed.Store(true)
	pm.mu.Unlock()
	// Bounded: init() is deliberately unbounded (see loadPlugin), so a plugin
	// wedged there must not turn shutdown into a hang too. A load that finishes
	// after this re-checks closed under pm.mu and abandons itself.
	if !waitWithin(&pm.loadWg, retireDrainTimeout) {
		log.Printf("[plugin] warning: a plugin load is still running after %s; shutting down without it",
			retireDrainTimeout)
	}

	// closed was set under pm.mu above, and beginHTTP adds under pm.mu.RLock —
	// so every Add that will ever happen has happened before this Wait.
	pm.httpWg.Wait()

	// Every policy client holds its own connection pool, and they are the only
	// clients plugin egress uses now.
	pm.closeEgressClients()
	// Once: Close is a deferred shutdown step and a t.Cleanup in a hundred
	// tests, so it gets called twice — and closing a closed channel panics.
	pm.closeDone.Do(func() { close(pm.done) })

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
		closeState(pm, L)
	}

	// Undelivered callbacks are dropped here rather than left in the maps.
	//
	// The drain goroutine selects between done and httpNotify, so with both
	// ready it can take done and exit leaving callbacks queued. Nothing collects
	// them afterwards — their VM has no worker and the goroutine that would have
	// started one is gone — and each entry pins an *lua.LState and an
	// *lua.LFunction, so a manager still reachable after Close keeps a whole Lua
	// state alive.
	//
	// After the teardown loop, not before it, because httpWg is not the whole
	// story: the mah.http bindings queue an error callback synchronously on
	// whatever goroutine called them, which is a render or a hook and is counted
	// by nothing. Three of those sites fire exactly when closed is set, so a
	// render running during shutdown actively produces entries. Only once every
	// state has been locked, revoked and closed is there no Lua left running and
	// no way to start any. Clearing a draining mark whose worker will delete it
	// again is harmless, and no worker can be executing a callback here: it
	// would have had to hold a VM lock that closeState has already taken.
	pm.httpMu.Lock()
	pm.httpPending = make(map[*lua.LState][]httpCallback)
	pm.httpDraining = make(map[*lua.LState]bool)
	pm.httpMu.Unlock()

	// Emptied, not niled. init() is unbounded and the wait above is not, so a
	// load can still be running here — and every registration function writes
	// its map while holding pm.mu. Assignment to a nil map panics; the panic is
	// caught by the protected Lua call around it, but pm.mu is left locked
	// forever and the next teardown blocks on it for the life of the process.
	// An empty map takes the write harmlessly: nothing dispatches once closed.
	pm.mu.Lock()
	defer pm.mu.Unlock()
	// The process-global block-type registry outlives this manager, so entries
	// pointing at VMs that are about to close have to go with them.
	for _, types := range pm.blockTypes {
		for _, pbt := range types {
			block_types.UnregisterBlockType(pbt.TypeName)
		}
	}

	pm.plugins = nil
	pm.states = nil
	pm.hooks = make(map[string][]hookEntry)
	pm.injections = make(map[string][]injectionEntry)
	pm.pages = make(map[string]map[string]pageEntry)
	pm.menuItems = nil
	pm.actions = make(map[string][]ActionRegistration)
	pm.blockTypes = make(map[string][]*PluginBlockType)
	pm.displayTypes = make(map[string][]*PluginDisplayType)
	pm.shortcodes = make(map[string][]*PluginShortcode)
	pm.docs = make(map[string][]*PluginDoc)
	// Settings hold operator secrets (a password-typed setting is an API key),
	// so they go with everything else rather than outliving the manager.
	pm.pluginSettings = make(map[string]map[string]any)
	pm.apiEndpoints = make(map[string]map[string]*APIEndpoint)
	// vmLocks is deliberately left alone. closeState removes each entry as it
	// closes that state, and a load still running here needs its entry to be
	// able to close its own VM afterwards. Registration is already refused once
	// closed is set, so a surviving entry grants nothing.
}
