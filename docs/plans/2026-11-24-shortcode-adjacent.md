# Plan: `[adjacent]`, previous/next sibling navigation (revision 3)

Slug: `adjacent`. Saved as `docs/plans/2026-11-24-shortcode-adjacent.md` (the index script files it under "Category templates & shortcodes").

## 0. What changed from the previous version, and why

Each point below answers one item of the evaluator's review.

1. **Deadline (review item 1, accepted).** Each lookup now runs under `context.WithTimeout(reqCtx, MRQLQueryTimeout())`. The timeout is applied with the existing `(*MahresourcesContext).WithParentContext(bounded)` (application_context/context.go). That helper rebases the handle onto the bounded context and carries over the scope filter and acting user already on `Statement.Context`. So the deadline reaches every query and the subtree scope is kept. The earlier rule "never call `.WithContext(reqCtx)`" still holds; `WithParentContext` is the sanctioned way to add a deadline. A timeout renders a dedicated marker.
2. **Indexes (review item 2, accepted).**
   - The sort keys are cut to `created` only, and the `order` attribute is gone. That leaves three composite indexes instead of six.
   - The resources index joins `resourceListIndexes`. It is on the resources table, so it **must** share `ResourceTableIndexBuildLockKey` and `oneResourceIndexBuildAtATime`, because two concurrent `CREATE INDEX CONCURRENTLY` builds on one table deadlock. That lock exists for exactly this case.
   - The notes and groups indexes get their own `EnsureSiblingNavIndexes` with a new lock key. That needs a small `main.go` change modelled on the Job filter index wiring.
3. **Tailwind (review item 3, accepted).** The old claim was wrong. The real reason the classes are generated: Tailwind v4 auto-detection reads every non-gitignored file in the checkout, and `index.css` excludes only `*.md`, `docs/`, `docs-site/`, `e2e/`, `tasks/`, `.claude/` and `skills/`. The class strings in the new `shortcodes/adjacent_handler.go` are therefore scanned. `public/tailwind.css` is a build product and is not tracked. `./scripts/css-scan-test.sh` is still run, and one test asserts that the built CSS contains the rules (section 9).
4. **Surfaces (review item 4, accepted).**
   - The resolver is also wired into the live template preview (`/v1/{category,resourceCategory,noteType}/previewTemplate`) and the description preview (`/v1/*/previewDescription`). Both are bound with `WithPrincipal` (`scopedAPI` and `scopedPreviewDescription`).
   - The entity picker is deliberately left out and documented. It renders many cards, and navigation links in a picker card are meaningless.
   - Every surface is listed in section 5, and each unavailable surface has a test.
5. **Scope callback claims (review item 5, verified).**
   - `registerScopeCallbacks` (application_context/scoping.go:507-522) registers `scopeReadCallback` on `Callback().Query()`, `Update()` and `Delete()` only, not on `Row()`. `Scan` and `Row` therefore bypass the scope. That is why every read here is a `Find` on a model.
   - The key subquery is unscoped, which the plan now says outright: it is safe only because step 2 has already proved the current row visible, and it reads the current row's own key. A test proves the seek queries never run when step 2 misses.
6. **SQLite plan (review item 6, accepted).** A SQLite `EXPLAIN QUERY PLAN` test is added. The keyset predicate also changes from a row-value comparison to the portable form `key >= K AND (key > K OR id > :id)`. Both planners use it as a plain range seek with a residual filter on ties, so nothing depends on SQLite's row-value range support.
7. **Ambiguities (review item 7, accepted).**
   - The conditional lint check for `series` on a non-resource carrier is dropped: `LintOptions` has no carrier type. Render time handles it.
   - "Carrier context" is now defined as `ctx.EntityType` not in {`group`, `resource`, `note`}.
8. **Usefulness (review item 8, accepted in part).**
   - Series mode now leads.
   - Ownerless rows now have siblings: other ownerless rows of the same category, through `owner_id IS NULL`. This covers top-level groups and the unowned inbox, and the same index serves it.
   - Name order is dropped rather than kept with its "Chapter 10 before Chapter 2" pitfall.
   - The intended future path for curated order is a meta-key order backed by a per-category expression index. It is recorded under Out of scope and not built here.

