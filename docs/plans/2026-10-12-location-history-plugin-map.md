# Interactive Location History map

## Success criterion

The Location History page shows an interactive map of a selected day's recorded
positions and visits. Users can browse days and imports, select a place on the
map, adjust its search radius, and run the existing queries without a page reload.
Every map read is confined to readable source Resources. Existing GET forms and
text answers continue to work without JavaScript.

## Design and tradeoffs

- Ship Leaflet 1.9.4 in the plugin's `public/` directory, with its license.
  There is no existing map library; browser primitives alone would require a
  bespoke map engine. Local assets avoid a runtime script CDN dependency.
- Use OpenStreetMap raster tiles with attribution and the browser's normal
  caching/referrer behavior. Explain on the page that tiles require internet
  access and reveal the viewed map area to the tile provider.
- Add `GET /v1/plugins/location-history/day?date=YYYY-MM-DD&offset=auto&resource=...`.
  Use the existing visible-import guard and monthly column storage. Read one
  local day, including visits starting before it. Return at most 2,000 fixes and
  500 located segments, and explicitly report truncation. Display recorded
  points rather than inventing routes across gaps or between different imports.
- Default to the latest recorded day. Day controls have an explicit time zone,
  and previous/next buttons use civil dates independent of browser time zone.
- Enhance query forms by fetching their existing HTML answer, so textual result
  formatting and validation stay in Lua. Include escaped JSON in result data
  attributes to synchronize the map. Cancel superseded requests and prevent
  late answers from changing the current map. Preserve shareable query URLs.
- Use labeled controls, keyboard map navigation and a button to select the map
  center. Keep a textual, keyboard-accessible list of the day's visits.

## Work and verification

- [x] Run baseline plugin/architecture tests and plugin page E2E tests.
- [x] Add day endpoint and tests for day boundaries, overlap, scoping, validation,
      empty arrays and display caps.
- [x] Add plugin HTML, local map assets, styling and browser interactions.
- [x] Add browser coverage for real import/day display, selection, radius,
      in-place queries, empty data, network errors and accessibility.
- [x] Update built-in plugin documentation and regenerate the plan index.
- [x] Run Go tests, browser and CLI E2E checks, and Postgres checks when available.

## Review

Implemented in the bundled plugin and an identical E2E fixture. Storage, import
semantics and permission grants are unchanged. Leaflet is a pinned local asset
with its upstream license; the main application has no new npm dependency.

Verification so far:

- All Location History Go tests pass, including the scoped day endpoint and
  fixture parity. Day boundaries exclude a visit ending at midnight from the
  following day, and include visits starting in an earlier month.
- Plugin and architecture suites pass on the repository's Go 1.26.4 toolchain.
  The installed Go 1.27.2 changes the standard JSON decoder's error wording,
  breaking the pre-existing `TestEachResourceJSONRefusesOversizedPieces` assertion;
  that test passes with Go 1.26.4. No unrelated test or production code was changed.
- Twelve browser tests pass: real upload/import, day browsing, point selection,
  query updates and matching filters, cancellation, stale day reads, back cache,
  failed tiles and day reads, axe accessibility, forced colors, mobile overflow,
  and HTML queries without JavaScript. The original plugin page tests pass too.
- `./scripts/css-scan-test.sh` passes. No generated main JS/CSS bundle diff.
- Desktop and mobile previews were inspected using synthetic Berlin locations
  on an isolated ephemeral server, including actual OpenStreetMap tiles.
- Review found and fixed stale query status, invalid day edits accepting an old
  response, back-cache teardown, and query-link filters disagreeing with the form.
  The page retains the base layout's single H1; the plugin header uses H2.
- `GOTOOLCHAIN=go1.26.4 go test --tags 'json1 fts5' ./...` passed.
- `cd e2e && npm run test:with-server:all` finished successfully: 2,458 passed,
  5 skipped and 4 flaky tests passing on retry (existing Job Center dismissal,
  global search focus, custom thumbnail focus and image actions focus tests).
  The final plugin-only run passed all 12 tests on the first attempt.
- Postgres could not run because Docker is not installed. All browser runs and
  manual previews used ephemeral instances; no real location data was used.

### Requested review loop

The user requested independent subagent reviews, fixing findings at every
priority and repeating until a review reports no P0/P1 issues.

- Round 1: no P0/P1; P2 default day was empty when the final visit ended at
  midnight. Fixed with a scoped day check that retains genuine midnight fixes
  and visits. A failing Go regression passed after the fix.
- Expanded round 1: no P0/P1; P2 coordinate previews accepted empty components
  as zero and omitted supported geo/degree formats; P2 recorded-fix details
  and selection had no keyboard equivalent; P3 tile outage messages persisted
  after recovery; P3 time-query trip starts used the visit color. Fixed the
  parser, added a native fix selector and button, tracked tile-load batches,
  and aligned trip colors. All 15 browser tests and the Location History Go
  tests passed; new regressions failed before the corresponding fixes.
- Round 2: no P0/P1; P2 trip destinations still had no keyboard selection when
  they lacked a corresponding visit or fix. Added separate start/destination
  controls and a regression using a trip with no recorded fixes. All 16 browser
  checks passed after the fix.
- Round 3: no P0/P1; the canvas audit identified P2 selection of time-query fixes
  and trip endpoints outside the displayed day, and P3 invalid bookmarked
  queries preventing the day from fitting. Made keyboard selection part of the
  answer-dot renderer, with shared place controls and atomic layer/list cleanup.
  Initialization now checks actual answer geometry. Both new regressions failed
  before the fixes. All 18 browser regressions passed after the fixes.
- Round 4: zero P0, P1, P2 or P3 findings. Reviewed all repairs, including the
  general keyboard-equivalence rule for every canvas popup, answer lifecycle
  cleanup, and initial-query geometry. In total, fixed five P2 and three P3
  findings; the stopping condition is satisfied.
- Final browser/CLI run: 2,466 passed, 5 skipped and 4 unrelated flaky tests
  passed on retry (custom thumbnail focus, image actions focus, video seek,
  and global search focus). All 18 final map regressions passed on the first run.
- The concurrent CSS scan caused one Go architecture scan to see a temporary
  probe disappear. The scan itself passed; rerunning Go after CSS completed
  passed the full Go suite (`GOTOOLCHAIN=go1.26.4 go test --tags 'json1 fts5' ./...`).
  Postgres remains unavailable because Docker is not installed.
