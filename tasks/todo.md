# Benchmark and Expose MRQL Query Counts/Plans

Source: `tasks/mrql-performance-report.html`, task 1.

## Agreed boundaries

- [x] Deliver both the benchmark foundation and the runtime explain extension.
- [x] Keep ordinary execution responses unchanged; actual execution measurements belong to the benchmark harness.
- [x] Keep generated-SQL explain available under existing authorization; make native plans opt-in and admin-only.
- [x] Never run `EXPLAIN ANALYZE`; use SQLite `EXPLAIN QUERY PLAN` and PostgreSQL `EXPLAIN (FORMAT JSON)`.
- [x] Explain the authorization-scoped Effective MRQL Query and preserve fail-closed scope behavior.
- [x] Report data-dependent SQL fan-out as execution-shape bounds rather than executing discovery or fabricating statements.
- [x] Use value-redacted, versioned MRQL query-shape fingerprints.
- [x] Treat timing regressions as advisory initially; hard-gate deterministic SQL/result/authorization invariants.

## Phase 1 — Lock down the explain contract (red)

- [x] Add API tests for `nativePlan: true`, admin success, non-admin `403`, and unchanged SQL-only access.
- [x] Add scoped-principal tests proving forced subtree scope remains in generated SQL/native plans and explicit `SCOPE` cannot broaden it.
- [x] Add denied-scope tests requiring `statements: []`, zero statement bounds, the existing warning, and no database planning.
- [x] Add cross-entity LIMIT/OFFSET fidelity tests: each branch uses `offset + limit` and no branch SQL OFFSET.
- [x] Add execution-shape tests for flat, aggregate, bucket fan-out, cross-entity, random conditional counts, and bounded minimum/maximum counts.
- [x] Add fingerprint tests proving stability, value redaction, sensitivity to structural/policy changes, and a versioned external form.
- [x] Add SQLite native-plan tests for structured `EXPLAIN QUERY PLAN` rows and absence of `ANALYZE`.
- [x] Add PostgreSQL native-plan tests for native JSON output and absence of `ANALYZE`.
- [x] Add shared-deadline, cancellation, unsupported-dialect, and atomic multi-statement failure tests.
- [x] Confirm the new tests fail for the expected missing behavior before implementation.

## Phase 2 — Make generated statements faithful (green/refactor)

- [x] Define additive explain response types for query fingerprint and execution shape (`strategy`, planned/min/max statements, data-dependent flag, description).
- [x] Extract the smallest shared cross-entity branch-spec helper used by both execution and explain; preserve checked overflow and execution-policy bounds.
- [x] Keep flat, aggregate, and bucket-key statement construction on the existing translator/builders; avoid a broad execution rewrite.
- [x] Ensure default limits, explicit limits, offsets, bucket limits, forced scope, and explicit unscoped `SCOPE` are applied once to the Effective MRQL Query.
- [x] Preserve bucket explain as key discovery plus explicit `1 + discoveredBuckets` bounds (maximum 201); do not synthesize bucket-item SQL.
- [x] Represent conditional cross-entity random population counts in execution-shape bounds without pretending they are unconditional statements.
- [x] Initialize response slices so empty scope encodes `[]`, not `null`.
- [x] Run focused SQLite and PostgreSQL explain/authorization tests.

## Phase 3 — Add safe native plans and HTTP authorization

- [x] Extend the explain request with additive `nativePlan` input and keep the existing endpoint/fields backward compatible.
- [x] Check the request-scoped principal before any native-plan database work; auth-disabled implicit root remains an admin.
- [x] Execute plans through the same GORM database connection used by MRQL execution, not the separate read-only SQL connection.
- [x] Apply one MRQL timeout to the complete native-planning operation and check cancellation between statements.
- [x] Build native plans into temporary results and publish only after every statement succeeds.
- [x] Return dialect-native data under a stable envelope: structured SQLite rows or PostgreSQL JSON.
- [x] Map native-plan errors to `403` authorization, `400` bad input/unsupported planning, `504` deadline, and `500` unexpected planner failure.
- [x] Never write SQL vars, interpolated SQL, plan constants, or bound MRQL values to new logs.
- [x] Update route OpenAPI metadata/examples and regenerate/validate the OpenAPI spec if tracked output changes.
- [x] Run focused API, auth, cancellation, SQLite-plan, and PostgreSQL-plan tests.

## Phase 4 — Define benchmark artifact contracts (red/green)

