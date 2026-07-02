# Fix test flakiness (Go + E2E) — active task

Root causes from read-only discovery workflow (8 agents). Root-cause fixes, adversarially
verified, proven under forced worst-case (`workers:1` repeated / Go `-count -race`).

## Tier 1 — content-hash collision (biggest class; fixes documented flakes 23, auto-detect, a11y/17)
`resource_upload_context.go:647` dedupes on GLOBAL SHA1; two specs on one worker uploading the
same `sample-image-N.png` either 409 or silently resolve to the *other* spec's resource. Worker
server+DB survive retries → can become deterministic-red on retry. Fix = every upload gets a
unique appended **ASCII marker** (decoder-safe for PNG/JPEG/GIF/SVG/TXT/MP4; TAR unsafe but never
hits this path). Per-process counter → retry-safe. Exemptions: none real (08's hardcoded SHA1 is
dead cleanup). CLI uploads out of scope (deterministic on their serial per-worker server).

- [x] `e2e/helpers/unique-upload.ts`: uniqueMarker/uniquifyBuffer/uniqueAssetFile
- [ ] `api-client.ts createResource`: uniquify by default (+ `exactBytes` opt-out)
- [ ] `pages/ResourcePage.ts createFromFile`: uniquify setInputFiles
- [ ] `08-resource` L57 uniquify + remove dead hardcoded-SHA1 cleanup
- [ ] `14-resource-versioning` L53 uniquify (create-form only)
- [ ] `auto-detect-category` L67 uniquify (browser upload test)
- [ ] PROVE: build; run 23, auto-detect, a11y/17, 13-family under workers:1 ×N

## Tier 1 — other deterministic/ordering flakes
- [ ] `100-global-search`: scope `.first()` to `hasText: GS100ResCat <id>`
- [ ] Serial-CRUD retry poisoning 01–07: idempotent afterAll + `.first()`/exact verify locators
- [ ] Lightbox position≠id: `13-lightbox` L859, `13d` L185 → select by `data-resource-id`
- [ ] Go `lib/id_lock_test.go:295`: deterministic winner via channels
- [ ] Go `runtime_settings_test.go:314`: `ORDER BY created_at asc, id asc`
- [ ] Go `timeline_test.go:49`: pin created_at + anchor to fixed UTC
- [ ] Go `resource_context_test.go:288`: widen TimeoutReader idle margin

## Tier 2 — surgical waitForTimeout RACE (high/med conf)
- [ ] 13-lightbox L949, 36 L61, 38 L61, 62 L45, 67 L53, c15-bh021 L58, mrql L113,
      schema-editor-meta-switch L135/169/223/251, timeline L79/L198, a11y/08-seven-fixes L121/210,
      08 L85, auto-detect L80

## Tier 3 — evaluate (only if quick + safe)
- [ ] paste-upload / remote-download retry uniqueness — assess
- [ ] SKIP: CLI (not flaky), schema-search-fields (low conf), search_context.go (no active flake)