## 1. Motivation and user value

A Category, Resource Category or Note Type template can show an entity's own data, run queries and link to its owner (`[link to="owner"]`). It cannot link to the entity's neighbours. Readers of a scanned book's pages, a photo set, a run of meeting notes or a client's invoices have to go back to the list page to open the next item. On owners with millions of children, that list page is also the slowest page in the app.

`[adjacent]` adds a Previous and a Next link:
- **Series mode (`series="true"`).** Steps through the members of a Resource Series. A Series is the curated "these belong together" grouping, so this is the mode that fits the motivation best.
- **Default mode.** Steps through siblings with the same owner and the same category, in creation order. This covers meeting notes, invoices and the unowned inbox.

Each step is one indexed keyset seek, so its cost does not grow with the sibling count.

It is not in `docs/ideas/category-templates-and-shortcodes.md`, and nothing already built can express "the next row after this one in a stable order". `[mrql]` has no keyset and no "current row" anchor; `[link]` and `[using]` target a known ID.

## 2. Syntax and attributes

Inline only (`BlockNo`):

```
[adjacent]
[adjacent series="true" label="Pages"]
```

| Attribute | Type | Default | Values | Meaning |
|---|---|---|---|---|
| `series` | boolean | `false` | `true`, `false` | Resources only. `true` takes siblings from the resource's Series, whatever their owner or category. |
| `label` | string | `Previous and next` | any text | The `<nav>`'s `aria-label`. Give each `[adjacent]` on a page its own label. |

**Order.** Siblings are always ordered by `created_at ASC, id ASC`. `id` breaks ties, so the order is total and stable, and editing a row never moves it.

**Siblings, default mode.** Rows of the same entity type with:
- the same owner: `owner_id = ?`, or `owner_id IS NULL` when the entity has no owner. A group's owner is its parent group.
- the same category column: `resource_category_id` (NOT NULL) for resources, `note_type_id` for notes, `category_id` for groups. A NULL value matches other NULL rows through `IS NULL`.

**Siblings, series mode.** Resources with the same `series_id`.

**Deliberately omitted (minimum design):**
- `order=` and any name or meta order. Name order mis-sorts unpadded numbers, and each order needs its own composite index.
- `same=` (owner-only siblings). It would need another composite index per table.
- `updated` order. Updated time changes on every edit, so it is not a reading order.
- Per-link label text.

## 3. Rendered output (accessible markup)

When both neighbours exist:

```html
<nav class="shortcode-adjacent flex flex-wrap justify-between gap-2" aria-label="Previous and next">
  <a href="/resource?id=12" rel="prev" class="text-amber-700 hover:text-amber-900 underline"><span aria-hidden="true">←</span> Previous: Page 1</a>
  <a href="/resource?id=14" rel="next" class="ml-auto text-amber-700 hover:text-amber-900 underline">Next: Page 3 <span aria-hidden="true">→</span></a>
</nav>
```

**Links**
- The path is `/group`, `/resource` or `/note` with `?id=<id>`, the URLs `resolveLinkURL` builds.
- Each link's accessible name is its visible text: "Previous: <name>" or "Next: <name>". The arrows are `aria-hidden="true"`.
- `rel="prev"` and `rel="next"` are set.
- The Next link always carries `ml-auto`, so it stays right-aligned when it is alone.
- Links are in DOM order, Previous first, which is also the keyboard order.

**Escaping**
- The sibling name goes through `html.EscapeString`. No `raw` option is offered.
- The `label` value is HTML-escaped into `aria-label`.
- An empty or whitespace-only name falls back to `<Type> <id>`, for example "Resource 14", so no link is left without an accessible name.

**Empty and partial output**
- When only one neighbour exists, only that link renders.
- When neither exists, the output is `""`. No empty landmark is emitted.

**Styling**
- amber-700 on white is the link colour `[mrql]` already uses (`mrql_renderer.go`), and it passes WCAG AA.
- Focus comes from the global link focus style. No `outline-none` is used.

## 4. Behaviour for empty, missing and invalid input

"Carrier context" means `ctx.EntityType` is anything other than `group`, `resource` or `note`. That includes `category`, `resourceCategory` and `noteType`, for example a CustomListHeader rendered against its carrier.

