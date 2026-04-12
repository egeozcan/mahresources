# Shortcodes Reference

This file is the practical shortcode catalog for category/template work in Mahresources.

## Where Shortcodes Work

Shortcodes are currently processed in:

- `CustomHeader`
- `CustomSidebar`
- `CustomSummary`
- `CustomAvatar` where the template actually renders it
- `CustomMRQLResult`
- detail-page entity descriptions

Detail-page descriptions process shortcodes. List-preview descriptions do not.

## Core Syntax

Built-in:

```text
[meta path="status"]
[property path="Name"]
[mrql query='type = resource AND tags = "photos"' format=list]
```

Plugin:

```text
[plugin:data-views:badge path="status"]
[plugin:meta-editors:slider path="rating" min=0 max=10]
[plugin:widgets:summary]
```

Rules:

- Attributes may be double-quoted, single-quoted, or unquoted.
- Repeated attributes resolve to the last value.
- `[mrql]` nesting is capped at depth 2.
- Plugin shortcodes require the plugin to be enabled.

## Built-in Shortcodes

### `[meta]`

Use for metadata fields stored under `Meta`.

Common attrs:

- `path` -- required dot path like `status`, `timeline.start`, `address.city`
- `editable=true` -- inline edit button
- `hide-empty=true` -- remove empty output entirely

Behavior:

- Uses the current `MetaSchema` slice for schema-aware rendering.
- Supports deep paths.
- Respects labeled enums and `x-display` behavior on the resolved schema slice.
- If no schema exists, falls back to plain formatted JSON value display.

Use `[meta]` when you want the category schema to stay the source of truth for display and editing.

### `[property]`

Use for built-in entity struct fields.

Common attrs:

- `path` -- required, case-sensitive Go struct field name
- `raw=true` -- output unescaped HTML/text

Notes:

- `path` is not a meta path. It is a struct field like `Name`, `Description`, `CreatedAt`, `UpdatedAt`.
- Output is escaped unless `raw=true`.
- Times render as RFC3339.
- Slices join with `, `.

Use `[property]` when the data is not in `Meta`.

### `[mrql]`

Use to embed related collections, KPIs, and search-backed summaries.

Common attrs:

- `query` or `saved`
- `format=table|list|compact|custom`
- `limit`
- `buckets`
- `scope=entity|parent|root|global|<group-id>`

Format behavior:

- empty or auto -- tries `CustomMRQLResult` where available, otherwise default cards
- `table` -- unified result table
- `list` -- vertical linked list
- `compact` -- inline links
- `custom` -- prefer each item's `CustomMRQLResult`, fallback to default for items without one

Scope behavior:

- For Groups, `scope=entity` means the group and its subtree.
- For Resources and Notes, `scope=entity` means the owner group's subtree.
- An explicit `SCOPE` clause inside the MRQL query takes precedence.

Use `[mrql]` in `CustomHeader` or `CustomSidebar` for "recent related items", "children by type", "open tasks", or rollups.

## Built-in Plugin Shortcodes

Only three built-in plugins currently ship shortcodes:

- `data-views`
- `meta-editors`
- `widgets`

Other built-in plugins do not currently contribute shortcode families.

## `data-views`

Purpose: read-only visualization and formatting widgets.

Best fit:

- `CustomHeader`
- `CustomSidebar`
- sometimes `CustomSummary`
- occasionally `CustomMRQLResult`

Common source patterns:

- `path` for meta fields
- `field` for built-in entity properties
- `mrql` for query-backed values
- `scope` for MRQL subtree selection
- `aggregate`, `value-key`, `label-key` for MRQL or structured data

Catalog:

| Shortcode | Best use | Common attrs |
| --- | --- | --- |
| `badge` | status pill | `path`, `field`, `mrql`, `values`, `colors`, `labels` |
| `format` | money, percent, date, filesize, duration | `path`, `field`, `type`, `currency`, `decimals`, `prefix`, `suffix` |
| `stat-card` | KPI card | `path`, `field`, `label`, `type`, `icon` |
| `meter` | threshold or score gauge | `path`, `min`, `max`, `low`, `high`, `label` |
| `sparkline` | tiny trend chart | `path`, `mrql`, `type`, `height`, `width`, `color` |
| `table` | owned entities or MRQL result table | `type`, `mrql`, `cols`, `labels`, `limit` |
| `list` | array display | `path`, `mrql`, `style` |
| `count-badge` | counts of arrays or owned entities | `type`, `path`, `count-where`, `eq`, `neq`, `label`, `icon` |
| `embed` | inline text resource preview | `resource-id`, `path`, `max-lines` |
| `image` | image from meta URL or resource id | `path`, `width`, `height`, `rounded`, `alt` |
| `barcode` | code128 barcode | `path`, `field`, `mrql`, `size` |
| `qr-code` | QR code | `path`, `field`, `mrql`, `size`, `color`, `bg` |
| `link-preview` | URL card | `path`, `field`, `mrql` |
| `json-tree` | interactive nested JSON view | `path`, `expanded` |
| `bar-chart` | key/value or array chart | `path`, `mrql`, `label-key`, `value-key`, `color` |
| `pie-chart` | proportional breakdown | `path`, `mrql`, `size`, `donut`, `label-key`, `value-key`, `colors` |
| `conditional` | show content only if condition matches | `path`, `field`, `eq`, `neq`, `gt`, `lt`, `contains`, `empty`, `not-empty`, `content`, `html`, `class` |
| `timeline-chart` | date-range timeline | `type`, `mrql`, `date-path`, `start-key`, `end-key`, `name-key`, `limit` |

