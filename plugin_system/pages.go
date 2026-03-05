package plugin_system

import (
	"context"
	"fmt"
	"log"

	lua "github.com/yuin/gopher-lua"
)

// PageContext holds the request data passed to a plugin page handler.
type PageContext struct {
	Path    string
	Method  string
	Query   map[string]any
	Params  map[string]any
	Headers map[string]any
	Body    string
}

// HandlePage executes the Lua page handler for the given plugin and path,
// passing the request context, and returns the rendered HTML string.
func (pm *PluginManager) HandlePage(pluginName, path string, ctx PageContext) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	pm.mu.RLock()
	pages, ok := pm.pages[pluginName]
	if !ok {
		pm.mu.RUnlock()
		return "", fmt.Errorf("no plugin %q registered", pluginName)
	}
	entry, ok := pages[path]
	if !ok {
		pm.mu.RUnlock()
		return "", fmt.Errorf("no page %q registered for plugin %q", path, pluginName)
	}
	pm.mu.RUnlock()

	L := entry.state
	mu := pm.VMLock(L)
	mu.Lock()
	defer mu.Unlock()

	// Build context table
	ctxData := map[string]any{
		"path":   ctx.Path,
		"method": ctx.Method,
	}
	if ctx.Query != nil {
		ctxData["query"] = ctx.Query
	} else {
		ctxData["query"] = map[string]any{}
	}
	if ctx.Params != nil {
		ctxData["params"] = ctx.Params
	} else {
		ctxData["params"] = map[string]any{}
	}
	if ctx.Headers != nil {
		ctxData["headers"] = ctx.Headers
	} else {
		ctxData["headers"] = map[string]any{}
	}
	if ctx.Body != "" {
		ctxData["body"] = ctx.Body
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      entry.fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: page handler %q/%q returned error: %v", pluginName, path, err)
		return "", fmt.Errorf("page handler error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}
