# Plan: `CustomPanelSummary` slot for cards inside detail-page panels

## Context

Group, resource and note cards render their carrier's `CustomSummary` below the
title everywhere the card appears: list pages, the dashboard, MRQL results,
relation and owner cards, and the entity panels on detail pages (a group's Own
Entities / Related Entities, a note's Groups / Resources, a resource's Notes /
Groups, and the tag / category / note type / resource category pages). Authors
want a card shown *inside another entity's detail page* to carry different
content from the list-page card, e.g. a compact line on a parent group's page.

Decided with the user:
- The new slot replaces **only the summary area** of the card. Title, avatar,
  badges, description, tags and plugin actions stay as they are.
- It applies to **every card rendered inside a `seeAll` detail panel**. Every
  other card surface keeps `CustomSummary`.
- Empty slot → the card falls back to `CustomSummary`, so no existing
  deployment changes appearance (same shape as HoverCard → Summary).

Name: `CustomPanelSummary` + `CustomPanelSummaryCSS` on Category,
ResourceCategory and NoteType. JSON `custom_panel_summary`, CLI
`--custom-panel-summary`, bundle key `panelSummary`, UI label "Custom Panel
Summary".

## Render mechanism (the only behavioral change)

- `templates/partials/seeAll.tpl`: the per-entity include becomes
  `{% include partial(templateName) with tagBaseUrl=formAction cardInDetailPanel=true %}`.
  Pongo2 includes inherit the caller's context, so the flag reaches
  `group.tpl` through `relation.tpl` / `relation_reverse.tpl` too (the
  Relations section on a group page is a `seeAll` panel, so its cards follow the
  same rule). Three detail panels include the resource card directly rather
  than through `seeAll`, and get the same `cardInDetailPanel=true`:
  `displayResource.tpl` Series siblings and Similar Resources, and
  `displaySeries.tpl` Resources. No other include site sets it. Verified:
  pongo2 v4.0.2 `tags_include.go:13-30` copies the parent context into a fresh
  map and then applies `with` pairs, so the flag flows down and never out; MRQL
  list pages build their card context separately (`list_card_renderer.go:55-66`),
  and the `[mrql]` shortcode renders `CustomMRQLResult` or Go-rendered default
  cards (`shortcodes/mrql_renderer.go`), never these partials.
- `templates/partials/{group,note,resource}.tpl`: replace the single
  `{% process_shortcodes <carrier>.CustomSummary entity %}` with
  `{% if cardInDetailPanel && <carrier>.CustomPanelSummary %}{% process_shortcodes <carrier>.CustomPanelSummary entity %}{% else %}{% process_shortcodes <carrier>.CustomSummary entity %}{% endif %}`,
  plus a one-line `{# #}` comment (Pongo2 comments are single-line).
- CSS: `{% custom_css entity %}` already emits `TemplateCSSParts()`; adding the
  companion there covers every page.

Surfaces that get the panel slot: displayGroup (Own Notes/Sub-Groups/Resources,
Related Groups/Resources/Notes, Relations, Reverse Relations), displayNote
(Groups, Resources), displayResource (Notes, Groups), displayTag,
displayCategory, displayNoteType, displayResourceCategory, displayResource
Series + Similar Resources, displaySeries Resources.
Unchanged: list pages, text list, dashboard, MRQL list page cards, `[mrql]`
shortcode cards, `/relation` page, owner card (`ownerDisplay.tpl`, a sidebar
card, not a panel), hover card.

Field names: HTTP/form/JSON-model fields are PascalCase `CustomPanelSummary` /
`CustomPanelSummaryCSS` like every other slot (the partial-update preservation
and the bundle copy key on those names); snake_case `custom_panel_summary` only
in the archive manifest and the plugin `mah.db` maps.

## Wiring (follow `CustomHoverCard` exactly; it is on all three carriers)

Pattern: wherever `CustomHoverCard` / `CustomHoverCardCSS` is listed, add the
new pair right after it, in the same order. Representative files:

- Models + DTOs: `models/{category,resource_category,note_type}_model.go`
  (field doc comment), `models/query_models/{category,resource_category,note}_query.go`.
  AutoMigrate adds the two text columns on SQLite and Postgres.
- `models/template_css.go` (cascade, after HoverCardCSS),
  `models/template_revision_fields.go` (tracked slots).
- Save paths: `application_context/{category,resource_category}_context.go`,
  `note_context.go`, `crud_factories.go`; partial-update preservation in
  `server/api_handlers/handler_factory.go` and `note_api_handlers.go`
  (`fieldWasSent` / `formHasField`).
- JSON pre-render: `server/routes.go` `processSlot` for all three carriers.
- Plugin `mah.db`: `application_context/plugin_db_adapter.go` (read map,
  create, patch, ×3 carriers).
