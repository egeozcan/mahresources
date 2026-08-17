package application_context

import (
	"fmt"
	"sync/atomic"

	"mahresources/models"
)

// scopedPluginAccess is the set of plugins a group-limited principal may reach,
// held as an immutable snapshot swapped on every change.
//
// It is a cache because of where it is read: every shortcode render, every
// injected slot, every plugin page and endpoint asks once per plugin per
// request, and a base layout carries six slots. A query there would put a round
// trip on the render path of every page in the application, for a value that
// changes when an operator clicks a toggle.
//
// A nil snapshot means "not loaded yet" and is materialised on first read, so a
// context that never touches plugins never pays for the load.
//
// It is held by POINTER on the context, never by value. MahresourcesContext is
// shallow-copied on every WithPrincipal, WithTransaction and WithRequest, and an
// atomic.Pointer carries a noCopy — so a by-value field would both trip vet and
// give every clone its own cache to fill.
type scopedPluginAccess struct {
	allowed atomic.Pointer[map[string]bool]
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
	if ctx.scopedAccess == nil {
		// A context built without the cache (a bare struct literal in a test)
		// still has to answer correctly, so it asks the database. Wiring builds
		// the cache, so this is not the production path.
		allowed, err := ctx.readScopedPluginAccess()
		if err != nil {
			return false
		}
		return allowed[pluginName]
	}
	snapshot := ctx.scopedAccess.allowed.Load()
	if snapshot == nil {
		loaded, err := ctx.loadScopedPluginAccess()
		if err != nil {
			return false
		}
		snapshot = loaded
	}
	return (*snapshot)[pluginName]
}

// SetPluginScopedAccess records whether group-limited principals may reach a
// plugin, and refreshes the snapshot so the next render sees it.
func (ctx *MahresourcesContext) SetPluginScopedAccess(pluginName string, allowed bool) error {
	if pluginName == "" {
		return fmt.Errorf("plugin name is required")
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

	_, err := ctx.loadScopedPluginAccess()
	return err
}

// InvalidateScopedPluginAccess drops the snapshot so the next read reloads it.
// Called wherever plugin rows change underneath this cache by some other route.
func (ctx *MahresourcesContext) InvalidateScopedPluginAccess() {
	if ctx.scopedAccess != nil {
		ctx.scopedAccess.allowed.Store(nil)
	}
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

func (ctx *MahresourcesContext) loadScopedPluginAccess() (*map[string]bool, error) {
	allowed, err := ctx.readScopedPluginAccess()
	if err != nil {
		return nil, err
	}
	if ctx.scopedAccess != nil {
		ctx.scopedAccess.allowed.Store(&allowed)
	}
	return &allowed, nil
}
