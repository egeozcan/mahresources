# Display Renderers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add built-in shape-aware rendering for well-known object types (URL, GeoLocation, DateRange, Dimensions) and a plugin API (`mah.display_type()`) for custom renderers via `x-display` schema annotations.

**Architecture:** Built-in renderers are pure TypeScript functions in `display-renderers.ts` with a detect/render pattern. Plugin renderers use the proven block-type pattern: Lua registration via `mah.display_type()`, server-side rendering via POST endpoint, client-side fetching in display-mode.ts. The renderer pipeline in `_renderValue` checks `x-display` annotation first, then shape detectors, then falls back to existing rendering.

**Tech Stack:** TypeScript/Lit (frontend), Go/Lua (server plugin system), Playwright E2E tests

**Spec:** `docs/superpowers/specs/2026-04-05-display-renderers.md`

---

### Task 1: Create Built-in Shape Detectors and Renderers

**Files:**
- Create: `src/schema-editor/display-renderers.ts`

- [ ] **Step 1: Create the display-renderers.ts file**

```typescript
import { html, nothing, type TemplateResult } from 'lit';

/**
 * A built-in display renderer: a shape detector paired with a renderer.
 * Detectors are checked in order; first match wins.
 */
export interface BuiltinRenderer {
  name: string;
  detect: (val: any) => boolean;
  render: (val: any) => TemplateResult;
}

// ── URL / Location ──────────────────────────────────────────────────────────

function isURLShape(val: any): boolean {
  return (
    val != null &&
    typeof val === 'object' &&
    typeof val.href === 'string' &&
    (typeof val.host === 'string' || typeof val.hostname === 'string')
  );
}

function renderURL(val: any): TemplateResult {
  const href = val.href as string;
  const host = (val.host || val.hostname || '') as string;
  return html`
    <div>
      <a href=${href} target="_blank" rel="noopener noreferrer"
        class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300 break-all"
        @click=${(e: Event) => e.stopPropagation()}
      >${href}</a>
      ${host ? html`<div class="text-[10px] font-mono text-stone-400 mt-0.5">${host}</div>` : nothing}
    </div>
  `;
}

// ── GeoLocation ─────────────────────────────────────────────────────────────

function isGeoShape(val: any): boolean {
  if (val == null || typeof val !== 'object') return false;
  const hasLatLon =
    (typeof val.latitude === 'number' && typeof val.longitude === 'number') ||
    (typeof val.lat === 'number' && typeof val.lng === 'number');
  return hasLatLon;
}

function renderGeo(val: any): TemplateResult {
  const lat = (val.latitude ?? val.lat) as number;
  const lng = (val.longitude ?? val.lng) as number;
  const osmUrl = `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=15/${lat}/${lng}`;
  return html`
    <div>
      <span class="font-mono text-sm">${lat.toFixed(6)}, ${lng.toFixed(6)}</span>
      <a href=${osmUrl} target="_blank" rel="noopener noreferrer"
        class="text-indigo-600 hover:text-indigo-800 underline decoration-indigo-300 text-xs ml-2"
        @click=${(e: Event) => e.stopPropagation()}
      >View on map</a>
    </div>
  `;
}

// ── Date Range ──────────────────────────────────────────────────────────────

function isDateRangeShape(val: any): boolean {
  if (val == null || typeof val !== 'object') return false;
  if (typeof val.start !== 'string' || typeof val.end !== 'string') return false;
  const s = new Date(val.start);
  const e = new Date(val.end);
  return !isNaN(s.getTime()) && !isNaN(e.getTime());
}

function renderDateRange(val: any): TemplateResult {
  const opts: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' };
  const s = new Date(val.start).toLocaleDateString(undefined, opts);
  const e = new Date(val.end).toLocaleDateString(undefined, opts);
  return html`<span class="text-sm">${s} \u2014 ${e}</span>`;
}

// ── Dimensions ──────────────────────────────────────────────────────────────

function isDimensionsShape(val: any): boolean {
  return (
    val != null &&
    typeof val === 'object' &&
    typeof val.width === 'number' &&
    typeof val.height === 'number'
  );
}

function renderDimensions(val: any): TemplateResult {
  return html`<span class="font-mono text-sm">${val.width} \u00D7 ${val.height}</span>`;
}

// ── Registry ────────────────────────────────────────────────────────────────

/** Ordered list of built-in renderers. First match wins. */
export const builtinRenderers: BuiltinRenderer[] = [
  { name: 'url', detect: isURLShape, render: renderURL },
  { name: 'geo', detect: isGeoShape, render: renderGeo },
  { name: 'daterange', detect: isDateRangeShape, render: renderDateRange },
  { name: 'dimensions', detect: isDimensionsShape, render: renderDimensions },
];

/** Look up a built-in renderer by name (for forced x-display values). */
export function getBuiltinRenderer(name: string): BuiltinRenderer | undefined {
  return builtinRenderers.find(r => r.name === name);
}

/** Run shape detectors against a value. Returns the first matching renderer, or undefined. */
export function detectShape(val: any): BuiltinRenderer | undefined {
  if (val == null || typeof val !== 'object' || Array.isArray(val)) return undefined;
  return builtinRenderers.find(r => r.detect(val));
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/egecan/Code/mahresources && npx vite build 2>&1 | tail -3`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add src/schema-editor/display-renderers.ts
git commit -m "feat(display): add built-in shape detectors for URL, geo, date range, dimensions"
```

---

### Task 2: Integrate Renderer Pipeline into display-mode.ts

**Files:**
- Modify: `src/schema-editor/modes/display-mode.ts`

- [ ] **Step 1: Add `xDisplay` to DisplayField and update flattenForDisplay**

In `display-mode.ts`, add `xDisplay` to the `DisplayField` interface:

```typescript
interface DisplayField {
  path: string;
  label: string;
  description: string;
  type: string;
  format: string;
  value: any;
  isEmpty: boolean;
  isLong: boolean;
  enum: any[] | null;
  enumLabels: string[] | null;
  xDisplay: string;  // x-display annotation from schema, empty if absent
}
```

In `flattenForDisplay`, read `x-display` from the raw prop. If `x-display` is present on an object property, do NOT flatten recursively — emit it as a single field:

Replace the section starting at `// Nested object with properties — flatten recursively` (around line 82-86) with:

