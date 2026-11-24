<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 3 plan/evaluate round(s).
> Last evaluation: consistency 7.6/10, usefulness 7.8/10, average 7.70/10. The bar was 8.5.
>
> **Pitch:** Renders the current group's typed GroupRelations, both outgoing and incoming, as an accessible list of linked groups grouped by relation type: [relations type="Depends on" direction="out|in|both" limit="20"]. Each entry shows the relation type, the target group's name and category, and an optional block body per relation exposing [item path="Name"] and [item path="Description"]. Today a template author can't reach the GroupRelation model at all; this lets project, contact and knowledge-graph categories show their dependency or relationship edges. It uses the existing unique_rel index with a hard limit, and when a subtree scope is set, targets outside it are dropped rather than shown.

# Plan: `[relations]`, a built-in shortcode that lists a group's typed relations (rev. 4)

## 0. Changes since rev. 3 (one line per evaluator point)
1. **Usefulness, and the per-page budget on list pages.** The design keeps one shared unit per expansion, and §6.1 now explains why. The page budget is 200 by default (`main.go`, `-mrql-page-query-budget`) and a list page shows at most 50 cards (`constants.MaxResultsPerPage`), so a CustomSummary with `[relations]` costs 50 units and leaves 150 for `[mrql]`. An expansion costs the same as one entity-scoped `[mrql]` per card and is cheaper to run. Starving later `[mrql]` calls is exactly what a shared budget exists to do, and the outcome is visible: a marker, not a silent drop. A test pins the interaction (U14), and the docs give the arithmetic. §1 also states the narrower value plainly: cards plus type filtering.
2. **Deduplication with a `type` filter.** This is now specified. Deduplication only compares edges inside the filtered result. With `type="Depends on" direction="both"`, an incoming "Depends on" whose mirror half is an outgoing "Required by" is shown as "Incoming: Depends on". That is correct, because the author asked for edges whose own type is "Depends on", and the mirror is a different type. The rule is documented and tested (A-dedup-3).
3. **Deduplication across the 500 scan cutoff.** This is now documented next to the bound (§6.4 and the docs). When the outgoing scan of a scoped principal stops at 500 and a mirror's outgoing half lies past the cutoff, the pair can show up as an incoming entry. That is only possible when one group has more than 500 outgoing edges.
4. **Typed builder and compile-time assertion.**
   - `BuildRelationLister` now takes `RelationListerContext`, the same shape as `BuildPartialResolver(PartialResolverContext)`.
   - The assertion `_ RelationListerContext = (*application_context.MahresourcesContext)(nil)` is added to the `var (...)` block in `shortcode_interfaces.go`.
   - The interface is also embedded in `PageRenderContext`, so the page path can no longer quietly lose the lister.
   - The two API paths reach it through `api_handlers` interfaces that already list `MRQLPageQueryBudget()` (`handler_interfaces.go`, lines 106/142). `ListGroupRelationsForTemplate` is added there too, so a signature drift fails to compile on every path.
5. **Go-level page-path test.**
   - `template_filters/shortcode_relation_lister_test.go` checks that `buildPageRenderContext` installs a lister that calls through, and installs nil for a nil appCtx.
   - `server/api_tests/relations_shortcode_scope_test.go` now also GETs the HTML page `/group?id=<sf-root>` as the scoped bearer, in the style of `job_detail_page_test.go`, and asserts that `sf-outside` is absent.
6. **Minor points.**
   - The claim that the new index speeds up the BackRelations preload is dropped.
   - The ordering is now stated exactly. Each direction's query orders by `rt.name, group_relations.id`. In `both`, the outgoing result set comes first and the incoming one follows. The two are concatenated, never interleaved. Rendering groups consecutive `(Direction, Type)` runs. Same-named types are adjacent because of the sort, so each type gets exactly one `<li>` per direction.
   - The plan file is named after the actual landing date.

## 1. Motivation and user value
`GroupRelation` rows are typed, directed edges between groups, for example "Backend API **Depends on** Auth Service". No shortcode can read them today:
- `[property]` reads only scalar fields.
- MRQL has no relation-edge field (`mrql/fields.go`).
- `[using]` and `[mrql]` go through MRQL.

