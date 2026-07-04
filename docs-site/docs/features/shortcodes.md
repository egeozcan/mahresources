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

- **Built-in:** `[meta ...]`, `[property ...]`, `[mrql ...]`, `[conditional ...]...[/conditional]`, `[link ...]`, `[each ...]...[/each]`, `[item ...]`, `[partial ...]`
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
| `default` | No | -- | Text rendered in place of the empty state when the value is missing. Ignored when `hide-empty` is set (hide wins) |

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
[meta path="rating" default="Unrated"]
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
| `path` | Yes | -- | Field name or dot path on the entity (e.g., `Name`, `Owner.Name`, `Tags.0.Name`) |
| `raw` | No | `false` | Skip HTML escaping; output the value verbatim |
| `default` | No | -- | Text rendered when the resolved value is empty |
| `format` | No | -- | Post-processes the value: `date`, `datetime`, `time` (time fields), or `filesize` (integer byte counts) |
| `layout` | No | -- | Custom Go time layout for time fields (e.g., `Jan 2, 2006`); wins over `format` |

### How It Works

- Accesses the field using Go reflection on the entity struct
- Output is HTML-escaped by default for safety
- `time.Time` values are formatted as RFC3339 unless `format`/`layout` is set
- `json.RawMessage` values are returned as-is
- Slices are joined with ", "
- Other types fall back to JSON encoding

### Dot-path Traversal

`path` may traverse into related structs and slices with dot notation:

- `Owner.Name` follows a related struct one hop.
- `Tags.0.Name` indexes into a slice (a purely numeric segment); an out-of-range index renders empty.
- A `nil` pointer, missing field, or out-of-range index anywhere along the path renders empty (or the `default`).

The shortcode never triggers database loads by design (list pages render many cards). Related structs resolve only where the page already preloaded them — detail pages preload `Owner`; card contexts may not. When a related struct is not loaded, the path renders empty.

### Formatting

- `format="date"` → `2006-01-02`, `format="datetime"` → `2006-01-02 15:04`, `format="time"` → `15:04` for `time.Time` fields; non-time values pass through unchanged.
- `layout="..."` applies a custom Go time layout and takes precedence over `format`.
- `format="filesize"` humanizes integer byte counts (e.g. `1.5 KB`).
- Unknown `format` values pass the text through unchanged (never an error).

### Examples

```
[property path="Name"]
[property path="CreatedAt" format="date"]
[property path="Owner.Name" default="Unassigned"]
[property path="FileSize" format="filesize"]
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
| `value` | No | -- | [Inline scalar mode](#inline-scalar-value): renders a single escaped value with no wrapper. `count` for the result count, or a column name from an aggregated result. Conflicts with a block body |
| `format` | No | auto | Render format: `table`, `list`, `compact`, `custom`, or empty for auto. With `value=`, formats the scalar like `[property]` (`date`/`datetime`/`time`/`filesize`) |
| `layout` | No | -- | Custom Go time layout for a `value=` time scalar (e.g., `Jan 2, 2006`). Wins over `format` |
| `limit` | No | `20` | Maximum number of results |
| `buckets` | No | `5` | Number of buckets for bucketed GROUP BY queries |
| `scope` | No | `"entity"` | Scope filter: `entity` (default), `parent`, `root`, `global`, or a numeric group ID |
| `link-all` | No | `false` | Appends a default "View all →" link to the `/mrql` page for this query (see [Totals and the view-all link](#totals-and-the-view-all-link)) |

*Either `query` or `saved` is required.

### Inline scalar value

`value=` turns `[mrql]` into an inline scalar: it renders a single escaped text value with **no wrapper `<div>`**, usable mid-sentence.

```html
<p>You have <strong>[mrql query="resources" value="count"]</strong> files.</p>
```

- `value="count"` -- the flat item count, the bucket count (bucketed), or the row count (aggregated).
- `value="<column>"` -- `Rows[0][<column>]` from an aggregated result (the same contract as `aggregate=` on `[conditional]`). A column has no meaning on a non-aggregated result and renders empty.
- `format=` / `layout=` post-process the value exactly like `[property]` (e.g. `format="filesize"` on a byte count, `format="date"` on a timestamp column).
- Errors render as an inline `<span class="mrql-error">` rather than a block `<div>`, so they don't break the surrounding line.
- A `value=` shortcode with a block body is a lint error; the body is ignored at render time.

:::caution `value="count"` is capped by `limit`
`value="count"` counts the **returned** rows, so it is bounded by `limit` (default 20). For a true total, use an aggregated `count()` query (`... GROUP BY ... count()` with `value="<count column>"`) or the `{total}` placeholder in a [block slot](#totals-and-the-view-all-link).
:::

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
| `[conditional ...]...[/conditional]` | Branches on the current item's meta, fields, or a nested query, including `[elseif ...]` and `[else]` |
| `[link to="..."]` | Resolves a detail-page URL for the current item (`self`, `owner`, `root`, `category`) |
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

#### Header, footer, and empty (`[else]`) slots

A block body may carry three optional slots alongside the per-item template, using literal tags handled locally by `[mrql]` (like `[else]` inside `[conditional]` -- they carry meaning only inside an `[mrql]` block):

```
[mrql query='notes WHERE tags = "todo"' limit="10"]
  [header]<h4>Open TODOs ({count} of {total})</h4>[/header]
  <li>[property path="Name"]</li>
  [footer]<p class="text-xs">updated live</p>[/footer]
