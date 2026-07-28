# Image Similarity v2: better hashes, DB-native matching, live thresholds

> **Status (2026-07-02): Phases 0–5 implemented.** Phase 6 (legacy retirement) is
> intentionally deferred — it is gated on the mahlayf 2.18M backfill completing in
> production. See the "Implementation review" section at the bottom for what shipped.


Scope: items 1, 2, 3 from the similarity review (2026-07-02). Constraint: the mahlayf
deployment has 2.18M images, so no phase may require a large one-shot migration. Every
schema change is additive (nullable columns, new indexes that grow with the backfill),
and the 2.18M re-hash runs as incremental background batches using the same pattern as
the existing string-to-int hash migration in `hash_worker/worker.go`.

## Target design (summary)

- New hash engine: `corona10/goimagehash` pHash (64-bit, DCT) as the primary hash,
  with proper interpolation. Inputs normalized first: EXIF orientation applied
  (orientation tag read via goexif, transform via the existing `bild` dependency),
  alpha flattened onto white, flat images detected by pixel variance and excluded.
- `image_hashes` gains `hash_version`, `p_hash_int`, `p_chunk0..3`, `status`.
  v2 rows keep writing the legacy imgsim dHash/aHash into the existing columns
  (cheap once decoded), so v1-vs-v2 comparisons keep working during the transition.
- Matching becomes DB-native: pigeonhole prefilter on 4x16-bit pHash chunks
  (4 indexed int columns), candidates verified with popcount in Go. Replaces the
  100k LRU cache, which silently misses pairs beyond its capacity at 2.18M images.
- Pairs are stored up to a fixed max pHash distance of 11; the runtime threshold
  (default 10) filters at read time, so threshold changes are instant and need no
  recompute. Chunk math: distance <= 11 over 4 chunks guarantees one chunk within
  distance 2 (worst case 3+3+3+2); radius-2 enumeration of a 16-bit chunk is 137
  values, 548 across 4 chunks, roughly 18k candidate rows per lookup at 2.18M rows.
  Storing to distance 16 would need radius 4 (about 10k values, 370k candidates per
  image), which is why 11 is the ceiling.
- `resource_similarities` gains nullable `p_distance` and `a_distance`. Existing
  `hamming_distance` stays as the legacy dHash distance. Read path sorts and filters
  on `COALESCE(p_distance, hamming_distance)`.

## Phase 0: schema groundwork (additive only, no behavior change)

- [ ] `models/image_hash_model.go`: add `HashVersion *int`, `PHashInt *int64`
      (bit-reinterpreted uint64, same convention as `DHashInt`), `PChunk0..PChunk3 *int32`,
      `Status string` (values: `""`/`ok`, `failed`, `flat`). Individual indexes on each
      chunk column. NULL `HashVersion` means legacy v1 row.
- [ ] `models/resource_similarity_model.go`: add nullable `PDistance *uint8`,
      `ADistance *uint8`.
- [ ] Helper methods: `GetPHash()`, chunk splitter `SplitChunks(uint64) [4]uint16`,
      and radius-2 chunk enumeration `ChunkNeighbors(uint16, radius int) []uint16`
      in `hash_worker` with property tests (every value within Hamming distance r of
      the chunk is enumerated, count matches C(16,0..r)).
- [ ] Verify AutoMigrate on SQLite and Postgres adds columns without rewriting
      `image_hashes` (additive nullable columns only, no defaults with backfill).
- [ ] Tests: model unit tests; run Go suite. No behavior change to assert beyond that.

Risk: near zero. Ships dark.

## Phase 1: v2 hash computation on new uploads (dual-write, matching unchanged)

- [ ] Add deps: `github.com/corona10/goimagehash`, `github.com/rwcarlsen/goexif`
      (orientation tag only). Keep `imgsim` for the legacy dual-write.