`displayGroup.tpl` already shows "Relations" and "Reverse Relations" panels on the group detail page, so the value is narrower than "show relations at all". It is:
- **CustomSummary / CustomMRQLResult cards** (the main value). A project list or an MRQL result can show "depends on X, Y" on each card. This is impossible today.
- **Filtering by type.** A card can show only "Depends on", which the built-in panel cannot do.
- **Custom placement** in CustomSidebar or CustomHeader, next to the rest of a category's curated layout, for example a contact's "Works at" line.

A category that moves relations into its own slot hides the built-in panel through the existing SectionConfig: `"relations": {"state": "off"}`, or `forwardRelations`/`reverseRelations` set to `false` (`models/section_config.go`). Nothing has to be built for that, and the docs say so. The roadmap (`docs/ideas/category-templates-and-shortcodes.md`) does not cover group relations.

## 2. Syntax and attributes
```
[relations]
[relations type="Depends on" direction="out" limit="10"]
[relations direction="in"]
  <a href="[item path='GroupURL']">[item path="GroupName"]</a>: [item path="Name" default="(unnamed)"]
[else]
  <p>No dependents.</p>
[/relations]
```
It can be written inline or as a block (`IsBlock: BlockOptional`).

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `type` | string | "" (all types) | Exact, case-sensitive match on `GroupRelationType.Name` after trimming surrounding whitespace. Types that share a name (the unique key is name + from_category + to_category) all match. |
| `direction` | enum `out` \| `in` \| `both` | `both` | `out` means the carrier is `from_group_id`. `in` means the carrier is `to_group_id`. `both` means the outgoing result set followed by the incoming result set, with mirror pairs removed. |
| `limit` | integer | `20` | Maximum entries in total. Values above 100 are capped at 100. Non-integer, `0` and negative values fall back to 20 (`parseIntAttr` semantics, as in `[mrql]` and `[each]`). |

No other attributes are added. There is no `scope=` attribute; scoping comes from the principal (§5).

### Mirror deduplication (`both` only)
`AddRelation` writes a real mirror edge whenever a type has `BackRelationId` (`relation_context.go`, around lines 129–135). The rule:
- An incoming edge `B → carrier` of type T is dropped when the **fetched, filtered** outgoing set contains `carrier → B` of type U, and either `T.BackRelationId == U.ID` or `U.BackRelationId == T.ID`.
- Only the outgoing half is kept, because it already reads from the carrier's point of view.
- Incoming edges of types with no back relation are always kept. That is the data `out` alone would hide.
- Two genuine edges between the same pair, with unrelated types, are both kept.
- **With `type` set.** Deduplication only sees edges that passed the type filter. If the mirror half has a different type name, the incoming edge stays and is labelled "Incoming: <its type>". That is intended, because the filter selects edges by their own type. Two mirror types that share a name (a self-paired type) still deduplicate.

### `[item]` fields inside a block body
Each element is a `map[string]any`, rendered through the existing `[each]` machinery (`markItems` → `processWithDepth` → `spliceLazy` → `renderItemValueObserved`). So `path`, `format`, `layout`, `default`, `index` and `raw` behave exactly as they do in `[each]`.

| Path | Value |
|---|---|
| `Name` | `GroupRelation.Name` (free text, may be empty) |
| `Description` | `GroupRelation.Description` |
| `Type` | `GroupRelationType.Name` |
| `Direction` | `"out"` or `"in"` |
| `GroupID` | the far group's ID, stored as `float64`. `formatJSONScalarObserved` expects JSON-decoded scalars (comment at `each_handler.go`, lines 247–251), so it renders as "12". |
| `GroupName` | the far group's Name |
| `GroupCategory` | the far group's Category Name, or "" when it has none |
| `GroupURL` | `groupURL(id)` from `link_handler.go`, which gives `/group?id=N` |

### Carrier context
The body and `[else]` are processed with the **carrier's** context, as in `[each]`. So `[property]`, `[meta]` and `[mrql]` inside the body read the carrier.
- The far group's Meta and its other fields cannot be reached.
- `[using type="group" id="[item path='GroupID']"]` does not work, because shortcode attributes are not expanded. The docs say so.
- Values are spliced in after processing, so a relation named `[property path="Name"]` renders as escaped literal text and never runs (U9).

