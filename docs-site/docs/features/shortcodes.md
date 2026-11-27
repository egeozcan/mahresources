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

- **Built-in inline:** `[meta ...]`, `[datetime ...]`, `[pills ...]`, `[property ...]`, `[item ...]`, `[partial ...]`, `[calendar ...]`, `[calc ...]`, `[similar ...]`, `[adjacent ...]`, `[inspect ...]`
- **Built-in block:** `[conditional ...]...[/conditional]`, `[each ...]...[/each]`, `[lazy]...[/lazy]`, `[details ...]...[/details]`, `[autorefresh ...]...[/autorefresh]`, `[using ...]...[/using]`, `[comment]...[/comment]`
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
| `[excerpt]` | Inline | Plain-text, length-bounded preview of a Description, string field or Meta string |
| `[calc]` | Inline | Compute a number from Meta, entity fields or [each] elements |
| `[mrql]` | Inline or block | Run an MRQL query and render results, a scalar value, or a custom item template |
| `[conditional]` | Block | Render the first matching branch from a meta, field, or MRQL condition |
| `[link]` | Inline or block | Resolve a detail-page URL or wrap content in a link |
| `[each]` | Block | Iterate an array in Meta |
| `[item]` | Inline | Render the current element inside an `[each]` or `[relations]` block |
| `[relations]` | Inline or block | List the group's typed relations to other groups |
| `[partial]` | Inline | Expand a named Template Partial |
| `[lazy]` | Block | Defer server rendering until the block scrolls into view |
| `[details]` | Block | Defer server rendering until a disclosure is opened |
| `[tabs]` | Block | Alternative sections as an accessible tab set; `lazy="true"` tabs render only when first opened |
| `[reload]` | Inline or block | Re-render the nearest deferred block or custom-content slot on demand |
| `[autorefresh]` | Block | Re-render the body after an inline editor saves |
| `[action]` | Inline | Run a plugin action on this entity from a button |
| `[using]` | Block | Render the body with another group, resource or note as the entity |
| `[adjacent]` | Inline | Previous/next links to the entity's siblings or Series |
| `[similar]` | Inline | Thumbnails of perceptually similar resources, with tier, distance and a compare link |
| `[calendar]` | Inline | Month grid of entities by a date field, with titles, day links and month navigation |
| `[pivot]` | Inline | Cross-tab of two fields with a count or sum per cell, totals and count links |
| `[inspect]` | Inline | Live preview only: list the Meta paths, fields and `[each]` element paths available at that point |
| `[comment]` | Block | Remove a section on the server so it never runs or renders |

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

In stylesheets, a bare built-in opener immediately after an ASCII letter, digit, `-`, `_`, `*`, `]`, or `|` is kept as a CSS attribute selector: `form[action]` and `.w[tabs]` work as written. Attributed shortcodes still expand normally. This rule does not apply to `[comment]`, which is stripped before shortcodes are parsed.

### Failure markers

A shortcode that cannot be expanded never leaks its raw `[…]` source. Instead, the rendered page carries a diagnosable marker:

- **A failing plugin shortcode** renders an inline `<span class="shortcode-error" title="…">⚠ plugin:name:shortcode</span>` with the error in the `title` attribute.
- **An unclosed `[conditional]`** (used without `[/conditional]`) renders the same inline marker -- it can't gate anything as written. The template lint flags this at edit time.
- **Structural stops** render an HTML comment rather than visible noise: the recursion depth cap emits `<!-- mr:shortcode depth limit reached -->`, and a context that deliberately wires no query executor or plugin renderer emits `<!-- mr:mrql unavailable in this context -->` / `<!-- mr:using unavailable in this context -->` / `<!-- mr:plugin unavailable in this context -->`.

A `<!-- mr: ... -->` comment means expansion stopped on purpose, either because the surface has no query or plugin support or because the depth cap was reached. It is not a failed query. A red `⚠` marker means a shortcode failed or was written wrongly: a failing plugin shortcode, a malformed plugin shortcode name, an unclosed `[conditional]` or `[using]`, a bad `[using]` target, or a nested `[reload]`. Its `title` attribute says why. A failing `[mrql]` shows a red `mrql-error` message instead.

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

`default` is shown when the stored value is empty. If both are set, `hide-empty` wins outside edit mode, so the `default` is not shown there. `hide-empty` hides an empty value outside edit mode, so an optional date does not leave a placeholder on every card. Do not combine `hide-empty` with `editable` on the same field: an empty field then has nothing to click, so it can never be filled in from that slot.

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

