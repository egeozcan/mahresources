<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 2 plan/evaluate round(s).
> Last evaluation: consistency 8.2/10, usefulness 5.3/10, average 6.75/10. The bar was 8.5.
>
> **Pitch:** Shows the latest audit LogEntry rows for the current entity (action, a message summary, relative time and the acting user where recorded): [history limit="5" actions="update,create"]. It queries the indexed entity_type and entity_id columns with a hard limit. Request paths, user agents and IP addresses are never exposed, and the shortcode renders nothing for viewers who can't see the admin audit log, so it fails closed. Shared or team-maintained groups get a 'recently changed' trail in the sidebar, which plugin [activity] (owned-children updates) doesn't cover.

# Plan: `[history]` built-in shortcode (logged changes for the entity)

When implemented, this plan is saved as `docs/plans/2026-11-24-history-shortcode.md` and the index is regenerated with `./docs/plans/generate-index.sh`.

## 0. Changes since the previous version (response to review)

| # | Review point | Decision |
|---|---|---|
| 1 | The entity history is incomplete (bulk, tag, relation and inline edits have a nil id) | **Accepted.** Section 1.2 audits every `Logger()` call site that writes `group`, `resource` or `note`. The UI is renamed "Logged changes" so it does not claim completeness. The docs list what is recorded and what is not. **Scope decision:** this plan adds no new logging. Per-entity rows for bulk edits would write N rows per request on million-row deployments, and per-entity rows for inline meta edits would flood the log from meta-editor widgets. That belongs in its own plan. |
| 2 | The rows carry little information | **Partly accepted.** Section 1.3 states plainly what the trail adds over `UpdatedAt`. I keep `Details` out, with evidence: of all per-entity rows, only `Merged resources` writes `Details` (`winnerId`, `loserIds`), and every row links to `/log?id=N`, which shows the details to the same admin. Rendering `Details` would add an escaping and secrets review for almost no rows. |
| 3 | The audience is narrow | **Accepted and stated** (section 1.4). |
| 4 | The database-error and budget-error paths contradict each other | **Fixed.** The loader owns the cache and the budget, and returns the sentinel `shortcodes.ErrHistoryBudgetExceeded` (wrapped with the limit). The handler checks it with `errors.Is`, renders the budget text for it, and renders the fixed "could not load the change log" for any other error. Test 6 now asserts that the error text is *not* printed. |
| 5 | `MarkExceeded` and the warning are skipped on exhaustion | **Fixed.** On exhaustion the loader calls `budget.MarkExceeded()` and, the first time, `logPageQueryBudgetExceeded`, so `Stats().Exceeded` (MRQL list warnings) and the `/logs` warning behave as they do for `[mrql]`. The warning text now names both shortcodes. |
| 6 | The lightbox and `[autorefresh]` disagree | **Fixed by design, not just documented.** The rule is now: the loader is installed on every surface that renders for the request's own principal and carries a refresh signer, because those are exactly the surfaces whose refreshes go through `/v1/shortcodes/deferred`. That adds `processShortcodesForJSON` (the lightbox) and the `/v1/mrql?render=list` card path. It also closes a gap the previous plan missed: list cards rendered there take `_reqCtxWithCache` from `buildMRQLAPIRenderContext`, so `buildPageRenderContext` never runs for them. Section 5.5 lists every surface. |
| 7 | Edge cases | **Added.** `[history]` inside `[mrql]` result templates binds to each result entity, by design, and is tested (section 10). All per-entity call sites write exactly `group`, `resource` and `note` (section 1.2). IDs are never reused: `gorm.io/driver/sqlite` v1.6.0 emits `integer PRIMARY KEY AUTOINCREMENT`, and Postgres uses sequences. So a new entity never inherits a deleted entity's rows. |
| 8 | Minor points | The heading follows the file's existing ``## `[x]` -- Title`` pattern. `public/index.css` has no dark mode (no `prefers-color-scheme` rule; it reuses the existing `details.details-shortcode` colours). Forced colours repaint text colours, and a 2px solid outline survives them, per the AGENTS rule. **The parser/doc drift test is dropped.** A `[history]`-specific dispatch test through `Process` covers the risk for this change. |

## 1. Motivation, value and limits

### 1.1 The problem

Groups shared or maintained by a team need a "what changed recently" trail where people work: the sidebar or header of the group, resource or note. Today the only way to get one is the admin `/logs` page, filtered by hand to `EntityType` and `EntityID`, which is the link `displayLog.tpl` builds. `[plugin:widgets:activity]` lists recently updated *owned children*. It does not show the entity's own changes, and it cannot read the audit log, because no Lua API exposes `log_entries`.