## 3. Rendered output (accessible markup)
```html
<div class="relations-shortcode" data-direction="both">
  <ul class="relations-shortcode__types">
    <li class="relations-shortcode__type">
      <span class="relations-shortcode__label">Depends on</span>
      <ul class="relations-shortcode__list">
        <li class="relations-shortcode__entry"><a href="/group?id=12">Auth Service</a> <span class="relations-shortcode__category">(Service)</span></li>
      </ul>
    </li>
    <li class="relations-shortcode__type">
      <span class="relations-shortcode__label">Incoming: Blocks</span>
      <ul class="relations-shortcode__list">…</ul>
    </li>
  </ul>
</div>
```
- **Grouping.** One type `<li>` per consecutive `(Direction, Type)` run in lister order: all outgoing runs, then all incoming runs.
- **Native list semantics only.** A screen reader announces the label and then "list, N items".
- **No IDs and no `aria-labelledby`, on purpose.** Many cards on a list page would otherwise create duplicate IDs. The nesting already ties each label to its list.
- **No headings.** This avoids breaking the heading order of the host slot; the a11y suite fails on skipped levels.
- **Direction is shown as text.** Incoming labels carry the visible prefix "Incoming: ", so direction never depends on styling. Outgoing labels are the bare type name.
- **Category span.** It is omitted when the far group has no category.
- **Escaping.** All text goes through `html.EscapeString`.
- **Classes.** Only BEM hooks are used, for CustomCSS. There are no new utility classes, which `./scripts/css-scan-test.sh` confirms.
- **Block form.** The wrapper, the type `<li>` and the label are the same. Each entry `<li>` contains the processed body, so list semantics survive whatever the author writes.

## 4. Empty, missing and invalid input
The checks run in this order.

| Situation | Output |
|---|---|
| `ctx.EntityType != "group"` or `EntityID == 0` | `shortcodeComment("relations: only groups have relations")`. The lister is not called. |
| `direction` is not in the enum | `shortcodeErrorMarker("relations", "direction must be \"out\", \"in\" or \"both\"")`. The lister is not called. |
| No lister, or a nil lister | `shortcodeComment("relations unavailable in this context")` |
| `if b := QueryBudgetFrom(reqCtx); b != nil && !b.Allow()` | `shortcodeErrorMarker("relations", "inline query budget exceeded; refine templates or raise -mrql-page-query-budget")`. The lister is not called. |
| No budget on the context | The lister is called without a charge, the same nil guard as `BudgetedExecutor` (`query_budget.go`, around line 148). |
| Lister error, including timeout or cancellation | `shortcodeErrorMarker("relations", "could not load relations")`. The raw error never reaches the HTML. |
| Render meter dead (`!meterFrom(reqCtx).alive()`) | `""` |
| Invalid `limit` | Silently becomes 20. The linter warns. |
| No visible edges, or `type` matches nothing | Inline: `""` (no empty `<ul>`). Block: the `[else]` body with carrier context, or `""` when there is no `[else]`. |

`MarkExceeded` is not called, so the executor's one-per-page "budget exceeded" log still fires on the next `[mrql]` miss.

## 5. Scoping and authorization (fails closed)
"Subtree scope" means the **principal's group-subtree confinement**. It does not mean `[mrql scope=]`, which is a filter and not authorization.

1. **No ORM help.** `group_relations` is not in `scopeColumn`, so no ORM callback confines it (`relation_context.go`, lines 18–30). The new method filters explicitly.
2. **Authoritative gate: `visibleGroupIDs`.** It reads the already-materialised allow-list and issues no SQL.
   - First the carrier is checked with `visibleGroupIDs([]uint{groupID})`. If it is not visible, the method returns `[]` and runs no edge query.
   - After the edge queries, one call `visibleGroupIDs(farIDs)` runs. Any row whose far endpoint is not visible is dropped.
   - Both endpoints must be visible, the same rule as `relationInScope`/`scopeRelationQuery`.
   - It returns an empty set for a scoped principal with no installed filter and for an empty allow-list. Both fail closed.
3. **Secondary check (deletion race only).** The far groups are loaded with `ctx.db.WithContext(reqCtx).Preload("Category").Where("id IN ?", visibleFarIDs).Find(&groups)`.
   - The list holds at most 500 IDs per direction, bounded by the carrier's degree and not by the subtree.
   - A visible ID with no returned row is dropped.
   - The load also passes through the scoped `groups` callback. An error returns the error and the generic marker renders.
   - Both checks stay in the code, and `visibleGroupIDs` is the gate.
