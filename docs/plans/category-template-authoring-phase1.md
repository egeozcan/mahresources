# Plan: Category Template Authoring Feedback Loop (Phase 1)

Implements Phase 1 of `docs/ideas/category-templates-and-shortcodes.md`: live
preview, lint, and autocomplete for the Custom* template slots on Category,
ResourceCategory, and NoteType edit forms.

## Goals

- Render any template slot against a real entity without saving (live preview).
- Surface template errors (unclosed blocks, unknown shortcodes, bad attrs) in
  the editor instead of leaking raw shortcode text into pages.
- Autocomplete shortcode names, attributes, and meta paths in the CodeMirror
  editors, with hover documentation.

## Non-goals

- No changes to the shortcode language itself (that is Phase 2+).
- No template versioning, duplication, or gallery (Phase 3).
- No preview for `SectionConfig` or `MetaSchema` (the schema editor modal
  already covers the latter).

## Current state (verified in code)

- The six slots are edited via `templates/partials/form/createFormCodeEditorInput.tpl`,
  backed by `src/components/codeEditor.js` — a lazy-loaded CodeMirror 6 editor
  with a language compartment, an `autocompletion()` extension already active,
  and the view exposed as `container._cmView` for tests.
- Rendering happens server-side: `shortcodes.Process` (in `shortcodes/processor.go`)
  driven by the `process_shortcodes` pongo2 tag
  (`server/template_handlers/template_filters/shortcode_tag.go`), which builds a
  `MetaShortcodeContext` via reflection (`buildMetaContext`), a plugin renderer
  from the `PluginManager`, and a `QueryExecutor` via `BuildQueryExecutor`.
- Plugin shortcode documentation (attrs, types, defaults, examples) already
  exists in `plugin_system/shortcodes.go` (`PluginShortcode`, `ShortcodeDocAttr`)
  and is rendered as HTML docs pages by `plugin_system/shortcode_docs.go`.
  The four built-ins (`meta`, `property`, `mrql`, `conditional`) have no
  machine-readable docs.
- `ParseWithBlocks` (`shortcodes/parser.go`) already tracks unmatched
  opening/closing tags internally but discards that information.
- Precedent for editor-support endpoints: `/v1/mrql/validate` and
  `/v1/mrql/complete` (POST, listed in `isReadViaPost` in
  `server/authz_policy.go` so read-only principals may call them).
- Authorization: `/v1/category*` and `/v1/resourceCategory*` writes are
  admin-only (`capTaxonomy`); `/v1/noteType*` writes are editor-level
  (`capEditor`). Path-prefix classification happens in `requiredCapability`.

## Step 1 — Shortcode docs registry + JSON endpoint

Foundation for both lint (unknown names/attrs) and autocomplete.

**Backend**

1. New file `shortcodes/builtin_docs.go`: a static registry describing the four
   built-ins in the same shape as `ShortcodeDocAttr` — name, description, block
   capability (`conditional` block-required, `mrql` optional-block, `meta`/
   `property` inline-only), attrs with type/required/default/description
   (including the `param-*` wildcard on `[mrql]`, the operator attrs on
   `[conditional]`, and `scope` keywords), and one example each.
2. New endpoint `GET /v1/shortcodes/docs` returning a JSON array merging:
   - built-ins from the static registry, and
   - every registered plugin shortcode from `PluginManager` (only enabled
     plugins), reusing the existing doc fields.
   Response shape per item: `{ name, syntax, description, isBlock ("no"|"optional"|"required"),
   attrs: [{ name, type, required, default, description }], examples: [...] }`.
   GET → `capRead`; no policy changes needed. Register in `routes.go` and
   `routes_openapi.go`.

**Testing**: Go unit test that the endpoint returns all four built-ins plus a
registered test-plugin shortcode; snapshot of one item's shape.

## Step 2 — Lint

**Backend**

