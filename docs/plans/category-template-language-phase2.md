# Plan: Template Language Quality-of-Life (Phase 2)

Implements Phase 2 of `docs/ideas/category-templates-and-shortcodes.md`: richer
conditionals, dot-path traversal and formatting in `[property]`, fallbacks on
`[property]`/`[meta]`, and a `[link]` helper. All changes live in the
`shortcodes/` package plus one web-component attribute; no new endpoints, no
schema changes.

Independent of Phase 1 (`category-template-authoring-phase1.md`), with one
coupling noted at the end: if the Phase 1 shortcode docs registry / lint have
landed, they must be updated in the same PR as each language change here.

## Current state (verified in code)

- `evaluateCondition` (`shortcodes/conditional_handler.go:86`) checks exactly
  one operator attr, first match wins in a fixed order: `eq`, `neq`, `gt`,
  `lt`, `contains`, `empty`, `not-empty`. Values resolve from one of `mrql=`,
  `field=`, or `path=` (`resolveConditionalValue`).
- `[else]` is handled by `SplitElse` (`shortcodes/split_else.go`), which splits
  the inner content on the first *top-level* `[else]`, skipping ones nested
  inside block shortcodes. There is no `[elseif]`.
- `RenderPropertyShortcode` (`shortcodes/property_handler.go`) uses a single
  `FieldByName(path)` — `path="Owner.Name"` silently returns "". Output
  formatting is fixed (`time.Time` → RFC3339, slices → comma-joined). Only
  attrs: `path`, `raw`.
- `[meta]` values are rendered client-side by the `<meta-shortcode>` web
  component (`src/webcomponents/meta-shortcode.ts`), which receives
  `data-path/-editable/-hide-empty/-entity-type/-entity-id/-schema/-value`.
  A fallback for empty values must flow through a new `data-*` attribute.
- The shortcode name whitelist is the regex alternation in `shortcodePattern`
  and `closingTagPattern` (`shortcodes/parser.go:27,70`) — new names (`link`,
  `elseif`) must be added there.
- Detail-page URL conventions (used across templates): `/group?id=`,
  `/resource?id=`, `/note?id=`, `/category?id=`, `/resourceCategory?id=`,
  `/noteType?id=`.
- `Owner` is preloaded on group/resource detail fetches
  (`group_crud_context.go:387`, `resource_crud_context.go:209`), so one-hop
  traversal works on detail pages; list/card contexts may not preload it.

## Work item 1 — Richer conditionals

All in `conditional_handler.go` + `split_else.go`, TDD against
`conditional_handler_test.go` / `split_else_test.go`.

1. **New operators** in `evaluateCondition`:
   - `gte` / `lte` (mirror `gt` / `lt`).
   - `in="a,b,c"` — true when `fmt.Sprint(value)` equals any comma-separated
     item (trimmed).
   - `matches="regexp"` — Go regexp against `fmt.Sprint(value)`. An invalid
     pattern evaluates to false (never an error box mid-page); Phase 1 lint
     gains a rule that flags invalid regexes at edit time.
2. **AND semantics across operators on one tag.** Today the first operator
   found wins and the rest are silently ignored — surprising and undocumented.
   Change: *every* operator attr present must pass (natural ranges:
   `[conditional path="score" gte="1" lte="10"]`). Add `combine="any"` to opt
   into OR across the present operators. This is a behavior change only for
   templates that already set multiple operators — which today do not do what
   their author intended anyway; call it out in the changelog.
3. **Multi-value conditions** (cross-value AND/OR): support numbered suffix
   groups — `path2=`/`field2=`/`mrql2=` plus operator attrs with the same
   suffix (`eq2=`, `gte2=`, …), evaluated as additional conditions and folded
   with `combine` (default `all`). Implementation: extract the existing
   resolve+evaluate pair into a helper, loop suffixes `""`, `"2"`, `"3"` …
   until a suffix has no value source. Nesting `[conditional]` blocks remains
   the readable way to AND; suffixes exist mainly to make OR expressible at
   all.
4. **`[elseif …]` chains.** Generalize `SplitElse` into
   `SplitBranches(content) []Branch` where
   `Branch = { Attrs map[string]string; Content string }`: split on top-level
   `[elseif …]` and `[else]` dividers (same nested-block skipping the current
   implementation has), parsing `[elseif]` attrs with the existing
   `parseAttrs`. `RenderConditionalShortcode` walks the branches: the opening
   tag's attrs guard branch 0, each `[elseif]`'s attrs guard its branch,
   `[else]` matches unconditionally. First match renders. `SplitElse` stays as
   a thin wrapper so existing tests keep passing. Parser: add `elseif` to the
   divider recognition only — it is *not* a block opener and must not enter
   `shortcodePattern`'s block matching (treat it like `[else]`: a literal
   scanned inside conditional inner content).

## Work item 2 — `[property]` dot-paths and formatting

