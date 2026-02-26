package plugin_system

import (
	"log"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// RenderSlot executes all injection renderers registered for the given slot,
// concatenates their string outputs, and returns the combined HTML.
// Errors in individual renderers are logged and skipped.
func (pm *PluginManager) RenderSlot(slot string, ctx map[string]any) string {
	injections := pm.GetInjections(slot)
	if len(injections) == 0 {
		return ""
	}

	var parts []string
	for _, inj := range injections {
		L := inj.state
		tbl := goToLuaTable(L, ctx)

		err := L.CallByParam(lua.P{
			Fn:      inj.fn,
			NRet:    1,
			Protect: true,
		}, tbl)

		if err != nil {
			log.Printf("[plugin] warning: injection for slot %q returned error: %v", slot, err)
			continue
		}

		ret := L.Get(-1)
		L.Pop(1)

		if str, ok := ret.(lua.LString); ok {
			parts = append(parts, string(str))
		}
	}

	return strings.Join(parts, "")
}
