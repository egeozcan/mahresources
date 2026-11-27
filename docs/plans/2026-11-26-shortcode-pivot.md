# Plan: `[pivot]`, a built-in shortcode for two-field cross-tabs (rev. 4)

Status: implemented; review at the end.

## 0. Changes from rev. 3

I accepted every review point. I checked each fix against the code.

1. **Sum cells are never linked** (§6.2, §11, §12). Links appear only in count mode. In sum mode the table shows plain numbers and has no `<a>`. Success criterion 2 and integration test 11.3.2 now apply to count pivots only. A unit test checks that a sum pivot contains no `<a`.
2. **Raw single-part datetime dimensions are rejected** (§3.1). `rows="created"` or `cols="startDate"` (any single-part `FieldDateTime`) gives this error: `[pivot] rows: "created" is a timestamp; use a date bucket like created.month or created.year`. The message uses the attribute's own name. A rejection test covers it. As a result no `time.Time` key ever reaches bucket conversion. The defensive conversion rule in §5.2 is the only fallback.
3. **A pivot-specific example test** (§11.2). `TestPivotBuiltinExamplesLintClean` lints every `pivot` example in `BuiltinDocs()` with `ValidateMRQL` wired to `application_context.CheckTemplateMRQL`. It runs on both the SQLite and Postgres static environments and asserts **zero issues of any severity**.
   - Message prefixes are now stated exactly. Structural errors start with `[pivot] `.
   - The composed-query check goes through the existing `check` closure with label `query`. Its messages therefore start with `MRQL error in query: `, which `TestBuiltinDocMRQLExamplesParse` also catches.
4. **Boolean labels are specified per database** (§5.2, §11.3).
   - **SQLite:** `json_extract` returns JSON `true`/`false` as integer `1`/`0`. The bucket label is `1` or `0`, and it merges with the number `1`/`0` and the string `"1"`/`"0"`.
   - **Postgres:** `->>` returns `true`/`false`. The label is `true`/`false`, and it merges with the string `"true"`/`"false"`.
   - In both cases the link (a string literal of the label) matches exactly the merged bucket.
   - This is documented as a known display difference. The integration test asserts each database's label.
5. **Truncation is deterministic** (§4). The composed query ends `ORDER BY count DESC, <rows> ASC, <cols> ASC`.
   - Aggregated-mode ORDER BY accepts group-by field names (`buildAggregateOrderKeys`) and orders by the surviving alias, so this is valid on both databases. The unit test pins the text.
   - Results are stable from one render to the next on a given database. SQLite and Postgres sort NULLs differently, so which tied rows survive a cut-off can differ between the two databases. Truncated tables have no links, so this affects only which numbers are shown.
   - The warning text now says the kept rows are the combinations "with the most items", even when `value="sum"`.
6. **Minor points.**
   - `field` is restricted to `meta.<path>`, or to `fileSize`, `width` or `height` (resource only). `id`, `category` and `noteType` are rejected with `[pivot] field must be a meta.* path or fileSize, width or height`.
   - ASCII case-merging applies to **every** string-valued bucket: meta, scalar `FieldString` (`name`, `contentType`, …) and relation names. MRQL `=` on those fields is `LOWER(...) = LOWER(?)` too, so merging keeps links exact.
   - The e2e file number is taken at implementation time as the next free `NNN-` prefix. `107` is the highest today, so the file is `108` unless a sibling shortcode lands first.
   - The scope is reduced. v1 links only count-mode buckets, and only when they are not truncated, not a date bucket, not `shared`, not none-buckets, and not fed by a SQLite float. Each rule is a single boolean check in one function, `pivotLinkable`.

## 1. Motivation and user value

Dashboards need matrices: tasks by status and assignee, resources by content type and year, notes by owner and note type. Today the options fall short:

- `[mrql query="… GROUP BY a, b COUNT()"]` returns a long `a | b | count` list, with no totals and nothing to click.
- `plugin:data-views:bar-chart` and `pie-chart` show only one dimension.
- The roadmap in `docs/ideas/category-templates-and-shortcodes.md` has no cross-tab.

`[pivot]` runs **one** aggregated MRQL `GROUP BY` over two fields and renders an accessible `<table>`. The table has:

- row and column headers;
- a count or sum in each cell;
- row, column and grand totals;
- in count mode, links to `/mrql` filtered to the cell, wherever that link shows exactly that number.

## 2. Where it lives: a built-in Go handler in `shortcodes/`

A built-in handler fits better than a data-views plugin shortcode, for four reasons:

1. **Same executor as `[mrql]`.** It uses the same `QueryExecutor`, which already handles:
   - the per-page budget and render cache (`BudgetedExecutor`);
   - `ErrQueryUnavailable` in previews;
   - forced scope and `VisibleRootID` for confined principals.
2. **Full lint coverage.** Only built-ins get full `ValidateMRQL` linting; plugin queries are parse-only. A built-in can lint the composed query exactly as it will run.
3. **Render meter.** The table is written under the render meter (`tick` / `chargeOut` / `charged`), like `renderAggregatedTable` in `shortcodes/mrql_renderer.go`.
4. **Always available.** It works when data-views is disabled.

## 3. Syntax and attributes

```
[pivot query="type = resource" rows="contentType" cols="created.year"]
[pivot query="type = note AND meta.status IS NOT NULL" rows="meta.status" cols="owner" caption="Open work by owner"]
[pivot query="type = resource" rows="contentType" cols="created.year" value="sum" field="fileSize"]
```

The shortcode is inline only (`BlockNo`). A closing tag is a lint error.

| Attribute | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `query` | string | yes | – | MRQL **filter expression** naming exactly one entity type. It must not contain `SCOPE`, `GROUP BY`, `ORDER BY`, `LIMIT` or `OFFSET`. |
| `rows` | dimension field (§3.1) | yes | – | Row dimension. |
| `cols` | dimension field (§3.1) | yes | – | Column dimension. It must differ from `rows` after MRQL de-duplication (`group` and `groups` count as the same field). |
| `value` | enum `count`, `sum` | no | `count` | What each cell holds. AVG, MIN and MAX are left out because their totals cannot be built by adding cells. |
| `field` | `meta.<path>`, or `fileSize` / `width` / `height` | when `value="sum"` | – | The field to sum. With `value="count"` it is ignored and lint gives a warning. |
| `scope` | `entity`, `parent`, `root`, `global`, or a numeric group ID | no | `entity` | Same as `[mrql]`, resolved by `resolveScopeKeyword`. |
| `max-rows` | integer | no | `20` | Number of row values shown. Clamped to 1–50. An invalid value or one ≤ 0 means the default (`parseIntAttr`). |
| `max-cols` | integer | no | `10` | Number of column values shown. Clamped to 1–20, with the same fallback. |
| `rows-label` | string | no | derived | Corner header text. Also used in the caption and in link screen-reader text. |
| `cols-label` | string | no | derived | Used in the caption and in link screen-reader text. |
| `caption` | string | no | derived | `<caption>` text. It is also the accessible name of the scroll region. |

