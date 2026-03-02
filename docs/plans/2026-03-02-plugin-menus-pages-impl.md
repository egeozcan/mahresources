# Plugin Menu Items and Pages — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow Lua plugins to register custom pages (`mah.page`) and navigation menu items (`mah.menu`), rendered within the standard base layout.

**Architecture:** Add page and menu registration to `PluginManager` during `init()`. A single wildcard route `/plugins/{pluginName}/{path:.*}` dispatches to Lua handlers. Menu items are injected into the template context and rendered as a conditional "Plugins" dropdown in the nav bar.

**Tech Stack:** Go (gopher-lua), Gorilla Mux, Pongo2 templates, Playwright (E2E)

---

### Task 1: Add Page and Menu Data Structures to PluginManager

**Files:**
- Modify: `plugin_system/manager.go`

**Step 1: Write the failing test**

Create test file `plugin_system/pages_test.go`:

```go
package plugin_system

import (
	"testing"
)

func TestPageRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dashboard", `
plugin = { name = "dashboard", version = "1.0", description = "dashboard plugin" }

function init()
    mah.page("home", function(ctx)
        return "<h1>Home</h1>"
    end)
    mah.page("stats", function(ctx)
        return "<h1>Stats</h1>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	pages := pm.GetPages()
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	// Verify lookup works
	if !pm.HasPage("dashboard", "home") {
		t.Error("expected HasPage('dashboard', 'home') to be true")
	}
	if !pm.HasPage("dashboard", "stats") {
		t.Error("expected HasPage('dashboard', 'stats') to be true")
	}
	if pm.HasPage("dashboard", "nonexistent") {
		t.Error("expected HasPage('dashboard', 'nonexistent') to be false")
	}
	if pm.HasPage("unknown", "home") {
		t.Error("expected HasPage('unknown', 'home') to be false")
	}
}

func TestMenuRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "analytics", `
plugin = { name = "analytics", version = "1.0", description = "analytics plugin" }

function init()
    mah.page("dashboard", function(ctx) return "<h1>Dashboard</h1>" end)
    mah.menu("Dashboard", "dashboard")
    mah.menu("Reports", "reports")
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	items := pm.GetMenuItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 menu items, got %d", len(items))
	}

	if items[0].Label != "Dashboard" {
		t.Errorf("expected label 'Dashboard', got %q", items[0].Label)
	}
	if items[0].FullPath != "/plugins/analytics/dashboard" {
		t.Errorf("expected path '/plugins/analytics/dashboard', got %q", items[0].FullPath)
	}
	if items[0].PluginName != "analytics" {
		t.Errorf("expected plugin name 'analytics', got %q", items[0].PluginName)
	}
	if items[1].Label != "Reports" {
		t.Errorf("expected label 'Reports', got %q", items[1].Label)
	}
	if items[1].FullPath != "/plugins/analytics/reports" {
		t.Errorf("expected path '/plugins/analytics/reports', got %q", items[1].FullPath)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./plugin_system/ -run 'TestPageRegistration|TestMenuRegistration' -v --tags 'json1 fts5'`
Expected: FAIL — `pm.GetPages`, `pm.HasPage`, `pm.GetMenuItems`, `MenuRegistration` undefined

**Step 3: Write minimal implementation**

In `plugin_system/manager.go`, add:

1. New types after the existing `injectionEntry` struct:

```go
// pageEntry stores a Lua page handler and its parent VM.
type pageEntry struct {
	state *lua.LState
	fn    *lua.LFunction
}

// MenuRegistration represents a plugin-contributed menu item.
type MenuRegistration struct {
	PluginName string
	Label      string
	FullPath   string
}
```

2. New fields on `PluginManager` struct (add alongside existing `hooks` and `injections`):

```go
pages     map[string]map[string]pageEntry // pluginName -> path -> handler
menuItems []MenuRegistration
```

3. Initialize the new map in `NewPluginManager` (alongside existing map inits):

```go
pages: make(map[string]map[string]pageEntry),
```

4. Track current plugin name during loading. Add a field to PluginManager:

```go
currentPluginName string // set during loadPlugin, used by mah.page/mah.menu
```

Set `pm.currentPluginName = info.Name` right after reading plugin metadata (after the `if tbl, ok := pluginTable.(*lua.LTable)` block) but *before* calling `init()`.

5. Register `mah.page` and `mah.menu` in `registerMahModule`:

