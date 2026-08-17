package application_context

import (
	"fmt"
	"sync/atomic"
	"time"

	"mahresources/models"
)

// scopedAccessTTL bounds how long a snapshot is trusted.
//
// Invalidation is process-local, and a deployment may run several processes
// behind a load balancer. Without a bound, a revocation applied on one process
// would leave every other one serving the old answer until it restarted — for a
// permission, indefinitely is the wrong length of time. Thirty seconds is short
// enough that a revocation lands on its own, and long enough that the render
// path of a busy page is not a query.
const scopedAccessTTL = 30 * time.Second

// scopedPluginAccess is the set of plugins a group-limited principal may reach,
// held as an immutable snapshot swapped on every change.
//
// It is a cache because of where it is read: every shortcode render, every
// injected slot, every plugin page and endpoint asks once per plugin per
// request, and a base layout carries six slots. A query there would put a round
// trip on the render path of every page in the application, for a value that
// changes when an operator clicks a toggle.
//
// It is held by POINTER on the context, never by value. MahresourcesContext is
// shallow-copied on every WithPrincipal, WithTransaction and WithRequest, and an
// atomic.Pointer carries a noCopy — so a by-value field would both trip vet and
// give every clone its own cache to fill.
type scopedPluginAccess struct {
	snapshot atomic.Pointer[scopedAccessSnapshot]
	// generation is bumped by every write and every invalidation. A loader
	// stamps the generation it started from and its result is discarded if the
	// world moved underneath it — without that, a slow read taken before a
	// revocation can be stored after it, and the permission comes back.
	generation atomic.Uint64
}

type scopedAccessSnapshot struct {
	allowed    map[string]bool
	generation uint64
	loadedAt   time.Time
}

// PluginAllowsScopedPrincipals reports whether a group-limited user or guest may
// reach pluginName's own surfaces.
//
// Fail-closed on a read error: the caller is deciding whether to run plugin code
// for a confined principal, and "I could not find out" must not resolve to yes.
func (ctx *MahresourcesContext) PluginAllowsScopedPrincipals(pluginName string) bool {
	if pluginName == "" {
		return false
	}
	cache := ctx.scopedAccess
	if cache == nil {
		// A context built without the cache (a bare struct literal in a test)
		// still has to answer correctly, so it asks the database. Wiring builds
		// the cache, so this is not the production path.
		allowed, err := ctx.readScopedPluginAccess()
		if err != nil {
			return false
		}
		return allowed[pluginName]
	}

	generation := cache.generation.Load()
	snapshot := cache.snapshot.Load()
	if snapshot == nil || snapshot.generation != generation || time.Since(snapshot.loadedAt) > scopedAccessTTL {
		loaded, err := ctx.loadScopedPluginAccess()
		if err != nil {
			return false
		}
		snapshot = loaded
	}
	return snapshot.allowed[pluginName]
}

// SetPluginScopedAccess records whether group-limited principals may reach a
// plugin.
//
// It invalidates rather than refreshing. A refresh here would have to read the
// database again, and a refresh that fails would leave the pre-write snapshot in
// place — so a revocation that committed would keep serving "allowed" while
// reporting an error to the operator. Dropping the snapshot cannot fail, and a
// reader that cannot reload answers no.
func (ctx *MahresourcesContext) SetPluginScopedAccess(pluginName string, allowed bool) error {
	if pluginName == "" {
		return fmt.Errorf("plugin name is required")
	}
	if ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager not initialized")
	}
	if ctx.findDiscoveredPlugin(pluginName) == nil {
		return fmt.Errorf("plugin %q not found", pluginName)
	}

	state := models.PluginState{PluginName: pluginName}
	if err := ctx.db.Where("plugin_name = ?", pluginName).FirstOrCreate(&state).Error; err != nil {
		return err
	}
	if err := ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("allow_scoped_principals", allowed).Error; err != nil {
		return err
	}

	ctx.InvalidateScopedPluginAccess()
	return nil
}

// InvalidateScopedPluginAccess drops the snapshot so the next read reloads it.
// Called wherever plugin rows change underneath this cache.
func (ctx *MahresourcesContext) InvalidateScopedPluginAccess() {
	if ctx.scopedAccess == nil {
		return
	}
	ctx.scopedAccess.generation.Add(1)
	ctx.scopedAccess.snapshot.Store(nil)
}

func (ctx *MahresourcesContext) readScopedPluginAccess() (map[string]bool, error) {
	states, err := ctx.GetPluginStates()
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(states))
	for _, state := range states {
		// Only an enabled plugin can be reached at all, so a stale allow on a
		// disabled one must not read as access. Disabling is not consent
		// withdrawal, but it is not an invitation either.
		if state.Enabled && state.AllowScopedPrincipals {
			allowed[state.PluginName] = true
		}
	}
	return allowed, nil
}

func (ctx *MahresourcesContext) loadScopedPluginAccess() (*scopedAccessSnapshot, error) {
	cache := ctx.scopedAccess
	var generation uint64
	if cache != nil {
		generation = cache.generation.Load()
	}

	allowed, err := ctx.readScopedPluginAccess()
	if err != nil {
		return nil, err
	}
	snapshot := &scopedAccessSnapshot{allowed: allowed, generation: generation, loadedAt: time.Now()}

	// Publish only if nothing changed while the read was in flight. A losing
	// loader still returns its own result to its own caller, which is at worst
	// as stale as the moment it started — it simply does not become everyone
	// else's answer.
	if cache != nil && cache.generation.Load() == generation {
		cache.snapshot.Store(snapshot)
	}
	return snapshot, nil
}
