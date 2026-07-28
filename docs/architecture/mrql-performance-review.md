# MRQL Performance Review

Date: 2026-07-17

## Executive summary

MRQL's parser and ordinary bounded, single-entity execution are generally well structured. The implementation has useful safety controls: a configurable default limit, query timeouts, bounded recursive CTEs, precomputed/indexed similarity pairs, native FTS, cross-entity concurrency limited to three workers, and a per-page shortcode query budget/cache.

It is **not yet ready for million-row workloads across all supported surfaces**. Two paths can become catastrophic even for small visible result sets:

1. Rendered note results perform per-item `NoteType` lookups that preload every note belonging to the type.
2. List-page MRQL filters are wrapped in a self-`IN` subquery that can materialize every matching ID and defeat index-ordered `LIMIT` execution.

The next tier of issues is missing/reversed indexes, unbounded explicit result sizes, bucketed `GROUP BY` query fan-out, and render/scoping work outside the core timeout and budget.

## Remediation status

The issues in this review have been remediated in the accompanying change set:

- MRQL rendering now batch-loads scalar carrier fields and group ancestry through a request cache; normal CRUD getter behavior remains unchanged.
- List and timeline filters compose their predicate directly into the scoped outer query.
- Scalar, case-insensitive-name, and reverse-junction indexes cover the reviewed query shapes.
- Parser, completion, pagination, grouping, and export work now have fixed ceilings and overflow-safe arithmetic. Bucket grouping also has hard query and retained-item caps with continuation signaling.
- API, preview, deferred, and shortcode render paths share deadlines, partial/plugin caches, and query budgets, and stop promptly on cancellation.
- MRQL routes retain SQL-enforced principal scoping without eagerly materializing subtrees in Go.
- FTS capability is cached, timeout SQL formatting is lazy, validation parses once, and browser requests cancel or ignore stale work.
- Browser exports use a validated native streaming download rather than buffering a JavaScript `Blob`; server-side materialization is bounded by a 10,000-row export limit and a 10,000-row offset ceiling.

SQLite planner/query-count regressions and focused API tests were added. Live PostgreSQL plans and million-row heap/load profiles remain environment-dependent validation work rather than unbounded production paths.

## Findings

### Critical 1 — Rendered note results can reload an entire note type once per result

**Evidence**

- `server/template_handlers/template_filters/shortcode_query_executor.go:184-281` enriches every flat result and calls `GetNoteType` per note; bucketed conversion repeats this at `:300-397`.
- `server/api_handlers/mrql_api_handlers.go:209-273` and `:278-359` do the same for `/v1/mrql?render=1`.
- `application_context/note_context.go:418-420` implements `GetNoteType` with `Preload(clause.Associations)`.
- `models/note_type_model.go:18` defines `NoteType.Notes`; therefore the preload includes all notes of that type.
- The main MRQL editor always requests `render=1` (`src/components/mrqlEditor.js:483-499`).

**Impact**

A 20-note result in one large note type can issue 20 carrier queries and repeatedly materialize that type's complete note collection. Category and hierarchy enrichment also produces per-item queries, although the note-type association preload is the most severe case.

**Recommendation**

Create a render-specific batch loader. Collect distinct category/note-type IDs, fetch only `id`, `meta_schema`, `custom_mrql_result`, and `custom_css` once per carrier kind, and attach them from maps. Never use association-preloading CRUD getters for MRQL rendering. Resolve parent/root scope data with one recursive query or a request-local memoized ancestry map.

### Critical 2 — List filters defeat efficient ordered pagination

**Evidence**

- `application_context/mrql_context.go:177-215` translates a list filter to `outer.id IN (SELECT table.id FROM table WHERE ...)`.
- Resource, note, and group list/count/sidebar paths all compose filters this way.
- A focused production-model SQLite planner probe showed:

```text
MRQL self-subquery:
SEARCH resources USING INTEGER PRIMARY KEY (rowid=?)
LIST SUBQUERY 1
SEARCH resources USING COVERING INDEX idx_resources_created_at (created_at>?)
USE TEMP B-TREE FOR ORDER BY

Equivalent direct predicate:
SEARCH resources USING INDEX idx_resources_created_at (created_at>?)
```

**Impact**

For a query such as a broad created-date range ordered by `created_at DESC LIMIT 50`, the MRQL shape can enumerate all matching IDs, perform primary-key lookups, and sort into a temporary B-tree. The direct predicate walks the existing ordered index and stops after 50 rows. This is a major regression at millions of rows.

**Recommendation**

Expose a translator operation that applies a validated filter AST directly to an existing GORM query, without resetting its table or adding MRQL pagination. `ParseFilter` already forbids clauses that would conflict with list pagination. Keep a subquery option only for call sites that truly require it, and add planner regression tests for the direct list path.

### High 1 — Schema indexes do not match several core MRQL query shapes

