# Plan: Template Composition and Reuse (Phase 3)

Implements Phase 3 of `docs/ideas/category-templates-and-shortcodes.md`:
`[each]` iteration, `[partial]` reusable snippets, template-set duplication and
export/import, and a starter template gallery. All four attack duplication;
the gallery deliberately reuses the import machinery so it doubles as its test.

Builds on Phase 2's `SplitBranches` (for `[each]`'s empty state) and, where
present, Phase 1's docs registry/lint (update them alongside each item here).

## Current state (verified in code)

- Block shortcodes, inside-out pair matching, and same-name nesting (depth
  tracking) already work in `ParseWithBlocks` (`shortcodes/parser.go`). New
  names must be added to the `shortcodePattern` / `closingTagPattern`
  alternations.
- `shortcodes.Process` takes two callbacks (`PluginRenderer`, `QueryExecutor`)
  wired in `template_filters/shortcode_tag.go`, the MRQL renderer, and (after
  Phase 1) the preview handler. A third callback for partial resolution pushes
  this signature past comfortable — refactor to a handlers struct (see item 2).
- Creator attribution: new content models must carry `CreatedByUserId *uint`
  and be added to `stampedModels()` in
  `application_context/user_admin_guard.go` (currently 14 models) so user
  deletion nulls the reference. The global GORM create-callback stamps any
  model with the column automatically. CLAUDE.md's "14 content models" count
  needs updating.
- `SavedMRQLQuery` (`models/saved_mrql_query_model.go`) is the pattern for a
  small named-content model: ID, timestamps, `CreatedByUserId`, unique Name,
  text content, `GetId/GetName/GetDescription`.
- Editor DTOs (`query_models.CategoryCreator` etc.) already carry all six
  slots + `MetaSchema` + `SectionConfig` as strings — duplication/export can
  round-trip through the existing create/update endpoints unchanged.
- Single-item JSON: there is no `GET /v1/category` single endpoint, but the
  template routes (`/category`, `/resourceCategory`, `/noteType`) are
  dual-response (`.json` suffix). Verify the `.json` payload exposes the full
  category object (slots included); if it does not, add plain single-item
  GET endpoints under `/v1/` — small, read-classified, follows existing
  patterns.
- Authorization: `/v1/category*`/`/v1/resourceCategory*` writes are admin-only
  (`capTaxonomy` via `isTaxonomyPath`), `/v1/noteType*` writes are
  editor-level.

## Work item 1 — `[each]` iteration over meta arrays

New `shortcodes/each_handler.go`; parser gains `each` and `item` names.

**Syntax**

```
[each path="ingredients" limit="20"]
  <li>[item path="name"] — [item path="qty" default="?"]</li>
[else]
  <p>No ingredients.</p>
[/each]
```

- `[each path="…"]` resolves the meta value at the dot-path
  (`extractRawValueAtPath` already exists in `conditional_handler.go`). A
  non-array value renders the else-branch (same as empty). Optional `limit=`
  caps iterations (default generous, e.g. 100 — templates render inline on
  pages).
- `[item]` renders the current element: scalar elements directly, objects via
  `path=` into the element. Reuse Phase 2's `format=`/`layout=`/`default=`
  helpers from the property handler so formatting is uniform. `[item
  index="true"]` renders the 1-based position (covers numbered lists without
  a new name).
- Empty/non-array state via a top-level `[else]` divider — reuses Phase 2's
  `SplitBranches`/`SplitElse` machinery verbatim.

**Implementation notes**

- Handler renders the item-branch once per element: substitute `[item …]`
  occurrences itself, then run the result through `processWithDepth` with the
  *parent* entity context so `[conditional]`, `[mrql]`, `[meta]` etc. keep
  working inside the loop.
- Nested `[each]`: the outer handler must NOT substitute `[item]` tokens that
  sit inside a nested `[each]` block span — `ParseWithBlocks` on the branch
  content yields the nested block spans to skip. `[item]` binds to the nearest
  enclosing `[each]`. Add an explicit test for one level of nesting.