**Derived labels.** A derived label is the field name with any leading `meta.` removed: `meta.status` becomes `status`, and `created.year` stays `created.year`. A label or caption that is blank after trimming counts as absent, so headers and accessible names are never empty.

**Default caption:**

- count: `Count by {rows-label} and {cols-label}`
- sum: `Sum of {field label} by {rows-label} and {cols-label}`

`param-*` placeholders are not supported in v1. An author can use `[using]` or `[mrql]` when they need parameters.

### 3.1 Dimension fields

Every name, including `field`, is first checked against `^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$` before any composition. A `rows` or `cols` value is then accepted only in one of these forms. `entity` is the entity type of the parsed `query`.

| Form | Test | Kind |
|---|---|---|
| `meta.<seg>[.<seg>…]` | prefix `meta.` | meta |
| `<base>.<suffix>` | `suffix` ∈ {`day`, `week`, `month`, `year`} and `mrql.LookupField(entity, base)` is `FieldDateTime` | date bucket |
| single part `<name>` | `LookupField(entity, name)` succeeds, `Type != FieldDateTime`, and `name != "similarImages"` | scalar, or relation when `Type == FieldRelation` |

**Suffix set.** The suffix set is a local copy (`pivotDateBucketSuffixes`), because `mrql.dateBucketSuffixes` is unexported. A unit test pins the local copy against the suffixes `mrql.Validate` accepts.

**Rejections:**

- Any other dotted name (`owner.name`, `tags.name`, `tags.count`, …) is rejected with `[pivot] rows: "owner.name" is not supported; use a scalar field, owner, groups, tags, a meta.* path, or a date bucket like created.year`.
- A single-part datetime is rejected with `[pivot] rows: "created" is a timestamp; use a date bucket like created.month or created.year`.
- Both messages use the attribute's own name (`rows` or `cols`).

**Multi-valued relations.** A relation is multi-valued when its `Column` is one of `tags`, `groups`, `notes`, `resources` or `children`. `owner_id` and `parent_id` are single-valued.

**`field` rules.** `field` must start with `meta.`, or be one of `fileSize`, `width`, `height` and pass `LookupField(entity, field)` with `FieldNumber`. Anything else is rejected with `[pivot] field must be a meta.* path or fileSize, width or height`.

## 4. Query composition: exactly one query

`pivotQuery(attrs map[string]string) (composed string, plan pivotPlan, err error)` lives in `shortcodes/pivot_handler.go`. It is pure, with no database access, and both the handler and the linter use it. It runs these steps in order; the first failure returns its error:

1. **Required attributes.** `query`, `rows` and `cols` must be present, else `[pivot] requires query, rows and cols`. Then `value` must be `count` or `sum`, else `[pivot] value must be count or sum`. With `sum`, `field` must be set, else `[pivot] value="sum" requires field`.
2. **Filter shape.** Parse the filter with `mrql.Parse(query)`; a parse error gets the `[pivot] ` prefix. Reject the query if `Scope != nil`, `GroupBy != nil`, `len(OrderBy) > 0`, `Limit >= 0` or `Offset >= 0`, with `[pivot] query must be a filter expression; use the scope attribute and leave GROUP BY/ORDER BY/LIMIT/OFFSET/SCOPE out`.
3. **Entity type.** `mrql.ExtractEntityType(parsed)` must name a type, else `[pivot] query must name one entity type (type = resource, note or group)`.
4. **Classify fields.** Classify `rows`, `cols` and `field` as in §3.1.
5. **Compose.** Build `"(" + query + ") GROUP BY " + rows + ", " + cols + " COUNT()" + [" SUM(" + field + ")"] + " ORDER BY count DESC, " + rows + " ASC, " + cols + " ASC"`.
6. **Validate the composed query.** Run `mrql.Parse` and `mrql.Validate` on it; an error gets the `[pivot] ` prefix. If fewer than two GROUP BY fields remain after de-duplication, return `[pivot] rows and cols must be different fields`. This is the only place that check exists. It runs before `Validate`, so `group`/`groups` gives exactly this one message.
7. **Build the plan.**
   - The row and column keys are `mrql.AggregatedColumns(composedParsed)[0]` and `[1]`.
   - The value key is `count` or `sum_<field>`.
   - The plan also records each dimension's kind and multi-valued flag, the entity type and the original `query` text.

The handler then makes one executor call:

```go
executor(reqCtx, composed, QueryOptions{Limit: pivotFetchLimit + 1, ScopeGroupID: resolveScopeKeyword(attrs["scope"], ctx)})
```

`pivotFetchLimit` is `5000`, so the call asks for 5001 rows. The result must have `Mode == "aggregated"`; any other mode renders the error box `[pivot] unexpected result mode`.

## 5. Processing the result

### 5.1 Truncation

If `len(Rows) > 5000`, the handler keeps only the first 5000 rows and sets `truncated`.

### 5.2 Bucket keys

Each raw key value `v` becomes text as follows:

| Raw value | Result |
|---|---|
| `nil` | the none-bucket, labelled `(none)` |
| `[]byte` | converted to a string |
| string | used as is |
| `int*` / `uint*` | `strconv.FormatInt` / `FormatUint` |
| `float64` | `FormatFloat(f, 'f', -1, 64)` |
| `bool` | `true` / `false` (defensive) |
| anything else | `fmt.Sprint(v)` (defensive; §3.1 rejects timestamps, so no current dimension yields this), and the bucket is unlinkable |

Merging:

- The merge key is `asciiLower(text)`, which folds only ASCII `A–Z`.
- Rows with the same merge key are merged: counts and sums are added. The label comes from the first row seen, which is the variant with the most items.
- `""` is labelled `(empty)`.
- On a meta dimension, the bucket records whether any `float64` fed into it. Only SQLite produces these.

The resulting merges are:

- "Open" and "open";
- SQLite `3` and `"3"` (Postgres `->>` already returns both as text);
- two owners with the same name;
- booleans: on SQLite `true` gets label `1` (merged with `1` and `"1"`); on Postgres it gets label `true` (merged with `"true"`).

### 5.3 Cell values and totals

**Cell values:**

- count: `toFloat(row["count"])`;
- sum: `toFloat(row["sum_<field>"])`. `toFloat` from `conditional_handler.go` handles Postgres numerics that arrive as strings.
- nil or unconvertible values count as 0.

**Totals:**

- Totals are summed in `float64` over every fetched row, including rows hidden by the caps.
- If `rows` is multi-valued, the `<tfoot>` row of column totals and the grand total are left out.
- If `cols` is multi-valued, the Total column and the grand total are left out.

### 5.4 Caps and number format

**Caps:**

- Rows are ranked by row total, descending, with ties broken by label, ascending. The top `max-rows` are kept. Columns are ranked the same way and capped at `max-cols`. The hidden values are counted.
- A date-bucket dimension is displayed in ascending label order (chronological); other dimensions are displayed in rank order.

**Number format:**

- Integral values use `strconv.FormatFloat(f, 'f', -1, 64)`. Other values use `formatFloat(f, 64)` from `property_handler.go`.
- There are no thousands separators.
- A combination the query did not return shows as a muted `0`. Under truncation it shows as `—` instead.

## 6. Rendering and accessible markup

All text goes through `html.EscapeString`. Every write is charged with `m.tick()` / `chargeOut`, following the `charged` / `flush` pattern of `renderAggregatedTable`. If the meter stops rendering, the handler returns `""`.

```html
<div class="mrql-pivot">
  <div class="overflow-x-auto" role="region" aria-label="{caption}" tabindex="0">
    <table class="min-w-full text-sm font-mono border border-stone-200 rounded-md">
      <caption class="text-left text-sm font-semibold text-stone-700 px-1 pb-2">{caption}</caption>
      <thead class="bg-stone-100"><tr>
        <th scope="col" class="px-3 py-2 text-left font-semibold text-stone-700">{rows-label}</th>
        <th scope="col" class="px-3 py-2 text-right font-semibold text-stone-700">{col label}</th> …
        <th scope="col" class="px-3 py-2 text-right font-semibold text-stone-700">Total</th>   <!-- omitted when cols is multi-valued -->
      </tr></thead>
      <tbody class="divide-y divide-stone-100">
        <tr>
          <th scope="row" class="px-3 py-2 text-left font-semibold text-stone-700">{row label}</th>
          <td class="px-3 py-2 text-right tabular-nums"><a href="/mrql?q=…" class="text-amber-700 hover:text-amber-900 underline">5<span class="sr-only">: {rows-label} {row label}, {cols-label} {col label}</span></a></td>
          <td class="px-3 py-2 text-right tabular-nums text-stone-400">0</td> …
          <td class="px-3 py-2 text-right tabular-nums font-semibold"><a href="/mrql?q=…" class="…">12<span class="sr-only">: total for {rows-label} {row label}</span></a></td>
        </tr> …
      </tbody>
      <tfoot class="bg-stone-50"><tr>                                           <!-- omitted when rows is multi-valued -->
        <th scope="row" class="px-3 py-2 text-left font-semibold text-stone-700">Total</th>
        <td class="… font-semibold"><a …>7<span class="sr-only">: total for {cols-label} {col label}</span></a></td> …
        <td class="… font-semibold"><a …>30<span class="sr-only">: total of all</span></a></td>   <!-- omitted when either dimension is multi-valued -->
      </tr></tfoot>
    </table>
  </div>
  <p class="text-xs text-stone-500 mt-1">and 12 more {rows-label} values and 3 more {cols-label} values not shown; totals include them.</p>  <!-- only when something is hidden; each clause only when non-zero -->
  <p class="text-xs text-amber-800 mt-1">Only the 5000 combinations with the most items were counted, so cells marked — and totals marked + are incomplete. Narrow the query.</p>  <!-- only when truncated -->
</div>
```

A cell that is not linkable (§6.2) renders the same `<td>` with its number as plain text and no `<a>` or `sr-only` span. Its row and column headers already name it for screen readers.

**Under truncation:**

- Totals render as `30+<span class="sr-only"> or more</span>`.
- Missing combinations render as `—<span class="sr-only">not counted</span>` with `text-stone-400`.
- Nothing is linked.

### 6.1 Accessibility

- **Scroll region.** The scroll container has `role="region"`, an `aria-label` and `tabindex="0"` (axe `scrollable-region-focusable`). It needs no ids, so several pivots on one page are safe.
- **Corner cell.** The corner cell is `<th scope="col">`, because it heads the column of row headers. It always has text (§3), which satisfies axe `empty-table-header`.
- **Headers.** The table has one `<caption>`. Every column has `scope="col"` and every row, including Total, has `scope="row"` (axe `td-has-header`, `th-has-data-cells`).
- **Link names.** Every link carries screen-reader text naming its headers (WCAG 2.4.4).
- **Colour.** Colour is never the only signal: the muted `0` and the `—` are real text.
- **Focus.** No `outline-none` is added; the existing link focus styles apply.

### 6.2 Cell links (count mode only)

`pivotLinkable(plan, dim, bucket) bool` is true only when all of these hold:

- `value == "count"`, and the result is not truncated;
- the dimension is not a date bucket and not `shared`;
- the bucket is not the none-bucket and not a defensive `fmt.Sprint` bucket;
- on a meta dimension, no `float64` fed the bucket;
- for a string literal, the text contains at most one kind of quote and no backslash.

The literal used in the link depends on the dimension:

- **meta, `FieldString` and relation:** a string literal of the bucket label, written `"…"`, or `'…'` when the label contains `"`. `owner`, `tags` and `groups` match by name, ignoring case.
- **scalar `FieldNumber`** (`fileSize`, `width`, `height`, `category`, `noteType`, `id`): a bare literal.

| Cell | Link filter |
|---|---|
| Data cell (both buckets linkable) | `(<query>) AND <rows> = <lit> AND <cols> = <lit>` |
| Row total (row linkable, Total column shown) | `(<query>) AND <rows> = <lit>` |
| Column total (column linkable, footer shown) | `(<query>) AND <cols> = <lit>` |
| Grand total (shown, count mode, not truncated) | `(<query>)` |