- [ ] New `hash_worker/hash_v2.go`: decode once, then
      1. apply EXIF orientation (JPEG only; transform with `bild`),
      2. flatten alpha onto white,
      3. compute grayscale variance on the downsampled grid; below epsilon => `flat`,
      4. compute goimagehash pHash + aHash, and legacy imgsim dHash/aHash from the
         same decoded image.
- [ ] `hashAndStoreSimilarities` writes v2 rows: legacy columns as today, plus
      `hash_version=2`, `p_hash_int`, chunks, `status` (`ok` or `flat`).
      `markResourceFailed` sets `status=failed`.
- [ ] Legacy matching path (LRU cache + dHash threshold) continues unchanged, so new
      uploads still match old images. No v2 matching yet.
- [ ] Fixture-based unit tests (TDD: write red first): EXIF-rotated JPEG pair hashes
      equal after normalization; transparent PNG matches its white-flattened JPEG;
      recompressed JPEG within small pHash distance of original; flat and near-flat
      images marked `flat`; GIF/WebP still decode.
- [ ] Run Go suite + browser/CLI E2E (rebuild `./mahresources` binary first).

Risk: low. New columns only ever written, never read for matching.

## Phase 2: v2 chunk-index matching on the write path

- [ ] `findSimilaritiesV2(resourceID, pHash, aHash)`: candidate query as UNION of 4
      selects, one per chunk column, `p_chunk_i IN (<radius-2 neighbor list>)`, with
      inline integer literals (avoids SQLite bind-variable limits), excluding
      `status IN ('failed','flat')`. Verify candidates in Go with popcount; store
      pairs with `p_distance <= 11` (constant `MaxStoredPDistance = 11`), populating
      `p_distance`, `a_distance`, and legacy `hamming_distance` (legacy hashes exist
      on both sides by construction).
- [ ] Pair upsert: `OnConflict DoUpdate` on the pair unique index to fill
      `p_distance`/`a_distance` when the legacy path inserted the row first.
- [ ] New uploads now run BOTH paths: legacy (matches v1 rows via cache) and v2
      (matches v2 rows via chunk index). Dedup handled by the upsert.
- [ ] EXPLAIN QUERY PLAN check on SQLite and Postgres: candidate query must hit the
      chunk indexes, no full scan of `image_hashes`.
- [ ] Tests: unit tests for candidate enumeration + verification; api_tests seeding
      v2 rows and asserting pair rows with distances; postgres run.

Risk: moderate (query correctness). Gated by `hash_version=2` rows only; v1-only
deployments see no change.

## Phase 3: incremental backfill of existing rows (the 2.18M re-hash)

- [ ] New worker batch task (priority after string-hash migration, before new-resource
      hashing): select `image_hashes` rows `WHERE hash_version IS NULL` joined to
      hashable resources, `ORDER BY resource_id DESC` (newest first, most user-visible
      benefit early), batch = `config.BatchSize`. For each: re-decode, compute v2
      fields, update the row in place, then run `findSimilaritiesV2`. Rows whose file
      is missing/corrupt get `status=failed`, `hash_version=2` (no infinite retry).
      Previously-failed empty rows (`d_hash_int IS NULL AND d_hash = ''`) are included,
      giving them exactly one retry.
- [ ] Resumable by construction (version predicate); safe to restart the server at any
      point. No transaction spans more than one row update.
- [ ] Pace control: reuse `hash-batch-size`/`hash-poll-interval`; document expected
      duration (500/min is roughly 3 days for 2.18M; decode dominates, 4 workers).
      Add runtime setting `hash_backfill_paused` (bool) so mahlayf can pause the
      backfill without disabling the whole worker.
- [ ] Admin stats: extend `SimilarityInfo` (admin_context.go) with counts by
      `hash_version` and `status` so backfill progress is visible on the admin page,
      plus progress logs via the existing `logProgress`.
