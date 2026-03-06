package plugin_system

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	defaultAPITimeout = 30 * time.Second
	maxAPITimeout     = 120 * time.Second
)

// APIEndpoint stores a Lua API handler and its parent VM.
type APIEndpoint struct {
	state   *lua.LState
	fn      *lua.LFunction
	timeout time.Duration
}

// APIResponse holds the result of executing a plugin API handler.
type APIResponse struct {
	StatusCode int
	Body       any
	Error      string
}

// HandleAPI executes the Lua API handler for the given plugin, method, and path,
// passing the request context, and returns an APIResponse.
func (pm *PluginManager) HandleAPI(pluginName, method, path string, ctx PageContext) APIResponse {
	if pm.closed.Load() {
		return APIResponse{StatusCode: 500, Error: "plugin manager is closed"}
	}

	method = strings.ToUpper(method)

	pm.mu.RLock()
	endpoints, ok := pm.apiEndpoints[pluginName]
	if !ok {
		pm.mu.RUnlock()
		return APIResponse{StatusCode: 404, Error: "plugin not found"}
	}

	key := method + ":" + path
	endpoint, ok := endpoints[key]
	if !ok {
		// Check if the path exists with a different method → 405
		for k := range endpoints {
			parts := strings.SplitN(k, ":", 2)
			if len(parts) == 2 && parts[1] == path {
				pm.mu.RUnlock()
				return APIResponse{StatusCode: 405, Error: "method not allowed"}
			}
		}
		pm.mu.RUnlock()
		return APIResponse{StatusCode: 404, Error: "endpoint not found"}
	}
	pm.mu.RUnlock()

	L := endpoint.state
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

	// Inject ctx.json(data) and ctx.status(code) closures
	var responseBody any
	bodySet := false
	statusCode := 0 // 0 sentinel means "not explicitly set"

	tbl.RawSetString("json", L.NewFunction(func(L *lua.LState) int {
		val := L.Get(1)
		responseBody = luaValueToGoForJson(val)
		bodySet = true
		return 0
	}))

	tbl.RawSetString("status", L.NewFunction(func(L *lua.LState) int {
		code := L.CheckInt(1)
		statusCode = code
		return 0
	}))

	timeoutCtx, cancel := context.WithTimeout(context.Background(), endpoint.timeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      endpoint.fn,
		NRet:    0,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		// Check for PLUGIN_ABORT
		if isAbort, reason := parseAbortError(err); isAbort {
			sc := 400
			if statusCode != 0 {
				sc = statusCode
			}
			return APIResponse{StatusCode: sc, Error: reason}
		}

		// Check for context deadline exceeded (timeout)
		if timeoutCtx.Err() == context.DeadlineExceeded {
			log.Printf("[plugin] warning: api handler %q %s:%s timed out after %v", pluginName, method, path, endpoint.timeout)
			return APIResponse{StatusCode: 504, Error: fmt.Sprintf("handler timed out after %v", endpoint.timeout)}
		}

		log.Printf("[plugin] warning: api handler %q %s:%s returned error: %v", pluginName, method, path, err)
		return APIResponse{StatusCode: 500, Error: "internal plugin error"}
	}

	// Determine final status code
	finalStatus := statusCode
	if finalStatus == 0 {
		if bodySet {
			finalStatus = 200
		} else {
			finalStatus = 204
		}
	}

	return APIResponse{StatusCode: finalStatus, Body: responseBody}
}

// HasAPIEndpoint checks if a plugin has registered any API endpoint at the given path
// (regardless of HTTP method).
func (pm *PluginManager) HasAPIEndpoint(pluginName, path string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	endpoints, ok := pm.apiEndpoints[pluginName]
	if !ok {
		return false
	}
	for k := range endpoints {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) == 2 && parts[1] == path {
			return true
		}
	}
	return false
}
