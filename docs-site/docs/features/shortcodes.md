---
sidebar_position: 17
title: Shortcodes
---

# Shortcodes

Shortcodes are bracket-delimited expressions embedded in the custom template slots of a Category, Resource Category, or Note Type, which expand into dynamic HTML at render time. See [Custom Templates](./custom-templates.md) for the full slot list. They provide schema-aware metadata display, inline query results, and entity property access without writing Alpine.js or Pongo2 code.

## Syntax

```
[name attr="value" attr2="value2"]
```

The parser recognizes these patterns:

- **Built-in inline:** `[meta ...]`, `[datetime ...]`, `[pills ...]`, `[property ...]`, `[item ...]`, `[partial ...]`
- **Built-in block:** `[conditional ...]...[/conditional]`, `[each ...]...[/each]`, `[lazy]...[/lazy]`, `[details ...]...[/details]`, `[using ...]...[/using]`
- **Built-in inline or block:** `[mrql ...]` or `[mrql ...]...[/mrql]`, `[link ...]` or `[link ...]...[/link]`, `[reload ...]` or `[reload ...]...[/reload]`
- **Plugin:** `[plugin:plugin-name:shortcode-name ...]`, inline or as a block closed by `[/plugin:plugin-name:shortcode-name]`

Attribute values can be double-quoted, single-quoted, or unquoted. When a key appears more than once, the last value wins.

### Quoting

- Wrap a value in the quote character it does not contain. `query='type = resource AND tags = "photo"'` is valid; a value cannot contain its own delimiter.
- There is no backslash escape. `path="name = \"x\""` is read as `name = \`, so switch to single quotes instead.
- Quote every value that contains a space. An unquoted value stops at the first space and the rest is dropped without an error: `[meta path=cooking time]` reads only `path=cooking`.
- A `]` inside quotes does not end the shortcode: `[mrql query='type = note AND name ~ "a]b"']` is one shortcode.

```
[meta path="cooking.difficulty" default="Not rated"]
[meta path='cooking.time' default='Not set']
[mrql query='type = note AND name ~ "draft"' limit="5"]
```

### Built-in Shortcodes

| Shortcode | Form | Use |
|-----------|------|-----|
| `[meta]` | Inline | Render a value from the current entity's Meta JSON |
| `[pills]` | Inline | Pill selector with schema enum or manual options |
| `[datetime]` | Inline | Display/edit a date/time Meta field, preserving its format |
| `[property]` | Inline | Render a scalar field or dot-path from the current entity |
| `[mrql]` | Inline or block | Run an MRQL query and render results, a scalar value, or a custom item template |
| `[conditional]` | Block | Render the first matching branch from a meta, field, or MRQL condition |
| `[link]` | Inline or block | Resolve a detail-page URL or wrap content in a link |
| `[each]` | Block | Iterate an array in Meta |
| `[item]` | Inline | Render the current element inside an `[each]` block |
| `[partial]` | Inline | Expand a named Template Partial |
| `[lazy]` | Block | Defer server rendering until the block scrolls into view |
| `[details]` | Block | Defer server rendering until a disclosure is opened |
| `[reload]` | Inline or block | Re-render the nearest deferred block or custom-content slot on demand |
| `[using]` | Block | Render the body with another group, resource or note as the entity |

### Block Syntax

Shortcodes can also be used as paired opening/closing tags wrapping content:

```
[name attr="value"]
  content here, including HTML and other shortcodes
[/name]
```

Block shortcodes can be nested. The inner content is processed after the outer shortcode decides what to render. Not all shortcodes use block mode -- each handler decides whether to use the inner content.

## Processing

Shortcodes are processed via the `process_shortcodes` Pongo2 template tag. Every `Custom*` slot processes shortcodes automatically: `CustomLightbox` is expanded before the JSON detail response is serialized, and `CustomCSS` is expanded by the `custom_css` tag. Entity description fields also process shortcodes on detail pages; truncated previews in list views do not.

A `CustomCSS` slot is expanded once per category per page. Its shortcodes resolve against the first entity of that category that the page renders: the entity being viewed on a detail page, or the first matching entity in a list. A stylesheet therefore cannot vary per card on a list page, so keep shortcodes in CSS for values that are the same across the category.

### Failure markers

A shortcode that cannot be expanded never leaks its raw `[…]` source. Instead, the rendered page carries a diagnosable marker:

- **A failing plugin shortcode** renders an inline `<span class="shortcode-error" title="…">⚠ plugin:name:shortcode</span>` with the error in the `title` attribute.
- **An unclosed `[conditional]`** (used without `[/conditional]`) renders the same inline marker -- it can't gate anything as written. The template lint flags this at edit time.
- **Structural stops** render an HTML comment rather than visible noise: the recursion depth cap emits `<!-- mr:shortcode depth limit reached -->`, and a context that deliberately wires no query executor or plugin renderer emits `<!-- mr:mrql unavailable in this context -->` / `<!-- mr:using unavailable in this context -->` / `<!-- mr:plugin unavailable in this context -->`.

A `<!-- mr: ... -->` comment means expansion stopped on purpose, either because the surface has no query or plugin support or because the depth cap was reached. It is not a failed query. A red `⚠` marker means a plugin shortcode or a `[reload]` failed, and its `title` attribute says why. A failing `[mrql]` shows a red `mrql-error` message instead.

## `[meta]` -- Metadata Display

Renders a metadata field from the entity's `meta` JSON, using the category's MetaSchema for type-aware display.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | -- | Dot-notation path into the entity's Meta JSON (e.g., `cooking.time`, `address.city`) |
| `inline` | No | `false` | Renders the bare value instead of the `<meta-shortcode>` element. The only form usable inside an HTML attribute, and it emits the stored value rather than the element's schema-aware display (see [Inline values](#inline-values-inside-html-attributes)) |
| `editable` | No | `false` | Shows a pencil edit button; clicking opens a schema-aware inline form. Ignored when `inline` is set |
| `hide-empty` | No | `false` | Hides the shortcode entirely when the value is absent or null |
| `default` | No | -- | Text rendered in place of the empty state when the value is missing. Ignored when `hide-empty` is set (hide wins) |
| `raw` | No | `false` | With `inline`, skips HTML escaping -- the same meaning `raw` has on `[property]` and `[item]`. No effect without `inline` |
| `format` | No | -- | With `inline`, formats the value like `[property]`: `date` / `datetime` / `time` for timestamps, `filesize` for byte counts. No effect without `inline` |
| `layout` | No | -- | With `inline`, a custom Go time layout (e.g. `Jan 2, 2006`). Wins over `format`. No effect without `inline` |

### How It Works

- Expands into a `<meta-shortcode>` web component at render time
- Client hydrates using the schema-editor rendering pipeline
- When `editable=true`, clicking the pencil calls the `editMeta` API endpoint
- If the path exists in the MetaSchema, rendering is schema-aware (type formatting, enum pills, shape detection, x-display)
- If no schema exists, falls back to plain value display
- All of that is the element's doing, in the browser. `inline="true"` emits no element and applies none of it (see [Inline output is the stored value](#inline-output-is-the-stored-value))

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

### Inline values inside HTML attributes

The default form expands to a `<meta-shortcode>` **element**, which cannot go inside an HTML attribute: an element nested in an attribute value is broken markup, and the element's own quotes close the attribute early. `inline="true"` renders the bare value instead, which is what an attribute needs.

```html
<a href="/archive/[meta path='slug' inline='true']"
   title="[meta path='blurb' inline='true']">Open</a>