`[history]` renders the newest `LogEntry` rows for the current entity: the action, the message and a relative time. Each row links to its `/log?id=N` detail page, and a footer link goes to the full filtered `/logs` view.

### 1.2 What the audit log records per entity (audited)

These are every `Logger()` call in `application_context/` that writes entity type `group`, `resource` or `note` with a non-nil id. These rows are the only ones `[history]` can show.

| Entity | Rows written with this entity's id |
|---|---|
| group | `Created group` (`CreateGroup`); `Updated group` (`UpdateGroup`, the full edit form/API); `Deleted group` |
| resource | `Created resource`, `Created resource from local path`, `Resource already exists, skipping` (create); `Updated resource` (`EditResource`); `Merged resources` (on the winner); `Removed from series`; `Deleted resource` |
| note | `Created note`, `Updated note` (`CreateOrUpdateNote`); `Created share token`, `Removed share token`; `Deleted note` |

These are **not** recorded against the entity's id, so they never appear:
- bulk tag, group and meta edits (`resource_bulk_context.go` and `note_bulk_context.go`, nil id);
- mass edits (`mass_edit_context.go`, nil id);
- inline name, description and meta edits (`EntityWriter.UpdateName`, `UpdateDescription` and `UpdateMetaAtPath` write no log row), which includes every meta-editor plugin write;
- relation changes (logged under the `relation` id);
- resource versions (logged under the `resource_version` id);
- note block edits;
- group bulk operations.

`Deleted …` rows exist but never render, because the entity's page is gone.

### 1.3 What it adds over `[property path="UpdatedAt"]`

`UpdatedAt` is one timestamp that every write moves, inline and bulk edits included. `[history]` gives:
- the *sequence* and cadence of full edits (create, then each form save, with times);
- events that leave no field trace: merges into a resource, share-token creation and removal on a note, removal from a series;
- a one-click path to each row's `/log?id=N` page, where the admin sees request path, IP and details. Those fields are not exposed in the card itself.

That value is modest but real. The UI calls the list "Logged changes", never "all changes", and the docs carry the table above.

### 1.4 Audience trade-off

The shortcode is admin-only, because `/logs` is admin-only and a template slot must not widen who can read the audit log. That makes it useful:
- in `-auth` deployments, for the administrators who oversee shared groups;
- in single-user `-auth`-off deployments, as an in-place convenience over `/logs`.

Editors, users and guests in multi-user deployments never see it. Showing them audit rows is a policy change to `/logs` visibility, a separate decision this plan does not make.

### 1.5 Dropped from the pitch

The pitch included "the acting user". `models.LogEntry` has no actor column and `Logger.log` records none, so there is nothing to show. Adding one means a schema change plus edits at every call site, which is a separate plan.

### 1.6 Not already built

`docs/ideas/category-templates-and-shortcodes.md` does not mention history, audit or activity. `GetEntityHistory` has no shortcode surface.

## 2. Syntax and attributes

```
[history]
[history limit="10"]
[history limit="5" actions="update,create"]
```

The shortcode is inline only (`BlockNo`). A `[/history]` closer gets the existing "closing tag on an inline-only shortcode" lint error.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `limit` | number | `5` | The maximum number of rows, 1–25. A value that is not a positive integer (`abc`, `0`, `-3`, empty) falls back to 5, as `parseIntAttr` does in `mrql_handler.go`. A value above 25 is capped at 25. |
| `actions` | string (comma-separated) | *(all actions)* | Restricts the rows to these actions. Allowed tokens are exactly the `models.LogAction*` values: `create`, `update`, `delete`, `system`, `progress`, `plugin`, `reset`. Tokens are trimmed, empty tokens are ignored and duplicates are removed. Matching is case-sensitive, because stored values are lower-case. If every token is empty, all actions are shown. An unknown token renders `shortcodeErrorMarker("history", ...)` with the detail ``actions= accepts create, update, delete, system, progress, plugin, reset; "<token>" is not one``, and no query runs. |

There are no other attributes.

## 3. Rendered output (accessible markup)

With rows (`<type>` is `group`, `resource` or `note`):

```html
<div class="history-shortcode">
  <ol class="history-shortcode-list" aria-label="Logged changes">
    <li class="history-shortcode-entry" data-action="update">
      <span class="history-shortcode-action">Updated</span>
      <a class="history-shortcode-message" href="/log?id=42">Updated group</a>
      <time class="history-shortcode-time" datetime="2026-11-24T09:12:03Z" title="2026-11-24 09:12 UTC">3 hours ago</time>
    </li>
    …
  </ol>
  <a class="history-shortcode-all" href="/logs?EntityType=group&amp;EntityID=7">All logged changes to this group</a>
</div>
```