| Situation | Output |
|---|---|
| No `AdjacentResolver` on `reqCtx` (see the surfaces in section 5) | `shortcodeComment("adjacent unavailable in this context")` |
| `series` not `true` or `false` | `shortcodeErrorMarker("adjacent", `series must be "true" or "false"`)`. The resolver is not called. |
| `series="true"` and `ctx.EntityType` is `group` or `note` | `shortcodeErrorMarker("adjacent", `series="true" works only on resources`)`. The resolver is not called. |
| Carrier context, or `ctx.EntityID == 0` | `""`. The resolver is not called. |
| Entity not visible to the viewer, or missing | `""` |
| `series="true"` and the resource has no Series | `""` |
| Budget refused | `shortcodeErrorMarker("adjacent", "inline query budget exceeded (N per page); refine templates or raise -mrql-page-query-budget")`, the same wording as `BudgetedExecutor` |
| `errors.Is(err, context.DeadlineExceeded)` | `shortcodeErrorMarker("adjacent", "lookup timed out")` |
| Any other resolver error | `shortcodeErrorMarker("adjacent", "lookup failed")`. The full error is logged through the resolver's `Logger()`, so no SQL text reaches the page. |
| `!meterFrom(reqCtx).alive()` after the resolver returns | `""`, the same check as `[using]` (using_handler.go:69) |
| Block form `[adjacent]…[/adjacent]` | Handled like any other `BlockNo` shortcode: the inline output renders, and lint reports the closing tag |
| Unknown attribute, for example a leftover `order="name"` | Lint warns through the existing generic "unknown attribute" check (lint.go:386). The attribute is ignored at render. |

## 5. Where it lives, and why

**It is a built-in Go handler, not a plugin.**
- Plugin shortcodes run Lua on the unscoped DB handle.
- `shortcode_tag.go` refuses plugins outright for group-confined principals, so a plugin would be unusable for exactly the users whose scoping matters.
- A plugin also gives no index or keyset control.

**It does not go through MRQL or the `QueryExecutor`.** MRQL has no "current row" anchor or keyset form. Its translator also passes datetime literals raw (`mrql/translator.go` `resolveValue`), so an exact tie on `created_at` would be fragile on both databases.

**Pattern.** It follows `PartialResolver`, `DeferredSigner` and `RefreshSigner`: a callback is put on the request context, which keeps `shortcodes/` free of DB imports.

### `shortcodes/adjacent_handler.go` (new)

```go
type AdjacentQuery struct {
    EntityType string // "group" | "resource" | "note"
    EntityID   uint
    Series     bool
}
type AdjacentLink struct { ID uint; Name string }
type AdjacentResult struct { Prev, Next *AdjacentLink }
type AdjacentResolver func(ctx context.Context, q AdjacentQuery) (AdjacentResult, error)

// ErrAdjacentBudget is returned by a resolver whose page budget refused the lookup.
var ErrAdjacentBudget = errors.New("adjacent: budget")

func WithAdjacentResolver(ctx context.Context, r AdjacentResolver) context.Context
func adjacentResolverFrom(ctx context.Context) AdjacentResolver
func RenderAdjacentShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string
```

- The budget error is a wrapped `ErrAdjacentBudget` whose message carries the limit. The handler renders that message.
- The resolver receives `ctx.EntityType` and `ctx.EntityID` from the context it is given. So inside `[using]` or an `[mrql]` item it navigates the switched entity.

`shortcodes/processor.go` gets one case: `case sc.Name == "adjacent": replacement = RenderAdjacentShortcode(reqCtx, sc, ctx)`.

### Data access: `application_context/adjacent_context.go` (new)

```go
func (ctx *MahresourcesContext) AdjacentEntities(reqCtx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error)
```

A fixed Go map, keyed by entity type, gives `{model, table, categoryColumn}`. Attribute text never reaches SQL.

1. **Fail-closed guard.** If `principalForcedScope()` returns `forced` or `deny`, and `scopeFromContext(ctx.db.Statement.Context) == nil`, return an empty result with no query.
   - This is the `WithMRQLPrincipal` binding, which carries no ORM scope.
   - It runs before the rebase in step 2, because `WithParentContext` keeps an absent filter absent.