Tips:

- Use `badge`, `format`, `stat-card`, and `count-badge` first. They cover most polished layouts.
- `conditional` is useful when you need a simple branch but do not want Alpine.
- `json-tree` is good for admin or debugging sidebars, not summaries.
- `table` and `timeline-chart` are usually sidebar or dedicated MRQL-result tools, not header tools.

## `meta-editors`

Purpose: inline editing widgets backed by `editMeta`.

Best fit:

- `CustomSidebar`
- sometimes `CustomHeader`
- rarely `CustomSummary`

These widgets persist changes immediately or with a short debounce. Use them only in trusted internal layouts where inline editing is desirable.

Catalog:

| Shortcode | Best use | Common attrs |
| --- | --- | --- |
| `slider` | bounded numeric value | `path`, `min`, `max`, `step`, `label` |
| `stepper` | increment/decrement number | `path`, `min`, `max`, `step` |
| `star-rating` | rating field | `path`, `max` |
| `toggle` | boolean switch | `path`, `label` |
| `multi-select` | many-choice chip group | `path`, `options`, `labels` |
| `button-group` | single-choice segmented control | `path`, `options`, `labels` |
| `color-picker` | choose a hex color | `path`, `colors` |
| `tags-input` | editable string-array chips | `path`, `placeholder` |
| `textarea` | longer plain text | `path`, `rows`, `placeholder` |
| `date-picker` | date field | `path`, `label` |
| `date-range` | object with `start` and `end` | `path`, `start-label`, `end-label` |
| `status-badge` | click-to-cycle status | `path`, `options`, `colors`, `labels` |
| `progress-input` | percent bar editor | `path`, `label` |
| `key-value` | editable object of string pairs | `path` |
| `checklist` | array of `{text, done}` items | `path` |
| `url-input` | validated URL editor | `path`, `placeholder`, `label` |
| `markdown` | markdown/code textarea | `path`, `rows`, `placeholder` |

Tips:

- Use these sparingly in summaries; they are interaction-heavy.
- If the schema already gives a good structured editor on the main form, reserve meta-editors for the handful of fields people tweak often.
- For status and progress, `status-badge`, `toggle`, and `progress-input` pair well with `data-views` badges and stat cards.

## `widgets`

Purpose: dashboard-style summary blocks.

Important caveat:

- These shortcodes are most naturally group-centric because they query by the current entity id as an owner/group anchor.
- They are often useful on group categories.
- They are usually less meaningful on note types and many resource-category views.

Catalog:

| Shortcode | Best use | Common attrs |
| --- | --- | --- |
| `summary` | count dashboard for resources, notes, groups | `show`, `style` |
| `gallery` | recent owned image strip/grid | `count`, `cols`, `content-type`, `size` |
| `progress` | completion based on owned entities | `field`, `complete`, `type`, `label` |
| `activity` | recently updated owned entities | `count`, `types`, `title` |
| `tree` | group hierarchy and relatives | `depth`, `show-self`, `title` |

Tips:

- Prefer `summary` and `gallery` for visual density.
- Use `tree` only when hierarchy is central to the category.
- If the target is not group-like, prefer `data-views` over `widgets`.

## Shortcode Selection Heuristics

Use this order of preference:

1. `[meta]` for schema-aware field rendering.
2. `[property]` for built-in entity fields.
3. `data-views` for presentation widgets.
4. `meta-editors` for frequent inline edits.
5. `[mrql]` for related collections and rollups.
6. `widgets` for group dashboards.

Good combinations:

- status-heavy header: `[meta]` + `data-views:badge` + `data-views:stat-card`
- admin sidebar: `[meta editable=true]` + `meta-editors:*`
- search result card: `[property path="Name"]` + `[meta path="status"]` + `[mrql ... format=compact]`

Avoid:

- large tables in `CustomSummary`
- interaction-heavy editors in avatars
- assuming plugin shortcodes exist without mentioning plugin enablement
