# Plan — Tier 0: Foundation Fixes (lightbox tagging)

## Scope

Three foundation fixes that make in-lightbox tagging fast and correct at scale.
This tier is first because every later tagging feature (bulk slots, smart
suggestions, keyboard flows) sits on top of three primitives that are currently
broken or unscalable: the tag-state cache during navigation, the tag typeahead
query, and tag creation idempotency. Fix the substrate before building on it.

- Item 1 (frontend, highest priority): stop blanking `resourceDetails` on every
  next/prev so quick-slot colors no longer flash neutral, and prefetch tag
  details for upcoming items the same way bitmaps are already prefetched.
- Item 2 (backend): make `/v1/tags` typeahead scale — index the join column the
  `most_used` correlated subquery scans, and stop paying a second full-table
  COUNT per keystroke.
- Item 3 (backend + small frontend): turn a duplicate-tag-name create into a
  select of the existing tag instead of a generic "Could not add" error.

All three are independent and can land in separate commits.

---

## Item 1: Stop blanking + prefetch tag state

> **Status: DONE (2026-06-30).** TDD red→green via new spec
> `e2e/tests/13d-lightbox-tag-prefetch.spec.ts` (3 tests). Implemented by removing
> the `resourceDetails = null` blanking + incoming-cache eviction in
> `onResourceChange`, adding `_preloadDetailsUpcoming()` (panel-gated, called from
> `_preloadUpcoming`) plus a `_detailsInFlight` guard, and `:aria-busy` bindings on
> both panel roots. Verified: 66/66 lightbox + tag specs green, including the
> stale-tag and cached-navigation guards in `13-lightbox.spec.ts`. Frontend-only;
> full browser/CLI/postgres sweep not yet run.

### Current behavior (file:line evidence)

- `onResourceChange` blanks state on every navigation:
  - `src/components/lightbox/editPanel.js:235` — `this.resourceDetails = null;`
  - `src/components/lightbox/editPanel.js:236-239` — reads `getCurrentItem().id`
    and `this.detailsCache.delete(resourceId)` for the **incoming** resource,
    evicting the entry we are about to need.
  - `src/components/lightbox/editPanel.js:240` — then `await this.fetchResourceDetails()`.
- `slotMatchState` returns `'none'` whenever details are null:
  - `src/components/lightbox/quickTagPanel.js:368-382`, specifically
    `:372` `if (!this.resourceDetails) return 'none';`.
- The slot color `:class` binding keys entirely off `matchState`
  (`templates/partials/lightbox.tpl:611-616`, via the `matchState` getter at
  `:541`). So when `resourceDetails` goes null, every filled slot collapses from
  green (`all`) / amber (`some`) to neutral stone (`none`) until the network
  round-trip in `fetchResourceDetails` completes and repaints.
- `fetchResourceDetails` already has a synchronous cache hit path
  (`editPanel.js:169-176`) and writes the cache on success (`:204`). Optimistic
  writes keep the cache correct after tag edits
  (`quickTagPanel.js:457-459`, `editPanel.js:373`, `:425`).
- Bitmaps already prefetch 5 ahead but **tag details do not**:
  - `src/components/lightbox/navigation.js:34` `_preloadAheadCount: 5`
  - `navigation.js:183-209` `_preloadUpcoming()` (images only)
  - called from `open()` `:175`, `next()` `:294`, `prev()` `:322`; navigation
    also calls `onResourceChange()` at `:295` / `:323`.
- `navigation.js:478-480` `getCurrentItem()` returns `this.items[this.currentIndex]`.

### Desired behavior

1. On navigation, keep the previously rendered `resourceDetails` object visible
   (dimmed via `aria-busy`) instead of nulling it; swap it only once the new
   resource's details are ready (cache hit = instant; otherwise after fetch).
2. Never evict the **incoming** resource's cache entry in `onResourceChange`
   (delete the `detailsCache.delete(resourceId)` call). Background-revalidate
   instead by letting `fetchResourceDetails` paint from cache then refresh.
3. Add `_preloadDetailsUpcoming()` mirroring `_preloadUpcoming()`: for the next
   `_preloadAheadCount` items, if not already cached and not in-flight, fetch
   `/resource.json?id=` through the CSRF-aware wrapper and seed `detailsCache`.
   Call it everywhere `_preloadUpcoming()` is called (`open`, `next`, `prev`).