2. **Bind the deadline.** `db := ctx.WithParentContext(reqCtx).db`. `reqCtx` already carries the resolver's timeout. The scope filter and actor come along, so `scopeReadCallback` still fires.
3. **Current row.**
   - `db.Model(&models.X{}).Select("id, owner_id, <cat>[, series_id]").Where("id = ?", q.EntityID).Limit(1).Find(&row)`.
   - `scopeReadCallback` adds `"<table>".owner_id IN allowlist` for resources and notes, and `"groups".id IN allowlist` for groups.
   - No row means an empty result, and no further query runs.
   - In series mode, `series_id` NULL means an empty result.
4. **Next.**
   - `db.Model(&models.X{}).Select("id, name")`
   - The partition, one of:
     - `owner_id = ?` or `owner_id IS NULL`, plus `<cat> = ?` or `<cat> IS NULL`, in default mode
     - `series_id = ?` in series mode
   - The keyset: `created_at >= (SELECT cur.created_at FROM <table> cur WHERE cur.id = ?) AND (created_at > (SELECT cur.created_at FROM <table> cur WHERE cur.id = ?) OR <table>.id > ?)`
   - `Order("created_at ASC, id ASC").Limit(1).Find(&next)`
   - The outer `id` and `created_at` are qualified with the table name, so the alias `cur` cannot capture them.
5. **Prev.** The mirror of Next: `<=` and `<`, `DESC, DESC`.

**Why the key comes from an unscoped subquery, and why that is safe.**
- The subquery's `FROM <table> cur` is not seen by the scope callback, which only appends to the outer statement.
- That is safe because it reads only the `created_at` of the current row, which step 3 has just proved visible through the scoped `Find`. When step 3 misses, steps 4 and 5 never run, and a test asserts this.
- Reading the key in SQL means a `time.Time` never makes a bind round trip, so ties compare exactly on SQLite (stored text) and Postgres (`timestamptz` precision).

**Why this predicate form.** `created_at >= K` is a plain range bound after the equality prefix. Both planners seek with it on `(owner_id, <cat>, created_at, id)` and read in index order, so `LIMIT 1` stops at the first match. The residual `OR` only filters the rows that tie on K, which are a handful at most. The query relies on no row-value comparison support.

These are plain reads with no transaction, so the read-only transaction rules do not apply.

The seek builder is factored as `adjacentSeek(db *gorm.DB, spec adjacentSpec, cur adjacentRow, next bool) *gorm.DB`, so tests can take its SQL with `ToSQL` for the EXPLAIN assertions.

### Server wiring

New file `server/template_handlers/template_filters/shortcode_adjacent_resolver.go`:

```go
type AdjacentContext interface {
    AdjacentEntities(context.Context, shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error)
    MRQLQueryTimeout() time.Duration
    Logger() *application_context.Logger
}
func BuildAdjacentResolver(appCtx AdjacentContext) shortcodes.AdjacentResolver
```

The closure:
- keeps a per-page cache keyed by `(type, id, series)`. It runs on a single goroutine, as `BuildPartialResolver` notes.
- on a cache miss, checks `shortcodes.QueryBudgetFrom(reqCtx)`. When a budget is present and `Allow()` refuses, it calls `MarkExceeded()`, calls `logPageQueryBudgetExceeded` once, and returns the wrapped `ErrAdjacentBudget`.
- runs the lookup as `bounded, cancel := context.WithTimeout(reqCtx, appCtx.MRQLQueryTimeout()); defer cancel()`, then `AdjacentEntities(bounded, q)`. `WithTimeout` keeps any earlier parent deadline, such as the deferred endpoint's.
- logs any other error at Warning, with entity type "shortcode", and caches only successes.

Plus one exported helper beside it:

```go
func WithAdjacentResolverFor(reqCtx context.Context, appCtx any) context.Context
```

It installs `BuildAdjacentResolver` when `appCtx.(AdjacentContext)` succeeds. Otherwise it returns `reqCtx` unchanged.

