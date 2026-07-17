# MRQL Performance Remediation

Source review: `docs/mrql-performance-review.md`

## Phase 1 — Rendering hot path
- [x] Add batched, request-cached carrier and group-ancestry loading.
- [x] Keep association-heavy CRUD getters out of MRQL rendering without changing their public behavior.
- [x] Refactor flat and bucketed shortcode/API rendering to use batch data.
- [x] Attach one query budget and overall deadline to API render paths.
- [x] Add query-count, association, scope, cancellation, and budget regression tests.

## Phase 2 — Filters, request scoping, and indexes
- [x] Compile/apply list filter predicates directly to outer queries (no self-`IN`).
- [x] Preserve fail-closed behavior and direct timeline reuse.
- [x] Bind MRQL HTTP routes to principal identity without eager subtree materialization.
- [x] Intersect nested shortcode scopes with forced principal scope and hide out-of-subtree scope metadata.
- [x] Add missing scalar, functional-name, and reverse-junction indexes.
- [x] Add SQL-shape/schema/planner tests.

## Phase 3 — Execution guardrails and overhead
- [x] Enforce fixed MRQL language byte/token/depth/IN-list limits.
- [x] Enforce interactive/export limit and offset ceilings with checked arithmetic.
- [x] Enforce a true bucket item cap and bounded bucket-query fan-out.
- [x] Build timeout SQL diagnostics lazily.
- [x] Reuse application FTS capability instead of catalog probing per query.
- [x] Remove validation's duplicate parse.
- [x] Make explain use the same interactive execution bounds.

## Phase 4 — Frontend cancellation and documentation
- [x] Abort/ignore superseded validation, completion, execute, and explain requests.
- [x] Preflight native exports so errors remain visible without browser Blob buffering.
- [x] Document new language/execution guardrails and update the performance review status.
- [x] Rebuild frontend bundle.

## Verification
- [x] Focused Go unit/API tests.
- [x] Full tagged Go suite.
- [x] Frontend unit tests and JS build.
- [x] Browser + CLI E2E.
- [x] PostgreSQL MRQL/API suite and full PostgreSQL E2E.
- [x] Fresh read-only performance and authorization review of the completed diff.

## Review

Completed 2026-07-17.

- Full Go suite passed with `json1 fts5` tags.
- Frontend: 29 files / 577 tests passed; Vite production build passed.
- SQLite browser + CLI E2E: 1,703 passed, 5 skipped.
- PostgreSQL MRQL/API tests passed; PostgreSQL browser + CLI E2E: 1,704 passed, 4 skipped.
- Auth E2E: 12 passed.
- Final independent review found no blocker/high/medium findings after scope-intersection, scope-resolution, and explain-bound follow-ups.
- Remaining environment-scale validation: million-row heap/load profiling. Runtime paths are bounded, but bucketed grouping intentionally uses a capped intermediate fan-out rather than a single set-based window query.
