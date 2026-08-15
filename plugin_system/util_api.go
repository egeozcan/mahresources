package plugin_system

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// registerUtilModule registers the mah.util sub-table: the small primitives a
// plugin cannot build for itself inside the sandbox.
//
// The VM opens base, table, string, math and coroutine and nothing else, so a
// plugin has no clock (it cannot timestamp a record or expire a cached value),
// no base64 (data-views hand-rolls a decoder in pure Lua to read what
// get_resource_data returns), and no hashing (a mah.api endpoint receiving a
// webhook has to trust any caller, because it cannot verify a signature).
//
// Every function here is a direct wrapper over the Go standard library with no
// filesystem, process or network reach, so the sandbox posture is unchanged.
func (pm *PluginManager) registerUtilModule(L *lua.LState, mahMod *lua.LTable) {
	utilMod := L.NewTable()

	// mah.util.now() -> number
	// Unix seconds. Fractional, so sub-second durations are measurable.
	utilMod.RawSetString("now", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(float64(time.Now().UnixNano()) / float64(time.Second)))
		return 1
	}))

	// mah.util.now_iso() -> string
	// RFC3339 in UTC. Deliberately not local time: local-offset timestamps
	// compare lexicographically against UTC bounds and silently mis-sort, which
	// this codebase has been bitten by before.
	utilMod.RawSetString("now_iso", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(time.Now().UTC().Format(time.RFC3339)))
		return 1
	}))

	// mah.util.base64.encode(str) -> string
	// mah.util.base64.decode(str) -> string or (nil, error)
	b64Mod := L.NewTable()
	b64Mod.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(base64.StdEncoding.EncodeToString([]byte(L.CheckString(1)))))
		return 1
	}))
	b64Mod.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		decoded, err := base64.StdEncoding.DecodeString(L.CheckString(1))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(decoded))
		return 1
	}))
	utilMod.RawSetString("base64", b64Mod)

	// mah.util.hex.encode(str) -> string
	// mah.util.hex.decode(str) -> string or (nil, error)
	hexMod := L.NewTable()
	hexMod.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(hex.EncodeToString([]byte(L.CheckString(1)))))
		return 1
	}))
	hexMod.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		decoded, err := hex.DecodeString(L.CheckString(1))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(decoded))
		return 1
	}))
	utilMod.RawSetString("hex", hexMod)

	// mah.util.sha256(str) -> lowercase hex string
	utilMod.RawSetString("sha256", L.NewFunction(func(L *lua.LState) int {
		sum := sha256.Sum256([]byte(L.CheckString(1)))
		L.Push(lua.LString(hex.EncodeToString(sum[:])))
		return 1
	}))

	// mah.util.hmac_sha256(key, message) -> lowercase hex string
	// The signature check a webhook receiver needs.
	utilMod.RawSetString("hmac_sha256", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		message := L.CheckString(2)
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(message))
		L.Push(lua.LString(hex.EncodeToString(mac.Sum(nil))))
		return 1
	}))

	// mah.util.secure_compare(a, b) -> boolean
	// Constant-time for equal-length inputs. Comparing a computed signature
	// with == leaks it a byte at a time through timing, and a plugin cannot
	// write a constant-time compare in Lua.
	utilMod.RawSetString("secure_compare", L.NewFunction(func(L *lua.LState) int {
		a := []byte(L.CheckString(1))
		b := []byte(L.CheckString(2))
		L.Push(lua.LBool(subtle.ConstantTimeCompare(a, b) == 1))
		return 1
	}))

	mahMod.RawSetString("util", utilMod)
}