**Scope.** When the resolved scope ID is not `0` and not `mrql.UnresolvedScopeSentinel`, the filter is wrapped by `mrql.InsertScopeClause(filter, scopeID)`, the same way `executeMRQLForShortcode` builds `linkQuery`.

**URL.** The URL is `/mrql?q=` + `url.QueryEscape(filter)`.

**Documented limitations.** A link's count can differ from its cell in these cases only:

1. Postgres `LOWER` folds non-ASCII case, so "É" and "é" are two cells whose links overlap.
2. Two multi-valued related entities with the same name are merged. An entity linked to both is counted twice in the cell but once by the link.
3. On SQLite, imported meta stored as JSON `3.0` groups with `3`, but `LOWER()` renders it as `3.0`.

**Documented display difference.** Booleans in meta show as `1`/`0` on SQLite and as `true`/`false` on Postgres.

## 7. Empty, missing and invalid input

| Situation | Output |
|---|---|
| Missing or blank `query`, `rows` or `cols`; bad `value`; `sum` without `field`; bad `field`; bad identifier; traversal, timestamp or unsupported dimension; filter-rule violation; unknown field; no entity type; same field twice | Error box `mrqlErrorHTML(err, false)` with the §3.1 / §4 message. The linter reports the same error once. |
| Executor error (SQL, timeout, budget) | `mrqlErrorHTML(err, false)` |
| `errors.Is(err, ErrQueryUnavailable)` | `unavailableComment(err)` |
| No executor wired (share pages) | `<!-- mr:pivot unavailable in this context -->` via `shortcodeComment` |
| Zero rows | `<div class="mrql-pivot"><p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p></div>` |
| `max-rows` / `max-cols` invalid or ≤ 0 | The default. A value above the ceiling is clamped. |
| Block body | Ignored at render. Lint error (`BlockNo`). |

## 8. Scoping and authorization

`[pivot]` adds no data access of its own. Its single query goes through `QueryExecutor` → `executeMRQLForShortcode` → `ExecuteMRQLGroupedWithScope`, which applies two existing controls:

- **Forced scope.** `effectiveMRQLRequestedScope` applies the forced scope. A confined principal asking for `scope="global"` gets its forced root. A group outside its subtree becomes `UnresolvedScopeSentinel`, which matches nothing, so the path fails closed.
- **Visible joins.** `VisibleRootID` applies visible-join rules. The group-by joins on `owner`, `groups`, `parent`, `children`, `notes` and `resources` use `visibleTable` / `visibleRelated`, so no label can name an entity outside the subtree.

Additional points:

- Tags are global by design.
- Traversal dimensions are rejected, so there is no new join path to audit.
- A filter-only `query` cannot contain `SCOPE`.
- Cell links open `/mrql`, which re-applies the viewer's own forced scope.

## 9. Performance at millions of resources

- **One aggregate statement per `[pivot]` cache miss.** The `GROUP BY … ORDER BY count DESC, r, c LIMIT 5001` runs under the existing MRQL timeout. 5001 is below `MaxMRQLInteractiveLimit` (10,000). A confined viewer with a concrete scope also needs a scope check.
- **Budget and caching.** It costs one unit of the per-page budget (`-mrql-page-query-budget`). Identical pivots on one page are cache hits. There are no per-cell or count queries.
- **Bounded memory and output.** Memory holds at most 5001 rows and buckets. Output is at most 52 × 22 cells, under the render meter.
- **Author guidance in the docs.**
  - `meta.*` keys cannot use an ordinary index, so authors should narrow `query` by type, owner, note type or date.
  - Pivots belong on detail pages or `CustomListHeader`, not in per-card `CustomSummary`.
  - The docs link to the existing junction-join warning in `mrql-reference.md` for two multi-valued dimensions.
- **Link queries** run only when a user clicks one.

## 10. Files to change

1. **`shortcodes/pivot_handler.go` (new).** Contains:
   - constants: `pivotFetchLimit = 5000`, `pivotDefaultMaxRows = 20`, `pivotMaxRowsCeiling = 50`, `pivotDefaultMaxCols = 10`, `pivotMaxColsCeiling = 20`;
   - `pivotFieldRe`, `pivotDateBucketSuffixes`, `pivotSumScalars` = {fileSize, width, height};
   - `pivotPlan`, `pivotQuery`, `pivotLinkable`;
   - `RenderPivotShortcode(reqCtx, sc, ctx, renderer, executor, depth)`;
   - helpers for merging (`asciiLower`), link literals and table writing.
2. **`shortcodes/processor.go`.** Add `case sc.Name == "pivot":`. It calls the handler when `executor != nil && depth < maxRecursionDepth`; otherwise it returns `shortcodeComment("pivot unavailable in this context")`. This mirrors the `mrql` case. Also update the doc comment that lists built-ins.
3. **`shortcodes/builtin_docs.go`.** Add `BuiltinDoc{Name: "pivot", IsBlock: BlockNo, …}` with the §3 attributes. Enums: `value` = `count, sum`; `scope` = `entity, parent, root, global`. Use the three examples from §3.
4. **`shortcodes/lint.go`.**
   - Add `"pivot"` to `builtinBaseNames`.
   - Structural `case "pivot"`: call `pivotQuery(tk.attrs)`. Report an error once at the `query` attribute offset (`attrOffsetW`), or at the whole token when `query` is missing. Warn when `field` is set without `value="sum"`.
   - In the `ValidateMRQL` switch, add `case "pivot"`. Only if `pivotQuery` succeeded, call `check(composed, MRQLCheckOptions{Limit: pivotFetchLimit + 1}, start, end, "query")`. Its messages start with `MRQL error in query: `.
5. **Reference panels.** Add one `[pivot …]` line to each of `templates/createCategory.tpl`, `templates/createResourceCategory.tpl` and `templates/createNoteType.tpl`.
6. **`docs-site/docs/features/shortcodes.md`.**
   - Add a row to the built-ins table.
   - Add a `## [pivot] -- Cross-tab` section covering: attributes, dimension forms (no raw timestamps), the `field` rule, query rules, totals with multi-valued dimensions, caps, truncation ("combinations with the most items"), links (count mode only, case/number merging, the boolean label difference, the three limitations), scope and budget.
   - Add the unavailable comment under "Failure markers".
