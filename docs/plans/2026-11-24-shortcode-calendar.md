# Plan: `[calendar]`, a month grid of dated entities (revision 2)

Status: implemented; review at the end.

## 0. Changes from revision 1 (one entry per evaluator point)

1. **Usefulness: titles, not just counts.** Day cells now list up to 3 entity links each, followed by a "+k more" link to the day query. For the common case this costs **one** query: a bounded list query fetches the month's entities (up to 101). When the month holds 100 or fewer, both titles and counts come from that one result, so they agree by construction. Only a dense month (more than 100) adds the aggregated `GROUP BY` count query and drops to counts only, with a visible note saying so. See §4 and §5.
2. **`$param` placeholders.** These are now rejected at compose time, so both the handler and lint reject them, with the message `[calendar] mrql cannot use $parameters`. Tested.
3. **Self-nesting swap.** The component lifecycle is now spelled out (§5.4):
   - A `<calendar-shortcode>` that sits inside another one does nothing on connect.
   - Init is `_init`-guarded and adds exactly one listener.
   - `reloadInto`'s bookkeeping is already cleared before the swap.
   - A response that contains no `<calendar-shortcode>` puts the old content back.
4. **Nav names and grid relationship.**
   - Buttons read "Previous month, February 2026". The visible text "February 2026" is part of that name.
   - The host is `role="group"`, labelled by the table caption, so the buttons and grid are announced as one named region.
   - Day links get their accessible names from visible text plus `sr-only` text, not from an `aria-label`, so the visible label is literally inside the name.
5. **Timezone.** There is now one rule: **a day is a calendar date on the server's local clock**. That is the clock `START_OF_MONTH()` and `[datetime]` already use. How each path meets the rule, and the one place it can deviate, is in §4.4. The deviation is documented.
6. **Meta value types.**
   - A `F ~ "????-??-??*"` guard is now added to every meta query and day link. Numbers, booleans and non-ISO strings therefore match nothing on both dialects, and the counts, titles and links stay consistent.
   - Range comparisons on meta strings are not `LOWER()`-wrapped. Only `=`/`!=` are, per `buildMetaClause`, so ISO strings compare as written.
   - There is a test with numeric and boolean `meta.due`.
7. **Per-card multiplication.** The exact budget-exhaustion output and the cost per card are now stated and documented. I push back on a lint warning: `LintOptions` has no slot identity, so lint cannot tell a card slot from a sidebar. Adding that field to every lint caller is outside this feature, and the runtime already reports exhaustion visibly (§8).
8. **Smaller points.**
   - The aggregated query has `ORDER BY F ASC`, so truncation is deterministic.
   - The a11y spec is renamed `calendar-shortcode-a11y.spec.ts`.
   - The Postgres test lives in `server/api_tests` (`//go:build postgres`, `SetupPostgresTestEnv`) and goes through the real shortcode executor and scoped context.
   - Lint takes the `calendarNow` seam.
   - `scope="entity"` is now described as the default fall-through of `resolveScopeKeyword`.
   - Paren-wrapping is covered: `parsePrimary` returns the inner expression, so `ExtractEntityType` still sees a top-level `type =`. A test asserts this, and EXPLAIN tests assert index use.

## 1. Motivation and user value

Event, journal, task and deadline categories have no calendar view today. Examples are groups with `meta.date`, notes with `meta.due`, and daily-log notes. The nearest option is `[mrql]` with `GROUP BY created.day COUNT()`. It gives a two-column `YYYY-MM-DD | count` table: no Meta dates, no month layout, no titles, no navigation.

`[calendar]` renders one month as an accessible grid. A typical personal month (100 entities or fewer) shows up to 3 linked titles per day. Each non-empty day links to its full list. Prev/Next swap the month in place through the existing deferred-render endpoint.

This is not the note `calendar` *block* (`CalendarEventsResponse`, ICS feeds). That block shows external events inside one note; `[calendar]` shows the app's own entities by a date field.

Already built and reused here, not re-proposed: the `[mrql]` date buckets, `[reload]`, `[autorefresh]`, `[lazy]`, and the `/v1/shortcodes/deferred` endpoint.

## 2. Where it lives: built-in Go handler (not a plugin)

- **Scoping.** Plugin Lua runs on the unscoped DB handle, so `auth.PluginCodeAllowed` gives group-confined principals "plugin unavailable". A plugin calendar would fail for exactly the users whose scoping matters. A built-in goes through the shortcode `QueryExecutor` (`executeMRQLForShortcode` → `ExecuteMRQLScoped` / `ExecuteMRQLGroupedWithScope`). That runs as the principal on the Find path, so the subtree callback applies, and it is wrapped by `BudgetedExecutor`, which supplies the per-page budget and result cache.
- **Navigation.** Navigation needs the per-render refresh signer (`refreshSignerFrom`). Only built-ins can reach it.
- **Lint.** `shortcodes/lint.go` already imports `mrql` and can check the composed queries statically.

