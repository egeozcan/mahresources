package plugin_system

import (
	"context"
	"fmt"
	"log"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaDisplayRenderTimeout = 5 * time.Second

// DisplayRenderContext holds all context passed to the Lua render function.
// Value is typed as any because metadata fields can be objects, arrays,
// strings, numbers, booleans, or null.
type DisplayRenderContext struct {
	Value      any            `json:"value"`
	Schema     map[string]any `json:"schema"`
	FieldPath  string         `json:"field_path"`
	FieldLabel string         `json:"field_label"`
	Settings   map[string]any `json:"settings"`
}

// RenderDisplay executes the Lua render function for a plugin display type
// and returns the rendered HTML string.
func (pm *PluginManager) RenderDisplay(pluginName, fullTypeName string, ctx DisplayRenderContext) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	dt := pm.GetPluginDisplayType(fullTypeName)
	if dt == nil {
		return "", fmt.Errorf("display type %q not found", fullTypeName)
	}
	if dt.PluginName != pluginName {
		return "", fmt.Errorf("display type %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	fn := dt.Render
	if fn == nil {
		return "", fmt.Errorf("no render function for display type %q", fullTypeName)
	}

	L := dt.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	ctxData := map[string]any{
		"value":       ctx.Value,
		"schema":      ctx.Schema,
		"field_path":  ctx.FieldPath,
		"field_label": ctx.FieldLabel,
	}
	if ctx.Settings != nil {
		ctxData["settings"] = ctx.Settings
	} else {
		ctxData["settings"] = map[string]any{}
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaDisplayRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: display render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("display render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}

// GetPluginDisplayType returns a specific plugin display type by full name, or nil.
func (pm *PluginManager) GetPluginDisplayType(fullTypeName string) *PluginDisplayType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, types := range pm.displayTypes {
		for _, dt := range types {
			if dt.TypeName == fullTypeName {
				return dt
			}
		}
	}
	return nil
}