7. **`docs-site/docs/features/custom-templates.md`.** Add `[pivot]` to the list of server-expanded shortcodes.
8. **`docs-site/docs/features/note-sharing.md`.** Name `[pivot]` in the "No queries" bullet.
9. **`.claude/skills/mahresources-category-designer/references/shortcodes.md`.** Add a short entry.
10. **`docs/plans/2026-11-24-pivot-shortcode.md`.** Save this plan there, then run `./docs/plans/generate-index.sh`.

The parser, `mrql`, the executor and the application context do not change.

## 11. Tests

### 11.1 Unit tests: `shortcodes/pivot_handler_test.go`

These use a capturing mock executor that returns `Mode: "aggregated"`.

1. **Composition.**
   - The exact composed text for count and for sum, including `ORDER BY count DESC, meta.status ASC, owner ASC`.
   - `Limit == 5001`.
   - `ScopeGroupID` for `entity`, `parent`, `root`, `global` and a numeric scope.
2. **Rejections.** Each renders an error box and never calls the executor:
   - a `query` with GROUP BY, ORDER BY, LIMIT, OFFSET or SCOPE;
   - `rows="meta.x) OR (1"`;
   - `owner.name`, `tags.name`, `tags.count`, `similarImages`;
   - `rows="created"` and `cols="startDate"`, with the timestamp message;
   - `group` with `groups`, which gives exactly one message;
   - a query with no entity type;
   - `value="avg"`;
   - sum without `field`;
   - `field="name"`, `field="id"` and `field="noteType"`;
   - `field="fileSize"` on a note query.

   Also pin `pivotDateBucketSuffixes` against the suffixes `mrql.Validate` accepts.
3. **Table shape** on a 3 × 2 fixture:
   - a single caption;
   - the corner `<th scope="col">` reads `status`;
   - two value columns plus Total;
   - three `scope="row"` headers plus the Total row;
   - cells, muted `0` and totals are correct;
   - `<script>` in a key and in `caption` is escaped;
   - a blank `rows-label` falls back to the derived label;
   - the default sum caption is `Sum of hours by status and assignee`.
4. **Caps.**
   - 25 rows with `max-rows="5"` show 5 row headers and `and 20 more status values`.
   - Totals include hidden values.
   - Ranking and tie-break order are correct.
   - `created.year` is displayed in ascending order.
5. **Merging.**
   - `"Open"`(3) and `"open"`(2) give one `Open` bucket of 5.
   - `int64(3)` and `"3"` give one bucket.
   - Two `owner` rows named `Inbox` give one row.
   - `name` keys `"Foo"` and `"foo"` give one row.
   - `[]byte("x")` behaves like `"x"`.
6. **Multi-valued dimensions.**
   - `rows="tags"`: no `<tfoot>` and no grand total.
   - `cols="groups"`: no Total column and no grand total.
7. **Truncation.**
   - 5001 rows: the warning appears, the table has no `<a `, totals show `N+` with ` or more`, and missing cells show `—` with `not counted`.
   - 5000 rows: none of these appear.
8. **Links (count mode).**
   - A meta string key gives an escaped `(<query>) AND meta.status = "open" AND …`.
   - Meta `int64(1)` gives `meta.x = "1"`.
   - A meta bucket fed by a `float64` has no link.
   - `fileSize` gives a bare literal.
   - None-bucket, date-bucket and `shared` keys have no link. Neither do labels containing both quote kinds or a backslash.
   - A label containing `"` uses `'…'`.
   - A numeric scope inserts the clause from `mrql.InsertScopeClause`; `global` adds no `SCOPE`.
   - Each total link carries its screen-reader text.
9. **Sum mode has no links.** `value="sum"` renders the right numbers with no `<a` anywhere.
10. **Postgres-shaped values.** Counts and sums delivered as strings (`"3"`, `"12.5"`) aggregate the same as numbers.
11. **Cache and budget.**
    - Put a counting executor behind `BudgetedExecutor` with `WithQueryBudget(ctx, 1)` and render two identical pivots through `Process`: the executor is called once and both pivots render.
    - A third, different pivot renders the `inline query budget exceeded` error box.
12. **Error paths.**
    - An executor error renders `mrql-error`.
    - `ErrQueryUnavailable` renders the unavailable comment.
    - Zero rows render `No results.`
    - A non-aggregated mode renders the error box.
    - A nil executor renders `<!-- mr:pivot unavailable in this context -->`.
    - An exhausted render meter gives `""`.

### 11.2 Lint and docs tests

- **`shortcodes/lint_test.go`.**
  - Each structural error is reported exactly once, at the `query` offset, prefixed `[pivot] `.
  - The `field`-without-sum warning is reported.
  - `ValidateMRQL` receives the composed query with `Limit: 5001`.
  - `ValidateMRQL` is not called when composition fails.
  - `[pivot][/pivot]` is reported as a block error.
- **`shortcodes/builtin_docs_consistency_test.go`.** Add `TestPivotBuiltinExamplesLintClean`. For `{FTS: true}` and `{Postgres: true, FTS: true}`, lint every example of the `pivot` doc with `Known: KnownFromBuiltins()` and `ValidateMRQL` → `application_context.CheckTemplateMRQL`. Fail on **any** issue. Also assert the validator was called once per example.

The existing consistency tests stay red until the docs, panels and examples are added. This is intended.

### 11.3 Integration tests: `server/api_tests/pivot_shortcode_test.go`

These follow `using_shortcode_scope_test.go` (`setupAuthEnv`, `buildScopingFixture`, `previewDescription`).

1. **`TestPivotShortcode_ScopedPrincipalCannotSeeOutside`.** A confined guest renders `rows="owner"` with `scope="global"` and with a numeric scope outside its subtree.
   - No outside entity is counted, and no outside group name appears.
   - The admin control render shows the outside rows.
   - A deny-all principal gets `No results.`
2. **`TestPivotShortcode_LinkCountsMatchCells`** (count mode).
   - Seed `meta.status` values `"open"`, `"Open"`, number `3`, string `"3"`, `true`, string `"true"` and a missing key. Pivot with `rows="owner"` (two owners share a name) and `cols="noteType"`.
   - For every link, run its `q` through the MRQL API as the same principal and assert that the item count equals the cell.
   - Assert the merged buckets:
     - one `open` row and one `3` bucket on both databases;
     - on SQLite, a `1` bucket containing `true`, with `"true"` as a separate `true` bucket;
     - on Postgres, one `true` bucket containing both.
   - Run on SQLite and under `--tags 'json1 fts5 postgres'`.
