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

// luaTableToGoMap converts a Lua table to a Go map.
func luaTableToGoMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		if k, ok := key.(lua.LString); ok {
			result[string(k)] = luaValueToGo(value)
		}
	})
	return result
}

// luaTableToGo converts a Lua table to either []any (if array-like) or map[string]any.
// A table is array-like if it has only consecutive integer keys starting from 1 with no gaps.
func luaTableToGo(tbl *lua.LTable) any {
	maxN := tbl.MaxN()
	if maxN > 0 {
		totalKeys := 0
		tbl.ForEach(func(_, _ lua.LValue) {
			totalKeys++
		})
		if totalKeys == maxN {
			arr := make([]any, maxN)
			for i := 1; i <= maxN; i++ {
				arr[i-1] = luaValueToGo(tbl.RawGetInt(i))
			}
			return arr
		}
	}

	// Mixed or string-keyed table → map
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		switch k := key.(type) {
		case lua.LString:
			result[string(k)] = luaValueToGo(value)
		case lua.LNumber:
			result[lua.LVAsString(key)] = luaValueToGo(value)
		}
	})
	return result
}

// luaValueToGo converts a Lua value to its Go equivalent.
func luaValueToGo(v lua.LValue) any {
	switch val := v.(type) {
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		return luaTableToGo(val)
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
		mu := pm.VMLock(L)
		mu.Lock()

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
		mu := pm.VMLock(L)
		mu.Lock()

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
