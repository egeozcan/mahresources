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
//
// reqCtx is the caller's request context. Injections looked like the one render
// seam with no request to inherit — the template tag has no ctx parameter — but
// it already reads one out of the render context to decide whether plugin code
// may run at all, and simply did not pass it on. They are also the widest
// surface there is: six slots live in the base layout, so they run on every
// page. Pass nil where there genuinely is no request.
// allow decides, per plugin, whether this caller may see that plugin's
// injection. A nil filter runs every plugin's injection, which is what the
// tests in this package want; the one production caller
// (template_filters/plugin_slot.go) always passes a real one, and
// internal/arch/plugin_render_gate_test.go fails the build if it stops.
func (pm *PluginManager) RenderSlot(reqCtx context.Context, slot string, ctx map[string]any, allow func(pluginName string) bool) string {
	if pm.closed.Load() {
		return ""
	}
	injections := pm.GetInjections(slot)
	if len(injections) == 0 {
		return ""
	}

	var parts []string
	for _, inj := range injections {
		if allow != nil && !allow(inj.plugin) {
			continue
		}
		L := inj.state
		mu := pm.LockVM(L)
		if mu == nil {
			// The plugin was disabled between GetInjections and here; its state is
			// being closed, so skip it rather than dereferencing a nil lock.
			continue
		}

		tbl := goToLuaTable(L, ctx)

		timeoutCtx, cancel := context.WithTimeout(vmParentContext(reqCtx), luaExecTimeout)
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