- **List.** The rows are an `<ol>`, newest first. The order carries meaning, and screen readers announce the item count. The list has `aria-label="Logged changes"` and no heading, so the author controls the slot's heading hierarchy. It is not a `<section>`, so it adds no landmark to every sidebar.
- **Action label.** The action always appears as text, never as colour alone. `create`, `update` and `delete` read as `Created`, `Updated` and `Deleted`. `system`, `progress`, `plugin` and `reset` read as `System`, `Progress`, `Plugin` and `Reset`. Any other stored value is shown HTML-escaped as stored. `data-action` holds the raw value for `CustomCSS`.
- **Message.** The message is always HTML-escaped, whatever `UntrustedSource` says, because plugins write log rows too. It is truncated to 200 runes, with `…` appended when cut. An empty message falls back to the action label, so the link always has an accessible name.
- **Time.** `datetime` is `CreatedAt.UTC().Format(time.RFC3339)` and `title` is `CreatedAt.UTC().Format("2006-01-02 15:04 UTC")`. The visible text is `timeAgo(CreatedAt)`, the logic extracted from `timeago_filter.go`.
- **Footer link.** The footer text names its target ("All logged changes to this group"), so cards repeated on one page never produce identical, ambiguous link text.
- **Focus.** `:focus-visible` gets a 2px solid outline (section 9), which survives forced colours. Nothing uses `outline: none`.
- **Empty state.** `<div class="history-shortcode"><p class="history-shortcode-empty">No logged changes.</p></div>`, with no footer link.

## 4. Behaviour for every input and context

| Situation | Output | Query? |
|---|---|---|
| No history loader on the request context (any non-admin, or a surface listed as "not installed" in section 5.5) | `shortcodeComment("history unavailable in this context")`, i.e. `<!-- mr:history unavailable in this context -->` | no |
| `ctx.EntityType` is not `group`, `resource` or `note` (e.g. a carrier `CustomListHeader`), or `EntityID == 0` | the same comment | no |
| `actions` has an unknown token | the actions error marker | no |
| The loader returns an error satisfying `errors.Is(err, ErrHistoryBudgetExceeded)` | `shortcodeErrorMarker("history", err.Error())`, where the text is `inline query budget exceeded (N per page); refine templates or raise -mrql-page-query-budget` | no |
| The loader returns any other error | `shortcodeErrorMarker("history", "could not load the change log")`. The error text is never printed. | attempted |
| No rows | the empty state | yes (or a cache hit) |
| Rows | the list | yes (or a cache hit) |

The comment is identical for "not allowed", "not wired" and "wrong entity", as with `plugin unavailable in this context`. Comparing page source across viewers therefore reveals nothing about whether audit data exists.

**Inside `[mrql]` result templates.** `contextFromItem` binds `ctx` to each result entity, so `[history]` in `CustomMRQLResult`, or in `CustomSummary` on a card, shows that entity's rows. This is intended. Each distinct entity costs one budget unit (section 6).

## 5. Authorization and scoping (fails closed)

### 5.1 Who

The shortcode renders only for a principal with `CanManageSystem()`. That is the predicate guarding `/logs`, `/log` and `/v1/log*` through `isSystemPath`, and it is nil-safe and equal to `IsAdmin()`. With `-auth` off every request is the root admin, matching `/logs`.

### 5.2 The gate

`template_filters.WithEntityHistory(reqCtx, reader)` attaches the loader only when `reader != nil && auth.PrincipalFromContext(reqCtx).CanManageSystem()`. The handler can reach audit data only through the loader. Any render path that does not call `WithEntityHistory`, including future ones, renders the comment.

### 5.3 Group-subtree confinement

`IsGroupLimited` requires `!IsAdmin()`, so every subtree-confined principal is refused, even for entities inside its own subtree. That is stricter than subtree scoping, and it is what the admin-only audit rule requires. Admins are never confined.

### 5.4 Replayed tokens and stripped data

- **Replayed tokens.** A `[lazy]`, `[details]`, `[reload]` or `[autorefresh]` token minted on an admin page and POSTed to `/v1/shortcodes/deferred` by a non-admin renders under the caller's principal. The caller gets no loader, so the result is the comment.
- **Stripped data.** The application method selects only `id`, `created_at`, `action` and `message`. `HistoryEntry` has no field for `RequestPath`, `UserAgent`, `IPAddress`, `Details`, `Level` or `EntityName`, so a later change cannot render them by accident.

### 5.5 Surfaces

**Rule:** install the loader wherever a render goes back to the request's own principal *and* that surface installs a refresh signer. A refresh from such a surface goes through the deferred endpoint, which renders for admins, so the first paint and later refreshes always agree.

