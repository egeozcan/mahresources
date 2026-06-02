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

- **Built-in:** `[meta ...]`, `[property ...]`, `[mrql ...]`, `[conditional ...]...[/conditional]`
- **Plugin:** `[plugin:plugin-name:shortcode-name ...]`

Attribute values can be double-quoted, single-quoted, or unquoted. When a key appears more than once, the last value wins.

### Block Syntax

Shortcodes can also be used as paired opening/closing tags wrapping content:

```
[name attr="value"]
  content here, including HTML and other shortcodes
[/name]
```

Block shortcodes can be nested. The inner content is processed after the outer shortcode decides what to render. Not all shortcodes use block mode — each handler decides whether to use the inner content.

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

Shortcodes can nest up to 10 levels deep (the processing recursion limit). This allows CustomMRQLResult templates and block templates to contain their own shortcodes, including nested `[mrql]` queries. Beyond the depth limit, unprocessed shortcodes are left as literal text.

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

### Block Syntax

`[mrql]` supports block mode, where the inner content becomes a per-item template. Instead of choosing one of the built-in formats, you write the HTML for a single result and the query repeats it once per entity:

```
[mrql query='type = resource AND tags = "recipe"' limit="5"]
  <div class="recipe-card">
    <h3>[property path="Name"]</h3>
    <p>Cook time: [meta path="cooking.time"] min</p>
  </div>
[/mrql]
```

The block body is rendered once for each result, with that entity bound as the current context. This is the same mechanism as a category's CustomMRQLResult template, except it lives inline in the field instead of on the category.

#### What works inside the block body