4. While revalidating the current item (cache miss or forced refresh), expose
   `detailsLoading` as `aria-busy` on the quick-tag panel and edit panel so the
   stale-but-correct colors read as "updating", not final.

### TDD test plan (write these FIRST — red)

New spec: `e2e/tests/13d-lightbox-tag-prefetch.spec.ts` (model on
`e2e/tests/13b-lightbox-adversary-fixes.spec.ts` — reuse its `beforeAll`
fixture that creates a category, owner group, and 3 image resources, and its
`LIGHTBOX` selector + `openLightbox` helper).

- [ ] Test A — "quick-slot colors survive navigation without flashing neutral":
      - Seed one quick slot with a tag, apply that tag to image 1 via the API in
        `beforeAll` so image 1 reads `matchState === 'all'`.
      - Open lightbox on image 1, open quick-tag panel (`press('t')`), assert the
        slot's container has the green `all` classes.
      - Navigate `next()` then back to image 1 and assert: immediately after the
        navigation microtask `$store.lightbox.resourceDetails` is **not null**
        (poll via `page.evaluate`), and the slot never carries the neutral
        `bg-stone-800` `none` class while details are loading. Failing assertion
        (red, current code): `resourceDetails` is null right after `next()`.
- [ ] Test B — "tag details for upcoming items are prefetched": install
      `await page.route('**/resource.json**', ...)` to **count** requests by id.
      Open lightbox (panel open), wait for settle, assert the next
      `_preloadAheadCount` ids were each fetched once. Then `next()` into a
      prefetched neighbor and assert **zero** additional `/resource.json`
      requests fire for that id (cache hit). Failing assertion (red): neighbor
      ids are not fetched on open, and `next()` triggers a fresh request.
- [ ] Test C — "panel exposes aria-busy during revalidation": with details
      loading (force a slow route), assert the quick-tag/edit panel root carries
      `aria-busy="true"`, then `"false"` after settle. (a11y guard.)

### Implementation steps

- [ ] `editPanel.js onResourceChange` (`:221-250`): remove the
      `this.resourceDetails = null` blanking and the
      `detailsCache.delete(resourceId)` eviction; rely on `fetchResourceDetails`
      to paint from cache instantly and revalidate in the background.
- [ ] `fetchResourceDetails` (`:165-219`): keep the synchronous cache-hit paint
      (`:171`); ensure a cache miss does not clear `resourceDetails` to null
      before the fetch resolves (hold the prior object, only replace on success
      for the still-current id, guarded by the existing `_detailsReq` token).
- [ ] Add `_preloadDetailsUpcoming()` to `navigation.js` (next to
      `_preloadUpcoming`, `:183-209`): iterate `currentIndex+1 .. +ahead`, skip
      ids already in `detailsCache` or in an in-flight set, fetch via the global
      CSRF `fetch` wrapper (NOT a bypass), and `detailsCache.set(id, details)`.
      Respect a small in-flight guard set so paging fast does not stampede.
- [ ] Call `_preloadDetailsUpcoming()` from `open()` (`:175`), `next()` (`:294`,
      `:306`), and `prev()` (`:322`, `:333`) alongside `_preloadUpcoming()`.
      Only warm when a panel is open (mirror the `open()` `:178` guard) to avoid
      needless fetches when tagging UI is closed.
- [ ] Bound the detail cache reuse: `fetchResourceDetails` already evicts oldest
      when `detailsCache.size > 100` (`:179-181`); confirm prefetch respects it.
- [ ] Template: add `:aria-busy="$store.lightbox.detailsLoading ? 'true' : 'false'"`
      to the quick-tag panel root (`[data-quick-tag-panel]`) and edit panel root
      (`[data-edit-panel]`) in `templates/partials/lightbox.tpl`.
- [ ] `npm run build-js` to rebuild the bundle.

### Files touched

- `src/components/lightbox/editPanel.js`
- `src/components/lightbox/navigation.js`
- `templates/partials/lightbox.tpl`
- `public/dist/main.js` (build output, via `npm run build-js`)
- `e2e/tests/13d-lightbox-tag-prefetch.spec.ts` (new)

### Risks & gotchas

- Prefetch must use the global CSRF-aware `fetch` wrapper (`src/csrf.js`) /
  `abortableFetch` from `src/index.js`; a raw `fetch` would drop `X-CSRF-Token`
  under `-auth` and 403. `/resource.json` is a GET (safe method, CSRF-exempt),
  but still route it through the wrapper for consistency and base-URL handling.
