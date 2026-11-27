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

In stylesheets, a bare built-in opener immediately after an ASCII letter, digit, `-`, `_`, `*`, `]`, or `|` stays literal (e.g. `form[action]` and `.w[tabs]`). An attributed shortcode still expands. `[comment]` is stripped before that rule applies.

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
- `inline=true` -- render the bare value instead of the element
- `editable=true` -- inline edit button (ignored with `inline`)
- `hide-empty=true` -- remove empty output entirely
- with `inline`: `format=date|datetime|time|filesize`, `layout="Jan 2, 2006"`, `raw=true` (skip escaping), `default="…"`

Behavior:

- Uses the current `MetaSchema` slice for schema-aware rendering.
- Supports deep paths.
- Respects labeled enums and `x-display` behavior on the resolved schema slice.
- If no schema exists, falls back to plain formatted JSON value display.

Use `[meta]` when you want the category schema to stay the source of truth for display and editing.

**Inside an HTML attribute, `inline=true` is mandatory.** The default form expands
to a `<meta-shortcode>` element, and an element cannot nest inside an attribute
value -- its own quotes close the attribute early. Inline output is HTML-escaped
(quotes included), so a Meta value carrying a `"` cannot break out of a *quoted*
attribute:

```html
<a href="/archive/[meta path='slug' inline='true']" title="[meta path='blurb' inline='true']">Open</a>
```

Use single quotes for the inner shortcode attributes. `raw=true` disables escaping
and must never be used inside an attribute. Inline mode is also the form that works
on share pages and in `CustomMRQLResult`, where the widget does not hydrate.

**Escaping is not general attribute safety**, and the linter warns on the four
shapes where it fails: an unquoted attribute (a value with a space adds
attributes), a whole `href`/`src` (a `javascript:` value runs), an `on*` handler
(the HTML parser undoes the escaping before the JS parser reads it), and `style`
(CSS injection). Never place `[meta inline]` in any of them. Templates are
authored by admins/editors; Meta is written by anyone who can edit the entity.

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

### `[excerpt]`

Use for a short preview of long text on cards, panels, hover cards and `[mrql]`/`CustomMRQLResult` items: a note's gist, a resource's description, a Meta summary string. Prefer it to `[property path="Description"]` on any card, which prints the whole Markdown source with mention markers and has no length bound.

Attrs:

- `words` -- 1 to 200, default 30. Also capped at 12 Unicode code points per allowed word (CJK, URLs); the cut backs up over partial accents and joined emoji.
- `field` -- one string field of this entity, default `Description`. No dot paths.
- `path` -- a Meta string instead (not with `field`).
- `more="Read more"` -- link to the entity's page, only when the text was cut. Its accessible name gets the entity's name appended.
- `default="…"` -- escaped text when there is nothing to show.

Rules:

- Template comments and shortcodes are stripped without running them; mentions are shown as their names. Bare Markdown link/image labels, including trailing whitespace, are left for Markdown. The preview is one escaped line ending in `…` when cut, including any preview from a source longer than 16 KiB.
- After Markdown conversion, it drops HTML comments, tags and all attributes (including image `alt` text), and skips the contents of `<script>`, `<style>`, `<textarea>`, `<title>`, `<xmp>`, `<iframe>`, `<noembed>`, `<noframes>`, `<noscript>`, `<plaintext>`, `<template>` and elements with a `hidden` attribute. It does not apply CSS, `aria-hidden` or other browser visibility rules.
- Without `more`, use it in text or inert quoted attributes such as `title="[excerpt words='25']"`, `data-*` and `aria-label`. Do not place it in script-bearing attributes, `srcdoc` or URL schemes; HTML escaping does not make those safe. A nonblank `more` in an attribute name or value is a lint error because it can emit link markup.
- It never loads data: preview a related entity inside `[using]` or an `[mrql]` block.
- No `more` link inside a `[reload]` face, in hover cards, or on the public share page.