```typescript
    // Read x-display annotation from the raw (unresolved) property first,
    // then fall back to the resolved property.
    const xDisplay = (rawProp['x-display'] || prop['x-display'] || '') as string;

    // If x-display is set on an object property, do NOT flatten — emit as a whole field
    // so the renderer receives the complete object.
    if (xDisplay && prop.properties) {
      // Skip recursive flattening — treat as a single field
    } else if (prop.properties) {
      // Nested object with properties — flatten recursively
      fields.push(...flattenForDisplay(prop, value, root, path, label, depth + 1));
      continue;
    }
```

Also update the field construction to include `xDisplay`:

```typescript
    const field: DisplayField = {
      path, label, description, type: fieldType, format,
      value: val,
      isEmpty: isEmptyValue(val),
      isLong: false,
      enum: enumValues,
      enumLabels,
      xDisplay,
    };
```

- [ ] **Step 2: Update `_renderValue` with the renderer pipeline**

Add the import at the top of display-mode.ts:

```typescript
import { detectShape, getBuiltinRenderer } from '../display-renderers';
```

In `_renderValue`, add the renderer pipeline BEFORE the existing enum/boolean/etc checks (right after the isEmpty check). Insert after `const val = field.value;` (line ~211):

```typescript
    // ── Renderer pipeline ──────────────────────────────────────────────
    const xd = field.xDisplay;

    // 1. Plugin renderer (x-display: "plugin:name:type")
    if (xd.startsWith('plugin:')) {
      return this._renderPluginDisplay(field);
    }

    // 2. Forced built-in renderer (x-display: "url", "geo", etc.)
    if (xd && xd !== 'raw' && xd !== 'none') {
      const renderer = getBuiltinRenderer(xd);
      if (renderer) return renderer.render(val);
    }

    // 3. Opt-out: x-display: "raw" or "none" → skip shape detection, use key-value grid
    // (falls through to existing object handling below)

    // 4. Auto shape detection for objects (when no x-display set)
    if (!xd && typeof val === 'object' && val !== null && !Array.isArray(val)) {
      const detected = detectShape(val);
      if (detected) return detected.render(val);
    }
    // ── End renderer pipeline ──────────────────────────────────────────
```

