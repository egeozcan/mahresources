package plugin_system

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// PluginAbortError is returned when a plugin calls mah.abort().
type PluginAbortError struct {
	Reason string
}

func (e *PluginAbortError) Error() string {
	return fmt.Sprintf("plugin aborted: %s", e.Reason)
}

// goToLuaTable converts a Go map to a Lua table.
func goToLuaTable(L *lua.LState, data map[string]any) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range data {
		tbl.RawSetString(k, goToLuaValue(L, v))
	}
	return tbl
}

// goToLuaValue converts a Go value to its Lua equivalent.
func goToLuaValue(L *lua.LState, v any) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case float32:
		return lua.LNumber(float64(val))
	case int:
		return lua.LNumber(float64(val))
	case int64:
		return lua.LNumber(float64(val))
	case uint:
		return lua.LNumber(float64(val))
	case uint64:
		return lua.LNumber(float64(val))
	case bool:
		return lua.LBool(val)
	case map[string]any:
		return goToLuaTable(L, val)
	case []any:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLuaValue(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", val))
	}
}

// Bounds on converting a Lua table to Go.
//
// Lua tables can be cyclic — `local t = {}; t.self = t` is legal, and a plugin
// can return one from a hook or an async handler — and they can also share
// subtables, so a cycle is not the only way to make the walk explode. A depth
// cap alone does not help: `t.a = t; t.b = t` doubles the work at every level,
// so it still exhausts memory long before the cap bites.
//
// Two bounds, therefore. `seen` holds the tables on the current path, which
// detects a cycle exactly and stops it at the first repeat rather than at some
// arbitrary depth. `budget` caps total tables visited, which bounds the shared-
// subtable blowup that no per-path check can see. The depth cap stays as a
// backstop, generous enough that real nested metadata is unaffected.
const (
	maxLuaConversionDepth = 256
	maxLuaConversionNodes = 100_000
)

// luaConversion carries the bounds down the walk.
type luaConversion struct {
	seen   map[*lua.LTable]bool
	budget int
	warned bool
}

func newLuaConversion() *luaConversion {
	return &luaConversion{seen: make(map[*lua.LTable]bool), budget: maxLuaConversionNodes}
}

// enter reports whether tbl may be walked, and marks it as on the current path.
func (c *luaConversion) enter(tbl *lua.LTable, depth int) bool {
	if depth >= maxLuaConversionDepth || c.budget <= 0 || c.seen[tbl] {
		return false
	}
	c.seen[tbl] = true
	return true
}

func (c *luaConversion) leave(tbl *lua.LTable) {
	delete(c.seen, tbl)
}

// spend charges one emitted value against the budget, reporting whether there
// was room for it.
//
// The budget counts values produced, not tables visited. Counting tables bounds
// the wrong thing: a table shared by both branches at each of 17 levels is only
// a few dozen distinct tables, but re-expanding a fat leaf under each path
// emits tens of thousands of copies of it. What must be bounded is the size of
// the Go value being built.
func (c *luaConversion) spend() bool {
	if c.budget <= 0 {
		// Once, not per dropped value: a blown budget drops a great many.
		if !c.warned {
			c.warned = true
			log.Printf("[plugin] warning: a Lua value exceeded the %d-value conversion budget and was truncated; "+
				"this usually means a table shares a subtable across many branches", maxLuaConversionNodes)
		}
		return false
	}
	c.budget--
	return true
}

// Truncated reports whether the budget ran out, so a caller that can surface it
// does not have to infer truncation from a suspiciously round result.
func (c *luaConversion) Truncated() bool { return c.warned }

// luaTableToGoMap converts a Lua table to a Go map.
func luaTableToGoMap(tbl *lua.LTable) map[string]any {
	return luaTableToGoMapDepth(tbl, 0, newLuaConversion())
}

func luaTableToGoMapDepth(tbl *lua.LTable, depth int, c *luaConversion) map[string]any {
	result := make(map[string]any)
	if !c.enter(tbl, depth) {
		return result
	}
	defer c.leave(tbl)
	tbl.ForEach(func(key, value lua.LValue) {
		if k, ok := key.(lua.LString); ok {
			if !c.spend() {
				return
			}
			result[string(k)] = luaValueToGoDepth(value, depth+1, c)
		}
	})
	return result
}