Each result entity gets its own shortcode context (entity type, ID, meta, and the category's MetaSchema), so every shortcode resolves against the current item:

| Shortcode | Behavior inside the block body |
|-----------|--------------------------------|
| `[property path="..."]` | Reads a struct field off the current item (`Name`, `Description`, `ContentType`, `CreatedAt`, ...) |
| `[meta path="..."]` | Reads the current item's meta JSON, rendered schema-aware using that item's own category MetaSchema |
| `[meta path="..." editable=true]` | Edits target the current item; the pencil writes back to that specific entity |
| `[conditional ...]...[/conditional]` | Branches on the current item's meta, fields, or a nested query, including `[else]` |
| `[mrql ...]` | A nested query; `scope` keywords resolve relative to the current item (see [Nested queries and scope](#nested-queries-and-scope)) |
| `[plugin:name:shortcode ...]` | Plugin shortcodes receive the current item context |

Because the body is HTML, you can wrap shortcodes in any markup (grids, cards, badges) and Tailwind classes.

#### Precedence rules

- Block template overrides any CustomMRQLResult set on the entity's category. The inline body always wins.
- Block template overrides the `format` attribute. When a non-empty body is present, `format` is ignored (the body *is* the format). For example, `[mrql query="..." format=table]...body...[/mrql]` renders the body, not a table.
- Empty or whitespace-only blocks fall back to normal rendering. The body is trimmed first, so `[mrql query="..."][/mrql]` and `[mrql query="..."]\n[/mrql]` behave exactly like the self-closing form and honor `format` / CustomMRQLResult as usual.

#### Result modes

| Query mode | Block template behavior |
|------------|-------------------------|
| Flat (no `GROUP BY`) | Body rendered once per entity |
| Bucketed `GROUP BY` (with `buckets`) | Body rendered once per entity *within* each bucket; bucket header bars render normally |
| Aggregated `GROUP BY` | Body ignored; the aggregated table renders as usual (aggregated rows are not entities, so there is nothing to bind) |

#### Empty results

When the query matches nothing, the block renders the standard `No results.` placeholder. The `[mrql]` block has no `[else]` branch. To show a fallback message when a query is empty, wrap the block in a `[conditional mrql="..."]`:

```
[conditional mrql='type = resource AND tags = "recipe"' gt="0"]
  [mrql query='type = resource AND tags = "recipe"' limit="5"]
    <div class="recipe-card"><h3>[property path="Name"]</h3></div>
  [/mrql]
[else]
  <p class="text-stone-400">No recipes yet.</p>
[/conditional]
```

#### Combining with other attributes

Block mode composes with every non-`format` attribute. `query` or `saved`, `limit`, `buckets`, and `scope` all still apply; only `format` is superseded by the body.

```
[mrql saved="recent-uploads" limit="8" scope="root"]
  <article class="p-3 border rounded-md">
    <a href="/resource?id=[property path='ID']">[property path="Name"]</a>
    <span class="text-xs text-stone-500">[property path="ContentType"]</span>
  </article>
[/mrql]
```

#### Nested queries and scope

A nested `[mrql]` inside a block body runs in the current item's context, so its `scope` keywords resolve relative to *that* item rather than the page entity:

- `scope="entity"` (default) -- the current item's own group subtree
- `scope="parent"` -- the current item's parent group subtree
- `scope="root"` -- the root of the current item's ownership chain
- `scope="global"` -- no scope filter

This makes drill-down dashboards possible. The outer query lists groups; the inner query counts or lists their contents:

```
[mrql query='type = group AND category = 3' limit="10"]
  <section class="mb-6">
    <h3>[property path="Name"]</h3>
    <p>Resources in this group:</p>
    [mrql query='type = resource' format=compact scope="entity"]
  </section>
[/mrql]
```

Nesting is bounded by the recursion limit of 10 levels (`maxRecursionDepth`). Beyond that, unprocessed shortcodes are left as literal text.

#### Heterogeneous results

A query without a `type` filter can return mixed entity types (resources, notes, groups). The same block body is applied to every item, so reference only fields common to all of them (`Name`, `Description`, `CreatedAt`) or branch on the type first:

```
[mrql query='tags = "featured"' limit="12"]
  [conditional field="ContentType" not-empty="true"]
    <figure><img src="/v1/resource/preview?id=[property path='ID']&height=200" alt="[property path='Name']"></figure>
  [else]
    <p class="font-medium">[property path="Name"]</p>
  [/conditional]
[/mrql]
```

#### More examples

Photo gallery from a query:

```html
[mrql query='type = resource AND contentType ~ "image/*"' limit="12" scope="entity"]
  <a href="/resource?id=[property path='ID']" class="block">
    <img src="/v1/resource/preview?id=[property path='ID']&height=128"
         alt="[property path='Name']"
         class="w-full h-32 object-cover rounded-md" />
  </a>
[/mrql]
```

Bucketed by owner, each item rendered as a custom card:

```html
[mrql query='type = note GROUP BY owner.name' buckets="6"]
  <div class="py-1">
    <a href="/note?id=[property path='ID']">[property path="Name"]</a>
    [meta path="status" hide-empty=true]
  </div>
[/mrql]
```

Status board mixing meta, conditionals, and HTML:

```html
[mrql query='type = group AND category = 5' limit="20"]
  <div class="flex items-center gap-2 py-1">
    <span class="font-medium">[property path="Name"]</span>
    [conditional path="status" eq="active"]
      <span class="text-green-600 text-xs">active</span>
    [else]
      <span class="text-stone-400 text-xs">idle</span>
    [/conditional]
  </div>
[/mrql]
```

## `[conditional]` -- Conditional Display

Conditionally renders content based on a metadata value, entity field, or query result.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | No* | -- | Dot-notation path into the entity's Meta JSON |
| `field` | No* | -- | Entity struct field name (e.g., `Name`, `CreatedAt`) |
| `mrql` | No* | -- | MRQL query expression; result is used as the condition value |
| `scope` | No | `entity` | Scope for MRQL queries: `entity`, `parent`, `root`, `global` |
| `aggregate` | No | -- | Column name for aggregated MRQL results |
| `eq` | No | -- | True when value equals this string |
| `neq` | No | -- | True when value does not equal this string |
| `gt` | No | -- | True when numeric value is greater than this |
| `lt` | No | -- | True when numeric value is less than this |
| `contains` | No | -- | True when value contains this substring |
| `empty` | No | -- | True when value is nil or empty string |
| `not-empty` | No | -- | True when value is non-nil and non-empty |

*One of `path`, `field`, or `mrql` is required as the condition source.

### Condition Sources

**Path** (default): reads from the entity's meta JSON using dot-notation.

**Field**: reads a struct field from the entity object using reflection. Same fields as `[property]`.

**MRQL**: runs a query and extracts a scalar value. For flat results, the value is the item count. For aggregated results, use the `aggregate` attribute to name the column. For bucketed results, the value is the number of groups.

### Else Branch

Use `[else]` inside the block to define a fallback when the condition is false:

```
[conditional path="status" eq="active"]
  <span class="text-green-600">Active</span>
[else]
  <span class="text-stone-400">Inactive</span>
[/conditional]
```

### Nesting

Conditional blocks can be nested, and can contain any other shortcode:

```
[conditional path="status" eq="active"]
  <h3>Active Item</h3>
  [meta path="status" editable=true]
  [conditional path="priority" eq="high"]
    <span class="text-red-600">High Priority!</span>
  [/conditional]
[/conditional]
```

### Examples

```
[conditional path="featured" eq="true"]
  <span class="badge">Featured</span>
[/conditional]

[conditional path="score" gt="90"]
  <span class="text-red-600 font-bold">High score</span>
[else]
  <span class="text-stone-500">Normal</span>
[/conditional]

[conditional path="notes" not-empty="true"]
  <p>This item has notes attached.</p>
[/conditional]
```

## Plugin Shortcodes

Plugins can register custom shortcodes via the `mah.shortcode()` Lua API. Plugin shortcodes use the format:

```
[plugin:plugin-name:shortcode-name attr="value"]
```

The plugin name and shortcode name must be lowercase with only letters, digits, hyphens, and underscores.

Plugin shortcodes also support block mode:

```
[plugin:plugin-name:shortcode-name attr="value"]
  content here
[/plugin:plugin-name:shortcode-name]
```

The plugin receives `inner_content` and `is_block` in its render context. Nested shortcodes inside plugin block output are expanded automatically after the plugin returns.

Note: in docs preview, nested shortcodes inside plugin block output are not expanded (they render as literal text). This is a preview-only limitation; runtime rendering expands them normally.

See [Plugin Lua API](./plugin-lua-api.md#mahshortcode----custom-shortcodes) for registration details.

## Related Pages

- [Custom Templates](./custom-templates.md) -- where shortcodes are used
- [MRQL Query Language](./mrql.md) -- query language used by the `[mrql]` shortcode
- [Meta Schemas](./meta-schemas.md) -- schemas that drive `[meta]` shortcode rendering
- [Plugin Lua API](./plugin-lua-api.md) -- registering custom plugin shortcodes