- `[item]` outside `[each]` renders empty (and Phase 1 lint warns).
- Documented v1 limitation: `[meta editable="true"]` inside `[each]` is not
  rewritten to element-absolute paths (e.g. `ingredients.2.name`); authors
  who need editable array items keep using explicit absolute paths. Path
  rewriting is a possible follow-up.

## Work item 2 — `[partial]` reusable snippets

**Model + CRUD**

1. New model `TemplatePartial` (pattern: `SavedMRQLQuery`): ID, timestamps,
   `CreatedByUserId *uint`, `GUID`, unique `Name` (validated kebab-case,
   `^[a-z][a-z0-9-]*$`, so `[partial name="…"]` stays parseable and lintable),
   `Description`, `Content text`.
2. Add to AutoMigrate and to `stampedModels()`; update CLAUDE.md's stamped-
   model count and list.
3. CRUD surface following the existing entity conventions: list/create/edit
   template pages (content edited with the same `createFormCodeEditorInput`
   partial, `mode="html"`, so Phase 1 lint/autocomplete apply automatically)
   plus `/v1/templatePartials` (list) and `/v1/templatePartial`
   (create/update/delete) JSON routes with OpenAPI registration.
4. Authorization: admin-only for writes in v1 — partials expand inside every
   carrier's templates, including admin-managed Category surfaces, so the
   write gate matches the most privileged consumer. Add the path prefix to
   `isTaxonomyPath` (reads stay open). Revisit (editor-level) if demand
   appears.

**Rendering**

5. Refactor `shortcodes.Process` to take a handlers struct instead of growing
   positional callbacks: `type Handlers struct { Plugin PluginRenderer;
   Query QueryExecutor; Partial PartialResolver }` with
   `type PartialResolver func(name string) (string, bool)`. Mechanical update
   of the call sites (pongo2 tag, MRQL renderer/API path, share renderer if
   any, Phase 1 preview). Keeps the `shortcodes` package free of DB imports.
6. `processor.go` gains a `case sc.Name == "partial"`: resolve by `name` attr,
   expand through `processWithDepth(…, depth+1)` with the *current* entity
   context. Unknown name → HTML comment (`<!-- partial "x" not found -->`),
   consistent with the no-silent-leak direction from the ideas doc. The
   existing `maxRecursionDepth` (10) bounds self-/mutually-recursive partials;
   Phase 1 lint additionally warns on direct self-reference.
7. Resolver implementation: request-scoped lookup with a small cache attached
   the same way the MRQL per-render cache is (`WithMRQLCache` pattern), so a
   list page rendering 50 cards hits the DB once per partial name.
8. Parser: add `partial` to `shortcodePattern` (inline-only; no closing tag).
9. No-args v1: a partial expands with the entity context, nothing else.
   Parameterized partials (`arg-*` attrs) are an explicit non-goal until
   real templates demand them.

## Work item 3 — Template-set duplication and export/import

Client-side only (plus possibly the single-item GET noted above); the existing
create/update endpoints already accept every field.

1. **"Copy from…" on the three edit forms**: a dropdown (existing autocomplete
   pattern) listing categories of the *same carrier* plus, secondarily, the
   other two carriers. On pick: fetch the source's JSON, fill the CodeMirror
   editors + MetaSchema + SectionConfig via `_cmView` dispatch. Cross-carrier
   copies fill only the shared fields (all six slots + MetaSchema) and skip
   `SectionConfig` (shapes differ per carrier — `GroupSectionConfig` vs
   `ResourceSectionConfig` vs `NoteSectionConfig`). Nothing saves until the
   user submits — it is a form-filling aid, deliberately not a server-side
   clone endpoint.
