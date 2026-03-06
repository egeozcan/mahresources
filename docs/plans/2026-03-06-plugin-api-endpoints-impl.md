# Plugin JSON API Endpoints Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `mah.api()` to the plugin system so plugins can register JSON API endpoints at `/v1/plugins/{pluginName}/{path}`.

**Architecture:** Extends the existing page system pattern. New `api_endpoints.go` file for types + `HandleAPI()`. Registration via `mah.api()` Lua binding in `manager.go`. HTTP handler in `plugin_api_handlers.go`. Route in `routes.go`.

**Tech Stack:** Go, gopher-lua, Gorilla Mux, Playwright (E2E tests)

---

### Task 1: Add API Endpoint Types and Storage

**Files:**
- Create: `plugin_system/api_endpoints.go`
- Modify: `plugin_system/manager.go:76-113` (add `apiEndpoints` field)
- Modify: `plugin_system/manager.go:121-136` (initialize map in `NewPluginManager`)
- Modify: `plugin_system/manager.go:703-715` (cleanup in `DisablePlugin`)
- Modify: `plugin_system/manager.go:847-861` (nil out in `Close`)

**Step 1: Create `plugin_system/api_endpoints.go` with types**

```go
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

// APIEndpoint stores a registered JSON API handler and its parent VM.
type APIEndpoint struct {
	state   *lua.LState
	fn      *lua.LFunction
	timeout time.Duration
}

// APIResponse holds the result of executing an API endpoint handler.
type APIResponse struct {
	StatusCode int
	Body       any
	Error      string
}

// HandleAPI executes the Lua API handler for the given plugin, method, and path,
// passing the request context, and returns the API response.
func (pm *PluginManager) HandleAPI(pluginName, method, path string, ctx PageContext) APIResponse {
	if pm.closed.Load() {
		return APIResponse{StatusCode: 500, Error: "plugin manager is closed"}
	}

	method = strings.ToUpper(method)
	key := method + ":" + path

	pm.mu.RLock()
	endpoints, pluginExists := pm.apiEndpoints[pluginName]
	if !pluginExists {
		// Check if any path exists for this plugin to distinguish 404 from "plugin not found"
		pm.mu.RUnlock()
		return APIResponse{StatusCode: 404, Error: "plugin not found"}
	}

	endpoint, endpointExists := endpoints[key]
	if !endpointExists {
		// Check if path exists with a different method -> 405
		methodNotAllowed := false
		for k := range endpoints {
			parts := strings.SplitN(k, ":", 2)
			if len(parts) == 2 && parts[1] == path {
				methodNotAllowed = true
				break
			}
		}
		pm.mu.RUnlock()
		if methodNotAllowed {
			return APIResponse{StatusCode: 405, Error: "method not allowed"}
		}
		return APIResponse{StatusCode: 404, Error: "endpoint not found"}
	}
	pm.mu.RUnlock()

	L := endpoint.state
	mu := pm.VMLock(L)
	mu.Lock()
	defer mu.Unlock()

	// Build context table (same as HandlePage)
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

	// Inject ctx.json() and ctx.status() closures
	var response APIResponse
	response.StatusCode = 0 // sentinel: 0 means "not set"
	bodySet := false

	tbl.RawSetString("json", L.NewFunction(func(L *lua.LState) int {
		response.Body = luaValueToGoForJson(L.CheckAny(1))
		bodySet = true
		return 0
	}))

	tbl.RawSetString("status", L.NewFunction(func(L *lua.LState) int {
		response.StatusCode = L.CheckInt(1)
		return 0
	}))

	timeout := endpoint.timeout
	if timeout <= 0 {
		timeout = defaultAPITimeout
	}
	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      endpoint.fn,
		NRet:    0,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		errStr := err.Error()
		// Check for abort
		if strings.Contains(errStr, "PLUGIN_ABORT:") {
			reason := strings.TrimSpace(strings.SplitN(errStr, "PLUGIN_ABORT:", 2)[1])
			// Strip trailing quote if present from Lua error wrapping
			reason = strings.TrimSuffix(reason, "'")
			reason = strings.TrimSuffix(reason, "\"")
			log.Printf("[plugin] api handler %q %s %q aborted: %s", pluginName, method, path, reason)
			return APIResponse{StatusCode: 400, Error: reason}
		}
		// Check for context deadline exceeded (timeout)
		if strings.Contains(errStr, "context deadline exceeded") {
			log.Printf("[plugin] warning: api handler %q %s %q timed out", pluginName, method, path)
			return APIResponse{StatusCode: 504, Error: "handler timed out"}
		}
		log.Printf("[plugin] warning: api handler %q %s %q error: %v", pluginName, method, path, err)
		return APIResponse{StatusCode: 500, Error: "internal plugin error"}
	}

	// Determine final status code
	if !bodySet {
		if response.StatusCode == 0 {
			response.StatusCode = 204
		}
		response.Body = nil
	} else if response.StatusCode == 0 {
		response.StatusCode = 200
	}

	return response
}

// HasAPIEndpoint checks if a plugin has any API endpoint registered at the given path
// (regardless of method).
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
```