func luaTableToGoDepth(tbl *lua.LTable, depth int, c *luaConversion) any {
	if !c.enter(tbl, depth) {
		return nil
	}
	defer c.leave(tbl)
	maxN := tbl.MaxN()
	if maxN > 0 {
		totalKeys := 0
		tbl.ForEach(func(_, _ lua.LValue) {
			totalKeys++
		})
		if totalKeys == maxN {
			arr := make([]any, 0, maxN)
			for i := 1; i <= maxN; i++ {
				if !c.spend() {
					break
				}
				arr = append(arr, luaValueToGoDepth(tbl.RawGetInt(i), depth+1, c))
			}
			return arr
		}
	}

	// Mixed or string-keyed table → map
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		switch k := key.(type) {
		case lua.LString:
			if !c.spend() {
				return
			}
			result[string(k)] = luaValueToGoDepth(value, depth+1, c)
		case lua.LNumber:
			if !c.spend() {
				return
			}
			result[lua.LVAsString(key)] = luaValueToGoDepth(value, depth+1, c)
		}
	})
	return result
}

// luaValueToGo converts a Lua value to its Go equivalent.
func luaValueToGo(v lua.LValue) any {
	return luaValueToGoDepth(v, 0, newLuaConversion())
}

func luaValueToGoDepth(v lua.LValue, depth int, c *luaConversion) any {
	switch val := v.(type) {
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		return luaTableToGoDepth(val, depth, c)
	case *lua.LNilType:
		return nil
	default:
		return nil
	}
}

// skipReentrantHook reports whether dispatching hook would re-enter a plugin VM
// that is already executing on this call chain, and logs it when it does.
//
// Taking that VM's lock again from the same goroutine wedges it permanently: the
// mutex is not reentrant, the 5s Lua timeout cannot preempt a block inside a Go
// call, and the lock is never released, so every later entry into that plugin
// queues behind it forever with no panic and nothing in the log.
//
// Both shapes are covered because inv carries the whole chain rather than one
// state: a plugin writing an entity it hooks itself, and two plugins whose hooks
// write entities the other hooks. The write still happens; only the notification
// is dropped, so a plugin is never told about a write made while it was already
// running. See docs/plans/2026-08-15-plugin-invocation-and-hook-integrity.md §2.
func (pm *PluginManager) skipReentrantHook(inv *Invocation, hook hookEntry, event string) bool {
	if !inv.holds(hook.state) {
		return false
	}
	pm.warnHookSkipped(inv, hook.pluginName, event,
		"it is already running on this call chain (a plugin is not notified of writes made while it is running)")
	return true
}

// warnHookSkipped reports a dropped hook dispatch through the application log,
// so it is visible at /logs rather than only on stderr. Falls back to the
// process log when no logger is wired (tests, and before wiring completes).
func (pm *PluginManager) warnHookSkipped(inv *Invocation, pluginName, event, reason string) {
	msg := fmt.Sprintf("skipped %q hook: %s", event, reason)
	if pl := pm.loggerForInvocation(inv); pl != nil {
		pl.PluginLog(pluginName, "warning", msg, map[string]any{"event": event})
		return
	}
	log.Printf("[plugin] warning: plugin %q: %s", pluginName, msg)
}

// ErrHookVMBusy is returned by RunBeforeHooks when a nested dispatch could not
// take a hook's VM lock in time.
//
// It exists so contention cannot silently bypass a veto: see lockVMForHook.
var ErrHookVMBusy = errors.New("plugin hook could not run: its VM was busy")