3. **`TestPivotShortcode_OneStatement`.** The query log shows exactly one aggregated statement per pivot.

### 11.4 E2E tests

1. **`e2e/tests/NNN-pivot-shortcode.spec.ts`.** `NNN` is the next free prefix at implementation time (`108` today).
   - Create a note type whose `CustomHeader` holds a count `[pivot]`, seed notes through the API, and open a note of that type.
   - Assert headers, cells and totals.
   - Click a cell and check that `/mrql` shows the same count.
2. **`e2e/tests/accessibility/pivot-shortcode-a11y.spec.ts`.**
   - Run axe (`a11y.fixture.ts`), including at a narrow viewport.
   - Tab reaches the scroll region and then the first link.
   - The link's accessible name includes the screen-reader text.

## 12. Success criteria

1. **One aggregate statement per table.** The §3 examples render on a real detail page as one `<table>` with a caption, headers, correct cells and correct totals. Each cache miss runs one aggregate statement, plus the concrete-scope check for a confined viewer.
2. **Exact links.** In a non-truncated **count** pivot, for every linked cell and total, the `/mrql` count equals the cell number on SQLite and on Postgres. The seeded data differs in case, number versus string, and boolean. Sum pivots render no links.
3. **Scoping fails closed.** A subtree-confined guest sees no count and no label from outside its subtree, whatever `scope` is set to.
4. **Accessibility.** axe reports no violations, and the region and links are keyboard-reachable and have names.
5. **Linter coverage.** Every invalid input in §7 is reported exactly once at edit time, and every documented example lints clean on both databases.
6. **Test suites pass.**
   - `go test --tags 'json1 fts5' ./...`
   - the Postgres run of `./server/api_tests/... ./mrql/...`
   - `cd e2e && npm run test:with-server:all`
   - `npm run test:with-server:a11y`
   - `./scripts/css-scan-test.sh`, which checks that `sr-only` and `tabular-nums` survive the Tailwind scan

---

## Last evaluator feedback

I checked the plan's code claims and most of them hold. These are correct as stated: mrql.AggregatedColumns (mrql/params.go), ExtractEntityType, buildAggregateOrderKeys (it accepts group-by field names and aliases), buildGroupByAliasMap (it resolves group/groups for ORDER BY), InsertScopeClause, UnresolvedScopeSentinel, the unexported dateBucketSuffixes, Limit/Offset defaulting to -1, resolveScopeKeyword, parseIntAttr (<=0 falls back to the default), toFloat, formatFloat, mrqlErrorHTML, unavailableComment, shortcodeComment, renderAggregatedTable with its charged/flush pattern, builtinBaseNames, attrOffsetW, the lint `check` closure that prefixes "MRQL error in <label>: ", BudgetedExecutor/WithQueryBudget, the CheckTemplateMRQL-based example test pattern, e2e 107 being the highest prefix, and LOWER(...) = LOWER(?) for string and meta equality. Nothing like a cross-tab exists in docs/ideas or plugins. I would not approve it as-is, mainly because of point 1.

1. **Most important (usefulness): `noteType` and `category` render as raw numeric IDs.** These fields are FieldNumber (note_type_id, resource_category_id). groupByFieldExprs selects the bare column, so the headers would read "3" and "7". The motivating example "notes by owner and note type" and the integration test's `cols="noteType"` produce a table that cannot be read without an ID lookup. Pick one fix and write it into the plan:
   - (a) resolve the IDs to names with one bounded lookup per render. This needs a scoped API, and the plan must say how that stays inside the executor's auth path.
   - (b) reject or warn on these dimensions and point authors to a meta field.
   - (c) at minimum, document that the labels are IDs and drop noteType from the motivating examples and tests.

2. **Owner/group merging by name undoes the translator's design.** groupByRelationJoins deliberately groups owner and groups by ID and selects MAX(name), so that distinct entities with the same name are not merged. The plan merges them back by name so its links stay exact. That is defensible, but say so explicitly as a trade-off. Also consider making two same-named owners unlinkable instead of merging them, which keeps both the entity distinction and correct link counts.

3. **Too complex for v1.** §5.2 has the bool, []byte, float64 and fmt.Sprint branches. §6.2 has five link-eligibility rules plus three documented limitations and a per-database boolean label difference. That is a lot of surface for a v1. Consider shipping without cell links, or with links only for scalar FieldString/relation dimensions. This removes most of the per-database meta edge cases and the SQLite float/`3.0` caveat, and the total links still carry the main value.

4. **Missing edge-case specs:**
   - `scope` set to a non-numeric, unknown keyword silently falls back to ctx.ScopeGroupID in resolveScopeKeyword. Should lint warn, as for `[mrql]`?
   - The none-bucket for `owner` on resources or notes with no owner (LEFT JOIN null), and for `parent` on root groups. The plan says none-buckets are unlinkable but never shows how "(none)" renders in a header, or whether it sorts last.
   - Which MRQL error the lint and handler emit when `rows="shared"` is used on a non-note entity.
   - Whether `field` set to a `meta.*` path is valid with SUM on Postgres for text JSON. Cite aggregateExpr's cast behaviour.

5. **Totals with multi-valued `cols` only.** Per-row totals are still shown, but a row's items spread over several tag columns are counted once per tag. The plan omits only the Total column, which is correct, but the rule "omit Total column when cols is multi-valued" should also be stated from the row-total side, to keep the logic unambiguous.

6. **Smaller points:**
   - docs/plans/README conventions use `-design.md` / `-impl.md` or a bare slug. `2026-11-24-pivot-shortcode.md` is acceptable, but confirm that generate-index.sh lists it under Active.
   - The plan says `ORDER BY count DESC, <rows> ASC, <cols> ASC` will be "pinned" in a unit test. Also add a Validate-level test that the ORDER BY on `group` resolves through the alias map on both dialects.
   - The a11y markup is good. Consider `<th scope="row">` text for the none-bucket and empty-bucket so screen readers announce "(none)" rather than punctuation.

---

## Review

### Checklist