**Step 2: Add `apiEndpoints` field to `PluginManager` struct**

In `plugin_system/manager.go`, add to the struct at line ~83 (after `actions`):

```go
apiEndpoints map[string]map[string]*APIEndpoint // pluginName -> "METHOD:path" -> endpoint
```

**Step 3: Initialize map in `NewPluginManager`**

In `plugin_system/manager.go` around line 125, add:

```go
apiEndpoints: make(map[string]map[string]*APIEndpoint),
```

**Step 4: Add cleanup in `DisablePlugin`**

In `plugin_system/manager.go`, after `delete(pm.actions, name)` (line ~715), add:

```go
// Remove API endpoints for this plugin.
delete(pm.apiEndpoints, name)
```

**Step 5: Add cleanup in `Close`**

In `plugin_system/manager.go`, after `pm.actions = nil` (line ~859), add:

```go
pm.apiEndpoints = nil
```

**Step 6: Run Go unit tests to make sure nothing is broken**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/... -count=1 -v -short 2>&1 | tail -20`
Expected: All existing tests PASS.

**Step 7: Commit**

```
feat(plugins): add API endpoint types and HandleAPI execution
```

---

### Task 2: Register `mah.api()` Lua Binding

**Files:**
- Modify: `plugin_system/manager.go:405-464` (add `mah.api` registration after `mah.action`)

**Step 1: Add `mah.api()` registration in `registerMahModule`**

In `plugin_system/manager.go`, after the `mah.action` registration block (after line ~464), add:

```go
mahMod.RawSetString("api", L.NewFunction(func(L *lua.LState) int {
	method := strings.ToUpper(L.CheckString(1))
	path := L.CheckString(2)
	handler := L.CheckFunction(3)

	// Validate method
	switch method {
	case "GET", "POST", "PUT", "DELETE":
		// ok
	default:
		L.ArgError(1, "method must be GET, POST, PUT, or DELETE")
		return 0
	}

	// Validate path (same regex as mah.page)
	if !validPagePath.MatchString(path) {
		L.ArgError(2, "invalid api path: must contain only alphanumeric characters, hyphens, underscores, and slashes")
		return 0
	}

	// Parse optional opts table (4th argument)
	timeout := defaultAPITimeout
	if optsTbl := L.OptTable(4, nil); optsTbl != nil {
		if t, ok := optsTbl.RawGetString("timeout").(lua.LNumber); ok {
			d := time.Duration(float64(t)) * time.Second
			if d > maxAPITimeout {
				d = maxAPITimeout
			}
			if d > 0 {
				timeout = d
			}
		}
	}

	name := *pluginNamePtr
	key := method + ":" + path

	pm.mu.Lock()
	if pm.apiEndpoints[name] == nil {
		pm.apiEndpoints[name] = make(map[string]*APIEndpoint)
	}
	pm.apiEndpoints[name][key] = &APIEndpoint{
		state:   L,
		fn:      handler,
		timeout: timeout,
	}
	pm.mu.Unlock()
	return 0
}))
```

**Step 2: Add `strings` import if not already present**

Check that `"strings"` is in the import block of `manager.go`. It already is (line 10).

**Step 3: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/... -count=1 -v -short 2>&1 | tail -20`
Expected: All existing tests PASS.

**Step 4: Commit**

```
feat(plugins): register mah.api() Lua binding for JSON endpoints
```

---

### Task 3: Write Go Unit Tests for API Endpoints

