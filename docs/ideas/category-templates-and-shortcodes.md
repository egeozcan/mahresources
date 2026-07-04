# Category Templates & Shortcodes — Improvement Roadmap

Ideas for improving the category template slots (`CustomHeader`, `CustomSidebar`,
`CustomSummary`, `CustomAvatar`, `CustomMRQLResult`, `CustomCSS`, `SectionConfig`)
and the shortcode engine (`shortcodes/`), grouped into phases. Each phase is
sized to be shippable on its own; later phases build on earlier ones but have no
hard dependency unless noted.

Applies to all three template carriers: `Category` (groups), `ResourceCategory`
(resources), and `NoteType` (notes).

---

## Phase 1 — Authoring feedback loop

The biggest pain today: editing a slot means save, navigate to an entity of that
category, look, go back. Everything in this phase shares infrastructure that
already exists (`shortcodes.Process`, `buildMetaContext`, the plugin shortcode
docs registry in `plugin_system/shortcode_docs.go`).

1. **Live preview in the category editor.** A preview endpoint (e.g.
   `POST /v1/category/previewTemplate`) taking slot content + a sample entity
   ID, rendering server-side exactly as the real page would. Wire it into a
   preview pane next to each `createFormCodeEditorInput` slot with an entity
   picker ("preview as group #42").
2. **Template linting/validation.** Unclosed `[conditional]` blocks, misspelled
   shortcode names, and unknown attrs currently fail silently — the raw text
   leaks into the page. `ParseWithBlocks` already detects unmatched pairs
   internally; expose it as a validate endpoint and show warnings in the editor
   (and optionally on save).
3. **Shortcode autocomplete + hover docs in the editors.** The docs registry
   already knows every plugin shortcode with attrs, types, defaults, and
   descriptions. Feed that (plus the four built-ins: `meta`, `property`,
   `mrql`, `conditional`) into the CodeMirror editors as completion and hover
   documentation.

**Outcome:** the edit → see result loop drops from ~30 seconds to instant, and
broken templates are caught at write time instead of render time.

---

## Phase 2 — Template language quality-of-life

Small, contained changes to the existing handlers. No new concepts, just
removing the sharpest edges.

1. **Richer conditionals** (`shortcodes/conditional_handler.go`):
   - `gte` / `lte` operators alongside the existing `gt` / `lt`.
   - `in="a,b,c"` membership test.
   - `matches` (regex) operator.
   - `[elseif ...]` divider alongside the existing `[else]` (the `SplitElse`
     mechanism already establishes the pattern).
   - A way to combine two conditions (`and` / `or`).
2. **Dot-path traversal in `[property]`.** `FieldByName(path)` only reaches one
   level, so `path="Owner.Name"` silently returns nothing. Add nested traversal
   through pointers/structs so related-entity fields are reachable without an
   MRQL query.
3. **Formatting and fallbacks.** `default="…"` attr on both `[property]` and
   `[meta]` for empty values; `format=` on `[property]` (date layout, filesize)
   so users don't need the data-views `[format]` plugin for trivial cases.
4. **A `[link]` helper.** Templates currently hardcode
   `/group?id=[property path="ID"]`-style URLs. A `[link]` / `[link to="owner"]`
   shortcode that emits the correct entity URL removes the most brittle pattern
   in user templates and stays correct if routes change.

**Outcome:** the most common template patterns (conditional badges, owner
links, formatted values with fallbacks) become one-liners.

---

## Phase 3 — Composition and reuse

Larger language features plus reuse across categories. These belong together
because they all attack the same problem: duplication.

1. **Iteration over meta arrays: `[each path="ingredients"] … [/each]`.**
   Today an array-valued meta field renders as raw JSON (or comma-joined via
   `[property]`). A block shortcode exposing each element (`[item]` /
   `[item path="name"]` for arrays of objects) is the single biggest
   expressiveness gap. The MRQL block-template code path proves per-item
   rendering already works in the processor.
2. **Reusable partials: `[partial name="rating-row"]`.** The six slots cannot
   share markup — a rating row used in both `CustomSummary` and
   `CustomMRQLResult` is copy-pasted. A small named-snippet store (per category
   or global) referenced by name removes most duplication. Recursion depth
   handling already exists in the processor.