**Surfaces where `[adjacent]` renders.** All are bound with `WithPrincipal`, so the ORM scope filter is installed.

| Surface | Binding | Change |
|---|---|---|
| HTML pages (`process_shortcodes` tag) | `scopedCtx` (`WithPrincipal`) | `buildPageRenderContext`: `reqCtx = WithAdjacentResolverFor(reqCtx, appCtx)` inside the existing `if appCtx != nil` |
| `/v1/shortcodes/deferred` (`[lazy]`, `[details]`, `[autorefresh]` bodies) | `scopedAPI` | `deferred_render_api_handlers.go`: right after `buildMRQLAPIRenderContext` |
| Template live preview (category, resource category and note-type editors) | `scopedAPI` | `template_preview_api_handlers.go`: right after `buildMRQLAPIRenderContext` |
| Description preview | `scopedPreviewDescription` (`WithPrincipal`) | `description_preview_api_handlers.go`: right after `buildMRQLAPIRenderContext` |

It is **not** added inside `buildMRQLAPIRenderContext`. That function also serves the MRQL render API, which is bound with `WithMRQLPrincipal`.

**Surfaces that render the unavailable comment.** These are deliberate, documented and tested:
- the `/v1/mrql` render API (`WithMRQLPrincipal`, no ORM scope)
- the share server (public, unauthenticated)
- `processShortcodesForJSON` (lightbox and JSON)
- the entity picker (cards, not a detail context)
- Generation-run tools (`template_run_tools.go`), so a model cannot read neighbouring rows
- the CLI

The guard in step 1 is the second line of defence if a `WithMRQLPrincipal` handle is ever wired by mistake.

## 6. Scoping and authorization

All three row reads are `Find` on the principal-bound handle, so group-confined users and guests see only rows inside their subtree:
- Resources and notes: `owner_id IN allowlist`. NULL owners are excluded, so unowned rows never resolve for a confined principal.
- Groups: `id IN allowlist`.

Consequences:
- At the root of a confined subtree, the group's siblings are outside the allowlist, so nothing renders and no names leak. The same holds for top-level groups.
- A Series member outside the subtree is skipped by the outer scope, and the next visible member is the one linked.
- An empty allowlist (`RequiresScope` with no group) gives `1 = 0`, so the result is empty.
- A handle with no scope for a principal that must be scoped is caught by the guard, so the result is empty.
- The unscoped key subquery runs only after the scoped current-row read has succeeded (section 5).
- Not visible and non-existent render the same thing (`""`), so the shortcode cannot be used to probe for rows.
- Only `id` and `name` of a visible sibling are emitted. That is no more than its card already shows.

## 7. Performance at millions of rows

**Per `[adjacent]`**
- At most three queries: one primary-key lookup and two `LIMIT 1` seeks.
- One unit of `-mrql-page-query-budget`. An identical repeat on the same page is free from the cache.
- Every query is bounded by `MRQLQueryTimeout`.

**Indexes**

| Index | Table | Columns | Ensured by | Lock |
|---|---|---|---|---|
| `idx_resources_adjacent_created` | resources | `owner_id, resource_category_id, created_at, id` | appended to `resourceListIndexes` | `ResourceTableIndexBuildLockKey`, plus `oneResourceIndexBuildAtATime` (same table, so it must serialise with the FTS prefix and list builds) |
| `idx_notes_adjacent_created` | notes | `owner_id, note_type_id, created_at, id` | new `siblingNavIndexes` | new `constants.SiblingNavIndexBuildLockKey` |
| `idx_groups_adjacent_created` | groups | `owner_id, category_id, created_at, id` | new `siblingNavIndexes` | new `constants.SiblingNavIndexBuildLockKey` |

- New in `models/job_filter_indexes.go`:
  - `EnsureSiblingNavIndexes(db)`, which is `ensurePlannedIndexes(db, "sibling navigation indexes", siblingNavIndexLockKey, siblingNavIndexes)`
  - `EnsureSiblingNavIndexesOnOwnConnection(dsn)`
- `main.go`, next to the Job filter index block:
  - Postgres: `go ensureIndexesUntilBuilt("sibling navigation indexes", …OnOwnConnection…, jobFilterIndexRetryInterval)`
  - SQLite: a synchronous call before serving