4. **Categories are not scoped, and need not be.** A visible group's category name is already shown to the same principal on that group's page.
5. **Why not a plugin.** Plugin Lua runs on the unscoped handle and is refused for confined principals (`shortcode_tag.go`, around line 95).
6. **Model preview.** `template_run_tools.go` installs a nil lister. The reason is **privacy, not authorization**: the request handle is already scoped. That preview builds a model-facing corpus with its own masking of user data, and edge names, descriptions and far-group names would bypass the masking.
7. **Share server.** It is not wired. It renders notes, which hit the non-group branch anyway.

## 6. Performance (millions of rows)
### 6.1 Budget
Each expansion charges one unit of the shared per-page budget, when a budget is present.
- **Why one shared unit is acceptable.**
  - The default budget is 200 and a list page shows at most 50 cards, so a summary with `[relations]` spends 50 and leaves 150 for `[mrql]`.
  - The cost is the same per card as one entity-scoped `[mrql]` in that summary, and the statements are cheaper (§6.2).
  - A page that then runs out shows the standard budget marker on the refused shortcode. Nothing is dropped silently.
  - A separate budget would let a template double its per-page statement count past the operator's cap, which defeats the flag.
- **Docs.** The docs state the arithmetic (cards × shortcodes per card ≤ budget) and point to `-mrql-page-query-budget`.
- **Test.** U14 pins the interaction.

### 6.2 Edge query (once per requested direction)
```sql
SELECT group_relations.* FROM group_relations
JOIN group_relation_types rt ON rt.id = group_relations.relation_type_id
WHERE group_relations.<near_col> = ? [AND rt.name = ?]
ORDER BY rt.name, group_relations.id
LIMIT <fetch>
```
After the edge queries, three follow-ups run once over the union of both directions:
1. relation types: `Find(&types, typeIDs)`, which also gives `BackRelationId`;
2. far groups;
3. categories (preload).

In total that is at most **4 statements for `out`/`in` and 5 for `both`**. The result sets are concatenated with outgoing first, never interleaved.

### 6.3 Indexes
- `out` uses the leading column of `unique_rel (from_group_id, to_group_id, relation_type_id)`.
- `in` has no usable index today. Add the following to `models.EnsureSupplementalIndexes` (`models/supplemental_indexes.go`):
  `CREATE INDEX IF NOT EXISTS idx__group_relations__to_group_id ON group_relations(to_group_id)`
  The edge table is small next to resources, so a synchronous build fits that function's other entries.

### 6.4 Fetch size, pipeline and bounds
**Fetch size.**
- Unscoped principal: `fetch = limit` (at most 100).
- Scoped principal: `fetch = templateRelationsScopedScan = 500` per direction, so a mostly-hidden edge set can still fill `limit`.

**Pipeline.** Scope filter, then mirror deduplication (`both` only), then truncation to `limit` (outgoing first).
- **Unscoped.** Incoming rows survive truncation only when the outgoing count is below `limit`, which means every outgoing row was fetched. So deduplication is complete.

**Known bounds.** Both are documented and only reachable when one group has more than 500 edges in a single direction after the type filter. That is far outside normal relation use.
- **(a) Short list.** A scoped principal can get a shorter list when most of the first 500 rows are hidden.
- **(b) Mirror shown as incoming.** In `both`, when a mirror's outgoing half lies past the 500 outgoing cutoff and its incoming half survives, the pair appears once as an incoming entry.

**No "N more hidden" signal, on purpose.** It would tell a confined user that edges exist outside their subtree.

**Why the scope is not pushed into SQL.** Doing that needs an `IN (<subtree>)` list, which `group_crud_context.go` (lines 320–335) rejects.

### 6.5 Cost
- The sort covers only the carrier's own edges, so its cost depends on that group's degree, not on table size.
- A block body runs once per entry (at most 100). A nested `[mrql]` is budgeted per call.
- All queries use `ctx.db.WithContext(reqCtx)`, so `MRQLQueryTimeout` applies on the API paths.

## 7. Where it lives and why
It is a **built-in Go handler** in `shortcodes/`. Data comes through a request-context callback that mirrors `PartialResolver`.
- Plugin shortcodes are disabled for confined users and run on the unscoped DB.
- MRQL cannot express edges.
- `shortcodes/` stays free of DB imports. `application_context` implements the query.

