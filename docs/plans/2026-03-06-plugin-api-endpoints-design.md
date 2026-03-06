# Plugin JSON API Endpoints (`mah.api()`)

## Summary

Add `mah.api(method, path, handler, [opts])` to the plugin system, enabling plugins to register JSON API endpoints at `/v1/plugins/{pluginName}/{path}`. This extends the existing page system pattern but returns JSON instead of rendering templates.

## Plugin Author API

### Registration

```lua
mah.api("GET", "stats", function(ctx)
    local count = mah.db.query_resources({limit = 0})
    ctx.json({ total = #count })
end)

mah.api("POST", "webhook/receive", function(ctx)
    local payload = mah.json.decode(ctx.body)
    mah.kv.set("last_webhook", payload)
    ctx.status(201)
    ctx.json({ received = true })
end, { timeout = 60 })
```

**Signature**: `mah.api(method, path, handler, [opts])`

| Parameter | Type | Description |
|-----------|------|-------------|
| `method` | string | `GET`, `POST`, `PUT`, `DELETE` |
| `path` | string | Endpoint path (same validation as `mah.page`: alphanumeric, hyphens, underscores, slashes) |
| `handler` | function | Receives `ctx` table |
| `opts` | table (optional) | `{ timeout = 30 }` — seconds, default 30, max 120 |

### Handler Context

Same fields as the existing page context, plus response helpers:

```lua
function(ctx)
    -- Request data (read-only, same as mah.page):
    ctx.path        -- "/stats"
    ctx.method      -- "GET"
    ctx.query       -- { page = "2", q = "search" }
    ctx.params      -- {} (form-decoded for POST, empty for GET)
    ctx.headers     -- { ["content-type"] = "application/json", ... }
    ctx.body        -- raw request body string

    -- Response helpers:
    ctx.json(data)      -- set response body (Lua table/value -> JSON)
    ctx.status(code)    -- set HTTP status code (default: 200)
end
```

### Response Behavior

- If `ctx.json()` is never called: `204 No Content`
- If `ctx.json()` is called: JSON-encoded value with `Content-Type: application/json`
- If `ctx.status()` is never called: `200` (or `204` if no body)
- Handler error/timeout: `500` / `504` with `{"error": "..."}`
- Duplicate `method + path` registration: second overwrites first

## Routing & HTTP Layer

### URL Structure

```
/v1/plugins/{pluginName}/{path...}
```

Examples:
- `GET /v1/plugins/my-weather/forecast`
- `POST /v1/plugins/github-sync/webhook/receive`
- `DELETE /v1/plugins/bookmarks/items`

### Route Registration

Single catch-all in `server/routes.go`:

```go
router.PathPrefix("/v1/plugins/").HandlerFunc(pluginAPIHandler(appContext))
```

### Method Matching

Exact match. Wrong method on existing path returns `405 Method Not Allowed`.

### Path Matching

Static only — no wildcards or path parameters. Plugins parse dynamic segments from `ctx.path` or `ctx.query` themselves.

### Error Responses

All errors return JSON:

| Scenario | Status | Body |
|----------|--------|------|
| Plugin not found/disabled | 404 | `{"error": "plugin not found"}` |
| Path not registered | 404 | `{"error": "endpoint not found"}` |
| Wrong HTTP method | 405 | `{"error": "method not allowed"}` |
| Handler timeout | 504 | `{"error": "handler timed out"}` |
| Handler runtime error | 500 | `{"error": "internal plugin error"}` |
| `mah.abort()` called | 400 | `{"error": "reason from abort"}` |

No CORS handling (private network assumption, same as all other endpoints).

## Go Implementation

### New File: `plugin_system/api_endpoints.go`

```go
type APIEndpoint struct {
    Method   string
    Path     string
    Handler  *lua.LFunction
    Timeout  time.Duration // default 30s, max 120s
}

type APIResponse struct {
    StatusCode int
    Body       any
    Error      string
}
```

Storage on `PluginManager`:

```go
apiEndpoints map[string]map[string]*APIEndpoint  // plugin -> "METHOD:path" -> endpoint
```

### Registration: `registerAPIEndpoint`

Called from Lua `mah.api()` binding:
1. Validate method is GET/POST/PUT/DELETE
2. Validate path with existing `validPagePath` regex
3. Parse optional timeout (default 30s, clamp to 120s)
4. Store in `apiEndpoints[pluginName]["METHOD:path"]`

### Execution: `HandleAPI`

```go
func (pm *PluginManager) HandleAPI(pluginName, method, path string, ctx PageContext) APIResponse
```

1. Look up endpoint by plugin + `"METHOD:path"`
2. Path exists but method doesn't: 405
3. Nothing matches: 404
4. Acquire VM lock
5. Build Lua context table (reuse `goToLuaTable`)
6. Inject `json` and `status` Go closures into context table
7. Call handler with configured timeout
8. Return `APIResponse`

### `ctx.json()` / `ctx.status()` — Go closures injected into Lua context

```go
ctx.RawSetString("json", L.NewFunction(func(L *lua.LState) int {
    response.Body = luaValueToGoForJson(L.CheckAny(1))
    return 0
}))

ctx.RawSetString("status", L.NewFunction(func(L *lua.LState) int {
    response.StatusCode = L.CheckInt(1)
    return 0
}))
```

### HTTP Handler

In `server/api_handlers/plugin_api_handlers.go` (or new file):

```go
func PluginAPIHandler(appContext interfaces.AppContext) http.HandlerFunc
```

1. Parse pluginName and path from URL
2. Build PageContext (reuse existing builder)
3. Call `pm.HandleAPI()`
4. Write JSON response

### Cleanup

`DisablePlugin(name)` adds `delete(pm.apiEndpoints, name)` alongside existing cleanup.

## Testing

### Go Unit Tests

- Register valid endpoint, verify stored
- Reject invalid method/path
- Duplicate registration overwrites
- `ctx.json()` sets body correctly
- `ctx.status(201)` sets status
- No `ctx.json()` call returns 204
- Handler timeout returns 504
- Handler error returns 500
- `mah.abort()` returns 400
- Wrong method returns 405
- Disabled plugin returns 404
- Timeout clamped to 120s

### E2E Tests

New test plugin `e2e/test-plugins/test-api/plugin.lua`:

```lua
mah.api("GET", "echo", function(ctx)
    ctx.json({ query = ctx.query, method = ctx.method })
end)

mah.api("POST", "echo", function(ctx)
    local body = mah.json.decode(ctx.body)
    ctx.status(201)
    ctx.json({ received = body })
end)

mah.api("DELETE", "echo", function(ctx)
    ctx.status(204)
end)

mah.api("GET", "slow", function(ctx)
    -- triggers timeout
end, { timeout = 1 })
```

Playwright tests: GET echo, POST echo with body, DELETE 204, wrong method 405, disabled plugin 404, timeout 504.

### Example Plugin

Update `plugins/example-plugin/plugin.lua` with commented `mah.api()` example.

## Documentation Updates

| File | Change |
|------|--------|
| `docs-site/docs/features/plugin-system.md` | Add `mah.api()` to capabilities overview |
| `docs-site/docs/features/plugin-lua-api.md` | New section: signature, context, response behavior, timeout, example |
| `docs-site/docs/features/plugin-actions.md` | "See also" link to `mah.api()` |
| `docs-site/docs/api/plugins.md` | Add `/v1/plugins/{name}/{path}` routes, methods, error responses |

## Design Decisions

- **JSON only** — no raw/binary responses, keeps things consistent under `/v1/`
- **No auth** — same private network trust model as all other endpoints
- **Static paths only** — avoids building a router in Lua; plugins parse dynamic segments themselves
- **Reuse PageContext** — same proven structure, minimal new code
- **Configurable timeout** — 30s default, 120s max, per-endpoint
- **No restrictions** — GET/POST/PUT/DELETE, no rate limiting