- Its lock differs from the resources lock because notes and groups builds do not conflict with resources builds. It does not need `oneResourceIndexBuildAtATime`.
- `idx_resources_adjacent_created` makes the existing `idx_resources_owner_created (owner_id, created_at)` partly redundant, but that index serves list pages filtered by owner alone, so it stays.

**Complexity**
- Each seek is O(log n), whatever the owner's or category's child count, including `owner_id IS NULL`. Both SQLite and Postgres btrees serve `IS NULL` as an equality on an index column.
- There is no sort node.
- Series mode uses the existing `series_id` index. It costs O(size of the series) plus a sort of that series' rows only. That is typically tens of rows, bounded by the timeout, and documented. A dedicated `(series_id, created_at, id)` index is not added: it would put another write cost on every resource row for an uncommonly large case.

**Stated costs**, in the commit message and the docs:
- Three extra indexes add write cost on inserts and on updates of owner, category or `created_at`.
- On SQLite, a large library builds them once at startup before serving. On Postgres they build `CONCURRENTLY` in the background, and `[adjacent]` is correct but slower until they exist.

**List pages.** `[adjacent]` in a per-card slot (CustomSummary, CustomMRQLResult) costs one budget unit per card. The budget caps that, and the docs recommend CustomHeader and CustomSidebar.

## 8. Files to change

- `shortcodes/adjacent_handler.go` (new).
- `shortcodes/processor.go`: the dispatch case.
- `shortcodes/builtin_docs.go`:
  - A `BuiltinDoc{Name: "adjacent", Syntax: "[adjacent]", IsBlock: BlockNo}`.
  - Attrs:
    - `series` (boolean, default `false`, Enum `true|false`)
    - `label` (string, default `Previous and next`)
  - Examples: `[adjacent]`, `[adjacent series="true" label="Pages"]`.
  - Add `adjacent` to the list comment.
- `shortcodes/lint.go`:
  - Add `"adjacent"` to `builtinBaseNames`.
  - Add `case "adjacent":` with an error when `series` is present and not `true` or `false`. There is no carrier-type check.
- `application_context/adjacent_context.go` (new).
- `models/job_filter_indexes.go`:
  - Add `idx_resources_adjacent_created` to `resourceListIndexes`.
  - Add `siblingNavIndexes`, `siblingNavIndexLockKey`, `EnsureSiblingNavIndexes` and `…OnOwnConnection`.
- `constants/constants.go`: `SiblingNavIndexBuildLockKey`, a new distinct int64.
- `main.go`: the two-branch call next to the Job filter index block.
- `server/template_handlers/template_filters/shortcode_adjacent_resolver.go` (new), and `shortcode_tag.go` (`buildPageRenderContext`).
- `server/api_handlers/deferred_render_api_handlers.go`, `template_preview_api_handlers.go` and `description_preview_api_handlers.go`: one `WithAdjacentResolverFor` line each.
- `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl`: a reference-panel entry containing `[adjacent`. `TestReferencePanelsCoverAllBuiltins` enforces this.
- `docs-site/docs/features/shortcodes.md`:
  - A table row `| \`[adjacent]\` | Inline | Previous/next links to the entity's siblings or Series |`. `TestDocsSiteShortcodesPageCoversAllBuiltins` enforces this.
  - A section `## \`[adjacent]\` -- Previous and next` that covers the attributes, series mode first, the sibling rules including ownerless rows, creation order (why there is no name order, and how to get curated order), the empty cases, scoping, the budget and timeout, the series-mode cost, and the surfaces that render the unavailable comment.
- `docs-site/docs/features/custom-templates.md`: one CustomHeader example line.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`: an entry that mirrors the docs.

## 9. Tests

All written first and seen to fail.

**Unit: `shortcodes/adjacent_handler_test.go`** (fake resolver)
- Both links, only prev, only next, and none (`""`, no `<nav>`).
- Exact markup: `aria-label`, `rel`, `aria-hidden` arrows, `ml-auto` on Next, and the URL for each entity type.
- Escaping and fallbacks: a `<script>` name is escaped; a label containing `"` is escaped; an empty or whitespace name falls back to "Resource 14".
- Invalid or unavailable input: no resolver gives the unavailable comment; an invalid `series`, or `series="true"` on a group or note, gives a marker, and the resolver is never called; a carrier context (`EntityType` "category", "noteType") and `EntityID 0` give `""` without a call.
- Errors: a wrapped `ErrAdjacentBudget` gives the budget marker; `context.DeadlineExceeded` gives "lookup timed out"; another error gives "lookup failed" and its text does not appear.
- Inside `[using]` and an `[mrql]` item, the resolver receives the switched entity.