[else]
  <p>Nothing to do 🎉</p>
[/mrql]
```

- **`[header]` / `[footer]`** render **once**, wrapped around the results, with the parent (page) entity as context -- not per item. The first occurrence of each is used; a `[header]`/`[footer]` nested inside another block is left untouched.
- **`[else]`** is the complete empty-state output. When the result has no rows (no items, no buckets, or no aggregated rows), only the `[else]` branch renders -- header and footer are suppressed. Without an `[else]`, an empty result still shows the standard `No results.` placeholder.
- The remaining content (after the slots are removed) is the per-item template, exactly as before.

Wrapping the block in a `[conditional mrql="..."]` still works and remains useful when the fallback needs to live outside the `[mrql]` wrapper.

#### Totals and the view-all link

Header, footer, and `[else]` slots substitute three placeholders **before** their content is processed:

| Placeholder | Expands to |
|-------------|------------|
| `{count}` | The number of rendered rows (items, buckets, or aggregated rows) -- capped by `limit` |
| `{total}` | The true total ignoring `limit`. Its presence anywhere in a slot triggers a second `COUNT` query over the same filter and scope; without it, no count query runs. Falls back to `{count}` for grouped/aggregated queries |
| `{link-all}` | The bare `/mrql` URL that reproduces this query (for custom markup) |

Set `link-all="true"` to append a default **"View all →"** link after the results (before a custom `[footer]`):

```
[mrql query='resources WHERE tags = "photo"' limit="6" link-all="true"]
  <li>[property path="Name"]</li>
[/mrql]
```

The link points at the `/mrql` page and always reproduces the same result set, scope included:

- **Unscoped saved queries** link by ID (`/mrql?saved=<id>`), preserving the saved-query identity (the name→ID lookup is resolved server-side).
- **Inline queries** link by their text (`/mrql?q=<query>`). When the shortcode applied a scope (via `scope=` or the default entity scope) and the query has no explicit `SCOPE` clause, a `SCOPE <id>` clause is spliced in at the correct position (before `GROUP BY` / `ORDER BY` / `LIMIT` / `OFFSET`) so the query stays valid.
- **Scoped saved queries** link by text as well (`/mrql?q=…`), because `/mrql?saved=<id>` would open the query globally and lose the scope. The saved-query identity is traded for a correct, scoped result set.
- Parameterized (`param-*`) queries link with their `$placeholders` unbound; the `/mrql` page renders inputs for the user to fill.

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
| `gte` | No | -- | True when numeric value is greater than or equal to this |
| `lte` | No | -- | True when numeric value is less than or equal to this |
| `in` | No | -- | True when value equals one of a comma-separated list (e.g. `in="a,b,c"`) |
| `contains` | No | -- | True when value contains this substring |
| `matches` | No | -- | True when value matches this Go regular expression. An invalid pattern evaluates to false |
| `empty` | No | -- | True when value is nil or empty string |
| `not-empty` | No | -- | True when value is non-nil and non-empty |
| `combine` | No | `all` | How to fold multiple operators and numbered-suffix conditions: `all` (AND) or `any` (OR) |

*One of `path`, `field`, or `mrql` is required as the condition source.

### Multiple Operators and Conditions

When more than one operator is present on the same tag, **every operator must pass** (AND). This makes natural ranges easy:

```
[conditional path="score" gte="1" lte="10"]In range[/conditional]
```

Set `combine="any"` to OR across the present operators instead.

For conditions on *different* values, add numbered-suffix sources and operators (`path2`, `field2`, `mrql2`, `eq2`, `gte2`, …). Each numbered group is an additional condition, folded with the same `combine` mode (default AND). The loop stops at the first suffix with no source:

```
[conditional path="status" eq="active" path2="score" gte2="5"]
  Active and scoring
