# Plan — Tier 3: Tag Untagged Only

> **STATUS: DONE (2026-06-30), core only.** Implemented on branch `feat/lightbox-tagging`.
> Backend: `Untagged bool` on `ResourceSearchQuery` + correlated `NOT EXISTS` predicate in
> `ResourceQuery` (mirrors `ShowDhashZero`), confirmed via `EXPLAIN QUERY PLAN` to hit the
> `resource_tags` composite-PK index (no full scan). `isEmptyResourceSearchQuery` intentionally
> left unchanged — matches existing precedent for `ShowDhashZero`/`ShowWithSimilar`. OpenAPI
> picks the field up via reflection (regenerated, diff-clean). Frontend: "Only Untagged"
> checkbox in `searchFormResource.tpl`; "Tag untagged" launcher link in the group page's
> Resources panel (`seeAll.tpl` gained an opt-in `showUntaggedLink` flag, not resource-specific).
> TDD (red→green): `server/api_tests/resource_untagged_filter_test.go` (unit + HTTP + scoped-user)
> and `e2e/tests/13h-lightbox-tag-untagged.spec.ts` (6 tests incl. a11y), both green on SQLite
> and Postgres; full E2E sweep (browser + CLI) green, only pre-existing unrelated flakes (see
> `project_known_flaky_e2e` memory) retried green. **Deferred:** the "(stretch)
> skip-tagged-while-paging" goal — explicitly optional per this plan's own effort note ("L if the
> stretch is included") and not requested; the lightbox already never shows a tagged item while
> paging a `Untagged=1`-filtered list (server-side filter), it just doesn't *additionally* hide
> items tagged earlier in the same browser session. Revisit if that gap is reported as a problem.

## Scope (what separates "tag 5" from "tag 5000")
Tagging a handful of resources is fine with the existing UI: open the gallery, open the
lightbox, tag each item. The pain at 5000 is *finding* the untagged ones. Today there is no
way to ask "show me only the resources that have no tags". You scroll past already-tagged
items, lose your place, and re-tag things you already did. This feature adds:

1. A backend predicate "resource has zero tags" that composes with every existing filter
   (owner/group, content type, RBAC subtree scope, paging).
2. A one-click launcher ("Tag untagged") that lands you on the resource list pre-filtered to
   untagged-only, so the existing custom lightbox naturally opens over and pages through
   only-untagged media.
3. (Stretch) While paging in the lightbox, skip items that were tagged earlier in this same
   session so you never see them again without a reload.

Out of scope: changing how tagging itself works (quick-tag panel already exists), bulk-tag
flows, or a separate "untagged count" badge (nice-to-have, noted in open questions).

## Current behavior (file:line evidence)

### ResourceSearchQuery
`models/query_models/resource_query.go:48-79`. Has `Tags []uint` (line 56) as an *include*
filter. No "untagged" / "has no tags" predicate. Boolean-flag precedent already exists on the
same struct: `ShowWithoutOwner` (69), `ShowWithSimilar` (70), `ShowDhashZero` (78).

### Resource scope — Tags include filter
`models/database_scopes/resource_scope.go:9-164`. `ResourceQuery(query, ignoreSort, originalDb)`.
- Tags include (22-35): builds a subquery off `originalDb` on `resource_tags rt`, groups by
  `rt.resource_id`, `HAVING count(*) = len(tags)`, then `resources.id IN (subQuery)`.
- Correlated-EXISTS precedent (the closest mirror for "untagged"): `ShowWithSimilar` (68-82)
  and `ShowDhashZero` (87-94) both do `dbQuery.Where("EXISTS (?)", correlatedSubqueryOnOriginalDb)`
  where the subquery references `resources.id`. A `NOT EXISTS` mirrors this exactly.
- The join table is `resource_tags` (GORM many2many: `models/resource_model.go:37`,
  `models/tag_model.go:18`), composite PK `(resource_id, tag_id)` — auto-migrated index that a
  correlated `WHERE rt.resource_id = resources.id` hits directly.