| Surface | Loader | How |
|---|---|---|
| Display pages, HTML list cards, `custom_css` (`buildPageRenderContext`) | yes | the last step: `if r, ok := appCtx.(EntityHistoryReader); ok { reqCtx = WithEntityHistory(reqCtx, r) }` |
| `/v1/shortcodes/deferred` (`deferred_render_api_handlers.go`) | yes | after `buildMRQLAPIRenderContext(..., true)` |
| `/v1/mrql?render=list` cards (`mrql_api_handlers.go` ~line 539, `listRender`) | yes | after `buildMRQLAPIRenderContext(..., true)`, by type assertion on `ctx`. The list-card renderer reuses this context as `_reqCtxWithCache`. |
| Lightbox JSON (`processShortcodesForJSON` in `server/routes.go`) | yes | `reqCtx = template_filters.WithEntityHistory(reqCtx, appCtx)` beside `WithRefreshSigner`. No page budget is wired there, so it is unbudgeted, but it is bounded to one entity's slots, and the per-render cache dedupes. |
| Human template preview (`template_preview_api_handlers.go`) | yes | after `buildMRQLAPIRenderContext` |
| Share server (`share_server.go`; `context.Background()`, no principal) | no | comment |
| `renderMRQLCustomTemplates` / grouped (`/v1/mrql` `render=1`, no signer) | no | comment |
| Description preview and entity picker (no signer) | no | comment |
| Template generation run tools (`template_run_tools.go`; model transcript, Privacy mode) | no | comment |

`buildMRQLAPIRenderContext` itself is unchanged. Each surface opts in at its call site, so a new caller fails closed.

## 6. Performance (millions of resources)

The loader issues one query per distinct `(entityType, entityID, sorted actions, limit)` per render:

```sql
SELECT id, created_at, action, message FROM log_entries
WHERE entity_type = ? AND entity_id = ? [AND action IN (?)]
ORDER BY id DESC LIMIT ?        -- ? ≤ 25
```

- **Index.** The query uses the existing composite `idx__log_entries__entity_type_entity_id` (`models/supplemental_indexes.go`). Its cost depends on one entity's row count, never on table size. No new index is needed.
- **Ordering.** `ORDER BY id DESC` matches insertion order and is deterministic on ties. On SQLite the index entries are already rowid-ordered within a key, so the query is a bounded backward scan with no sort step. On Postgres only one entity's small row set is sorted.
- **Budget.** A cache miss calls `budget.Allow()` on the existing per-page `QueryBudget` (default 200, `-mrql-page-query-budget`), the same budget `[mrql]` draws from.
  - On refusal the loader calls `budget.MarkExceeded()`. If that returns true and an `onExceeded` callback is set, the callback runs `logPageQueryBudgetExceeded(appCtx, limit)`. The loader then returns `fmt.Errorf("%w (%d per page); refine templates or raise -mrql-page-query-budget", ErrHistoryBudgetExceeded, limit)`, where the sentinel's text is `inline query budget exceeded`.
  - A nil budget means unbudgeted.
  - On a 50-card list with `[history]` in `CustomSummary`, the cost is at most 50 queries, and the page degrades to error markers rather than running unbounded.
  - `Stats().Exceeded` becomes true, so `renderMRQLListCards` adds its existing warning.
- **Cache.** A mutex-guarded per-render cache in the loader closure holds successful results only. Hits are free.
- **Output.** The 25-row cap bounds the output. No `COUNT` query runs.

## 7. Where it lives: a built-in Go handler, not a plugin

- Plugins have no audit-log API. Adding a Lua reader would widen the plugin surface onto an admin-only table.
- The gate is the request principal's system capability, decided in Go at the render seam. The same pattern is used by `WithCredentialFieldsVisible` and the plugin-access gate.
- The query needs the budget and the diagnostics in `shortcodes/` and `template_filters/`.

## 8. Design of the change

### `application_context/log_context.go`

```go
// GetRecentEntityHistory returns the newest log entries for one entity, newest
// first, optionally restricted to actions. It selects only id, created_at, action
// and message: callers that render it must never see request path, user agent,
// IP address or details.
func (ctx *MahresourcesContext) GetRecentEntityHistory(entityType string, entityID uint, actions []string, limit int) ([]models.LogEntry, error)
```

The body chains `Select("id", "created_at", "action", "message")`, `Where("entity_type = ? AND entity_id = ?", …)`, `Where("action IN ?", actions)` when `len(actions) > 0`, `Order("id desc")`, `Limit(limit)` and `Find`.

### `shortcodes/history_handler.go` (new)