// lockVMForHook takes hook's VM lock, bounding the wait only when this goroutine
// already holds one — the sole condition under which it can be half of a lock
// cycle. It also gives the wait up whenever reqCtx ends.
//
// reqCtx belongs to whoever made the write that raised the event, and only a
// before-hook has one. RunAfterHooks passes Background, and the argument for
// that is at its own declaration.
//
// Returns (nil, false) when the plugin is gone, which is always a safe skip, and
// (nil, true) when the plugin is alive but its lock was not taken: the nested
// bound expired, or reqCtx did. The two are separated because the caller's
// correct response differs, and the difference matters: for an *after* hook a
// timeout is a missed notification of something already committed, so skipping
// is honest; for a *before* hook it is a veto that never got to run, and
// skipping it would mean an unrelated plugin being busy silently disables a
// protection hook. RunBeforeHooks therefore fails the operation instead.
func (pm *PluginManager) lockVMForHook(reqCtx context.Context, inv *Invocation, hook hookEntry, event string) (*vmMutex, bool) {
	var (
		mu   *vmMutex
		busy bool
	)
	if inv == nil || len(inv.states) == 0 {
		// Top-level dispatch: holds no VM lock, so it cannot deadlock against
		// another goroutine's hook dispatch. Wait as long as its caller waits.
		var err error
		mu, err = pm.LockVMWithContext(reqCtx, hook.state)
		busy = err != nil
	} else {
		mu, busy = pm.TryLockVMWithin(reqCtx, hook.state, hookLockWait)
	}

	// Re-check registration on *both* outcomes, because waiting is exactly when
	// a hook can stop existing. DisablePlugin unregisters hooks first and only
	// drops the vmLocks entry once it has acquired the VM lock, so for the whole
	// time it is blocked on a busy VM the plugin still looks live: a dispatcher
	// working from a snapshot taken before that would fail a caller's write over
	// a hook that no longer exists (on timeout), or run one (on acquisition).
	// Both are wrong, and the liveness of the vmLocks entry cannot tell them
	// apart — the registration is the condition that actually matters.
	if !pm.hookStillRegistered(hook, event) {
		if mu != nil {
			mu.Unlock()
		}
		return nil, false
	}
	return mu, busy
}

// hookStillRegistered reports whether hook is still registered for event.
func (pm *PluginManager) hookStillRegistered(hook hookEntry, event string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, h := range pm.hooks[event] {
		if h.state == hook.state && h.fn == hook.fn {
			return true
		}
	}
	return false
}

// hookContext returns the context a hook's Lua call runs under: Background, not
// the request, because a hook has no request of its own — but carrying inv, so a
// mah.db write made *from* the hook sees the chain it is on and can refuse to
// re-enter any VM already in it.
func hookContext(inv *Invocation) (context.Context, context.CancelFunc) {
	return context.WithTimeout(withInvocation(context.Background(), inv), luaExecTimeout)
}

// beforeHookWaitFailed names which of the two waits ended, because they have
// different remedies and only one of them is about the plugin at all: a VM held
// past the bound is the plugin author's problem, a caller that hung up is
// nobody's.
//
// Decided from the caller rather than from the wait, and in that order
// deliberately. When both have happened (the nested bound expired, and the
// caller went away) the caller is gone either way, so reporting the request as
// abandoned is true whichever of the two tripped first.
func beforeHookWaitFailed(reqCtx context.Context, event, pluginName string) error {
	if reqCtx != nil && reqCtx.Err() != nil {
		return fmt.Errorf("%q hook for plugin %q: %w", event, pluginName, errVMWaitAbandoned)
	}
	return fmt.Errorf("%w: %q hook for plugin %q did not start within %s",
		ErrHookVMBusy, event, pluginName, hookLockWait)
}

