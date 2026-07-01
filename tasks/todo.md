# Tier 3 — Context-Aware Suggested Tags Row

Branch: `feat/lightbox-tagging` (continuing). Plan: `tasks/tier3-suggested-tags.md`.

## Backend (TDD: red → green)
- [x] Write failing `application_context/resource_suggest_test.go` (rank, exclude already-applied, cap, empty-similarity, scoped-principal) — GREEN
- [x] Write failing `server/api_tests/suggested_tags_test.go` (200/400/404, RBAC, empty) — GREEN
- [x] `SuggestedTag` DTO + `ResourceSuggestionReader` interface in `server/interfaces/resource_interfaces.go` (DTO lives in interfaces to avoid import cycle)
- [x] `GetSuggestedTags(id, limit)` in new `application_context/resource_suggest_context.go` (consts: maxSimilar=50, default limit=8, wSim=0.6, wGroup=0.4)
- [x] `GetSuggestedTagsHandler` in `server/api_handlers/resource_api_handlers.go`
- [x] Register `GET /v1/resource/suggestedTags` via `scopedAPI` in `server/routes.go`
- [x] Register OpenAPI route in `server/routes_openapi.go`; regenerated `openapi.yaml` (valid)
- [ ] Go tests green (sqlite); postgres tests green

## Frontend (TDD: red → green)
- [x] Write failing `e2e/tests/13g-lightbox-suggested-tags.spec.ts` (13d was taken) — GREEN (4/4)
- [x] Suggested-tags state/methods in `src/components/lightbox/quickTagPanel.js`
- [x] Chip row + Shift+digit bindings in `templates/partials/lightbox.tpl`
- [x] `npm run build-js` + full `npm run build`; E2E spec green

## Verification gate
- [x] `go test --tags 'json1 fts5' ./...` — all pass
- [x] postgres `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1` — pass
- [x] `cd e2e && npm run test:with-server:all` — 1572 passed, 3 flaky (all retried green; see below)
- [x] `cd e2e && npm run test:with-server:a11y` — 171 passed, 1 flaky (crop, unrelated)
- [x] `cd e2e && npm run test:with-server:postgres` — 1572 passed, 2 flaky (known/unrelated)

## Review

**Backend.** New read endpoint `GET /v1/resource/suggestedTags?id=` (scopedAPI, fail-closed
404 on out-of-subtree id). Ranking in `application_context/resource_suggest_context.go`:
unions tags from perceptual-hash-similar resources (cap 50) and the owner group's popular
tags, blended `0.6*simNorm + 0.4*groupNorm`, excludes already-applied, cap 8, deterministic
tiebreak (name then id). DTO `interfaces.SuggestedTag` lives in `interfaces` (not
`application_context`) to avoid the import cycle — same precedent as `interfaces.MetaKey`.
OpenAPI route + regenerated spec (valid). No N+1 (GetSimilarResources preloads Tags;
GetPopularResourceTags is one grouped query).

**Frontend.** `quickTagPanel.js`: `suggestedTags` state + per-resource cache + monotonic
`_suggestedReq` stale-guard; `fetchSuggestedTags` (mirrors fetchResourceDetails), reused
optimistic `applySuggestedTag`, `handleSuggestedTagKeydown` (Shift+1..8, keyed on
`event.code` since a shifted digit reports punctuation in `event.key`). Triggered on panel
open and on resource change (cleared first so no stale flash). Chip row above the tab bar in
`lightbox.tpl`, `role="list"`, per-chip `aria-label` + `⇧N` kbd hint.

**Regression caught & fixed by the full sweep:** the suggested row's `<ul>` originally used
`flex flex-wrap gap-2` — the exact `.flex.flex-wrap.gap-2` selector ~12 lightbox specs use to
target the tag-pills container. When a resource had suggestions, that selector matched two
elements → intermittent strict-mode `toBeVisible()` violation (13-lightbox:792 flaky).
Fixed by using `gap-1.5` so the suggested row stays off that selector. Verified: 43/43
lightbox specs green twice.
