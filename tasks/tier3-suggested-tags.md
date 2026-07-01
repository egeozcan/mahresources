# Plan — Tier 3: Context-Aware Suggested Tags Row

## Scope & why this is the highest-ceiling feature
A new read endpoint `GET /v1/resource/suggestedTags?id=<resourceId>` that unions and
ranks tag suggestions from two signals the app already computes but never surfaces:
(a) tags on perceptual-hash-similar resources, and (b) the most common tags in the
resource's owner group. The lightbox shows a one-tap "Suggested" chip row above the
numpad tab bar, fetched alongside resource details, cached per resource, refreshed on
navigation, and keyboard-applyable.

This is the marquee feature because the expensive half of the work is done: a populated,
indexed `resource_similarities` table and a tag-usage ranking primitive already exist. We
are wiring high-value latent data into the one surface (the lightbox quick-tag panel)
where rapid tagging happens. Net new code is a thin ranking method plus one chip row.

## Data sources that already exist
- Perceptual-hash similarity: `GetSimilarResources(id uint) ([]*models.Resource, error)`
  in `application_context/resource_crud_context.go:28-109`. Returns resources ordered by
  Hamming distance ascending; **preloads `Tags` and joins `Owner` in one query** (no N+1,
  lines 81-87). Falls back to exact-hash match when no precomputed rows exist (lines
  60-78). Empty slice when neither yields anything.
- Precomputed table `resource_similarities`, queried raw (UNION ALL on both id columns,
  `resource_crud_context.go:36-41`). Model `models/resource_similarity_model.go:5-11`:
  `ResourceID1 < ResourceID2`, `HammingDistance uint8`, indexed
  `idx_sim_r1_dist` / `idx_sim_r2_dist` (composite id+distance) — **distance-ordered
  lookups are index-backed; confirmed.** Similarity threshold is the worker's
  `-hash-similarity-threshold` (default 10); reads here just consume whatever rows the
  worker stored.
- Tag-usage ranking primitive: `GetPopularResourceTags(query *ResourceSearchQuery)
  ([]PopularTag, error)` in `application_context/resource_bulk_context.go:591-604`. Joins
  `resource_tags` + `tags`, `count(*) DESC`, `Limit(20)`, runs through
  `database_scopes.ResourceQuery(query, true, ctx.db)`. `PopularTag{Name, Id, Count}` is
  defined at `application_context/context.go:30-34`. Passing `ResourceSearchQuery{OwnerId:
  groupID}` filters to that owner group via `resource_scope.go:125-126`
  (`resources.owner_id = ?`). (The `most_used_` sort prefix in
  `models/database_scopes/tag_scope.go:24-48` is the global-ranking analogue; we use the
  owner-scoped `GetPopularResourceTags` instead because it filters by owner group.)
- Owner group: `models.Resource` has `owner_id` (the scope column,
  `application_context/scoping.go:54-62`) and an `Owner` association. Read the resource
  via `GetResource`/`GetResourceByID` (`resource_crud_context.go:15-26`) to obtain
  `OwnerId` and its existing `Tags`.

## Backend design

### New endpoint
- Route: `GET /v1/resource/suggestedTags`, query param `id` (uint, required), registered
  in `server/routes.go` next to the other resource reads (~line 456) via
  `scopedAPI(appContext, api_handlers.GetSuggestedTagsHandler)`.
- Response shape (slim DTO so the frontend can hand it straight to `_batchToggleTags`,
  which consumes `{ID, Name}` — `quickTagPanel.js:414-489`):
  ```json
  { "suggestions": [ { "ID": 12, "Name": "sunset", "score": 1.0, "sources": ["similar","group"] } ] }
  ```
  `score`/`sources` are advisory (useful for tooltips/telemetry); the frontend only needs
  `ID` and `Name`. Always HTTP 200 with a possibly-empty `suggestions` array; 404 only
  when the resource itself is not visible/not found; 400 on missing/zero `id`.