- [x] Create `internal/mrqlbench` types for dataset profiles, fixture manifests, scenarios, samples, aggregate results, plan signatures, environment metadata, comparison policies, and versioned JSON artifacts.
- [x] Define standard resource-led profiles at 100k, 1m, and 3m resources with a fixed seed and explicit auxiliary-entity/relation ratios.
- [x] Define a tiny test profile that exercises the same generator without expensive setup.
- [x] Define a curated scenario catalog covering scalar filters, relation filters, shallow/deep/empty scope, hierarchy traversal, JSON metadata, FTS, similarity, first/middle/deep pagination, aggregates, bucket grouping, cross-entity sorting/random, raw/compact/table/custom rendering, and nested MRQL.
- [x] Give every scenario explicit feature/dialect requirements, expected SQL/result bounds, scope class, render mode, and regression policy.
- [x] Implement deterministic nearest-rank p50/p95/p99; suppress p99 unless at least 100 measured samples exist.
- [x] Implement compatibility keys using fingerprint + database/version + profile/version/cardinality + scope + render mode + concurrency + code/schema revision.
- [x] Implement advisory comparisons with scenario overrides, full deltas, and exact SQL-count/result-bound failures.
- [x] Add table tests for JSON stability, profile/scenario uniqueness, percentile edge cases, incompatible baselines, and regression classification.

## Phase 5 — Build deterministic fixture preparation

- [x] Implement batched/streaming fixture generation without retaining million-row datasets in memory or invoking per-row GORM hooks.
- [x] Generate deterministic hierarchy depth, duplicate names, metadata cardinality/selectivity, relation density, FTS content, and similarity rows.
- [x] Keep generated IDs/timestamps/content stable for a given profile/seed/version.
- [x] Refresh planner statistics after seeding (`ANALYZE` on SQLite and PostgreSQL) without using `EXPLAIN ANALYZE`.
- [x] Write manifests containing generator/profile versions, seed, resource cardinality, exact auxiliary counts, database/version, schema revision, feature flags, checksum, and preparation duration.
- [x] Reject stale or mismatched reusable fixtures rather than silently rebuilding or benchmarking them.
- [x] Default PostgreSQL preparation to a disposable PostgreSQL 16 container; permit explicit DSNs only in a marked benchmark database/schema.
- [x] Refuse destructive operations on an unmarked explicit database without a separate confirmation flag; redact DSN credentials.
- [x] Test two tiny preparations for identical manifests/data, bounded batching, stale-manifest refusal, cancellation, and SQLite/PostgreSQL parity.

## Phase 6 — Measure real execution paths

- [x] Implement a benchmark-scoped, concurrency-safe GORM logger wrapper that delegates normally and records actual statements, rows, elapsed time, and errors by sample ID.
- [x] Exclude fixture setup, statistics collection, warmups, and post-run EXPLAIN from measured SQL counts.
- [x] Count all statements during the measured operation, including scope resolution, auxiliary counts, hydration, and nested MRQL.
- [x] Classify statements without forcing interpolation of every fast query or retaining sensitive bind values.
- [x] Add read-only request-local cache/budget statistics only where needed to report hits, misses, and executions; preserve cache semantics.
- [x] Measure honest layers: parse/bind/validate, translation/DryRun, execute+transfer+decode, hydration, rendering, and end-to-end.
- [x] Report exact encoded/rendered `outputBytes` and Go allocation metrics; do not claim database wire-byte measurements.
- [x] Warm scenarios explicitly, record `firstRun` separately, then collect configurable samples (100 for canonical p99 baselines).
- [x] Keep official baselines single-worker; accept optional exploratory concurrency without mixing it into baseline groups.
- [x] Capture each distinct normalized SQL shape and one fixture argument set during execution, then collect native plans after timing.
- [x] Store full fixture-only plans for investigation plus stable dialect-specific plan signatures; do not byte-gate raw plans/cost estimates.
- [x] Add race tests for cross-entity collection, sample isolation, timeout/error atomicity, and query-count invariants.

## Phase 7 — Expose the harness

- [x] Add `cmd/mrql-bench list` for deterministic profile/scenario/feature discovery without a database.
- [x] Add `prepare` for versioned SQLite/PostgreSQL fixtures and manifests.
- [x] Add `run` for scenario selection, warmups, sample count, JSON output, and concise summaries.
- [x] Add `compare` for compatibility validation, advisory thresholds, deterministic failures, and meaningful exit codes.
- [x] Write manifest/result files atomically and handle signals/context cancellation cleanly.
- [x] Add CLI tests for help, flag validation, redaction, destructive safeguards, deterministic listing, atomic output, cancellation, and compare exit codes.
- [x] Add standard `Benchmark...` entry points under `benchmarks/mrql/`, backed by the same scenario/runner code and `ReportAllocs`.
- [x] Make standard benchmarks skip with actionable setup instructions when prepared fixtures are absent; never seed 100k+ rows during ordinary `go test`.

