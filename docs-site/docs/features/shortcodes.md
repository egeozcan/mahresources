---
sidebar_position: 17
title: Shortcodes
---

# Shortcodes

Shortcodes are bracket-delimited expressions embedded in custom template fields (CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, and CustomMRQLResult) that expand into dynamic HTML at render time. They provide schema-aware metadata display, inline query results, and entity property access without writing Alpine.js or Pongo2 code.

## Syntax

```
[name attr="value" attr2="value2"]
```

The parser recognizes these patterns:

- **Built-in:** `[meta ...]`, `[property ...]`, `[mrql ...]`
- **Plugin:** `[plugin:plugin-name:shortcode-name ...]`

Attribute values can be double-quoted, single-quoted, or unquoted. When a key appears more than once, the last value wins.

## Processing

Shortcodes are processed via the `process_shortcodes` Pongo2 template tag. The five custom template fields (CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, CustomMRQLResult) process shortcodes automatically. Entity description fields also process shortcodes on detail pages; truncated previews in list views do not.

## `[meta]` -- Metadata Display

Renders a metadata field from the entity's `meta` JSON, using the category's MetaSchema for type-aware display.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | -- | Dot-notation path into the entity's Meta JSON (e.g., `cooking.time`, `address.city`) |
| `editable` | No | `false` | Shows a pencil edit button; clicking opens a schema-aware inline form |
| `hide-empty` | No | `false` | Hides the shortcode entirely when the value is absent or null |

### How It Works

- Expands into a `<meta-shortcode>` web component at render time
- Client hydrates using the schema-editor rendering pipeline
- When `editable=true`, clicking the pencil calls the `editMeta` API endpoint
- If the path exists in the MetaSchema, rendering is schema-aware (type formatting, enum pills, shape detection, x-display)
- If no schema exists, falls back to plain value display

### Examples

```
[meta path="cooking.time"]
[meta path="cooking.difficulty" editable=true]
[meta path="address.city" hide-empty=true]
```

Mixed with HTML:

```html
<div class="flex gap-4">
  <strong>Cook time:</strong> [meta path="cooking.time"]
  <strong>Difficulty:</strong> [meta path="cooking.difficulty"]
</div>
```

## `[property]` -- Entity Field Access

Renders a struct field value from the entity object itself (not metadata). Uses Go reflection to access the field by name.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | -- | The struct field name on the entity (e.g., `Name`, `Description`, `CreatedAt`) |
| `raw` | No | `false` | Skip HTML escaping; output the value verbatim |

### How It Works

- Accesses the field using Go reflection on the entity struct
- Output is HTML-escaped by default for safety
- `time.Time` values are formatted as RFC3339
- `json.RawMessage` values are returned as-is
- Slices are joined with ", "
- Other types fall back to JSON encoding

### Examples

```
[property path="Name"]
[property path="CreatedAt"]
[property path="Description" raw=true]
```

### Available Fields by Entity Type

**Group:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `CategoryId`, `OwnerId`, `Meta`

**Resource:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `ContentType`, `OriginalFilename`, `FileSize`, `Width`, `Height`, `Meta`

**Note:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `NoteTypeId`, `OwnerId`, `StartDate`, `EndDate`, `Meta`

## `[mrql]` -- Inline Query Results

Embeds MRQL query results inline. Executes a query and renders the results in one of several formats.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `query` | Yes* | -- | MRQL query expression (e.g., `type = resource AND tags = "photos"`) |
| `saved` | Yes* | -- | Name of a saved MRQL query to execute |
| `format` | No | auto | Render format: `table`, `list`, `compact`, `custom`, or empty for auto |
| `limit` | No | `20` | Maximum number of results |
| `buckets` | No | `5` | Number of buckets for bucketed GROUP BY queries |
| `scope` | No | `"entity"` | Scope filter: `entity` (default), `parent`, `root`, `global`, or a numeric group ID |

*Either `query` or `saved` is required.

### Render Formats

**For flat queries:**

| Format | Description |
|--------|-------------|
| (empty/auto) | Tries custom templates first (if any entity has CustomMRQLResult), falls back to card layout |
| `table` | HTML table with entity type, name, and link |
| `list` | Vertical list of linked entity names |
| `compact` | Inline comma-separated links |
| `custom` | Uses each entity's CustomMRQLResult template for rendering |

**For aggregated GROUP BY queries:** Always renders as an HTML table of aggregated rows (column headers from the GROUP BY fields and aggregate functions).

**For bucketed GROUP BY queries:** Renders bucket groups, each with a header bar showing the key values and item count, followed by the items rendered using the specified format.

### Scope

The `scope` attribute limits query results to a group's subtree. By default, it scopes to the current entity's owning group:

- `entity` (default) -- the entity's owning group and its subtree
- `parent` -- the parent group's subtree
- `root` -- the root group's subtree (everything in the hierarchy)
- `global` -- no scope filter

An explicit `SCOPE` clause in the MRQL query takes precedence over the attribute.

### Nesting

`[mrql]` shortcodes can nest up to 2 levels deep. This allows CustomMRQLResult templates to contain their own `[mrql]` shortcodes. Beyond the depth limit, nested shortcodes are left as-is.

### Examples

```
[mrql query='type = resource AND tags = "photos"']
[mrql query='type = note AND created > -7d' format=table limit=10]
[mrql saved="recent-uploads" format=compact]
[mrql query='type = group AND category = 5 GROUP BY owner.name' buckets=10]
```

In a custom template:

```html
<h3>Recent Photos</h3>
[mrql query='type = resource AND contentType ~ "image/*" AND created > -30d' format=list limit=5]
```

## Plugin Shortcodes

Plugins can register custom shortcodes via the `mah.shortcode()` Lua API. Plugin shortcodes use the format:

```
[plugin:plugin-name:shortcode-name attr="value"]
```

The plugin name and shortcode name must be lowercase with only letters, digits, hyphens, and underscores.

See [Plugin Lua API](./plugin-lua-api.md#mahshortcode----custom-shortcodes) for registration details.

## Related Pages

- [Custom Templates](./custom-templates.md) -- where shortcodes are used
- [MRQL Query Language](./mrql.md) -- query language used by the `[mrql]` shortcode
- [Meta Schemas](./meta-schemas.md) -- schemas that drive `[meta]` shortcode rendering
- [Plugin Lua API](./plugin-lua-api.md) -- registering custom plugin shortcodes