**Lint: `shortcodes/lint_test.go`**
- `series="yes"` is an error. `[/adjacent]` is reported. `[adjacnt]` gives a near-miss suggestion. `order="name"` gives the unknown-attribute warning. The valid forms give no issues.

`builtin_docs_consistency_test.go` passes unchanged once the docs and panels are updated.

**Data: `application_context/adjacent_context_test.go`** (SQLite)
- Ordering: two siblings with an identical, explicitly set `created_at` (the id tie-break gives the right prev and next in both directions, and walking from the first item to the last visits every sibling exactly once); the first and last items; a different category is skipped, and so is a different owner; ownerless resources, notes and groups pair with each other, and not with owned rows; NULL-category notes and groups pair with each other.
- Series mode across two owners. A resource with no series gives empty.
- **Scope:** a guest confined to subtree S: siblings inside S resolve; the group at S's root gets empty; a Series member outside S is skipped; an unowned resource gets empty; `RequiresScope` with no group gives empty; a `WithMRQLPrincipal`-bound guest gives empty with **zero** queries; **an entity outside S** gives empty with **exactly one** query.
- **Deadline:** a `reqCtx` already past its deadline returns `context.DeadlineExceeded`; a scoped guest's bounded read still carries the scope filter.
- **SQLite EXPLAIN QUERY PLAN** for each table, in default mode both with and without an owner, and in both directions: `SEARCH <table> USING INDEX idx_<table>_adjacent_created` (or `COVERING INDEX`), and no `USE TEMP B-TREE FOR ORDER BY`.

**Postgres: `application_context/adjacent_context_pg_test.go`** (tag `postgres`)
- The tie, order, ownerless and scope tests above.
- `EXPLAIN` for each table after seeding about 20k children under one owner and running `ANALYZE`: an Index Scan (or Index Only Scan) on `idx_*_adjacent_created`, no Sort node, and the same for the `owner_id IS NULL` partition.

**Indexes: `models/` index test**
- `EnsureResourceListIndexes` creates `idx_resources_adjacent_created`, and `EnsureSiblingNavIndexes` creates the notes and groups indexes, checked with `pragma_index_info` for the column order. A rerun of each is idempotent.

**HTTP: `server/api_tests/adjacent_shortcode_scope_test.go`** (modelled on `using_shortcode_scope_test.go`, with `-auth`)
- A confined guest's HTML note detail page shows only in-subtree sibling names, never an out-of-subtree name or id.
- `/v1/shortcodes/deferred` with `[lazy][adjacent][/lazy]` behaves the same way.
- The template preview endpoint and the description preview render the nav for the previewed entity, scoped the same way.
- `-mrql-page-query-budget=1` with two `[adjacent]` (one in a `[using]`) renders the second as the budget marker.
- These surfaces render `adjacent unavailable in this context`: the `/v1/mrql` render API, the entity picker, the public share page, and the lightbox JSON.

**CSS:** `scripts/css-scan-test.sh` is run, and the e2e spec asserts the computed `display: flex` and `justify-content: space-between` on `.shortcode-adjacent`.

**E2E: `e2e/tests/shortcodes/adjacent-shortcode.spec.ts`**
- A Note Type whose CustomHeader is `[adjacent label="Meetings"]`, and three notes in one group. The middle note shows Previous and Next, and clicking Next opens the third note. The first note has only Next, right-aligned.
- A Resource Category with `[adjacent series="true"]`, on three resources in one Series under different owners, steps across the owners.
- The note-type editor's live preview shows the nav.

**A11y: `e2e/tests/accessibility/adjacent-shortcode-a11y.spec.ts`**
- An axe scan of the note page, the `navigation` landmark named "Meetings", tab order Previous then Next, and each link's accessible name equal to its visible text.