// RunBeforeHooks executes all registered hooks for the given event sequentially.
// Each hook receives the data, can modify it, and returns the modified data.
// If a hook calls mah.abort(), a PluginAbortError is returned.
// If a hook has a runtime error, it is logged and skipped.
//
// inv identifies the actor whose write fired this event and the plugin VMs
// already executing on the call chain. It may be nil, which means "no actor and
// no chain" — the shape a caller with no plugin manager wiring produces.
//
// reqCtx bounds only the wait for a busy plugin's VM, not the hook's own
// execution: a caller that has gone away stops queueing behind somebody else's
// call into that plugin, and its write fails. That is safe precisely because it
// is a before-hook. Nothing has been written yet, so nobody is left believing
// otherwise, and it is the same answer ErrHookVMBusy already gives when a
// nested dispatch's bound expires. RunAfterHooks may not do this; see there.
func (pm *PluginManager) RunBeforeHooks(reqCtx context.Context, inv *Invocation, event string, data map[string]any) (map[string]any, error) {
	if pm.closed.Load() {
		return data, nil
	}
	hooks := pm.GetHooks(event)
	if len(hooks) == 0 {
		return data, nil
	}

	for _, hook := range hooks {
		L := hook.state
		if pm.skipReentrantHook(inv, hook, event) {
			continue
		}
		mu, busy := pm.lockVMForHook(reqCtx, inv, hook, event)
		if mu == nil {
			if busy {
				// Fail closed. This hook may be the one that would have vetoed,
				// and we cannot know without running it.
				return nil, beforeHookWaitFailed(reqCtx, event, hook.pluginName)
			}
			// The plugin was disabled between GetHooks and here; its state is
			// being closed, so skip it rather than dereferencing a nil lock.
			continue
		}

		tbl := goToLuaTable(L, data)

		timeoutCtx, cancel := hookContext(inv.with(L))
		L.SetContext(timeoutCtx)

		err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    1,
			Protect: true,
		}, tbl)

		L.RemoveContext()
		cancel()

		if err != nil {
			mu.Unlock()
			if isAbort, reason := parseAbortError(err); isAbort {
				return nil, &PluginAbortError{Reason: reason}
			}
			log.Printf("[plugin] warning: hook for %q returned error: %v", event, err)
			continue
		}

		// Read the return value — if the hook returned a table, use it as the new data.
		ret := L.Get(-1)
		L.Pop(1)

		if retTbl, ok := ret.(*lua.LTable); ok {
			data = luaTableToGoMap(retTbl)
		}

		mu.Unlock()
	}

	return data, nil
}

// RunAfterHooks executes all registered hooks for the given event.
// Errors are logged and ignored; execution is synchronous — on the caller's own
// goroutine, which is why inv has to carry the whole chain. See RunBeforeHooks.
//
// It takes no caller context, and the omission is the point. An after-hook
// describes a write that has already committed, so abandoning it drops
// bookkeeping for a change that really happened and leaves the plugin's view of
// the database diverged from the database, permanently and silently. A client
// that hung up is not a reason for that. The deferred queue makes the gap wider
// still: those hooks are dispatched once a plugin transaction commits, by which
// point the request that opened it may be gone by design, so honouring a caller
// context here would skip them routinely rather than rarely.
func (pm *PluginManager) RunAfterHooks(inv *Invocation, event string, data map[string]any) {
	if pm.closed.Load() {
		return
	}
	hooks := pm.GetHooks(event)
	if len(hooks) == 0 {
		return
	}

	for _, hook := range hooks {
		L := hook.state
		if pm.skipReentrantHook(inv, hook, event) {
			continue
		}
		mu, busy := pm.lockVMForHook(context.Background(), inv, hook, event)
		if mu == nil {
			if busy {
				// Safe to skip: the change is already committed, so this is a
				// missed notification rather than a bypassed guard.
				pm.warnHookSkipped(inv, hook.pluginName, event, fmt.Sprintf(
					"its VM was busy for %s while this call already held another plugin's VM; "+
						"skipped rather than risking a lock cycle", hookLockWait))
			}
			continue
		}

		tbl := goToLuaTable(L, data)

		timeoutCtx, cancel := hookContext(inv.with(L))
		L.SetContext(timeoutCtx)

		err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, tbl)

		L.RemoveContext()
		cancel()

		mu.Unlock()

		if err != nil {
			log.Printf("[plugin] warning: after-hook for %q returned error: %v", event, err)
		}
	}
}

// parseAbortError checks if a Lua error contains the PLUGIN_ABORT marker
// and extracts the abort reason. The reason is trimmed to the first line
// since gopher-lua appends a stack trace after the error message.
func parseAbortError(err error) (bool, string) {
	msg := err.Error()
	const marker = "PLUGIN_ABORT: "
	idx := strings.Index(msg, marker)
	if idx == -1 {
		return false, ""
	}
	reason := msg[idx+len(marker):]
	// Trim stack trace (everything after the first newline).
	if nl := strings.IndexByte(reason, '\n'); nl != -1 {
		reason = reason[:nl]
	}
	return true, reason
}