- [ ] **Step 3: Add the plugin display rendering method**

Add a new `@state()` field to track plugin render results, and the render method. Add to the class:

```typescript
  /** Cache of plugin-rendered HTML keyed by field path */
  @state() private _pluginHtml: Record<string, string> = {};
  @state() private _pluginErrors: Record<string, boolean> = {};

  private _renderPluginDisplay(field: DisplayField): TemplateResult {
    const key = field.path;

    // Already fetched
    if (this._pluginHtml[key] !== undefined) {
      const wrapper = document.createElement('div');
      wrapper.innerHTML = this._pluginHtml[key];
      return html`${wrapper}`;
    }

    // Error state — fall back to key-value grid
    if (this._pluginErrors[key]) {
      if (typeof field.value === 'object' && field.value !== null) {
        return this._renderObjectValue(field.value);
      }
      return html`<span class="text-stone-400 text-xs italic">Render error</span>`;
    }

    // Parse plugin:name:type
    const parts = field.xDisplay.split(':');
    if (parts.length < 3) {
      return this._renderObjectValue(field.value);
    }
    const pluginName = parts[1];
    const typeName = parts[2];

    // Start async fetch
    this._fetchPluginDisplay(key, pluginName, typeName, field);

    // Show loading placeholder
    return html`<span class="text-stone-400 text-xs animate-pulse">Loading...</span>`;
  }

  private async _fetchPluginDisplay(key: string, pluginName: string, typeName: string, field: DisplayField) {
    try {
      const resp = await fetch(`/v1/plugins/${pluginName}/display/render`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: typeName,
          value: field.value,
          schema: {},
          field_path: field.path,
          field_label: field.label,
        }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const htmlStr = await resp.text();
      this._pluginHtml = { ...this._pluginHtml, [key]: htmlStr };
    } catch {
      this._pluginErrors = { ...this._pluginErrors, [key]: true };
    }
  }
```

- [ ] **Step 4: Verify build succeeds**

Run: `cd /Users/egecan/Code/mahresources && npx vite build 2>&1 | tail -3`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add src/schema-editor/modes/display-mode.ts
git commit -m "feat(display): integrate renderer pipeline with x-display, shape detection, and plugin support"
```

---

### Task 3: Server-side Plugin Display Types

**Files:**
- Create: `plugin_system/display_types.go`
- Create: `plugin_system/display_render.go`
- Modify: `plugin_system/manager.go`

- [ ] **Step 1: Create display_types.go**

```go
package plugin_system

import (
	"fmt"
	"regexp"

	lua "github.com/yuin/gopher-lua"
)

// PluginDisplayType holds a plugin-defined display renderer.
type PluginDisplayType struct {
	PluginName string
	TypeName   string // full namespaced: plugin:<pluginName>:<type>
	Label      string
	Render     *lua.LFunction
	State      *lua.LState
}

var validDisplayTypeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)