## 8. Files to change
### New files
1. **`shortcodes/relations_handler.go`**
   - `type GroupRelationEntry struct { Direction, Type, Name, Description, GroupName, GroupCategory string; GroupID uint }`
   - `type RelationsRequest struct { GroupID uint; Type, Direction string; Limit int }`
   - `type RelationLister func(context.Context, RelationsRequest) ([]GroupRelationEntry, error)`
   - `WithRelationLister` / `relationListerFrom`, following the `WithPartialResolver` pattern. A nil lister means unavailable.
   - consts `defaultRelationsLimit = 20` and `maxRelationsLimit = 100`, plus the direction enum.
   - `RenderRelationsShortcode(reqCtx, sc, ctx, renderer, executor, depth)` runs the §4 checks in order, then renders §3.
   - For a block, it calls `splitElseM`, then `markItems` once per branch, and `processWithDepth` + `spliceLazy` once per entry.
2. **`application_context/relation_template_context.go`**
   - `func (ctx *MahresourcesContext) ListGroupRelationsForTemplate(reqCtx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error)`
   - It implements §5 and §6.
   - It re-validates direction (returning an error) and clamps `Limit`.
3. **`server/template_handlers/template_filters/shortcode_relation_lister.go`**
   - `type RelationListerContext interface { ListGroupRelationsForTemplate(context.Context, shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) }`
   - `func BuildRelationLister(c RelationListerContext) shortcodes.RelationLister` returns nil for a nil interface or a typed-nil pointer (the same reflect check as `pageRenderContext`), and otherwise returns `c.ListGroupRelationsForTemplate`.

### Edited files
4. **`shortcode_interfaces.go`**
   - Add `_ RelationListerContext = (*application_context.MahresourcesContext)(nil)` to the `var (...)` block.
   - Embed `RelationListerContext` in `PageRenderContext`.
   - Add the method to the existing test fakes that implement `PageRenderContext` (for example `countingPageContext` in `meta_scope_memo_test.go`), returning `nil, nil`.
5. **`shortcode_tag.go` (`buildPageRenderContext`)**
   - Add `var relations RelationListerContext; if appCtx != nil { relations = appCtx }`, with the same nil-interface guard as `partials`.
   - Then `reqCtx = shortcodes.WithRelationLister(reqCtx, BuildRelationLister(relations))`.
   - The page `_appContext` is the request-scoped copy (`routes.go`, around lines 560/580).
6. **`server/api_handlers/handler_interfaces.go`**: add `ListGroupRelationsForTemplate(...)` to the interfaces at lines 106 and 142 that `buildMRQLAPIRenderContext` and its callers use. A signature drift then breaks the build on the API paths too.
7. **`server/api_handlers/mrql_api_handlers.go` (`buildMRQLAPIRenderContext`)**: add the `WithRelationLister` line. It covers:
   - `/mrql` rendering;
   - previews;
   - the deferred `[lazy]`/`[details]`/`[reload]` endpoint;
   - the entity picker.
8. **`server/routes.go` (`processShortcodesForJSON`, around line 433)**: add the same line for the lightbox JSON path.
9. **`server/api_handlers/template_run_tools.go`**: after `buildMRQLAPIRenderContext`, add `renderCtx = shortcodes.WithRelationLister(renderCtx, nil)`, with a privacy-corpus comment.
10. **`shortcodes/processor.go`**: add a `case sc.Name == "relations":` dispatch.
11. **`shortcodes/builtin_docs.go`**
    - Add an entry after `item`:
      - `Name: "relations"`, the §2 syntax, `BlockOptional`;
      - three attrs with defaults, and `Enum: []string{"out","in","both"}`;
      - examples: "All relations", "One type, outgoing only", "Custom row with edge label and [else]".
    - Change the `item` description to "inside an [each] or [relations] block".
    - Update the `BuiltinDoc` comment list.
12. **`shortcodes/lint.go`**
    - Add `"relations"` to `builtinBaseNames`, which gives a near-miss suggestion for `[relation]`.
    - Add `relationsSpans := namedBlockSpans(tokens, "relations")`.
    - `case "item"` accepts spans from `eachSpans` or `relationsSpans`. The new message is "[item] is only meaningful inside an [each] or [relations] block".
    - The `[using]` check uses the later of the innermost `[each]` start and the innermost `[relations]` start.
    - Do not add `relations` to `otherContext`, because its body reads the carrier.
    - Add `case "relations"`:
      - a bad `direction` is a SeverityError, with the render-marker text;
      - a `limit` that is non-numeric or ≤ 0 is a SeverityWarning ("falls back to 20");
      - a `limit` > 100 is a SeverityWarning ("capped at 100").