## Verify
- [x] Go `-race -count=20` id_lock; `-count=10` timeout-reader/audit/timeline — all green
- [x] `go test --tags 'json1 fts5' ./...` — 0 failures
- [x] Collision fix PROVEN: 22/23/14/auto-detect/16 together, workers:1 retries:0 x3 → 93/93
- [x] My E2E fixes: 13/13d/100/08/auto-detect, workers:1 retries:0 x2 → 106 pass
- [x] Agent fixes: 01–07 + waitFor batch, workers:1 retries:0 x2 → 304 pass
- [x] Postgres: timeline test -count=3 → green
- [x] Full E2E browser + CLI + auth (run #2): 1588 passed, 0 flaky, 0 failed
- [x] a11y regression (my marker exposed compare.tpl orange-600 contrast) → text-amber-700, c17 8/8
- [x] Postgres: collision+ordering specs 101/102 (only pre-existing 100 PG-FTS residual)
- [x] 100 SQLite wrong-type flake fixed (removed competing category); PG residual pre-existing (documented)
- [x] Update project_known_flaky_e2e memory
- [x] Final full-suite re-run: 1587 passed, 0 failed, 1 flaky (13-lightbox focus-restore)
- [x] Fixed the 1 flaky: read-once document.activeElement → expect.poll (verified 10/10)

## DONE — all confirmed flakes fixed; residuals documented in project_known_flaky_e2e memory

## Follow-up: commit + fix residuals (2026-07-01)
Committed flakiness fixes to master, then addressed the documented residuals.

- [x] Commit flakiness fixes (`4d4dc7d6`) + gitignore SQLite `-shm/-wal` sidecars (`aa95633f`)
- [x] Root-cause the `100-global-search` PG "flake": it was NOT PG-FTS visibility lag (search_vector is
      a synchronous GENERATED STORED column). Real cause probed on real PG: PG's English parser reads a
      hyphen+digit run (`2024-3q` → signed-int `-3` + orphaned `q`) so the split `:*` query misses its
      own row — 273/1000 for the old random `Date.now()-base36` token. Deterministic-per-term.
- [x] Test fix: `100` uses a letters-only token (`9eada1df`) — probe 0/1000, PG spec 16/16.
- [x] Product fix (documented residual): `globalSearch.js` caches non-empty results only (`2150dfbf`).
- [x] Backend fix (user-approved): `fts/postgres.go` builds the prefix/exact tsquery from the raw
      term's own `to_tsvector` lexemes (`to_tsquery('simple', …)`), `ParsedQuery.RawTerm` added
      (`165814fd`). 0/1000, GIN index preserved, no regression. Regression test
      `fts_hyphenated_number_pg_test.go` + parser unit tests.
- [x] 26-paste-upload / 43-resource-from-url: verified retry-safe by dedup semantics (attach-and-return
      existing with original name) — no patch needed.
- [x] Verify: fts unit; full SQLite Go suite; full PG api_tests; search+a11y E2E on SQLite AND PG (44/44 each).
- [x] Memory updated: corrected 100 diagnosis + new `reference_pg_fts_hyphen_tokenization`.

## DONE — residuals fixed and verified on both backends

# Resource search: "Include subgroups" owner filter (2026-07-02)

Add an option to the resource search to widen the OwnerId filter to the whole group
subtree (owner + all descendant subgroups, recursively). Plan approved via plan mode.

- [x] TDD red: 5 API tests in `server/api_tests/resource_owner_subtree_filter_test.go`
      (subtree match + count, exact-match regression, flag-alone no-op, HTTP binding, RBAC intersection)
- [x] `IncludeSubgroups bool` on `ResourceSearchQuery` (binds via gorilla/schema, no handler changes)
- [x] `groupSubtreeCTE` const in `database_scopes/db_utils.go` (recursive CTE, UNION-dedup = cycle-safe)
- [x] Owner filter branch in `resource_scope.go` (`owner_id IN (<subtree>)` when flag set);
      timeline + popular-tags inherit via shared scope
- [x] "Include subgroups" checkbox in `searchFormResource.tpl` under the Owner autocompleter
- [x] CLI: `--include-subgroups` on `mr resources list` and `mr resources timeline`,
      help docs + doctest (doctest needed unique upload bytes to defeat content-hash dedup)
- [x] OpenAPI spec regenerated + validated
- [x] E2E browser spec `103-owner-subtree-filter.spec.ts` (a[title=...] locators avoid
      strict-mode clash with the hidden lightbox header link)

## Review

Verified: new API tests red then green; full SQLite Go suite; full browser+CLI E2E
(1593 passed); CLI E2E rerun with fresh mr binary (314 passed); PG Go tests
(mrql + api_tests); full PG E2E (1594 passed); `mr docs lint` OK; doctests pass
(5 remaining failures are pre-existing missing-plugin-fixture ones).

Drive-by fix: `/admin/overview` a11y color-contrast failure from the storage-stats
feature (e1cf3759) — badge `text-stone-500` on `bg-stone-100` is ~4.2:1, below AA;
bumped to `text-stone-600` in `adminOverview.tpl`, a11y test green.

Follow-up candidate (not done): same option for the note search (`note_scope.go`
has the identical exact-match OwnerId pattern; `groupSubtreeCTE` is ready to reuse).