```html
<p class="text-sm text-stone-600">[excerpt words="25" more="Read more"]</p>
[mrql query="type = note ORDER BY updated DESC" limit="5"]<p><a href="[link]">[property path="Name"]</a>: [excerpt words="12"]</p>[/mrql]
```

### `[calc]`

Use for a number derived from other fields: a line total, a discounted price, a remaining budget, megapixels. Never store a redundant field for these; it goes stale on the next edit.

Attrs:

- `expr` -- required. Operands `meta.<path>`, `property.<Field>` (numeric struct fields only) and, at the top level of the item branch of `[each]` or `[relations]`, `item` / `item.<path>`; numeric literals (plain decimals, no exponent); `+ - * / %`, parentheses; functions `min`, `max` (1-8 args), `abs`, `round(x[, digits])` (half away from zero, digits 0-10), `coalesce(a, b, …)` (first argument that has a value).
- `decimals=0..10` -- fixed fraction digits, trailing zeros kept. Unset: up to 10 digits, trailing zeros dropped.
- `format=number|grouped|percent` -- `grouped` gives `1,234.50` (not locale-aware); `percent` multiplies by 100 and appends `%`.
- `default="…"` -- escaped text when there is no numeric result.

Rules:

- A Meta string counts only when it is wholly a decimal number of at most 64 bytes (`"12.50"` yes, `"12,5"` no). Missing, null, boolean, object, array, division by zero and overflow give no value, which renders `default` unless `coalesce` catches it: `coalesce(meta.discount, 0)`, `coalesce(meta.a / meta.b, 0)`.
- Syntax errors, unknown names (`qty` without `meta.`), unknown functions, wrong arity, a bad `decimals`/`format`, or an expression past 512 bytes / 128 tokens / 32 levels / 32 references render the `⚠ calc` marker; the linter flags them.
- Only Meta keys of ASCII letters, digits and `_`, not starting with a digit, can be referenced (`meta.unit-price` is `meta.unit - price`). Design the schema with `snake_case` keys for anything `[calc]` reads.
- Output is a bare number, so it works inside attributes (`<meter value="[calc …]">`). `item` operands have no value outside the direct item branch (including inside nested blocks and in `[else]`), as for `[item]`.
- It never queries, so it is safe on every card of a long list, on share pages and in `CustomMRQLResult`.
- Accessibility: label derived numbers (a `<th scope="col">` header, or "Total due: …"), give a `<meter>` an `aria-label` and fallback text, and prefer `default="not set"` over a lone dash outside tables.

```html
[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="not set"]
[each path="lines"]<tr><td>[item path="name"]</td><td>[calc expr="item.qty * item.price" decimals="2"]</td></tr>[/each]
<meter min="0" max="1" value="[calc expr='meta.spent / meta.budget' default='0']" aria-label="Budget used">[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter>
```

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

### `[lazy]` and `[details]`

Both are block shortcodes that **defer server-side rendering** of their body to a small on-demand fetch, keeping expensive shortcodes (especially `[mrql]`) out of the initial page render.

- `[lazy]...[/lazy]` -- renders when the block scrolls into view. Reach for it in `CustomSummary` (per-card slots on list pages) so an entity-scoped `[mrql]` runs only for cards the user actually sees.
- `[details summary="..." open="true|false"]...[/details]` -- a keyboard/screen-reader-accessible disclosure that renders its body the first time it is opened.

Deferral applies only on the main display pages; on share pages, the live preview, and the carrier slots (`CustomListHeader`, `CustomListFooter`) the body renders inline (and `[details]` stays a plain collapsible `<details>`). Requires JavaScript; the deferred body renders against the entity, so an `[item]` from an enclosing `[each]` is not visible inside it.

```
[lazy][mrql query='type = "resource"' format="list"][/lazy]
[details summary="Nutrition"][meta path="calories"] kcal[/details]
```

### `[tabs]`

A block of **alternative sections as an accessible tab set** (WAI-ARIA tabs, manual activation). Each `[tab label="..." lazy="true|false"]...[/tab]` directly inside `[tabs label="..."]...[/tabs]` is one tab; the first is open on load. Reach for it when a header would otherwise stack several `[details]` blocks, or when the sections are alternatives a reader flips between (Overview / Open tasks / Files, Ingredients / Steps / Nutrition). For one optional section, keep `[details]`.