- [x] §10.1 `shortcodes/pivot_handler.go`: `pivotQuery`, the handler, bucket merging, caps, table writer, `lintPivot`.
- [x] §10.2 `shortcodes/processor.go` dispatch (comment on share pages). Also `shortcodes/parser.go`: `pivot` added to both token regexes, which the plan missed; without it `[pivot …]` stays literal text.
- [x] §10.3 `shortcodes/builtin_docs.go` entry with the three §3 examples.
- [x] §10.4 `shortcodes/lint.go`: `builtinBaseNames`, the structural case (`lintPivot`), and the composed-query check in the `ValidateMRQL` switch.
- [x] §10.5 Reference panels in `createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`.
- [x] §10.6–8 `docs-site` shortcodes page (table row and `## [pivot] -- Cross-tab` section), `custom-templates.md`, `note-sharing.md`.
- [x] §10.9 Category designer skill reference.
- [x] §10.10 Plan saved here (`docs/plans/2026-11-26-shortcode-pivot.md`, per the lane brief rather than the `2026-11-24` name). `generate-index.sh` and `docs/ideas/shortcodes/README.md` are left to the merge, as instructed.
- [x] §11.1 `shortcodes/pivot_handler_test.go` (composition, rejections, suffix pin, shape, escaping, caps, merging, multi-valued totals, truncation, links, literals, sum mode, Postgres-shaped values, budget and cache, error paths, render meter).
- [x] §11.2 `shortcodes/pivot_lint_test.go` and `shortcodes/pivot_docs_test.go` (`TestPivotBuiltinExamplesLintClean`, plus relation dimensions building on both static dialects). In a new file rather than `builtin_docs_consistency_test.go`, to keep the shared file out of the merge.
- [x] §11.3 `server/api_tests/pivot_shortcode_test.go` and `pivot_shortcode_pg_test.go`: scope, link counts, sum semantics, one aggregate statement, on SQLite and PostgreSQL; a preview-route scope check on SQLite.
- [x] §11.4 `e2e/tests/111-pivot-shortcode.spec.ts` (111, not 108: 108–110 are taken by sibling lanes) and `e2e/tests/accessibility/pivot-shortcode-a11y.spec.ts`.
- [x] §12 criteria 1–5 shown by the tests above; 6 by the validation listed below.

### Deviations from the plan

