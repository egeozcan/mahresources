package plugin_system

import (
	"context"
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// RenderSlot executes all injection renderers registered for the given slot,
// concatenates their string outputs, and returns the combined HTML.
// Errors in individual renderers are logged and skipped.
func (pm *PluginManager) RenderSlot(slot string, ctx map[string]any) string {
	if pm.closed.Load() {
		return ""
	}
	injections := pm.GetInjections(slot)
	if len(injections) == 0 {
		return ""
	}

	var parts []string
	for _, inj := range injections {
		L := inj.state
		mu := pm.VMLock(L)
		mu.Lock()

		tbl := goToLuaTable(L, ctx)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
		L.SetContext(timeoutCtx)

		err := L.CallByParam(lua.P{
			Fn:      inj.fn,
			NRet:    1,
			Protect: true,
		}, tbl)

		L.RemoveContext()
		cancel()

		if err != nil {
			mu.Unlock()
			log.Printf("[plugin] warning: injection for slot %q returned error: %v", slot, err)
			continue
		}

		ret := L.Get(-1)
		L.Pop(1)

		if str, ok := ret.(lua.LString); ok {
			parts = append(parts, string(str))
		}

		mu.Unlock()
	}

	return strings.Join(parts, "")
}