13. **`models/supplemental_indexes.go`**: add the `to_group_id` index.

## 9. Tests
### Unit tests: `shortcodes/relations_handler_test.go`
These use a fake lister that records requests.
- **U1.** Nested-list markup; `href="/group?id=N"`; the category span is present, and omitted when the category is empty.
- **U2.** Grouping by consecutive (direction, type) runs; outgoing runs come before incoming ones; the "Incoming: " prefix appears only on `in` entries.
- **U3.** `<script>` and `"` are escaped in every text field, in both inline and block form.
- **U4.** Attribute pass-through:
  - `type` is trimmed;
  - `direction` defaults to `both`;
  - `limit`: "" → 20, "abc"/"0"/"-3" → 20, "250" → 100, "7" → 7.
- **U5.** An invalid direction gives the marker and 0 lister calls.
- **U6.** A resource, note or category carrier, or `EntityID == 0`, gives the comment and 0 calls.
- **U7.** A missing lister and an explicit nil lister both give "unavailable".
- **U8.** Empty results: `""` inline; the `[else]` body with carrier context; `""` in block form without `[else]`.
- **U9.** Block body:
  - `Name`, `GroupName`, `GroupURL`, `Direction`, `GroupID` ("12") and `index` render per entry;
  - `[property path="Name"]` renders the carrier;
  - a relation named `[property path="Name"]` renders literally.
- **U10.** A lister error gives the generic marker; the error text is absent from the HTML.
- **U11.** Budget:
  - with a budget of 1, two expansions make one call, and the second renders the budget marker;
  - a budget of 0 always calls;
  - with no budget on the context at all, the lister is called and nothing panics.
- **U12.** A dead meter returns `""`.
- **U13.** A `processor_test.go` dispatch case.
- **U14. Budget shared with `[mrql]`.** Set `WithQueryBudget(ctx, 3)` and use `BudgetedExecutor` over a counting fake executor. Render `[relations]` followed by three distinct `[mrql]` queries. Expect one lister call, two executor calls, and the budget marker on the third `[mrql]`. This proves both shortcodes draw on one counter.

### Lint tests: `shortcodes/lint_test.go`
- `[relations][item path="Name"][/relations]` produces no item warning.
- `[item]` outside both block kinds gives the new message; update the existing expectation around line 114.
- `[using]` inside `[relations]` with `[item]` gives the using warning.
- `direction="sideways"` gives an error.
- `limit="0"` and `limit="500"` give warnings.
- An unknown attribute gives the standard warning.
- `[relation]` gives a near-miss suggestion.
- `builtin_docs_consistency_test.go` and `TestBuiltinDocMRQLExamplesParse` pass.

### Template-filter tests: `server/template_handlers/template_filters/shortcode_relation_lister_test.go`
- `BuildRelationLister(nil)` returns nil, and a typed-nil `*MahresourcesContext` returns nil.
- `buildPageRenderContext` with a fake `PageRenderContext` installs a lister that forwards the request (GroupID, Type, Direction, Limit) to the fake.
- `buildPageRenderContext(ctx, nil)` makes the `[relations]` render show "unavailable", not panic.

### Application-context tests: `application_context/relation_template_context_test.go`
These use the `relationScopeFixture` style.
- **Unscoped:**
  - `out`, `in` and `both` return the correct far group, category, type, name and description;
  - the `type` filter matches two same-named types from different categories;
  - order is type name, then ID, within each direction, and `both` is outgoing then incoming;
  - `limit` truncates, with outgoing first.
- **Deduplication:**
  - **A-dedup-1.** A type with a back relation, through `AddRelation`: `both` lists the neighbour once, as the outgoing edge.
  - **A-dedup-2.** An incoming edge of a type with no back relation is kept. Two unrelated genuine edges between the same pair are both kept.
  - **A-dedup-3.** With `type="Depends on" direction="both"`, where the pair is "Depends on" (B → carrier) mirrored by "Required by" (carrier → B), exactly one entry appears: Direction "in", Type "Depends on".