3. **Copy/duplicate template sets.** A "duplicate from category…" action on the
   edit form, and/or export/import of a template bundle (all six slots +
   `SectionConfig`) as one JSON file — including across carrier types where
   fields align.
4. **Starter template gallery.** A handful of built-in presets ("project
   dashboard", "media collection", "contact card") selectable when creating a
   category, showcasing `meta` / `badge` / `format` / `gallery` / `progress`
   together. Doubles as living documentation and exercises the
   duplicate/import machinery from the previous item.

**Outcome:** write a template fragment once, use it everywhere; new users start
from working examples instead of blank textareas.

---

## Phase 4 — `[mrql]` shortcode ergonomics

Rounding out the query shortcode so common dashboard patterns are native.

1. **Inline scalar mode.** Aggregated results always render a table. For
   "42 items · 3.1 GB" style headers users currently detour through the
   data-views `[format]` / `[badge]` plugins' `mrql=` attrs. A first-class
   `[mrql query="…" value="total"]` inline mode makes the common case native.
2. **Empty/header/footer slots in block templates.** The block template stamps
   every item identically; there is no way to customize the "No results."
   message, add a heading with the total count, or a separator. Sub-blocks like
   `[empty]…[/empty]` plus a `{count}` placeholder.
3. **"View all" link.** An optional attr that appends a link to the `/mrql`
   page with the query prefilled, for result sets larger than the shortcode
   limit.

**Outcome:** `[mrql]` covers headers, stats, and dashboards without plugin
detours or hand-rolled empty states.

---

## Phase 5 — Robustness and consistency

Small fixes that make failures diagnosable and behavior predictable. Cheap
enough to fold into any adjacent phase if convenient.

1. **Don't silently leak raw shortcodes.** When `maxRecursionDepth` is hit or a
   plugin renderer errors, the raw `[shortcode …]` text is shown to the end
   user. Render an HTML comment or a subtle inline error marker (like the
   existing `mrql-error` div) instead.
2. **Per-page MRQL query budget for list views.** A `CustomSummary` containing
   `[mrql]` runs one query per card per page load; the per-render cache only
   dedupes identical query+scope, and scope differs per entity so it rarely
   hits. At minimum document this; ideally enforce a per-page budget with a
   friendly "too many inline queries" notice — some deployments have millions
   of resources.
3. **Unify `CustomAvatar` semantics.** For groups it *replaces* the initials
   avatar; for resources it renders *next to the category name*
   (`resource_category_model.go` vs `category_model.go`). Either align them or
   make the difference explicit in the edit-form descriptions.

**Outcome:** template failures are visible and explainable; no surprise query
storms on large list pages.

---

## Phase 6 — New template surfaces

New places where category templates can render. Each is independent; pick by
demand.

1. **`CustomListHeader` slot.** Rendered at the top of a list page when it is
   filtered to that category, for dashboards like "12 active projects · 3
   overdue" above the cards. The plumbing (CustomCSS injection on list pages,
   per-entity shortcode processing) already exists; this is the missing
   aggregate-level slot.
2. **Share-page templating for note types.** Let a NoteType's
   `CustomHeader`/`CustomCSS` (or a dedicated `CustomShareLayout`) apply to the
   public `/s/<token>` page so people can brand what they publish. Needs care
   about which shortcodes are safe to run in the unauthenticated share-server
   context (notably `[mrql]` and plugin shortcodes).
3. **Hover cards.** A `CustomTooltip` slot rendered when hovering an entity
   link elsewhere in the app — the `CustomSummary` machinery reused in a
   popover.

**Outcome:** category templating extends beyond detail pages and cards to list
pages, public shares, and cross-references.

---

## Suggested order

Phase 1 first — it multiplies the value of everything after it, since every
later feature is easier to build templates with when preview/lint/autocomplete
exist. Phase 2 next (small, high leverage). Phases 3–4 are the substantial
language work. Phases 5–6 can interleave as capacity allows.