- Do not poison the cache: keep the existing post-await id guard
  (`getCurrentItem()?.id === resourceId`, `:202`) so a fast navigator does not
  write resource A's details under resource B.
- `aria-busy` on a stale-but-shown panel must flip back to `false`; tie it to
  the existing `_detailsReq` discipline so an aborted fetch cannot strand it.
- Watch the prefetch fan-out at the page boundary: `next()` past the last loaded
  item appends a page (`loadNextPage`), so prefetch should clamp to
  `this.items.length` (as `_preloadUpcoming` already does at `:185`).

---

## Item 2: Autocomplete scaling

> **Status: DONE (2026-06-30).** B1 + B5 landed (commit 3a963422). Added tag_id
> indexes on resource_tags/note_tags/group_tags to both index blocks in main.go,
> routed the count-skipping GetTagsHandler at GET /v1/tags/suggest (unscoped, like
> the other tag routes — the plan's scopedAPI suggestion was wrong; tag routes are
> not scoped), and repointed both lightbox autocompleters. B2 (denormalized count)
> deferred. New test server/api_tests/tag_suggest_endpoint_test.go (prefix match +
> cap). Verified green incl. all existing tag tests.

### Current behavior (file:line evidence)

- Each `/v1/tags` typeahead keystroke (debounced 200ms,
  `src/components/dropdown.js:433-460`) runs through the generic CRUD list path:
  - Route: `server/routes.go:531-532` `tagFactory.ListHandler()`.
  - `server/api_handlers/handler_factory.go:68-112 ListHandler` calls
    `reader.List(...)` (`:88`) **and** `reader.Count(typedQuery)` (`:103`) for
    pagination metadata — two queries per request.
  - `application_context/generic_crud.go:71-78 List` and `:81-85 Count`.
- The scope is `models/database_scopes/tag_scope.go`:
  - Name filter is a substring `LIKE`/`ILIKE` `%term%`
    (`tag_scope.go:58-61`, pattern from `models/database_scopes/db_utils.go:75-80`)
    — leading wildcard, cannot use the `unique_tag_name` index.
  - `most_used_<entity>` sort emits a **correlated COUNT subquery** per row:
    `tag_scope.go:46`
    `ORDER BY (SELECT count(*) FROM <entity>_tags jt WHERE jt.tag_id = tags.id)`.
    The lightbox slot autocompleter requests `sortBy: 'most_used_resource'`
    (`templates/partials/lightbox.tpl:570`).
- `resource_tags` (GORM many2many on `models/tag_model.go:18`) is auto-created
  with a composite PK `(resource_id, tag_id)`. There is **no standalone index on
  `tag_id`**, so the correlated subquery's `WHERE jt.tag_id = ?` cannot use the
  PK (leftmost-prefix is `resource_id`) and scans. With millions of resources
  this is a join-table scan per candidate tag, per keystroke, plus the separate
  pagination COUNT over `tags`.
- A lean, count-skipping handler already exists but is **unrouted**:
  `server/api_handlers/tag_api_handlers.go:15-45 GetTagsHandler` calls
  `ctx.GetTags(...)` and passes `-1` to `SetPaginationHeaders` (`:41`), skipping
  the COUNT. Backed by `application_context/tags_context.go:14-18 GetTags`.
  `constants.MaxResultsPerPage = 50` (`constants/constants.go:3`).

### Plan (in priority order)

- **B1 (must): index `resource_tags.tag_id` (and `note_tags`, `group_tags`).**
  Add to the existing post-migrate index block in `main.go:459-489`
  (`indexQueries` for Postgres and `indexQueriesSqlite`), following the existing
  `idx__<table>__<col>` naming and `CREATE INDEX IF NOT EXISTS` idempotency:
  - `resource_tags(tag_id)`, `note_tags(tag_id)`, `group_tags(tag_id)` — these
    are exactly the three `validMostUsedEntities` junctions
    (`tag_scope.go:11-16`). This turns the correlated subquery's per-tag count
    into an index range scan.
- **B5 (should): route a lean typeahead endpoint and point the tag
  autocompleter at it.** Wire `GetTagsHandler` (already count-skipping) at a new
  path, e.g. `GET /v1/tags/suggest`, in `server/routes.go` near `:532`, and
  change `url: '/v1/tags'` to `'/v1/tags/suggest'` in the tag autocompleter
  template includes (search `templates/` for `url='/v1/tags'` + the lightbox
  inline `autocompleter({ url: '/v1/tags' ... })` at
  `templates/partials/lightbox.tpl:568`). This drops the second full COUNT per
  keystroke. Leave `/v1/tags` (the counted list path) for timeline/merge UIs
  that need totals. Decision recorded below.
