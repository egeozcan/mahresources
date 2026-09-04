package plugin_system

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

const luaJSONArrayMarker = "__mah_json_array"

// registerJsonModule registers the mah.json sub-table in the Lua VM.
// Provides mah.json.encode(value), mah.json.decode(string), and
// mah.json.array(table).
func (pm *PluginManager) registerJsonModule(L *lua.LState, mahMod *lua.LTable) {
	jsonMod := L.NewTable()

	// mah.json.encode(value) -> string or (nil, error)
	jsonMod.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		val := L.CheckAny(1)
		goVal := luaValueToGoForJson(val)
		bytes, err := json.Marshal(goVal)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(bytes)))
		return 1
	}))

	// mah.json.decode(string) -> value or (nil, error)
	jsonMod.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		var goVal any
		if err := json.Unmarshal([]byte(str), &goVal); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaValue(L, goVal))
		return 1
	}))

	// mah.json.array(table) -> table
	//
	// Lua's empty table has no shape, so the normal encoder must treat {} as an
	// object. Plugins that expose a JSON list need a way to preserve [] when it
	// happens to be empty. Marking the table keeps ordinary Lua indexing and
	// ipairs behavior while giving both mah.json.encode and ctx.json the missing
	// shape information.
	jsonMod.RawSetString("array", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		maxN := tbl.MaxN()
		totalKeys := 0
		tbl.ForEach(func(_, _ lua.LValue) { totalKeys++ })
		if totalKeys != maxN {
			L.ArgError(1, "array must contain only consecutive integer keys starting at 1")
			return 0
		}
		metatable := L.NewTable()
		metatable.RawSetString(luaJSONArrayMarker, lua.LTrue)
		tbl.Metatable = metatable
		L.Push(tbl)
		return 1
	}))

	mahMod.RawSetString("json", jsonMod)
}

// luaValueToGoForJson converts a Lua value to a Go value suitable for JSON marshaling.
// Unlike luaValueToGo, this detects array-like Lua tables (consecutive integer keys 1..N)
// and returns []any instead of map[string]any.
func luaValueToGoForJson(v lua.LValue) any {
	switch val := v.(type) {
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		return luaTableToGoForJson(val)
	case *lua.LNilType:
		return nil
	default:
		return nil
	}
}

// luaTableToGoForJson converts a Lua table to either []any (if array-like) or map[string]any.
// A table is array-like if it has only consecutive integer keys starting from 1 with no gaps.
func luaTableToGoForJson(tbl *lua.LTable) any {
	maxN := tbl.MaxN() // highest consecutive integer key from 1
	explicitArray := false
	if metatable, ok := tbl.Metatable.(*lua.LTable); ok {
		explicitArray = lua.LVAsBool(metatable.RawGetString(luaJSONArrayMarker))
	}
	if maxN > 0 || explicitArray {
		// Check if the table is purely array-like (no string keys beyond the array part).
		totalKeys := 0
		tbl.ForEach(func(_, _ lua.LValue) {
			totalKeys++
		})
		if totalKeys == maxN {
			// Pure array
			arr := make([]any, maxN)
			for i := 1; i <= maxN; i++ {
				arr[i-1] = luaValueToGoForJson(tbl.RawGetInt(i))
			}
			return arr
		}
	}

	// Mixed or string-keyed table → map
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		switch k := key.(type) {
		case lua.LString:
			result[string(k)] = luaValueToGoForJson(value)
		case lua.LNumber:
			// Use numeric string key for mixed tables
			result[lua.LVAsString(key)] = luaValueToGoForJson(value)
		}
	})
	return result
}
