# Plan: `[relations]`, a built-in shortcode that lists a group's typed relations (rev. 5)

Slug: `relations`. Saved as `docs/plans/2026-11-26-shortcode-relations.md`.

## 0. Changes since rev. 4 (one line per evaluator point)

1. **Scoping of the far-group load (blocking).** Rev. 4 loaded far groups with a `Find` on the scoped handle, which appends the principal's whole allow-list as a second `IN` list and errors past SQLite's variable limit or Postgres's 65535 parameters. Rev. 5 never reads `groups` through the scope callback:
   - the edge statement reads `group_relations` and joins the type, the far group and its category; the scope callback keys on the statement's table, which is `group_relations`, so it appends nothing;
   - visibility is then decided in Go by `visibleGroupIDs` (the materialised allow-list, no SQL), carrier first and far endpoints after;
   - the optional far-group details statement (§2) reads `groups` through `unscopedDB()`, with only IDs that `visibleGroupIDs` already admitted, at most `limit` (≤ 100) of them.
   - Tests pin a scoped principal whose allow-list holds 40,000 IDs (SQLite) and 70,000 (Postgres), above both drivers' limits (§9, A-big).
2. **Statements versus budget units.** Rev. 4 ran 4–5 statements for one unit. Rev. 5 runs **one statement per expansion** (edges, types, far groups and categories in one join), plus one more only when a block body reads `GroupDescription` or `GroupMeta`, and **charges one budget unit per statement**. So the operator's `-mrql-page-query-budget` caps statements exactly; nothing is "cheaper than `[mrql]`" by assertion any more. A list page of 50 cards with an inline `[relations]` spends 50 units.
3. **Usefulness: the far group was unreachable.** A block body now also gets `GroupDescription` and `GroupMeta` (the far group's decoded Meta, so `[item path="GroupMeta.status"]` works). They are loaded only when an `[item]`/`[calc]`/`[inspect]` token of the body names them, in one extra statement over at most `limit` IDs, charged one extra unit. The far group is visible to the viewer by then (both endpoints are in their subtree), so its Meta is no more than its own page shows.
4. **Complexity against value: `both` and mirror deduplication are dropped.** `direction` is `out` (default) or `in`. Justification: `AddRelation` writes a real mirror edge for every type with a back relation, so `out` already lists both halves of a paired relation from this group's side; `in` is for types with no back relation, whose edges `out` cannot see. Two invocations cost the same two statements `both` would, and give the author two separately placed lists. This removes the mirror-dedup rules, the A-dedup tests and the §6.4(b) anomaly; only the scoped short-list bound (§6.4) remains.
5. **Small checks.**
   - `handler_interfaces.go` line 106 (the MRQL handler interface) is not touched; `mrqlRenderContext` embeds `template_filters.RelationListerContext`, which is all `buildMRQLAPIRenderContext` needs.
   - The docs state that the unique key of a relation type is (name, from category, to category), so `type=` is a name match across categories.
6. **A11y check named.** The functional markup is nested native lists; the a11y spec runs axe (`e2e/fixtures/a11y.fixture.ts`, WCAG 2.0/2.1 A and AA tags, which include the `list`, `listitem`, `link-name` and `color-contrast` rules) on the group detail page, the group list page (CustomSummary cards) and the `/mrql` result page (CustomMRQLResult cards), and asserts zero violations on each.

Other changes from rev. 4, decided while implementing and recorded in §13:
- The lookup goes through a page-level resolver in `template_filters` (cache, budget, timeout, log), the same shape as `[adjacent]`'s `BuildAdjacentResolver`, rather than a budget check inside the shortcode handler.
- `[relations]` refuses to render inside a `[reload]` button face, and inside a hover card (a tooltip the keyboard cannot enter) the inline form shows names without links.
- The entity picker installs no lister: picker cards render per keystroke, up to 50 at a time, and sit inside an option the keyboard selects, where links would nest interactive content.

## 1. Motivation and user value

`GroupRelation` rows are typed, directed edges between groups, for example "Backend API **Depends on** Auth Service". No shortcode can read them today: `[property]` reads only scalar fields, MRQL has no relation-edge field, and `[using]`/`[mrql]` go through MRQL.

`displayGroup.tpl` already shows "Relations" and "Reverse Relations" panels on the group detail page, so the value is narrower than "show relations at all". It is:
- **CustomSummary / CustomMRQLResult cards** (the main value). A project list or an MRQL result can show "depends on X, Y" on each card.
- **Filtering by type.** A card can show only "Depends on", which the built-in panel cannot do.
- **The far group's own data.** A block body can show the far group's Description or a Meta field, e.g. a dependency's `status`.
- **Custom placement** in CustomSidebar or CustomHeader, next to the rest of a category's curated layout.

A category that moves relations into its own slot hides the built-in panel through the existing SectionConfig: `"relations": {"state": "off"}`, or `forwardRelations`/`reverseRelations` set to `false`.

## 2. Syntax and attributes

```
[relations]
[relations type="Depends on" limit="10"]
[relations direction="in"]
  <a href="[item path='GroupURL']">[item path="GroupName"]</a>: [item path="GroupMeta.status" default="no status"]
[else]
  <p>No dependents.</p>
[/relations]
```
Inline or block (`BlockOptional`).

| Attribute | Default | Meaning |
|---|---|---|
| `type` | "" (all types) | Exact, case-sensitive match on `GroupRelationType.Name` after trimming surrounding whitespace. A type's unique key is (name, from category, to category), so every type of that name matches, whatever its categories. |
| `direction` | `out` | `out`: the carrier is `from_group_id`. `in`: the carrier is `to_group_id`. Anything else is an error marker. |
| `limit` | `20` | Maximum entries. Above 100 is capped at 100. Non-integer, `0` and negative fall back to 20 (`parseIntAttr`, as in `[mrql]` and `[each]`). |

### `[item]` fields inside a block body
Each element is a `map[string]any` rendered through the `[each]` machinery (`markItems` → `processWithDepth` → `spliceLazy` → `renderItemValueObserved`; `[calc]` and `[inspect]` as in `[each]`).

| Path | Value |
|---|---|
| `Name` | `GroupRelation.Name` (free text, may be empty) |
| `Description` | `GroupRelation.Description` |
| `Type` | `GroupRelationType.Name` |
| `Direction` | `"out"` or `"in"` |
| `GroupID` | the far group's ID, as `float64` so it renders "12" |
| `GroupName` | the far group's Name |
| `GroupCategory` | the far group's Category name, or "" |
| `GroupURL` | `/group?id=N` |
| `GroupDescription` | the far group's Description (loaded on demand) |
| `GroupMeta` | the far group's Meta, decoded (loaded on demand) |

`GroupDescription` and `GroupMeta` are loaded only when one of the body's element tokens (`[item]`, `[calc]`, `[inspect]`) mentions either name in an attribute; that costs one more statement and one more budget unit.

### Carrier context
The body and `[else]` are processed with the **carrier's** context, as in `[each]`: `[property]`, `[meta]` and `[mrql]` inside the body read the carrier. Attributes are not expanded, so `[using type="group" id="[item path='GroupID']"]` does not work; `GroupMeta` is the way to read the far group. Values are spliced in after processing, so a relation named `[property path="Name"]` renders as escaped literal text.

## 3. Rendered output (accessible markup)
```html
<div class="relations-shortcode" data-direction="out">
  <ul class="relations-shortcode__types">
    <li class="relations-shortcode__type">
      <span class="relations-shortcode__label">Depends on</span>
      <ul class="relations-shortcode__list">
        <li class="relations-shortcode__entry"><a href="/group?id=12">Auth Service</a> <span class="relations-shortcode__category">(Service)</span></li>
      </ul>
    </li>
  </ul>
</div>
```
- One type `<li>` per consecutive type run in lister order (the order is by type name, so each name gets one).
- Native list semantics only: a screen reader announces the label and then "list, N items".
- No IDs and no `aria-labelledby` (many cards would duplicate IDs); no headings (the host slot's heading order is not ours).
- Incoming labels carry the visible prefix "Incoming: ", so direction never depends on styling.
- The category span is omitted when the far group has none. A blank far-group name reads "Group N"; a blank type name reads "Relation".
- All text goes through `html.EscapeString`. Only BEM hooks are emitted (no new utility classes).
- Block form: the same wrapper, type `<li>` and label; each entry `<li>` contains the processed body.
- Inside a hover card (a tooltip the keyboard cannot enter) the inline form renders the far group's name as text, not a link.

## 4. Empty, missing and invalid input (checked in this order)

| Situation | Output |
|---|---|
| No lister on the context (share pages, entity picker, generation-run tools) | `shortcodeComment("relations unavailable in this context")` |
| Inside a `[reload]` button face | error marker: "[relations] cannot be used inside a [reload] button face" |
| `direction` not `out`/`in` | error marker `direction must be "out" or "in"`; no lookup |
| `ctx.EntityType != "group"` or `EntityID == 0` | `shortcodeComment("relations: only groups have relations")`; no lookup |
| Budget refused | error marker with the standard budget text naming the limit; the first refusal per page is logged once |
| The surface's handle carries no ORM scope for a confined principal (the MRQL render API binds `WithMRQLPrincipal`) | `shortcodeComment("relations unavailable in this context")`; no query |
| Timeout | error marker "lookup timed out" |
| Other lister error | error marker "could not load relations"; the error is logged, never shown |
| Render meter dead | `""` |
| No visible edges, or `type` matches nothing | inline: `""`; block: the `[else]` body with carrier context, or `""` |

## 5. Scoping and authorization (fails closed)

1. `group_relations` is not in `scopeColumn`, so no ORM callback confines it. The method filters explicitly.
2. **Unfiltered handle.** A confined principal whose handle carries no scope filter (a `WithMRQLPrincipal` binding) gets `ErrRelationsUnavailable` before any query.
3. **Carrier.** `visibleGroupIDs([]uint{groupID})`: not visible → empty result, no query.
4. **Edges.** One statement reads `group_relations` joined to the type, the far group and its category. It is never scoped by the callback (its table is `group_relations`), so it carries no allow-list. Then `visibleGroupIDs(farIDs)` drops every row whose far endpoint is outside the subtree. Both endpoints must be visible, the rule of `relationInScope`.
5. **Details.** `LoadRelatedGroupDetails(ids)` re-gates the IDs through `visibleGroupIDs` and reads `id, description, meta` through `unscopedDB()`. An ID with no row (deleted meanwhile) is simply absent.
6. Categories are not scoped and need not be: a visible group's category name is shown on its own page.
7. Generation-run tools install no lister (privacy, not authorization: they build a model-facing corpus with its own masking, which edge names and far-group data would bypass).
8. Share pages render notes only; no lister is installed there.

## 6. Performance (millions of rows)

### 6.1 Budget
One unit per statement: one for the edge statement, one more for the details statement. Identical lookups on one page (same group, type, direction, limit, details) are served from a page cache and cost nothing. A list page of 50 cards × one inline `[relations]` spends 50 of the default 200 units; the docs give that arithmetic and point at `-mrql-page-query-budget`.

### 6.2 Edge statement
```sql
SELECT group_relations.id, group_relations.name, group_relations.description,
       rt.name AS type_name, far.id AS group_id, far.name AS group_name, c.name AS category_name
FROM group_relations
JOIN group_relation_types rt ON rt.id = group_relations.relation_type_id
JOIN groups far ON far.id = group_relations.<far_col>
LEFT JOIN categories c ON c.id = far.category_id
WHERE group_relations.<near_col> = ? [AND rt.name = ?]
ORDER BY rt.name, group_relations.id
LIMIT <fetch>
```
The sort covers only the carrier's own edges, so its cost is the carrier's degree, not table size.

### 6.3 Indexes
- `out` reads `unique_rel (from_group_id, to_group_id, relation_type_id)` by its leading column.
- `in` had no index. Add `CREATE INDEX IF NOT EXISTS idx__group_relations__to_group_id ON group_relations(to_group_id)` to `models.EnsureSupplementalIndexes`, which builds its other junction indexes synchronously at startup the same way.

### 6.4 Fetch size and the one known bound
- Unscoped principal: `fetch = limit` (≤ 100).
- Scoped principal: `fetch = 500`, so a mostly-hidden edge set can still fill `limit`. A scoped principal can get a shorter list when one group has more than 500 edges in one direction after the type filter and most of the first 500 are hidden. There is no "N more hidden" signal, on purpose: it would tell a confined user that edges exist outside their subtree.
- Pushing the scope into SQL would need an `IN (<subtree>)` list, which is the parameter-limit failure §5 avoids.

### 6.5 Timeout
Every statement runs under `-mrql-query-timeout` (the resolver wraps each lookup, as `[adjacent]` does).

## 7. Where it lives
A built-in Go handler in `shortcodes/` with data through a request-context lister (the `PartialResolver`/`AdjacentResolver` pattern), so `shortcodes/` stays free of DB imports. Plugin shortcodes would run on the unscoped DB and are refused for confined users; MRQL cannot express edges.

## 8. Files

New:
1. `shortcodes/relations_handler.go`: `RelationsRequest`, `GroupRelationEntry`, `RelationLister`, `WithRelationLister`, `ErrRelationsBudget`, `ErrRelationsUnavailable`, `RenderRelationsShortcode`.
2. `application_context/relation_template_context.go`: `ListGroupRelationsForTemplate` and `LoadRelatedGroupDetails`.
3. `server/template_handlers/template_filters/shortcode_relation_lister.go`: `RelationListerContext`, `BuildRelationLister` (cache, budget per statement, timeout, log).

Edited:
4. `shortcode_interfaces.go`: compile-time assertion; `RelationListerContext` embedded in `PageRenderContext`.
5. `shortcode_tag.go` (`buildPageRenderContext`): install the lister.
6. `server/api_handlers/handler_interfaces.go`: `mrqlRenderContext` embeds `RelationListerContext`.
7. `server/api_handlers/mrql_api_handlers.go` (`buildMRQLAPIRenderContext`): install the lister (MRQL render API, list render, previews, deferred endpoint).
8. `server/api_handlers/entity_picker_handlers.go` and `template_run_tools.go`: install a nil lister, with the reason.
9. `server/routes.go` (`processShortcodesForJSON`): install the lister for the lightbox JSON.
10. `shortcodes/processor.go`: dispatch. `shortcodes/builtin_docs.go`: entry, and `[item]`/`[calc]` descriptions name `[relations]`. `shortcodes/lint.go`: near-miss, `[item]`/`[calc]` placement, `[using]` inside, direction/limit/reload checks.
11. `models/supplemental_indexes.go`: the `to_group_id` index.
12. Reference panels (`createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`), `public/index.css` (BEM styles), docs-site, category-designer skill.

## 9. Tests

Unit (`shortcodes/relations_handler_test.go`, fake lister):
- U1 markup, href, category span present/omitted, "Group N" and "Relation" fallbacks.
- U2 grouping by type runs; "Incoming: " only for `in`.
- U3 escaping of every text field, inline and block.
- U4 attribute pass-through: type trimmed; direction default `out`; limit "", "abc", "0", "-3" → 20, "250" → 100, "7" → 7.
- U5 invalid direction (including `both`): marker, no lister call.
- U6 resource/note/category carrier or `EntityID == 0`: comment, no call.
- U7 no lister and nil lister: "unavailable".
- U8 empty: inline `""`, block `[else]` with carrier context, block without `[else]` `""`.
- U9 block body fields per entry, `index`, carrier `[property]`, a relation named `[property path="Name"]` stays literal; `Details` is requested only when a token names `GroupMeta`/`GroupDescription`, and `GroupMeta.status` renders.
- U10 errors: generic marker without the error text; budget error shows the budget text; timeout; `ErrRelationsUnavailable` renders the comment.
- U11 dead meter `""`; reload face marker; tooltip renders names without links.
- U12 processor dispatch.

Lister (`template_filters/shortcode_relation_lister_test.go`):
- one unit per statement (1 inline, 2 with details), identical lookups cached, a refused lookup runs nothing, the exhaustion is logged once, each statement runs under the MRQL timeout;
- budget shared with `[mrql]`: `[relations]` then three `[mrql]` with a budget of 3 → one lister call, two executor calls, the marker on the third;
- `BuildRelationLister(nil)` and a typed-nil `*MahresourcesContext` return nil; `buildPageRenderContext` installs a lister that forwards the request; `buildPageRenderContext(ctx, nil)` renders "unavailable".

Lint (`shortcodes/relations_lint_test.go`): `[item]` inside `[relations]` is quiet and outside both blocks names both; `[calc]` with item inside `[relations]` is quiet; `[using]` inside warns; bad direction errors; `limit="0"`/`"500"` warn; unknown attribute warns; `[relation]` near-miss; inside `[reload]` errors.

Application context (`application_context/relation_template_context_test.go`, SQLite, with a `_pg_test.go` running the same cases on Postgres):
- out/in return far group, category, type, name, description; the type filter matches same-named types of different categories; order by type name then id; limit truncates.
- Scope (synthetic principal): inside edge listed; mixed edge dropped in out and in; carrier outside → empty and no query; deny-all principal → empty; a `WithMRQLPrincipal` guest → `ErrRelationsUnavailable` and no query; 30 hidden edges sorting ahead of 2 visible with `limit=5` → the 2.
- A-big: a scoped principal with a 40,000-ID allow-list (70,000 on Postgres) lists edges and loads details without error.
- Details: description and meta of visible IDs; an invisible ID is not loaded; a deleted ID is absent.
- One statement per call, counted with a query callback.
- Index: `EXPLAIN QUERY PLAN` of the `in` statement uses `idx__group_relations__to_group_id` (and the `models` supplemental-index test checks it is created).

HTTP (`server/api_tests/relations_shortcode_scope_test.go`): scoped bearer on the group page (built-in panels off), the deferred endpoint, the description preview and the lightbox JSON sees `sf-child`/inside edges and never the outside group, with an admin control on each; the MRQL render API renders for an admin and is "unavailable" for a scoped user; the entity picker and generation-run tools render "unavailable"; two-statement budget sharing on a real page.

E2E (`e2e/tests/112-relations-shortcode.spec.ts`): group page shows label, link, category; the link navigates; the built-in panel is hidden by SectionConfig; `direction="in"` shows "Incoming: <type>"; a group with no edges shows `[else]`; a block reads `GroupMeta`; list cards show filtered edges. A11y (`e2e/tests/accessibility/relations-shortcode-a11y.spec.ts`): axe zero violations on the detail page, the group list page and the `/mrql` result page.

## 10. Docs
- `docs-site/docs/features/shortcodes.md`: table row and a `[relations]` section (attributes, item fields, carrier rule, `out`/`in` and paired types, scoping, the 500 bound, budget arithmetic, SectionConfig, examples); `[item]` row names `[relations]`.
- `docs-site/docs/features/custom-templates.md`: overview line.
- `docs-site/docs/concepts/relationships.md`: "Showing relations in templates".
- The three reference panels.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`.

## 11. Success criterion
1. On a group whose template contains `[relations]`, an admin sees every outgoing typed edge (or incoming with `direction="in"`), grouped by type, each a working link with the far group's name and category, in nested-list markup that passes axe on the detail page, the list page and the `/mrql` page.
2. A group-confined user sees only edges whose both endpoints are inside their subtree on every surface that renders it (page, lightbox JSON, previews, deferred), "unavailable" where the handle has no ORM scope, and none with no allow-list; a 40,000+-ID allow-list works.
3. Signature drift fails compilation (assertion plus embedded interfaces).
4. Each statement costs one budget unit; at most two statements per expansion; `direction="in"` uses `idx__group_relations__to_group_id`.
5. `go test --tags 'json1 fts5' ./...`, the Postgres tests, `./scripts/css-scan-test.sh`, the new E2E and a11y specs pass.

## 12. Checklist
- [x] Plan revised (rev. 5)
- [x] Shortcode handler, dispatch (tests first)
- [x] Lint, builtin docs (tests first)
- [x] Application context and index (tests first; SQLite and Postgres)
- [x] Lister, page and API wiring (tests first)
- [x] HTTP scope tests
- [x] CSS, reference panels, docs-site, skill reference
- [x] E2E and a11y specs
- [x] Suites run

## 13. Review

### Deviations from rev. 5 as written
- **Names.** The details method is `LoadRelatedGroupDetails` (returns `[]models.Group` with `id, description, meta` only); `RelationListerContext` also carries `LoadRelatedGroupDetails`, `MRQLQueryTimeout` and `Logger`, because the lister owns the budget, the timeout and the failure log as `[adjacent]`'s resolver does.
- **`countingPageContext`** (a test fake) gained explicit no-op relation methods rather than an embedded `RelationListerContext`: embedding a second interface that also declares `Logger` makes the struct's `Logger` ambiguous.
- **U12** (processor dispatch) has no separate `processor_test.go` case: every handler test renders through `Process`, so the dispatch line is exercised by all of them.
- **The parser** (`shortcodes/parser.go`) needed `relations` in both name regexes; rev. 5 did not list the file.
- **Reference panels.** On resource categories and note types the entry shows `[using rel="owner"][relations][/using]`, since only groups have relations.
- **API client.** `e2e/helpers/api-client.ts` `createRelationType` takes an optional `reverseName`, so the E2E can create a paired type.
- **The CSS** has no rule for the category span: a reduced opacity there would have been a contrast risk for no gain.

### Found while implementing
- `direction="in"` on a group with a paired type also lists the stored reverse half of each pair (E2E: "Incoming: Required by" beside "Incoming: Blocks"). That is the behaviour §2 and the docs describe ("use `in` for types without a back relation"); the E2E now asserts it rather than hiding it.
- An HTML scope test's `href` used `[item path='GroupURL']`; the linter warns on a bare `[item]` as a URL. The docs and examples use `/group?id=[item path='GroupID']`, and say why.

### How each evaluator point (rev. 4, appended below) was resolved
1. **Far-group load through the scope callback.** Resolved: no statement reads `groups` through the scoped handle. The edge statement joins the far group (its table is `group_relations`, which the callback does not map); details go through `unscopedDB()` after `visibleGroupIDs`. `TestRelationsForTemplateLargeAllowList` (40,000 IDs, SQLite) and `_Postgres` (70,000 IDs) pass; mutating details back to the scoped handle fails the SQLite test with "too many SQL variables".
2. **Statements versus units.** Resolved: one statement per expansion (+1 for details), one unit per statement. `TestRelationsForTemplateRunsOneStatement` counts statements; `TestRelationListerChargesOneUnitPerStatement` and `TestRelationListerRefusesDetailsPastTheBudget` count units (removing the second charge fails both); `TestRelationsShortcode_SharesThePageBudget` checks it on a real page.
3. **Far group unreachable.** Resolved: `GroupDescription` and `GroupMeta` in block bodies, loaded only when named (`TestRelations_FarGroupDetailsLoadOnlyWhenNamed`, the deferred HTTP test and the E2E block body).
4. **Complexity of `both` and dedup.** Resolved by removal: `direction` is `out|in`, default `out`; `both` is an error marker and a lint error.
5. **Small checks.** `handler_interfaces.go` line 106 untouched; only `mrqlRenderContext` embeds the interface. The type-name-across-categories rule is in the attribute table of the docs-site page, the builtin docs and the skill reference, and `runRelationsForTemplateListing` pins it.
6. **A11y check named.** `e2e/tests/accessibility/relations-shortcode-a11y.spec.ts` runs `checkA11y` (axe, WCAG A/AA tags) on the group detail page, the group list page and the `/mrql` results page, and asserts the nested list roles; all pass.

### Validation
- `go test --tags 'json1 fts5' ./... -count=1 -timeout 40m`: 48 packages ok, `application_context` failed once on `TestAPauseAskedOfAnotherProcessIsDeliveredToTheTransfer` (a download-pause timing test this change does not touch; 5/5 in isolation) and then passed in a full package re-run.
- The new `EnsureSupplementalIndexes` entry broke `TestSuggestedTags_LargePopulationQueryPlan`, whose fixture migrated no `group_relations`; the fixture now migrates it.
- Postgres: `go test --tags 'json1 fts5 postgres' ./application_context/ -run 'RelationsForTemplate.*_Postgres|RelatedGroupStagingProbesPerResourcePG|HolderCursorIsServedByTheHashIDIndexPG|TestAdjacentScope_Postgres'` ok.
- `go vet --tags 'json1 fts5' ./...` reports one pre-existing issue, in `application_context/job_download_progress_test.go` (untouched here).
- `./scripts/css-scan-test.sh` passes; E2E `112-relations-shortcode.spec.ts` and `relations-shortcode-a11y.spec.ts` 7/7; neighbouring specs (adjacent, panel-summary-slot, template-preview, category-template-authoring a11y, adjacent a11y) 34/34.

### Not verified (rev. 5)
- The HTTP-level tests (`server/api_tests/relations_shortcode_scope_test.go`) run on SQLite only; Postgres parity is at the application-context level (listing, scope, large allow-list).
- No Postgres `EXPLAIN` was run for the `in` index in rev. 5 (covered in round 1, below).
- The full E2E suite, the CLI E2E suite and the Postgres E2E suite were not run (shared machine); the new specs and the neighbouring shortcode, template-preview and category-authoring a11y specs were.

### Integration review, round 1

- **Visibility before the limit.** Both directions now filter on the far endpoint before applying `limit`, superseding the 500-row candidate scan in §§5–6. The filter binds the existing ORM scope snapshot as one JSON set (`json_each` on SQLite; `jsonb_array_elements_text` converted to `bigint` on PostgreSQL). Re-traversing the live subtree could disagree with the carrier and details checks after a concurrent move, and let a row hidden in this request's snapshot displace one it should see. Unconfined reads use no set parameter. `TestRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges` and its PostgreSQL twin check adding 500 earlier hidden edges in either direction, then moving a hidden endpoint without rebinding. The 40,000/70,000-ID tests still cover both driver limits. `TestRelationsForTemplateUsesAnIndexPerDirection` and its PostgreSQL twin check both directions with and without the scope predicate; on SQLite the far-endpoint predicate is a filter, not a probe of `unique_rel` for every ID in the snapshot.
- **Request wiring and cost.** The bound filter lazily memoizes its Go membership map and its serialized set once per scope (`TestVisibleGroupIDsReusesBoundScopeMembership`). Invisible carriers and unavailable bindings are answered before charging a query unit (`TestRelationListerNoStatementLeavesBudgetUnchanged`). MRQL `render=list` rebinds only the relation lister to the scoped card hydration handle, preserving the existing render budget and other caches. `render=1` still fails closed without an ORM binding. `TestRelationsShortcode_ListCardsAreScoped` covers ordinary `/groups` and MRQL list rendering with a confined guest and an admin control; `TestRelationsShortcode_MRQLRenderAPI` and `TestRelationsShortcode_PublicShareIsUnavailable` pin the unavailable paths.
- **Index rollout.** On PostgreSQL the incoming index uses the planned-index helper, built concurrently on a dedicated connection beside the server. Reads are correct but may be slower until it is ready. SQLite still builds it before serving, holding the writer lock on the initial build. `TestRelationIncomingIndexIsNotBuiltByPostgresStartupMigration` and `TestRelationIncomingIndexBuildAllowsPostgresWrites` verify the migration seam and that a new relation write succeeds while the index build waits for an older writer.
- **Binding and evaluation.** `[item]` and `[calc]` item operands bind only directly in an `[each]` or `[relations]` item branch, before `[else]`, including the rule that neither eager nor lazy tabs inherit an outer item's binding. A nested loop binds its own item. `TestLintItemBindingMatchesTheTopLevelElementBranch` handles nested blocks and else branches. `TestScoreChecks/relations_else_slot_passes` covers `[else]` recognition in the evaluator.
- **Accessibility.** Both unbulleted `<ul>` levels have explicit `role="list"` to preserve their semantics on Safari/VoiceOver. `TestRelations_InlineMarkup` and `relations-shortcode-a11y.spec.ts` assert the roles; Safari/VoiceOver was not exercised manually.
- **Not changed.** `raw="true"` remains an explicit trusted-template-author opt-in, just as for `[item]` and `[property]` elsewhere. The reference now warns that this emits unsanitized far-group Description and Meta. Ordering by the joined type name still requires examining and ordering the carrier's qualifying edges on high-degree groups; `limit` is an output bound, not a work bound. The existing MRQL query timeout still bounds the statement. Scoped SQL receives the serialized scope for each edge statement; serialization and Go membership construction are memoized, not database parsing of the set.

Round 1 validation: the targeted SQLite and PostgreSQL tests, focused HTTP tests, and `go test --tags 'json1 fts5' ./shortcodes/... ./internal/... ./server/template_handlers/... -count=1 -timeout 45m` passed. `npm run build`, `./scripts/css-scan-test.sh`, and the two relations Playwright specs (7/7) passed. Vet reported only the pre-existing lock-copy warning at `application_context/job_download_progress_test.go:99`. An extra unfiltered run of the main, application-context, models and API handler packages failed only on the unrelated `TestPluginCommandInputsAgainstTheRealStore` (`Discard: output unverified`); it passed 5/5 in isolation.

### Integration review, round 2

- **Charge details only for item field references.** The far-group lookup is requested only for a top-level `[item]` value path or a parsed `[calc]` item operand rooted at `GroupDescription` or `GroupMeta`. An index-only `[item]`, fallback text, carrier references and `[inspect]` do not request details. `TestRelationsShortcodeChargesDetailsOnlyForItemReads` runs the renderer with the real lister and one budget unit: fallback text, an unrelated path, index-only output and a carrier reference render normally, while the real `[item]` and `[calc]` details reads exhaust the budget. `TestRelations_FarGroupDetailsLoadOnlyWhenNamed` verifies that details still render when budget permits.

Round 2 validation: the new one-unit regression failed on the old handler and passed with the fix. The targeted Go suites (`shortcodes`, `internal`, `server/template_handlers`) and all `TestRelationsShortcode_` API tests passed with `json1 fts5`. `npm run build` and both relations Playwright specs passed (7/7). Vet reported only the pre-existing lock-copy warning in `application_context/job_download_progress_test.go:99`. No SQL changed; PostgreSQL tests were not rerun.

#### Known limits

- A literal `[else]` inside a quoted shortcode attribute can be mistaken for the branch divider by rendering and evaluator slot detection. The shared divider finder already behaves this way for `[each]` and `[using]`; a fix is deferred.
- On pathological nested sources the binding linter can reparse enough attributes to exhaust its work budget. It fails safe with “too complex to check”; reusing the original token stream is deferred.

## Appendix: last evaluator feedback on rev. 4

1. Scoping (blocks approval): the "secondary check" in §5.3 loads far groups with `ctx.db.WithContext(reqCtx).Preload("Category").Where("id IN ?", visibleFarIDs)`. That goes through the scoped `groups` read callback, which appends the principal's whole allow-list as a second `IN` clause. Past SQLITE_MAX_VARIABLE_NUMBER or Postgres's 65535-parameter cap the query errors. Load names and categories through a handle that skips the scope callback, handle the deletion race by dropping missing rows, and add a test with an allow-list larger than the SQLite variable limit.
2. Statement count on list pages: each card runs 4–5 statements but costs one unit. Say plainly that the cap is exceeded, batch, or charge per statement or per direction.
3. Usefulness: relations panels already exist; the far group's Meta cannot be reached. Exposing far-group Description or a Meta path through `[item]` would raise the value considerably.
4. Complexity against value: consider dropping `both` dedup in favour of `out` as the default, or justify `both` over two invocations.
5. `handler_interfaces.go` line 106 is the MRQL handler interface; `mrqlRenderContext` alone is enough. The docs must explain that the type filter is a name match across categories.
6. Name the exact a11y check for "Incoming: " labels in nested lists, and confirm axe passes on the MRQL result page as well as the group list page.