**Evidence**

A production-model SQLite AutoMigrate probe, checked against the manual startup indexes in `main.go:519-546`, found:

- no index on `notes.owner_id`, `notes.note_type_id`, `notes.start_date`, or `notes.end_date` (`models/note_model.go:24-28`);
- no index on `groups.category_id` (`models/group_model.go:39`);
- no indexes on queryable `resources.file_size`, `width`, or `height` (`models/resource_model.go:27-29`);
- `groups_related_notes` is keyed `(group_id, note_id)` with no startup reverse index, so note-side probes by `note_id` cannot use its leading column. Startup does add reverse indexes for resource-note, group-resource, and tag junctions; those paths are not missing the same migration.

Focused SQLite plans confirmed `SCAN notes` for `notes.owner_id = ?` and a full `groups_related_notes` scan for note-side group probes.

Case-insensitive translation compounds the problem. `mrql/translator.go:1007-1010` emits `LOWER(column) = LOWER(?)`; relation and `IN` paths do likewise. Existing raw-column B-trees cannot satisfy those expressions. A probe of indexed `resources.name` still produced `SCAN resources`. Relation-name filters also need an index matching the emitted case-insensitive name expression; otherwise the planner must scan the related-name side even where the junction reverse key is indexed.

**Recommendation**

1. Add immediate indexes for `notes(owner_id)`, `notes(note_type_id)`, and reverse junction keys such as `groups_related_notes(note_id)`.
2. Add workload-driven indexes for note dates, group category, and resource size/dimensions.
3. Align case-insensitive semantics with index expressions/collations. On PostgreSQL remove redundant `LOWER()` around `ILIKE` so the existing raw-name trigram index can match, or index the exact emitted expression. Use `COLLATE NOCASE`/expression indexes deliberately on SQLite.
4. Test production migrations, not hand-written test junction schemas.

### High 2 — Explicit limits and exports can materialize unbounded results

**Evidence**

- Validation does not impose a maximum explicit `LIMIT` or `OFFSET`.
- `server/api_handlers/mrql_export_handler.go:97-185` describes streaming but first materializes the complete flat/grouped result in model slices.
- `application_context/mrql_context.go:688-872` fetches `offset + limit` full rows from each entity table for cross-entity queries, builds another merged slice, sorts it, then discards the offset.
- The browser reads the full export into a Blob (`src/components/mrqlEditor.js:563-594`).

**Impact**

`LIMIT 1000000`, a deep page, or a large grouped export can exhaust server and browser memory. Timeouts bound database duration, not allocations after rows have been returned.

**Recommendation**

Define separate hard ceilings for interactive execution/rendering and export. Implement exports with projected columns plus `Rows`/`ScanRows` or bounded chunks; move very large exports to the existing background-job/file pattern. Replace deep cross-entity offset paging with a database `UNION ALL` common projection and keyset/cursor pagination.

### High 3 — Bucketed `GROUP BY` performs up to 1,001 sequential statements

**Evidence**

- `mrql/translator_groupby.go:99-109` discovers up to `MaxBuckets + 1` keys.
- `application_context/mrql_context.go:532-631` and `:1505-1596` then execute one full item query per key, rebuilding filters, joins, and scope CTEs each time.
- The 10,000-item check occurs before each bucket. A bucket can take the total from 9,999 to 19,999, so the documented cap is not a hard cap.

**Impact**

High-cardinality grouping with small per-bucket results can issue 1,001 statements inside one request. Rendering those rows then multiplies the carrier/ancestry N+1 described above.

**Recommendation**

Use a selected-keys CTE plus `ROW_NUMBER() OVER (PARTITION BY bucket keys ORDER BY ...)` and fetch a page in one statement. A batched row-value/OR predicate is an acceptable intermediate step. Add a query-count ceiling and a true retained-item cap immediately.

### High 4 — API rendering bypasses the configured shortcode query budget

**Evidence**

- Normal page rendering attaches MRQL cache, partial resolver, and `QueryBudget` in `server/template_handlers/template_filters/shortcode_tag.go:95-115`.
- API rendering attaches only the plugin cache and partial resolver at `server/api_handlers/mrql_api_handlers.go:209-213` and `:284-291`.
- `BudgetedExecutor` runs unbudgeted when the context contains no budget (`shortcodes/query_budget.go:108-139`).

**Impact**

A `CustomMRQLResult` containing entity-scoped `[mrql]` can fan out once per result on the application's main MRQL UI, despite the configured page budget. Each nested query gets its own database timeout; there is no overall render deadline.

**Recommendation**

Build one shared render-context helper and use it on every rendering surface. Attach a query budget and one overall render deadline to flat/grouped API rendering, previews, deferred rendering, and normal pages.

### High 5 — Scoped MRQL requests duplicate subtree work in Go and SQL

**Evidence**