- **Types:**
  - `type HistoryEntry struct { ID uint; Action, Message string; CreatedAt time.Time; Relative string }`
  - `type HistoryRequest struct { EntityType string; EntityID uint; Actions []string; Limit int }`. `Actions` is deduped and in authored order; the loader sorts it for its cache key.
  - `type HistoryLoader func(HistoryRequest) ([]HistoryEntry, error)`
- **Sentinel:** `var ErrHistoryBudgetExceeded = errors.New("inline query budget exceeded")`
- **Context helpers:** `WithHistoryLoader(ctx, l) context.Context` and `historyLoaderFrom(ctx)`, mirroring `WithPartialResolver`.
- **Renderer:** `RenderHistoryShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string` implements sections 2–4, with the error mapping in section 4.
- **Constants:** `defaultHistoryLimit = 5`, `maxHistoryLimit = 25`, `historyMessageRunes = 200`.
- **Vocabulary:** `historyActions`, in the same order as the `models.LogAction*` constants. A test pins them, because `shortcodes` must not import `models`.

### Dispatch, parsing, lint and docs entries in `shortcodes/`

- **`processor.go`:** add `case sc.Name == "history": replacement = RenderHistoryShortcode(reqCtx, sc, ctx)`.
- **`parser.go`:** add `history` to both `shortcodePattern` and `closingTagPattern`.
- **`lint.go`:**
  - add `"history"` to `builtinBaseNames`;
  - add `case "history":` that reports an **error** on an unknown `actions` token (the renderer's text);
  - a **warning** on a `limit` that is not a positive integer (`[history] limit must be a whole number from 1 to 25; it falls back to 5`);
  - a **warning** on a `limit` above 25 (`[history] limit is capped at 25`).
- **`builtin_docs.go`:** add a `BuiltinDoc`:
  - `IsBlock: BlockNo`;
  - attributes `limit` (number, default `5`) and `actions` (string, no `Enum`, because the value is a list);
  - a description saying it renders only for administrators, only the logged changes listed in the docs, and nothing on share pages, MRQL API renders, previews other than template preview, and generation runs;
  - examples `[history]`, `[history limit="10"]` and `[history actions="update,create"]`;
  - `history` added to the type comment's list.

### `server/template_handlers/template_filters/history_loader.go` (new)

- `type EntityHistoryReader interface { GetRecentEntityHistory(string, uint, []string, int) ([]models.LogEntry, error) }`. Add the assertion `_ EntityHistoryReader = (*application_context.MahresourcesContext)(nil)` to the `var (...)` block in `shortcode_interfaces.go`.
- `BuildHistoryLoader(reader EntityHistoryReader, budget *shortcodes.QueryBudget, onExceeded func(limit int)) shortcodes.HistoryLoader`:
  - It keeps a mutex-guarded cache keyed on `%q` type, id, the sorted actions joined by commas, and limit.
  - On a miss it calls `budget.Allow()`, following the exhaustion path in section 6. It then calls the reader and maps each row, setting `Relative` to `timeAgo(t)`.
  - It caches only successes.
- `WithEntityHistory(reqCtx context.Context, reader EntityHistoryReader) context.Context`:
  - It returns `reqCtx` unchanged unless the section 5.2 condition holds.
  - Otherwise it attaches `BuildHistoryLoader(reader, shortcodes.QueryBudgetFrom(reqCtx), cb)`. `cb` calls `logPageQueryBudgetExceeded(l, limit)` when `reader` also implements `budgetLogger`, and is nil otherwise.
  - It must run after `WithQueryBudget`.

### Other changes in `template_filters`

- **`shortcode_query_executor.go`:** the warning text becomes `inline query budget exceeded (%d per page); some [mrql] or [history] shortcodes were not executed. Refine templates or raise -mrql-page-query-budget.` No test asserts the old tail.
- **`timeago_filter.go`:** extract the body into `func timeAgo(t time.Time) string`. The filter becomes a two-line wrapper with unchanged behaviour.
- **`shortcode_tag.go` `buildPageRenderContext`:** add the type-asserted install from section 5.5 as the last step. Fakes such as `countingPageContext` keep compiling and fail closed.

### `server/api_handlers/`

- **`handler_interfaces.go`:** embed `template_filters.EntityHistoryReader` in `TemplatePreviewContext`. `DeferredRenderContext` inherits it. `MRQLAPIContext` is not widened; it uses a type assertion.
- **`deferred_render_api_handlers.go`, `template_preview_api_handlers.go`:** add `reqCtx = template_filters.WithEntityHistory(reqCtx, ctx)` right after `buildMRQLAPIRenderContext(...)`.
- **`mrql_api_handlers.go`:** inside `if listRender { … }`, add `if r, ok := ctx.(template_filters.EntityHistoryReader); ok { renderCtx = template_filters.WithEntityHistory(renderCtx, r) }` before `request.WithContext(renderCtx)`.

### `server/routes.go` `processShortcodesForJSON`

Inside the existing `if appCtx != nil { … }`, add `reqCtx = template_filters.WithEntityHistory(reqCtx, appCtx)`.

## 9. Files to change

- **Application:** `application_context/log_context.go`.
- **Shortcodes:** `shortcodes/history_handler.go` (new), `processor.go`, `parser.go`, `lint.go`, `builtin_docs.go`.
- **Template filters:** `server/template_handlers/template_filters/history_loader.go` (new), `shortcode_interfaces.go`, `shortcode_tag.go`, `shortcode_query_executor.go`, `timeago_filter.go`.
- **API handlers:** `server/api_handlers/handler_interfaces.go`, `deferred_render_api_handlers.go`, `template_preview_api_handlers.go`, `mrql_api_handlers.go`.
- **Routes:** `server/routes.go`.
- **`public/index.css`:** new rules next to `details.details-shortcode`, using semantic classes only, so `scripts/css-scan-test.sh` is unaffected:
  - `.history-shortcode-list { list-style: none; padding: 0; display: grid; gap: 0.25rem; }`
  - `.history-shortcode-action { font-weight: 600; color: #292524; }`
  - `.history-shortcode-time { color: #57534e; }` (AA on white)
  - `.history-shortcode-message:focus-visible, .history-shortcode-all:focus-visible { outline: 2px solid #b45309; outline-offset: 2px; }`
- **Reference panels:** `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl` each get an entry for `[history limit="5" actions="…"]`, as `TestReferencePanelsCoverAllBuiltins` requires.
- **Docs:** see section 11.

There are no new routes (so no OpenAPI, MCP safety-class or `isSystemPath` changes), no new flags and no new dependencies.

## 10. Tests

### `shortcodes/history_handler_test.go` (fake loader that records its calls)

1. With no loader, the output is exactly `<!-- mr:history unavailable in this context -->`.
2. With `EntityType` set to `category` or `""`, or with `EntityID` 0, the output is the same comment and the loader is never called.
3. Rows render in loader order inside `<ol aria-label="Logged changes">`:
   - labels for all seven actions plus an unknown one;
   - `href="/log?id=N"` and an RFC3339 UTC `datetime`;
   - a footer `href` containing `&amp;` and the text "All logged changes to this group";
   - `<img src=x onerror=1>` escaped, even with `UntrustedSource=false`;
   - a 300-rune message cut to 200 runes plus `…`;
   - an empty message shown as the action label.
4. The loader receives `limit` 5 for `""`, `abc`, `0` and `-3`; 10 for `10`; and 25 for `999`.
5. Action parsing:
   - `actions=" update, create,update ,"` passes `["update","create"]`;
   - `""` and `","` pass nil;
   - `actions="update,Edit"` renders the actions marker and never calls the loader.
6. A loader error `errors.New("pq: secret-db-detail")` renders `could not load the change log`, and the output does **not** contain `secret-db-detail`. A loader error wrapping `ErrHistoryBudgetExceeded` renders the budget text.
7. Zero rows render the empty state with no footer.
8. `[history]` dispatches through `Process`:
   - at top level, and inside `[conditional]`, `[each]` and `[using]` bodies;
   - inside a `[mrql]` whose fake executor returns two items, `group` 3 and `group` 4, with a per-item block template `[history]`. The loader receives two requests with ids 3 and 4, which proves per-row binding.
9. `historyActions` equals the `models.LogAction*` constants. This test lives in an external `_test` package that imports `models`.

### Parser and lint

- **Parser:** `[history limit="3"]` parses as `history` with its attributes.
- **`lint_test.go`:**
  - the unknown-action error;
  - both `limit` warnings;
  - no issue for `[history]` or `[history limit="25" actions="create,delete"]`;
  - the closer error for `[history][/history]`;
  - a "did you mean" suggestion of `history` for `[histroy]`.
- **Existing tests:** these now cover the new entry: `TestBuiltinDocMRQLExamplesParse`, `TestReferencePanelsCoverAllBuiltins`, `TestDocsSiteShortcodesPageCoversAllBuiltins` and `TestBuiltinGenerationPromptDocsAreComplete`.

### `template_filters/history_loader_test.go`

- `WithEntityHistory` installs a loader for an admin and for the auth-off root admin. It installs none for a nil principal, a guest, a user, a subtree-scoped user, an editor, or a nil reader.
- The same request twice calls the reader once. Two distinct requests call it twice.
- Budget limit 1, with a fake reader that also implements `Logger()`:
  - the second distinct request returns an error satisfying `errors.Is(err, shortcodes.ErrHistoryBudgetExceeded)` and does not call the reader;
  - `budget.Stats().Exceeded` is true;
  - `onExceeded` fires once, even after a third refused request;
  - a repeat of the first request is still served from the cache.
- A reader error is not cached: the next call retries.
- The existing timeago tests keep passing after the `timeAgo` extraction.

### `application_context` test: `GetRecentEntityHistory`

- It filters by type and id: a `resource` row with the same id is excluded, and so is a nil-id `group` bulk row.
- It filters by action, returns newest-by-id first, and respects `limit`.
- Returned entries have empty `RequestPath`, `UserAgent`, `IPAddress` and `Details`, even when the stored row has all four.
- On SQLite, `EXPLAIN QUERY PLAN` uses `idx__log_entries__entity_type_entity_id` and contains no `USE TEMP B-TREE FOR ORDER BY`. Call `models.EnsureSupplementalIndexes` if the setup does not already.

### Integration: `server/api_tests/history_shortcode_test.go`

The test uses `setupAuthEnv`, `doReq` and Bearer tokens, as in `mrql_scope_auth_test.go`. Setup:
- a Category whose `CustomSidebar` and `CustomSummary` are `[history]`;
- a group in it;
- an update made through the API with `User-Agent: history-ua-marker`.

Cases:
- **Admin.** `GET /group?id=N` contains `Updated group`, `history-shortcode` and `/log?id=`. It does not contain `history-ua-marker` or the client IP string.
- **Non-admins.** An editor, a user scoped to the group's own subtree, and a guest scoped to it all get the comment, with no `history-shortcode` or `Updated group`.
- **Deferred.** `POST /v1/shortcodes/deferred` with a token sealing `[history]` returns the list for the admin and the comment for the scoped user.
- **Lightbox.** `GET /v1/group?id=N` as JSON, on the path that runs `processShortcodesForJSON`, returns a processed `CustomSidebar` containing `history-shortcode` for the admin and the comment for the scoped user.
- **MRQL list cards.** `POST /v1/mrql?render=list` with `group where id = N` returns card HTML containing `history-shortcode` for the admin. With `-mrql-page-query-budget 1` and two groups, one card shows the budget marker, the response carries the existing budget warning, and `/v1/logs` contains the warning naming `[history]`.
- **Share server.** A shared note whose NoteType `CustomSidebar` is `[history]` renders the comment.
- **`render=1`.** `/v1/mrql` with `render=1` and `CustomMRQLResult` set to `[history]` renders the comment.
- **Auth off.** With `SetupTestEnv`, the list renders.
- **Postgres.** Run the file under `--tags 'json1 fts5 postgres'` too.

### E2E: `e2e/tests/shortcodes/history-shortcode.spec.ts` (ephemeral server, auth off)

1. Create a category with `CustomSidebar` set to `[history limit="3"]`, create a group in it, and save the edit form twice.
2. The sidebar list has 3 items and the first reads "Updated".
3. The first message link opens `/log?id=…`.
4. "All logged changes to this group" opens `/logs` filtered to the group.

**Accessibility:** add `e2e/tests/accessibility/history-shortcode-a11y.spec.ts` using `e2e/fixtures/a11y.fixture.ts`. It requires zero axe violations on that page and asserts the accessible name "Logged changes".

### Suites

Run each of these before and after the change:
- `go test --tags 'json1 fts5' ./...`;
- the Postgres command from AGENTS for `./server/api_tests/...`;
- `cd e2e && npm run test:with-server:all`;
- `npm run test:with-server:a11y`.

## 11. Docs to update

- **`docs-site/docs/features/shortcodes.md`:**
  - add `[history ...]` to the "Built-in inline" list;
  - add the table row ``| `[history]` | Inline | Logged audit changes to the current entity (administrators only) |``;
  - add a section ``## `[history]` -- Logged changes`` after `[autorefresh]`. It holds the attribute table, the markup, the admin-only rule and its audience trade-off, the section 1.2 table of what is and isn't recorded, the surfaces that render nothing, the shared query budget, and the note that no user, path, user agent, IP or details are shown.
- **`docs-site/docs/features/custom-templates.md`:** a bullet in the shortcode list (around line 507).
- **`.claude/skills/mahresources-category-designer/references/shortcodes.md`:** a ``### `[history]` `` entry that includes the "not recorded" caveat.
- **`docs/architecture/configuration.md`:** the `-mrql-page-query-budget` row notes that `[history]` cache misses draw from the same budget.
- **`docs/architecture/authentication-and-roles.md`:** one sentence: `[history]` is gated on `CanManageSystem` (the `/logs` rule) at the render seam (`WithEntityHistory`), and surfaces without the loader render nothing.
- **`docs/plans/`:** the plan file, then `./docs/plans/generate-index.sh`.

## 12. Success criterion

The work is done when all five of these hold:

1. On a display page, lightbox, MRQL list card or deferred render for a group, resource or note whose slot holds `[history limit="5" actions="update,create"]`, an administrator (or anyone with `-auth` off) sees:
   - at most 5 of that entity's create and update log rows, newest first, in an accessible ordered list labelled "Logged changes";
   - no request path, user agent, IP or details anywhere in the HTML.
2. These all get only `<!-- mr:history unavailable in this context -->`, with no query run:
   - an editor, a plain user, a guest, and a subtree-confined user (even inside their own subtree);
   - a share visitor, the `/v1/mrql` `render=1` path, the description preview, the entity picker and a generation run;
   - a replayed admin token at `/v1/shortcodes/deferred` used by a non-admin.

   The tests in section 10 prove each case.
3. Each distinct `[history]` costs exactly one indexed `LIMIT ≤ 25` query, charged to the per-page budget. Exhaustion renders the budget marker, sets `Stats().Exceeded` and logs one warning. On SQLite the plan uses `idx__log_entries__entity_type_entity_id` with no sort step. A database error never prints its text.
4. The linter flags bad `actions` and `limit` values. The docs, reference panels, autocomplete and generation prompt list `[history]`, and the docs state which changes the audit log does not record.
5. The Go suites (SQLite and Postgres), the browser and CLI E2E suites, and the a11y suite all pass.

---

## Last evaluator feedback

I checked the plan's code claims against the repo and most of them hold. `GetEntityHistory` exists. `idx__log_entries__entity_type_entity_id` is a supplemental index. `CanManageSystem` is the same check as `IsAdmin`, and `IsGroupLimited` requires `!IsAdmin`. The `BudgetedExecutor` exhaustion path (`Allow`, then `MarkExceeded`, then `onExceeded`) matches the loader design. The `processShortcodesForJSON` signer, the `listRender` call to `buildMRQLAPIRenderContext(...,true)` and the `/logs?EntityType=&EntityID=` link all match. The logged messages are fixed strings: `UpdateGroup` logs "Updated group".

The architecture is careful and fails closed. The problem is how little the feature delivers. Issues, most important first:

1) Usefulness is low. The plan admits the value is "modest", and the code confirms it. Every message is a fixed string ("Updated group", "Created note"), with no field names and no diff. The `/log?id=N` page adds no actor. Inline name, description and meta edits are not logged, and neither are bulk, mass, relation or block edits. On a well-used entity the widget reads "Updated / Updated / Updated" with timestamps.
   - Only admins see it, so in multi-user deployments, where a "what changed" trail matters most, the people collaborating never see it.
   - In single-user mode it saves one click over the existing link to `/logs` filtered by entity.
   - What would make it worth building is the missing logging (per-entity rows for inline edits, plus an actor column), and the plan defers both. Consider merging this with that work, or at least rendering the stored `EntityName` so renames are visible.