1. **ID dimensions are refused** (`id`, `category`, `noteType`): `[pivot] rows: "noteType" holds IDs, which would head the table as bare numbers; pivot by a meta.* field instead, or filter by noteType in query`. Evaluator point 1, option (b). The motivating "notes by owner and note type" example became "notes by status and owner", and the integration test uses `owner` and `group` instead of `noteType`. Name resolution (option a) would need a new scoped resolver threaded through every render surface; left for a follow-up.
2. **String literals are always double-quoted and escaped** (`\"`, `\\`), which the MRQL lexer reads back exactly. The plan's "at most one quote kind, no backslash, else `'…'`" rules are gone; such labels are now linkable.
3. **Linkability lives in `pivotBucket.literal`** ("" means unlinkable) instead of a separate `pivotLinkable` function. The rules are the plan's minus the quote rule, plus one (deviation 4).
4. **A multi-valued relation bucket that merged two raw rows in one cell loses its links** (evaluator point 2). Two raw rows in one cell are distinct values that compare equal; on a multi-valued relation they can be two same-named entities on one item, which the cell counts twice and a link once. This removes the plan's limitation 2. Single-valued `owner`/`parent` keep merging by name and keep their links, which are exact (`owner = "Inbox"` matches both groups). The table still shows same-named groups as one row: the aggregated result carries only the name (`MAX(name)`), so the entities cannot be told apart. That trade-off is stated in the docs.
5. **`shared` is labelled `yes`/`no` and linked** as `shared = true|false` (the plan left it unlinked with raw `1`/`0` labels). Exact, since the group-by expression is `share_token IS NOT NULL`.
6. **`$parameters` in `query` are refused** (`[pivot] query cannot use $parameters`), as `[calendar]` does; the plan only said param-* was unsupported.
7. **`[pivot]` inside a `[reload]` face is refused** by the renderer and the linter, as `[calendar]`/`[similar]` are (a table is not phrasing content).
8. **Duplicate-field check** compares canonical columns in `pivotQuery` before composition, instead of counting GROUP BY fields after `Validate` (dedup happens inside `Validate`, so the plan's order could not work). Same single message.
9. **Missing `query`/`rows`/`cols` in lint** is left to the generic required-attribute check, so it is reported exactly once; `lintPivot` returns early.
10. **Unknown `scope` keyword warns in lint** (evaluator point 4). The evaluator's premise that `[mrql]` warns is not right: no shortcode lints `scope` today. `[pivot]` now does: `[pivot] scope must be entity, parent, root, global or a group ID; "x" counts in the entity's own group`.
11. **Links carry `aria-label` instead of an `sr-only` span**, per the lesson "Tailwind's `.sr-only` is `position: absolute`, so it escapes the scroll container". The truncation markers (`—` "not counted", `+` "or more") still use `sr-only`, so the region is `relative`, which makes it their containing block; the a11y spec injects such a span at 390 px and asserts the page does not widen (it fails without `relative`: 899 > 390). Decorative `—` and `+` are `aria-hidden`.
12. **Muted zero is `text-stone-500`**, not `stone-400` (2.5:1 fails WCAG AA; axe would flag it).
13. **Numbers use `calcFormat(v, -1, "number")`** (as `[calc]`: at most ten fraction digits, trailing zeros dropped) rather than `FormatFloat`/`formatFloat`, so a float sum like 0.1 + 0.2 does not print 0.30000000000000004.
14. **The bool branch of §5.2 is gone** (evaluator point 3): no dimension yields a Go `bool` (SQLite returns 0/1, Postgres `->>` text); such a value falls into the defensive opaque branch, which is unlinkable.
15. **The unavailable comment** is documented in the `[pivot]` section, as `[calendar]` does, not in the shared "Failure markers" paragraph (keeps a line other lanes edit out of the merge).
16. **`RenderPivotShortcode(reqCtx, sc, ctx, executor)`** takes no renderer or depth (unused), matching `RenderCalendarShortcode`; the processor needs no depth check (`processWithDepth` already stops at the cap).
17. **The `BuiltinDoc` type comment that lists every built-in was not updated**, to keep that shared line out of the five-lane merge; the parent may add `pivot` when resolving.

### Evaluator points

1. *noteType/category render as IDs* — resolved with option (b): refused with a pointer to meta fields or filtering in `query`; examples and tests changed (deviation 1).
2. *Owner/group merging by name* — kept for single-valued relations (links exact, trade-off documented); multi-valued buckets that merge in a cell become unlinkable (deviation 4). Making same-named owners separate is not possible from the result, which carries only the name.
3. *Too complex for v1* — reduced: the quote-kind rules and their limitation are gone (deviation 2), the bool branch is gone (14), limitation 2 is gone (4), and limitation 3 (SQLite `3.0`) is covered by the updated numeric-axis rule below. One documented mismatch remains: PostgreSQL's `LOWER` folds non-ASCII case. Links were kept for meta dimensions: status-by-owner is the main use case, and the integration test proves every link count equals its cell on both databases.
4. *Edge cases* —
   - unknown `scope` keyword: lint warning (deviation 10);
   - none-bucket: rendered as `(none)` in a `<th scope="row">` (or column header), always shown last, ranked like any value for the caps, never linked; an empty string is `(empty)`. Both are words a screen reader announces;
   - `rows="shared"` on a resource: `[pivot] rows: "shared" is not a resource field`, the same in lint and render (unit tests);
   - `field="meta.*"` with SUM on Postgres: `aggregateExpr` wraps meta in `pgMetaNumericExpr` (`CASE WHEN text ~ '^-?[0-9]+(\.[0-9]+)?$' THEN ::numeric ELSE NULL END`), so non-numeric text is skipped. SQLite's `SUM(json_extract(…))` reads the numeric prefix of a string and counts JSON `true` as 1. Pinned by `checkPivotSumSemantics` on both databases and documented.
5. *Totals with multi-valued `cols`* — stated from the row side in the docs and the description: each row's items spread over several columns, so a row total would overcount; the Total column and grand total are omitted. Tested.
6. *Smaller points* — the plan lives under `docs/plans/` with the lane's name and the parent regenerates the index. The alias check: `TestPivotRelationDimensionsBuildOnBothDialects` builds `rows="group"` and other relation dimensions through `CheckTemplateMRQL` on both static dialects, and `checkPivotLinkCountsMatchCells` runs `cols="group"` for real on SQLite and PostgreSQL (`ORDER BY "group" ASC` resolves through the alias map). The none/empty header text is a word, see 4.

### Round 1 integration review follow-up

- Drill-downs stay plain text for a confined viewer whose pivot uses a strict descendant of their allowed group, because `/mrql` would use the full subtree. The linked result count is also bounded by the list page's effective default limit, including its interactive ceiling. Tests cover sibling groups and links at vs. above a configured limit of two.
- SQLite disables links that depend on a Meta axis with any numeric result key, even when that key is not displayed. INTEGER `3` can mask REAL `3.0` in the same SQL bucket; a separate TEXT `"3.0"` bucket's drill-down can overlap too. Per-axis SQL REAL-presence flags were rejected in favor of the conservative numeric-key check. Pure-text axes and independent totals still link. SQLite integration tests pin both representations.
- Lint and render reject SQL aliases over 63 bytes; a real PostgreSQL test exercises a 63-byte dimension and sum alias, then refuses each at 64 bytes. Long free-text fields (`description`, `originalName`, `originalLocation`, `url`) cannot be dimensions. The composed link, including its scope, must fit MRQL's 32 KiB query limit. Authors should group by short Meta labels, not free-form Meta text; the combination limit is not a byte cap on aggregate rows.
- Truncated signed sums label totals `(partial)`, not `+` or "or more"; the regression test excludes a negative contribution. `float64` sum precision is documented rather than changed.
- SQLite and PostgreSQL tests show that an out-of-subtree association on a visible note cannot change a confined guest's joined labels or counts. The aggregate statement test now also observes the concrete scope check for a confined viewer; identical pivots share both via the cache and consume one budget unit.
- Hover cards retain the table without links or a scroll-region tab stop. Transparent captions and notices inherit the surface text color; table sections pair their foregrounds and backgrounds. A real hover-card render and a lightbox axe contrast check pin both changes.

Round 1 validation: the shortcode, internal, template-handler and focused MRQL application suites passed, as did the SQLite and PostgreSQL pivot API tests, the CSS scan, and all 7 pivot browser tests. Removing the SQL junction-visibility check via a Go overlay made both joined-label tests fail. Vet reported only the existing copylocks finding below.

### Round 2 integration review follow-up

- Links are suppressed when their final filter contains a carriage return, since the MRQL editor would rewrite it as a line feed. Totals that do not depend on the affected filter still link. `TestPivotLinkQueryRejectsCarriageReturns` covers CR and CRLF keys, a CRLF in the base filter, and an LF-only control.
- Hover cards now get the unavailable comment without running the pivot query, instead of a table that may require horizontal scrolling in a tooltip that keyboard users cannot enter. `TestPivotShortcode_UnavailableInTooltip` checks both eager and deferred hover-card bodies and keeps the detail page as a control.
- Built-in help, the shortcode reference and the category-designer skill distinguish the display caps (highest total count or sum) from the SQL cutoff (most items per combination, even in sum mode). `TestPivotSumCapsRankBySumNotItemCount` checks both display caps.

Round 2 validation: the carriage-return and hover-card regressions failed before the fixes and passed afterward. The shortcode, internal and template-handler suites, focused SQLite and PostgreSQL pivot API tests, CSS scan and all 7 pivot browser tests passed. Vet reported only the pre-existing `application_context/job_download_progress_test.go:99` copylocks finding.

### Known limits

- The 5000-combination cutoff does not bound the bytes in a Meta dimension key. As with any MRQL `GROUP BY` over Meta, the executor can load large keys before display caps are applied. Bounding projected key bytes belongs in the executor; use short Meta labels as dimensions in the meantime.
- Distinct relations with the same displayed name can still tie at the SQL cutoff. A tie can change which amount contributes to a truncated sum. Truncated tables are already marked partial and have no links.

### Not verified / risks

- The full E2E suite was not run (shared machine, per the brief); the pivot specs and the nearest neighbours (shortcode, calendar, adjacent, calc, lint, autocomplete, template preview, category authoring a11y) were.
- Truncation (5001 combinations) is covered by unit tests only, not on a real database.
- On PostgreSQL, non-ASCII case variants are separate rows whose links overlap (documented).
- `guid` and `hash` are accepted as dimensions; they are legal but rarely useful (one row per entity, bounded by the caps).
- `go vet --tags 'json1 fts5 postgres'` reports a pre-existing copylocks issue in `resource_reduction_review_fixes_pg_test.go`, and `go vet --tags 'json1 fts5' ./...` one in `application_context/job_download_progress_test.go`; neither file is touched by this branch.