- **B2 (optional, defer unless B1 proves insufficient): denormalized
  `resource_count` on `tags`.** Maintain on add/remove tag and on merge; sort by
  the stored column instead of the correlated subquery. Larger surface (write
  paths in `resource_bulk_context.go`, `note`/`group` bulk paths, merge,
  delete) — only pursue if profiling after B1 still shows the subquery hot.

### TDD test plan (write these FIRST — red)

Backend Go tests (run with `--tags 'json1 fts5'` and again with `postgres`):

- [ ] `server/api_tests/tag_suggest_endpoint_test.go` (new; model on
      `server/api_tests/duplicate_tag_friendly_error_test.go` for `SetupTestEnv`
      / `MakeRequest`): assert `GET /v1/tags/suggest?name=<prefix>` returns 200,
      a JSON array, and respects the `MaxResultsPerPage` cap. Failing first
      because the route does not exist yet.
- [ ] `application_context` test (model on existing `tags_context` tests, e.g.
      `application_context/tag_*_test.go`) asserting `most_used_resource` sort
      orders tags by descending resource usage. This guards that adding the
      index does not change result ordering. Seed N tags with differing
      `resource_tags` counts; assert order.
- [ ] B1 index presence guard: after migrate, assert the index exists. SQLite:
      query `sqlite_master` for `idx__resource_tags__tag_id`. Keep it cheap; or
      assert via `EXPLAIN QUERY PLAN` that the most_used subquery uses an index
      (SQLite-only assertion, gate behind the non-postgres build).

### Implementation steps

- [ ] Add the three `CREATE INDEX IF NOT EXISTS idx__<jt>__tag_id` statements to
      both `indexQueries` and `indexQueriesSqlite` in `main.go:459-475`.
- [ ] Register `router.Methods(GET).Path("/v1/tags/suggest").HandlerFunc(
      scopedAPI(appContext, api_handlers.GetTagsHandler))` in `server/routes.go`
      (match the surrounding `scopedAPI`/handler-wiring style used for tags).
- [ ] Repoint tag autocompleter `url` from `/v1/tags` to `/v1/tags/suggest` in
      the lightbox inline configs (`templates/partials/lightbox.tpl:376`,
      `:568`). Optionally repoint the form-partial tag autocompleters too
      (`templates/partials/tagList.tpl`, `bulkEditorResource.tpl`,
      `bulkEditorGroup.tpl`, `createResource.tpl`, `createNote.tpl`,
      `createGroup.tpl`, `pasteUpload.tpl`) for a uniform fast path — decide
      scope in review (lightbox-only is the minimal change).
- [ ] `npm run build-js` if any bundled JS changed (template URL changes are
      template-only, but rebuild to be safe).

### Files touched

- `main.go` (index block)
- `server/routes.go` (new `/v1/tags/suggest` route)
- `templates/partials/lightbox.tpl` (+ optionally other autocompleter includes)
- `server/api_tests/tag_suggest_endpoint_test.go` (new)
- `application_context/<tag most_used sort>_test.go` (new)

### Risks & gotchas

- `GetTagsHandler` uses `tryFillStructValuesFromRequest` (form/query), while
  `ListHandler` uses `decoder.Decode` + `FillMetaQueryFromRequest`. The
  autocompleter sends `name`, `SortBy`, and filter params as URL query params;
  confirm `GetTagsHandler` parses `SortBy` (slice) correctly — if not, prefer a
  tiny dedicated suggest handler over reusing `GetTagsHandler` verbatim.
- B1 index creation runs at startup on existing DBs that may hold millions of
  `resource_tags` rows; `CREATE INDEX` is one-time but can be slow on first boot
  after upgrade. It is idempotent (`IF NOT EXISTS`) and matches the existing
  pattern, so acceptable, but note it in the changelog.
- Prefix-vs-substring: B5 keeps substring `%term%` initially. A true
  prefix-first (`term%`, sargable) match needs `COLLATE NOCASE` index on SQLite
  / `text_pattern_ops` on Postgres to be index-usable — out of scope for Tier 0;
  B1 + skipping the pagination COUNT is the bulk of the win.
