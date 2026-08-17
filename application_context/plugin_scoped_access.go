package application_context

import (
	"fmt"
	"sync"
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

// errScopedAccessStale is returned when a load takes longer than the cache
// trusts an answer for. It is a refusal rather than a result: the caller is
// deciding a permission, and reads it as "no" like any other failure to find
// out.
var errScopedAccessStale = fmt.Errorf("plugin access read outlived its own freshness window")

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
	// publishing makes the compare-and-store atomic, and it is deliberately
	// held across NOTHING ELSE. An earlier version held it across the database
	// read too, which inverts against the connection pool: a caller inside a
	// transaction holds the connection and then asks this cache a question,
	// while the loader holds this lock and waits for a connection. On SQLite,
	// where the pool can be one connection, that is a deadlock — and the
	// clones a transaction makes share this very cache pointer.
	publishing sync.Mutex
	// ttl overrides scopedAccessTTL. Zero means the constant; only tests set
	// it, because the expiry paths are otherwise reachable in thirty-second
	// increments and would be tested by sleeping or not at all.
	ttl time.Duration
}

func (c *scopedPluginAccess) lifetime() time.Duration {
	if c.ttl > 0 {
		return c.ttl
	}
	return scopedAccessTTL
}

// stale reports whether an answer is older than the cache trusts.
func (c *scopedPluginAccess) stale(snapshot *scopedAccessSnapshot) bool {
	return time.Since(snapshot.observedBy) > c.lifetime()
}

type scopedAccessSnapshot struct {
	allowed    map[string]bool
	generation uint64
	// observedBy is a time the database had certainly not been read before —
	// taken BEFORE the query, so it is an upper bound on this answer's age
	// rather than a description of when the work finished.
	//
	// That is what makes the TTL a real bound. Stamping completion instead lets
	// a loader observe the old permission, stall arbitrarily, and then publish
	// with a fresh timestamp: the stale answer is trusted for another full TTL
	// measured from long after it was true, and a repeatedly slow loader
	// extends that indefinitely.
	//
	// Two concurrent loaders are NOT ordered, and no local clock can order
	// them: a loader that started first may observe last, so neither its start
	// nor its finish says whose answer is newer. Two attempts to order them
	// were written and both were wrong. The bound is what survives — a
	// revocation is served for at most one TTL after it commits, whether it was
	// missed by an unlucky interleaving or belongs to another process — and the
	// TTL is the number to change if that is ever too long.
	observedBy time.Time
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
		// the cache, so this is not the production path — but it gets the same
		// age check anyway, because a bound that holds only on some paths is
		// not a bound, and the next caller to build a context by hand should
		// not have to know which kind they made.
		observedBy := time.Now()
		allowed, err := ctx.readScopedPluginAccess()
		if err != nil || time.Since(observedBy) > scopedAccessTTL {
			return false
		}
		return allowed[pluginName]
	}

	generation := cache.generation.Load()
	snapshot := cache.snapshot.Load()
	if snapshot == nil || snapshot.generation != generation || cache.stale(snapshot) {
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

	observedBy := time.Now()
	var generation uint64
	if cache != nil {
		generation = cache.generation.Load()
	}

	// The read happens with no lock held, on purpose — see publishing.
	allowed, err := ctx.readScopedPluginAccess()
	if err != nil {
		return nil, err
	}
	snapshot := &scopedAccessSnapshot{allowed: allowed, generation: generation, observedBy: observedBy}

	if cache != nil {
		// Refusing to publish is not enough on its own: this loader's OWN
		// caller would still be served the answer it just judged too old, and
		// that is the interleaving the bound turns on — a read that saw the
		// permission, a revocation, and a stall past the TTL. An answer the
		// cache will not keep is not an answer this call may use either.
		if !cache.publish(snapshot) && cache.stale(snapshot) {
			return nil, errScopedAccessStale
		}
	}
	return snapshot, nil
}

// publish installs a snapshot unless a local decision has moved past it, and
// reports whether it did.
//
// Two rules, both decidable without knowing anything about other loaders:
//
//   - The generation must not have moved. An invalidation during the read means
//     this answer predates a decision made in this process, and a process knows
//     the order of its own decisions exactly.
//   - The answer must not already be older than the TTL. A read that stalled
//     long enough to expire has nothing to offer the next caller, and
//     publishing it would restart the clock on an answer that was already out
//     of time.
//
// It deliberately does NOT try to order two concurrent loaders. That was
// attempted twice — first by publish time, then by start time — and both are
// wrong for the same reason: neither says when the database was actually
// observed, and a loader that starts first can observe last. What survives is
// the bound in loadedAt's comment: an answer this process did not decide can be
// stale by at most one TTL.
//
// A losing loader still returns its own result to its own caller.
func (c *scopedPluginAccess) publish(snapshot *scopedAccessSnapshot) bool {
	c.publishing.Lock()
	defer c.publishing.Unlock()

	if c.generation.Load() != snapshot.generation {
		return false
	}
	if c.stale(snapshot) {
		return false
	}
	c.snapshot.Store(snapshot)
	return true
}