- Archive: `archive/manifest.go` (`custom_panel_summary,omitempty`, so
  `schema_version` stays 1), `groupio/export_context.go`, `groupio/apply_import.go`.
- CLI: one row pair in `cmd/mr/commands/template_slot_flags.go`; help md in
  `cmd/mr/commands/*_help/`.
- Edit forms: new `template-cluster` after Custom Hover Card in
  `templates/create{Category,ResourceCategory,NoteType}.tpl` (generation
  controls, HTML + CSS editors, `slotPreview.tpl`), plus a reference-panel
  `<dt>/<dd>` entry stating the fallback.
- Front end: `src/components/templatePreviewRender.js` SLOTS,
  `src/components/templateBundle.js` SLOT_FIELDS; `npm run build-js`, commit
  `public/dist`.
- Generator: `templateGenerateSharedSlots`
  (`server/api_handlers/template_generation_api_handlers.go`),
  `templateRunSlotNames` (`application_context/template_run.go`),
  `slotRoleLine` + `alpineSlots` + the once-per-card `[lazy]` hint
  (`application_context/template_generation_prompt.go`), the mirrored lists in
  `internal/templateeval/corpus.go`; the OpenAPI prose in
  `server/routes_openapi.go`, then regenerate `openapi.yaml`. The three
  `templatePromptSlots` lists in `template_generation_prompt_test.go` (they
  generate the pair/bundle cases, so a missing entry silently skips the slot).
- E2E helper: `e2e/helpers/api-client.ts` option + formData append for the
  three carriers.
- Docs: `docs-site/docs/features/custom-templates.md` (slot table + fallback),
  `shortcodes.md` / `template-revisions.md` / `plugin-lua-api.md` /
  `api/*.md` where slots are enumerated, the slot tables in
  `docs-site/docs/concepts/tags-categories.md`, `concepts/notes.md` and
  `user-guide/managing-notes.md`, regenerated `docs-site/docs/cli/*`,
  and the category-designer skill (`.claude/skills/mahresources-category-designer/`).
  Every claim checked against the code.

The six drift guards (`template_slot_wiring_test.go`,
`template_slot_coverage_test.go`, `template_slot_flags_test.go`, the revision
tracked-set test) fail on anything missed, so they double as a checklist.

## Steps

1. Branch `feat/panel-summary-slot`; copy this plan to
   `docs/plans/2026-11-09-panel-summary-slot.md`, run
   `./docs/plans/generate-index.sh`.
2. Baseline: Go unit (SQLite), E2E browser + CLI, so pre-existing failures are known.
3. Red: E2E spec `e2e/tests/shortcodes/panel-summary-slot.spec.ts` (below); run
   it, watch it fail.
4. Model field + render mechanism; spec goes green for render.
5. Remaining wiring until drift guards pass; build-js, openapi, CLI docs.
6. Docs + skill.
7. pi review of the diff; fix; repeat until clean.
8. Full suites on SQLite and Postgres; tick plan, add review section.

## Verification

E2E spec (new), for each carrier (Category, ResourceCategory, NoteType), with
`CustomSummary` = `<span data-probe="summary">` and `CustomPanelSummary` =
`<span data-probe="panel">` markers (unique per run). Assertions are scoped to
the card element inside a named panel, so raw slot text in embedded entity JSON
cannot satisfy them:
- Group detail: owned child in Own Entities, related one in Related Entities,
  forward and reverse Relations cards: panel marker, no summary marker.
- Note detail → Groups and Resources panels; resource detail → Notes, Groups
  and Series panels; series detail → Resources; tag, category, note type and
  resource category pages: panel marker.
- Mixed page: on a resource detail page whose owner group has a panel slot, the
  sidebar owner card shows the summary marker while a panel card shows panel.
- `/groups`, `/notes`, `/resources` lists, the dashboard, the `/relation` page
  and an MRQL list page (`CustomMRQLResult` empty): summary marker, no panel.
- Fallback: a carrier with only `CustomSummary` shows the summary marker in
  the panels.
- Similar Resources needs perceptual-hash fixtures; covered by the shared
  include flag and a review of that line, not by a dedicated case.
- `CustomPanelSummaryCSS` reaches a detail page that shows the card.
- `[property path="Name"]` in the panel slot resolves to the *card's* entity,
  not the page's.
- Edit form: field saves and re-renders; partial API update that omits it
  keeps the stored value.

Go: drift guards green. New round-trip test: export a group whose Category,
a resource's ResourceCategory and a note's NoteType each carry both new
fields, import into a fresh instance, assert all six values (the source-level
guard matches any assignment in a file, so one carrier could satisfy it for
all three). Accessibility suite (the forms gain a cluster).

Suites: `go test --tags 'json1 fts5' ./...`, `cd e2e && npm run test:with-server:all`,
Postgres `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
and `npm run test:with-server:postgres`. Rebuild the binary before E2E.