// parseDisplayTypeTable parses a Lua table from mah.display_type({...}) into a PluginDisplayType.
// Required fields: type, label, render.
func parseDisplayTypeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginDisplayType, error) {
	dt := &PluginDisplayType{
		PluginName: pluginName,
	}

	// Required: type
	if v := tbl.RawGetString("type"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'type'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'type' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validDisplayTypeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid type name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		dt.TypeName = "plugin:" + pluginName + ":" + raw
	}

	// Required: label
	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		dt.Label = v.String()
	}

	// Required: render (must be a function)
	if v := tbl.RawGetString("render"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render' must be a function")
	} else {
		dt.Render = fn
	}

	return dt, nil
}
```

- [ ] **Step 2: Create display_render.go**

```go
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
type DisplayRenderContext struct {
	Value      map[string]any `json:"value"`
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
```

- [ ] **Step 3: Update manager.go**

Three changes to `plugin_system/manager.go`:

**3a.** Add `displayTypes` field to the `PluginManager` struct. Find the line `blockTypes   map[string][]*PluginBlockType` and add after it:

```go
	displayTypes map[string][]*PluginDisplayType   // pluginName -> display types
```

**3b.** Initialize `displayTypes` in `NewPluginManager`. Find where `blockTypes` is initialized (in the struct literal) and add:

```go
		displayTypes: make(map[string][]*PluginDisplayType),
```

**3c.** Register `mah.display_type` in `registerMahModule`. Find the block that registers `mah.block_type` (search for `mahMod.RawSetString("block_type"`) and add a similar block right after it:

```go
	mahMod.RawSetString("display_type", L.NewFunction(func(L *lua.LState) int {
		tbl := L.CheckTable(1)
		dt, err := parseDisplayTypeTable(L, tbl, *pluginNamePtr)
		if err != nil {
			L.ArgError(1, err.Error())
			return 0
		}
		dt.State = L

		pm.mu.Lock()
		for _, existing := range pm.displayTypes[*pluginNamePtr] {
			if existing.TypeName == dt.TypeName {
				pm.mu.Unlock()
				L.ArgError(1, fmt.Sprintf("duplicate display type %q", dt.TypeName))
				return 0
			}
		}
		pm.displayTypes[*pluginNamePtr] = append(pm.displayTypes[*pluginNamePtr], dt)
		pm.mu.Unlock()
		return 0
	}))
```

**3d.** Add cleanup in `DisablePlugin`. Find the block that cleans up blockTypes (search for `delete(pm.blockTypes, name)`) and add right after it:

```go
	delete(pm.displayTypes, name)
```

- [ ] **Step 4: Verify Go compiles**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add plugin_system/display_types.go plugin_system/display_render.go plugin_system/manager.go
git commit -m "feat(plugins): add mah.display_type() registration and render execution"
```

---

### Task 4: Display Render API Endpoint

**Files:**
- Modify: `server/api_handlers/plugin_api_handlers.go`
- Modify: `server/routes.go`

- [ ] **Step 1: Add the handler in plugin_api_handlers.go**

Add the following function to `server/api_handlers/plugin_api_handlers.go`:

```go
// GetPluginDisplayRenderHandler renders a plugin display type's HTML.
func GetPluginDisplayRenderHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(w, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		vars := mux.Vars(r)
		pluginName := vars["pluginName"]
		if pluginName == "" {
			http.Error(w, "plugin name required", http.StatusBadRequest)
			return
		}

		var req struct {
			Type       string         `json:"type"`
			Value      map[string]any `json:"value"`
			Schema     map[string]any `json:"schema"`
			FieldPath  string         `json:"field_path"`
			FieldLabel string         `json:"field_label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Type == "" {
			http.Error(w, "type is required", http.StatusBadRequest)
			return
		}

		fullTypeName := "plugin:" + pluginName + ":" + req.Type

		renderCtx := plugin_system.DisplayRenderContext{
			Value:      req.Value,
			Schema:     req.Schema,
			FieldPath:  req.FieldPath,
			FieldLabel: req.FieldLabel,
			Settings:   pm.GetPluginSettings(pluginName),
		}

		htmlStr, err := pm.RenderDisplay(pluginName, fullTypeName, renderCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlStr))
	}
}
```

Make sure `encoding/json` is in the imports of the file (it likely already is).

- [ ] **Step 2: Register the route in routes.go**

In `server/routes.go`, find the block render route (search for `block/render`). Add the display render route right after it:

```go
	// Plugin display render endpoint (must be before the catch-all)
	router.Methods(http.MethodPost).Path("/v1/plugins/{pluginName}/display/render").HandlerFunc(
		api_handlers.GetPluginDisplayRenderHandler(appContext),
	)
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add server/api_handlers/plugin_api_handlers.go server/routes.go
git commit -m "feat(api): add POST /v1/plugins/{name}/display/render endpoint"
```

---

### Task 5: E2E Tests for Built-in Renderers

**Files:**
- Create: `e2e/tests/display-renderers.spec.ts`

- [ ] **Step 1: Write E2E tests for built-in shape rendering**

```typescript
/**
 * E2E tests for built-in display renderers (URL, GeoLocation, DateRange, Dimensions).
 * Tests that object values matching well-known shapes render with smart formatting
 * instead of raw key-value grids.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Built-in display renderers', () => {
  let categoryId: number;
  let groupId: number;

  const schema = JSON.stringify({
    type: 'object',
    properties: {
      url: {
        type: 'object',
        title: 'Website',
        properties: {
          href: { type: 'string' },
          host: { type: 'string' },
          protocol: { type: 'string' },
        },
      },
      location: {
        type: 'object',
        title: 'Location',
        properties: {
          latitude: { type: 'number' },
          longitude: { type: 'number' },
        },
      },
      period: {
        type: 'object',
        title: 'Period',
        properties: {
          start: { type: 'string' },
          end: { type: 'string' },
        },
      },
      size: {
        type: 'object',
        title: 'Size',
        properties: {
          width: { type: 'number' },
          height: { type: 'number' },
        },
      },
    },
  });

  const meta = JSON.stringify({
    url: {
      href: 'https://www.example.com/page',
      host: 'www.example.com',
      protocol: 'https:',
    },
    location: {
      latitude: 52.520008,
      longitude: 13.404954,
    },
    period: {
      start: '2024-03-15',
      end: '2024-04-01',
    },
    size: {
      width: 1920,
      height: 1080,
    },
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Renderer Test ${Date.now()}`,
      'Testing built-in display renderers',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Renderer Group ${Date.now()}`,
      categoryId: cat.ID,
      meta,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders URL as clickable link with host subtitle', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    // Should render as a clickable link, not a key-value grid
    const link = display.locator('a[href="https://www.example.com/page"]');
    await expect(link).toBeVisible({ timeout: 3000 });

    // Host subtitle should be visible
    await expect(display).toContainText('www.example.com');
  });

  test('renders GeoLocation as coordinates with map link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    // Should show coordinates
    await expect(display).toContainText('52.520008');
    await expect(display).toContainText('13.404954');

    // Should have an OpenStreetMap link
    const mapLink = display.locator('a[href*="openstreetmap.org"]');
    await expect(mapLink).toBeVisible();
  });

  test('renders DateRange as formatted dates', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    // Should show formatted date range (locale-dependent, check for year)
    await expect(display).toContainText('2024');
    // The em-dash separator between dates
    await expect(display).toContainText('\u2014');
  });

  test('renders Dimensions as W x H', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });

    // Should show "1920 × 1080"
    await expect(display).toContainText('1920');
    await expect(display).toContainText('1080');
    await expect(display).toContainText('\u00D7');
  });
});

test.describe('x-display opt-out', () => {
  test('x-display: "raw" shows key-value grid for URL-shaped object', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        link: {
          type: 'object',
          title: 'Link',
          'x-display': 'raw',
          properties: {
            href: { type: 'string' },
            host: { type: 'string' },
          },
        },
      },
    });
    const cat = await apiClient.createCategory(
      `Raw Test ${Date.now()}`,
      'Testing x-display raw opt-out',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Raw Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({
        link: { href: 'https://example.com', host: 'example.com' },
      }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const display = page.locator('schema-editor[mode="display"]');
      await expect(display).toBeVisible({ timeout: 5000 });

      // Should show as key-value grid (not a clickable link)
      // The href value should be plain text, not an <a> tag
      await expect(display).toContainText('https://example.com');
      const link = display.locator('a[href="https://example.com"]');
      await expect(link).not.toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
```

- [ ] **Step 2: Commit**

```bash
git add e2e/tests/display-renderers.spec.ts
git commit -m "test(e2e): add tests for built-in display renderers and x-display opt-out"
```

---

### Task 6: Build, Run All Tests, Fix Issues

**Files:**
- Possibly modify any of the above files if tests fail

- [ ] **Step 1: Build everything**

```bash
cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5' && npx vite build
```

- [ ] **Step 2: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass.

- [ ] **Step 3: Run the new E2E tests**

Run: `cd e2e && npx playwright test tests/display-renderers.spec.ts --reporter=list --retries=0`
Expected: All tests pass. If any fail, investigate and fix.

- [ ] **Step 4: Run the full E2E suite**

Run: `cd e2e && npm run test:with-server:all`
Expected: All existing tests still pass.

- [ ] **Step 5: Commit any fixes + built assets**

```bash
git add -A
git commit -m "build: rebuild with display renderers, fix any test issues"
```
