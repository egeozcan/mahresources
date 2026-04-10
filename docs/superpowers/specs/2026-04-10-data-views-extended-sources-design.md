# Data-Views Plugin: MRQL and Entity Property Data Sources

**Date:** 2026-04-10
**Status:** Approved

## Summary

Extend the data-views plugin to support two new data sources beyond metadata: entity properties (core struct fields like Name, FileSize, ContentType) and MRQL query results. Each shortcode uses exactly one data source per instance, selected via the `field`, `mrql`, or existing `path` attribute.

Architecture: Hybrid (Approach C) — Lua dispatches data resolution, Go executes and caches.

## Data Source Resolution

Each data-views shortcode uses one of three data source attributes:

- `path` — existing behavior, navigates entity metadata JSON (`ctx.value`)
- `field` — new, reads a core entity property from `ctx.entity` Lua table
- `mrql` — new, executes an MRQL query via `mah.db.mrql_query()` Lua function

**Priority (if multiple specified):** `mrql` > `field` > `path`

A shared Lua helper `resolve_data_source(ctx)` is called by all shortcode render functions instead of directly navigating `ctx.value`:

```lua
function resolve_data_source(ctx)
  local attrs = ctx.attrs
  if attrs.mrql then
    return mah.db.mrql_query(attrs.mrql, {
      scope_entity_id = ctx.entity_id,
      scope = attrs.scope,  -- nil defaults to "entity"
      limit = tonumber(attrs.limit),
      buckets = tonumber(attrs.buckets),
    })
  elseif attrs.field then
    return ctx.entity[attrs.field]
  else
    return resolve_path(ctx.value, attrs.path)
  end
end
```

## Entity Properties: `ctx.entity` Table

When building the Lua shortcode context in `plugin_system/shortcodes.go`, a new `entity` sub-table is populated using reflection on `MetaShortcodeContext.Entity` (same approach as the existing `RenderPropertyShortcode`).

**Plumbing required:** Currently `RenderShortcode()` only receives `entityType`, `entityID`, `meta`, and `attrs` — it does not receive the full entity object. The signature must be extended to accept the entity (or the full `MetaShortcodeContext`), and the call sites in `shortcode_tag.go` (line 49) and `server/routes.go` (line 189) must be updated to pass it through. The `PluginRenderer` callback type in `shortcodes/processor.go` must also be updated.

### Fields Exposed

| Field | Resource | Note | Group | Lua Type |
|-------|----------|------|-------|----------|
| `Name` | yes | yes | yes | string |
| `Description` | yes | yes | yes | string |
| `ID` | yes | yes | yes | number |
| `CreatedAt` | yes | yes | yes | string (RFC3339) |
| `UpdatedAt` | yes | yes | yes | string (RFC3339) |
| `OwnerId` | yes | yes | yes | number/nil |
| `FileSize` | yes | - | - | number |
| `ContentType` | yes | - | - | string |
| `Width` | yes | - | - | number |
| `Height` | yes | - | - | number |
| `OriginalName` | yes | - | - | string |
| `Hash` | yes | - | - | string |
| `Category` | yes | - | - | string |
| `StartDate` | - | yes | - | string/nil |
| `EndDate` | - | yes | - | string/nil |
| `URL` | - | - | yes | string/nil |

### Usage Examples

```
[plugin:data-views:stat-card field="FileSize" format="filesize" label="Size"]
[plugin:data-views:badge field="ContentType" labels="image/*:Image,video/*:Video" colors="image/*:blue,video/*:purple"]
[plugin:data-views:format field="CreatedAt" format="date"]
```

## MRQL Query API

### Lua Function

New function `mah.db.mrql_query(query, opts)` registered in `plugin_system/db_api.go`.

**Parameters:**
- `query` (string) — MRQL query expression
- `opts` (table):
  - `scope_entity_id` (number) — current entity's ID for scoping
  - `scope` (string, default `"entity"`) — `"entity"`, `"parent"`, `"root"`, or `"global"`
  - `limit` (number, default 20)
  - `buckets` (number, default 5)