<div class="card" data-status="[meta path='status' inline='true' default='none']">…</div>
```

Note the single quotes on the inner attributes: shortcode attributes accept `key='value'` as well as `key="value"`, so single quotes keep the outer HTML attribute intact.

**Inline output is HTML-escaped**, quotes included, so a Meta value containing a `"` cannot break out of a **quoted** attribute. `raw="true"` turns escaping off for the cases where the value really is markup -- never use it inside an attribute.

That guarantee stops where the browser re-parses the value. Escaping covers `&`, `<`, `>`, `'` and `"` and nothing else, so every shape below remains unsafe and the editor warns on each:

| Shape | Why escaping does not help |
|---|---|
| `<a href="[meta path='u' inline='true']">` | A value of `javascript:alert(1)` runs on click. Put a scheme or path in front: `href="/x/[meta …]"` |
| `<a href=/x/[meta path='u' inline='true']>` | Unquoted: a value containing a space adds attributes of its own (`x onfocus=alert(1)`). Quote it |
| `<button onclick="f('[meta path='u' inline='true']')">` | The HTML parser decodes `&#39;` back to `'` before the script is parsed, so a quote in the value escapes the JS string. Do not interpolate Meta into a handler |
| `<div style="color:[meta path='u' inline='true']">` | CSS injection; escaping does not apply to CSS syntax |
| `<iframe srcdoc="… [meta path='u' inline='true']">` | The browser decodes `srcdoc` and parses it as a document, so escaping buys nothing anywhere in the value |
| anything with `raw="true"` | Nothing is escaped at all -- in an attribute the value can close it, and in ordinary text a value like `<img src=x onerror=…>` becomes a real element |
| a Custom CSS slot | The whole slot is a stylesheet, so `;` and `}` in a value start new declarations. The editor knows because it sends the slot's language along with the content |
| `<script>… [meta path='u' inline='true'] …</script>` | A script body decodes no entities, so the value reaches JavaScript verbatim -- `${…}` in a template literal is not escaped at all. Pass it in through a `data-` attribute instead |
| `<style>… [meta path='u' inline='true'] …</style>` | Likewise for CSS: `;` and `}` are untouched |
| `<div @click="f('[meta path='u' inline='true']')">` | Alpine evaluates a directive's value as JavaScript after the parser has decoded the escaping, exactly as `on*` does. `x-*`, `@*` and a leading `:` are all directives |
| `<div data-[meta path='u' inline='true']="x">` | Interpolating a *name* -- nothing delimits it, so a space or `=` in the value adds attributes |

This matters because the two halves have different authors: an admin or editor writes the template, but the Meta value it interpolates is written by anyone who can edit the entity -- which includes the plain `user` role.

`format` and `layout` work here as they do on `[property]` and `[item]`, including on the string form a timestamp takes in JSON:

```html
[meta path="published" inline="true" format="date"]     <!-- 2026-08-22 -->
[meta path="published" inline="true" layout="Jan 2, 2006"]
[meta path="filesize" inline="true" format="filesize"]  <!-- 2.0 KB -->
```

`editable` is ignored under `inline`: the output is text, so there is nothing to edit, and honouring it would put an edit affordance inside whatever attribute you were building.

Inline mode also works where the widget does not hydrate -- public share pages and `CustomMRQLResult` cards -- because it is plain server-rendered text.

The alternative, for a slot that runs in a browser with the entity in scope, is Alpine: every entity-bound slot is wrapped in `x-data="{ entity: … }"`, so `:href="'/archive/' + entity.Meta.slug"` reads the same value client-side.

### Inline output is the stored value