## 10. Out of scope (recorded, not built)

**Curated order by a meta field**, for example `order="meta:chapter"`. The intended path is a per-category expression index created when a category opts in. The cast semantics and the index lifecycle each need their own design. Until then, curated order comes from a Series or from creation order.

## 11. Success criterion

On a detail page, `[adjacent]` links the entity's previous and next siblings in a total, stable `created_at, id` order, with the accessible markup in section 3. Siblings are the same Series in series mode, or the same owner (including none) and category otherwise.

It renders on HTML pages, the deferred endpoint and both editor previews. It renders the unavailable comment on every other surface.

## 12. Checklist

- [x] Plan saved and indexed
- [x] Shortcode handler, dispatch and parser (tests first)
- [x] Builtin docs, lint, reference panels, docs-site, skill reference
- [x] Data access with scope guard, deadline and keyset seeks (tests first)
- [x] Indexes, lock key and main.go wiring (tests first)
- [x] Server resolver and the four surfaces (tests first)
- [x] HTTP scope tests
- [x] E2E and a11y specs
- [x] Suites run

## 13. Review

Built as planned, with these deviations, each found while testing:

- **PostgreSQL ignored the index for NULL partitions.** The first EXPLAIN run on Postgres (20k rows, ANALYZE) walked `idx_*_created_at` with a filter and an Incremental Sort whenever the partition had `owner_id IS NULL` or a NULL category. Postgres only treats `=` as a constant when matching an index to an ORDER BY, so with `IS NULL` in the prefix it did not see that the composite index is already in `created_at, id` order. Default mode now orders by `owner_id, <category>, created_at, id`. That is the same order, because both prefix columns are constant within the partition, and it is exactly the index's order. Every seek then reads `idx_*_adjacent_created` with no sort node, on both databases.
- **Inside a `[reload]` face** `[adjacent]` renders an error marker, and lint reports it as an error, as `[lazy]`, `[details]` and `[inspect]` do. A `<button>` may not hold a `<nav>` of links. The plan did not cover this case.
- `adjacentSeek` takes an explicit `series` flag next to the row.
- **main.go:** the sibling index build sits next to the resource list index block rather than inside `migrateJobCore`. It has the same two branches (a background build on its own connection for Postgres, a synchronous build on SQLite), and it is placed with the other entity-table index builds.
- **Budget test:** it uses a budget of 2. The `[using]` lookup also takes a unit, so with 1 the `[using]` itself would be refused before the inner `[adjacent]` ran.
- **Template preview test:** it runs as an admin. Group-confined principals are refused at that route (403), so scoping there is covered by the gate. The description preview and the deferred endpoint are tested with confined accounts.
- **Tests that had to change:** `models/resource_list_indexes_test.go` and `server/api_tests/index_search_path_pg_test.go` build a minimal `resources` table, which now needs a `resource_category_id` column for the new index.
- **Notes and groups index lock (post-review fix).** The plan gave the notes and groups indexes a lock key of their own. That was wrong: the FTS prefix build also runs `CREATE INDEX CONCURRENTLY` on notes and groups, and `InitFTS` runs plain DDL on them. They now share `ResourceTableIndexBuildLockKey`, run under `oneResourceIndexBuildAtATime`, and start with the other resource builds after `InitFTS`. `SiblingNavIndexBuildLockKey` is gone. The index table in §7 and the file list describe the first version.
- **Hover cards:** focusing an `[adjacent]` link opens that entity's hover card, which sits outside every landmark. The axe scan therefore runs before the keyboard checks. The hover card has its own scan in `20-a11y-hover-cards.spec.ts`.

Verified:
- `go test --tags 'json1 fts5'` for `shortcodes`, `application_context`, `models`, `server/template_handlers/template_filters` and `server/api_tests`.
- The Postgres `[adjacent]` tests and the two Postgres index tests, against a Docker Postgres.
- `./scripts/css-scan-test.sh`.
- The new e2e and a11y specs, plus `tests/shortcodes/`, `template-preview.spec.ts` and the whole `tests/accessibility/` suite.