- [ ] Tests: worker test seeding v1 rows and asserting incremental conversion +
      new pair rows; pause-setting test; full suites (Go, E2E browser+CLI, Postgres).

Risk: operational, not correctness. Backfill I/O is the cost; pacing and pause
control keep it in budget. LRU/legacy path still active, so nothing regresses if the
backfill takes weeks.

## Phase 4: read-time thresholds and confidence surfacing

- [ ] `getSimilarResourcesLimited`: add
      `WHERE COALESCE(p_distance, hamming_distance) <= <runtime threshold>` and when
      the aHash threshold setting is nonzero
      `AND (a_distance IS NULL OR a_distance <= <ahash threshold>)`;
      `ORDER BY COALESCE(p_distance, hamming_distance) ASC`. Threshold read per
      request from runtime settings, so changes apply instantly with no recompute.
- [ ] Write path stores everything up to distance 11 unconditionally (the runtime
      threshold no longer gates writes for v2; legacy write path keeps its gate until
      Phase 6). Update runtime-setting descriptions in `runtime_setting_spec.go`:
      the similarity threshold is now a read-time filter, valid range 0..11 for v2.
- [ ] Surface distance in the resource detail similar-resources section as a
      confidence tier (0-2 near-certain duplicate, 3-10 similar). Verify the
      suggested-tags path (`resource_suggest_context.go`) inherits the filter via
      `getSimilarResourcesLimited` and still degrades gracefully.
- [ ] Tests: TDD an E2E test that changes the runtime threshold and observes the
      similar-resources list change without rehash; api_tests for filter + ordering;
      a11y check on the tier UI; Postgres run.

Risk: low. Pure read-path change over data written in Phases 2-3.

## Phase 5: admin jobs (recompute pairs, retry failed)

- [ ] "Recompute similarities" admin action: background job that deletes pairs where
      both endpoints are v2 and re-runs `findSimilaritiesV2` per v2 row in batches
      (DB-only, no image decode). For algorithm/constant changes. Runs on the worker's
      batch loop machinery with progress logging; guard against concurrent runs.
- [ ] "Retry failed hashes" admin action: reset `status=failed` rows to
      `hash_version=NULL, status=''` so the Phase 3 backfill task picks them up.
- [ ] Per-resource "rehash" already exists via hash invalidation on version change
      (context.go InvalidateHash path); verify it produces v2 rows and pairs now.
- [ ] Expose both actions on the admin page and via `/v1/admin/...` + `mr` CLI;
      update `<group>_help/*.md` docs (docs lint runs in CI).
- [ ] Tests: api_tests for both jobs; CLI E2E specs; full suites.

Risk: low. Both jobs reuse Phase 2/3 machinery.

Note: `RecomputeV2Pairs` intentionally prunes dHash-only pairs between two v2
rows (rows the legacy LRU path stored with `p_distance IS NULL` because their
pHash distance exceeds 11 — dHash false positives that v2 rejects). The legacy
matching path still runs for every new upload until Phase 6, so it recreates
some of these pairs over time; results therefore differ slightly depending on
whether a recompute has run. Phase 6 removes the asymmetry.

## Phase 6: legacy retirement (after mahlayf backfill completes)

Gate: admin stats show `hash_version=2` for all rows (minus permanently failed).

- [ ] Stop computing imgsim legacy hashes for new uploads; drop the legacy matching
      path, LRU cache, `warmCache`, and `hash-cache-size` plumbing (flag/setting
      deprecation note in CLAUDE.md config table). This also removes the
      recompute/live-path asymmetry around dHash-only v2-v2 pairs (see Phase 5 note).
- [ ] Remove the `imgsim` dependency and the zero-hash solid-color guard (superseded
      by `status=flat`); keep `AreSimilar` only for whatever the legacy read of old
      pairs still needs, or fold entirely into v2.
- [ ] Keep legacy columns and old pair rows (harmless, avoids any rewrite); optionally
      stop selecting them.
