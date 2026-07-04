# Plan: New Template Surfaces (Phase 6)

Implements Phase 6 of `docs/ideas/category-templates-and-shortcodes.md`:
`CustomListHeader`, share-page templating for note types, and hover cards.
The three items are fully independent — pick by demand; the order below is a
suggestion by effort/value.

Dependency note: item 2 requires Phase 5's work item 1 (failure markers) —
specifically the "nil executor renders a comment, not raw text" behavior.

## Current state (verified in code)

- **List pages** already resolve the active category when filtered: the group
  list context provider loads it when `groupQuery.CategoryId != 0`
  (`group_template_context.go:248-249`). CustomCSS is already injected
  page-wide on list pages via the `{% custom_css %}` head-block tag
  (`listGroups.tpl:3`, `custom_css_tag.go`). There are multiple list variants
  per entity (`listGroups/Text/Timeline`, four `listResources*`,
  `listNotes/Timeline`) — a new header must be a shared partial, not
  per-template markup.
- **The share server** (`server/share_server.go`) renders
  `templates/shared/displayNote.tpl` with **no `process_shortcodes` at all**
  today — NoteType templates simply don't apply to `/s/<token>` pages. It
  runs unauthenticated on a separate port with its own (stricter) CSP that
  already allows inline `<style>` blocks (comment at `share_server.go:69`).
  Share-side writes are strictly allowlisted (only `todos` block state,
  BH-031).
- **Hover cards** have no existing infrastructure: no popover/tooltip
  component in `src/components/` (closest is `dropdown.js`), and entity
  links across templates use the plain `/group?id=`, `/resource?id=`,
  `/note?id=` conventions — easy to target with a delegated listener.
- `process_shortcodes` requires an entity for context (`shortcode_tag.go:36`
  writes content through unchanged when the entity is nil) — a list header
  has no single entity, so item 1 must pick its context deliberately.

## Work item 1 — `CustomListHeader`

A category-level slot rendered at the top of list pages when the list is
filtered to exactly that one category.

1. **Model**: new `CustomListHeader string` (`gorm:"type:text"`) on all three
   carriers (Category, ResourceCategory, NoteType) + the corresponding
   `*Creator` DTO fields. AutoMigrate picks the columns up; no data
   migration.
2. **Rendering context**: the slot is processed with the *category/type
   itself* as the entity. Consequences, documented in the slot description:
   - `[property path="Name"]` → the category name; `Meta` is empty (carriers
     have no meta), so `[meta]` renders its empty state.
   - `[mrql]` scope resolution: a category is not a group, so the default
     entity scope must resolve to **global**. Concretely: build the
     `MetaShortcodeContext` with `ScopeGroupID/ParentGroupID/RootGroupID = 0`
     (global), not the unresolved sentinel — dashboard queries like
     "count of groups in this category" are the whole point:
     `[mrql query="groups WHERE category = \"Projects\"" value="count"]`.
     This needs a small extension in `buildMetaContext`
     (`shortcode_tag.go:97`) to accept the three carrier types (today it
     only handles Group/Resource/Note and returns nil otherwise).
3. **Templates**: one shared partial (`partials/customListHeader.tpl`) that
   renders when the context carries a single resolved category, included in
   every list variant. The group provider already loads the category for its
   filter chip; mirror that lookup in the resource/note list providers (the
   query DTOs carry the category filter; single-category detection = exactly
   one category filter value).