**Files:**
- Create: `plugin_system/api_endpoints_test.go`

**Step 1: Write the test file**

```go
package plugin_system

import (
	"strings"
	"testing"
)

func TestAPIRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "api-test", `
plugin = { name = "api-test", version = "1.0", description = "api test" }

function init()
    mah.api("GET", "hello", function(ctx)
        ctx.json({ message = "hello" })
    end)
    mah.api("POST", "data", function(ctx)
        ctx.status(201)
        ctx.json({ created = true })
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("api-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if !pm.HasAPIEndpoint("api-test", "hello") {
		t.Error("expected HasAPIEndpoint('api-test', 'hello') to be true")
	}
	if !pm.HasAPIEndpoint("api-test", "data") {
		t.Error("expected HasAPIEndpoint('api-test', 'data') to be true")
	}
	if pm.HasAPIEndpoint("api-test", "nonexistent") {
		t.Error("expected HasAPIEndpoint('api-test', 'nonexistent') to be false")
	}
	if pm.HasAPIEndpoint("unknown", "hello") {
		t.Error("expected HasAPIEndpoint('unknown', 'hello') to be false")
	}
}

func TestAPIRegistration_InvalidMethod(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-method", `
plugin = { name = "bad-method", version = "1.0", description = "bad method" }

function init()
    mah.api("PATCH", "hello", function(ctx) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("bad-method")
	if err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestAPIRegistration_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad-path", `
plugin = { name = "bad-path", version = "1.0", description = "bad path" }

function init()
    mah.api("GET", "hello world", function(ctx) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("bad-path")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestAPIRegistration_DuplicateOverwrites(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dup", `
plugin = { name = "dup", version = "1.0", description = "dup" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ version = 1 })
    end)
    mah.api("GET", "data", function(ctx)
        ctx.json({ version = 2 })
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("dup"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("dup", "GET", "data", PageContext{
		Path: "/v1/plugins/dup/data", Method: "GET",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["version"] != float64(2) {
		t.Errorf("expected version 2 (overwritten), got %v", body["version"])
	}
}

func TestHandleAPI_JsonResponse(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "json-test", `
plugin = { name = "json-test", version = "1.0", description = "json test" }

function init()
    mah.api("GET", "info", function(ctx)
        ctx.json({ name = "test", count = 42 })
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("json-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("json-test", "GET", "info", PageContext{
		Path: "/v1/plugins/json-test/info", Method: "GET",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (error: %s)", resp.StatusCode, resp.Error)
	}
	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["name"] != "test" {
		t.Errorf("expected name 'test', got %v", body["name"])
	}
	if body["count"] != float64(42) {
		t.Errorf("expected count 42, got %v", body["count"])
	}
}

func TestHandleAPI_CustomStatus(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "status-test", `
plugin = { name = "status-test", version = "1.0", description = "status test" }

function init()
    mah.api("POST", "create", function(ctx)
        ctx.status(201)
        ctx.json({ created = true })
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("status-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("status-test", "POST", "create", PageContext{
		Path: "/v1/plugins/status-test/create", Method: "POST",
	})
	if resp.StatusCode != 201 {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestHandleAPI_NoBody204(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "no-body", `
plugin = { name = "no-body", version = "1.0", description = "no body" }

function init()
    mah.api("DELETE", "item", function(ctx)
        -- no ctx.json() call
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("no-body"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("no-body", "DELETE", "item", PageContext{
		Path: "/v1/plugins/no-body/item", Method: "DELETE",
	})
	if resp.StatusCode != 204 {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
	if resp.Body != nil {
		t.Errorf("expected nil body, got %v", resp.Body)
	}
}

func TestHandleAPI_NoBodyCustomStatus(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "custom-no-body", `
plugin = { name = "custom-no-body", version = "1.0", description = "custom no body" }

function init()
    mah.api("DELETE", "item", function(ctx)
        ctx.status(204)
        -- no ctx.json() call
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("custom-no-body"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("custom-no-body", "DELETE", "item", PageContext{
		Path: "/v1/plugins/custom-no-body/item", Method: "DELETE",
	})
	if resp.StatusCode != 204 {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
}

func TestHandleAPI_PluginNotFound(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	resp := pm.HandleAPI("nonexistent", "GET", "anything", PageContext{})
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	if resp.Error != "plugin not found" {
		t.Errorf("expected error 'plugin not found', got %q", resp.Error)
	}
}

func TestHandleAPI_EndpointNotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sparse", `
plugin = { name = "sparse", version = "1.0", description = "sparse" }

function init()
    mah.api("GET", "exists", function(ctx) ctx.json({}) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("sparse"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("sparse", "GET", "missing", PageContext{})
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	if resp.Error != "endpoint not found" {
		t.Errorf("expected error 'endpoint not found', got %q", resp.Error)
	}
}

func TestHandleAPI_MethodNotAllowed(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "methods", `
plugin = { name = "methods", version = "1.0", description = "methods" }

function init()
    mah.api("GET", "data", function(ctx) ctx.json({}) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("methods"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("methods", "POST", "data", PageContext{})
	if resp.StatusCode != 405 {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
	if resp.Error != "method not allowed" {
		t.Errorf("expected error 'method not allowed', got %q", resp.Error)
	}
}

func TestHandleAPI_HandlerError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "crashy", `
plugin = { name = "crashy", version = "1.0", description = "crashy" }

function init()
    mah.api("GET", "crash", function(ctx)
        error("intentional crash")
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("crashy"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("crashy", "GET", "crash", PageContext{})
	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
	if resp.Error != "internal plugin error" {
		t.Errorf("expected error 'internal plugin error', got %q", resp.Error)
	}
}

func TestHandleAPI_Abort(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "aborter", `
plugin = { name = "aborter", version = "1.0", description = "aborter" }

function init()
    mah.api("POST", "validate", function(ctx)
        mah.abort("name is required")
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("aborter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("aborter", "POST", "validate", PageContext{})
	if resp.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Error, "name is required") {
		t.Errorf("expected error to contain 'name is required', got %q", resp.Error)
	}
}

func TestHandleAPI_WithQueryParams(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "query", `
plugin = { name = "query", version = "1.0", description = "query" }

function init()
    mah.api("GET", "search", function(ctx)
        ctx.json({ q = ctx.query.q, method = ctx.method })
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("query"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("query", "GET", "search", PageContext{
		Path:   "/v1/plugins/query/search?q=test",
		Method: "GET",
		Query:  map[string]any{"q": "test"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (error: %s)", resp.StatusCode, resp.Error)
	}
	body := resp.Body.(map[string]any)
	if body["q"] != "test" {
		t.Errorf("expected q='test', got %v", body["q"])
	}
	if body["method"] != "GET" {
		t.Errorf("expected method='GET', got %v", body["method"])
	}
}

func TestHandleAPI_TimeoutClamped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "clamp", `
plugin = { name = "clamp", version = "1.0", description = "clamp" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ ok = true })
    end, { timeout = 999 })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("clamp"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Verify it's stored (clamped) and callable
	pm.mu.RLock()
	ep := pm.apiEndpoints["clamp"]["GET:data"]
	pm.mu.RUnlock()
	if ep.timeout != maxAPITimeout {
		t.Errorf("expected timeout clamped to %v, got %v", maxAPITimeout, ep.timeout)
	}
}

func TestHandleAPI_DisabledPluginCleansUp(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ephemeral", `
plugin = { name = "ephemeral", version = "1.0", description = "ephemeral" }

function init()
    mah.api("GET", "data", function(ctx) ctx.json({}) end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("ephemeral"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if !pm.HasAPIEndpoint("ephemeral", "data") {
		t.Fatal("expected endpoint to exist after enable")
	}

	if err := pm.DisablePlugin("ephemeral"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	if pm.HasAPIEndpoint("ephemeral", "data") {
		t.Error("expected endpoint to be removed after disable")
	}

	resp := pm.HandleAPI("ephemeral", "GET", "data", PageContext{})
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 after disable, got %d", resp.StatusCode)
	}
}
```

**Step 2: Run the tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./plugin_system/... -run TestAPI -count=1 -v 2>&1 | tail -40`
Expected: All tests PASS.

**Step 3: Commit**

```
test(plugins): add unit tests for API endpoint registration and execution
```

---

### Task 4: Add HTTP Handler and Route

**Files:**
- Modify: `server/api_handlers/plugin_api_handlers.go` (add `PluginAPIHandler`)
- Modify: `server/routes.go:362-381` (add route before `/v1/plugins/manage`)

**Step 1: Add `PluginAPIHandler` to `plugin_api_handlers.go`**

Add at the end of the file:

```go
// PluginAPIHandler handles JSON API requests to plugin-registered endpoints.
// Routes: GET/POST/PUT/DELETE /v1/plugins/{pluginName}/{path...}
func PluginAPIHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "plugin system not available"})
			return
		}

		// Parse /v1/plugins/{pluginName}/{path...}
		trimmed := strings.TrimPrefix(r.URL.Path, "/v1/plugins/")
		parts := strings.SplitN(trimmed, "/", 2)
		pluginName := ""
		apiPath := ""
		if len(parts) >= 1 {
			pluginName = parts[0]
		}
		if len(parts) >= 2 {
			apiPath = parts[1]
		}

		if pluginName == "" || pluginName == "manage" {
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "plugin not found"})
			return
		}

		// Build PageContext (reuse the same pattern as plugin_page_context.go)
		queryMap := make(map[string]any)
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				queryMap[k] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				queryMap[k] = items
			}
		}

		headerMap := make(map[string]any)
		for k, v := range r.Header {
			if len(v) == 1 {
				headerMap[strings.ToLower(k)] = v[0]
			} else {
				items := make([]any, len(v))
				for i, val := range v {
					items[i] = val
				}
				headerMap[strings.ToLower(k)] = items
			}
		}

		var body string
		paramsMap := make(map[string]any)
		if r.Body != nil {
			const maxBodySize = 50 << 20 // 50MB
			limited := io.LimitReader(r.Body, maxBodySize)
			bodyBytes, err := io.ReadAll(limited)
			if err == nil {
				body = string(bodyBytes)
			}
		}

		pageCtx := plugin_system.PageContext{
			Path:    r.URL.String(),
			Method:  r.Method,
			Query:   queryMap,
			Params:  paramsMap,
			Headers: headerMap,
			Body:    body,
		}

		resp := pm.HandleAPI(pluginName, r.Method, apiPath, pageCtx)

		w.Header().Set("Content-Type", constants.JSON)
		if resp.Error != "" {
			w.WriteHeader(resp.StatusCode)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": resp.Error})
			return
		}

		w.WriteHeader(resp.StatusCode)
		if resp.Body != nil {
			_ = json.NewEncoder(w).Encode(resp.Body)
		}
	}
}
```

Also add `"io"` to the imports at the top of the file if not already present.

**Step 2: Add route in `routes.go`**

In `server/routes.go`, BEFORE the `/v1/plugins/manage` route (line ~363), add:

```go
// Plugin JSON API endpoints (must be registered before /v1/plugins/manage to avoid prefix conflicts)
if appContext.PluginManager() != nil {
	router.PathPrefix("/v1/plugins/").HandlerFunc(api_handlers.PluginAPIHandler(appContext))
}
```

**IMPORTANT**: The `/v1/plugins/manage` route at line 363 uses `.Path()` (exact match), so it will take priority over `.PathPrefix()` in Gorilla Mux. But to be safe, register the catch-all AFTER the exact `/v1/plugins/manage` route. So add it at line ~368, after the `purge-data` route.

Actually, looking at the Gorilla Mux behavior: `.Path()` exact matches take priority over `.PathPrefix()` matches. So place the new `PathPrefix` route after all the existing exact-path `/v1/plugin/*` routes (after line ~367):

```go
// Plugin JSON API endpoints (catch-all for /v1/plugins/{name}/{path})
if appContext.PluginManager() != nil {
	router.PathPrefix("/v1/plugins/").HandlerFunc(api_handlers.PluginAPIHandler(appContext))
}
```

**Step 3: Build the application**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Build succeeds with no errors.

**Step 4: Run all Go tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./... -count=1 -short 2>&1 | tail -20`
Expected: All tests PASS.

**Step 5: Commit**

```
feat(plugins): add HTTP handler and route for plugin JSON API endpoints
```

---

### Task 5: Create E2E Test Plugin and Tests

**Files:**
- Create: `e2e/test-plugins/test-api/plugin.lua`
- Create: `e2e/tests/plugins/plugin-api.spec.ts`

**Step 1: Create the test plugin**

```lua
plugin = {
    name = "test-api",
    version = "1.0",
    description = "E2E test plugin for JSON API endpoints"
}

function init()
    -- GET endpoint that echoes query params and method
    mah.api("GET", "echo", function(ctx)
        ctx.json({ query = ctx.query, method = ctx.method, path = ctx.path })
    end)

    -- POST endpoint that echoes parsed body
    mah.api("POST", "echo", function(ctx)
        local body = mah.json.decode(ctx.body)
        ctx.status(201)
        ctx.json({ received = body })
    end)

    -- PUT endpoint
    mah.api("PUT", "echo", function(ctx)
        local body = mah.json.decode(ctx.body)
        ctx.json({ updated = body })
    end)

    -- DELETE endpoint with no body
    mah.api("DELETE", "echo", function(ctx)
        ctx.status(204)
    end)

    -- Endpoint that uses KV store
    mah.api("POST", "store", function(ctx)
        local body = mah.json.decode(ctx.body)
        mah.kv.set("api_data", body)
        ctx.status(201)
        ctx.json({ stored = true })
    end)

    mah.api("GET", "store", function(ctx)
        local data = mah.kv.get("api_data")
        if data then
            ctx.json(data)
        else
            ctx.status(404)
            ctx.json({ error = "no data" })
        end
    end)

    -- Endpoint that calls mah.abort
    mah.api("POST", "validate", function(ctx)
        mah.abort("validation failed")
    end)

    -- Endpoint that errors
    mah.api("GET", "crash", function(ctx)
        error("intentional crash")
    end)

    -- Nested path
    mah.api("GET", "nested/deep/path", function(ctx)
        ctx.json({ nested = true })
    end)
end
```

**Step 2: Create the E2E test spec**

```typescript
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin JSON API Endpoints', () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.enablePlugin('test-api');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-api');
    } catch {
      // Ignore if already disabled
    }
  });

  test('GET endpoint echoes query params', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/echo?msg=hello&count=5`);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(body.method).toBe('GET');
    expect(body.query.msg).toBe('hello');
    expect(body.query.count).toBe('5');
  });

  test('POST endpoint returns 201 with parsed body', async ({ request, baseURL }) => {
    const response = await request.post(`${baseURL}/v1/plugins/test-api/echo`, {
      data: { name: 'test', value: 42 },
    });
    expect(response.status()).toBe(201);
    const body = await response.json();
    expect(body.received.name).toBe('test');
    expect(body.received.value).toBe(42);
  });

  test('PUT endpoint returns 200 with parsed body', async ({ request, baseURL }) => {
    const response = await request.put(`${baseURL}/v1/plugins/test-api/echo`, {
      data: { updated: true },
    });
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.updated.updated).toBe(true);
  });

  test('DELETE endpoint returns 204 with no body', async ({ request, baseURL }) => {
    const response = await request.delete(`${baseURL}/v1/plugins/test-api/echo`);
    expect(response.status()).toBe(204);
    const text = await response.text();
    expect(text).toBe('');
  });

  test('wrong method returns 405', async ({ request, baseURL }) => {
    // PATCH is not registered
    const response = await request.patch(`${baseURL}/v1/plugins/test-api/echo`, {
      data: {},
    });
    expect(response.status()).toBe(405);
    const body = await response.json();
    expect(body.error).toBe('method not allowed');
  });

  test('nonexistent path returns 404', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/nonexistent`);
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toBe('endpoint not found');
  });

  test('nonexistent plugin returns 404', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/no-such-plugin/anything`);
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toBe('plugin not found');
  });

  test('mah.abort returns 400 with reason', async ({ request, baseURL }) => {
    const response = await request.post(`${baseURL}/v1/plugins/test-api/validate`, {
      data: {},
    });
    expect(response.status()).toBe(400);
    const body = await response.json();
    expect(body.error).toContain('validation failed');
  });

  test('handler error returns 500', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/crash`);
    expect(response.status()).toBe(500);
    const body = await response.json();
    expect(body.error).toBe('internal plugin error');
  });

  test('nested path works', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/nested/deep/path`);
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.nested).toBe(true);
  });

  test('KV store integration works across endpoints', async ({ request, baseURL }) => {
    // Store data
    const storeResp = await request.post(`${baseURL}/v1/plugins/test-api/store`, {
      data: { key: 'value', number: 123 },
    });
    expect(storeResp.status()).toBe(201);

    // Read it back
    const readResp = await request.get(`${baseURL}/v1/plugins/test-api/store`);
    expect(readResp.status()).toBe(200);
    const body = await readResp.json();
    expect(body.key).toBe('value');
    expect(body.number).toBe(123);
  });

  test('disabled plugin returns 404', async ({ apiClient, request, baseURL }) => {
    await apiClient.disablePlugin('test-api');
    const response = await request.get(`${baseURL}/v1/plugins/test-api/echo`);
    expect(response.status()).toBe(404);
  });
});
```

**Step 3: Build and run E2E tests**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server -- --grep "Plugin JSON API"`
Expected: All E2E tests PASS.

**Step 4: Commit**

```
test(plugins): add E2E test plugin and Playwright tests for JSON API endpoints
```

---

### Task 6: Update Example Plugin

**Files:**
- Modify: `plugins/example-plugin/plugin.lua` (add commented `mah.api()` example)

**Step 1: Add example at the end of the `init()` function**

Add before the final `end` of `init()` in `plugins/example-plugin/plugin.lua`:

```lua
    -- =========================================================================
    -- JSON API Endpoint Example (mah.api)
    -- =========================================================================
    -- Register a JSON API endpoint at /v1/plugins/example-plugin/status
    -- mah.api("GET", "status", function(ctx)
    --     local notes = mah.db.query_notes({ limit = 0 })
    --     local resources = mah.db.query_resources({ limit = 0 })
    --     ctx.json({
    --         plugin = "example-plugin",
    --         notes = #notes,
    --         resources = #resources,
    --         greeting = mah.get_setting("greeting")
    --     })
    -- end)

    -- POST endpoint with custom status and body parsing
    -- mah.api("POST", "webhook", function(ctx)
    --     local payload = mah.json.decode(ctx.body)
    --     mah.kv.set("last_webhook", payload)
    --     mah.log("info", "Webhook received", payload)
    --     ctx.status(201)
    --     ctx.json({ received = true })
    -- end, { timeout = 60 })
```

**Step 2: Run Go tests to verify nothing broke**

Run: `cd /Users/egecan/Code/mahresources && go test ./... -count=1 -short 2>&1 | tail -10`
Expected: PASS.

**Step 3: Commit**

```
docs(plugins): add mah.api() examples to example plugin
```

---

### Task 7: Update Documentation

**Files:**
- Modify: `docs-site/docs/features/plugin-system.md`
- Modify: `docs-site/docs/features/plugin-lua-api.md`
- Modify: `docs-site/docs/features/plugin-hooks.md`
- Modify: `docs-site/docs/api/plugins.md`

**Step 1: Update `plugin-system.md`**

In the Plugin Lifecycle section (line ~52-56), update step 3 to mention API endpoints:

Change "Hooks, actions, injections, pages, and menus registered during `init()` become active." to "Hooks, actions, injections, pages, menus, and API endpoints registered during `init()` become active."

In step 5, change "All hooks, injections, pages, menus, and actions are removed." to "All hooks, injections, pages, menus, actions, and API endpoints are removed."

Also add in the overview (line ~8), change to: "Lua-based plugins extend Mahresources with custom actions, hooks, pages, JSON API endpoints, and menu items."

**Step 2: Update `plugin-lua-api.md`**

Add a new section before `mah.get_setting` (before line ~544). Insert:

```markdown
## mah.api -- JSON API Endpoints

Register custom JSON API endpoints accessible at `/v1/plugins/{pluginName}/{path}`.

### mah.api(method, path, handler, [opts])

| Parameter | Type | Description |
|-----------|------|-------------|
| `method` | string | HTTP method: `"GET"`, `"POST"`, `"PUT"`, or `"DELETE"` |
| `path` | string | Endpoint path (alphanumeric, hyphens, underscores, slashes) |
| `handler` | function | Receives a context table with request data and response helpers |
| `opts` | table | Optional. `{ timeout = 30 }` -- seconds (default 30, max 120) |

### Handler Context

The handler receives a single `ctx` table:

| Field | Type | Description |
|-------|------|-------------|
| `ctx.path` | string | Full request URL path |
| `ctx.method` | string | HTTP method |
| `ctx.query` | table | URL query parameters |
| `ctx.params` | table | Form-decoded parameters (empty for non-form requests) |
| `ctx.headers` | table | Request headers (lowercase keys) |
| `ctx.body` | string | Raw request body |
| `ctx.json(data)` | function | Set the JSON response body |
| `ctx.status(code)` | function | Set the HTTP status code (default: 200) |

### Response Behavior

| Scenario | Status | Body |
|----------|--------|------|
| `ctx.json()` called | 200 (or custom via `ctx.status()`) | JSON-encoded data |
| `ctx.json()` not called | 204 No Content | Empty |
| Handler error | 500 | `{"error": "internal plugin error"}` |
| Handler timeout | 504 | `{"error": "handler timed out"}` |
| `mah.abort()` called | 400 | `{"error": "reason"}` |
| Path not found | 404 | `{"error": "endpoint not found"}` |
| Wrong HTTP method | 405 | `{"error": "method not allowed"}` |

### Example

```lua
function init()
    -- GET endpoint returning JSON
    mah.api("GET", "stats", function(ctx)
        local notes = mah.db.query_notes({ limit = 0 })
        ctx.json({ total_notes = #notes, query = ctx.query })
    end)

    -- POST endpoint with custom status
    mah.api("POST", "webhook", function(ctx)
        local payload = mah.json.decode(ctx.body)
        mah.kv.set("last_webhook", payload)
        ctx.status(201)
        ctx.json({ received = true })
    end, { timeout = 60 })

    -- DELETE with no body
    mah.api("DELETE", "cache", function(ctx)
        mah.kv.delete("cached_data")
        ctx.status(204)
    end)
end
```

Duplicate registrations for the same method + path overwrite the previous handler.
```

**Step 3: Update `plugin-hooks.md`**

At the bottom of the file, add to "Related Pages" or add a new "See Also" line:

```markdown
- [Plugin Lua API Reference](./plugin-lua-api.md) -- includes `mah.api()` for JSON API endpoints
```

**Step 4: Update `api/plugins.md`**

After the "Plugin Pages" section (after line ~241), add a new section:

```markdown
## Plugin JSON API Endpoints

```
GET|POST|PUT|DELETE /v1/plugins/{pluginName}/{path}
```

Plugin-registered JSON API endpoints. Unlike plugin pages (which return HTML), these return `application/json` responses.

```bash
# GET endpoint
curl http://localhost:8181/v1/plugins/my-plugin/stats

# POST with JSON body
curl -X POST http://localhost:8181/v1/plugins/my-plugin/webhook \
  -H "Content-Type: application/json" \
  -d '{"event": "test"}'
```

**Success Response:**

```json
{
  "total_notes": 42,
  "query": { "page": "1" }
}
```

**Error Responses:**

| Status | Condition | Body |
|--------|-----------|------|
| 400 | Handler called `mah.abort()` | `{"error": "reason"}` |
| 404 | Plugin not found or path not registered | `{"error": "plugin not found"}` or `{"error": "endpoint not found"}` |
| 405 | Path exists but method not registered | `{"error": "method not allowed"}` |
| 500 | Handler runtime error | `{"error": "internal plugin error"}` |
| 504 | Handler exceeded timeout | `{"error": "handler timed out"}` |

See [Plugin Lua API Reference](../features/plugin-lua-api.md) for the `mah.api()` registration function.
```

**Step 5: Commit**

```
docs: add mah.api() JSON API endpoints to plugin documentation
```

---

### Task 8: Final Verification

**Step 1: Run all Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./... -count=1 2>&1 | tail -20`
Expected: All tests PASS.

**Step 2: Build the full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds.

**Step 3: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All tests PASS (including existing tests -- no regressions).

**Step 4: Manual smoke test**

Run: `cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8181`

Then in another terminal:
```bash
# Should return 404 (no plugins enabled)
curl -s http://localhost:8181/v1/plugins/test-api/echo | jq .

# Plugin manage page should still work
curl -s http://localhost:8181/v1/plugins/manage | jq .
```

Expected: `{"error":"plugin not found"}` for the first, array of plugins for the second.

**Step 5: Commit final state if any fixes were needed**

```
fix(plugins): address issues found during final verification
```
