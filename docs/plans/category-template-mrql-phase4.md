# Plan: `[mrql]` Shortcode Ergonomics (Phase 4)

Implements Phase 4 of `docs/ideas/category-templates-and-shortcodes.md`:
inline scalar mode, empty/header/footer slots in block templates, and a
"view all" link. All work lives in `shortcodes/` plus the executor in
`template_filters/shortcode_query_executor.go`; no new endpoints, no schema
changes, **no parser (`shortcodePattern`) changes** — the new structure tags
are handled locally inside the `[mrql]` handler, following the `[else]`
precedent.

Independent of Phases 1–3, with the usual coupling: update Phase 1's docs
registry/lint (new attrs, new sub-block rules) if it has landed, and reuse
Phase 2's `format=` helpers and `SplitElse`/`SplitBranches` if they exist
(inline fallbacks noted below if this ships first).

## Current state (verified in code)

- `RenderMRQLShortcode` (`shortcodes/mrql_handler.go`) renders three modes:
  `aggregated` → always a table, `bucketed` → group boxes, flat → cards/
  table/list/compact/custom. A block body becomes the per-item template for
  every item (`applyBlockTemplate`), nothing else.
- The scalar-extraction logic the inline mode needs **already exists** in
  `resolveConditionalValue` (`conditional_handler.go:36`): flat → item count,
  `aggregated` + `aggregate="col"` → `Rows[0][col]`, bucketed → group count.
- Empty states are hardcoded per renderer ("No results." in
  `mrql_renderer.go`); there is no way to customize them, add a heading, or
  show counts.
- `QueryExecutor` takes seven positional params (`ctx, query, savedName,
  params, limit, buckets, scopeGroupID`) — at its limit before an options
  refactor.
- The executor resolves `saved=` names via `GetSavedMRQLQueryByName` and
  discards the resolved query text and saved ID; `QueryResult` carries
  neither, so a "view all" link cannot currently be built for saved queries.
- The `/mrql` page prefills from `?q=<query text>` and `?saved=<saved ID>`
  (`src/components/mrqlEditor.js:293-295`). Note the asymmetry: the shortcode
  identifies saved queries by *name*, the URL by *ID*.
- Explicit `SCOPE` in query text overrides the shortcode's scope attr
  (`shortcode_query_executor.go:55`); a scope applied via attr is invisible
  in the query text, so a naive `?q=` link would silently drop scoping.
- `[else]` is a literal tag handled locally by `SplitElse`, not a
  parser-recognized shortcode name — the model for this phase's sub-blocks.

## Work item 0 — Executor options refactor (enabler)

Replace the positional `QueryExecutor` tail with an options struct:

```go
type QueryOptions struct {
    SavedName    string
    Params       map[string]string
    Limit        int
    Buckets      int
    ScopeGroupID uint
    WantTotal    bool // work item 3
}
type QueryExecutor func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error)
```

Mechanical update of the call sites (`mrql_handler.go`,
`conditional_handler.go`, `shortcode_query_executor.go`, tests). `QueryResult`
gains three fields populated by the executor:

- `EffectiveQuery string` — the query text actually executed (resolved from
  `saved=` when applicable).
- `SavedID uint` — the saved query's ID when `saved=` was used (0 otherwise).
- `Total *int64` — true total ignoring `limit`, only when `WantTotal` (work
  item 3); nil otherwise.

Own commit, no behavior change. (If Phase 3 has already introduced its
`Handlers` struct for `Process`, this composes with it; neither depends on
the other.)

## Work item 1 — Inline scalar mode

`[mrql query="…" value="…"]` (and `saved=` + `value=`) renders a single
escaped text value with **no wrapper div** — usable mid-sentence in a header
("**[mrql query="resources" value="count"]** files").

- `value="count"` — flat item count / bucketed group count.
- `value="<column>"` — `Rows[0][column]` from aggregated results (same
  contract as the `aggregate=` attr on `[conditional]`).
- Extract the shared logic out of `resolveConditionalValue` into one helper
  (`extractScalarFromResult(result, key)`) used by both `[conditional]` and
  inline `[mrql]`, so semantics can never drift apart.
- Formatting: honor `format=`/`decimals=`-style attrs by reusing Phase 2's
  property formatting helpers (if Phase 2 hasn't landed, ship unformatted and
  let Phase 2 add it).