4. **Edit forms**: one more `createFormCodeEditorInput` slot on the three
   forms ("Rendered at the top of list pages filtered to this
   category/type."). Phase 1 lint/preview/autocomplete apply automatically;
   the Phase 1 preview pane needs a carrier-entity mode for this slot
   (preview against the category itself — simpler than the entity picker,
   since the context *is* the category).
5. `{% custom_css %}` is already emitted on list pages, so `CustomCSS` can
   style the new header with no further plumbing.

## Work item 2 — Share-page templating for note types

Opt-in application of a NoteType's presentation to the public `/s/<token>`
page.

1. **Opt-in flag**: new `ApplyTemplatesToShares bool` on NoteType (default
   false) with a checkbox on the NoteType form. Existing shares must not
   change appearance without an explicit choice.
2. **What applies**: `CustomHeader` (above the note content) and `CustomCSS`
   (as a `<style>` block — the share CSP already permits inline styles).
   Explicitly not `CustomSidebar` (the share layout has no sidebar) and not
   `CustomSummary`/`CustomAvatar`/`CustomMRQLResult` (no card/list context
   on a share page).
3. **Restricted processing mode.** The share server renders for anonymous
   viewers; templates must be inert there:
   - `QueryExecutor`: **nil** — `[mrql]` must not run queries on the
     unauthenticated surface (it would leak data beyond the shared note and
     add unauthenticated DB load). With Phase 5's markers this renders as an
     HTML comment, not leaked raw text.
   - `PluginRenderer`: **nil** — plugin Lua runs against the unscoped DB
     handle (same reasoning as the group-confined-principal plugin deny in
     `authz_policy.go`); an anonymous surface must never trigger it.
   - `[meta]`: force `data-editable="false"` regardless of the attr — the
     share page must not render edit affordances that POST to the primary
     server. Implement as a `ForceReadOnly bool` on `MetaShortcodeContext`
     honored by `RenderMetaShortcode`, set only by the share renderer. Also
     verify the `meta-shortcode` web component is actually loaded by the
     shared base template; if the share page doesn't ship the JS bundle,
     render `[meta]` server-side as plain text in this mode instead
     (decide during implementation by checking `templates/shared/base.tpl`).
   - `[conditional]`/`[property]`/Phase 2-3 additions (`[each]`, `[partial]`,
     `[link]`): safe — pure functions over the already-authorized note.
     `[partial]` resolution is a plain DB read; wire the resolver.
4. **Docs**: the NoteType form checkbox description and docs-site must state
   the restricted mode plainly ("no queries, no plugins, read-only meta on
   shared pages").

## Work item 3 — Hover cards

A popover preview when hovering/focusing an entity link anywhere in the app,
reusing the card-slot machinery (`CustomAvatar` + `CustomSummary`) rather
than adding a new template slot in v1.

1. **Endpoint**: `GET /hovercard?type=group|resource|note&id=N` (template
   route, HTML fragment response) rendering a compact card: name (linked),
   category/type label, thumbnail or avatar (processing `CustomAvatar`), and
   `CustomSummary` processed with the entity context — the exact machinery
   the card partials use, extracted into a shared partial. GET → `capRead`;
   group-scoped principals go through the standard scoped single-item read
   (fail-closed) so a confined user cannot preview entities outside their
   subtree — add an API test for exactly this.
2. **Frontend** (`src/components/hoverCard.js` + one global initializer):
   - Delegated `mouseover`/`focusin` listener matching
     `a[href^="/group?id="], a[href^="/resource?id="], a[href^="/note?id="]`
     (skip links inside the lightbox and the hover card itself).
   - ~500 ms hover intent delay; fetch with `abortableFetch` (exists in
     `src/index.js`); per-page in-memory cache keyed by type+id; position
     via fixed-position popover with viewport-edge flipping.
   - **A11y is a first-class requirement** (WCAG 1.4.13 content-on-hover):
     dismissible (Escape closes without moving the pointer), hoverable (the
     popover itself can be hovered without closing), persistent (stays until
     hover/focus leaves both trigger and popover). Also `role="tooltip"` +
     `aria-describedby` wiring on the trigger while open. This goes through
     the existing axe/a11y E2E suite.
   - Respect `prefers-reduced-motion` for the appear transition.
3. **Off-switch**: a user setting ("Show hover previews", default on) in the
   existing server-backed `user_settings` — hover cards are the kind of
   feature a subset of users hates; make turning it off trivial.
4. Explicit non-goal: a dedicated `CustomTooltip` slot. If `CustomSummary`
   proves wrong for popovers in practice, adding the slot later is cheap and
   slots into the same endpoint.

## Testing & verification

- Item 1: provider unit tests (single-category detection per entity type),
  E2E per carrier (filtered list shows the header, unfiltered does not,
  multi-category filter does not), preview-pane E2E if Phase 1 landed.
- Item 2: API tests on the share server — flag off: byte-identical rendering
  to today; flag on: header renders, `[mrql]`/plugin shortcodes render as
  comments (not raw text, not results), `[meta]` carries no edit affordance.
  Reuse the share security-header test setup
  (`share_server_security_headers_test.go`) as the harness pattern.
- Item 3: API test for the scoped-principal denial; browser E2E for
  hover-open, focus-open, Escape-dismiss, popover-hover persistence; axe
  pass with the popover open.
- Full suites after each item (Go unit, rebuild `./mahresources`, browser+CLI
  E2E, Postgres for items 1–2 which touch queries/models). `npm run build-js`
  after `src/` changes.

## Docs to update

- CLAUDE.md: nothing structural (no new stamped models — all three items add
  columns/routes, not content models).
- docs-site: list-header slot on the category template pages, share-mode
  restrictions on the sharing page, hover-card setting on the UI page.
- OpenAPI: item 3's route if registered under `/v1` (or leave as a template
  route like `/hovercard` — decide during implementation; template route is
  the closer precedent and skips OpenAPI).
- Phase 1 registry: `CustomListHeader` slot metadata for lint/preview.

## Delivery order (by value/effort)

1. Work item 1 (`CustomListHeader`) — smallest, pure extension of existing
   machinery, completes the "dashboard" story from the presets in Phase 3.
2. Work item 3 (hover cards) — self-contained, high perceived value.
3. Work item 2 (share templating) — last: smallest audience, and it wants
   Phase 5's markers plus careful security review time.