- **Scoped (synthetic principal):**
  - the inside edge is listed;
  - the mixed edge is dropped in `out`, `in` and `both`;
  - a carrier outside the subtree returns `[]` and makes no edge query;
  - a scoped principal with no allow-list returns `[]`;
  - with 30 hidden edges sorting ahead of 2 visible ones, `limit=5` returns the 2;
  - a visible far group deleted between the queries is dropped.
- **Index.** `EnsureSupplementalIndexes` creates `idx__group_relations__to_group_id` (checked with `PRAGMA index_list`).
- **Postgres.** A `_pg_test.go` variant of the out/in/type/order and deduplication cases, using the existing Postgres harness (as in `relation_endpoint_lock_pg_test.go`).

### API tests: `server/api_tests/relations_shortcode_scope_test.go`
These follow `using_shortcode_scope_test.go` (`setupAuthEnv`, `buildScopingFixture`, `previewDescription`). Add edges from `sf-root` to `sf-child` and from `sf-root` to `sf-outside`.
- A scoped bearer previewing `[relations]` with `out`, `in` and `both` sees `sf-child` but not `sf-outside`. The admin control sees both.
- **Page path.** Give `sf-root`'s category `CustomSidebar: "[relations]"`, GET `/group?id=<sf-root>` as the scoped bearer, and assert:
  - a 200 status;
  - the `relations-shortcode` wrapper and `sf-child` inside it are present;
  - `sf-outside` is absent everywhere in the rendered HTML, with the built-in panels turned off through SectionConfig so they cannot mask the result.
- **Deferred path.** `[lazy][relations][/lazy]` through `/v1/shortcodes/deferred` as the scoped user does not leak `sf-outside`.
- **Lightbox JSON path.** `processShortcodesForJSON` as the scoped user does not leak `sf-outside`.
- **Template run tools.** They render "unavailable" and no group names (extend `template_run_tools_test.go`).

### E2E tests: `e2e/tests/shortcodes/relations-shortcode.spec.ts`
- **Setup** through `apiClient`:
  - a category with `CustomSidebar: '[relations]'`;
  - a block form with `[else]` in `CustomHeader`;
  - `SectionConfig {"relations":{"state":"off"}}`;
  - relation types (one with a back relation, one without) and edges.
- **Group page:**
  - the label, the link `href="/group?id=<id>"` and the category text are shown;
  - the back-related neighbour appears once;
  - the built-in Relations panel is absent;
  - the link navigates to the target.
- **Target page:** "Incoming: <type>" appears for the type without a back relation. A group with no edges shows the `[else]` text.
- **Group list:** a list page whose category has `CustomSummary: '[relations type="Depends on" limit="3"]'` shows the entry on each card.
- **A11y:** the axe check (`e2e/fixtures/a11y.fixture.ts`) reports zero violations on the detail and list pages.

## 10. Docs to update
- **`docs-site/docs/features/shortcodes.md`**
  - Add the table row `` | `[relations]` | Inline or block | List the group's typed relations to other groups | ``.
  - Add a section `` ## `[relations]` -- Group Relations `` covering:
    - attributes and `[item]` fields;
    - the carrier-context rule, and that the far group's Meta cannot be reached;
    - mirror deduplication, including the type-filter case;
    - the subtree rule (both endpoints visible, hidden edges omitted);
    - the 500-edge scan bound with its two consequences (§6.4 a/b);
    - the budget cost, with the list-page arithmetic;
    - hiding the built-in panel with SectionConfig;
    - examples.
  - Update the `[item]` row.
- **`docs-site/docs/features/custom-templates.md`**: add one overview line linking to the section.
- **`docs-site/docs/concepts/relationships.md`**: add a short "Showing relations in templates" paragraph.
- **Reference panels in `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl`**: add an entry to all three; the consistency test requires it. The resource and note panels say it renders only on groups.
- **`.claude/skills/mahresources-category-designer/references/shortcodes.md`**: add the shortcode, the SectionConfig pairing and the budget note.
- **Plan file.** Per AGENTS.md: save the plan as `docs/plans/<landing date YYYY-MM-DD>-relations-shortcode.md`, using the date of the commit that lands it. Run `./docs/plans/generate-index.sh`, tick items off as they land, and end with a review section.