## 3. Syntax and attributes

```
[calendar mrql='type = "note" AND noteType = "Task"' date="meta.due" month="current" scope="entity"]
```

The shortcode is inline only (`IsBlock: BlockNo`). A `[/calendar]` closer is a lint error, and the renderer ignores any body.

| Attr | Type | Required | Default | Values and rules |
|---|---|---|---|---|
| `mrql` | string | yes | — | An MRQL **filter expression only**, with exactly one entity type through a top-level-AND `type = …` (what `mrql.ExtractEntityType` reports). It must not contain `GROUP BY`, `ORDER BY`, `LIMIT`, `OFFSET`, `SCOPE` or `$parameters`. |
| `date` | string | no | `created` | `created`, `updated`, or `meta.<path>`, where `<path>` matches `^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`. A Meta value counts only when it is a JSON string starting `YYYY-MM-DD`: a JSON-Schema `format: date` value, or an RFC3339 date-time as `[datetime]` stores it. Its day is the date **as written** (the first 10 characters). |
| `month` | string | no | `current` | `current` (the month of `calendarNow()` on the server's local clock) or `YYYY-MM`, with year 1000–9999 and month 01–12. |
| `scope` | string | no | (entity) | As `[mrql]`: `global`, `parent`, `root`, or a numeric group ID. Any other value, including empty or `entity`, falls through `resolveScopeKeyword`'s default branch to the entity's own scope group. |

There are no other attributes. Week start (always Monday), the per-day title count (3), the list threshold (100), `param-*` and `saved=` are fixed or out of scope.

## 4. Queries (1 per render; 2 only for months with more than 100 items)

### 4.1 Composition

`shortcodes/calendar_handler.go` holds a pure function shared by the handler and lint:

```go
type calendarQueries struct {
    List  string                 // flat list query for the month
    Count string                 // aggregated per-day count query for the month
    Day   func(time.Time) string // MRQL for one day (link target)
    Field string                 // "created" | "updated" | "meta.x.y"
}
func composeCalendarQueries(filter, date string, first time.Time) (calendarQueries, error)
```

**Validation** (all errors are prefixed `[calendar]`):
- `mrql.Parse(filter)`. On failure, return the parse error.
- If `GroupBy != nil`, `len(OrderBy) > 0`, `Limit >= 0`, `Offset >= 0` or `Scope != nil`, return `mrql must be a filter only (no GROUP BY, ORDER BY, LIMIT, OFFSET or SCOPE)`. `Parse` defaults Limit and Offset to -1.
- If `len(mrql.ListParams(q)) > 0`, return `mrql cannot use $parameters`.
- If `mrql.ExtractEntityType(q) == EntityUnspecified`, return `mrql must name one entity type (type = "group", "resource" or "note")`.
- If `date` is invalid, return `date must be created, updated or meta.<path>`.

**Definitions.** `W` is `(\n<filter>\n)`. The newlines close any trailing line comment. Parentheses are transparent to `ExtractEntityType` because `parsePrimary` returns the inner node. `A` is the first day of the month and `B` the first day of the next month (December rolls into January), both formatted `"2006-01-02"`. `G` is the meta guard ` AND F ~ "????-??-??*"`, present only for `meta.*`.

The `~` operator goes through `convertMRQLWildcards`: `?` becomes `_` and `*` becomes `%`, with no `%…%` wrapping because the pattern has wildcards. So a value matches only if it is text with `-` at positions 5 and 8. JSON numbers and booleans fail on both dialects: SQLite's `json_extract` yields a number or 0/1, and Postgres's `->>` yields `"20260304"` or `"true"`.

| Query | Text | Executor options |
|---|---|---|
| `List` | `W AND F >= "A" AND F < "B"` + `G` + ` ORDER BY F ASC` | `Limit: 101` |
| `Count` (created/updated) | `W AND F >= "A" AND F < "B" GROUP BY F.day COUNT() ORDER BY F.day ASC` | `Limit: 1000` |
| `Count` (meta) | `W AND F >= "A" AND F < "B"` + `G` + ` GROUP BY F COUNT() ORDER BY F ASC` | `Limit: 1000` |
| `Day(d)` | `W AND F >= "d" AND F < "d+1"` + `G` | (link only) |

`ScopeGroupID` is the resolved scope for every call. The constants are `calendarListProbe = 101` (100 listed plus one probe row), `calendarItemsPerDay = 3` and `calendarMaxRows = 1000`.

### 4.2 Execution

1. Run `List`.
   - **100 or fewer items.** If `len(Items) <= 100`, this is **items mode**. Each item is placed on a day (§4.3), and counts, titles and total all come from this one result. No second query runs.
   - **More than 100 items.** If `len(Items) == 101`, this is **counts mode**. Run `Count`. Read each row's key column with a local `string`/`[]byte` coercion: `F.day` for created/updated, `F` for meta. Read `row["count"]` with a local coercion over `int64`, `int`, `float64`, `[]byte` and decimal `string`, the shapes `derefMRQLRows` leaves on SQLite and Postgres. Fold rows by the key's first 10 characters, keeping only keys that `time.Parse("2006-01-02", …)` accepts within [A, B). Titles are not shown.
2. Truncation in counts mode. Rows come back `ORDER BY` the key, so the 1000-row cap always drops the latest values. If `len(Rows) == 1000`, the grid still renders, with the cap note shown in §5.

Executing `List` with `ORDER BY F ASC` reuses `stableMRQLEntityOrder`, which appends `id` as a tie-breaker, so the 101-row probe is deterministic.

### 4.3 Placing items on days (items mode)

- **`created` / `updated`.** Read `traversePropertyPath(item.Entity, "CreatedAt" | "UpdatedAt", false)` as a `time.Time`, then take `.In(time.Local)` formatted as `2006-01-02`.
- **`meta.*`.** Decode `item.Meta`, then `navigateJSONValue(meta, path)`. The value must be a `string` whose first 10 characters parse as `2006-01-02`. Anything else is skipped, which cannot happen given guard `G`.
- **Out-of-window values.** Items whose day falls outside [A, B) are skipped. This can only happen for created/updated on Postgres when the session TimeZone differs from the server's (§4.4).
- **Names and links.** The item name is `extractEntityName(item)`, the existing helper. The link is `/<EntityType>?id=<EntityID>`, the same form `renderFlatList` uses.

### 4.4 The day rule

**Rule:** a day is a calendar date on the server's local clock. `month="current"` and the "today" highlight use `calendarNow().In(time.Local)`, where `var calendarNow = time.Now` is the test seam.

- **SQLite, created/updated.** GORM stores timestamps with the Go process's local offset (`2026-03-04 23:30:00+01:00`). The string-literal range `F >= "2026-03-04"` compares those stored strings lexicographically, which means by local date. That is the idiom MRQL's own validator recommends. The ranges, day links and items-mode placement (`.In(time.Local)`) therefore all agree with the rule.
- **Postgres, created/updated.** A `"YYYY-MM-DD"` literal against `timestamptz` is cast at midnight in the session `TimeZone`, and `to_char` also uses the session zone. The rule holds when the database `TimeZone` equals the server's zone; both default to UTC in the documented container setup. The docs say to set them equal.
- **Meta.** The date as written. It is a string and depends on no zone.
- **The one deviation.** In counts mode only (months with more than 100 items), SQLite's `F.day` is `strftime`, which normalises to UTC. On a non-UTC SQLite server, a dense month's counts can therefore put an entity within the UTC offset of midnight on the neighbouring day. An entity on the first or last day of the month can drop out of both months' counts, while its day link still lists it under its local date. The docs state this beside the counts-mode note. Items mode, which covers typical journals and task lists, has no deviation.

## 5. Rendering

### 5.1 Markup

Styles go in `public/index.css` beside `reload-shortcode`, with dark-mode variants. Tailwind utilities are used where they already exist.

```html
<calendar-shortcode class="calendar-shortcode" role="group" aria-labelledby="cal-Xy12…-cap"
                    data-prev-token="…" data-next-token="…">
  <div class="calendar-shortcode-nav">
    <button type="button" data-calendar-nav="prev"><span aria-hidden="true">‹</span> <span class="sr-only">Previous month, </span>February 2026</button>
    <button type="button" data-calendar-nav="next"><span class="sr-only">Next month, </span>April 2026 <span aria-hidden="true">›</span></button>
  </div>
  <table class="calendar-shortcode-grid">
    <caption id="cal-Xy12…-cap">March 2026 <span class="calendar-shortcode-total">(12 items)</span></caption>
    <thead><tr><th scope="col"><span aria-hidden="true">Mon</span><span class="sr-only">Monday</span></th> … Sun</tr></thead>
    <tbody>
      <tr>
        <td class="calendar-shortcode-pad"></td> …
        <td>
          <a class="calendar-shortcode-daylink" href="/mrql?q=…"><span class="sr-only">March </span><span class="calendar-shortcode-day">4</span><span class="sr-only">, </span><span class="calendar-shortcode-count">5</span><span class="sr-only"> items</span></a>
          <ul class="calendar-shortcode-items">
            <li><a href="/note?id=12">Dentist</a></li>
            <li><a href="/note?id=15">Call Bob</a></li>
            <li><a href="/note?id=19">Pay rent</a></li>
          </ul>
          <a class="calendar-shortcode-more" href="/mrql?q=…">+2 more<span class="sr-only"> items on March 4</span></a>
        </td>
        <td><span class="calendar-shortcode-day" aria-hidden="true">5</span><span class="sr-only">March 5, no items</span></td>
      </tr> …
    </tbody>
  </table>
  <!-- counts mode only -->
  <p class="calendar-shortcode-note">Showing counts only: more than 100 items this month.</p>
  <!-- counts mode and len(rows) == 1000 only -->
  <p class="calendar-shortcode-note">Counts may be incomplete: more than 1000 distinct date values this month.</p>
</calendar-shortcode>
```

The ID prefix is `"cal-" + rand.Text()[:12]` (`crypto/rand`, as `each_handler.go` already uses), so two calendars on one page never share an ID.

### 5.2 Grid and cells

**Grid**
- The week starts on Monday, matching MRQL `created.week`.
- A month renders 4 to 6 `<tr>`.
- Days belonging to the neighbouring months are empty `<td class="calendar-shortcode-pad">`.

**Accessible names**
- Day links take their name from text: "March 4, 5 items" or "March 9, 1 item".
- Empty days are plain text that reads "March 5, no items".
- Names use English month names (`time.Month.String()`). The caption total uses the same singular and plural forms: "(1 item)", "(N items)", "(no items)".

**Item list (items mode)**
- A day shows at most 3 item links, in `List` order (date, then ID).
- When a day has more than 3 items, a "+k more" link to `Day(d)` follows.
- Counts mode has no `<ul>`; only the day link carries the count.

**Links**
- Day links and "+k more" go to `/mrql?q=` + `url.QueryEscape(Day(d))`.
- When the resolved scope is non-zero and not `mrql.UnresolvedScopeSentinel`, the day query is first passed through `mrql.InsertScopeClause(…, scope)`, as `executeMRQLForShortcode` does for its view-all link.
- With the sentinel, no links are emitted.

**Today, escaping and overflow**
- **Today.** When the shown month contains today's server-local date, that cell's day link or day span gets `aria-current="date"`, and the cell gets the `calendar-shortcode-today` class.
- **Escaping.** Every interpolated string goes through `html.EscapeString`. That covers entity names, which `extractEntityName` returns raw.
- **Overflow.** Item names are clipped by CSS (`truncate`), and the full text stays in the accessibility tree. Focus-visible rings and today's border have at least 3:1 contrast in light and dark.

### 5.3 Navigation tokens (server)

- **When buttons render.** Prev/Next render only when `refreshSignerFrom(reqCtx) != nil && isDeferrableEntity(ctx)`. That covers display pages, cards, the lightbox JSON render and deferred re-renders. On share pages, live preview and list-header carriers there are no tokens and no nav.
- **Token contents.** Each token is `signer(ctx.EntityType, ctx.EntityID, body, ctx.UntrustedSource)`, where `body` is the shortcode re-serialized with `month` set to the adjacent absolute `YYYY-MM`.
- **Serialization.** Only the attributes the author wrote, plus `month`, are carried, in sorted key order. Each value is quoted with `"`, or with `'` when it contains `"`. A value containing both cannot have parsed.
- **Year bounds.** There is no Prev at 1000-01 and no Next at 9999-12.

### 5.4 `src/webcomponents/calendarShortcode.js` (new; imported in `src/main.js` beside `reloadShortcode.js`)

**Connect**
- `connectedCallback()` returns at once if `this._init` is set, or if `this.parentElement?.closest('calendar-shortcode')` is non-null. The second case is the transient copy that exists inside the old host during a swap. A calendar has no body, so one calendar is never legitimately inside another.
- Otherwise it sets `this._init = true` and adds **one** delegated `click` listener on `this`.

**Click**
1. Act only when `e.target.closest('button[data-calendar-nav]')` exists, its closest `calendar-shortcode` is `this`, and `this._busy` is false. Then set `this._busy = true`.
2. Read `token = this.dataset.prevToken | nextToken`.
3. Call `reloadInto(this, this, token, undefined, () => { saved = Array.from(this.childNodes); })`. `beforeSwap` runs only for a swap that lands, immediately before `innerHTML` is replaced.

**Bookkeeping.** `reloadInto`'s `current()` already removes `aria-busy`, the `deferred-reloading` class and the `activeReloads` entry before it swaps. `reloadGenerations` is a `WeakMap`, so the discarded host leaves nothing behind. `hydrate()` runs `Alpine.initTree` on content with no Alpine directives, which does nothing.

**Resolution**
- **Resolves `true` with a fresh calendar.** If `fresh = this.querySelector(':scope > calendar-shortcode')` exists, call `this.replaceWith(fresh)`. The move reconnects `fresh` with no calendar ancestor, so it initialises exactly once with one listener. Focus the same-direction button in `fresh`, or `parkFocus(fresh)` when that button is absent at a year bound. Then call `window.mahAnnounce(fresh caption text)`.
- **Resolves `true` without one.** This covers an error marker, an `unavailableComment` or a budget error. Call `this.replaceChildren(...saved)`, set `_busy = false`, focus the clicked button again, and call `mahAnnounce("Could not load the month", { assertive: true })`.
- **Resolves `false`.** Something else took over the content, for example a page morph or an enclosing `[reload]`. Set `_busy = false` and do nothing else.
- **Rejects.** The old content is untouched. Set `_busy = false` and make the same assertive announcement.

**Interaction with `[reload]` and `[lazy]`.** A `[reload]` around a calendar, through its region or an enclosing `[lazy]`, re-renders the authored month. This is documented.

## 6. Empty, missing and invalid inputs

| Situation | Output |
|---|---|
| Executor nil | `shortcodeComment("calendar unavailable in this context")` |
| `ErrQueryUnavailable` (model preview with MRQL off) | `unavailableComment(err)` |
| `mrql` missing or blank | `shortcodeErrorMarker("calendar", "[calendar] requires mrql=")` |
| Bad `month` | marker `[calendar] month must be "current" or YYYY-MM` |
| Bad `date`, clause present, `$param` present, no entity type, parse error | marker with the `composeCalendarQueries` error |
| Executor or MRQL runtime error, including the budget error `inline query budget exceeded (N per page); refine templates or raise -mrql-page-query-budget` | `mrqlErrorHTML(err, false)`, as `[mrql]` does; the page-level budget notice from `onExceeded` also shows |
| Meter stopped (`!m.alive()`) | `""` |
| Inside a `[reload]` face (`withinReloadFace`) | marker `[calendar] cannot be used inside a [reload] button face` |
| Zero results | Full grid, caption "(no items)", every day "no items", no links |
| Unresolved scope sentinel (ownerless entity, entity scope) | The query returns nothing, so the grid is empty and has no links (fails closed) |
| Meta values that are numbers, booleans or non-ISO strings | Not counted, not listed, not in day links (guard `G`) |

## 7. Scoping and authorization

- **Counts and titles.** Both come only from the executor, which runs as the request principal. A group-confined user's queries carry the subtree predicate through the GORM callback on the Find path, so `scope="global"` or an outside numeric scope still counts and lists only rows in the user's subtree. This is the same as `[mrql]`. There is no direct DB access.
- **Links.** Item links go to the normal detail pages and day links go to `/mrql`. Both are principal-scoped, so a link never shows more than the grid did.
- **Navigation tokens.** These are AES-GCM sealed `(type, id, body, untrusted)` tuples, so the client cannot alter `mrql`. `/v1/shortcodes/deferred` reloads the entity through the scoped context, so an outside-subtree token returns 404, and the `untrusted` flag carries over.
- **No new routes.** No new route is added, so no MCP safety class is needed.

## 8. Performance (millions of rows)

**Cost per render**
- **Items mode** (100 or fewer items): one query, `LIMIT 101`.
- **Counts mode**: two queries (`LIMIT 101` plus an aggregate `LIMIT 1000`).
- Each query is charged once to `-mrql-page-query-budget` (default 200).
- Identical calendars on one page (same text, scope and options) are served from the budget's result cache.
- Navigation costs one or two queries per click, on its own request budget.

**Index use**
- Both queries are month-range predicates.
- For `created`/`updated` they must use the `created_at`/`updated_at` indexes. Add EXPLAIN tests next to `mrql/explain_test.go` (SQLite) and `mrql/explain_pg_test.go` (Postgres) for the paren-wrapped composed `List` and `Count` queries.
- For `meta.*`, the `>=`/`<` comparison is the plain extraction expression with no `LOWER()` (`buildMetaClause` lowers only `=`/`!=`). *Correction after review:* no metadata expression index applies. Text metadata indexes are built on `LOWER(...)`, and `USING HASH` on Postgres, so they serve equality only. The docs page says no index serves a `meta.*` range and tells authors to narrow the filter.

**Per-item slots**
- In `CustomMRQLResult`, `CustomAvatar`, `CustomCell` or card slots with entity scope, every card has a distinct scope. That means a distinct cache key, so N cards cost N to 2N queries.
- A 150-card list exhausts the default budget. The remaining cards render the `mrqlErrorHTML` budget box, and the page shows the existing budget notice.
- This is documented: the docs advise putting calendars in sidebars and headers, or using a fixed `scope`, which caches.
- **No lint warning (pushback).** Lint has no slot identity (`LintOptions` carries no slot name), so it cannot warn. Threading one through every lint caller would be a separate change.

**Other bounds.** The render-context query timeout applies, output is charged to the render meter as usual, and the output is bounded at 42 cells × (3 titles + 2 links).

## 9. Files to change

- [x] `shortcodes/calendar_handler.go` (new): `RenderCalendarShortcode(reqCtx, sc, ctx, renderer, executor, depth)`, `composeCalendarQueries`, item placement and row folding, grid builder, attribute serializer, constants, `var calendarNow = time.Now`.
- [x] `shortcodes/parser.go`: add `calendar` to `shortcodePattern` and the closer pattern, so a stray `[/calendar]` is recognised.
- [x] `shortcodes/processor.go`: `case sc.Name == "calendar":` dispatch, with the nil-executor comment, mirroring `[mrql]`.
- [x] `shortcodes/lint.go`:
  - Add `"calendar"` to `builtinBaseNames`.
  - Add `case "calendar":` errors for missing `mrql`, bad `month`, bad `date`, `$param`, clauses, missing entity type and reload-face nesting. Shape errors run only when the filter parses; parse errors are left to the MRQL pass.
  - In the `ValidateMRQL` switch, validate `List` with `MRQLCheckOptions{Limit: 101}` and `Count` with `{Limit: 1000}` for the authored month (or `calendarNow`'s month), anchored at the `mrql` attribute offset.
- [x] `shortcodes/builtin_docs.go`: a `calendar` entry with the attributes above and these examples, each of which must pass `TestBuiltinDocMRQLExamplesParse`:
  - `[calendar mrql='type = "note"' date="meta.due"]`
  - `[calendar mrql='type = "resource"' date="created" month="2026-03" scope="global"]`
- [x] `src/webcomponents/calendarShortcode.js` (new), plus the import in `src/main.js`. Run `npm run build-js`.
- [x] `public/index.css`: grid, pad, today, count, item list, "more", note and nav styles, with dark variants. Run `./scripts/css-scan-test.sh`.
- [x] `server/routes_openapi.go`: add "or a [calendar] month change" to the `/v1/shortcodes/deferred` description, then regenerate `openapi.yaml` (`go run ./cmd/openapi-gen`).
- [x] Docs: see §11.

## 10. Tests

**`shortcodes/calendar_handler_test.go`** (fake executor that records calls, fake signer, fixed `calendarNow`, `time.Local` pinned per test)
- **Composition.**
  - The exact `List`, `Count` and `Day` text for created, updated and `meta.due`, including guard `G` and `ORDER BY`.
  - December→January rollover.
  - The options: `List` Limit 101, `Count` Limit 1000, and `ScopeGroupID` for global, parent, root, numeric, `entity` and empty.
  - `ExtractEntityType` on the composed queries equals the filter's type, which proves the parens are transparent.
  - Rejections: `$x` filters, each forbidden clause, a type under `OR`.
- **Modes.**
  - 100 items gives exactly one executor call (items mode).
  - 101 items gives two calls (counts mode) and the counts-only note.
  - The aggregate returning 1000 rows gives the cap note.
- **Placement.**
  - `CreatedAt` near midnight under a non-UTC `time.Local` lands on the local day.
  - Meta `2026-03-04` and `2026-03-04T23:30:00-05:00` land on March 4.
  - Numeric, boolean and non-ISO meta values are skipped.
  - Out-of-window values are skipped.
- **Counts-mode folding.** Counts in int64, float64, `[]byte` and string forms are all read. Raw meta keys fold by date; non-date keys are ignored.
- **Grid shape.**
  - March 2026 (starts Sunday) has 6 leading pads and 5 rows.
  - February 2027 has 4 rows.
  - February 2028 has 29 days.
  - The cell count is always a multiple of 7.
- **Accessibility markup.**
  - The computed text of day links is "March 4, 5 items" and "March 9, 1 item".
  - Empty days read "March 5, no items" and have no `<a>`.
  - A day with 5 items shows 3 `<li>` and "+2 more items on March 4".
  - `aria-current="date"` appears only in the current month.
  - `th` cells carry `scope="col"`.
  - The host has `role="group"`, and its `aria-labelledby` matches the caption ID.
  - Nav buttons read "Previous month, February 2026" and "Next month, April 2026".
  - Two calendars on one page get distinct IDs.
  - Entity names with `<script>` are escaped.
- **Links.** The day `href` decodes to `Day(d)` with SCOPE spliced for a non-zero scope, and with no SCOPE for global. The sentinel scope gives no links.
- **Navigation.**
  - With a signer and a deferrable entity, both tokens are present.
  - The bodies the signer received re-parse (`parseWithBlocks`) to the original attributes with `month` set to 2026-02 and 2026-04.
  - The `untrusted` flag is passed through.
  - No signer, a carrier context, 1000-01 and 9999-12 each drop the relevant buttons.
- **Errors.** Every row of the §6 table has a test, including a fake executor returning the budget error.

**`shortcodes/lint_test.go`**
- Each lint error fires, `$param` included.
- `ValidateMRQL` receives `List` with Limit 101 and `Count` with Limit 1000 for an injected `calendarNow`.
- A valid calendar is clean.
- `[/calendar]` gives the stray-closer diagnostic.

**Existing guards.** `builtin_docs_consistency_test.go`, `TestBuiltinDocMRQLExamplesParse` and `TestReferencePanelsCoverAllBuiltins` pass after the docs and panels are updated.

**`mrql/explain_test.go` and `mrql/explain_pg_test.go`.** The composed `created` `List` and `Count` queries use the `created_at` index.

**`server/api_tests/calendar_shortcode_test.go`** (SQLite, description-preview path, as `using_shortcode_scope_test.go`)
- **Real data.**
  - Seed notes with fixed `created_at` and `meta.due`: a date, a date-time, a numeric value and a boolean.
  - Items-mode titles and counts per day are correct, and numeric and boolean values are absent.
  - Fetching each day link's `/mrql?q=` as JSON returns exactly the IDs shown and counted.
- **Counts mode.** Seed 150 notes in one month and check the counts-only note and the per-day counts.
- **Scope** (`setupAuthEnv`, `buildScopingFixture`):
  - A confined bearer with `scope="global"` and with an outside numeric scope sees only in-subtree notes, in both titles and counts.
  - An out-of-subtree note on the same day is not counted.
  - A control shows the in-subtree note is counted.
- **Deferred navigation.** An in-subtree nav token POSTed to `/v1/shortcodes/deferred` returns the adjacent month's grid. An admin-minted token for an outside entity returns 404 for the confined bearer.

**`server/api_tests/calendar_shortcode_pg_test.go`** (`//go:build postgres`, `SetupPostgresTestEnv`)
- The same seed, rendered through the real shortcode executor and the principal-scoped app context (`env.AppCtx.WithPrincipal`).
- Asserts the same items-mode and counts-mode results as SQLite. That covers `to_char` buckets, the jsonb `->>` range and guard `G` rejecting numeric jsonb.
- A confined principal sees only its subtree.

**`src/webcomponents/calendarShortcode.test.ts`** (Vitest, `reloadInto` mocked)
- A click calls `reloadInto` with the right token.
- **Success.** The host is replaced by the inner calendar, the inner element inits once (one listener, so a later click sends one request), focus lands on the same-direction button or on the host at a bound, and `mahAnnounce` gets the caption.
- **A response without a calendar.** The old children are put back, the announcement is assertive, and a second click works.
- **Rejection.** The content is kept, the announcement is assertive, and a second click works.
- **Abandoned (`false`).** No announcement, and the component is not busy afterwards.
- A double click sends one request.
- A click on a foreign button inside the host is ignored.

**`e2e/tests/108-calendar-shortcode.spec.ts`**
- Set up a category whose `CustomSidebar` holds a calendar over notes owned by the group, with dates this month and next.
- The grid shows titles and counts.
- Next sends a deferred POST, the caption changes, focus stays on Next, and next month's titles appear.
- A day link and a title link open the expected pages.

**`e2e/tests/accessibility/calendar-shortcode-a11y.spec.ts`**
- axe reports no violations before and after navigation, in light and dark.
- Keyboard-only: Tab reaches Prev, Next, the day links, the titles and "+k more" in order, and Enter on Next works.

**Suites.** Run `go test --tags 'json1 fts5' ./...`, the postgres-tagged suite, `cd e2e && npm run test:with-server` and `npm run test:with-server:a11y`, before and after.

## 11. Docs to update

- [x] **`docs-site/docs/features/shortcodes.md`**
  - Add `[calendar]` to the "Built-in inline" list.
  - Add the table row `` | `[calendar]` | Inline | Month grid of entities by a date field, with titles, day links and month navigation | ``.
  - Add a section `` ## `[calendar]` -- Month grid of dated entities `` covering: the attributes, the filter-only rule and `$param` rejection; the Meta date format and "date as written"; the Monday start; items mode versus counts mode (the 100 threshold, 3 titles per day) and their query cost; the day rule and the Postgres `TimeZone` advice; the counts-mode SQLite non-UTC deviation; the 1000-row cap; per-card budget cost; the surfaces without navigation (share, preview, list header); the `[reload]` interaction; the metadata expression index tip.
- [x] **`docs-site/docs/features/custom-templates.md`**: one line in the shortcode overview that links to the new section.
- [x] **Reference panels** `templates/createCategory.tpl`, `templates/createResourceCategory.tpl` and `templates/createNoteType.tpl`: a `[calendar …]` entry each.
- [x] **`.claude/skills/mahresources-category-designer/references/shortcodes.md`**: an entry and an event-category example.
- [x] **`docs/ideas/category-templates-and-shortcodes.md`**: mark calendar as built if the file tracks built items.

## 12. Success criterion

On a group whose category sidebar holds `[calendar mrql='type = "note"' date="meta.due" scope="global"]`, the following must hold:

1. An admin sees this month with up to 3 linked titles per day, "+k more" where needed, and correct counts, on SQLite and Postgres. The page issues exactly one MRQL query for it.
2. With more than 100 notes in the month, the grid shows counts only, with the note, using two queries.
3. A guest confined to one subtree sees only that subtree's titles and counts, and gets 404 when replaying a nav token for an outside entity.
4. Prev/Next change the month without a page load, keep focus on the same-direction button, and announce the new month.
5. Every day link lists exactly the entities the cell counted, and numeric or non-ISO `meta.due` values appear nowhere.
6. axe reports no violations.

Every test in §10 passes, and the existing consistency, lint, Go, Postgres and E2E suites stay green.

## 13. Review (implementation record)

Built as planned, on SQLite and Postgres. Every §9 item and every §10 test file exists and passes; §11 docs are updated except `docs/ideas/category-templates-and-shortcodes.md`, which does not track built items, so it is unchanged.

**Deviations from the plan, each deliberate**

- **Day-link markup.** The plan interleaved visible and `sr-only` pieces (`<sr>March </sr>4<sr>, </sr>5<sr> items</sr>`). Chromium adds a space beside an absolutely positioned (`sr-only`) span when it computes a name, so the link read "March 4 , 5 items" (caught by the E2E spec). The name is now one `sr-only` sentence, "March 4, 5 items", and the two visible digits are `aria-hidden`. Both visible numbers are still in the name, in the order shown, the same shape the empty-day cell already had.
- **Target size.** axe's `target-size` rule failed on 15px title links. Every link and button in the grid is now at least 24px tall (`min-height: 1.5rem`), so day cells are taller than a compact calendar.
- **"+1 more item".** Singular for one; "+k more items on …" otherwise.
- **Wrapping.** `W` is `(<filter>)` without the newlines: MRQL has no line comments, so they guarded nothing and only made the day-link URLs uglier.
- **Signature.** `RenderCalendarShortcode(reqCtx, sc, ctx, executor)`: it renders no nested template, so it takes no renderer or depth.
- **Grid shape test.** §10 said March 2026 has 5 rows; 6 leading pads plus 31 days is 37 cells, so it has 6. The test asserts 6.
- **Dark mode.** The app has no dark theme. The calendar takes the slot's text colour (`currentColor` lines and badges), so it reads on the lightbox's dark panel, and the focus ring and today's outline are amber-700, which is 5.0:1 on white and 3.5:1 on stone-900. The a11y spec scans the one theme there is.
- **E2E paths.** The specs follow the existing layout: `e2e/tests/shortcodes/calendar-shortcode.spec.ts` and `e2e/tests/accessibility/calendar-shortcode-a11y.spec.ts`. They use a fixed `month="2026-03"` so they cannot flake at a month boundary.
- **EXPLAIN tests** are in their own files beside the existing ones (`mrql/calendar_explain_test.go`, `mrql/calendar_explain_pg_test.go`). The Postgres one prices sequential scans out inside a transaction, since the planner scans a tiny table whatever indexes exist.
- **Lint, missing `mrql`.** Reported by the generic required-attribute check (`[calendar] is missing required attribute "mrql"`); the renderer's marker says `[calendar] requires mrql=`.
- **Plan file name.** Saved as `2026-11-24-shortcode-calendar.md`, matching its sibling shortcode plans.

**Verified**

- `go test --tags 'json1 fts5' ./...` and the postgres-tagged `mrql` and `server/api_tests` calendar tests.
- Vitest (`npm run test:unit`), `./scripts/css-scan-test.sh`, `npm run build`.
- Both E2E specs against an ephemeral server.

**Not done / open**

- No lint warning for a calendar in a per-card slot (the §8 pushback stands).
- The counts-mode SQLite deviation on non-UTC servers (§4.4) is documented, not fixed.