2) The surface rule contradicts its own table. Section 5.5 says to install the loader where a refresh signer exists. But the template preview calls `buildMRQLAPIRenderContext(..., false)`, which installs no refresh or deferred signer, and the table still marks it "yes". Either change the rule (for example, "human-facing surfaces rendered for the request's own principal") or drop the preview.

3) Success criterion 3 is wrong for the lightbox. It says every `[history]` is "charged to the per-page budget", but `processShortcodesForJSON` installs no `WithQueryBudget`, and section 5.5 itself calls the lightbox unbudgeted. Fix the criterion, or wire a budget into the lightbox path and test it.

4) Link text: every row's link text is the message ("Updated group"), so five links with the same name go to different targets. That passes WCAG 2.4.4 only if the list-item context counts, and screen-reader link lists will show five identical names. Put the time or the log id in the accessible name, for example `aria-describedby` pointing to the `<time>`, or include the time inside the link.

5) Log retention: rows disappear after `CleanupOldLogs`, and the docs table should say so. Otherwise "No logged changes." on an old entity is misleading.

6) Minor:
   - The warning text change in `shortcode_query_executor.go` affects every `[mrql]` budget warning. Grep the e2e and api tests for the old text, not only the Go unit tests.
   - The plan never says whether `[history]` is listed as allowed in model-preview and generation-run vocabularies. It renders the comment there, but the generation prompt docs test (`TestBuiltinGenerationPromptDocsAreComplete`) will put it in front of the model. The prompt should say it renders nothing outside admin pages.
   - "Created share token" rows sit against a note id: confirm the note owner, not the token, is the one logged.
