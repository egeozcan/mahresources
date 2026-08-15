package plugin_system

import (
	"context"
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

// luaTableToGo converts a Lua table to either []any (if array-like) or map[string]any.
// A table is array-like if it has only consecutive integer keys starting from 1 with no gaps.
func luaTableToGo(tbl *lua.LTable) any {
	return luaTableToGoDepth(tbl, 0, newLuaConversion())
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

// RunBeforeHooks executes all registered hooks for the given event sequentially.
// Each hook receives the data, can modify it, and returns the modified data.
// If a hook calls mah.abort(), a PluginAbortError is returned.
// If a hook has a runtime error, it is logged and skipped.
func (pm *PluginManager) RunBeforeHooks(event string, data map[string]any) (map[string]any, error) {
	if pm.closed.Load() {
		return data, nil
	}
	hooks := pm.GetHooks(event)
	if len(hooks) == 0 {
		return data, nil
	}

	for _, hook := range hooks {
		L := hook.state
		mu := pm.LockVM(L)
		if mu == nil {
			// The plugin was disabled between GetHooks and here; its state is
			// being closed, so skip it rather than dereferencing a nil lock.
			continue
		}

		tbl := goToLuaTable(L, data)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
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
// Errors are logged and ignored; execution is synchronous.
func (pm *PluginManager) RunAfterHooks(event string, data map[string]any) {
	if pm.closed.Load() {
		return
	}
	hooks := pm.GetHooks(event)
	if len(hooks) == 0 {
		return
	}

	for _, hook := range hooks {
		L := hook.state
		mu := pm.LockVM(L)
		if mu == nil {
			// The plugin was disabled between GetHooks and here; its state is
			// being closed, so skip it rather than dereferencing a nil lock.
			continue
		}

		tbl := goToLuaTable(L, data)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
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