### Query-param wiring (automatic via gorilla/schema)
`server/api_handlers/api_handlers.go:15` `decoder = schema.NewDecoder()`, `:32` `IgnoreUnknownKeys(true)`.
The API path uses `tryFillStructValuesFromRequest(&query, request)`
(`server/api_handlers/resource_api_handlers.go:29-31`); the HTML/template path uses
`decoder.Decode(&query, request.URL.Query())`
(`server/template_handlers/template_context_providers/resource_template_context.go:28-29, 127, 201, 276`).
Both reflect struct fields, so adding `Untagged bool` is auto-bound from `?Untagged=1` /
`?Untagged=true`. No per-field decode code to touch (same as `ShowWithSimilar`, which has no
manual wiring).

### RBAC scope composition
`application_context/resource_crud_context.go:118-135` `GetResources` runs
`ctx.db.Scopes(database_scopes.ResourceQuery(query, false, ctx.db))`. For a group-limited
principal, `scopeReadCallback` (`application_context/scoping.go:231-246`) appends
`resources.owner_id IN (subtree)` (or `1 = 0` fail-closed) to the *same* `resources` statement.
The untagged predicate is just another `WHERE` on that statement, so it ANDs with the scope
clause. The `NOT EXISTS` subquery targets `resource_tags` (not a scopeable table per
`scopeColumn`, line 54-63) but is correlated to `resources.id`, which is already scoped — no
bypass. (`originalDb` passed in is `ctx.db`, the scoped handle; harmless because resource_tags
is unscoped anyway.)

### Lightbox pagination + list template wiring
- Custom Alpine store (NOT baguetteBox). `src/components/lightbox/navigation.js`:
  - `initFromDOM` (59-91) scans `.list-container/.gallery/.dashboard-grid` for
    `[data-lightbox-item]`, sets `baseUrl = window.location.pathname + window.location.search`
    (72) — so any query param on the page (e.g. `Untagged=1`) is preserved.
  - `fetchPage` (421-463) requests `{baseUrl pathname}.json{search}&page=N`, reads
    `data.resources` + `data.pagination.NextLink.Selected` (443), filters to
    `image/*`|`video/*` (445-446).
  - `loadNextPage`/`loadPrevPage` (343-419) append/prepend items.
- `templates/listResourcesDetails.tpl:38-48` emits the `data-lightbox-item` anchors
  (`data-resource-id`, `data-content-type`, `data-resource-name`, `data-resource-hash`).
- Routes: `server/routes.go:44-46` `/resources`, `/resources/details`, `/resources/simple`
  all -> `ResourceListContextProvider`; `.json` suffix is dual-response (routes.go:141-179).
- Search form: `templates/partials/form/searchFormResource.tpl` already renders a boolean
  checkbox via `checkboxInput.tpl` for `ShowWithSimilar` (the exact pattern to copy;
  `checkboxInput.tpl` posts `value="1"` when checked).
- Group page resource link: `templates/displayGroup.tpl:38` `seeAll` partial with
  `formAction="/resources"`, `formParamName="ownerId"`.

## Backend design

### The "untagged" predicate
SQL approach: **correlated `NOT EXISTS`**, not `LEFT JOIN ... IS NULL`.

```sql
NOT EXISTS (SELECT 1 FROM resource_tags rt WHERE rt.resource_id = resources.id)
```

In the scope, mirroring `ShowDhashZero` (resource_scope.go:87-94):

```go
if query.Untagged {
    untaggedSub := originalDb.
        Table("resource_tags rt").
        Where("rt.resource_id = resources.id").
        Select("1")
    dbQuery = dbQuery.Where("NOT EXISTS (?)", untaggedSub)
}
```

Why NOT EXISTS over LEFT JOIN:
- `LEFT JOIN resource_tags ... WHERE tag_id IS NULL` forces a join + dedup against a list that
  is already de-duplicated by other filters, and interacts badly with the existing
  `resources.id IN (...)` subqueries and `Preload` set; it can also multiply rows before the
  null-check. `NOT EXISTS` is a semijoin the planner short-circuits on first match.