```go
mahMod.RawSetString("page", L.NewFunction(func(L *lua.LState) int {
    path := L.CheckString(1)
    handler := L.CheckFunction(2)

    pluginName := pm.currentPluginName
    pm.mu.Lock()
    if pm.pages[pluginName] == nil {
        pm.pages[pluginName] = make(map[string]pageEntry)
    }
    pm.pages[pluginName][path] = pageEntry{state: L, fn: handler}
    pm.mu.Unlock()
    return 0
}))

mahMod.RawSetString("menu", L.NewFunction(func(L *lua.LState) int {
    label := L.CheckString(1)
    path := L.CheckString(2)

    pluginName := pm.currentPluginName
    fullPath := "/plugins/" + pluginName + "/" + path

    pm.mu.Lock()
    pm.menuItems = append(pm.menuItems, MenuRegistration{
        PluginName: pluginName,
        Label:      label,
        FullPath:   fullPath,
    })
    pm.mu.Unlock()
    return 0
}))
```

6. Add accessor methods:

```go
// GetPages returns a flat list of all registered page paths (for diagnostics).
func (pm *PluginManager) GetPages() []struct{ PluginName, Path string } {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    var result []struct{ PluginName, Path string }
    for pluginName, pages := range pm.pages {
        for path := range pages {
            result = append(result, struct{ PluginName, Path string }{pluginName, path})
        }
    }
    return result
}

// HasPage checks if a plugin has registered a page at the given path.
func (pm *PluginManager) HasPage(pluginName, path string) bool {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    if pages, ok := pm.pages[pluginName]; ok {
        _, exists := pages[path]
        return exists
    }
    return false
}

// GetMenuItems returns a copy of all registered menu items.
func (pm *PluginManager) GetMenuItems() []MenuRegistration {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    result := make([]MenuRegistration, len(pm.menuItems))
    copy(result, pm.menuItems)
    return result
}
```

7. Nil out `pages` and `menuItems` in `Close()`:

```go
pm.pages = nil
pm.menuItems = nil
```

**Step 4: Run test to verify it passes**

Run: `go test ./plugin_system/ -run 'TestPageRegistration|TestMenuRegistration' -v --tags 'json1 fts5'`
Expected: PASS

**Step 5: Run all existing plugin tests to ensure no regressions**

Run: `go test ./plugin_system/... -v --tags 'json1 fts5'`
Expected: All PASS

**Step 6: Commit**

```bash
git add plugin_system/manager.go plugin_system/pages_test.go
git commit -m "feat(plugins): add page and menu registration to PluginManager"
```

---

### Task 2: Add HandlePage Method

**Files:**
- Create: `plugin_system/pages.go`
- Modify: `plugin_system/pages_test.go`

**Step 1: Write the failing test**

Append to `plugin_system/pages_test.go`:

```go
func TestHandlePage_Success(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "myapp", `
plugin = { name = "myapp", version = "1.0", description = "test app" }

function init()
    mah.page("hello", function(ctx)
        return "<h1>Hello from " .. ctx.method .. " " .. ctx.path .. "</h1>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	html, err := pm.HandlePage("myapp", "hello", PageContext{
		Path:   "/plugins/myapp/hello",
		Method: "GET",
		Query:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "<h1>Hello from GET /plugins/myapp/hello</h1>"
	if html != expected {
		t.Errorf("expected %q, got %q", expected, html)
	}
}