- Errors keep the existing `mrql-error` div (an inline error span variant,
  `<span class="mrql-error …">`, so it doesn't break surrounding layout).
- `value=` + block body is a lint error (Phase 1) and the block body is
  ignored at render time.
- Caveat to document: `value="count"` counts *returned* items, capped by
  `limit` (default 20). For a true count, users should use an aggregated
  query (`SELECT count(*) AS total …` equivalent in MRQL) — or `{total}` from
  work item 3. The docs entry must say this explicitly.

## Work item 2 — Header / footer / empty slots in block templates

New structure inside `[mrql]…[/mrql]` inner content, all parsed *locally* by
the handler (literal tags, like `[else]` — no global parser changes, so
existing templates containing these words in text are only affected inside
`[mrql]` blocks, called out in the changelog):

```
[mrql query="notes WHERE tag=todo" limit="10"]
  [header]<h4>Open TODOs ({count} shown)</h4>[/header]
  <li>[property path="Name"]</li>
  [footer]<p class="text-xs">updated live</p>[/footer]
[else]
  <p>Nothing to do 🎉</p>
[/mrql]
```

- **Extraction order**: pull out `[header]…[/header]` and `[footer]…[/footer]`
  spans first (first occurrence of each; local regex + literal scan skipping
  nested block spans, same technique `SplitElse` uses), then split the
  remainder on top-level `[else]` (reuse `SplitElse`; if Phase 2's
  `SplitBranches` exists, use it — `[elseif]` makes no sense here and lint
  flags it). What remains is the per-item template.
- **Semantics**: header and footer render once, wrapped around the results,
  processed with the *parent* entity context. The `[else]` branch replaces
  the entire body (header/footer included? No — header/footer render only
  when there are results; the else-branch is the complete empty-state
  output). Bucketed mode: header/footer wrap the whole bucket list; the
  else-branch fires when there are zero buckets.
- **Placeholders** `{count}` and `{total}`: substituted in header/footer/else
  content before shortcode processing. `{count}` = returned items (or bucket
  count); `{total}` = the true total, and its *presence* in the template is
  what sets `WantTotal` on the query options (no total query runs otherwise).
  Escaping hatch `{{count}}` → literal `{count}` is not needed — braces are
  otherwise meaningless in this context; document the substitution instead.
- Default renderers are untouched: without a block body, the hardcoded
  "No results." remains.

## Work item 3 — True totals and the "view all" link

1. **`Total`**: when `QueryOptions.WantTotal` is set, the executor runs a
   count variant of the query — same parsed AST, same scope path
   (`ExecuteMRQLScoped`'s WHERE), `COUNT(*)` instead of entity selection,
   ignoring limit. Add `CountMRQLScoped` next to `ExecuteMRQLScoped` in the
   application context, reusing the translator output so filters/scope cannot
   diverge from the main query. Aggregated mode: `{total}` is a lint warning
   (totals of aggregations are ambiguous) and renders as `{count}` of rows.
2. **View-all link**: new attr `link-all` on `[mrql]`:
   - `link-all="true"` appends a default footer link ("View all → ", styled
     like existing renderer output) after the results (before the custom
     `[footer]` if both are present).
   - Inside `[header]`/`[footer]`, the placeholder `{link-all}` expands to
     the bare URL for custom markup.
   - **URL construction** (this is where the verified details matter):
     - `saved=` queries → `/mrql?saved=<SavedID>` (from work item 0's
       `QueryResult.SavedID` — the name→ID asymmetry is resolved server-side
       where the saved query was already loaded).
     - Inline queries → `/mrql?q=<url-encoded EffectiveQuery>`.
     - **Scope preservation**: when the shortcode applied a scope (attr or
       default entity scope) and the query text has no explicit `SCOPE`
       clause, append ` SCOPE group(<id>)` to the linked query text so the
       `/mrql` page reproduces the same result set. Skip when scope is 0
       (global) or the unresolved sentinel.
     - `param-*` bindings: verify during implementation whether the `/mrql`
       page reads param values from the URL; if yes, carry them as query
       params, if no, link with the `$placeholders` unbound (the page renders
       its param inputs and the user fills them — acceptable v1).

## Testing & verification

- TDD in `shortcodes/`: `mrql_handler_test.go` grows tables for inline value
  mode (all three result modes), header/footer/else extraction (including
  nested-block skipping and bucketed mode), placeholder substitution, and
  link-all URL construction (scope appending, saved-vs-inline, sentinel
  scope). Executor-side tests for `CountMRQLScoped` parity (same WHERE as the
  main query) live next to the existing MRQL executor tests.
- E2E: one seeded category whose CustomHeader uses an inline `[mrql value=]`
  count and whose CustomSidebar uses a block template with
  header/else/link-all; assert rendered markers and that the view-all link
  lands on `/mrql` showing the same first result.
- MRQL is the one subsystem with meaningful engine divergence — run the
  Postgres suites, not just SQLite:
  `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...`
  and `cd e2e && npm run test:with-server:postgres`, alongside the standard
  `go test --tags 'json1 fts5' ./...` + rebuilt-binary
  `npm run test:with-server:all`.

## Docs to update

- Phase 1 registry/lint (if landed): `value=`, `link-all=` attrs; sub-block
  rules (`[header]`/`[footer]`/`[else]` only inside `[mrql]` blocks,
  `value=` + block body conflict, `{total}` in aggregated mode).
- docs-site `[mrql]` reference: new attrs, sub-blocks, placeholders, and the
  `value="count"`-is-capped-by-limit caveat.
- No OpenAPI or `mr` CLI changes.

## Delivery order

1. Work item 0 (options refactor) — own commit, pure mechanics.
2. Work item 1 (inline scalar) — smallest visible win, exercises the refactor.
3. Work item 2 (slots) — the bulk of the handler work.
4. Work item 3 (totals + link-all) — last; depends on 0 for
   `EffectiveQuery`/`SavedID`/`WantTotal` and integrates with 2's footer.