## 11. Success criterion
1. On a group whose category template contains `[relations]`, an admin sees every outgoing and incoming typed edge, each mirror pair once (under the type-filter and 500-bound rules in §2/§6.4), grouped by type with outgoing first. Each entry is a working link with the far group's name and category, in nested-list markup that passes axe. The category can hide the built-in panel, and a CustomSummary can show filtered edges on list cards.
2. A group-confined user sees only edges whose both endpoints are inside their subtree, and a scoped principal with no allow-list sees none. Each of these paths is proven by a §9 test:
   - the HTML page;
   - the lightbox JSON;
   - the preview;
   - `/mrql`;
   - the deferred endpoint.
3. `MahresourcesContext` drifting from `RelationListerContext` fails compilation through the assertion and the embedded interfaces.
4. Each expansion costs at most one unit of the shared per-page budget (none and no panic without a budget, proven alongside `[mrql]` by U14). It runs at most 5 statements, each reading at most 500 rows (at most 100 unscoped). `direction="in"` uses `idx__group_relations__to_group_id`.
5. These all pass:
   - `go test --tags 'json1 fts5' ./...`
   - the Postgres tests
   - `./scripts/css-scan-test.sh`
   - the new E2E spec
   - `npm run test:with-server:a11y`

---

## Last evaluator feedback

1. Scoping (blocks approval): the "secondary check" in §5.3 loads far groups with `ctx.db.WithContext(reqCtx).Preload("Category").Where("id IN ?", visibleFarIDs)`. That goes through the scoped `groups` read callback, which appends the principal's whole allow-list as a second `IN` clause (`scoping.go`, around line 582: `db.Where("<table>.<col> IN ?", sf.allowed)`). The comments on `visibleGroupIDs` and `relationInScope` both reject this pattern: past SQLITE_MAX_VARIABLE_NUMBER or Postgres's 65535-parameter cap the query errors. A confined user with a large subtree would therefore always get the "could not load relations" marker. The claim "bounded by the carrier's degree and not by the subtree" is wrong for that statement. Fix: after the `visibleGroupIDs` gate, load names and categories through a handle that skips the scope callback (or use whatever unscoped read the export or relation code already uses after `visibleGroupIDs`). Handle the deletion race by dropping missing rows. Add a test with a scoped principal whose allow-list is larger than the SQLite variable limit.
2. Statement count on list pages: each card runs 4–5 statements, so 50 cards with `both` is about 250 statements but only 50 budget units. The budget caps expansions, not statements. Either say plainly that the per-page statement cap the operator set is exceeded about 5x, or batch across cards. "Cheaper than one `[mrql]`" is asserted, not measured. Consider charging per statement, or at least per direction.
3. Usefulness: `displayGroup.tpl` already renders forward and reverse relation panels, so the real gain is filtered relation lists on CustomSummary/CustomMRQLResult cards and custom placement. That is valuable but narrow. Group relations are a secondary feature in most deployments, and the far group's Meta cannot be reached (attributes are not expanded, so no `[using]` chaining). That limits how rich a card can be. Allowing a nested `[property]`/`[meta]` on the far group, or exposing a few far-group fields such as Description or a meta path through `[item]`, would raise the value considerably.
4. Complexity against value: mirror-dedup rules, the 500-scan bound with two documented anomalies, a new index, and about 13 edited files plus 6 test files is a lot for a list renderer. Consider dropping `both` dedup in favour of `out` as the default, which removes the §6.4(b) anomaly and the A-dedup-3 subtlety. Or justify why authors need `both` more than two explicit invocations.
5. Small checks I verified as correct: `parseIntAttr`, `groupURL`, `splitElseM`/`markItems`/`spliceLazy`, `visibleGroupIDs` failing closed, `buildMRQLAPIRenderContext` taking `mrqlRenderContext` (line 142), the concrete type of `processShortcodesForJSON`, the SectionConfig relations toggles, and the `unique_rel` leading column. Still to fix:
   - `handler_interfaces.go` line 106 is the large MRQL handler interface, not a render-context interface. Adding the method there is unnecessary; adding it to `mrqlRenderContext` alone is enough.
   - Spell out that `GroupRelationType.Name` uniqueness is (name, from_category, to_category). The plan says so, which is good, but the docs must explain that the type filter is a name match across categories.
6. Name the exact a11y check for "Incoming: " labels inside nested lists in card layouts (the label is a plain span, not tied to the list). Native nesting is acceptable, but confirm axe passes on the MRQL result page as well as the group list page.