[/conditional]
```

Nesting `[conditional]` blocks remains the readable way to AND several conditions; the numbered suffixes mainly exist to make OR across values expressible.

### Condition Sources

**Path** (default): reads from the entity's meta JSON using dot-notation.

**Field**: reads a struct field from the entity object using reflection. Same fields as `[property]`.

**MRQL**: runs a query and extracts a scalar value. For flat results, the value is the item count. For aggregated results, use the `aggregate` attribute to name the column. For bucketed results, the value is the number of groups.

### Else and Elseif Branches

Use `[else]` inside the block to define a fallback when the condition is false:

```
[conditional path="status" eq="active"]
  <span class="text-green-600">Active</span>
[else]
  <span class="text-stone-400">Inactive</span>
[/conditional]
```

Use `[elseif ...]` dividers to chain additional conditions. Each `[elseif]` carries its own condition attributes (the same set as the opening tag). The first matching branch renders; `[else]` matches unconditionally:

```
[conditional path="tier" eq="gold"]
  Gold
[elseif path="tier" eq="silver"]
  Silver
[elseif path="tier" eq="bronze"]
  Bronze
[else]
  Basic
[/conditional]
```

`[elseif]` and `[else]` dividers nested inside an inner `[conditional]` block belong to that inner block, not the outer one.

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

## `[link]` -- Detail-page URLs

Resolves a detail-page URL for the current entity or a related target.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `to` | No | `self` | Link target: `self`, `owner`, `root`, or `category` |

### Targets

- `self` (default) — the current entity's detail page (`/group?id=`, `/resource?id=`, `/note?id=` by entity type).
- `owner` — the owning group (`/group?id=`). For resources and notes this is their group; for groups it is the parent group.
- `root` — the root of the ownership chain (`/group?id=`).
- `category` — the entity's category/type page (`/category?id=`, `/resourceCategory?id=`, `/noteType?id=`).

### Inline vs Block

- **Inline** (`[link to="..."]`) renders just the URL, HTML-escaped, so you can write it inside an `href`:

  ```html
  <a href="[link]" class="underline">Open</a>
  ```

- **Block** (`[link to="..."]inner[/link]`) renders a full anchor around its processed inner content:

  ```
  [link to="owner"]Back to group[/link]
  ```

When the target cannot be resolved (unknown `to`, an unset category, or an owner/root that is not resolvable), the inline form renders nothing and the block form renders its inner content without a wrapping anchor — never a link to a placeholder ID.

### Examples

```
<a href="[link]" class="btn">This page</a>
[link to="owner"]Back to group[/link]
[link to="category"]View type[/link]
```

## `[each]` -- Iterate an array

Renders its inner content once per element of an array meta value.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | | Dot-notation path to an array in the entity meta, e.g. `ingredients` |
| `limit` | No | `100` | Maximum number of elements to render |

### How It Works

`[each]` is a block shortcode. Reference the current element with `[item]` inside the block. A non-array or empty value renders the `[else]` branch, or nothing when there is no `[else]`. Inner `[meta]`, `[conditional]`, `[mrql]`, and `[property]` shortcodes run against the **parent entity**, not the element — use `[item]` for element data.

### `[item]`

`[item]` renders the current element inside an `[each]` block. It uses the same `format`, `layout`, and `default` helpers as `[property]`, and is HTML-escaped unless `raw="true"`. Outside an `[each]` block it renders nothing.

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | No | | Dot-path into the current element when it is an object, e.g. `name`. Omit to render a scalar element directly |
| `index` | No | `false` | When `true`, renders the element's 1-based position instead of its value |
| `format` | No | | `date`/`datetime`/`time` for time values; `filesize` for byte counts |
| `layout` | No | | Custom Go time layout for time values (wins over `format`) |
| `default` | No | | Text rendered when the resolved value is empty |
| `raw` | No | `false` | When `true`, output is not HTML-escaped |

### Examples

```
[each path="tags"]
  <span class="badge">[item]</span>
[/each]

[each path="ingredients"]
  <li>[item index="true"]. [item path="name"] — [item path="qty" default="?"]</li>
[else]
  <p>No ingredients.</p>
[/each]
```

`[item]` binds to the nearest enclosing `[each]`; `[item]` tokens inside a nested `[each]` belong to that inner loop.

## `[partial]` -- Reusable snippets

Expands a reusable template partial by name. Partials are managed under **Template Partials** (admin only) and referenced from any category-template slot.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `name` | Yes | | Kebab-case name of the partial to expand, e.g. `status-badge` |

### How It Works

The partial's content is rendered with the **current entity context**, so its own `[meta]`, `[conditional]`, `[mrql]`, and `[each]` shortcodes resolve against the entity that includes it. An unknown name renders an HTML comment (`<!-- partial "x" not found -->`) rather than leaking the raw shortcode. Self- and mutually-referential partials terminate at the recursion depth limit.

### Example

```
[partial name="status-badge"]
```

See [Custom Templates](./custom-templates.md) for authoring partials, bundles, and presets.

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