`raw=true` skips escaping, so a value that contains HTML is rendered as HTML. Use it only for text an admin or editor wrote, and never for a value that a plain `user` can edit. The linter warns on every `raw=true`. With [authentication](./authentication.md#raw-html-in-descriptions-and-text-blocks) on, `raw=true` is ignored in a description or note text block (and in anything those pull in), so the value is escaped there.

### Available Fields by Entity Type

`[property]` reflects over the model, so any exported field resolves. These are the commonly used ones rather than an allow-list.

The exception is a credential field: a note's `ShareToken` (and `TokenHash`, `CsrfToken` and `PasswordHash`, wherever a path reaches one). It resolves, in `[property]` and in `[conditional field=]`, only for a reader that may share the note (admin, editor or user, or any request with authentication off). For a guest it renders as empty (or as `default=`) on every page, the deferred `[lazy]` body and text blocks included. The public share page shows it, because its visitor already holds the token.

**Group:** `ID`, `Name`, `Description`, `URL`, `CreatedAt`, `UpdatedAt`, `CategoryId`, `OwnerId`, `Meta`

**Resource:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `ContentType`, `OriginalName`, `OriginalLocation`, `FileSize`, `Width`, `Height`, `Hash`, `ResourceCategoryId`, `OwnerId`, `Meta`

**Note:** `ID`, `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `NoteTypeId`, `OwnerId`, `StartDate`, `EndDate`, `Meta`

## `[excerpt]` -- Text Preview

Renders a short, single-line, plain-text preview of an entity's long text: its Description by default, another string field, or a string in its Meta. Use it on cards and in query results, where `[property path="Description"]` would print the whole Markdown source, mention markers and all.

```
[excerpt words="25" more="Read more"]
```

`[excerpt]` is inline only. It never runs a query and never loads data: it reads the entity the surface already shows, so it works in every slot, in `CustomMRQLResult` and in `[mrql]` and `[using]` bodies.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `words` | No | `30` | Maximum number of words, a whole number from 1 to 200. A value that is not a whole number, or is below 1, falls back to 30; above 200 is clamped to 200. |
| `field` | No | `Description` | One string field of the current entity, such as `Name` or `OriginalName`. No dot paths: to preview a related entity, put `[excerpt]` inside [`[using]`](#using----switch-to-another-entity). A field that is missing, not a string, or a credential the reader may not see (as with `[property]`) renders the default. |
| `path` | No | -- | Dot-notation path to a string in the entity's Meta, e.g. `recipe.intro`. A number, boolean, object, array, `null` or missing key renders the default. Cannot be combined with `field`. |
| `more` | No | -- | Visible label of a link to the entity's detail page, shown only when the text was cut. |
| `default` | No | -- | Text rendered when there is nothing to preview. It is always HTML-escaped. |

There is no `raw`: the output is always escaped text. Setting both `path` and `field` renders the `⚠ excerpt` [failure marker](#failure-markers), and the linter reports it.

### What is stripped

The source goes through these steps on the server, in order:

1. Only its first 16 KiB are read.
2. [Template comments](#comment----comment-out-a-section) are removed.
3. Every shortcode is removed without running: a block goes with its body, so `[details]…[/details]` disappears entirely, and an `[mrql]` in the text runs no query. This happens before Markdown is read, so a shortcode inside a code span or fenced code block is removed too; the rest of the code text stays.
4. Each mention (`@[resource:5:Sunset photo]`) becomes its name, `Sunset photo`, exactly as written: a `*` or `<` in a name is not read as Markdown or HTML.
5. Markdown is rendered and only the visible text kept. HTML tags, attributes (image alt text included), HTML comments and the contents of `<script>`, `<style>` and similar elements are dropped. Paragraphs, list items, table cells and line breaks become spaces.

The result is one line: every run of whitespace becomes one space.

### Truncation

- The text is cut after `words` words, and in any case after 12 characters per allowed word, so text without spaces (CJK, a long URL) stays bounded too.
- A cut text ends with `…`, after any trailing `, ; : . -` has been removed.
- A source longer than 16 KiB always ends with `…`, even when its first 16 KiB show fewer than `words` words, because the excerpt cannot know what follows.

### Empty state

When there is no text to show (an empty field, a non-string Meta value, or a Description that is only shortcodes, comments or images), `[excerpt]` renders `default`, escaped, or nothing.

### Output and the `more` link

Without `more`, the output is a bare escaped value with no wrapper element, so it can sit inside a quoted attribute like `[property]`:

```
<span title="[excerpt words='25']">[property path="Name"]</span>
```

With `more` and a cut text, a link follows the ellipsis:

```html
Lorem ipsum dolor… <a href="/note?id=42" class="underline" aria-label="Read more: Meeting notes">Read more</a>
```

- Its accessible name adds the entity's name (or `note 42` when it has none), so a list of "Read more" links reads as distinct links, and it starts with the visible label.
- It is underlined, so it does not rely on colour alone.
- It renders for read-only viewers as well: it is navigation, not an edit control.
- It is left out inside a [`[reload]`](#reload----re-render-on-demand) button face, which cannot contain a link, and on the [public share page](./note-sharing.md), which serves no detail pages.
- Do not use `more` inside an HTML attribute: the link is markup.

### Examples

A note type's `CustomSummary`:

```
<p class="text-sm text-stone-600">[excerpt words="25" more="Read more"]</p>
```

The latest notes in a group header, each with a preview:

```
[mrql query="type = note ORDER BY updated DESC" limit="5"]
  <p><a href="[link]">[property path="Name"]</a>: [excerpt words="12"]</p>
[/mrql]
```

A Meta string with a fallback:

```
[excerpt path="recipe.intro" words="15" default="No intro yet"]
```

## `[calc]` -- Arithmetic over Fields

Computes a number from values the page already holds: the entity's Meta, its numeric fields and, inside `[each]`, the current element. Use it for derived values that would go stale if stored, such as a line total, a discounted price, a remaining budget or a resource's megapixels.

```
[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="-"]
```

`[calc]` is inline only. It never runs a query and never calls a plugin, so it works everywhere `[meta inline="true"]` works (share pages, Custom MRQL Result cards, the live preview) and shows nothing the reader could not already see on the page. For totals across many entities, use an aggregate `[mrql value=…]` over a `GROUP BY` instead.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `expr` | Yes | -- | The expression. It never contains quotes, so either attribute quoting works. A blank `expr` renders nothing. |
| `decimals` | No | unset | A whole number from 0 to 10. The result is rounded half away from zero to exactly that many fraction digits, keeping trailing zeros (`10` with `decimals="2"` renders `10.00`). Unset, the result is rounded to 10 fraction digits and trailing zeros are dropped, so `0.1 + 0.2` renders `0.3` and `1 / 3` renders `0.3333333333`. |
| `format` | No | `number` | `number` is a plain decimal (`1234567.89`). `grouped` puts a `,` between every three integer digits and uses `.` as the decimal point (`1,234,567.89`); it is not locale-aware. `percent` multiplies by 100, applies `decimals` and appends `%` (`0.256` renders `25.6%`). |
| `default` | No | empty | Text rendered, HTML-escaped, when the expression has no numeric result. |

There is no `raw`, `hide-empty`, `editable` or `layout`: the output is always a number or the escaped default.

### Operands

| Operand | Reads | Counts as a number when |
|---------|-------|-------------------------|
| `meta.<path>` | The entity's Meta at a dot path, e.g. `meta.price.net` | It is a JSON number, or a string that is wholly a decimal number after trimming spaces (`"12.50"`, `" 3 "`, `"1e3"`). `"12,5"`, `"0x10"`, `"NaN"`, `"1_000"`, `""` and a string longer than 64 bytes are not numbers. |
| `property.<Path>` | An entity field, the same traversal as `[property]` (e.g. `property.Width`, `property.Tags.0.ID`) | It is an integer or floating-point field. Strings, booleans, times and structs are never numbers. A nil pointer (an ownerless entity's `OwnerId`) has no value. Credential fields are withheld exactly as in `[property]`. |
| `item`, `item.<path>` | The current `[each]` element, or a dot path into it | As for `meta.`. See [Inside `[each]`](#inside-each). |
| `12`, `0.5` | A literal | Always. Literals are plain decimals: no exponent, no leading `.`, at most 32 characters. |

Names are case-sensitive. A bare name (`qty`), any other root (`entity.x`) or any other function (`sqrt`) is a syntax error.

**Only Meta keys made of ASCII letters, digits and `_`, not starting with a digit, can be referenced.** Keys containing `-`, spaces, dots or non-ASCII letters cannot. `meta.unit-price` reads as `meta.unit - price`, and `price` is an unknown name, so it is a syntax error. Rename the key (`unit_price`) to use it in `[calc]`.

### Operators and functions

- `+ - * /` as usual; `%` is the remainder and takes the sign of the dividend (`-7 % 3` is `-1`).
- Unary `-` and `+`, then `* / %`, then `+ -`; all left-associative. Parentheses group.
- `min(a, …)` and `max(a, …)` take 1 to 8 arguments.
- `abs(x)`.
- `round(x)` and `round(x, digits)` round half away from zero (`round(2.5)` is `3`, `round(-2.5)` is `-3`, `round(2.675, 2)` is `2.68`). `digits` must be a whole number from 0 to 10, otherwise the call has no value.
- `coalesce(a, b, …)` (1 to 8 arguments) returns the first argument that has a value. Arguments are evaluated left to right and evaluation stops at the first hit.

### Missing values and `coalesce`

A missing key, `null`, a boolean, an object, an array or a non-numeric string has no value. So has division or remainder by zero and any overflow. No value spreads through every operator and function except `coalesce`, and when the whole expression has no value `[calc]` renders `default`.

```
[calc expr="meta.qty * meta.unit_price" default="not set"]
[calc expr="coalesce(meta.spent / meta.budget, 0)" format="percent"]
```

The second line renders `0%` when the budget is zero or missing, instead of the default.

A mistake in the template itself renders the `⚠ calc` [failure marker](#failure-markers), with the reason in its tooltip: a syntax error, an unknown name or function, a wrong argument count, an expression past the limits below, or a `decimals` or `format` value outside the lists above. The editor's linter reports the same mistakes. Template generation, which lints against the category's Meta Schema, also warns about a `meta.` key the schema does not declare.

### Formatting and rounding

The output is bare text with no wrapper element: digits, an optional leading `-`, `.`, the `,` of `grouped` and the `%` of `percent`. It never uses exponent notation, and a result that rounds to zero renders `0` (or `0.00`), never `-0`. Rounding works on the shortest decimal form of the number, so `1.005` with `decimals="2"` renders `1.01`, as written, rather than the `1.00` that binary floating point would give.

`[calc]` computes in double-precision floating point, which holds about 15 significant digits exactly. An integer beyond 9,007,199,254,740,992 (2^53), such as a 19-digit ID, is rounded before any arithmetic, so it does not render as stored. Store such identifiers as JSON strings and show them with `[meta]`.

Because the output is plain text, `[calc]` can sit inside an HTML attribute: `style="width: [calc expr='meta.done / meta.total * 100' decimals='0' default='0']%"`.

### Inside `[each]`

At the top level of an `[each]` body, `item` is the current element:

```
[each path="lines"]
  [item path="name"]: [calc expr="item.qty * item.price" decimals="2"]
[/each]
```

The same rules as for `[item]` apply. A `[calc]` inside another block within the body (`[conditional]`, `[lazy]`, `[details]`, `[autorefresh]`, `[using]`) cannot see the element, so its `item` operands have no value; `meta.` and `property.` still read the entity. A nested `[each]` binds its own element. Outside any `[each]`, `item` has no value and the linter warns.

### Limits

An expression may be at most 512 bytes, 128 tokens, 32 levels deep (parentheses, calls and unary signs together) and 32 references, with at most 8 arguments per call. Past any of them it renders the failure marker. Each `[calc]` is a few microseconds of work, and the entity's Meta is decoded once per render however many references there are.

### Accessibility

- Put a derived number where its meaning is clear: a table cell under a header cell (`<th scope="col">`), or a sentence such as "Total due: …". A number on its own tells a screen-reader user nothing.
- When a value drives a bar, use `<meter>` or `<progress>` with an accessible name, and put the value in the fallback text so it does not rely on the bar or on colour alone:

  ```
  <meter min="0" max="1" value="[calc expr='meta.spent / meta.budget' default='0']" aria-label="Budget used">[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter>
  ```

- Give `default` words (`not set`, `unknown`) where the reader needs to know why the value is absent. A `-` is fine in a table cell.

### Examples

An invoice table:

```
<table>
  <caption>Invoice lines</caption>
  <tr><th scope="col">Item</th><th scope="col">Qty</th><th scope="col">Price</th><th scope="col">Total</th></tr>
  [each path="lines"]
  <tr><td>[item path="name"]</td><td>[item path="qty"]</td><td>[item path="price"]</td><td>[calc expr="item.qty * item.price" decimals="2" format="grouped"]</td></tr>
  [/each]
</table>
<p>Total due: [calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" format="grouped" default="not set"]</p>
```

A budget:

```
<p>Remaining: [calc expr="meta.budget - meta.spent" decimals="2" default="not set"]</p>
<meter min="0" max="1" value="[calc expr='meta.spent / meta.budget' default='0']" aria-label="Budget used">[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter>
```

A resource's size in megapixels (in a Resource Category template):

```
[calc expr="property.Width * property.Height / 1000000" decimals="1" default="unknown"] MP
```

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

Pick the row a scalar reads with `ORDER BY` on an aggregated query: `value=` reads the first row. A `LIMIT` in the query text is replaced by the shortcode's `limit` attribute, so leave it out. This gives the largest file size across all resources, in bytes:

```html
<p>Largest file: [mrql query="type = resource GROUP BY contentType MAX(fileSize) ORDER BY max_fileSize DESC" value="max_fileSize"] bytes</p>
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
| `[excerpt ...]` | A plain-text preview of the current item's Description, string field or Meta string |
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
| `matches` | No | -- | True when value matches this Go regular expression. An invalid pattern, or one past the regular expression allowance, evaluates to false |
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

`matches` takes a Go regular expression. An invalid pattern is false, so a typo quietly hides the block; test the pattern on a real value. A pattern is also false when it is past the regular expression allowance: at most 32 KiB of pattern text, 20,000 compiled instructions and 100,000 character-class runes for one pattern (a Unicode class such as `\pL` counts as about 1,300), and 200,000 instructions, 2,000,000 runes and 4 MiB of pattern text for all the patterns of one render together. Each distinct pattern is counted once per render. The editor lint reports such a pattern as an error. The last example uses `combine="any"` to show the block when either condition is true. Without it, both must hold.

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

`[item]` renders the current element inside an `[each]` block (or the current relation inside a [`[relations]`](#relations----group-relations) block). It uses the same `format`, `layout`, and `default` helpers as `[property]`, and is HTML-escaped unless `raw="true"`. Outside an `[each]` block it renders nothing.

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

## `[relations]` -- Group relations

`[relations]` lists the current group's typed [relations](../concepts/relationships.md) to other groups, grouped by relation type, each entry a link to the group at the far end. Use it to show "depends on X, Y" on the cards of a list or an MRQL result, to show only one type of relation, or to place relations in a slot of your own.

```
[relations]
[relations type="Depends on" limit="5"]
[relations direction="in"]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `type` | No | | Only relations of this type: an exact, case-sensitive name (surrounding spaces are ignored). A relation type is unique by its name **and** its from and to categories, so two types named "Depends on" for different categories are both matched. Empty lists every type. |
| `direction` | No | `out` | `out` lists the relations from this group; `in` lists the relations to it. Any other value renders an error marker. |
| `limit` | No | `20` | The most relations listed. Above 100 is capped at 100; `0`, a negative or a non-number falls back to 20. The linter warns on each. |

### Outgoing and incoming

A relation is directed: "Backend API **Depends on** Auth Service" is outgoing from Backend API and incoming to Auth Service.

- A relation type with a **back relation** stores the reverse edge as a relation of its own: adding "Backend API depends on Auth Service" also adds "Auth Service is required by Backend API". So `direction="out"` already lists both halves of such a pair, each from its own group's side, and `in` would list the same pair again.
- Use `direction="in"` for types without a back relation, whose edges only the far group lists as outgoing. Its labels read `Incoming: <type>`.
- To show both, write two shortcodes, each where it belongs.

### Output

```html
<div class="relations-shortcode" data-direction="out">
  <ul class="relations-shortcode__types">
    <li class="relations-shortcode__type">
      <span class="relations-shortcode__label">Depends on</span>
      <ul class="relations-shortcode__list">
        <li class="relations-shortcode__entry"><a href="/group?id=12">Auth Service</a> <span class="relations-shortcode__category">(Service)</span></li>
      </ul>
    </li>
  </ul>
</div>
```

- One list per relation type, in type-name order, and within a type in the order the relations were created.
- The markup is nested native lists, so a screen reader announces each type's label and then its list with the number of items. It has no headings and no IDs, so it fits any slot and any number of cards on one page.
- The far group's category is shown in parentheses when it has one. A group with a blank name is shown as "Group 12". All text is HTML-escaped.
- The `relations-shortcode__*` classes are hooks for `CustomCSS`.
- In a hover card, which the keyboard cannot enter, the names are plain text rather than links.
- With no relation to show it renders nothing (or the `[else]` branch of a block).

### Block form

As a block, each relation renders the body instead of the default entry, inside its own list item. `[item]` reads the relation:

| `[item path=…]` | Value |
|---|---|
| `Name` | The relation's own name (may be empty) |
| `Description` | The relation's description |
| `Type` | The relation type's name |
| `Direction` | `out` or `in` |
| `GroupID` | The far group's ID |
| `GroupName` | The far group's name |
| `GroupCategory` | The far group's category name, or empty |
| `GroupURL` | `/group?id=<GroupID>` |
| `GroupDescription` | The far group's description |
| `GroupMeta` | The far group's Meta: `[item path="GroupMeta.status"]` reads one field |

```
[relations direction="in"]
  <a href="/group?id=[item path='GroupID']">[item path="GroupName"]</a>
  -- [item path="GroupMeta.status" default="no status"]
[else]
  <p>Nothing depends on this.</p>
[/relations]
```

- The body renders against **this** group, as an `[each]` body does: `[property]`, `[meta]` and `[mrql]` inside it read the carrier, not the far group. Read the far group through `GroupName`, `GroupDescription` and `GroupMeta`.
- Attributes are not expanded, so `[using type="group" id="[item path='GroupID']"]` does not work.
- In an `href`, write `/group?id=[item path='GroupID']` rather than `[item path='GroupURL']`: the linter cannot tell that a bare `[item]` holds a safe URL and warns about it.
- Values are spliced in after the body renders, so a relation named `[property path="Name"]` shows that text literally and never runs it.
- `[calc]` reads the relation too, e.g. `[calc expr="item.GroupMeta.cost * 2"]`.

### Scoping

A relation is listed only when the viewer may see **both** of its groups. A group-confined user or guest therefore sees only relations inside their subtree, and a relation to a group outside it is left out without a trace: no count of hidden relations is shown, since that would tell them such groups exist. A guest whose account has no scope group sees none.

For a confined viewer, `[relations]` reads up to 500 of the group's relations in one direction (after the `type` filter), drops the hidden ones, and then applies `limit`. Only a group with more than 500 relations in one direction, most of them hidden, can therefore list fewer than `limit` visible ones.

### Cost

- One query per `[relations]`, which counts one unit of the [per-page query budget](#per-page-query-budget). A block body that reads `GroupDescription` or `GroupMeta` runs a second, small query by primary key and counts a second unit. An identical `[relations]` repeated on the same page is free. Past the budget it renders an error marker.
- In a per-card slot it runs once per card: a group list page shows at most 50 cards, so an inline `[relations]` in `CustomSummary` spends 50 of the default 200 units, leaving 150 for `[mrql]` and the rest. Raise `-mrql-page-query-budget` if a dense page needs more.
- Each lookup is bounded by the MRQL query timeout (`-mrql-query-timeout`), and one that runs out of time renders "lookup timed out".
- `direction="out"` reads the relation table's unique index; `direction="in"` reads `idx__group_relations__to_group_id`, which is built at startup.

### Hiding the built-in panel

A group page already shows Relations and Reverse Relations panels. When a category shows its relations through `[relations]`, turn the built-in ones off under **Section Visibility** in the category form, or in its [section configuration](./custom-templates.md#section-configuration): `{"relations": {"state": "off"}}`, or `forwardRelations` / `reverseRelations` set to `false`.

### Where it renders

`[relations]` works only on groups; on a resource or note it renders `<!-- mr:relations: only groups have relations -->`. To list the relations of a group a resource or note refers to, wrap it in [`[using]`](#using----switch-to-another-entity), e.g. `[using rel="owner"][relations][/using]`.

It renders on display pages and their list cards, the lightbox, hover cards, deferred `[lazy]`/`[details]`/`[autorefresh]` bodies, MRQL results and the template and description previews. Everywhere else it renders `<!-- mr:relations unavailable in this context -->`: share pages, the entity picker and the AI template generation tools. The MRQL result API also renders that comment for a group-confined viewer, because it is bound to run MRQL only and has no subtree to check relations against.

`[relations]` cannot be used inside a `[reload]` button face.

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

`[lazy]` and `[details]` defer across the **standard authenticated template pipeline**, wherever a `Custom*` slot renders a member entity (Group, Resource, or Note). That includes the entity detail pages, list-page `CustomSummary` cards, the dashboard, and hovercards. Everywhere else the body renders **inline** as a graceful fallback -- the content still appears, and `[details]` remains a plain collapsible `<details>` -- either because no deferred signer is installed (public share pages, the live authoring preview, JSON API responses) or because the context is a carrier rather than a member entity (carrier list slots (`CustomListHeader`, `CustomListFooter`), whose `category` / `resource_category` / `note_type` context cannot be reloaded by the deferred endpoint). A `lazy="true"` tab of a [`[tabs]`](#tabs----tabbed-sections) block is a `[lazy]` block and follows the same rules. When a page re-render (a paste upload, or a list refreshed after a bulk edit) reaches a lazy tab the reader had opened and then left, it is not fetched again in the background: it loads fresh the next time its tab is opened. Only the lazy tab on screen re-fetches at once.

### Limitations

- **JavaScript is required** for the deferral to hydrate (as it is for the rest of the app). A `<noscript>` note is shown when it is disabled.
- The deferred body renders against the **entity**, so an `[item]` from an enclosing `[each]` is not available inside a `[lazy]`/`[details]` block (the same non-goal as `[each]`'s element-relative paths).
- The token key is per-process and per-boot by default, so a placeholder minted before a restart -- or, in a multi-process deployment, resolved by a different process -- fails to load and shows a small "could not load -- Retry" message. Set the `TEMPLATE_SIGNING_KEY` environment variable to a shared secret across all processes for behind-a-load-balancer deployments.

## `[tabs]` -- Tabbed sections

Two or more alternative views of the entity in one accessible tab set. Only the open panel takes up space, and the layout height follows its content. Use it where readers should switch between views instead of opening several `[details]` sections at once: a project header with Overview, Open tasks and Files, or a recipe with Ingredients, Steps and Nutrition. For one optional section, `[details]` is still the right tool.

`[tabs]` is a block shortcode and requires a closing `[/tabs]`. Each `[tab label="…"]…[/tab]` directly inside it is one tab. The first tab is open when the page loads.

```
[tabs label="Project views"]
  [tab label="Overview"][property path="Description"][/tab]
  [tab label="Open tasks" lazy="true"][mrql query='type = note AND tags = "todo"' format="list"][/tab]
  [tab label="Files" lazy="true"][mrql query='type = resource' format="list"][/tab]
[/tabs]
```

### Attributes

On `[tabs]`:

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `label` | No | none | The accessible name of the tab list. Set it whenever a page has more than one `[tabs]`, so a screen-reader user can tell them apart |

On each `[tab]`:

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `label` | No | `Tab N` | The tab's visible text and accessible name. It is HTML-escaped and shortcodes in it are not expanded. A blank label shows `Tab 2` and so on |
| `lazy` | No | `false` | `true` makes the tab a `[lazy]` block: its body is rendered on the server only when the tab is first opened. Any other value counts as `false` |

### Eager and lazy tabs

A tab without `lazy="true"` renders with the page, whether it is open or not, so its queries run on every page view. Put `lazy="true"` on every tab after the first that holds an `[mrql]`, a gallery or a plugin data view. A lazy tab costs nothing until a reader opens it, and then one request to the same endpoint `[lazy]` uses, under the same scope checks. A lazy first tab is open on load, so it loads right after the page does.

A lazy tab's body renders against the entity, so an `[item]` from an enclosing `[each]` is not available inside it, the same as in `[lazy]`. On share pages, in the live preview and in list headers, lazy tabs render inline. See [Where deferral applies](#where-deferral-applies).

### Keyboard and screen readers

The tab set follows the WAI-ARIA tabs pattern, with manual activation:

- **Left and Right arrows** move between tabs, wrapping at the ends. **Home** and **End** go to the first and last tab. Moving does not open a tab, so arrowing past lazy tabs does not load them.
- **Enter**, **Space** or a click opens the focused tab.
- The tab list is one stop in the Tab order. **Tab** moves on into the open panel. A panel with no descendants in the Tab order is itself a stop, so its text can be reached.

The tabs wrap onto more rows instead of scrolling sideways, and a long label wraps inside its tab, so a narrow sidebar never scrolls horizontally. The open tab is marked by weight as well as colour.

Without JavaScript, before the script has loaded, and in hover cards, all panels are stacked under their labels as named groups. Hover cards do not show tab controls; lazy panels there can still defer.

### Rules and limits

- Only a `[tab]` directly inside `[tabs]` is a tab. A `[tab]` inside a nested block such as `[conditional]` stays literal text, so tabs cannot be added conditionally: put the `[conditional]` inside a tab instead. A `[tabs]` inside a tab works, and each set responds only to its own tabs.
- Text between the tabs, outside any `[tab]…[/tab]`, is not rendered.
- A `[tabs]` cannot go inside a `[reload]` button face.
- When `[reload]`, `[autorefresh]` or a page re-render replaces the surrounding content, selection is kept only if the same tab set can be matched unambiguously. Repeated identical tab sets in the refreshed content start over on their first tab.
- Use it in `CustomHeader`. In `CustomSidebar`, keep to about three short labels. Do not use it in a per-card slot such as `CustomSummary`: each card would carry its own tab set and run its eager tabs' queries.

The template linter reports a `[tabs]` with no tabs, a `[tab]` without its `[/tab]` or with another `[tab]` inside it, a missing label, a `lazy` value other than `true` or `false`, unknown `[tab]` attributes, text outside the tabs, and any `[tab]` or `[/tab]` that no `[tabs]` reads.

## `[reload]` -- Re-render on demand

A button that re-renders content on the server and swaps it into the page. Which content it refreshes is not fixed at render time: the button carries no token and names no target, and the browser resolves what to refresh at click time by walking up the DOM from the button.

`[reload]` works self-closing or as a block. Self-closing it renders a circular-arrow icon button; a block body becomes the button face instead. A block body that expands to nothing falls back to the icon.

```
[reload]
[reload label="Refresh totals"]
[reload]Refresh[/reload]
```

A typical use is a sidebar that shows a live list. The button refreshes the whole sidebar slot, because it sits outside any `[lazy]`, `[details]` or `[autorefresh]` block:

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

1. **The innermost enclosing `[lazy]`, `[details]` or `[autorefresh]` block.** It already carries a sealed token for its own body, so only that block is re-rendered.
2. **Otherwise the whole custom-content slot the button was written in**, which is any slot the `process_shortcodes` tag renders against a group, resource, or note: CustomHeader, CustomDetailFooter, CustomSidebar, CustomPreview, CustomOwnEntities, CustomSummary, CustomAvatar, CustomHoverCard, CustomCell, or a description. The JSON rendering of the `Custom*` slots does the same for every slot but the CSS, which is how `CustomLightbox` (or the `CustomSidebar` the lightbox falls back to) gets one in the lightbox's info panel. Both wrap such a slot in `<div class="shortcode-region" data-shortcode-region="TOKEN">`, where the token seals the whole raw slot body the same way `[lazy]` seals its own. The wrapper is emitted only when the rendered slot actually contains a reload button and the slot is rendering against a group, resource, or note, so slots without a `[reload]` are byte-for-byte unchanged. It is `display: contents`, so it adds no layout box.
3. **Otherwise the page** (`window.location.reload()`). Anywhere the walk finds neither a deferred block nor a region, the button falls straight through to a page reload without attempting a request. That covers every surface whose slots are rendered without a signing key: the public share server, the category-template live preview, and `CustomMRQLResult` cards on `/mrql`. It also covers the `CustomListHeader` and `CustomListFooter` carrier slots, whose `category` / `resource_category` / `note_type` context the deferred endpoint cannot load by id.

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

## `[autorefresh]` -- Re-render after an edit

Renders its body with the page, then re-renders it on the server whenever an inline editor saves. An inline editor is a `[meta]`, `[pills]` or `[datetime]` with `editable="true"`. Use it around content the server expands once and that does not otherwise follow an edit: `[conditional]` branches, `[meta inline="true"]` values, `[mrql]` results and plugin output. A `[meta]` element without `inline` already updates itself.

```
[pills path="status" editable="true"]
[autorefresh]
  [conditional path="status" eq="done"]<span class="badge">Done</span>[/conditional]
[/autorefresh]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `on` | No | `edit` | What triggers a refresh. `edit` is the only accepted value; anything else renders a `⚠ autorefresh` marker, and the linter flags it |

### What triggers a refresh

A block refreshes after a save when either holds:

- **The save was for the entity the block was rendered for**, wherever the editor sits on the page. An editor in `CustomHeader` refreshes a block in `CustomSidebar`.
- **The editor that saved sits inside the block**, whatever entity it edited. An editor inside a `[using]` or an `[mrql]` item template inside the block counts.
- **An [`[action]`](#action----run-a-plugin-action) button finished a successful run** for the block's entity, or sits inside the block. It counts as a save of that entity.

Saves less than 250 ms apart are coalesced: each save restarts the timer, and when it runs out every block that matched any of them refreshes once. When a block that is refreshing sits inside another that is also refreshing, only the outer one is fetched, since its re-render rebuilds the inner one.

Saves made any other way do not trigger it: the metadata panel, the lightbox edit fields, the API, or another browser tab.

### How it works

The server wraps the rendered body in `<autorefresh-shortcode data-token="TOKEN" data-entity-type="…" data-entity-id="…">`. The token seals the block's **raw** body the same way `[lazy]` seals its own, so a refresh re-evaluates every shortcode in it against the current entity. The element is `display: contents` and adds no layout box. A refresh fetches `POST /v1/shortcodes/deferred`, the endpoint `[lazy]`, `[details]` and `[reload]` use, and behaves like a `[reload]` of a deferred block: the current content stays on screen dimmed while the request is in flight, the Alpine `entity` scope is refreshed along with the markup, and a newer refresh or reload of an overlapping area wins over an older one.

The host is emitted on full pages, in content those pages load through `[lazy]` and `[details]`, and in the lightbox's info panel. Everywhere else the body renders with no wrapper and never refreshes: the public share server, the category-template live preview, `CustomMRQLResult` cards on `/mrql`, and the `CustomListHeader` and `CustomListFooter` carrier slots.

A `[reload]` inside an `[autorefresh]` reloads that block, not the slot around it.

### Limitations

- **Everything in the body is replaced.** An editor inside the block closes when the block refreshes, and one that was open with unsaved input loses it. Wrap only the content that depends on the edited values, and keep other editors outside. If focus was inside the block when it refreshed, it moves to the control in the same position in the fresh content; when there is none, it goes to the refreshed content, or the nearest element beside it that can take focus.
- A failed refresh leaves the previous content as it was, with no message: the edit itself has already been saved.
- An `[item]` from an enclosing `[each]` renders nothing inside it, as inside any block other than `[each]` itself.
- It cannot go inside a `[reload]` button face; it renders a `⚠` marker there, and the linter flags it.
- The token carries the block's source, so each block adds about 1.34x its raw body size to the page, as `[lazy]` does.
- **JavaScript is required** for the refresh. The body itself renders without it.

## `[action]` -- Run a plugin action

A button that runs one registered plugin action (`mah.action`) on the entity the template is rendering, with the permissions of the person who clicks it. It posts to `POST /v1/jobs/action/run`, the endpoint the actions menu uses, so CSRF, roles, subtree scoping, the action's filters and its parameter checks all apply to the click exactly as they do there. Use it to put one step of a workflow next to the data it belongs to, instead of behind the generic actions menu.

```
[action plugin="fal-ai" name="upscale" label="Upscale (Topaz)" confirm="Upscale this image on fal.ai? This is billed." param-model="topaz"]
```

`[action]` is inline only. A `[action]…[/action]` block renders a `⚠ action` marker, and the linter flags it.

In `CustomCSS`, `form[action]` stays a selector; `form[ action]` is an optional equivalent CSS spelling. The attributed `[action plugin="…" name="…"]` form still expands, but category stylesheets (including inline `[mrql]` result CSS) render the unavailable comment instead of a button.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `plugin` | Yes | -- | The plugin's name, as in `[plugin:<name>:…]`. Surrounding spaces are trimmed |
| `name` | Yes | -- | The action's `id` from `mah.action({ id = … })`. Action ids are registered in lowercase |
| `label` | No | the action's `label` | The button text, which is also its accessible name. Blank means the default |
| `confirm` | No | the action's `confirm` | The confirmation question asked before the run, replacing the action's own. `confirm=""` keeps the action's confirmation: a template can add or reword one, never remove it |
| `param-<name>` | No | the action's default | Sets the action parameter `<name>` (see below) |

### Parameters

The button starts from the action's own defaults, the way the action dialog does: an `entity_ref` parameter whose default is `trigger` (or `both`) and whose entity type matches gets this entity, a `selection` default gets nothing, `info` parameters are skipped, and every other parameter gets its `default` (or `false` for a `boolean` and `""` otherwise). Each `param-<name>` attribute then replaces one value. `<name>` must be a parameter the action declares, and the value is converted by its type:

| Parameter type | Accepted value | Otherwise |
|---|---|---|
| `text`, `textarea`, `hidden` | the value as written | -- |
| `select` | one of the parameter's options | `⚠ action` marker |
| `number` | a finite number, e.g. `2` or `0.5` | `⚠ action` marker: "must be a number" |
| `boolean` | exactly `true` or `false` | `⚠ action` marker: "must be true or false" |
| `entity_ref`, `info` | cannot be set | `⚠ action` marker: "cannot be set from a template" |
| not declared | -- | `⚠ action` marker: "no such parameter" |

The result then goes through the action's own checks (required fields, `min`/`max`, options, `show_when`), and the first failure becomes the marker's text, so a mistake shows on the page rather than as a refusal on every click. The marker is only shown to viewers who could run the action. Values are literal: a shortcode written inside an attribute value is not expanded. Parameter values are visible in the page source, as the defaults are in the action dialog.

An action with a visible, required `entity_ref` parameter whose default is `selection` needs a bulk selection and cannot run from a template. It renders the marker "needs a bulk selection and cannot run from a template". A parameter hidden by `show_when` after template overrides does not impose that requirement.

### Who sees the button

The button is drawn only where the click could succeed. Every other case renders one HTML comment, `<!-- mr:action unavailable in this context -->`, the same one a plugin or action that does not exist renders, so a page cannot be read for which plugins are installed or which ones its viewer may run:

- **Guests and other accounts that cannot write.** Running an action is a write.
- **Group-limited accounts, for a plugin the operator has not opened to them.**
- **An entity the action does not apply to:** the action's `entity` is a different type, its `filters` (content types, categories, note types) exclude this entity, or the filters of a visible `entity_ref` exclude the trigger it would receive. In an `[mrql]` result, only matching rows get a button.
- **A missing or disabled plugin or action.**
- **A Description or note text block when login is on.** That text may be written by a user-role account, and a button there would run with the rights of whoever clicks it, behind a confirmation text its author chose. Category, Resource Category and Note Type templates are written by admins and editors and are not affected.
- **Surfaces that cannot run it:** share pages, the live template preview, the description preview, hover cards, `CustomListHeader` and `CustomListFooter` (they render a category, not an entity), the direct MRQL API render (`render=1`), the entity picker, and template generation tools.

The button works on display pages, list cards (including `CustomMRQLResult` cards on `/mrql`, which use `render=list`), `[mrql]` and `[using]` results, `[lazy]`, `[details]` and `[autorefresh]` bodies, and the lightbox. The run endpoint checks everything again on the click: a button for an entity outside a group-limited account's subtree gets the endpoint's 403, as a hand-made request would.

The hover-card exception also applies after a deferred body loads or an `[autorefresh]` block refreshes: the response still omits the button.

`placement` does not matter here. It decides where the actions menu lists an action, and the run endpoint never checks it, so `[action]` accepts any registered action whose entity type matches.

### What happens on a click

- The app asks for confirmation first when required. Declining does nothing. If a refresh removes the button or changes what it would run while the dialog is open, accepting the old confirmation does nothing either.
- While the run is in flight the button is marked `aria-disabled="true"` (it keeps focus) and further clicks are ignored. The text next to the button reads "Running…".
- A synchronous action shows its message (or "Done") next to the button and announces it. A refusal, a validation error or a failed request shows and announces the reason instead; a Job handle returned with a refusal is not followed.
- An `async` action starts a Job; the button reads "Started" and then follows the Job every 2 seconds until it finishes, the button is removed, or 5 minutes pass. At the deadline it reads "Still running. Follow it in the Jobs panel." The Jobs panel is never opened for you, and stopping the watcher does not cancel the Job.
- An action that answers with a redirect navigates there unless it refused the run.
- After a successful run, the `[autorefresh]` blocks for the submitted entity re-render, along with the enclosing block if the button still represents that run. The page is never reloaded, so wrap what the action changes in `[autorefresh]`:

```
[autorefresh]
  [action plugin="project-management" name="pm-set-status" label="Mark done" param-status="done"]
  Status: [meta path="status"]
[/autorefresh]
```

This is for the PM Task note type of the bundled project-management plugin, whose `pm-set-status` action takes a `select` parameter `status`.

- **Focus.** When the button sits inside the block that refreshes, focus moves to the control at the same position in the fresh content. Put the button first in the block (or outside it), so content that appears only after the action cannot shift that position.
- **Status text.** The text next to a button inside a refreshing block goes with the old content. The announcement is not lost: it is made through the page's live region, outside the block.

### Accessibility

The button is a real `<button>` whose visible text is its accessible name. Progress and outcomes are announced through the page's live region: success politely, failures assertively. The status text next to the button is for sighted readers and is not itself a live region, so nothing is announced twice.

### For plugin authors

An action that only makes sense with a selection cannot run from `[action]`. Declare its selection parameter `required = true`, so template authors see a clear error instead of a button that fails. See [Plugin Actions](./plugin-actions.md#running-an-action-from-a-template).

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

## `[inspect]` -- Context Explorer (preview only)

`[inspect]` is an aid for writing templates. Put it anywhere in a slot and the editor's live preview shows a collapsed panel listing what that point of the template can read, each row with a shortcode ready to copy. Remove it when you are done; if you forget, it does no harm, because **it renders nothing outside the live preview**.

```
[inspect]
[inspect path="project"]

[each path="steps"]
  [inspect]
  [item path="title"]
[/each]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | No | (all of Meta) | A Meta dot-path, written as for `[meta path=]`. Limits the Meta section to the subtree at that path. The other sections are unaffected. |

`[inspect]` is inline only. A closing `[/inspect]` is a lint error.

### What the panel shows

The summary names the entity in context, for example `Inspect: group #42`. It adds `inside [using]` when a `[using]` block switched the entity. Opening the panel shows up to three tables:

- **Current [each] element**, only at the top level of an `[each]` body: the element's position (`[item index="true"]`) and every path in the element (`[item path="..."]`), or `[item]` when the element is a plain value. Only the first element gets a panel.
- **Meta**: every key, depth-first in sorted order down to 6 levels, with its JSON type (`string`, `number`, `boolean`, `null`, `object (n keys)`, `array (n items)`), a sample value and a snippet: `[meta path="..."]`, or `[each path="..."][item][/each]` for an array. A key containing `.`, a quote or a bracket cannot be written as a path, so it is listed as "not addressable by path=".
- **Properties**: the entity's own fields for `[property]`, with a sample and a snippet. Time fields get `format="date"`, and `FileSize` gets `format="filesize"`. Association fields such as `Owner`, `Tags` or `Category` are listed by name and kind (`relation`, `list`) only. The panel never reads values through them, so it shows nothing about entities outside what the preview loaded. Credential fields such as a note's share token are always left out. `Meta` itself is covered by the Meta section.

Inside an `[mrql]` block body, a `CustomMRQLResult` template or a `[using]` block, the panel describes the result item or target entity, because that is what the context has switched to.

An `[inspect]` nested in another block inside a loop (for example in a `[conditional]` within an `[each]`) is not at the loop's top level. It shows the entity only, just as an `[item]` there renders empty.

### Copying a snippet

Each snippet is a read-only text field. Press Tab to reach it, then Ctrl+A (Cmd+A) and Ctrl+C (Cmd+C) to copy it. The preview frame is sandboxed, so there is no copy button. The panel is a native disclosure: Enter or Space on its summary opens it, and screen readers announce whether it is open.

### Where it renders

Only in the template editor's live preview (`POST /v1/{category,resourceCategory,noteType}/previewTemplate`), which requires the same rights as saving the template. That includes the MCP preview tools (`previewCategoryTemplate` and its siblings), which call the same route for the same caller; the panel shows them nothing that a `[meta]` in the same preview would not. Everywhere else (entity pages, list cards, hover cards, the lightbox, share pages, deferred `[lazy]`/`[details]` bodies, MRQL API renders and the AI template tools) `[inspect]` renders as the empty string, not a comment. The page reads exactly as if it were absent. It never runs a query.

### Limits

- 200 rows per panel across its sections. Past that, a note says how many paths were not shown; narrow the panel with `path=`.
- 10 panels per preview. An `[inspect]` inside an `[mrql]` body runs once per result, so later panels are replaced by one note.
- Samples are cut at 80 characters.
- `[inspect]` cannot be used inside a `[reload]` button face.

## `[adjacent]` -- Previous and next

`[adjacent]` adds a Previous and a Next link to the entity's neighbours, so a reader can step through a scanned book's pages, a photo set, a run of meeting notes or a client's invoices without going back to the list.

```
[adjacent series="true" label="Pages"]
[adjacent label="Meetings"]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `series` | No | `false` | `true` or `false`. Resources only: `true` steps through the other members of the resource's [Series](../user-guide/managing-resources.md), whatever their owner or category. |
| `label` | No | `Previous and next` | The accessible name of the navigation landmark. A name already used on the page gets a number (`Previous and next (2)`), so repeated or per-card `[adjacent]`s stay distinct; a label of your own reads better. |

`[adjacent]` is inline only. A closing `[/adjacent]` is a lint error, and so is a `series` value other than `true` or `false`.

### Which entities are siblings

- **Series mode** (`series="true"`, resources only). The resources in the same Series. A resource with no Series renders nothing. On a group or a note, `series="true"` renders an error marker.
- **Default mode.** Entities of the same type with the same owner and the same category column: the resource category for resources, the note type for notes and the category for groups. A group's owner is its parent group.
  - An entity with no owner pairs with the other ownerless entities of the same category, which covers top-level groups and an unowned inbox.
  - A note or group with no type or category pairs with the others that have none.

### Order

Siblings are always in creation order: `created_at`, then `id` to break ties. The order is total and stable, and editing an entity never moves it.

There is no name order: names sort as text, so "Chapter 10" would come before "Chapter 2". For a curated order, put the items in a Series and create them in reading order, or create them in the order you want to read them.

### Output

```html
<nav class="shortcode-adjacent ..." aria-label="Pages">
  <a href="/resource?id=12" rel="prev" ...><span aria-hidden="true">←</span> Previous: Page 1</a>
  <a href="/resource?id=14" rel="next" ...>Next: Page 3 <span aria-hidden="true">→</span></a>
</nav>
```

- Each link's accessible name is its visible text; the arrows are hidden from assistive technology. Previous comes first in both reading and Tab order.
- With only one neighbour only that link renders, and Next stays right-aligned. With neither, `[adjacent]` renders nothing at all, not an empty landmark.
- A sibling with a blank name is shown as its type and ID, for example "Resource 14". Names are always HTML-escaped.
- Inside `[using]` or an `[mrql]` item template, it navigates the entity the context switched to.

### Scoping

Siblings are read as the viewer. A group-confined user or guest only ever sees siblings inside their subtree:

- The group at the root of their subtree, and every top-level group, has no visible siblings and renders nothing.
- A Series member outside the subtree is skipped, and the next visible member is linked.
- Unowned resources and notes are outside every subtree.
- An entity the viewer cannot see and one that does not exist both render nothing, so `[adjacent]` cannot be used to probe for IDs.

Only the ID and name of a visible sibling reach the page.

### Cost

- Each `[adjacent]` runs at most three small queries: the current row by primary key and one seek in each direction. In default mode each seek is a single index probe, so its cost does not grow with the number of siblings; Series mode is the exception below.
- It counts one unit of the [per-page query budget](#per-page-query-budget); an identical `[adjacent]` repeated on the same page is free. Past the budget it renders an error marker.
- Each lookup is bounded by the MRQL query timeout (`-mrql-query-timeout`), and a lookup that runs out of time renders "lookup timed out".
- Put it in CustomHeader or CustomSidebar. In a per-card slot (CustomSummary, CustomMRQLResult) it costs one budget unit per card.
- Default mode reads three composite indexes (`idx_resources_adjacent_created`, `idx_notes_adjacent_created`, `idx_groups_adjacent_created`), which add a small write cost on inserts and on changes of owner, category or creation time. On SQLite a large library builds them once at startup, before serving. On PostgreSQL they are built concurrently in the background, and `[adjacent]` is correct but slower until they exist.
- Series mode uses the Series index and sorts that Series' members, so its cost grows with the size of the Series (typically tens of rows).

### Where it renders

On entity display pages (their slots and list cards), in deferred `[lazy]`/`[details]`/`[autorefresh]` bodies, and in the template editor's and the description editor's live previews. Everywhere else it renders the HTML comment `<!-- mr:adjacent unavailable in this context -->`: the MRQL render API, share pages, the lightbox, hover cards (a tooltip the keyboard cannot enter), the entity picker, the AI template generation tools and the CLI.

`[adjacent]` cannot be used inside a `[reload]` button face.

## `[similar]` -- Lookalike Thumbnails

`[similar]` shows the resources that look like the current one: duplicates, re-exports, crops and light edits. It reads the perceptual-hash pairs the hash worker has already stored, the same pairs behind the resource page's Similar Resources panel and MRQL's `SIMILAR TO`. Use it where that panel is not: the lightbox, the sidebar, the detail footer, or a group page through `[using]`.

```
[similar]
[similar max-distance="2" limit="12" size="sm" label="Possible duplicates"]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `max-distance` | No | the hash similarity threshold | The largest perceptual distance shown, a whole number from 0 to 11. Unset, the threshold from the settings applies, the one the Similar Resources panel uses. |
| `limit` | No | `8` | The most thumbnails shown, a whole number from 1 to 24. The nearest come first; equal distances are in ID order. |
| `size` | No | `md` | Thumbnail edge: `sm` is 48 px, `md` 96 px, `lg` 200 px. |
| `label` | No | `Similar resources` | The visible caption, which is also the strip's accessible name. Blank means the default. |

`[similar]` is inline only and has no `id` attribute: it always shows the lookalikes of the current resource. An invalid `max-distance`, `limit` or `size` renders an error marker, and the linter reports the same message.

### Tiers

Each thumbnail carries a badge with its tier and distance, with the same cut-off as the Similar Resources panel:

| Distance | Badge | Meaning |
|----------|-------|---------|
| 0 to 2 | `Duplicate · d` | A near-certain duplicate |
| 3 and above | `Similar · d` | Visually similar |

### Output

```html
<figure class="similar-strip my-2" data-similar-for="42">
  <figcaption class="text-xs font-semibold mb-1">Similar resources</figcaption>
  <ul class="flex flex-wrap gap-2" role="list">
    <li class="flex flex-col items-center gap-0.5">
      <a href="/resource?id=17" ...><img src="/v1/resource/preview?id=17&amp;width=96&amp;height=96&amp;v=..." width="96" height="96"
           loading="lazy" alt="Beach sunset (2), near-certain duplicate, distance 1" ...></a>
      <span ... aria-hidden="true">Duplicate · 1</span>
      <a href="/resource/compare?r1=42&amp;r2=17" ...>Compare<span class="sr-only"> Beach sunset (2) with Beach sunset</span></a>
    </li>
  </ul>
</figure>
```

- The caption names the strip. Each image's alt text gives the name, the tier and the distance, so the badge is hidden from assistive technology. A resource with a blank name is shown as "Resource 17".
- The Compare link opens the side-by-side compare page. Its accessible name starts with the visible word "Compare" and names both resources.
- The badges carry their own background, and the caption and links take the slot's text colour, so the strip reads on the white page and on the dark lightbox panel.
- Names are always HTML-escaped and are never expanded as shortcodes.

### When it renders nothing

- With no stored pairs within the distance, or when the viewer may see none of them, `[similar]` renders nothing. It never hashes images while a page renders, and it does not fall back to exact hash matches. A resource that is not an image has no pairs.
- On a group or a note it renders an error marker. Use `[using]` to point it at a resource.
- On share pages it renders `<!-- mr:similar unavailable in this context -->`.

### Another resource: `[using]`

```
[using type="resource" id-path="cover"][similar label="Lookalikes of the cover"][/using]
```

Inside `[using]` the strip belongs to the resolved resource: its Compare links compare against that resource and name it. A cover the viewer cannot see resolves to no target, so the block renders its `[else]` branch or nothing.

### Scoping and cost

- Lookalikes are read as the viewer. A group-confined user or guest only sees resources inside their subtree, and an unowned resource is outside every subtree. "No lookalikes" and "lookalikes you may not see" look the same.
- Each `[similar]` counts one unit of the [per-page query budget](#per-page-query-budget), and an identical one on the same page is free. It runs one indexed `SIMILAR TO` query and one lookup of at most 24 pair rows, so its cost follows the size of the resource's lookalike cluster, not of the library.
- Put it in `CustomSidebar`, `CustomLightbox` or `CustomDetailFooter`. In a per-card slot it costs one budget unit per card.

### In the lightbox

The lightbox renders `CustomLightbox` (or, when that is empty, `CustomSidebar`) when it opens a resource, so `[similar]` shows the image's lookalikes next to it. Its links are ordinary links: following one leaves the lightbox for the resource or compare page, like the lightbox's own "Open resource page" link.

`[similar]` cannot be used inside a `[reload]` button face.

`similar` is now a reserved shortcode name: text written as `[similar …]` in an existing template renders the strip (or an error marker on a group or note) instead of showing as text.

## `[calendar]` -- Month grid of dated entities

`[calendar]` shows one month as a grid of the entities an MRQL filter matches, placed on days by a date field. Use it for event, journal, task and deadline categories: a group sidebar that shows the month's notes by due date, or a daily-log calendar.

```
[calendar mrql='type = "note"' date="meta.due"]
[calendar mrql='type = "resource"' date="created" month="2026-03" scope="global"]
[calendar mrql='type = "note" AND noteType = "Task"' date="meta.due" scope="parent"]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `mrql` | Yes | | An MRQL **filter**, naming exactly one entity type through a top-level `type = …` (`type = "note" OR type = "group"` names none). It cannot carry `GROUP BY`, `ORDER BY`, `LIMIT`, `OFFSET`, `SCOPE` or `$parameters`: the calendar adds its own month range, order and limits. |
| `date` | No | `created` | The field that places an entity on a day: `created`, `updated`, or `meta.<path>` (letters, digits and underscores, dot-separated, such as `meta.event.start`). |
| `month` | No | `current` | `current`, the server's current month, or `YYYY-MM` with a year from 1000 to 9999. |
| `scope` | No | the entity's group | As on [`[mrql]`](#mrql----inline-query-results): `global`, `parent`, `root` or a group ID. Any other value, `entity` included, is the entity's own group. |

`[calendar]` is inline only; a `[/calendar]` closer is a lint error. An invalid attribute renders an error marker, and the linter reports the same message. The linter also checks the two queries the calendar runs.

### What a month shows

- The week starts on Monday. Days of the neighbouring months are empty cells.
- Each day with entities shows its count as a link to that day's list on the MRQL page, and up to 3 linked titles. A day with more has a "+k more" link to the same list. The day link carries the same filter, date range and scope as the grid, so it lists exactly what the cell counted.
- The caption names the month and the total, such as "March 2026 (12 items)".
- Today's cell is highlighted and marked `aria-current="date"`.

**Items mode and counts mode.** A month of up to 100 entities is listed in one query, and its titles and counts come from the same rows. A month with more than 100 runs a second, per-day count query and shows counts only, with the note "Showing counts only: more than 100 items this month." That count query returns at most 1000 distinct date values, earliest first; past that the grid adds "Counts may be incomplete".

### Meta dates

A Meta value counts only when it is a JSON string that starts `YYYY-MM-DD`: a JSON-Schema `format: date` value such as `"2026-03-04"`, or an RFC3339 date-time such as `"2026-03-04T23:30:00-05:00"`, which is how `[datetime]` stores one. Its day is the date **as written**, the first ten characters, whatever zone the time carries. Numbers, booleans and other strings are not counted, not listed and not in the day links. Each query carries the guard `meta.<path> ~ "????-??-??*"` for this.

### Which day an entity is on

A day is a calendar date on the server's local clock, the clock `START_OF_MONTH()` and `[datetime]` use. `month="current"` and the today highlight read it too.

- **SQLite, `created`/`updated`.** Timestamps are stored with the server's offset, and the month and day ranges compare them as stored, so an entity is on its local date. A timestamp stored with another offset (written before the server's timezone changed, or kept from an imported group archive) is counted by the day link of its stored date but drawn on its date in the server's zone, which can be the neighbouring day.
- **PostgreSQL, `created`/`updated`.** A date literal is read at midnight in the database session's `TimeZone`. Set the database `TimeZone` to the server's zone (both default to UTC in the container setup) or days are shifted by the difference.
- **Meta.** The date as written; no zone applies.
- **The one deviation.** In counts mode only, SQLite's per-day bucket normalises timestamps to UTC. On a SQLite server not running in UTC, a dense month's counts can put an entity within the UTC offset of midnight on the neighbouring day, and one on the first or last day of the month can drop out of both months' counts. Its day link still lists it under its local date. Items mode has no deviation.

### Month navigation

Prev and Next buttons swap the month in place, without a page load, through the same endpoint `[lazy]` and `[reload]` use. Focus stays on the button that was pressed, and the new month is announced. The buttons carry a sealed copy of the shortcode with the adjacent month, so the filter cannot be changed from the browser.

- They render on display pages, cards, the lightbox and deferred bodies. Share pages, hover cards, the live template preview and list headers show the authored month without them.
- There is no Prev at 1000-01 and no Next at 9999-12.
- A `[reload]` around a calendar, or a `[lazy]` it sits in, re-renders the authored month, not the one navigated to.

### Output

```html
<calendar-shortcode class="calendar-shortcode" role="group" aria-labelledby="cal-…-cap" data-prev-token="…" data-next-token="…">
  <div class="calendar-shortcode-nav">
    <button type="button" data-calendar-nav="prev">‹ <span class="sr-only">Previous month, </span>February 2026</button>
    <button type="button" data-calendar-nav="next"><span class="sr-only">Next month, </span>April 2026 ›</button>
  </div>
  <table class="calendar-shortcode-grid">
    <caption id="cal-…-cap">March 2026 <span class="calendar-shortcode-total">(12 items)</span></caption>
    …
    <td class="calendar-shortcode-cell">
      <a class="calendar-shortcode-daylink" href="/mrql?q=…">…4…5…</a>
      <ul class="calendar-shortcode-items"><li><a href="/note?id=12">Dentist</a></li>…</ul>
      <a class="calendar-shortcode-more" href="/mrql?q=…">+2 more<span class="sr-only"> items on March 4</span></a>
    </td>
    …
  </table>
</calendar-shortcode>
```

- The group is named by the table caption, so the buttons and the grid are announced together. Column headers are full weekday names for assistive technology.
- A day link reads "March 4, 5 items"; an empty day reads "March 5, no items" and is not a link. The visible number is part of each name.
- Long titles are clipped on screen and read in full. Titles are always HTML-escaped.

### Scoping and cost

- Counts and titles are read as the viewer. A group-confined user or guest only sees entities inside their subtree, whatever `scope` says, and the day and title links open pages that are scoped the same way. A navigation token replayed for an entity outside the viewer's subtree is refused.
- An entity with no owning group and the default scope matches nothing: the grid is empty and has no links.
- Each render costs one unit of the [per-page query budget](#per-page-query-budget), or two in counts mode. An identical calendar on the same page is free. Each Prev/Next click costs one or two queries on its own request.
- Put calendars in `CustomSidebar` or `CustomHeader`. In a per-card slot (`CustomMRQLResult`, `CustomAvatar`, `CustomCell`) with the entity's scope, every card runs its own queries: a list of 150 cards exhausts the default budget, and the remaining cards render the budget error. A fixed `scope` lets identical calendars share one result.
- Both queries are a month-range predicate. `created`/`updated` use the timestamp indexes. No index serves a `meta.*` date range: [indexed metadata keys](./meta-schemas.md#indexed-metadata-keys) of the text kind serve equality only. On a large library, narrow a `meta.*` calendar's filter (an entity type plus a category, note type or tag) and keep the default entity scope rather than `scope="global"`, or every render and every Prev/Next click reads every row the filter matches.

`[calendar]` cannot be used inside a `[reload]` button face. On share pages it renders `<!-- mr:calendar unavailable in this context -->`.

`calendar` is now a reserved shortcode name: text written as `[calendar …]` in an existing template renders the calendar (or an error marker) instead of showing as text.

## `[pivot]` -- Cross-tab

`[pivot]` counts the entities an MRQL filter matches by two fields at once and shows the result as a table: one row per value of `rows`, one column per value of `cols`, a count (or a sum) in each cell, and row, column and grand totals. Use it for dashboards an `[mrql … GROUP BY]` list or a one-dimensional chart cannot show: tasks by status and assignee, resources by content type and year, notes by status and owner.

```
[pivot query="type = resource" rows="contentType" cols="created.year"]
[pivot query="type = note AND meta.status IS NOT NULL" rows="meta.status" cols="owner" caption="Open work by owner"]
[pivot query="type = resource" rows="contentType" cols="created.year" value="sum" field="fileSize"]
```

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `query` | Yes | | An MRQL **filter** naming exactly one entity type through a top-level `type = …`. It cannot carry `GROUP BY`, `ORDER BY`, `LIMIT`, `OFFSET`, `SCOPE` or `$parameters`: the pivot adds its own grouping and order. |
| `rows` | Yes | | The row field. See [Dimensions](#dimensions). |
| `cols` | Yes | | The column field, in the same forms. It must be a different field from `rows`; `group` and `groups` count as the same field. |
| `value` | No | `count` | `count` (entities per cell) or `sum` (of `field`). Averages, minimums and maximums are left out because their totals cannot be built by adding cells. |
| `field` | With `value="sum"` | | The field to add up: `meta.<path>`, or `fileSize`, `width` or `height` on resources. Set without `value="sum"` it is ignored, and the linter says so. |
| `scope` | No | `entity` | As on [`[mrql]`](#scope): `entity` (the entity's own group), `parent`, `root`, `global`, or a group ID. The linter warns on any other value, which counts in the entity's own group. |
| `max-rows` | No | `20` | How many row values are shown, most items first. At most 50; an invalid value or one below 1 means the default. |
| `max-cols` | No | `10` | How many column values are shown, most items first. At most 20, with the same fallback. |
| `rows-label` | No | the `rows` field without `meta.` | The heading of the row-label column, also used in the caption and the link names. |
| `cols-label` | No | the `cols` field without `meta.` | The name of the column field in the caption and the link names. |
| `caption` | No | `Count by <rows> and <cols>`, or `Sum of <field> by <rows> and <cols>` | The table caption, which also names the scroll region. |

A blank label or caption counts as unset. `[pivot]` is inline only; a `[/pivot]` closer is a lint error. An invalid attribute renders the red error box and the linter reports the same message once, at `query=`. The linter also checks the composed query, on SQLite and PostgreSQL alike.

### Dimensions

`rows` and `cols` each accept one of:

- **A Meta path**, `meta.<key>[.<key>…]`: `meta.status`, `meta.project.phase`.
- **A field of the entity**: a text field such as `name`, `contentType` or `originalName`; `fileSize`, `width` or `height` on resources; `shared` on notes, shown as `yes` and `no`.
- **A relation, by name**: `owner`, `groups` (or `group`), `tags`, `notes` and `resources` where the entity has them, and `parent` and `children` on groups.
- **A date bucket of a timestamp**: `created`, `updated`, or a note's `startDate` and `endDate`, followed by `.day`, `.week`, `.month` or `.year`, such as `created.month`. Buckets are shown oldest first.

These are refused, with a message saying what to use instead:

- a bare timestamp (`created`): pick a date bucket;
- an ID field (`id`, `category`, `noteType`): they would head the table with numbers. Filter by the category or note type in `query` and pivot by a Meta field;
- a traversal (`owner.name`, `tags.count`) and `similarImages`.

Using a field the entity does not have, such as `shared` on a resource, is refused with `"shared" is not a resource field`.

### What a cell holds

- **Equal values share a row or column.** Values that differ only in ASCII letter case (`Open` and `open`) are one row, because MRQL's `=` compares text that way. So are a Meta number and the same digits written as a string (`3` and `"3"`), and two related groups with the same name: the result carries a group's name, not its ID, so the table cannot tell them apart. The spelling shown is the one in the combination with the most items.
- **Booleans in Meta** show as `1` and `0` on SQLite (merged with the numbers 1 and 0 and the strings "1" and "0") and as `true` and `false` on PostgreSQL (merged with the strings "true" and "false").
- **Missing values** go in a `(none)` row or column, shown last: a missing Meta key, an entity with no owner or no tags, or an owner the viewer cannot see. An empty string is `(empty)`.
- **Sums** add numbers. How other Meta values count depends on the database. PostgreSQL adds a string only when it is wholly a plain decimal (`"2"`, `"-1.5"`) and skips everything else. SQLite reads the number a string starts with (`"2"` and `"2 hours"` both add 2, `"x"` adds nothing) and adds a JSON `true` as 1. A cell with no number to add shows 0.
- **Numbers** are written as `[calc]` writes them: plain decimals, at most ten fraction digits, no thousands separators. A combination with no entities shows a muted `0`.

### Totals

The Total column holds each row's total and the Total row each column's; the bottom-right cell is the grand total. Totals include the values hidden by `max-rows` and `max-cols`.

A multi-valued relation (`tags`, `groups`, `notes`, `resources`, `children`) puts an entity in one row or column per value, so adding across it would count that entity several times:

- with `rows` multi-valued, the Total row and the grand total are left out;
- with `cols` multi-valued, the Total column and the grand total are left out. Each row's items are spread over several columns, so a row total would overcount for the same reason.

### Caps and truncation

Rows are ranked by their total, highest first, with ties broken by label, and the top `max-rows` are shown; columns likewise with `max-cols`. A note under the table says how many values are not shown. A date bucket is still shown oldest first among the values kept.

One query reads at most 5000 (row, column) combinations, those with the most items first (with ties broken by the two fields, so a render is stable). Past that the table says "Only the 5000 combinations with the most items were counted", combinations it did not read show `—` (read as "not counted"), totals are marked `+` (read as "or more"), and nothing is linked. Narrow the query.

### Links

In count mode every cell and total that can be matched exactly links to the [MRQL page](./mrql.md) for what it counted, in the scope the table was counted in: `(<query>) AND meta.status = "open" AND owner = "Inbox"` for a cell, one condition for a row or column total, and the bare `query` for the grand total. The link's count equals the cell's.

No links appear:

- in sum mode;
- in a truncated table;
- for date buckets and the `(none)` row or column;
- for a Meta row or column that held a JSON number with a fraction on SQLite, which `=` cannot match by its label;
- for a multi-valued relation's row or column that merged two values in one cell, such as two groups named Inbox and inbox, where one note in both would be counted twice by the cell and once by the link.

One known mismatch remains: PostgreSQL's `LOWER` folds non-ASCII letters too, so `É` and `é` are two rows there whose links each list both.

### Output and accessibility

```html
<div class="mrql-pivot my-2">
  <div class="relative overflow-x-auto …" role="region" aria-label="Count by status and assignee" tabindex="0">
    <table class="min-w-full text-sm border border-stone-200">
      <caption>Count by status and assignee</caption>
      <thead><tr><th scope="col">status</th><th scope="col">ann</th><th scope="col">bob</th><th scope="col">Total</th></tr></thead>
      <tbody>
        <tr><th scope="row">open</th><td><a href="/mrql?q=…" aria-label="3: status open, assignee ann">3</a></td>…</tr>
        …
      </tbody>
      <tfoot><tr><th scope="row">Total</th>…<td><a href="/mrql?q=…" aria-label="10: total of all">10</a></td></tr></tfoot>
    </table>
  </div>
  <p>1 more status value is not shown; the totals include them.</p>
</div>
```

- The scroll region is reachable with Tab and named by the caption, so a wide table can be scrolled from the keyboard.
- Every column header is `scope="col"` and every row header, the Total row included, `scope="row"`.
- Each link's name starts with its visible number and names the row and column it counts.
- Colour never carries meaning on its own: an empty combination is a written `0`.
- Labels and the caption are always HTML-escaped.

### Scoping and cost

- Counts and labels are read as the viewer. A group-confined user or guest only counts entities inside their subtree, whatever `scope` says, and a group outside their subtree is never named; an entity whose owner they cannot see falls in `(none)`. The links open the MRQL page, which applies the same confinement.
- Each `[pivot]` is one SQL statement of at most 5001 rows and costs one unit of the [per-page query budget](#per-page-query-budget). An identical pivot on the same page is free.
- No index serves a `meta.*` grouping. On a large library, narrow `query` by type plus a category, note type, owner or date range, and keep the default entity scope rather than `scope="global"`. Two multi-valued dimensions multiply the rows the query joins; see the junction-join note in the [MRQL reference](./mrql-reference.md).
- Put pivots in `CustomHeader`, `CustomSidebar` or `CustomListHeader`, not in a per-card slot, where every card runs its own query.

`[pivot]` cannot be used inside a `[reload]` button face. On share pages it renders `<!-- mr:pivot unavailable in this context -->`.

`pivot` is now a reserved shortcode name: text written as `[pivot …]` in an existing template renders the table (or an error box) instead of showing as text.

## `[comment]` -- Comment out a section

`[comment]...[/comment]` removes everything between the tags on the server, before any other shortcode is read. Nothing inside runs: a commented-out `[mrql]` never queries, and a half-written `[conditional]` cannot pair with a closer outside the comment. None of it is rendered: it is absent from the HTML of the slot, the description, list-card and search previews, and the live preview. The edit form still holds the source, as it does for the rest of the template.

An HTML comment works differently. `<!-- [mrql ...] -->` still runs the query, and the comment and its expanded output are sent to the browser. The editor lint marks a shortcode inside an HTML comment with a hint pointing to `[comment]`.

```
[comment reason="old sidebar"]
  <aside>[mrql query='type = "note"' format="list"]</aside>
[/comment]
```

### Rules

- **Where it applies:** every custom template slot (including `CustomCSS`), Template Partials, entity descriptions, and every body the engine expands while rendering: a partial's content, an `[each]` body, and an MRQL result template. Descriptions and note text blocks drop comments wherever they are rendered for reading, including where shortcodes are not processed: list-card previews, hover cards, shared notes and their blocks, the note text view, the resource Description row, search results, and `[property path="Description"]`. Views that show the source itself keep it: the edit form and the group compare page's Description diff.
- **Nesting:** comments nest, so a section that already holds a comment can be commented out. `[comment] A [comment] B [/comment] C [/comment]` removes all of it.
- **Unclosed opener:** a `[comment]` without `[/comment]` hides everything after it in that slot. The lint reports it as an error.
- **Stray closer:** a `[/comment]` with no opener is removed. The lint reports it as a warning.
- **Attributes:** the opener accepts any attributes, such as `reason="..."`, and ignores them. The opener ends at its first `]`, even inside quotes, so an attribute value cannot contain `]`, and it must fit on one line: `[comment` with a line break before its `]` is plain text.
- **Whitespace:** only the text from `[comment` to `[/comment]` is removed. Line breaks around it stay.
- **Joined tags:** if removing a comment joins the text around it into a new `[comment]` or `[/comment]`, that is removed too, for up to 16 rounds. `[comm[comment][/comment]ent]` becomes `[comment]`, which then hides what follows.
- **CSS fields:** each CSS field is handled on its own before the fields are joined into one stylesheet, so an unclosed `[comment]` in `CustomCSS` does not hide a companion field such as `CustomHeaderCSS`.
- **Lint:** the lint skips the contents of a comment, so unfinished shortcodes inside it are not flagged.
- **No escape:** there is no way to write a literal `[comment]` on the page.

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

A `[conditional]` on a query result, inside `[lazy]` so that both the count query and the list query wait until the section scrolls into view:

```html
[lazy]
  [conditional mrql='type = note AND tags = "todo"' gt="0"]
    <h3>To-dos</h3>
    [mrql query='type = note AND tags = "todo"' format=list limit=5]
  [else]
    <p class="text-stone-500">No to-dos.</p>
  [/conditional]
[/lazy]
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