- An eager tab renders with the page even while hidden. Put `lazy="true"` on every tab after the first that holds `[mrql]`, a gallery or a plugin data view: a lazy tab is a `[lazy]` block, fetched the first time it is opened, with `[lazy]`'s inline fallback on share pages, the live preview and the carrier slots.
- An `[item]` from an enclosing `[each]` or `[relations]` is not available inside eager or lazy tabs.
- Give the `[tabs]` a `label` when the page has more than one, and give every `[tab]` a short `label` (a blank one shows "Tab N").
- Slots: use it in `CustomHeader`; in `CustomSidebar` keep to about three short labels; never in `CustomSummary` or another per-card slot.
- Only a `[tab]` directly inside `[tabs]` counts. Tab-shaped text elsewhere stays literal without suppressing other shortcodes. A `[conditional]` cannot add a tab: put the `[conditional]` inside a tab. A `[tabs]` inside a tab works. Text between tabs is dropped, and `[tabs]` cannot go in a `[reload]` face.
- Only the open panel takes up space; its content determines the height. Without JavaScript and in hover cards, all panels show stacked under their labels.
- On `[reload]`, `[autorefresh]` or a page re-render, the selected tab is kept only for an unambiguous match. Identical repeated sets in the refreshed content start on their first tab. Focus follows the matched tab on `[autorefresh]` and page re-renders, or the equivalent `[reload]` button in the same panel when it replaces the set.

```
[tabs label="Project views"]
  [tab label="Overview"][property path="Description"][/tab]
  [tab label="Open tasks" lazy="true"][mrql query='type = note AND tags = "todo"' format="list"][/tab]
[/tabs]
```

### `[reload]`

A button that **re-renders content on the server without navigating**. Both the self-closing and the block form are valid.

- `[reload]` -- circular-arrow icon button. `label` (default `Reload`) is its accessible name and, because the icon is the whole button face, its tooltip.
- `[reload]Refresh[/reload]` -- the inner content (itself shortcode-expanded) becomes the button face instead of the icon, and names the button on its own. Keep it to phrasing content: a `<button>` may not contain links or other interactive elements. A `[reload]` inside another `[reload]`, and a `[lazy]` or `[details]` inside a `[reload]`, are refused outright (they render a `⚠` marker, and the linter flags them). Put the `[reload]` *inside* the `[lazy]`, never the other way round. `label` is optional here; if you set it, keep the visible text inside it (WCAG 2.5.3 Label in Name). A body that expands to nothing falls back to the icon.

What gets reloaded is resolved in the browser at click time by walking up from the button:

