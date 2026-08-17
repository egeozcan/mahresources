package plugin_system

import (
	"encoding/json"

	lua "github.com/yuin/gopher-lua"
)

// registerKvModule registers the mah.kv sub-table in the Lua VM.
// kvStoreFor is getKVStore for a call made from L: nil for a revoked VM, which
// every caller already renders as "not available". Same reason as querierFor —
// a worker that was inside when its plugin was disabled keeps its mah table
// until it finishes, and a disable reported complete should not still be able
// to write the plugin's stored data.
func (pm *PluginManager) kvStoreFor(L *lua.LState) KVStore {
	if !pm.stateIsLive(L) {
		return nil
	}
	// Inside a mah.db.transaction, through the transaction — a plugin's stored
	// data is written to the same database as everything else, so a write on a
	// second connection would block on the lock the transaction holds. It also
	// means stored data rolls back with the transaction, which is what an author
	// who wrapped a cursor update and an entity write together asked for.
	if tx := pm.invocationFor(L).transactionBinding(); tx != nil {
		return tx
	}
	return pm.getKVStore()
}

func (pm *PluginManager) registerKvModule(L *lua.LState, mahMod *lua.LTable, pluginNamePtr *string) {
	kvMod := L.NewTable()

	// mah.kv.get(key) -> value or nil
	kvMod.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		kv := pm.kvStoreFor(L)
		if kv == nil {
			L.Push(lua.LNil)
			return 1
		}
		val, found, err := kv.KVGet(*pluginNamePtr, key)
		if err != nil {
			L.RaiseError("kv get failed: %s", err.Error())
			return 0
		}
		if !found {
			L.Push(lua.LNil)
			return 1
		}
		var goVal any
		if err := json.Unmarshal([]byte(val), &goVal); err != nil {
			L.RaiseError("kv get: failed to deserialize value: %s", err.Error())
			return 0
		}
		L.Push(goToLuaValue(L, goVal))
		return 1
	}))

	// mah.kv.set(key, value)
	kvMod.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.CheckAny(2)
		kv := pm.kvStoreFor(L)
		if kv == nil {
			L.RaiseError("kv store not available")
			return 0
		}
		goVal := luaValueToGoForJson(val)
		jsonBytes, err := json.Marshal(goVal)
		if err != nil {
			L.RaiseError("failed to serialize value: %s", err.Error())
			return 0
		}
		if err := kv.KVSet(*pluginNamePtr, key, string(jsonBytes)); err != nil {
			L.RaiseError("kv set failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// mah.kv.delete(key)
	kvMod.RawSetString("delete", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		kv := pm.kvStoreFor(L)
		if kv == nil {
			L.RaiseError("kv store not available")
			return 0
		}
		if err := kv.KVDelete(*pluginNamePtr, key); err != nil {
			L.RaiseError("kv delete failed: %s", err.Error())
			return 0
		}
		return 0
	}))

	// mah.kv.list([prefix]) -> table of key strings
	kvMod.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		prefix := ""
		if L.GetTop() >= 1 {
			prefix = L.CheckString(1)
		}
		kv := pm.kvStoreFor(L)
		if kv == nil {
			L.Push(L.NewTable())
			return 1
		}
		keys, err := kv.KVList(*pluginNamePtr, prefix)
		if err != nil {
			L.RaiseError("kv list failed: %s", err.Error())
			return 0
		}
		tbl := L.NewTable()
		for _, k := range keys {
			tbl.Append(lua.LString(k))
		}
		L.Push(tbl)
		return 1
	}))

	mahMod.RawSetString("kv", kvMod)
}