- MRQL execute/explain/export routes use `scopedAPI` (`server/routes.go:627-638`).
- `WithPrincipal` eagerly materializes up to one million subtree IDs through `collectSubtreeGroupIDs` (`application_context/scoping.go:124-162`, `application_context/group_tree_context.go:83-105`).
- MRQL subsequently applies its own recursive scope CTE via translation.
- The generic scoped DB is rooted at `context.Background()`, so subtree collection is not canceled with the request.

**Impact**

A scoped user's `LIMIT 1` or EXPLAIN can still allocate an O(subtree-size) ID list before MRQL executes, then traverse the subtree again in SQL.

**Recommendation**

Give raw MRQL handlers a principal/actor binding that does not build the generic ORM allow-list; enforce scope only through MRQL's recursive CTE. Longer term, represent generic ORM scope as a subquery/CTE rather than a giant in-memory `IN` list.

### Medium findings

1. **List filters compile repeatedly and lack request-bound timeout.** A resource page checks the filter, then rows, count, and popular-tags each parse/validate/translate it again. These queries use the detached scoped DB rather than `request.Context()` and `MRQL_QUERY_TIMEOUT`.
2. **MRQL language input is unbounded.** `Parse`, `ParseFilter`, `Complete`, logical nesting, and `IN` length have no MRQL-specific limits. Recursive parser/validator walks can consume disproportionate CPU/stack on large authenticated inputs. Add byte, token, nesting, and list-size limits.
3. **Every statement pays for timeout diagnostics.** `executeMRQLFind`/`executeMRQLCount` build and interpolate a DryRun SQL statement before every successful query (`application_context/mrql_context.go:77-89`), although it is used only on timeout.
4. **FTS capability is queried during translation.** `mrql/translator.go:1604-1651` checks catalog tables for each TEXT predicate; RANK repeats the check at `:1873-1907`. Cache FTS availability in translation options/application context.
5. **Expensive operators need explicit cost guidance.** `ORDER BY RANDOM()`, broad relation-count sorting, SQLite RANK, and similarity-distance ordering require scans, temp sorting, or correlated lookups. Document and optionally reject unfiltered interactive use.
6. **Frontend requests are not superseded/canceled.** Validation, completion, execute, and explain can continue after newer editor input and consume avoidable server work.

## Strengths

- Lexer/parser are linear for normal input and use one-token lookahead; no backtracking or per-request regex compilation was found.
- Query ASTs are usually reused by execution handlers rather than reparsed.
- Default result limits and runtime query timeouts protect ordinary queries without explicit limits.
- Recursive hierarchy and scope CTEs have depth guards and use indexed `groups.owner_id` traversal.
- Similarity membership uses precomputed pairs and indexes both resource directions.
- Native SQLite FTS5/PostgreSQL FTS is used when available.
- Cross-entity table reads use only three concurrent workers and surface timeout warnings as partial results.
- Normal page shortcodes have a well-designed distinct-query budget, dedupe cache, and defensive cloning.

## Recommended remediation order

1. Replace MRQL render carrier/ancestry N+1; specifically remove `GetNoteType` association preload from result rendering.
2. Apply list filters directly and add SQLite/PostgreSQL planner regression tests.
3. Add note/reverse-junction indexes and align case-insensitive predicates with indexes.
4. Add interactive hard limits and true streaming/background exports.
5. Replace bucketed query-per-key execution and enforce a hard retained-item cap.
6. Unify render budget/deadline setup and eliminate scoped subtree pre-materialization for MRQL routes.
7. Add MRQL input/depth limits, request cancellation, cached FTS capability, and lazy timeout SQL diagnostics.

## Validation performed

- `go test --tags 'json1 fts5' ./mrql ./shortcodes ./plugin_system ./server/template_handlers/template_filters` — passed.
- Production-model SQLite AutoMigrate plus startup-index inspection — confirmed missing note/category/range indexes and the specific `groups_related_notes(note_id)` reverse index.
- Focused SQLite `EXPLAIN QUERY PLAN` probes — confirmed:
  - self-`IN` list filter materialization and temporary sorting versus a direct indexed predicate;
  - `LOWER(name)` scanning despite the raw name index;
  - `groups_related_notes` reverse-key scanning without a `note_id` index;
  - note-owner scanning;
  - correlated relation-count scan and temporary sort.
- Three independent read-only review passes covered SQL/query plans, language CPU/allocations, and end-to-end runtime behavior.

## Validation gaps

- No live PostgreSQL `EXPLAIN (ANALYZE, BUFFERS)` was run; PostgreSQL index-expression findings are based on emitted SQL and declared indexes.
- No million-row concurrent load or heap profile was run.
- Existing `ExplainDB` tests render SQL via GORM DryRun; they do not inspect a database query plan.
- There are no persistent MRQL benchmarks or query-count assertions for rendering and bucketed execution.