2. **Export**: a button on edit forms downloading
   `{ "schemaVersion": 1, "carrier": "category"|"resourceCategory"|"noteType",
   "name", "description", "slots": { header, sidebar, summary, avatar,
   mrqlResult, css }, "metaSchema", "sectionConfig" }` as a `.json` file,
   assembled client-side from the current editor contents (exports unsaved
   edits — that is a feature: it doubles as a backup before experimenting).
3. **Import**: file picker on create/edit forms parsing the same shape,
   warning on carrier mismatch (then filling shared fields only) and on
   unknown `schemaVersion` (reject > 1, ignore unknown keys — same
   forward-compatibility rules as the archive manifest).
4. This bundle format is a *UI convenience*, not part of the group
   export/import archive contract (`archive/manifest.go` is untouched).
   Explicit non-goal: carrying partials inside bundles — a bundle referencing
   `[partial name="x"]` imports fine and lint flags the missing partial.

## Work item 4 — Starter template gallery

1. 3–4 presets as JSON files in the work-item-3 bundle format, embedded via
   `go:embed` (e.g. `server/template_presets/*.json`):
   - **Project dashboard** (Category): header with `[mrql]` counts +
     data-views `[progress]`, sidebar with `[conditional]` status badge,
     summary with `[format]`.
   - **Media collection** (ResourceCategory): widgets `[gallery]` header,
     `[each]` over a credits array, custom MRQL result card.
   - **Contact card** (Category): avatar slot, `[link]`, meta-editors
     widgets in the sidebar.
   - **Reading log** (NoteType): star-rating, date formatting, summary slot.
   Each preset exercises the Phase 2/3 language features, so the gallery is
   also a living regression fixture.
2. Serve via `GET /v1/templatePresets` (read-classified, static content).
   The create forms show a "start from preset" picker that routes through the
   exact same client-side import path as work item 3.
3. E2E: a test that imports each preset into a fresh category and asserts the
   rendered detail page contains the expected markers — this pins both the
   presets and the import path.

## Testing & verification

- TDD: `shortcodes/each_handler_test.go` and partial-resolution tests in
  `processor_test.go` first (pure functions, table-driven). Handler-struct
  refactor is covered by the existing suite compiling + passing.
- API tests: TemplatePartial CRUD + role matrix (editor denied writes,
  admin allowed, reads open), preset endpoint shape.
- E2E: partial round-trip (create partial → reference from a category header →
  group page renders it), `[each]` on a seeded entity with array meta,
  duplication flow (copy from existing category fills the form), preset import
  (work item 4.3). A11y pass on the new list/edit pages.
- Full suites at the end of each work item: Go unit
  (`go test --tags 'json1 fts5' ./...`), rebuild `./mahresources`, browser+CLI
  E2E (`cd e2e && npm run test:with-server:all`); Postgres suite after items
  2 and 4 (the ones with new tables/endpoints). `npm run build-js` after
  `src/` changes.

## Docs to update

- Phase 1 registry/lint (if landed): `each`, `item`, `partial` entries;
  lint rules for `[item]` outside `[each]`, unknown partial names, partial
  self-reference.
- docs-site shortcode reference + a new "reusing templates" page covering
  partials, bundles, and presets.
- OpenAPI regeneration; CLAUDE.md stamped-models count.
- `mr` CLI: optional follow-up (partials CRUD via CLI); if commands are added,
  update `cmd/mr/commands/*_help/*.md` per the CLI docs policy — otherwise no
  CLI changes.

## Delivery order

1. **Work item 1 (`[each]`)** — pure shortcodes-package work, immediately
   useful, no schema changes.
2. **Work item 2 (`[partial]`)** — model + refactor; the handlers-struct
   refactor lands here in its own commit before the feature.
3. **Work item 3 (duplication/export/import)** — frontend; independent of 1–2
   but its bundles get more valuable once partials/each exist.
4. **Work item 4 (gallery)** — last, since it consumes everything above and
   pins it with E2E fixtures.