func TestHandlePage_WithQueryParams(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "search", `
plugin = { name = "search", version = "1.0", description = "search" }

function init()
    mah.page("results", function(ctx)
        return "<p>Query: " .. (ctx.query.q or "none") .. "</p>"
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	html, err := pm.HandlePage("search", "results", PageContext{
		Path:   "/plugins/search/results",
		Method: "GET",
		Query:  map[string]any{"q": "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if html != "<p>Query: test</p>" {
		t.Errorf("expected '<p>Query: test</p>', got %q", html)
	}
}

func TestHandlePage_NotFound(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	_, err = pm.HandlePage("nonexistent", "page", PageContext{})
	if err == nil {
		t.Fatal("expected error for nonexistent plugin page")
	}
}

func TestHandlePage_LuaError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "broken", `
plugin = { name = "broken", version = "1.0", description = "broken" }

function init()
    mah.page("crash", function(ctx)
        error("intentional crash")
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	_, err = pm.HandlePage("broken", "crash", PageContext{})
	if err == nil {
		t.Fatal("expected error from crashing handler")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./plugin_system/ -run 'TestHandlePage' -v --tags 'json1 fts5'`
Expected: FAIL — `PageContext` and `HandlePage` undefined

**Step 3: Write implementation**

Create `plugin_system/pages.go`:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./plugin_system/ -run 'TestHandlePage' -v --tags 'json1 fts5'`
Expected: PASS

**Step 5: Run all plugin tests**

Run: `go test ./plugin_system/... -v --tags 'json1 fts5'`
Expected: All PASS

**Step 6: Commit**

```bash
git add plugin_system/pages.go plugin_system/pages_test.go
git commit -m "feat(plugins): add HandlePage method for executing plugin page handlers"
```

---

### Task 3: Add Plugin Page Template

**Files:**
- Create: `templates/pluginPage.tpl`

**Step 1: Create the template**

Create `templates/pluginPage.tpl`:

```django
{% extends "/layouts/base.tpl" %}
{% block head %}
    <title>{{ pluginPageTitle }} - mahresources</title>
{% endblock %}
{% block body %}
    {% if pluginError %}
    <div class="bg-red-50 border border-red-200 rounded p-4 mb-4" role="alert">
        <h2 class="text-red-800 font-semibold mb-1">Plugin Error</h2>
        <p class="text-red-700 text-sm">{{ pluginError }}</p>
    </div>
    {% else %}
    {{ pluginContent }}
    {% endif %}
{% endblock %}
```

**Step 2: Commit**

```bash
git add templates/pluginPage.tpl
git commit -m "feat(plugins): add pluginPage template for plugin-rendered pages"
```

---

### Task 4: Add Plugin Page Route and Handler

**Files:**
- Modify: `server/routes.go`

**Step 1: Add the wildcard route**

In `server/routes.go`, add the plugin page route at the end of `registerRoutes` function (before the closing brace). This registers a catch-all for plugin pages:

```go
// Plugin pages
pm := appContext.PluginManager()
if pm != nil {
    pluginCtxFn := wrapContextWithPlugins(appContext, template_context_providers.PluginPageContextProvider(appContext, pm))
    router.Methods(http.MethodGet, http.MethodPost).
        PathPrefix("/plugins/").
        HandlerFunc(template_handlers.RenderTemplate("pluginPage.tpl", pluginCtxFn))
}
```

**Step 2: Create the context provider**

Create `server/template_handlers/template_context_providers/plugin_page_context.go`:

```go
package template_context_providers

import (
	"io"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/plugin_system"
)

func PluginPageContextProvider(appContext *application_context.MahresourcesContext, pm *plugin_system.PluginManager) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := staticTemplateCtx(request)

		// Parse /plugins/{pluginName}/{path...} from URL
		path := strings.TrimPrefix(request.URL.Path, "/plugins/")
		parts := strings.SplitN(path, "/", 2)

		pluginName := ""
		pagePath := ""
		if len(parts) >= 1 {
			pluginName = parts[0]
		}
		if len(parts) >= 2 {
			pagePath = parts[1]
		}

		ctx["pageTitle"] = "Plugin: " + pluginName

		if !pm.HasPage(pluginName, pagePath) {
			ctx["pluginError"] = "Page not found"
			ctx["pluginPageTitle"] = "Not Found"
			ctx["errorMessage"] = "" // prevent error.tpl from rendering
			return ctx
		}

		// Build query map
		queryMap := make(map[string]any)
		for k, v := range request.URL.Query() {
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

		// Build headers map
		headerMap := make(map[string]any)
		for k, v := range request.Header {
			headerMap[strings.ToLower(k)] = v[0]
		}

		// Read body for POST requests
		var body string
		if request.Method == http.MethodPost && request.Body != nil {
			bodyBytes, err := io.ReadAll(request.Body)
			if err == nil {
				body = string(bodyBytes)
			}
		}

		pageCtx := plugin_system.PageContext{
			Path:    request.URL.String(),
			Method:  request.Method,
			Query:   queryMap,
			Headers: headerMap,
			Body:    body,
		}

		html, err := pm.HandlePage(pluginName, pagePath, pageCtx)
		if err != nil {
			ctx["pluginError"] = err.Error()
			ctx["pluginPageTitle"] = "Error"
		} else {
			ctx["pluginContent"] = html
			ctx["pluginPageTitle"] = pluginName + " - " + pagePath
		}

		return ctx
	}
}
```

**Step 3: Run full Go test suite to ensure no compile errors or regressions**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS (or existing passes unchanged)

**Step 4: Commit**

```bash
git add server/routes.go server/template_handlers/template_context_providers/plugin_page_context.go
git commit -m "feat(plugins): add wildcard route and context provider for plugin pages"
```

---

### Task 5: Add Plugins Dropdown to Navigation Menu

**Files:**
- Modify: `server/routes.go` (the `wrapContextWithPlugins` function)
- Modify: `templates/partials/menu.tpl`

**Step 1: Inject menu items into template context**

Modify `wrapContextWithPlugins` in `server/routes.go` to include menu items:

```go
func wrapContextWithPlugins(appContext *application_context.MahresourcesContext, ctxFn func(request *http.Request) pongo2.Context) func(request *http.Request) pongo2.Context {
	pm := appContext.PluginManager()
	if pm == nil {
		return ctxFn
	}
	return func(request *http.Request) pongo2.Context {
		ctx := ctxFn(request)
		ctx["_pluginManager"] = pm
		ctx["currentPath"] = request.URL.String()
		ctx["pluginMenuItems"] = pm.GetMenuItems()
		return ctx
	}
}
```

**Step 2: Add "Plugins" dropdown to menu template**

In `templates/partials/menu.tpl`, add the Plugins dropdown after the Admin dropdown (before the closing `</div>` of `navbar-links`). Add it right after the Admin dropdown's closing `</div>`:

```django
{% if pluginMenuItems %}
<div class="navbar-dropdown" @click.outside="pluginsOpen = false">
    <button @click="pluginsOpen = !pluginsOpen"
            class="navbar-link navbar-link--dropdown"
            :class="{ 'navbar-link--active': pluginsOpen {% for pi in pluginMenuItems %}|| '{{ pi.FullPath }}' == '{{ path }}'{% endfor %} }">
        <span>Plugins</span>
        <svg class="navbar-dropdown-arrow" :class="{ 'rotate-180': pluginsOpen }" width="10" height="10" viewBox="0 0 10 10" fill="none">
            <path d="M2 4L5 7L8 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
    </button>
    <div x-show="pluginsOpen"
         x-cloak
         x-transition:enter="transition ease-out duration-150"
         x-transition:enter-start="opacity-0 -translate-y-1"
         x-transition:enter-end="opacity-100 translate-y-0"
         x-transition:leave="transition ease-in duration-100"
         x-transition:leave-start="opacity-100 translate-y-0"
         x-transition:leave-end="opacity-0 -translate-y-1"
         class="navbar-dropdown-menu">
        {% for pi in pluginMenuItems %}
        <a href="{{ pi.FullPath }}"
           class="navbar-dropdown-item {% if pi.FullPath == path %}navbar-dropdown-item--active{% endif %}"
           @click="pluginsOpen = false">
            {{ pi.Label }}
        </a>
        {% endfor %}
    </div>
</div>
{% endif %}
```

Also update the `x-data` at the top of the `<nav>` to include `pluginsOpen`:

Change: `x-data="{ mobileOpen: false, adminOpen: false }"`
To: `x-data="{ mobileOpen: false, adminOpen: false, pluginsOpen: false }"`

And add plugin items to the mobile nav section (after the Admin mobile section, before the mobile menu's closing `</div>`):

```django
{% if pluginMenuItems %}
<div class="navbar-mobile-divider"></div>

<div class="navbar-mobile-section">
    <span class="navbar-mobile-label">Plugins</span>
    {% for pi in pluginMenuItems %}
    <a href="{{ pi.FullPath }}"
       class="navbar-mobile-link {% if pi.FullPath == path %}navbar-mobile-link--active{% endif %}"
       @click="mobileOpen = false">
        {{ pi.Label }}
    </a>
    {% endfor %}
</div>
{% endif %}
```

**Step 3: Build the application to verify templates compile**

Run: `npm run build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add server/routes.go templates/partials/menu.tpl
git commit -m "feat(plugins): add Plugins dropdown to navigation menu"
```

---

### Task 6: Add E2E Test Plugin and Tests

**Files:**
- Modify: `e2e/test-plugins/test-banner/plugin.lua` (add page and menu registration)
- Create: `e2e/tests/plugins/plugin-pages.spec.ts`

**Step 1: Update the test plugin**

Add page and menu registration to `e2e/test-plugins/test-banner/plugin.lua` (append to the existing `init()` function, before the closing `end`):

```lua
    mah.page("test-page", function(ctx)
        return '<div data-testid="plugin-page-content"><h2>Test Plugin Page</h2><p>Method: ' .. ctx.method .. '</p><p>Path: ' .. ctx.path .. '</p></div>'
    end)

    mah.page("echo-query", function(ctx)
        local q = ctx.query.msg or "no message"
        return '<div data-testid="plugin-echo">' .. q .. '</div>'
    end)

    mah.menu("Test Page", "test-page")
    mah.menu("Echo Query", "echo-query")
```

**Step 2: Write E2E test**

Create `e2e/tests/plugins/plugin-pages.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Pages', () => {
  test('should show Plugins dropdown in navigation', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const pluginsButton = page.locator('button', { hasText: 'Plugins' });
    await expect(pluginsButton).toBeVisible();
  });

  test('should show plugin menu items in dropdown', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const pluginsButton = page.locator('button', { hasText: 'Plugins' });
    await pluginsButton.click();
    await expect(page.locator('a[href="/plugins/test-banner/test-page"]')).toBeVisible();
    await expect(page.locator('a[href="/plugins/test-banner/echo-query"]')).toBeVisible();
  });

  test('should navigate to plugin page and display content', async ({ page }) => {
    await page.goto('/plugins/test-banner/test-page');
    await page.waitForLoadState('load');
    const content = page.getByTestId('plugin-page-content');
    await expect(content).toBeVisible();
    await expect(content).toContainText('Test Plugin Page');
    await expect(content).toContainText('Method: GET');
  });

  test('should pass query parameters to plugin page', async ({ page }) => {
    await page.goto('/plugins/test-banner/echo-query?msg=hello+world');
    await page.waitForLoadState('load');
    const echo = page.getByTestId('plugin-echo');
    await expect(echo).toBeVisible();
    await expect(echo).toContainText('hello world');
  });

  test('should show error for nonexistent plugin page', async ({ page }) => {
    await page.goto('/plugins/test-banner/nonexistent');
    await page.waitForLoadState('load');
    await expect(page.locator('text=Page not found')).toBeVisible();
  });

  test('should show error for nonexistent plugin', async ({ page }) => {
    await page.goto('/plugins/no-such-plugin/anything');
    await page.waitForLoadState('load');
    await expect(page.locator('text=Page not found')).toBeVisible();
  });

  test('plugin page should have standard navigation', async ({ page }) => {
    await page.goto('/plugins/test-banner/test-page');
    await page.waitForLoadState('load');
    // Should have the standard nav bar
    await expect(page.locator('a[href="/notes"]')).toBeVisible();
    await expect(page.locator('a[href="/resources"]')).toBeVisible();
  });
});
```

**Step 3: Build the application**

Run: `npm run build`
Expected: Build succeeds

**Step 4: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Plugin Pages"`
Expected: All tests PASS

Note: If tests fail, debug by:
1. Check server logs for Lua errors
2. Run with `npm run test:with-server:headed` to see the browser
3. Check the template renders correctly

**Step 5: Run all E2E tests to check for regressions**

Run: `cd e2e && npm run test:with-server`
Expected: All existing tests still PASS

**Step 6: Commit**

```bash
git add e2e/test-plugins/test-banner/plugin.lua e2e/tests/plugins/plugin-pages.spec.ts
git commit -m "test(plugins): add E2E tests for plugin pages and menu items"
```

---

### Task 7: Update Example Plugin in Documentation

**Files:**
- Modify: `plugins/example-plugin/plugin.lua` (if it exists, add page/menu example)
- Verify: Run full test suite one final time

**Step 1: Check if example plugin exists and update it**

If `plugins/example-plugin/plugin.lua` exists, add page and menu examples to its `init()` function:

```lua
    -- Register a custom page
    mah.page("info", function(ctx)
        return "<h2>Example Plugin</h2><p>This page is rendered by the example plugin.</p>"
    end)

    -- Add a menu item for the page
    mah.menu("Plugin Info", "info")
```

**Step 2: Run full Go test suite**

Run: `go test ./... --tags 'json1 fts5'`
Expected: All PASS

**Step 3: Run full E2E test suite**

Run: `cd e2e && npm run test:with-server`
Expected: All PASS

**Step 4: Commit**

```bash
git add -A
git commit -m "docs(plugins): add page/menu examples to example plugin"
```