**Return value** — Lua table:

```lua
{
  mode = "flat" | "aggregated" | "bucketed",
  entity_type = "resource" | "note" | "group",

  -- flat mode:
  items = {
    { ID=1, Name="...", Description="...", Meta={...}, entity_type="resource", ... },
  },

  -- aggregated mode:
  rows = {
    { contentType="image/png", COUNT=42, SUM_fileSize=1024000 },
  },

  -- bucketed mode:
  groups = {
    { key={contentType="image/png"}, items={...} },
  },
}
```

Each item in flat/bucketed results includes all entity fields (same set as `ctx.entity`) plus `Meta` as a nested table.

**Error handling:** Returns `nil, error_string`. The `resolve_data_source` helper renders a styled error div matching the built-in `[mrql]` shortcode error style.

**Go implementation:** Reuses the existing `QueryExecutor` pipeline — parses MRQL, applies scoping, executes via GORM, converts to Lua tables.

### Usage Examples

```
[plugin:data-views:table mrql="type=resource" columns="Name,ContentType,FileSize"]
[plugin:data-views:pie-chart mrql="type=resource GROUP BY contentType COUNT()"]
[plugin:data-views:stat-card mrql="type=resource GROUP BY category COUNT()" aggregate="COUNT" label="Total Resources"]
[plugin:data-views:list mrql="type=note LIMIT 10"]
```