- Business logic lives in a new context method (keeps ranking unit-testable and matches
  the repo's "logic in application_context" pattern), e.g.
  `GetSuggestedTags(resourceId uint, limit int) ([]SuggestedTag, error)` in a new file
  `application_context/resource_suggest_context.go`. The handler
  (`server/api_handlers/resource_api_handlers.go`, modeled on `GetResourceHandler`
  lines 63-82) just decodes `id`, calls the method, encodes JSON.

### Ranking algorithm
1. `res, err := ctx.GetResource(id)` — used for the exclude-set (`res.Tags`) and
   `res.OwnerId`. If err (record not found / out-of-subtree under scope) → return the
   error so the handler emits 404. **This is the primary access guard.**
2. Source A (similar): `sims, _ := ctx.GetSimilarResources(id)`; cap to the first
   `maxSimilar` (const, e.g. 50) since it is distance-ordered. For each similar resource's
   preloaded `Tags`, accumulate `freqSim[tagID]++` and capture name. `scoreSim = freqSim /
   maxFreqSim` (normalize to [0,1]).
3. Source B (group): if `res.OwnerId != nil`, `pop, _ :=
   ctx.GetPopularResourceTags(&ResourceSearchQuery{ResourceQueryBase:{OwnerId:
   *res.OwnerId}})`. `scoreGroup = count / maxCount` (normalize to [0,1]).
4. Union by tag id: `final = wSim*scoreSim + wGroup*scoreGroup`, constants `wSim=0.6`,
   `wGroup=0.4` (similar tags are more contextually specific than group-popular tags).
   Track which sources contributed for the `sources` field.
5. Exclude any tag already on the resource (`res.Tags` id set) — the "exclude
   already-applied" requirement. Also dedupe across sources.
6. Sort by `final` DESC, tiebreak by `Name` ASC for determinism; truncate to `limit`
   (default const `8`; the row renders 5-8). Empty similarity + no owner → `[]`.

### RBAC scoping (fail-closed)
- Wrap with **`scopedAPI`** (`server/request_scope.go:87-102`), exactly like every other
  resource read in `routes.go:456-467`. `scopedAPI` runs the handler against
  `scopedCtx(appCtx, r)` = `appCtx.WithPrincipal(...)` (`scoping.go:97-128`), whose `db`
  carries the subtree `scopeFilter`.
- Three layers of confinement for a group-limited user/guest:
  1. `GetResource(id)` runs through the scoped `db`; the query callback adds
     `resources.owner_id IN (subtree)` (`scoping.go:231-246`), so an out-of-subtree (or
     owner-less) resource yields `ErrRecordNotFound` → handler 404. Fail-closed: an
     unresolvable subtree produces an empty allow-list → `WHERE 1=0` (`scoping.go:241-243`).
  2. `GetSimilarResources` raw query only returns candidate **ids**; the final
     `Find(&resources)` is a GORM query on `resources`, so the scope callback filters out
     any similar resource outside the subtree before its tags are read
     (`resource_crud_context.go:81-87`).
  3. `GetPopularResourceTags` runs `Table("resources")` with the scope callback active, so
     its `owner_id IN (subtree)` AND `owner_id = ownerId` yields rows only when the owner
     group is in the caller's subtree.
- Net effect: a confined principal can only ever receive suggestions derived from
  resources inside its subtree, and is 404'd on out-of-subtree ids. Tags themselves are
  global (not a scoped table, `scoping.go:54-62`), which is correct — only the *resources*
  that contribute tags are confined. No `denyScopedPrincipal` needed (unlike plugin/import
  endpoints): subtree-confined suggestions are coherent.

### Performance (avoid N+1; cap; cache)
- No N+1: `GetSimilarResources` preloads `Tags` in a single query; `GetPopularResourceTags`
  is one grouped/aggregated query (`Limit(20)`).
- `resource_similarities` distance lookups are index-backed (`idx_sim_r1_dist`,
  `idx_sim_r2_dist`); confirmed in the model.
- Cap the similar set processed (`maxSimilar` const, e.g. 50) so tag aggregation is bounded
  even for resources with huge similarity fan-out (deployments hit millions of resources).
- Result capped to `limit` (5-8). Frontend caches per resource (see below); no server-side
  cache needed for v1.

### TDD: Go tests first
- `application_context/resource_suggest_test.go` (unit, red first):
  - [ ] Seed group + resources with overlapping tags + `ResourceSimilarity` rows
    (pattern: `server/api_tests/resource_delete_orphan_similarities_test.go:84-96` shows
    creating `models.ResourceSimilarity{ResourceID1,ResourceID2,HammingDistance}` with
    `ResourceID1 < ResourceID2`). Assert ranking order, exclusion of already-applied tags,
    cap honored.
  - [ ] Empty-similarity case: a resource with no similarity rows and no exact-hash match
    → suggestions come only from the owner group (or `[]` when also owner-less).
  - [ ] RBAC-confined case: build a scoped ctx via
    `ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, ScopeGroupID: &rootID})`
    (pattern: `application_context/scoping_test.go:67`). Assert (i) querying an
    out-of-subtree resource returns an error, and (ii) suggestions for an in-subtree
    resource never include tags sourced from out-of-subtree resources/groups.
- `server/api_tests/suggested_tags_test.go` (api, red first; `SetupTestEnv` already
  AutoMigrates `ResourceSimilarity` — `api_test_utils.go:68`):
  - [ ] `GET /v1/resource/suggestedTags?id=` → 200 with ranked `suggestions`.
  - [ ] Missing/zero `id` → 400; nonexistent id → 404.
  - [ ] RBAC: using `roleBearer` + `setupAuthEnv` (`server/api_tests/authz_test.go:13-34`),
    a guest/group-limited user gets 404 for an out-of-subtree resource and only in-subtree
    suggestions for an in-subtree one.
  - [ ] Empty-similarity over HTTP returns 200 + group-only (or empty) array.

## Frontend design

### Where the row lives, one-tap + keyboard, per-resource cache, refresh on nav
- Store wiring: add state/methods in `src/components/lightbox/quickTagPanel.js`, spread into
  the store alongside the existing panels (`src/components/lightbox.js:19-20,104-105`).
  New state: `suggestedTags: []`, `suggestedTagsLoading: false`,
  `_suggestedCache: new Map()`, `_suggestedReq: 0` (monotonic token).
- `fetchSuggestedTags(id, forceRefresh=false)`: mirror `fetchResourceDetails`
  (`editPanel.js:165-219`) — per-resource cache, `_suggestedReq` guard against stale
  responses, `abortableFetch('/v1/resource/suggestedTags?id='+id)`, only commit when
  `getCurrentItem()?.id === resourceId`.
- Triggers (load alongside details, refresh on nav):
  - In `openQuickTagPanel` after the existing `fetchResourceDetails(undefined, true)`
    (`quickTagPanel.js:246`).
  - In `onQuickTagResourceChange` (`quickTagPanel.js:493-500`), which already fires on
    navigation via `onResourceChange` (`editPanel.js:221-250`, calls
    `onQuickTagResourceChange` at 249).
  - Clear `suggestedTags = []` immediately on resource change so the row never shows the
    previous resource's stale chips while refetching.
- `applySuggestedTag(tag)`: `await this._batchToggleTags([{ID: tag.ID, Name: tag.Name}],
  'add')` (`quickTagPanel.js:414`), then optimistically drop it from `suggestedTags`. This
  reuses the existing optimistic update, `detailsCache` write, `needsRefreshOnClose`, and
  `this.announce(...)` live-region announcement (`quickTagPanel.js:457-470`). After apply,
  `isTagOnResource` will hide/strike it on subsequent renders.
- Row placement: a horizontal chip row in `templates/partials/lightbox.tpl` just above the
  tab bar inside the `!isExpanded()` block (insert before line 495), guarded by
  `x-show="$store.lightbox.suggestedTags.length"`. `role="list"`, each chip a `<button>`
  with `@click="$store.lightbox.applySuggestedTag(tag)"`, `aria-label="Apply suggested tag
  {name}"`, and a `<kbd>` showing the shortcut. a11y matters here (CLAUDE.md): focusable
  buttons, `aria-label`s, announcements already handled by `announce`.
- Keyboard: numpad `1-9` are already bound to slots (`lightbox.tpl:532-` /
  `handleSlotKeydown`), so bind **Shift+1..Shift+8** to apply suggestion N, added to the
  root keydown handlers near the existing tab-switch bindings
  (`lightbox.tpl:68-72`), gated by `quickTagPanelOpen && canPanelShortcut() &&
  !$event.repeat`. Chips display `⇧1`..`⇧8`.

### TDD: failing Playwright E2E first
- New spec `e2e/tests/13d-lightbox-suggested-tags.spec.ts` (or extend `13-lightbox.spec.ts`).
  Drive via the **playwright-cli skill** for UI; seed via the API client helper.
- Seed deterministically through the **group-common-tags source** (perceptual similarity
  needs real image hashing and the background worker, which is not deterministic in E2E —
  cover that path in the Go tests instead): create a group, create several resources in it
  carrying shared tags, then open a tag-less resource from that group in the lightbox.
  - [ ] (red) Assert the Suggested row renders the group's common tags.
  - [ ] Click a chip → the tag is applied (appears in the resource's tags; verify via API).
  - [ ] Press `Shift+1` → applies the first suggestion; live-region announces it.
  - [ ] Navigating to the next item refreshes the row (different suggestions / cleared).

## Implementation steps
- [ ] Write failing `application_context/resource_suggest_test.go` (rank, exclude, cap,
      empty-similarity, scoped-principal).
- [ ] Write failing `server/api_tests/suggested_tags_test.go` (200/400/404, RBAC, empty).
- [ ] Add `GetSuggestedTags(id, limit)` + `SuggestedTag` DTO in
      `application_context/resource_suggest_context.go`; constants `maxSimilar`,
      default limit `8`, weights `wSim`/`wGroup`.
- [ ] Add `GetSuggestedTagsHandler` to `server/api_handlers/resource_api_handlers.go` and a
      handler-input interface (method set: `GetSuggestedTags`) in
      `server/interfaces/resource_interfaces.go`.
- [ ] Register `GET /v1/resource/suggestedTags` via `scopedAPI` in `server/routes.go`
      (~line 456). Make Go tests green.
- [ ] Register the route in `server/routes_openapi.go` (`registerResourceRoutes`,
      pattern at lines 1042-1052) and regenerate: `go run ./cmd/openapi-gen`.
- [ ] Write failing `e2e/tests/13d-lightbox-suggested-tags.spec.ts`.
- [ ] Frontend: add suggested-tags state/methods to `quickTagPanel.js`; spread is automatic
      via `lightbox.js`. Add the chip row + Shift+digit bindings to `lightbox.tpl`. Run
      `npm run build-js`. Make E2E green.

## Files touched (backend + frontend)
- Backend (new): `application_context/resource_suggest_context.go`,
  `application_context/resource_suggest_test.go`,
  `server/api_tests/suggested_tags_test.go`.
- Backend (edit): `server/api_handlers/resource_api_handlers.go`,
  `server/interfaces/resource_interfaces.go`, `server/routes.go`,
  `server/routes_openapi.go` (+ regenerated `openapi.yaml`).
- Frontend (edit): `src/components/lightbox/quickTagPanel.js`,
  `templates/partials/lightbox.tpl`, rebuilt `public/dist/main.js` (via `npm run build-js`).
- Frontend (new): `e2e/tests/13d-lightbox-suggested-tags.spec.ts`.

## Risks & gotchas
- **Scope leak via raw SQL**: `GetSimilarResources`'s `resource_similarities` query is raw
  (bypasses GORM scope callbacks). Safe here only because the *follow-up* `Find` on
  `resources` is scoped, and we additionally gate on `GetResource(id)` returning visible.
  Do not "optimize" by reading tags directly off the raw similarity ids — that would skip
  the scope callback and leak out-of-subtree tags. Keep aggregation behind the scoped
  GORM `Find`.
- **Similarity may be empty or stale**: the hash worker may not have processed the resource
  (`-hash-worker-disabled`, fresh import). Group source is the always-available fallback;
  endpoint must degrade to `[]` gracefully, never error.
- **Ranking quality**: normalization + weights are heuristic. Keep `wSim`/`wGroup`/caps as
  named constants for easy tuning; expose `score`/`sources` so quality can be inspected.
- **Owner-less resources**: under scope, `owner_id IN (subtree)` excludes NULL owners →
  404 for confined users (acceptable fail-closed); for unscoped users the group source is
  simply skipped.
- **Keyboard clash**: digits 1-9 already toggle slots; must use Shift+digit, and respect
  `canPanelShortcut()` / `$event.repeat` like the existing tab keys.
- **Stale chips on nav**: clear `suggestedTags` on resource change and use the
  `_suggestedReq` token so a late response for the previous resource can't paint.

## Effort (M)
One thin context method + handler/route + OpenAPI entry; one chip row + ~3 store methods +
keybindings. The heavy data (similarities, popular-tag ranking) already exists. Bulk of the
work is the TDD suite (unit + api + RBAC + E2E) and getting normalization/weights right.

## Open questions / decisions
- Confirm `wSim=0.6 / wGroup=0.4`, `limit=8`, `maxSimilar=50` defaults, or expose as flags?
  (Recommendation: hardcoded constants for v1; no flag.)
- Response field casing: return `{ID, Name}` (matches `models.Tag` JSON and what
  `_batchToggleTags` expects). Confirm slim DTO vs. full `models.Tag`.
- Should "already-applied" exclusion also drop tags the user removed this session? (v1: no;
  the per-resource cache + optimistic drop handle the in-session case.)
- Verification gate before done: `go test --tags 'json1 fts5' ./...`; postgres
  `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`;
  `npm run build-js`; `cd e2e && npm run test:with-server:all` and
  `npm run test:with-server:postgres`.