## Phase 8 — Documentation and baselines

- [x] Document `prepare`, `run`, `compare`, and Go benchmark commands in `benchmarks/mrql/README.md`.
- [x] Document warm-cache semantics, `firstRun`, 100-sample percentile policy, environment compatibility, fixture reuse, disk/time prerequisites, threshold interpretation, and baseline review/promotion.
- [x] Document that developer-machine results are `reference`; only a documented stable host may produce a `canonical` baseline.
- [x] Commit aggregate/sanitized artifacts only—never generated databases, payload dumps, credentials, or per-sample traces.
- [x] Generate and review a complete 100k SQLite reference baseline.
- [x] Generate and review a complete 100k PostgreSQL reference baseline.
- [x] Prove 1m and 3m fixture preparation and selected smoke scenarios for both dialects; document full manual-run commands rather than committing full matrices.

## Verification

- [x] Focused explain, fingerprint, native-plan, auth, and cross-entity tests pass with `json1 fts5`.
- [x] Tagged PostgreSQL MRQL/explain/API tests pass.
- [x] `internal/mrqlbench` unit and integration tests pass.
- [x] `internal/mrqlbench` race tests pass.
- [x] `go run ./cmd/mrql-bench list` succeeds without a database.
- [x] Tiny SQLite and PostgreSQL `prepare → run → compare` flows pass.
- [x] Generated OpenAPI validates and docs are fresh.
- [x] Full Go suite passes: `go test --tags 'json1 fts5' ./...`.
- [x] PostgreSQL Go suite passes: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... ./internal/mrqlbench/... -count=1`.
- [x] Browser and CLI E2E pass in parallel: `cd e2e && npm run test:with-server:all`.
- [x] PostgreSQL E2E passes: `cd e2e && npm run test:with-server:postgres`.
- [x] 100k SQLite/PostgreSQL reference runs self-compare cleanly.
- [x] 1m/3m smoke evidence and exact commands are recorded.
- [x] Final diff receives a fresh read-only correctness/security/performance review.

## Review

Implemented the runtime explain extension and shared deterministic benchmark harness.

### Delivered

- `/v1/mrql/explain` now returns value-redacted query fingerprints and execution-shape bounds; administrators may opt into non-executing dialect-native plans with `nativePlan: true`.
- Effective-query scope, cross-entity pagination, bucket/random fan-out bounds, shared planning deadlines, atomic errors, and SQL-only authorization are covered by SQLite, PostgreSQL, and API tests.
- `internal/mrqlbench` and `cmd/mrql-bench` provide deterministic profiles, fixture integrity validation, real execution-path collection, native plan signatures, aggregate artifacts, comparisons, and `list`/`prepare`/`run`/`compare` commands.
- Fixture validation records exact entity/relation counts plus an ordered FTS-membership digest, detecting membership changes even when match counts remain unchanged.
- Aggregate-only 100k SQLite and PostgreSQL reference baselines use artifact schema 5, fixture generator `mrql-bench-fixture-v7`, scenario catalog `mrql-bench-scenarios-v3`, 100 samples, and 21 scenarios. Both self-compare cleanly.
- 1m and 3m fixtures and selected smoke scenarios completed for SQLite and disposable PostgreSQL 16; exact reproduction commands are in `benchmarks/mrql/README.md`. Generated fixtures/results remain ignored.

### Verification evidence

- `go test --tags 'json1 fts5' ./...` — passed.
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... ./internal/mrqlbench/... -count=1` — passed.
- `go test -race --tags 'json1 fts5' ./internal/mrqlbench ./application_context ./mrql ./shortcodes -count=1` — passed.
- OpenAPI generation and validation — passed (204 paths, 93 schemas, 26 tags).
- `cd e2e && npm run test:with-server:all` — passed.
- `cd e2e && npm run test:with-server:postgres` — 1,703 passed, 4 skipped, with one unrelated download-cockpit focus retry classified flaky; targeted PostgreSQL rerun with `--repeat-each=3` passed 9/9.
- Fresh read-only review reported no blocker, high, or medium findings.

### Residual notes

- Timing and raw planner output remain host/database-version sensitive; compatibility keys and advisory timing gates prevent incompatible runs from being presented as regressions.
- Large fixtures require deliberate disk/WAL/time budgeting and are never generated by ordinary tests.
- `go vet --tags 'json1 fts5' ./...` still reports the pre-existing lock-copy warning at `plugin_system/action_jobs.go:87`; this change does not touch that code.
