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

	// mah.kv.ABSENT is the expectation "nothing is stored under this key yet".
	//
	// It needs a value of its own because Lua nil is already taken: set(k, nil)
	// stores a JSON null, so nil as an expectation means "the key holds null",
	// which is a different state from a key that is not there. Recognised by
	// identity before the expectation is serialized: through the encoder it
	// would arrive as an empty object, which is a value an author can
	// legitimately store.
	absent := L.NewTable()
	kvMod.RawSetString("ABSENT", absent)

	// The cap the store enforces, readable before a write rather than only
	// discoverable by failing one: a value over it raises, which takes the
	// author's handler with it unless they wrapped the call.
	kvMod.RawSetString("max_value_size", lua.LNumber(MaxKVValueSize))

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

	// mah.kv.compare_and_set(key, expected, value) -> boolean
	//
	// The answer to a key that something else may have written between an
	// author's read and their write: a later call into the same plugin, whose
	// VM lock the earlier call no longer holds, or the same plugin in another
	// process, which never shared that lock.
	//
	// The refusal is a return value rather than a raise because retrying is the
	// author's whole response to losing the race, and a handler that has already
	// been unwound cannot retry.
	//
	// Deliberately a plain compare rather than an update callback the host
	// invokes while holding the key: that shape holds a lock across arbitrary
	// Lua. This package has spent its last three changes taking such holds back
	// out -- a blocking HTTP call capped at its caller's budget, an abandoned
	// request that stops waiting for a busy plugin, one plugin's callbacks no
	// longer queued behind another's -- and is not the place to add one.
	kvMod.RawSetString("compare_and_set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		expectedVal := L.CheckAny(2)
		val := L.CheckAny(3)
		kv := pm.kvStoreFor(L)
		if kv == nil {
			// Not a lost race. Answering false here would tell an author that
			// somebody else got in first and invite them to retry forever.
			L.RaiseError("kv store not available")
			return 0
		}
		var expected *string
		if expectedVal != lua.LValue(absent) {
			expectedJSON, err := json.Marshal(luaValueToGoForJson(expectedVal))
			if err != nil {
				L.RaiseError("failed to serialize expected value: %s", err.Error())
				return 0
			}
			// Serialized by the same encoder set uses, or a table read back
			// with mah.kv.get could never be compared against what is stored.
			asString := string(expectedJSON)
			expected = &asString
		}
		jsonBytes, err := json.Marshal(luaValueToGoForJson(val))
		if err != nil {
			L.RaiseError("failed to serialize value: %s", err.Error())
			return 0
		}
		wrote, err := kv.KVCompareAndSet(*pluginNamePtr, key, expected, string(jsonBytes))
		if err != nil {
			L.RaiseError("kv compare_and_set failed: %s", err.Error())
			return 0
		}
		L.Push(lua.LBool(wrote))
		return 1
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
