# Plan: Template Robustness and Consistency (Phase 5)

Implements Phase 5 of `docs/ideas/category-templates-and-shortcodes.md`: no
silent raw-shortcode leaks, a per-page MRQL query budget, and unified
`CustomAvatar` semantics. Three small, independent items — cheap enough to
fold into adjacent phases if convenient, planned here so nothing gets lost.

## Current state (verified in code)

- **Silent leaks.** `processWithDepth` (`shortcodes/processor.go:66`) returns
  the input unchanged when `maxRecursionDepth` (10) is hit; a plugin renderer
  error falls back to `sc.Raw` (`processor.go:109`); `[mrql]` with a nil
  executor renders `sc.Raw` (`processor.go:95`); a non-block `[conditional]`
  renders `sc.Raw` (`conditional_handler.go:136`). In every case the end user
  sees literal `[shortcode …]` text with no hint of why.
- **Query cost on list views.** Every card partial runs `process_shortcodes`
  on `CustomSummary` and `CustomAvatar` (`templates/partials/group.tpl:11,62`,
  `resource.tpl:37,45`, `note.tpl:9,18`). The per-render `MRQLCache`
  (`plugin_system/mrql_cache.go`, attached once per page in
  `shortcode_tag.go:56-69`) dedupes by query+scope+limit+buckets+params — but
  entity-scoped queries differ per card, so a `CustomSummary` containing
  `[mrql]` typically runs one query per card per page load. Nothing bounds
  this.
- **`CustomAvatar` asymmetry.** For groups and notes it *replaces* the
  initials avatar (`{% if not … %}` fallback, `group.tpl:11-12`,
  `note.tpl:9-10`); for resources it renders *next to the category name*
  under the thumbnail (`resource.tpl:37-38`) — resources have no initials
  avatar to replace. The model comments state this
  (`resource_category_model.go:30` vs `category_model.go:30`), but the edit
  forms and docs don't make the difference obvious.

## Work item 1 — Visible failure markers

Principle: a template failure should be diagnosable from the rendered page,
subtle for viewers, explicit for authors. Two tiers:

1. **Author-facing inline marker** for actionable failures, reusing the
   existing `mrql-error` visual language but smaller (inline span, e.g.
   `<span class="shortcode-error" title="…">⚠ plugin:foo:bar</span>`):
   - Plugin renderer error → marker with the plugin/shortcode name and error
     in `title` (trust model is a private tool; error strings are fine).
   - `[conditional]` used without a closing tag (the non-block case) →
     marker "conditional requires a closing [/conditional]".
2. **HTML comment** for structural stops where visible output would be noise:
   - Depth cap hit → the content is emitted as-is (unchanged behavior — it
     may be meaningful text) plus a trailing
     `<!-- mr:shortcode depth limit reached -->` so authors inspecting the
     page see why expansion stopped.
   - `[mrql]` with no executor available (contexts that deliberately don't
     wire one) → `<!-- mr:mrql unavailable in this context -->` instead of
     leaking the raw shortcode. (Phase 6's share-page rendering depends on
     this behavior.)

Behavior change note: templates that today "render" their broken shortcodes
as literal text will start showing markers/comments instead. That is the
point; changelog entry required. Where Phase 1's lint exists, every marker
case has a corresponding lint rule so authors catch it at edit time.

## Work item 2 — Per-page MRQL query budget

1. Extend the per-render cache context (the natural place — it is already
   created once per page render and shared across all `process_shortcodes`
   and `custom_css` tags) with an executed-query counter.
2. In the shortcode query executor path: cache hits are free; each cache
   *miss* increments the counter. Beyond the budget, skip execution and
   render the standard error box with "inline query budget exceeded (N per
   page); refine templates or raise -mrql-page-query-budget", and log one
   warning per page (entity type `sql`-style, visible at `/logs`).
3. New config `-mrql-page-query-budget` / `MRQL_PAGE_QUERY_BUDGET`,
   default 50, `0` disables. Add to the CLAUDE.md config table and docs-site
   (config reference + a note on the shortcodes page explaining per-card
   scoping is why list pages can hit it).
4. Deployments with millions of resources are the motivation; 50 distinct
   queries per page is far above legitimate use (a 3-query summary on a
   20-card page is 60 — wait, that is legitimate). **Default check during
   implementation**: measure a realistic worst case first; likely default is
   150–200, not 50. The mechanism matters more than the number; the number
   must be generous enough that nobody hits it accidentally.

## Work item 3 — `CustomAvatar` semantics

Keep the per-carrier behavior (it exists for a structural reason: resource
cards are thumbnail-led and have no initials avatar to replace) and make it
explicit instead of unifying:

1. Edit-form descriptions (`createCategory.tpl`, `createNoteType.tpl` say
   "Replaces the default initials avatar"; `createResourceCategory.tpl` must
   say "Shown next to the category name on resource cards — resources keep
   their thumbnail") — verify all three against actual card behavior and fix
   any that mislead.
2. Same clarification in the docs-site category-template pages and in the
   Phase 1 docs registry entries (so autocomplete hover text is accurate
   per carrier).
3. Explicit non-goal: no template or model changes. If someone later wants
   avatar-replacement on resource cards, that is a feature request, not a
   consistency fix.

## Testing & verification

- Work item 1: table-driven tests in `processor_test.go` /
  `conditional_handler_test.go` asserting marker/comment output for each
  failure case (plugin error, depth cap, nil executor, non-block
  conditional).
- Work item 2: unit test on the executor wrapper (budget 2, three distinct
  queries → third returns the budget error; repeated identical query →
  cache hit, no increment). One E2E: seeded category whose `CustomSummary`
  runs an entity-scoped `[mrql]`, list page with budget set low via server
  flag, assert the budget notice renders and the page still loads.
- Work item 3: doc-only; covered by reading the three card partials against
  the new descriptions (no test surface).
- Full suites at the end (`go test --tags 'json1 fts5' ./...`, rebuild
  `./mahresources`, `cd e2e && npm run test:with-server:all`, Postgres
  suites) — work item 2 touches the MRQL execution path, so the Postgres run
  is not optional.

## Delivery order

1. Work item 1 (markers) — pure `shortcodes/` package, unblocks Phase 6's
   share rendering.
2. Work item 2 (budget) — config + executor wrapper.
3. Work item 3 (avatar docs) — anytime; bundle with whichever PR is open.