1. the innermost enclosing `[lazy]`, `[details]` or `[autorefresh]` block, if there is one -- only that block re-renders;
2. otherwise the whole custom-content slot being rendered (`CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `CustomLightbox` in the lightbox, or a description). A `[reload]` inside a `CustomMRQLResult` card reloads the slot holding the `[mrql]`, not the card;
3. otherwise the page -- share pages, the live preview iframe, the carrier slots (`CustomListHeader`, `CustomListFooter`), and `/mrql` result cards.

So pair `[reload]` with `[lazy]` to scope the refresh to that one block, and place it outside any deferred block to refresh the entire slot. Prefer the paired form in `CustomSummary`: a bare `[reload]` makes every card on a list page carry a sealed copy of the slot's source (about 1.34x the slot's own size, per card), while one inside a `[lazy]` reuses the token that block seals anyway. Steps 1 and 2 go through the same deferred-render fetch `[lazy]` and `[details]` use; the current content stays on screen dimmed while the request is in flight, and is left exactly as it was if the request fails. Requires JavaScript.

```
[reload]
[reload label="Refresh stock"]Refresh stock[/reload]
[lazy][reload label="Refresh open tasks"][mrql query='type = "note"' format="list"][/lazy]
```

### `[autorefresh]`

A block that **re-renders its body on the server after an inline editor saves** (`[meta]`, `[pills]` or `[datetime]` with `editable="true"`). Use it around what the server expands once and would otherwise go stale after an edit: `[conditional]` branches, `[meta inline="true"]` values, `[mrql]` results, plugin output.

- It refreshes when the save was for the entity it was rendered for (an editor anywhere on the page, in any slot), or when the editor that saved sits inside it (whatever entity that editor edits, e.g. inside a `[using]`).
- Saves under 250 ms apart cause one refresh. Nested blocks refresh once, through the outermost.
- `on="edit"` is the only accepted value of its one attribute, and the default.
- Everything in the body is replaced, so an editor inside it closes and loses unsaved input. Wrap only the dependent content; keep other editors outside.
- A `[reload]` inside it reloads just the block. It cannot go inside a `[reload]` button face.
- Works on full pages, in `[lazy]`/`[details]` content and in the lightbox. On share pages, the live preview, `/mrql` result cards and the carrier slots it renders once and never refreshes. Requires JavaScript for the refresh.

```
[pills path="status" editable="true"]
[autorefresh][conditional path="status" eq="done"]<span class="badge">Done</span>[/conditional][/autorefresh]
```

### `[action]`

A **button that runs one registered plugin action (`mah.action`) on this entity**, as the person who clicks it, through the same `POST /v1/jobs/action/run` the actions menu uses. Inline only.

- `plugin` and `name` (the action id) are required. `label` replaces the button text (and accessible name); `confirm` replaces the action's confirmation question, and `confirm=""` keeps it (a template can never remove one).
- `param-<name>="…"` sets a parameter the action declares: `number` takes a number, `boolean` `true`/`false`, `select` one of its options, text types the value as written. `entity_ref` and `info` cannot be set; an `entity_ref` that defaults to the trigger gets this entity. A bad value renders a `⚠ action` marker naming the parameter. An action cannot run from a template when a required bulk-selection parameter is visible after `show_when`.
- It renders an HTML comment, not a button, for viewers who cannot run the action (guests, read-only roles, group-limited accounts the plugin is not opened to), for entities excluded by the action or a visible trigger reference's filters, for a missing or disabled plugin, in Descriptions or note text blocks under login, on share pages, in the live preview, in hover cards and in direct MRQL API renders (`render=1`). Authorized `/mrql` list cards (`render=list`) can show the button. Check it on the real page, not in the live preview. Placement does not matter.
- Visible trigger references are checked after overrides and `show_when`, before required-value validation: an excluded entity renders the unavailable comment even if a required value is missing.
- If a refresh replaces or retargets the button while confirmation is open, accepting no longer submits the run. After submission, an in-place list refresh keeps an unchanged button busy; a retargeted button is usable immediately and stops watching the old Job without cancelling it.
- A synchronous success shows its message, even if the response includes a legacy `job_id`. On observed success, the `[autorefresh]` blocks for the submitted entity refresh, along with the enclosing block if the button is unchanged. Put the content the action changes in `[autorefresh]`, with the button **first** in that block (or outside it) so focus returns to it.
- Category stylesheets, including inline `[mrql]` result CSS, render the unavailable comment for an attributed `[action]`. Hover cards still omit the button when a deferred body loads or refreshes.
- Name an action that exists in an enabled plugin and say so: the bundled `project-management` (`pm-set-status`, `pm-set-priority`, `pm-open-board`, …) and `fal-ai` (`upscale`, `colorize`, …) plugins register actions. The linter warns about an action that does not exist.

```
[autorefresh]
  [action plugin="project-management" name="pm-set-status" label="Mark done" param-status="done"]
  Status: [meta path="status"]
[/autorefresh]
[action plugin="fal-ai" name="upscale" label="Upscale (Topaz)" confirm="Upscale this image on fal.ai?" param-model="topaz"]
```

### `[using]`

A block that **renders its body with another group, resource or note as the entity**, so every shortcode inside (`[property]`, `[meta]`, `[conditional]`, `[each]`, `[link]`, `[mrql scope=…]`) reads the target.

Name the target with exactly one source:

- `rel="owner"` / `rel="root"` -- the owning group or the top of the ownership chain (same meaning as `[link to=]`).
- `type="group|resource|note" id="42"` -- a fixed ID.
- `type="…" id-path="links.client"` -- an ID stored in this entity's Meta (number or numeric string).
- `type="…" id-prop="OwnerId"` -- an ID field on this entity.
- `query='type = note AND name = $n ORDER BY created DESC' param-n="Index"` -- first result of an MRQL query (`scope=` and `param-*` apply only here; no `GROUP BY`).

`[else]` renders, against the original entity, when there is no target or the viewer cannot see it. The lookup runs as the viewer and costs one query per render (counted by the page budget, cached for identical lookups), so wrap it in `[lazy]` in `CustomSummary`. It is unavailable on share pages. Inside the block there is no way back to the original entity, an `[item]` from an enclosing `[each]` renders nothing, and the target category's CustomCSS is not loaded.

Reach for it when a template needs fields from a related entity, typically the owner group's status or a Meta-referenced client. Use `[mrql]` instead when you need a list.

```
[using rel="owner"]
  Project: [link][property path="Name"][/link] ([meta path="status" inline="true"])
[else]
  Unfiled
[/using]
```

### `[relations]`

A group's **typed relations** (GroupRelations) as nested native lists: one list per relation type, in type-name order, each entry a link to the group at the far end with its category in parentheses. `direction="out"` (default) lists edges from this group; `direction="in"` lists edges to it, labelled "Incoming: <type>". A type with a back relation stores the reverse edge as its own relation, so `out` already shows both halves of such a pair; use `in` only for types without one. `type="Depends on"` is an exact, case-sensitive name match across every type of that name (types are unique by name plus from/to categories). `limit` 1–100, default 20.

As a block, each relation renders the body (inside its own `<li>`) against **this** group, with top-level `[item]`/`[calc]` reading the edge: `Name`, `Description`, `Type`, `Direction`, `GroupID`, `GroupName`, `GroupCategory`, `GroupURL`, and the far group's `GroupDescription` and `GroupMeta` (e.g. `[item path="GroupMeta.status"]`). `[else]` renders when there are none and has no item binding. `raw="true"` on `[item]` emits unsanitized values, including the far group’s description and Meta. In an `href` write `/group?id=[item path='GroupID']`; the linter warns on a bare `[item]` there.

Only groups have relations (a comment elsewhere; wrap in `[using rel="owner"]` on a resource or note). Read as the viewer: an edge is listed only when both groups are visible, and hidden edges never count toward `limit`. One page-query budget unit per statement: one per `[relations]`, plus one when the body reads `GroupDescription`/`GroupMeta`; unavailable lookups spend none. It is fine in `CustomSummary` (50 cards = 50 of the default 200 units). Pair it with SectionConfig `{"relations":{"state":"off"}}` (or `forwardRelations`/`reverseRelations: false`) so the built-in panels do not repeat it. Confined viewers can use MRQL `render=list` cards, not `render=1`. Unavailable on share pages, the entity picker and AI template tools; not allowed inside a `[reload]` face. The incoming index builds concurrently on PostgreSQL and before serving on SQLite.

```
[relations type="Depends on" limit="5"]
[relations direction="in"]<a href="/group?id=[item path='GroupID']">[item path="GroupName"]</a> [item path="GroupMeta.status"][else]None[/relations]
```

### `[inspect]`

An **authoring aid, not page content**. In the template editor's live preview it renders a collapsed panel listing what that point of the template can read: Meta paths with type, sample and a copyable `[meta]`/`[each]` snippet; the entity's own fields for `[property]` (associations named, never read; credential fields omitted); and, at the top level of an `[each]` body, the first element's `[item]` paths. Inside `[mrql]`/`[using]` it describes the switched entity. `path="project"` limits the Meta section to one subtree.

On every other surface (pages, cards, share pages, deferred bodies, MRQL API, AI template tools) it renders the empty string. Use it while exploring an entity's data, then **remove it before delivering a template**: it never belongs in a finished design. Inline only; not allowed inside a `[reload]` face.

```
[inspect]
[each path="steps"][inspect][item path="title"][/each]
```

### `[adjacent]`

Previous/Next links to the entity's siblings, in creation order (`created_at`, then `id`). Default mode: same entity type, same owner (ownerless pairs with ownerless) and same category / resource category / note type (none pairs with none). `series="true"` (resources only; an error marker on groups and notes) steps through the resource's Series instead. `label` names the `<nav>` landmark (default "Previous and next"); a name already used on the page gets a number ("Previous and next (2)"), but a label of your own reads better.

Renders a `<nav>` with `rel="prev"`/`rel="next"` links, leaves out a missing side and renders nothing when both are missing. Read as the viewer, so a group-confined user only sees in-subtree siblings. One page-query budget unit each and bounded by the MRQL timeout: put it in `CustomHeader` or `CustomSidebar`, not a per-card slot. Renders an "unavailable" comment on the MRQL render API, share pages, the lightbox, hover cards, the entity picker and AI template tools. There is no name order; use a Series for curated order. Inline only; not allowed inside a `[reload]` face.

```
[adjacent label="Meetings"]
[adjacent series="true" label="Pages"]
```

### `[similar]`

Thumbnails of the resources that look like the current one, from the stored perceptual-hash pairs, nearest first (ties by ID). Each has a badge `Duplicate · d` (distance 0–2, a near-certain duplicate) or `Similar · d`, an alt text with name, tier and distance, and a Compare link to `/resource/compare?r1=<this>&r2=<match>`. Attributes: `max-distance` (0–11; unset uses the hash similarity threshold setting), `limit` (1–24, default 8), `size` (`sm` 48 px, `md` 96 px default, `lg` 200 px), `label` (caption and accessible name, default "Similar resources").

Resources only: on a group or note it renders an error marker, so use `[using type="resource" id-path="cover"][similar][/using]` there. Renders nothing when there are no pairs within the distance (non-images never have any). Read as the viewer, so a group-confined user only sees in-subtree resources. One page-query budget unit each: put it in `CustomSidebar`, `CustomLightbox` or `CustomDetailFooter`, not a per-card slot. Renders inline in the lightbox; an "unavailable" comment on share pages. Inline only; not allowed inside a `[reload]` face.

```
[similar]
[similar max-distance="2" limit="12" size="sm" label="Possible duplicates"]
[using type="resource" id-path="cover"][similar label="Lookalikes of the cover"][/using]
```

### `[calendar]`

One month as a grid (Monday first) of the entities an MRQL filter matches, placed by a date field. Each non-empty day shows its count as a link to that day's list on `/mrql`, up to 3 linked titles, and "+k more" past that; the caption names the month and total. Prev/Next buttons swap the month in place (display pages, cards, lightbox, deferred bodies; not share pages, the live preview or list headers). Attributes: `mrql` (required; a filter naming exactly one type through a top-level `type = …`, with no `GROUP BY`, `ORDER BY`, `LIMIT`, `OFFSET`, `SCOPE` or `$params`), `date` (`created` default, `updated`, or `meta.<path>`), `month` (`current` default or `YYYY-MM`), `scope` (as `[mrql]`; default the entity's group).

A Meta date counts only when it is a string starting `YYYY-MM-DD` (a schema `format: date` value or an RFC3339 date-time, which is what `[datetime]` writes); its day is the date as written. Numbers, booleans and other strings are ignored everywhere. A day is a date on the server's local clock. One page-query budget unit for a month of up to 100 entities (titles and counts from one query); two above that, and then counts only, with a note. Read as the viewer, so a confined user sees only their subtree. Put it in `CustomSidebar` or `CustomHeader`, not a per-card slot, where each card runs its own queries. Inline only; not allowed inside a `[reload]` face.

```
[calendar mrql='type = "note"' date="meta.due"]
[calendar mrql='type = "resource"' date="created" month="2026-03" scope="global"]
```

Event category example: a group category whose `MetaSchema` has `"date": {"type": "string", "format": "date"}`, and whose parent group's sidebar shows its events:

```html
<section aria-labelledby="events-h">
  <h2 id="events-h" class="text-sm font-semibold">Events</h2>
  [calendar mrql='type = "group" AND category = "Event"' date="meta.date" scope="entity"]
</section>
```

### `[pivot]`

A cross-tab of two fields over the entities an MRQL filter matches: one row per `rows` value, one column per `cols` value, a count (or sum) per cell, and row, column and grand totals, in an accessible `<table>` with a caption inside a keyboard-scrollable region. Attributes: `query` (required; a filter naming exactly one type through a top-level `type = …`, with no `GROUP BY`, `ORDER BY`, `LIMIT`, `OFFSET`, `SCOPE` or `$params`), `rows` and `cols` (required, different fields), `value` (`count` default or `sum`), `field` (with `sum`: `meta.<path>`, or `fileSize`/`width`/`height` on resources), `scope` (as `[mrql]`), `max-rows` (default 20, at most 50), `max-cols` (default 10, at most 20), `rows-label`, `cols-label`, `caption`.

A dimension is a `meta.<path>`, a text field (`name`, `contentType`, …), `fileSize`/`width`/`height`, a note's `shared` (yes/no), a relation by name (`owner`, `groups`, `tags`, `notes`, `resources`, `parent`, `children`) or a date bucket (`created.month`, `updated.year`, …). Refused: bare timestamps, ID fields, traversals, unbounded text (`description`, `originalName`, `originalLocation`, `url`), and SQL aliases over 63 bytes (including the `sum_` prefix for sums). Filter by refused fields in `query` instead, or shorten a Meta path to fit. Values equal up to ASCII case share a row, as do same-named groups. A multi-valued relation drops the totals that would double-count.

Eligible counts link to `/mrql`. No links in sum pivots, truncated tables, or pivots scoped to a strict descendant of a confined viewer's allowed group. A count over MRQL's result limit or a composed filter over its query size limit or containing a carriage return (`\r`) does not link. Date buckets, `(none)`, and SQLite Meta axes with any numeric key do not support links that filter on them.

`max-rows` and `max-cols` keep the values with the highest total count or sum, according to `value`. The separate SQL cutoff keeps at most 5000 combinations with the most items even in sum mode. Truncated count totals show `+`; truncated sum totals are partial, and all sums use `float64` (integers above `2^53` in magnitude can round).

One aggregate statement plus a scope check for a confined viewer with concrete scope, one page-query budget unit on a cache miss; read as the viewer. Put it in `CustomHeader`, `CustomSidebar` or `CustomListHeader`, not a per-card slot. Inline only; not inside a `[reload]` face. It renders an "unavailable" comment without running a query on share pages and in hover cards, including deferred or refreshed hover-card bodies.

```
[pivot query="type = note AND meta.status IS NOT NULL" rows="meta.status" cols="owner" caption="Open work by owner"]
[pivot query="type = resource" rows="contentType" cols="created.year" value="sum" field="fileSize"]
```

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
   - `[calc]` for numbers derived from either, instead of storing them.
   - `[excerpt]` for a bounded preview of a Description or other long text on cards, instead of `[property path="Description"]`.
3. `data-views` for presentation widgets.
4. `meta-editors` for frequent inline edits.
5. `[using]` for fields of one related entity (owner, Meta-referenced ID); `[mrql]` for related collections and rollups.
6. `widgets` for group dashboards.

Good combinations:

- status-heavy header: `[meta]` + `data-views:badge` + `data-views:stat-card`
- admin sidebar: `[meta editable=true]` + `meta-editors:*`
- search result card: `[property path="Name"]` + `[meta path="status"]` + `[mrql ... format=compact]`

Avoid:

- large tables in `CustomSummary`
- interaction-heavy editors in avatars
- assuming plugin shortcodes exist without mentioning plugin enablement