- Do not change `/v1/tags` (ListHandler) semantics; timeline/merge UIs depend on
  the pagination total header.

---

## Item 3: Duplicate-tag handling

> **Status: DONE (2026-06-30).** CreateTag is now idempotent on a unique-name
> conflict: it returns the existing tag (via new GetTagByName), skipping create
> hooks/log/cache-invalidation, with the friendly-error fallback if the row cannot
> be read back. Applied at the CreateTag layer so JSON/form/CRUD/plugin paths all
> match. Rewrote duplicate_tag_friendly_error_test.go to the idempotent contract
> (red→green) and updated tags_help/tag_create.md. Audited dependents:
> error_sanitization_test.go covers category/resourceCategory/query (not tags),
> the CLI doctest uses random names. Verified: api_tests + application_context +
> plugin_system all green.

### Current behavior (file:line evidence)

- `application_context/tags_context.go:56-103 CreateTag` does a plain
  `ctx.db.Create(&tag)` and on a unique-constraint violation returns
  `fmt.Errorf("a tag named %q already exists", ...)` (`:86-90`,
  `isUniqueConstraintError` from `application_context/db_errors.go:8-17`). It
  does **not** return the existing tag.
- The create route: `server/routes.go:533`
  `CreateTagHandler(appContext)`; handler at
  `server/api_handlers/handler_factory.go:221-285`. For JSON (the autocompleter
  path) it returns the create error via `HandleFormError` → 4xx.
- Frontend autocompleter add flow: `src/components/dropdown.js:173-208 addVal`
  POSTs `{ Name }` as JSON to `addUrl` (`/v1/tag`); on non-OK it sets
  `errorMessage = "Could not add <name>"` (`:201-203`).
- "Add mode" only triggers when the typed value is **absent from the 50-row
  result list** (`dropdown.js:247` `if (!this.results.find(x => x.Name === value))`).
  Because the list is `LIMIT 50`, a tag that exists beyond row 50 is invisible,
  so the user hits Add on an existing name and gets the generic failure toast.
- No tag-by-name lookup helper exists on the context (only `GetTagByID`,
  `GetTagsWithIds`); ad-hoc `Where("name = ?").First` is used elsewhere
  (`application_context/import_context.go:488`, `:542`).

### Desired behavior

`CreateTag` becomes idempotent on name: on a unique-name conflict, look up and
return the existing tag with a nil error. The handler then returns 200 with that
tag; `addVal` pushes it into `selectedResults` and fires `onSelect`
(→ `saveTagAddition` in the lightbox), which already dedupes. The user gets the
tag they asked for instead of an error.

### TDD test plan (write these FIRST — red)

- [ ] **Update** `server/api_tests/duplicate_tag_friendly_error_test.go` — it
      currently asserts the OLD behavior (duplicate POST → `resp.Code >= 400`,
      `:26`, `:56`). Rewrite to the new contract: a duplicate JSON
      `POST /v1/tag` returns **200** and a body whose returned tag has the same
      `ID`/`Name` as the first create (idempotent select). Keep the assertion
      that no raw `UNIQUE constraint failed` / `tags.name` leaks. This is the
      red→green driver for the backend change. (Rename the test to reflect
      idempotent-select semantics.)
- [ ] `application_context` unit test (model on existing `tags_context` tests):
      call `CreateTag` twice with the same name; assert the second call returns
      `(*Tag, nil)` with the same `ID` as the first and no error.
- [ ] E2E (frontend) — extend `e2e/tests/01-tag.spec.ts` or add a focused spec:
      in a tag autocompleter, type a name that already exists but sits beyond the
      50-row window (seed >50 tags so it is not in `results`), press Enter to
      enter add mode, confirm Add, and assert the existing tag is selected (a
      pill appears, `selectedIds` contains its real ID) with no error toast.
      Red first because today this path 4xx's and shows "Could not add".

### Implementation steps

- [ ] Add `GetTagByName(name string) (*models.Tag, error)` to
      `application_context/tags_context.go` (exact-match `Where("name = ?").
      First`), or inline the lookup inside `CreateTag`.
- [ ] In `CreateTag` (`tags_context.go:86-90`): when `isUniqueConstraintError`,
      fetch the existing tag by name and return it with `nil` error (still run
      `after_tag_create`? No — it already exists; skip create hooks/logging and
      do not re-`InvalidateSearchCache`). Return early.