- Identical SQL string works on both SQLite and Postgres (no dialect branching like
  `likeOperator`).
- Hits the `resource_tags(resource_id, tag_id)` composite-PK index directly: the planner
  probes the index by `resource_id` and stops at the first row.

Performance at millions of rows: the predicate is O(1) index-probe per candidate row, applied
*after* cheaper filters (owner/group/content-type) have already narrowed the set in the same
`WHERE`. No extra index needed — the many2many PK index covers it. Validate the plan with
`EXPLAIN QUERY PLAN` (SQLite) and `EXPLAIN` (Postgres) on a seeded DB to confirm index use and
no full scan of `resource_tags`.

Mutual exclusivity: `Untagged=1` together with `Tags=[...]` is contradictory (can't be both
untagged and have a given tag). Decision: let them AND naturally (yields empty set) rather than
erroring — simplest, and the UI never sets both at once. Note in open questions.

### Query-param wiring + composition with RBAC scope
- Add `Untagged bool` to `ResourceSearchQuery` (resource_query.go). gorilla/schema auto-binds
  `?Untagged=1`; no handler edits.
- Composition with scope is automatic (see Current behavior > RBAC). The predicate adds a
  `WHERE`, never replaces the statement, so the scope callback's `owner_id IN (subtree)` still
  applies. Confirm with a scoped-user test that an untagged resource owned *outside* the
  subtree is not returned.
- `isEmptyResourceSearchQuery` (`resource_api_handlers.go:96`): check whether it needs to count
  `Untagged` as "non-empty" so `/v1/resource/content` search-by-criteria still works. Read and
  update if it field-lists explicitly.

### TDD: Go api/unit tests first
Write these RED before touching the scope (model on `resource_dhash_zero_filter_test.go` and
`resource_filter_test.go`):

- [ ] `server/api_tests/resource_untagged_filter_test.go`
  - Create 3 resources: one with a tag, one with two tags, one with zero tags.
  - `tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{Untagged: true})` returns
    only the zero-tag resource. Assert by name (Contains/NotContains like the dhash test).
  - HTTP layer: `GET /v1/resources?Untagged=1` with `Accept: application/json` returns only the
    untagged resource (mirror `resource_filter_test.go` request style).
  - Negative: without `Untagged`, all three are returned.
- [ ] Scoped-user test (model on existing scoped tests; search `application_context` and
  `server/api_tests` for `WithPrincipal`/scope helpers): a group-limited principal querying
  `Untagged=true` sees only untagged resources *inside its subtree*; an untagged resource owned
  by an out-of-subtree group is excluded. Proves no scope bypass.
- [ ] Edge: a resource that *had* tags then had them all removed (zero rows in resource_tags)
  is treated as untagged.

Run RED: `go test --tags 'json1 fts5' ./server/api_tests/... ./models/...` — expect compile/
assert failures. Then implement the scope field + predicate to GREEN.

## Frontend design

### Launcher entry point(s)
The launcher is just a link to the resource list with `Untagged=1` in the query string; the
lightbox then opens naturally over untagged-only items because `baseUrl` carries the param and
`fetchPage` preserves it across pages.

1. Resource list sidebar (primary): add an `Untagged` checkbox to
   `templates/partials/form/searchFormResource.tpl`, copying the `ShowWithSimilar` line:
   `{% include "/partials/form/checkboxInput.tpl" with name='Untagged' label='Only Untagged' value=queryValues.Untagged.0 id=getNextId("Untagged") %}`.
   Submitting the filter form reloads `/resources/details?...&Untagged=1`.
2. Group page (the "tag this group's new imports" flow): add a "Tag untagged" button near the
   Resources `seeAll` block in `templates/displayGroup.tpl:38` linking to
   `/resources/details?ownerId={{ group.ID }}&Untagged=1`. This composes owner + untagged so a
   confined user lands on exactly their group's untagged media.
3. (Optional) a top-level "Tag untagged" affordance on the resource list header linking to
   `/resources/details?Untagged=1`.