1. New `shortcodes/lint.go`: `Lint(input string, known KnownShortcodes) []LintIssue`
   where `LintIssue = { Start, End int; Severity string; Message string }`
   (byte offsets — the parser already carries `Start`/`End`). Checks, all pure
   parsing (no execution):
   - Unclosed block opener / orphan closing tag (extend `ParseWithBlocks`'s
     phase-2 matching to report unmatched tokens instead of dropping them).
   - Closing tag for an inline-only shortcode (`[/meta]`, `[/property]`).
   - `[conditional]` without block form, without any operator attr, or with
     more than one `[else]` in its inner content.
   - Missing required attrs: `[meta]`/`[property]` without `path` (for
     `property`, `path` or nothing renders empty), `[mrql]` without
     `query`/`saved`, `[conditional]` without `path`/`field`/`mrql`.
   - Unknown shortcode-looking brackets: `[metaa path=...]` — a bracket
     expression that *almost* matches the pattern is left as literal text
     today; warn (severity `info`) when a bracket token starts with a known
     name prefix or `plugin:`.
   - Unknown attr on a *documented* shortcode (warning, not error — attrs on
     undocumented plugin shortcodes are skipped). `param-*` treated as valid
     on `[mrql]`/`[conditional]`.
   - MRQL syntax: for `mrql="..."`/`query="..."` attrs, run the existing MRQL
     parser (same code path as `/v1/mrql/validate`) and report its error with
     the attr's offset.
   `KnownShortcodes` is built from the Step 1 registry (names + attrs + block
   capability), so lint stays in sync with docs automatically.
2. New endpoint `POST /v1/shortcodes/lint` — body `{ content: string }`,
   response `{ issues: [...] }`. Pure parse, no plugin code, no DB. Add to
   `isReadViaPost` (same treatment as `/v1/mrql/validate`).

**Frontend**

3. Add `@codemirror/lint` to `package.json`.
4. In `codeEditor.js`, when a new option `shortcodeLint: true` is set (passed
   from the tpl partial only for the six template slots — not for MetaSchema
   or plain HTML/CSS fields), install a `linter()` source that debounces
   (~500 ms), POSTs to `/v1/shortcodes/lint`, and maps byte offsets to
   CodeMirror diagnostics. CSS slots skip shortcode lint except shortcode
   syntax inside them (CustomCSS supports shortcodes — lint applies there too,
   so gate by slot, not by mode).
5. On form submit, if any editor holds `error`-severity diagnostics, show a
   non-blocking confirm ("Template has N issues — save anyway?"). Never hard-
   block: the trust model allows arbitrary HTML and false positives must not
   prevent saves.

**Testing**: table-driven Go unit tests for every lint rule (`shortcodes/lint_test.go`);
API test for the endpoint; E2E test typing a broken `[conditional]` into the
category form and asserting a diagnostic appears (via `container._cmView`).

## Step 3 — Autocomplete + hover docs

All frontend; consumes the Step 1 endpoint.

1. New module `src/components/shortcodeCompletion.js` exporting a CodeMirror
   completion source and a `hoverTooltip` source, both fed by a
   once-per-page-cached fetch of `/v1/shortcodes/docs`:
   - After `[` → complete shortcode names (insert `[name ]` or the block
     skeleton `[name]…[/name]` for block-required shortcodes), with the
     description as `info`.
   - Inside an open shortcode → complete attr names (`attr=""` with cursor
     between quotes), marking required attrs and showing type/default/
     description.
   - Attr-value completion for closed enums we know server-side: `scope=`
     (`entity|parent|root|global`), `format=` on `[mrql]`
     (`table|list|compact|custom`), boolean attrs (`true|false`).
   - `path=` value completion from the **MetaSchema being edited in the same
     form**: read the `MetaSchema` textarea/editor live, parse the JSON Schema
     client-side, offer dot-paths from nested `properties` (best-effort; skip
     silently on invalid JSON). This makes the edit form self-consistent
     without a server round-trip.
2. Wire into `codeEditor.js` behind the same `shortcodeLint`-style flag
   (`shortcodes: true`), composing with the existing `autocompletion()`
   extension via `override`-free config (add as a language-agnostic
   `EditorState.languageData` or explicit `autocompletion({ override })` on
   the compartment — decide in implementation; must not break HTML tag
   completion from `@codemirror/lang-html`).
3. Hover over `[name` shows the same doc card as completion `info`.

**Testing**: E2E test that typing `[` in the CustomHeader editor offers `meta`
and a plugin shortcode; unit-level JS is covered indirectly (no JS test runner
in this repo — E2E is the harness).

## Step 4 — Live preview

**Backend**

1. Extract `buildMetaContext` + scope resolution from
   `template_filters/shortcode_tag.go` into a reusable location (e.g.
   `template_filters.BuildMetaContextForEntity(entity, appCtx)` exported, or
   moved to a small `server/shortcode_render` helper) so the preview handler
   and the pongo2 tag share one implementation.