- [ ] Confirm `CreateTagHandler` (handler_factory.go:262-263) needs no change —
      it already encodes whatever `CreateTag` returns. Verify the JSON 200 path
      returns the tag object the autocompleter expects (`ID`, `Name`).
- [ ] `dropdown.js addVal` (`:173-208`): no functional change required since a
      200 with the existing tag flows through `selectedResults.push(newVal)` +
      `onSelect`. Optionally harden: if `newVal.ID` is already in `selectedIds`,
      skip the duplicate push and surface an "already added" announcement rather
      than a second pill. Decide in review.
- [ ] `npm run build-js` if `dropdown.js` changed.

### Files touched

- `application_context/tags_context.go`
- `server/api_tests/duplicate_tag_friendly_error_test.go` (rewrite to new contract)
- `src/components/dropdown.js` (optional hardening)
- `e2e/tests/01-tag.spec.ts` or a new focused spec
- `public/dist/main.js` (if dropdown.js changed)

### Risks & gotchas

- **This intentionally changes documented behavior.** Grep for other tests/docs
  asserting the duplicate-create error before landing: search `"already exists"`
  and `UNIQUE constraint` across `server/api_tests`, `application_context`,
  `e2e/tests/cli`, and CLI help docs (`cmd/mr/.../*_help/*.md`). The CLI
  `mr tags create` may rely on the error — confirm and update help/tests if so.
- `UpdateTag` (`tags_context.go:105-153`) must keep erroring on a name clash with
  a **different** tag — only `CreateTag` (ID == 0, new) becomes idempotent. Do
  not touch the `UpdateTag` unique path.
- Name normalization: the unique index is on raw `Name`. `CreateTag` already
  validates non-empty + `ValidateEntityName`. Match the lookup to whatever the
  DB collation enforces (SQLite default is case-sensitive for the unique index
  unless declared otherwise) so the returned tag is the one that actually
  conflicted.
- Concurrency: two simultaneous creates of the same new name — one wins the
  insert, the other hits the conflict and now selects the winner. Acceptable and
  in fact more correct than today.

---

## Verification commands (whole tier)

Backend (Items 2, 3):

```bash
go test --tags 'json1 fts5' ./...
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
```

Frontend (Items 1, 2 template/JS, 3 dropdown):

```bash
npm run build-js
cd e2e && npm run test:with-server
# full sweep before calling it done:
cd e2e && npm run test:with-server:all   # browser + CLI in parallel
```

Targeted during development:

```bash
go test --tags 'json1 fts5' ./server/api_tests/... -run Tag
go test --tags 'json1 fts5' ./application_context/... -run Tag
cd e2e && npx playwright test 13d-lightbox-tag-prefetch 01-tag
```

## Effort summary (S/M/L per item)

- Item 1 — Stop blanking + prefetch: **M.** Logic localized to two JS files +
  one template + bundle rebuild; the deterministic E2E (route-counting +
  microtask polling) is the fiddly part.
- Item 2 — Autocomplete scaling: **S–M.** B1 is a few index lines (S); B5 is one
  route + template URL swap + two Go tests (S–M). B2 deferred (would be L).
- Item 3 — Duplicate-tag handling: **S.** One context method change, one test
  rewrite, optional one-line dropdown hardening; main cost is auditing existing
  duplicate-error assertions.

## Open questions / decisions to confirm

1. Item 2 B5 scope: route a new `/v1/tags/suggest` (recommended, low risk) vs.
   make `/v1/tags` itself count-skipping (would remove totals from timeline/merge
   UIs — not recommended). Confirm we keep `/v1/tags` counted.
2. Item 2 B5 reach: repoint only the lightbox autocompleters, or all tag
   autocompleters across templates? Minimal = lightbox-only; uniform = all.
3. Item 2: defer B2 (denormalized count) entirely for Tier 0, or scaffold the
   column now? Recommend defer until B1 is profiled.
4. Item 3: does `mr tags create` (CLI) or any doc/test depend on the duplicate
   returning an error? If yes, update those to the idempotent-select contract.
5. Item 3: should the idempotent-select apply to the form path (`/tag/new`) too,
   or only JSON requests? Recommend apply at the `CreateTag` layer (uniform), so
   the form also "creates or selects" — confirm this is acceptable UX.
6. Item 1: only prefetch tag details when a tagging panel is open (recommended,
   avoids needless fetches), or always warm them? Confirm panel-gated.
