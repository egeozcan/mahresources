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
[plugin:data-views:stat-card mrql="type=resource COUNT()" label="Total Resources"]
[plugin:data-views:list mrql="type=note LIMIT 10"]
```

## Scoping Mechanism

MRQL queries are scoped by injecting an `AND owner=<id>` condition at the AST level after parsing, before translation. This avoids string manipulation and injection issues.

### Scope Resolution

| Scope | Behavior | ID Source |
|-------|----------|-----------|
| `"entity"` (default) | Owned by current entity | `scope_entity_id` from opts |
| `"parent"` | Owned by current entity's owner | Look up `OwnerId` of entity with `scope_entity_id` |
| `"root"` | Owned by root of ownership chain | Walk `OwnerId` chain until nil |
| `"global"` | No scoping applied | — |

### Edge Cases

- Entity has no owner + `scope="parent"` → falls back to global (no results is a natural outcome)
- `scope="root"` on entity with no owner → same as `scope="entity"`
- `scope="root"` traversal capped at 50 hops to prevent cycles
- Scoping is most meaningful on Group detail pages (groups own things). On Resource/Note pages, `scope="entity"` filters to things owned by that entity, which is usually empty but correct.

### AST Injection

After parsing the MRQL query, wrap the existing `Where` node:

```go
parsed := mrql.Parse(query)
if scopeID != 0 {
    ownerFilter := &ComparisonExpr{Field: "owner", Op: "=", Value: scopeID}
    if parsed.Where != nil {
        parsed.Where = &BinaryExpr{Left: ownerFilter, Op: AND, Right: parsed.Where}
    } else {
        parsed.Where = ownerFilter
    }
}
```

### Usage Examples

```
[plugin:data-views:table mrql="type=resource" scope="parent"]
[plugin:data-views:pie-chart mrql="type=resource GROUP BY contentType COUNT()" scope="root"]
[plugin:data-views:list mrql="type=note" scope="global"]
```

## Per-Render Query Cache

Duplicate MRQL queries within a single page render hit the DB only once.

### Implementation

A `map[string]*QueryResult` created at the start of `shortcodes.Process()`:

1. Created when `Process()` is called — one cache per page render
2. Passed to the plugin renderer closure, which passes it to `mah.db.mrql_query()`
3. On each `mrql_query` call: build cache key → check map → return cached or execute and store
4. Garbage collected when `Process()` returns — no cross-request leakage

**Cache key format:** `fmt.Sprintf("%s|%d|%d|%d", normalizedQuery, scopeID, limit, buckets)`

Entity property lookups (`ctx.entity`) are not cached — they're already in memory from context building.

## Shortcode Compatibility Matrix

### Single-Value Shortcodes

Work with `path`, `field`, and `mrql` in aggregated mode. For aggregated results, single-value shortcodes use the first aggregate column (COUNT, SUM, AVG, MIN, MAX) from the first row — not the group-by column:

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
| `plugin_system/shortcodes.go` | Add `ctx.entity` table building via reflection |
| `plugin_system/db_api.go` | Register `mah.db.mrql_query()` function |
| `shortcodes/processor.go` | Add per-render query cache, pass to renderer |
| `plugins/data-views/plugin.lua` | Add `resolve_data_source` helper, update all render functions |
| `mrql/scoping.go` (new) | AST-level scope injection logic |