Everything under [How It Works](#how-it-works) is the `<meta-shortcode>` element's doing, in the browser. `inline="true"` emits no element, so none of it applies. What it renders is the stored value as plain text, shaped by `format` / `layout` and by the `default` / `hide-empty` fallbacks, then escaped unless `raw="true"`.

With a MetaSchema that declares `status` as a labeled enum (`in_progress` labelled `In Progress`):

```html
[meta path="status"]                        <!-- In Progress, in an enum pill -->
[meta path="status" inline="true"]          <!-- in_progress -->
```

The same split applies to the element's other display rules. The element stops at the first rule that matches the field, so each row below is what that rule produces when it is the one that fires. Inline reaches none of them.

| Rule | Element form | `inline="true"` |
|---|---|---|
| `x-display` naming a built-in renderer | The renderer's output | The value, renderer not run |
| `x-display` naming a `plugin:` type | The plugin's output | The value, renderer not run |
| An object matching a shape (`url`, `geo`, `daterange`, `dimensions`) | The shape's rendering | The value's JSON, no detection |
| A [labeled enum](./meta-schemas.md#type-aware-rendering) (`oneOf` + `const` + `title`) | The matching entry's `title`, or the stored value when that entry has no `title` and when no entry matches | The stored value |
| A plain `enum` list | The value, in a pill | The value |
| `"type": "boolean"` | `Yes` / `No` | `true` / `false` |
| No rule matched, object value | The value's JSON | The value's JSON |
| No rule matched, array value | The array's JSON | The elements joined with `, ` |

Precedence among those rules has one wrinkle worth knowing when you write a schema. The element checks for an enum twice: once against the schema as written, before it consults a `plugin:` renderer, and once against the schema with composition resolved, after. So an enum you declare on the field itself preempts a `plugin:` renderer, while one that only becomes visible once a `$ref`, `allOf`, `oneOf` or `anyOf` has been resolved does not. A built-in `x-display` declared on the field, and a matching object shape, are both checked ahead of either enum pass. None of it reaches inline output either way.

**This is what `inline` is for, not a gap in it.** Its main use is building an attribute or a URL, and there the stored value is the one that has to travel. `data-status="[meta path='status' inline='true']"` hands a CSS rule or a script the key it was written against, `in_progress`, and not a label an operator can rename in the schema without touching the template. Resolving labels here would break the mode's primary use.

So pick the form by what consumes the output. Use `[meta path="status"]` where a reader sees the value and wants the label. Use `inline="true"` where markup, a URL or a script consumes it. Use both where a card needs each:

```html
<div class="card" data-status="[meta path='status' inline='true']">
  Status: [meta path="status"]
</div>
```

## `[pills]` -- Pill Selector

Use this in a category's Custom Header, Sidebar, or another entity template slot:

```html
[pills path="priority" editable="true"]
```

The selector gets its choices from the field's MetaSchema `enum`, or from a labeled `oneOf` enum (`const` values with `title` labels). A selected entry's `x-color` supplies its background tint and dark text color; entries without a valid color use the default purple styling. It also works in Resource Category and Note Type templates. Selecting a pill saves immediately and updates other metadata shortcodes for the same entity. Arrow keys, Home, and End select choices; Tab enters and leaves the selector.

Supply `options` to override the schema choices, or to use a field without a schema:

```html
[pills path="priority" editable="true" options='["Low","Medium","High"]']
[pills path="priority" editable="true" options='[{"value":1,"label":"Low"},{"value":2,"label":"Medium"},{"value":3,"label":"High"}]']
```

A read-only selector shows the stored value without saving anything, and `hide-empty` removes the selector when nothing is stored yet:

```html
[pills path="status"]
[pills path="rating" options='[1,2,3,4,5]' hide-empty="true"]
```

Manual options are a JSON array of strings, numbers, booleans, or null, or objects with `value` and `label`. Stored values retain their JSON types: numeric `1` differs from string `"1"`. Labels are plain text. Manual options still must satisfy any schema validation when saved.

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | — | Dot-notation Meta field path |
| `editable` | No | `false` | Save immediately on selection; otherwise show a read-only selector |
| `options` | No | Schema choices | JSON array of values or `{value, label}` objects; overrides schema choices |
| `hide-empty` | No | `false` | Hide the entire selector when the value is empty; leave false to fill an empty field |

Empty fields start with no selection and are only written when a pill is selected. A stored value outside the choices is shown alongside the selector. Missing or invalid options show “No options configured”; failed saves keep the previous selection and offer a retry message. Public note shares always render read-only.

## `[datetime]` -- Date/Time Display and Editor

Use `[datetime path="event.start" editable="true"]` to display a Meta date/time field with an optional native picker and Save/Cancel controls. Without `editable`, it is display-only.

The existing value determines the format: ISO dates, times, and timestamps retain their separator, seconds, fractional precision, and timezone offset. The picker edits the stored wall-clock time without converting it to the browser's timezone. If the current value is invalid, it displays verbatim and the editor uses the field's JSON Schema `default`. Without a valid default, the editor is empty and its input type follows the schema's `format` (`date`, `time`, or `date-time`). Opening or cancelling never writes the fallback.

| Attribute | Meaning |
|-----------|---------|
| `path` | Required dot path into Meta |
| `editable` | Enable editing; defaults to `false` |
| `layout` | Custom display layout; defaults to the stored format |
| `input-layout` | Custom format for parsing and saving the stored string |
| `default` | Display text for an empty value; editor defaults come from the schema |
| `hide-empty` | Hide empty values; defaults to `false` |

```text
[datetime path="event.start" layout="January 2, 2006 at 15:04"]
[datetime path="deadline" editable="true" input-layout="02/01/2006" layout="Jan 2, 2006"]
[datetime path="openingTime" editable="true"]
[datetime path="event.start" default="No date set" editable="true"]
[datetime path="followUp" editable="true"]
```

`default` is shown when the stored value is empty. `hide-empty` hides an empty value outside edit mode, so an optional date does not leave a placeholder on every card. Do not combine `hide-empty` with `editable` on the same field: an empty field then has nothing to click, so it can never be filled in from that slot.

Layouts use these Go reference-time tokens: `2006` (year), `January`/`Jan`/`01`/`1` (month), `02`/`2` (day), `15` (24-hour hour), `03`/`3` (12-hour hour), `04`/`4` (minute), `05`/`5` (second), `PM`/`pm`, `.000` (fraction, one to nine zeros), and `Z07:00`/`-07:00`/`Z0700`/`-0700` (offset). Other characters are literal. Use `input-layout` for non-ISO values, including ambiguous day/month formats. `layout` only changes the display, never the saved representation. The native picker edits fractions up to milliseconds; opening and saving without changing the input preserves all stored fractional digits.

Public share pages render this shortcode read-only, as with `[meta]`.

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
- Types with a `String()` method, such as group `URL`, render through that method
- Other types fall back to JSON encoding

### Dot-path Traversal

`path` may traverse into related structs and slices with dot notation:

- `Owner.Name` follows a related struct one hop.
- `Tags.0.Name` indexes into a slice (a purely numeric segment); an out-of-range index renders empty.
- A `nil` pointer, missing field, or out-of-range index anywhere along the path renders empty (or the `default`).

The shortcode never triggers database loads by design (list pages render many cards). Related structs resolve only where the page already preloaded them -- detail pages preload `Owner`; card contexts may not. When a related struct is not loaded, the path renders empty.

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
[property path="Tags.0.Name" default="No tags"]
[property path="CreatedAt" layout="2 Jan 2006"]
[property path="Description" default="No description"]
```

`raw=true` skips escaping, so a value that contains HTML is rendered as HTML. Use it only for text an admin or editor wrote, and never for a value that a plain `user` can edit. The linter warns on every `raw=true`.

### Available Fields by Entity Type

`[property]` reflects over the model, so any exported field resolves. These are the commonly used ones rather than an allow-list.

**Group:** `ID`, `Name`, `Description`, `URL`, `CreatedAt`, `UpdatedAt`, `CategoryId`, `OwnerId`, `Meta`

**Resource:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `ContentType`, `OriginalName`, `OriginalLocation`, `FileSize`, `Width`, `Height`, `Hash`, `ResourceCategoryId`, `OwnerId`, `Meta`

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
| `param-*` | No | -- | Wildcard family that binds an MRQL `$name` placeholder, e.g. `param-tag="x"` fills `$tag`, `param-since="-7d"` fills `$since` |

*Either `query` or `saved` is required.

### Inline scalar value

`value=` turns `[mrql]` into an inline scalar: it renders a single escaped text value with **no wrapper `<div>`**, usable mid-sentence.

```html
<p>Uploads this week: <strong>[mrql query="type = resource AND created > -7d" value="count" limit="100"]</strong> (at most 100 are counted)</p>
```

Pick the row a scalar reads with `ORDER BY` and `LIMIT 1` on an aggregated query. This gives the largest file size across all resources, in bytes:

```html
<p>Largest file: [mrql query="type = resource GROUP BY contentType MAX(fileSize) ORDER BY max_fileSize DESC LIMIT 1" value="max_fileSize"] bytes</p>
```

- `value="count"` -- the flat item count, the bucket count (bucketed), or the row count (aggregated).
- `value="<column>"` -- `Rows[0][<column>]` from an aggregated result (the same contract as `aggregate=` on `[conditional]`). A column has no meaning on a non-aggregated result and renders empty.
- `format=` / `layout=` post-process the value exactly like `[property]` (e.g. `format="filesize"` on a byte count, `format="date"` on a timestamp column).
- Errors render as an inline `<span class="mrql-error">` rather than a block `<div>`, so they don't break the surrounding line.
- A `value=` shortcode with a block body is a lint error; the body is ignored at render time.

:::caution `value="count"` is capped by `limit`
`value="count"` counts the **returned** rows, so it is bounded by `limit` (default 20). For a true total, use the `{total}` placeholder in a [block slot](#totals-and-the-view-all-link). Do not try an aggregated `COUNT()` with `value=`: its output key is `count`, and `value="count"` reads the number of rows, so you get the number of groups instead of the count.
:::

### Render Formats

**For flat queries:**

| Format | Description |
|--------|-------------|
| (empty/auto) | Tries custom templates first (if any entity has CustomMRQLResult), falls back to card layout |
| `table` | HTML table with columns for name (linked), entity type, and description |
| `list` | Vertical list of linked entity names, each followed by its description when present |
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

### Per-page query budget

Because a category's `Custom*` templates render once per card, an entity-scoped `[mrql]` in a `CustomSummary` runs **one query per card** -- so a list page of many cards can execute many queries. Identical queries within a single render are deduplicated by a per-page cache and cost nothing; each distinct query counts against the per-page budget (`-mrql-page-query-budget`, default 200). Once the budget is spent, further distinct `[mrql]` queries render the standard error box ("inline query budget exceeded (N per page)…") instead of executing, and one warning per page is logged. Raise the flag if a legitimately dense page trips it, or set `0` to disable. See [Advanced configuration](../configuration/advanced.md#inline-mrql-page-query-budget).

### Nesting

Shortcodes can nest up to 10 levels deep (the processing recursion limit). This allows CustomMRQLResult templates and block templates to contain their own shortcodes, including nested `[mrql]` queries. Beyond the depth limit, the remaining content is emitted as-is with a trailing `<!-- mr:shortcode depth limit reached -->` comment so authors can see where expansion stopped.

### Examples

```
[mrql query='type = resource AND tags = "photos"']
[mrql query='type = note AND created > -7d' format=table limit=10]
[mrql saved="recent-uploads" format=compact]
[mrql saved="recent-uploads" link-all="true" limit="5"]
[mrql query='type = resource GROUP BY meta.camera_model' buckets=6]
[mrql query='type = note AND tags = $tag' param-tag="todo" limit=5]
```

`param-*` binds a `$name` placeholder, so one saved query can serve many shortcodes with different values. Without the binding, the shortcode renders an MRQL error (`missing parameter $tag`) in place of its results.

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
[mrql query='type = note AND tags = "todo"' limit="10"]
  [header]<h4>Open TODOs ({count} of {total})</h4>[/header]
  <li>[property path="Name"]</li>
  [footer]<p class="text-xs">updated live</p>[/footer]
[else]
  <p>Nothing to do 🎉</p>
[/mrql]
```

- **`[header]` / `[footer]`** render **once**, wrapped around the results, with the parent (page) entity as context -- not per item. The first occurrence of each is used; a `[header]`/`[footer]` nested inside another block is left untouched.
- **`[else]`** is a divider with no closing tag (never write `[/else]`; it would render as literal text). It is the complete empty-state output. When the result has no rows (no items, no buckets, or no aggregated rows), only the `[else]` branch renders -- header and footer are suppressed. Without an `[else]`, an empty result still shows the standard `No results.` placeholder.
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
[mrql query='type = resource AND tags = "photo"' limit="6" link-all="true"]
  <li>[property path="Name"]</li>
[/mrql]
```

The link points at the `/mrql` page and always reproduces the same result set, scope included:

- **Unscoped saved queries** link by ID (`/mrql?saved=<id>`), preserving the saved-query identity (the name→ID lookup is resolved server-side).
- **Inline queries** link by their text (`/mrql?q=<query>`). When the shortcode applied a scope (via `scope=` or the default entity scope) and the query has no explicit `SCOPE` clause, a `SCOPE <id>` clause is spliced in at the correct position (before the first of `GROUP BY` / `HAVING` / `ORDER BY` / `LIMIT` / `OFFSET`) so the query stays valid.
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

Nesting is bounded by the recursion limit of 10 levels (`maxRecursionDepth`). Beyond that, the remaining content is emitted as-is with a trailing `<!-- mr:shortcode depth limit reached -->` comment.

#### Heterogeneous results

A query without a `type` filter can return mixed entity types (resources, notes, groups). The same block body is applied to every item, so reference only fields common to all of them (`Name`, `Description`, `CreatedAt`) or branch on the type first:

```
[mrql query='tags = "featured"' limit="12"]
  [conditional field="ContentType" not-empty="true"]
    <figure><img src="/v1/resource/preview?id=[property path='ID']&amp;height=200" alt="[property path='Name']"></figure>
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
    <img src="/v1/resource/preview?id=[property path='ID']&amp;height=128"
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
| `scope` | No | `entity` | Scope for MRQL queries: `entity`, `parent`, `root`, `global`, or a numeric group ID |
| `aggregate` | No* | -- | Column name for aggregated MRQL results. *Required when the `mrql` source returns aggregated rows; the block renders an error if it is unset |
| `limit` | No | `20` | Result limit for the `mrql` condition source |
| `buckets` | No | `5` | Bucket count for a grouped `mrql` condition source |
| `param-*` | No | -- | Wildcard family that binds an MRQL `$name` placeholder for the `mrql` condition source (e.g. `param-tag="x"`) |
| `eq` | No | -- | True when value equals this string |
| `neq` | No | -- | True when value does not equal this string |
| `gt` | No | -- | True when numeric value is greater than this |
| `lt` | No | -- | True when numeric value is less than this |
| `gte` | No | -- | True when numeric value is greater than or equal to this |
| `lte` | No | -- | True when numeric value is less than or equal to this |
| `in` | No | -- | True when value equals one of a comma-separated list (e.g. `in="a,b,c"`) |
| `contains` | No | -- | True when value contains this substring |
| `matches` | No | -- | True when value matches this Go regular expression. An invalid pattern evaluates to false |
| `empty` | No | -- | True when value is nil or empty string. Write `empty="true"`; the value is ignored, so `empty="false"` does not negate it |
| `not-empty` | No | -- | True when value is non-nil and non-empty. Write `not-empty="true"`; the value is ignored. An empty array counts as non-empty, so use `[each]`'s `[else]` for an empty list |
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

**Field**: reads a struct field from the entity object using reflection. Unlike `[property]`, this resolves a single top-level struct field only (`Name`, `ContentType`, `CreatedAt`, ...); it does not follow dot-paths or slice indices (`Owner.Name`, `Tags.0.Name`).

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

`[elseif]` and `[else]` are dividers with no closing tag: `[/elseif]` and `[/else]` render as literal text, and the editor flags them as errors. `[elseif]` and `[else]` dividers nested inside an inner `[conditional]` block belong to that inner block, not the outer one.

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

More operators, one per use:

```
[conditional path="notes" empty="true"]No notes yet.[/conditional]

[conditional path="status" in="todo,doing"]Still open[/conditional]

[conditional path="title" contains="draft"]Work in progress[/conditional]

[conditional path="code" matches="^[A-Z]{3}-[0-9]+$"]Valid ticket code[/conditional]

[conditional path="status" eq="urgent" path2="priority" eq2="high" combine="any"]
  Needs attention now
[/conditional]
```

`matches` takes a Go regular expression. An invalid pattern is false, so a typo quietly hides the block; test the pattern on a real value. The last example uses `combine="any"` to show the block when either condition is true. Without it, both must hold.

### Conditions from a query

`mrql=` tests a query result, so a block can appear only when a related set of entities exists:

```
[conditional mrql='type = note AND tags = "todo"' gt="0"]
  <h3>Open to-dos</h3>
  [mrql query='type = note AND tags = "todo"' format=list limit=5]
[/conditional]
```

The condition counts at most `limit` results (default 20). A threshold test is exact only when `limit` is above the threshold: to test for more than 100, set `limit="101"`, because a count of 101 means "more than 100". Each `mrql=` condition runs a query when the page renders, so wrap it in `[lazy]` on list pages.

## `[link]` -- Detail-page URLs

Resolves a detail-page URL for the current entity or a related target.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `to` | No | `self` | Link target: `self`, `owner`, `root`, or `category` |

### Targets

- `self` (default) -- the current entity's detail page (`/group?id=`, `/resource?id=`, `/note?id=` by entity type).
- `owner` -- the owning group (`/group?id=`). For resources and notes this is their group; for groups it is the parent group.
- `root` -- the root of the ownership chain (`/group?id=`).
- `category` -- the entity's category/type page (`/category?id=`, `/resourceCategory?id=`, `/noteType?id=`).

### Inline vs Block

- **Inline** (`[link to="..."]`) renders just the URL, HTML-escaped, so you can write it inside an `href`:

  ```html
  <a href="[link]" class="underline">Open</a>
  ```

- **Block** (`[link to="..."]inner[/link]`) renders a full anchor around its processed inner content:

  ```
  [link to="owner"]Back to group[/link]
  ```

When the target cannot be resolved (unknown `to`, an unset category, or an owner/root that is not resolvable), the inline form renders nothing and the block form renders its inner content without a wrapping anchor -- never a link to a placeholder ID.

### Examples

```
<a href="[link]" class="btn">This page</a>
<a href="[link to='owner']">Parent group</a>
[link to="owner"]Back to group[/link]
[link to="root"]Top of the hierarchy[/link]
[link to="category"]View type[/link]
```

Inline output is the bare URL, so it fits any `href` that takes a whole URL. Block output is a complete `<a>` element, so do not wrap a block in another link.

## `[each]` -- Iterate an array

Renders its inner content once per element of an array meta value.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | | Dot-notation path to an array in the entity meta, e.g. `ingredients` |
| `limit` | No | `100` | Maximum number of elements to render |

### How It Works

`[each]` is a block shortcode. Reference the current element with `[item]` inside the block. A non-array or empty value renders the `[else]` branch, or nothing when there is no `[else]`. `[else]` is a divider with no closing tag; end the block with `[/each]`. Inner `[meta]`, `[conditional]`, `[mrql]`, and `[property]` shortcodes run against the **parent entity**, not the element -- use `[item]` for element data.

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
  <li>[item index="true"]. [item path="name"] -- [item path="qty" default="?"]</li>
[else]
  <p>No ingredients.</p>
[/each]
```

A list of plain values, numbered, with no path needed:

```
[each path="steps"]
  <li>[item index="true"]. [item]</li>
[/each]
```

For an array of objects, `[item path="..."]` reads a field of each element. Inner `[meta]`, `[conditional]` and `[property]` still read the parent entity, so use `[item]` for element values. This example puts a value in an `href`, so the value gets a relative prefix and no scheme:

```
[each path="links"]
  <a href="/archive/[item path='slug']">[item path="label" default="Link"]</a>
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

A partial is written once and reused in any slot. For example, a partial named `status-badge` could hold the following body; a Category's Custom Header then includes it with `[partial name="status-badge"]`:

```html
<span class="rounded px-2 py-0.5 text-xs bg-stone-100">[meta path="status" default="no status"]</span>
```

Because the partial expands in the including entity's context, the same markup shows each entity's own status.

See [Custom Templates](./custom-templates.md) for authoring partials, bundles, and presets.

## `[lazy]` -- Defer until visible

Defers its inner content: on a display page the body is rendered **on the server only when the block scrolls into view**, keeping expensive shortcodes -- especially `[mrql]` -- out of the initial page render. This is most valuable in per-card slots (`CustomSummary`) on long list pages, where a slot renders once per card and each entity-scoped `[mrql]` would otherwise run a query for every card up front.

`[lazy]` is a block shortcode and requires a closing `[/lazy]`.

```
[lazy]
  [mrql query='type = "resource"' format="list"]
[/lazy]
```

### How it works

On the initial render the block emits a small `<lazy-shortcode>` placeholder carrying a sealed token; nothing inside is computed yet. When the placeholder scrolls near the viewport, the browser fetches `POST /v1/shortcodes/deferred`, which opens the token, rebuilds the entity context, renders the body, and returns the HTML. The token is authenticated encryption (AES-256-GCM), so it both binds the exact template body and entity the server produced -- no client-supplied template text is ever trusted -- and keeps that body opaque on the page rather than exposing the template source in an attribute.

## `[details]` -- Load on open

A disclosure, like the HTML `<details>`/`<summary>` element, whose inner content is rendered **on the server only the first time it is opened**. It is keyboard- and screen-reader-accessible (it wraps a native `<details>`).

`[details]` is a block shortcode and requires a closing `[/details]`.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `summary` | No | `Details` | The always-visible label that toggles the disclosure |
| `open` | No | `false` | When `true`, the disclosure starts expanded (and loads its content immediately) |

```
[details summary="Nutrition"]
  [meta path="calories"] kcal
[/details]

[details summary="Open to-dos" open="true"]
  [mrql query='type = note AND tags = "todo"' format=list limit=5]
[/details]
```

On entity pages, cards, the dashboard and hovercards, a `[details]` loads its body the first time it is opened, so a heavy query inside one costs nothing until a reader asks for it. Elsewhere (share pages, the live preview, JSON responses) the body renders inline right away. See [Where deferral applies](#where-deferral-applies). Use `open="true"` only when the content is wanted on every view.

The load-on-open mechanism is the same signed round-trip as `[lazy]`.

### Where deferral applies

`[lazy]` and `[details]` defer across the **standard authenticated template pipeline**, wherever a `Custom*` slot renders a member entity (Group, Resource, or Note). That includes the entity detail pages, list-page `CustomSummary` cards, the dashboard, and hovercards. Everywhere else the body renders **inline** as a graceful fallback -- the content still appears, and `[details]` remains a plain collapsible `<details>` -- either because no deferred signer is installed (public share pages, the live authoring preview, JSON API responses) or because the context is a carrier rather than a member entity (carrier list slots (`CustomListHeader`, `CustomListFooter`), whose `category` / `resource_category` / `note_type` context cannot be reloaded by the deferred endpoint).

### Limitations

- **JavaScript is required** for the deferral to hydrate (as it is for the rest of the app). A `<noscript>` note is shown when it is disabled.
- The deferred body renders against the **entity**, so an `[item]` from an enclosing `[each]` is not available inside a `[lazy]`/`[details]` block (the same non-goal as `[each]`'s element-relative paths).
- The token key is per-process and per-boot by default, so a placeholder minted before a restart -- or, in a multi-process deployment, resolved by a different process -- fails to load and shows a small "could not load -- Retry" message. Set the `TEMPLATE_SIGNING_KEY` environment variable to a shared secret across all processes for behind-a-load-balancer deployments.

## `[reload]` -- Re-render on demand

A button that re-renders content on the server and swaps it into the page. Which content it refreshes is not fixed at render time: the button carries no token and names no target, and the browser resolves what to refresh at click time by walking up the DOM from the button.

`[reload]` works self-closing or as a block. Self-closing it renders a circular-arrow icon button; a block body becomes the button face instead. A block body that expands to nothing falls back to the icon.

```
[reload]
[reload label="Refresh totals"]
[reload]Refresh[/reload]
```

A typical use is a sidebar that shows a live list. The button refreshes the whole sidebar slot, because it sits outside any `[lazy]` or `[details]` block:

```html
<h3>Open to-dos</h3>
[reload label="Refresh open to-dos"]
[mrql query='type = note AND tags = "todo"' format=list limit=5]
```

To refresh only one part of a slot, put the button inside a `[lazy]` block with the content it should refresh. The button then reloads just that block.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `label` | No | `Reload` | The button's accessible name. The default applies whenever the button face carries no text of its own, which covers the icon and a block body that is only an `<svg>` or an `<img>`; those also get it as a tooltip. A button with visible text and no `label` is named by that text and gets no `aria-label` at all; when you do set `label` there, keep the visible text inside it (WCAG 2.5.3 Label in Name) |

### What it reloads

Walking up from the button, the first of these wins:

1. **The innermost enclosing `[lazy]` or `[details]` block.** It already carries a sealed token for its own body, so only that block is re-rendered.
2. **Otherwise the whole custom-content slot the button was written in**, which is any slot the `process_shortcodes` tag renders against a group, resource, or note: CustomHeader, CustomDetailFooter, CustomSidebar, CustomPreview, CustomOwnEntities, CustomSummary, CustomAvatar, CustomHoverCard, CustomCell, or a description. The `process_shortcodes` tag wraps such a slot in `<div class="shortcode-region" data-shortcode-region="TOKEN">`, where the token seals the whole raw slot body the same way `[lazy]` seals its own. The wrapper is emitted only when the rendered slot actually contains a reload button and the slot is rendering against a group, resource, or note, so slots without a `[reload]` are byte-for-byte unchanged. It is `display: contents`, so it adds no layout box.
3. **Otherwise the page** (`window.location.reload()`). Anywhere the walk finds neither a deferred block nor a region, the button falls straight through to a page reload without attempting a request. That covers every surface whose slots are rendered outside the `process_shortcodes` tag or without a signer: the public share server, the category-template live preview, the JSON rendering of the `Custom*` slots, and `CustomMRQLResult` cards on `/mrql`. It also covers the `CustomListHeader` and `CustomListFooter` carrier slots, whose `category` / `resource_category` / `note_type` context the deferred endpoint cannot load by id.

Cases 1 and 2 fetch `POST /v1/shortcodes/deferred`, the endpoint `[lazy]` and `[details]` use. `TEMPLATE_SIGNING_KEY` applies to region tokens exactly as it does to `[lazy]`/`[details]` tokens.

Because this is a proximity walk, a `[reload]` placed inside a `[lazy]` body refreshes just that block, and the same `[reload]` moved outside it refreshes the whole slot:

```
[lazy]
  [reload label="Refresh open tasks"]
  [mrql query='type = "note"' format="list"]
[/lazy]
```

### During a reload

- The content being replaced stays on screen and is dimmed while the request is in flight, rather than collapsing to a loading placeholder. A `[lazy]` or `[details]` block *inside* a reloaded slot does come back unrevealed, and loads again on its own terms.
- The Alpine `entity` scope is refreshed along with the markup, so bindings written against it (`x-text="entity.Meta.status"` and friends, see [Custom Templates](./custom-templates.md)) show current values rather than the snapshot the page was loaded with. Shortcodes are expanded server-side against the same fresh entity, so both halves of a template stay in step.
- `aria-busy` is set on the content being refreshed and on the button, and the button greys out. On the icon form the glyph also spins, which `prefers-reduced-motion` suppresses.
- Activation announces "Reloading" politely, then "Content reloaded" on success. A fast reload coalesces the two.
- On failure the previous content is left exactly as it was, "Could not reload the content" is announced assertively, and a "Reload failed." marker appears beside the button so the stale content is not mistaken for fresh.
- The button is normally inside the content it replaces. If it had focus when it was activated, and focus has not moved elsewhere in the meantime, focus goes to the equivalent reload button in the fresh content; if no reload button survives the re-render, it goes to the refreshed container, then to the first thing inside it that can hold focus, and failing that outwards to the nearest ancestor that can -- so the reader is never dropped onto `<body>`. A region wrapper is `display: contents` and generates no box, so it cannot always take focus itself, and content that came back as bare text offers nothing inside to land on.
- Repeat activations while a reload is in flight are ignored. If the page itself re-renders while a reload is in flight (an Alpine morph), that reload is abandoned: the button becomes clickable again immediately against the newly rendered content, and the abandoned request reports neither success nor failure.
- When one reload encloses another -- a region reload and a `[lazy]` block inside it -- the later activation wins in both directions, regardless of which response arrives first. Replacing a region also replaces every block inside it, so letting the slower answer land would put back a render older than the one the reader just asked for. The superseded button is released the moment it is overtaken rather than when its own request answers, so a slow or hung request cannot leave a live button spinning and unclickable.

### Limitations

- **JavaScript is required.** Unlike `[lazy]` and `[details]`, which show a `<noscript>` note in place of content they cannot fetch, the button itself renders with scripting off and simply does nothing.
- The block body is the face of a `<button>`, so keep it to phrasing content: a button may not contain links or other interactive elements. Two cases are refused outright rather than left to produce invalid markup, and the template linter flags both:
  - **A `[reload]` inside another `[reload]`.** Nested buttons are repaired differently by every browser, and each repair leaves two controls in the accessibility tree. The outer `[reload]` renders a `⚠ reload` marker instead of a button. The body's *source* is checked as well as its rendered output, because a `[lazy]`/`[details]` inside the face is sealed rather than expanded, so a `[reload]` within it would be invisible at render time and arrive inside the button when the deferred fetch landed.
  - **A `[lazy]` or `[details]` inside a `[reload]`.** Both emit a block-level element, which a button may not contain. Each renders its own `⚠` marker in place. This is also what stops the deferred-nesting case above from reaching the page by way of a `[partial]`, whose source the checks on the button's own body never see.
- A region token carries the slot's own source, so a `[reload]` in a `CustomSummary` adds one sealed copy of that slot per card on a list page. The sealed token runs about **1.34x the size of the raw slot body** (base64 over a nonce and an authentication tag), so a 2 KB `CustomSummary` adds roughly 2.7 KB per card -- about 135 KB on a 50-card page. The ciphertext is randomised, so identical slots across cards do not compress against each other. This scales with cards rendered per page, not with database size. Putting the `[reload]` inside a `[lazy]` mints no region at all, because the block's own token (which `[lazy]` seals regardless) serves the button. That only shrinks the page when the deferred body is smaller than the whole slot, but it never adds a second copy on top.
- A description is sealed **after** its Markdown and mention filters have run, so a `[reload]` written in one re-evaluates the shortcodes inside it but replays the surrounding prose as it stood when the page loaded. Edits to the description text itself need a page load.

## `[using]` -- Switch to another entity

Renders its body with another group, resource or note as the entity. Every shortcode inside -- `[property]`, `[meta]`, `[conditional]`, `[each]`, `[link]`, `[mrql scope=…]`, a nested `[using]` -- reads the target instead of the entity the template belongs to.

```
[using rel="owner"]
  <h3>[link][property path="Name"][/link]</h3>
  Status: [meta path="status" inline="true"]
[else]
  <p>No owner.</p>
[/using]
```

### Attributes

Name the target with **exactly one** of `id`, `id-path`, `id-prop`, `rel` or `query`.

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `type` | With `id`, `id-path`, `id-prop` | | `group`, `resource` or `note`. Not used with `rel` (always a group) or `query` (the query names its type) |
| `id` | | | The target's ID, a positive whole number |
| `id-path` | | | Dot-path into this entity's Meta holding the target's ID, as a number or a numeric string, e.g. `links.client` |
| `id-prop` | | | Field of this entity holding the target's ID, e.g. `OwnerId` |
| `rel` | | | `owner` (the group that owns this entity) or `root` (the top of its ownership chain), as in `[link to=]` |
| `query` | | | MRQL query; its first result is the target |
| `scope` | No | `entity` | With `query` only: subtree to run the query in, as on `[mrql]` |
| `param-<name>` | No | | With `query` only: binds `$name` in the query |

### How It Works

The target is loaded with an MRQL query run as the viewer, the same way `[mrql]` runs. That has three consequences:

- **You only see what you may see.** A target outside the viewer's groups is treated as missing. Missing and hidden targets both render the `[else]` branch (or nothing when there is no `[else]`), so the block cannot be used to probe which IDs exist.
- **It costs a query.** Each block runs one query per render and counts toward the [per-page query budget](#per-page-query-budget). In a per-card slot that is one query per card, so wrap it in `[lazy]` on long lists. Identical lookups on one page (every card pointing at the same owner) are cached and cost one.
- **It is unavailable where `[mrql]` is.** On share pages a `[using]` block renders an HTML comment.

A lookup by `id`, `id-path`, `id-prop` or `rel` searches globally: the `[mrql]` default scope does not apply to it. `[else]` is a divider with no closing tag; end the block with `[/using]`. It renders against the original entity.

`query` should name its entity type (`type = group`, `resource` or `note`) and have an `ORDER BY`. Without a type the first match can be any of the three, and without an `ORDER BY` which match becomes the target is not stable. The linter warns about both. `GROUP BY` is not allowed.

### Limits

- An `[item]` from an enclosing `[each]` is not available inside `[using]`; it renders nothing.
- There is no way to reach the original entity from inside the block. Put values from it before or after the block.
- The target category's CustomCSS is not added to the page.
- An editable `[meta]` inside the block edits the target, subject to the viewer's write access.

### Examples

```
[using type="group" id-path="client"]
  Client: [property path="Name"]
[/using]

[using rel="root"]
  Project: [link][property path="Name"][/link]
[/using]

[using type="group" id-prop="OwnerId"]
  Owned by [link][property path="Name"][/link]
[else]
  No owner group.
[/using]

[using type="resource" id="42"]
  <img src="/v1/resource/preview?id=[property path='ID']" alt="[property path='Name']">
[/using]

[using query='type = note AND name = $name ORDER BY created DESC' param-name="Index"]
  [property path="Description"]
[else]
  No index note yet.
[/using]
```

## Plugin Shortcodes

Plugins can register custom shortcodes via the `mah.shortcode()` Lua API. Plugin shortcodes use the format:

```
[plugin:plugin-name:shortcode-name attr="value"]
```

The plugin name and the shortcode name must each start with a lowercase letter, contain only lowercase letters, digits, hyphens (`-`) and underscores (`_`), and be at most 50 characters long. A shortcode whose name does not match is rejected when the plugin registers it.

Plugin shortcodes also support block mode:

```
[plugin:plugin-name:shortcode-name attr="value"]
  content here
[/plugin:plugin-name:shortcode-name]
```

The plugin receives `inner_content` and `is_block` in its render context. For example, a plugin named `weather` that registers a `forecast` shortcode is used as follows (the attributes are whatever that plugin defines):

```
[plugin:weather:forecast city="Berlin" days="3"]
[plugin:weather:forecast city="Berlin"]
  Forecast for the trip
[/plugin:weather:forecast]
```

Shortcodes in a plugin's returned HTML are expanded after the plugin returns, in both forms: whether the author wrapped a body says nothing about what the plugin emits. Expansion is bounded by the same nesting depth limit as every other shortcode, so a plugin that emits its own shortcode stops instead of looping. Only a successful render is expanded, because an error marker and the "plugin unavailable" comment carry text the plugin did not author.

Note: in docs preview, shortcodes inside plugin output are not expanded (they render as literal text). This is a preview-only limitation; runtime rendering expands them normally.

See [Plugin Lua API](./plugin-lua-api.md#mahshortcode----custom-shortcodes) for registration details.

## Recipes

Complete patterns that combine several shortcodes. Each one is written for a Category, Resource Category or Note Type slot, and each passes the shortcode linter.

### A status card

For a Custom Summary on a note or group card. Optional fields are hidden when empty, so the card stays short:

```html
<div class="flex flex-wrap items-center gap-2">
  <strong>[property path="Name"]</strong>
  [pills path="status" hide-empty="true"]
  [datetime path="due" layout="Jan 2, 2006" hide-empty="true"]
</div>
```

### A list with a live total

The `{total}` placeholder counts every match, while the list shows only the first five:

```html
[mrql query='type = note AND tags = "todo"' limit="5"]
  [header]<h4>Open to-dos: {total}</h4><ul class="list-disc pl-5">[/header]
  <li><a href="[link]">[property path="Name"]</a></li>
  [footer]</ul>[/footer]
[else]
  <p>Nothing open.</p>
[/mrql]
```

### A section that appears only with content

A `[conditional]` on a query result, with the query itself deferred until the section scrolls into view:

```html
[conditional mrql='type = note AND tags = "todo"' gt="0"]
  <h3>To-dos</h3>
  [lazy]
    [mrql query='type = note AND tags = "todo"' format=list limit=5]
  [/lazy]
[else]
  <p class="text-stone-500">No to-dos.</p>
[/conditional]
```

### A list with an empty state

`[each]` takes its `[else]` branch for a missing or empty array, which a `[conditional]` on `not-empty` does not do (an empty array counts as present):

```html
<h3>Ingredients</h3>
[each path="ingredients"]
  <li>[item path="name"] -- [item path="qty" default="?"]</li>
[else]
  <li class="text-stone-500">No ingredients yet.</li>
[/each]
```

### A link to the owner, with a fallback

`[using rel="owner"]` switches to the owning group, so its name and link both come from that group. The `[else]` branch covers an entity with no owner:

```html
Owned by [using rel="owner"]<a href="[link]">[property path="Name"]</a>[else]no group[/using]
```

## Common mistakes

- **Closing a divider.** `[/else]` and `[/elseif]` are not closers. They render as literal text. Only `[/conditional]`, `[/each]` and the other block closers take a `[/name]` tag.
- **Unquoted values with spaces.** The value stops at the first space and the rest is dropped without an error. Quote it (see [Quoting](#quoting)).
- **`value=` with a block body.** The body is ignored at render time, and the linter reports it as an error.
- **`[item]` outside its loop.** `[item]` renders nothing outside an `[each]`. An `[item]` from an enclosing `[each]` is not available inside any other block shortcode, including `[conditional]`, `[mrql]`, `[using]`, `[lazy]` and `[details]`.
- **Counting with `value="count"`.** It counts returned rows, capped by `limit`. Use `{total}` for the real number.
- **Per-card queries on a list page.** Every card runs an entity-scoped `[mrql]` in `CustomSummary`. Wrap it in `[lazy]` so the query runs only for cards on screen.
- **Per-card values in CSS.** A `CustomCSS` slot is rendered once per category per page, so on a list page its `[meta]` values come from the first entity of that category in the list.
- **Meta in a script, style or unquoted attribute.** Escaping does not protect these contexts. See [Inline values inside HTML attributes](#inline-values-inside-html-attributes) for the list.

## Related Pages

- [Custom Templates](./custom-templates.md) -- where shortcodes are used
- [MRQL Query Language](./mrql.md) -- query language used by the `[mrql]` shortcode
- [Meta Schemas](./meta-schemas.md) -- schemas that drive `[meta]` shortcode rendering
- [Plugin Lua API](./plugin-lua-api.md) -- registering custom plugin shortcodes