- [ ] Full suites incl. Postgres; verify admin stats and similar-resources UX on an
      ephemeral seed of a real DB (`-memory-db -seed-db`).

Risk: low, but only safe once backfill coverage is confirmed on mahlayf.

## Explicit non-goals

- No pair-table rewrite and no re-keying of `resource_similarities`.
- No mirror/rotation-invariant matching, no video/PDF thumbnail hashing, no >64-bit
  hashes (all possible later; `hash_version` gives them a clean upgrade path).
- No change to the export/import archive contract (hashes are derived data and are
  not part of the manifest).

## Verification checklist (applies at the end of every phase)

- [ ] `go test --tags 'json1 fts5' ./...`
- [ ] Rebuild binary, then `cd e2e && npm run test:with-server:all`
- [ ] Postgres: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
      and `npm run test:with-server:postgres` (phase ends that touch DB queries)
- [ ] For phases 3+: manual smoke on an ephemeral instance seeded from a large DB

## Implementation review (2026-07-02, Phases 0–5)

What shipped, by area:

- **Schema (Phase 0):** `image_hashes` gained `hash_version`, `p_hash_int`,
  `p_chunk0..3` (each individually indexed), `status`. `resource_similarities`
  gained nullable `p_distance`, `a_distance`. Helpers `SplitChunks`,
  `ChunkNeighbors` (radius-2), and the pigeonhole invariant are property-tested
  in `hash_worker/chunks_test.go`. Additive AutoMigrate verified on SQLite and
  Postgres (api_tests run green under both).
- **v2 hashing (Phase 1):** `hash_worker/hash_v2.go` — decode once, EXIF
  orientation (goexif + bild), alpha flattened onto white, flat detection by
  grayscale variance, goimagehash pHash. Legacy imgsim dHash/aHash are dual-written
  for v1↔v2 comparison. The v2 goimagehash aHash was intentionally NOT computed:
  `a_distance` is derived from the persisted imgsim aHash, and there is no v2-aHash
  column, so a second hash per image would be wasted work at 2.18M scale.
- **DB-native matching (Phase 2):** `FindSimilaritiesV2` — UNION of four indexed
  chunk lookups with inlined radius-2 neighbour literals, popcount verification in
  Go, pairs stored up to `MaxStoredPDistance = 11` via an upsert that fills v2
  distances onto legacy-inserted rows. Index usage asserted via EXPLAIN QUERY PLAN.
- **Backfill (Phase 3):** `hash_worker/backfill.go` — incremental, newest-first,
  resumable (hash_version IS NULL cursor), pausable via the new
  `hash_backfill_paused` runtime setting. Runs between string-hash migration and
  new-resource hashing. Admin `SimilarityInfo` extended with v1/v2/flat/failed/
  v2-pair counts, surfaced on the admin overview.
- **Read-time thresholds (Phase 4):** `getSimilarResourcesLimited` filters and
  orders by `COALESCE(p_distance, hamming_distance)`, with the aHash secondary
  filter applied only when its threshold is nonzero (legacy NULL a_distance always
  passes). Threshold read per request → instant, no recompute. Distance surfaced
  as a confidence tier ("Near-certain duplicate" / "Similar") on the resource
  detail page. Similarity-threshold setting range narrowed to 0..11.
- **Admin jobs (Phase 5):** `RecomputeV2Pairs` (background job via the shared job
  manager, guarded against concurrent runs) and `RetryFailedHashes`. Exposed at
  `POST /v1/admin/similarity/{recompute,retry-failed}` (admin-gated), on the admin
  overview page, and via `mr admin similarity {recompute,retry-failed}` with docs.
  Per-resource rehash (OnResourceFileChanged) already flows through the v2 write
  path, so it produces v2 rows and pairs with no extra code.

Verification: `go test --tags 'json1 fts5' ./...` green; Postgres
`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...` green;
E2E browser + CLI run at end of the work.