No JS change required for the basic launcher — the existing store already preserves the query
string. Verify `queryValues.Untagged.0` is populated so the checkbox renders checked on reload
(the template context exposes `queryValues` from the raw query map; confirm in
`resource_template_context.go`).

### (stretch) skip-tagged-while-paging
Goal: once you tag an item in the lightbox during a session, don't show it again as you keep
paging, even though the server-side list for the current page already included it.
- The quick-tag panel already posts tags (search `src/components/lightbox/` for the tag-apply
  method and `fetchResourceDetails`, referenced at navigation.js:179). On a successful
  add-tags for the current resource, record `this._taggedThisSession.add(id)`.
- In `fetchPage` (navigation.js:445) extend the `.filter(...)` to also drop
  `this._taggedThisSession.has(r.ID)`. This only affects *newly fetched* pages, so it does not
  mutate `currentIndex` mid-view (avoids the index-shift bugs the file already guards against,
  e.g. BH:L1, BH:M1).
- Do NOT retroactively splice tagged items out of `this.items` while the user is viewing them
  (that would shift `currentIndex` and skip/repeat media). Keep it fetch-time only.
- Reset `_taggedThisSession` on `close()` is optional; keeping it for the page's lifetime is
  fine and matches "in this session".

### TDD: failing Playwright E2E first
Add `e2e/tests/<nn>-tag-untagged.spec.ts` (RED first), run against an ephemeral server:
- [ ] Seed via API client: N resources, tag some, leave some untagged (at least 2 untagged
  images so paging/next works).
- [ ] Navigate to `/resources/details?Untagged=1`; assert the rendered rows are exactly the
  untagged set (count + names), and the tagged ones are absent.
- [ ] Open the lightbox on the first untagged item; press Next; assert it lands on the second
  *untagged* item (not a tagged one), proving the filter rides through `fetchPage`.
- [ ] (stretch) Tag the current item via the quick-tag panel, page forward then back / to next
  page, assert the just-tagged item is not shown again.
- [ ] a11y: run the axe check on `/resources/details?Untagged=1` (the new checkbox must have a
  label — `checkboxInput.tpl` already wires `for`/`id`).

## Implementation steps
- [ ] RED: write `server/api_tests/resource_untagged_filter_test.go` (unit + HTTP + scoped-user)
      and run to confirm failure.
- [ ] Add `Untagged bool` to `ResourceSearchQuery` (`models/query_models/resource_query.go`)
      with a short comment.
- [ ] Add the `NOT EXISTS` predicate block to `ResourceQuery`
      (`models/database_scopes/resource_scope.go`, beside `ShowDhashZero`).
- [ ] If `isEmptyResourceSearchQuery` enumerates fields, include `Untagged`
      (`server/api_handlers/resource_api_handlers.go`).