**Note:** MRQL aggregates (COUNT, SUM, etc.) require a GROUP BY clause. For total counts without grouping, use the existing `mah.db.count_resources()` / `mah.db.count_notes()` / `mah.db.count_groups()` Lua functions directly (these are already available and don't need MRQL).

## Scoping Mechanism

MRQL queries are scoped by applying a direct `owner_id = <scopeID>` filter at the translator level (GORM scope), not at the AST level. This is necessary because the MRQL `owner` field resolves to owner group **name** matching (via `translateRelationComparison` in `translator.go:624`), not ID matching. Injecting `owner = 42` at the AST would try to find entities whose owner is named "42".

### Scope Resolution

| Scope | Behavior | ID Source |
|-------|----------|-----------|
| `"entity"` (default) | Owned by current entity | `scope_entity_id` from opts |
| `"parent"` | Owned by current entity's owner | Look up `OwnerId` of entity with `scope_entity_id` |
| `"root"` | Owned by root of ownership chain | Walk `OwnerId` chain until nil |
| `"global"` | No scoping applied | — |

### Edge Cases

- Entity has no owner + `scope="parent"` → returns empty results (not global). Unresolvable scopes never fan out.
- `scope="root"` on entity with no owner → same as `scope="entity"`
- `scope="root"` traversal capped at 50 hops to prevent cycles
- Scoping is most meaningful on Group detail pages (groups own things). On Resource/Note pages, `scope="entity"` filters to things owned by that entity, which is usually empty but correct.

### Translator-Level FK Filter

After parsing and validating the MRQL query, the scope filter is applied as a GORM scope during translation — a direct FK condition on the entity table, bypassing the MRQL field resolution:

```go
// Applied in the translator, after normal WHERE translation
if scopeID != 0 {
    db = db.Where("owner_id = ?", scopeID)
}
```

This runs alongside (AND'd with) any user-specified filters in the MRQL query. It uses the raw `owner_id` column, avoiding the name-based traversal that `owner = "..."` triggers.

### Usage Examples

```
[plugin:data-views:table mrql="type=resource" scope="parent"]
[plugin:data-views:pie-chart mrql="type=resource GROUP BY contentType COUNT()" scope="root"]
[plugin:data-views:list mrql="type=note" scope="global"]
```

## Per-Render Query Cache

Duplicate MRQL queries within a single page render hit the DB only once.

### Implementation

The cache is stored in the request context (`context.Context`), not inside `shortcodes.Process()`. This is necessary because `Process()` is called multiple times per page render — once each for CustomHeader, CustomSidebar, CustomSummary, and CustomAvatar (4 calls per entity in `server/routes.go:209-239`, plus the template filter call in `shortcode_tag.go`). A cache scoped to `Process()` would miss duplicates across these calls.

**Lifecycle:**
1. Created in the HTTP handler (or route setup) and stored in the request context via `context.WithValue`
2. The `mah.db.mrql_query()` Go function retrieves the cache from the request context
3. On each call: build cache key → check map → return cached or execute and store
4. Garbage collected when the request ends — no cross-request leakage

**Cache key format:** `fmt.Sprintf("%s|%d|%d|%d", normalizedQuery, scopeID, limit, buckets)`

Entity property lookups (`ctx.entity`) are not cached — they're already in memory from context building.

## Shortcode Compatibility Matrix

### Single-Value Shortcodes

Work with `path`, `field`, and `mrql` in aggregated mode. For aggregated results, single-value shortcodes require an explicit `aggregate` attribute (e.g., `aggregate="COUNT"`, `aggregate="SUM_fileSize"`) to select which value from the result row to use. This is necessary because aggregated rows are `map[string]any` with no guaranteed key order, and queries may return multiple rows without explicit ORDER BY. If `aggregate` is omitted, the shortcode renders an error hint. If the query returns multiple rows, only the first row is used (author should use `LIMIT 1` or a single-group query for deterministic results):

| Shortcode | `path` | `field` | `mrql` (aggregated) |
|-----------|--------|---------|---------------------|
| badge | yes | yes | yes |
| format | yes | yes | yes |
| stat-card | yes | yes | yes |
| meter | yes | yes | yes |
| barcode | yes | yes | yes |
| qr-code | yes | yes | yes |
| link-preview | yes | yes | yes |
| conditional | yes | yes | yes |

### Collection Shortcodes

Work with `path` (array/object meta values) and `mrql` (flat and aggregated results):

| Shortcode | `path` | `field` | `mrql` (flat) | `mrql` (aggregated) |
|-----------|--------|---------|---------------|---------------------|
| sparkline | yes | no | yes | yes |
| list | yes | no | yes | yes |
| bar-chart | yes | no | yes | yes |
| pie-chart | yes | no | yes | yes |
| table | yes | no | yes | yes |
| timeline-chart | yes | no | yes | no |
| count-badge | yes | no | yes (result count) | yes (COUNT value) |

### Unchanged Shortcodes

| Shortcode | Reason |
|-----------|--------|
| embed | Resource-ID based, no new data source needed |
| image | Resource-ID/path based, no new data source needed |
| json-tree | Works with `path`, `field` (JSON-typed), and `mrql` (renders result as tree) |

The `resolve_data_source` helper normalizes results so each render function gets a consistent shape (single value, array, or keyed object) regardless of source.

## Files Changed

| File | Change |
|------|--------|
| `plugin_system/shortcodes.go` | Extend `RenderShortcode()` to accept full entity object; build `ctx.entity` Lua table via reflection |
| `plugin_system/db_api.go` | Register `mah.db.mrql_query()` function with scope resolution and request-context cache lookup |
| `server/routes.go` | Create request-scoped MRQL cache; pass entity object through to plugin renderer closure |
| `server/template_handlers/template_filters/shortcode_tag.go` | Update `pluginRenderer` closure to forward `MetaShortcodeContext.Entity` to `RenderShortcode()` |
| `shortcodes/processor.go` | Update `PluginRenderer` callback signature to include entity object |
| `plugins/data-views/plugin.lua` | Add `resolve_data_source` helper with `aggregate` selector; update all render functions |
| `mrql/scoping.go` (new) | Scope resolution logic (entity/parent/root ID lookup) and GORM scope application |