2. New handler `previewTemplateHandler`, mounted at **three paths** so the
   existing path-prefix authorization applies with zero policy changes:
   - `POST /v1/category/previewTemplate` → `capTaxonomy` (admin)
   - `POST /v1/resourceCategory/previewTemplate` → `capTaxonomy` (admin)
   - `POST /v1/noteType/previewTemplate` → `capEditor`
   Deliberately **not** in `isReadViaPost`: preview executes MRQL and plugin
   shortcodes, so it must be gated like the corresponding edit capability.
   (Editors and admins are never group-scoped, so the unscoped plugin-code
   concern from `isPluginCodePath` does not arise; assert this with a test.)
   Request: `{ entityId: uint, content: string, css: string }` — the carrier
   type is implied by the path (group/resource/note). Response:
   `{ html: string, css: string, issues: [...] }` (lint issues piggybacked so
   the preview pane can show them without a second call).
   Handler: load the entity with the same preloads the display page uses
   (category/type relation for MetaSchema), build the context via the shared
   helper, run `shortcodes.Process` with the real plugin renderer and
   `BuildQueryExecutor`, and process `css` the same way `CustomCSS` is
   processed on real pages. 404 with a friendly message when the entity does
   not exist or has a different category than the one being edited (warn,
   don't fail, on category mismatch — previewing against any entity is useful
   while iterating).
3. Register both routes files + OpenAPI metadata.

**Frontend**

4. New Alpine component `templatePreview` + a partial included once per edit
   form (category, resourceCategory, noteType create/edit templates):
   - Entity picker: a small autocomplete input reusing the existing list
     endpoints (`/v1/groups?name=`, `/v1/resources?...`, `/v1/notes?...`),
     defaulting to the most recent entity of the category being edited;
     remember the last-used entity ID per category in `localStorage`.
   - A slot selector (or one preview per slot — decide by trying it; start
     with a single pane + dropdown of the six slots to keep the form compact).
   - Renders into a **sandboxed `<iframe srcdoc>`** that includes
     `/public/dist/main.css`, `/public/tailwind.css`, the returned `<style>`
     CSS, and `/public/dist/main.js` — the JS bundle is required because
     `[meta]` renders a `<meta-shortcode>` web component that is inert without
     it, and Alpine-based widgets need initialization. `sandbox="allow-scripts"`
     keeps it same-page-isolated; no `allow-same-origin`, so component fetches
     that need the API will fail gracefully — document this limitation in the
     pane ("interactive editors are non-functional in preview").
   - Debounced refresh (~700 ms) on editor change: the `updateListener` in
     `codeEditor.js` already syncs the hidden input; dispatch a bubbling
     `template-slot-changed` CustomEvent from there and have `templatePreview`
     listen for it.
   - Show returned lint issues under the pane.
5. CSRF is handled automatically (the JS `fetch` wrapper attaches
   `X-CSRF-Token`).

**Testing**: API tests for all three paths (happy path, entity not found,
role denial: editor → category preview 403, editor → noteType preview 200,
guest → 403 everywhere); E2E test editing CustomHeader on a category and
asserting the iframe shows the rendered output for a seeded group; a11y pass
on the modified forms.

## Delivery order & checkpoints

1. Step 1 (docs registry + endpoint) — small, unblocks 2 and 3.
2. Step 2 (lint) — server first with TDD, then editor wiring.
3. Step 4 (preview) — independent of 3; highest user value, so it goes before
   autocomplete polish.
4. Step 3 (autocomplete/hover) — pure frontend, lands last.

Each step ships independently behind no flags (all additive). After each step:
`go test --tags 'json1 fts5' ./...`, rebuild `./mahresources` (E2E reuses the
prebuilt binary), then `cd e2e && npm run test:with-server:all`. Postgres suite
(`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...` +
`npm run test:with-server:postgres`) at the end of Steps 2 and 4 (the ones with
backend surface). Run `npm run build-js` after any `src/` change.

## Docs to update

- `docs-site` pages covering category templates and shortcodes (new endpoints,
  editor features).
- OpenAPI spec regeneration (`go run ./cmd/openapi-gen`).
- No `mr` CLI changes (no new CLI commands in this phase).

## Open questions (decide during implementation, none blocking)

- Preview pane placement: inline per-slot vs. one shared pane with a slot
  dropdown vs. a modal. Start with the shared pane; revisit after using it.
- Whether `[mrql]` inside preview should carry a lower default `limit` to keep
  preview snappy on large deployments (lean yes: cap at 5 with a note).
- Whether the docs endpoint should also power a static in-app reference page
  (cheap addition once the JSON exists — nice-to-have, not in scope).