- [ ] GREEN: `go test --tags 'json1 fts5' ./server/api_tests/... ./models/...`.
- [ ] Validate SQL on Postgres: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`.
- [ ] Manual `EXPLAIN`/`EXPLAIN QUERY PLAN` on a seeded DB (both engines) to confirm index use.
- [ ] OpenAPI: confirm whether the generator reflects `ResourceSearchQuery` fields
      automatically (the existing `ShowDhashZero`/`ShowWithSimilar` are not hand-listed in
      `server/routes_openapi.go`, suggesting reflection). If so, just regenerate; if params are
      hand-declared anywhere, add `Untagged`. Run `go run ./cmd/openapi-gen` and check the diff.
- [ ] Frontend: add `Untagged` checkbox to `searchFormResource.tpl`; add "Tag untagged" link to
      `displayGroup.tpl` (and optional list-header affordance).
- [ ] Confirm `queryValues.Untagged.0` renders the checkbox checked on reload.
- [ ] RED: write `e2e/tests/<nn>-tag-untagged.spec.ts`; run to confirm failure.
- [ ] (stretch) Implement skip-tagged-while-paging in `navigation.js` (`_taggedThisSession`
      set + `fetchPage` filter + record on quick-tag success); make the stretch E2E GREEN.
- [ ] `npm run build-js`.
- [ ] GREEN E2E: `cd e2e && npm run test:with-server:all` and `npm run test:with-server:postgres`.
- [ ] Full suite: `go test --tags 'json1 fts5' ./...` then the postgres command above.
- [ ] Update `tasks/todo.md` review section; capture any lesson.

## Files touched
- `models/query_models/resource_query.go` — add `Untagged bool`.
- `models/database_scopes/resource_scope.go` — add `NOT EXISTS` predicate.
- `server/api_handlers/resource_api_handlers.go` — only if `isEmptyResourceSearchQuery` lists fields.
- `templates/partials/form/searchFormResource.tpl` — add checkbox.
- `templates/displayGroup.tpl` — add "Tag untagged" launcher link.
- `src/components/lightbox/navigation.js` — (stretch) skip-tagged-while-paging.
- `server/routes_openapi.go` + regenerated `openapi.yaml`/`openapi.json` — only if params are hand-declared.
- `server/api_tests/resource_untagged_filter_test.go` — new (unit + HTTP + scoped).
- `e2e/tests/<nn>-tag-untagged.spec.ts` — new.

## Risks & gotchas
- **Perf of NOT EXISTS at scale**: must verify the planner uses the `resource_tags(resource_id,
  tag_id)` index and does not scan. Apply the predicate alongside (not before) the cheaper
  owner/content-type filters so the candidate set is already small. Validate on BOTH SQLite and
  Postgres — the SQL string is engine-neutral but plans differ.
- **Pagination correctness as items get tagged (stretch)**: only filter at fetch time, never
  splice the live `items` array while viewing, or `currentIndex` shifts and media is skipped/
  repeated (the file already documents BH:L1/M1 hazards). Keep skip-tagged fetch-only.
- **Scope leak**: the predicate must remain an additional `WHERE`; never build it in a way that
  replaces the statement or queries a fresh unscoped DB for the *outer* resource set. The
  scoped-user test is the guard. The correlated subquery on `resource_tags` is safe because the
  outer `resources` rows are already scope-filtered.
- **Checkbox value binding**: `checkboxInput.tpl` posts `value="1"`; gorilla/schema decodes `1`
  to `bool true`. An *unchecked* box sends nothing, so the filter clears correctly on a fresh
  submit (matches `ShowWithSimilar`).
- **`Untagged` + `Tags` both set** yields empty (contradiction). Acceptable; UI never sets both.
- **`.json` dual-response**: the lightbox relies on `/resources/details.json?Untagged=1&page=N`
  returning `{resources, pagination}`. This is the existing list JSON contract; the new param
  just rides the query string. Re-confirm the JSON payload shape is unchanged.

## Effort (M-L)
Backend predicate + tests: S-M (one struct field, one scope block, mirrors `ShowDhashZero`).
Frontend launcher: S (one checkbox + one link, no JS). Stretch skip-tagged-while-paging + its
E2E: M (touches the lightbox tag-apply path and fetch filter). Cross-engine + full E2E
verification dominates the wall-clock. Overall **M**, **L** if the stretch is included.

## Open questions / decisions
- Naming: `Untagged` (chosen) vs `NoTags`/`ShowUntagged`. `Untagged` reads cleanest as a query
  param and matches the UI label.
- Should there be an untagged *count* badge (e.g. on the group page) so users know how many
  remain? Cheap-ish (`SELECT count(*) ... NOT EXISTS ...`) but adds a query per page render;
  defer unless requested.
- Should `Untagged` apply to notes/groups too (they also have tags)? Out of scope for Tier 3
  (resource tagging is the stated pain); note as a possible follow-up with the same pattern.
- Reset `_taggedThisSession` on lightbox `close()` or keep for the page lifetime? Leaning keep
  (matches "in this session"); confirm with product intent.
- Confirm the OpenAPI generator reflects `ResourceSearchQuery` (no hand-listed params for the
  existing booleans suggests yes) before assuming `routes_openapi.go` needs edits.
