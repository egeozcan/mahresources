package plugin_system

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaShortcodeRenderTimeout = 5 * time.Second

type PluginShortcode struct {
	PluginName string
	TypeName   string // full: plugin:<pluginName>:<name>
	Label      string
	Render     *lua.LFunction
	State      *lua.LState
}

var validShortcodeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)

func parseShortcodeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginShortcode, error) {
	sc := &PluginShortcode{PluginName: pluginName}

	if v := tbl.RawGetString("name"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'name'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'name' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validShortcodeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid shortcode name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		sc.TypeName = "plugin:" + pluginName + ":" + raw
	}

	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		sc.Label = v.String()
	}

	if v := tbl.RawGetString("render"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render' must be a function")
	} else {
		sc.Render = fn
	}

	return sc, nil
}

func (pm *PluginManager) GetPluginShortcode(fullTypeName string) *PluginShortcode {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, scs := range pm.shortcodes {
		for _, sc := range scs {
			if sc.TypeName == fullTypeName {
				return sc
			}
		}
	}
	return nil
}

func (pm *PluginManager) RenderShortcode(pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	sc := pm.GetPluginShortcode(fullTypeName)
	if sc == nil {
		return "", fmt.Errorf("shortcode %q not found", fullTypeName)
	}
	if sc.PluginName != pluginName {
		return "", fmt.Errorf("shortcode %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	fn := sc.Render
	if fn == nil {
		return "", fmt.Errorf("no render function for shortcode %q", fullTypeName)
	}

	L := sc.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	var metaMap map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &metaMap)
	}
	if metaMap == nil {
		metaMap = map[string]any{}
	}

	attrsMap := make(map[string]any, len(attrs))
	for k, v := range attrs {
		attrsMap[k] = v
	}

	settings := pm.GetPluginSettings(pluginName)
	if settings == nil {
		settings = map[string]any{}
	}

	ctxData := map[string]any{
		"entity_type": entityType,
		"entity_id":   float64(entityID),
		"value":       metaMap,
		"attrs":       attrsMap,
		"settings":    settings,
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaShortcodeRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: shortcode render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("shortcode render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}