In `property_handler.go`, TDD against `property_handler_test.go`.

1. **Dot-path traversal.** Split `path` on `.`; walk with `FieldByName` per
   segment, dereferencing pointers and stopping (empty output) on nil. A
   purely numeric segment indexes into a slice (`Tags.0.Name`), out-of-range →
   empty. Keep the existing single-segment fast path semantics unchanged.
   Document the preload caveat: related structs render only where the page
   already loads them (detail pages preload `Owner`; cards may not) — the
   shortcode never triggers DB loads itself, by design (list pages render many
   cards).
2. **`default="…"` attr.** When the formatted value is the empty string,
   render the default instead (HTML-escaped unless `raw="true"`, same as the
   value).
3. **`format=` attr**, applied server-side after value extraction:
   - `format="date"` → `2006-01-02`; `format="datetime"` → `2006-01-02 15:04`;
     `format="time"` → `15:04` (for `time.Time` fields; non-time values pass
     through unchanged).
   - `layout="…"` → custom Go time layout (wins over `format` for times).
   - `format="filesize"` → human-readable bytes for integer fields (reuse the
     existing filesize helper used by templates if exported, else add one).
   Unknown `format` values pass the text through unchanged (never an error).
4. **`[meta default="…"]`.** Server side: pass through as `data-default` in
   `RenderMetaShortcode` (`meta_handler.go:37`). Client side: add
   `@property({ attribute: 'data-default' })` to `meta-shortcode.ts`; when
   `_isEmpty` and a default is set, render the default as plain text instead
   of the empty state (and `hide-empty` keeps precedence: hide wins over
   default when both are set — lint-warn on the combination). Rebuild the JS
   bundle (`npm run build-js`).

## Work item 3 — `[link]` helper

New `shortcodes/link_handler.go` + parser change.

1. **Parser**: add `link` to `shortcodePattern` and `closingTagPattern`
   alternations so both inline and block forms parse.
2. **Semantics**:
   - Inline `[link]` renders just the URL (so authors can write
     `<a href="[link]" class="…">`), HTML-escaped.
   - Block `[link]inner[/link]` renders a full
     `<a href="URL">processed inner</a>` (inner content goes through
     `processWithDepth`, consistent with other block shortcodes).
   - `to=` attr selects the target, resolved from `MetaShortcodeContext`:
     - `self` (default) → the current entity's detail page
       (`/group?id=`, `/resource?id=`, `/note?id=` by `EntityType`).
     - `owner` → `/group?id=<ScopeGroupID>` for resources/notes,
       `/group?id=<ParentGroupID>` for groups. Renders nothing (inline) /
       just the processed inner (block) when the scope field is the
       unresolved sentinel — never emit a link to a sentinel ID.
     - `root` → `/group?id=<RootGroupID>`, same sentinel rule.
     - `category` → the carrier's page (`/category?id=`,
       `/resourceCategory?id=`, `/noteType?id=`), reading the category ID off
       the entity via reflection (`CategoryId` / `ResourceCategoryId` /
       `NoteTypeId`); nothing when unset.
3. Register the new name in `processor.go`'s switch, before the plugin-prefix
   case.

## Testing & verification

- TDD throughout: each work item starts by extending the corresponding
  `shortcodes/*_test.go` table tests (red), then implements (green). These are
  pure functions — no DB or server needed for the bulk of coverage.
- `meta-shortcode` default: covered by an E2E test (a seeded category whose
  CustomHeader uses `[meta path="missing" default="n/a"]` renders "n/a" on the
  group page). One more E2E exercising `[elseif]` + `[link to="owner"]` on a
  seeded group tree end-to-end.
- Full suites after implementation: `go test --tags 'json1 fts5' ./...`,
  rebuild `./mahresources` before E2E (the suite reuses the prebuilt binary),
  `cd e2e && npm run test:with-server:all`, then the Postgres suite
  (`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...`
  and `npm run test:with-server:postgres`) — conditionals can execute MRQL via
  `mrql=`, so both engines matter.
- `npm run build-js` after the web-component change.

## Docs & Phase 1 coupling

- If Phase 1's `shortcodes/builtin_docs.go` registry exists by then, update it
  in the same PR as each language change (new operators, `default`/`format`/
  `layout` attrs, `link`, `elseif`) so lint and autocomplete stay truthful.
  Same for the lint rules mentioned above (invalid `matches` regex,
  `hide-empty`+`default` combination).
- Update the docs-site shortcode reference pages.
- No `mr` CLI or OpenAPI changes (no new endpoints).

## Delivery order

1. Work item 2 (`[property]` paths/format/default + `[meta default]`) — purely
   additive, no parser changes, immediately useful.
2. Work item 1 (conditionals) — contains the one deliberate behavior change
   (multi-operator AND), so it ships with a clear changelog note.
3. Work item 3 (`[link]`) — touches the parser regex; last so the new-name
   mechanics (also needed by `elseif`) are already proven by item 1.
