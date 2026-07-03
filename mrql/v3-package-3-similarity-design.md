# MRQL v3 Package 3: Similarity Search — Design

Implements the `SIMILAR TO resource(N)` perceptual-hash predicate (see
`v3-packages.md` §3a) plus `ORDER BY distance`. Similarity v2 (pHash +
chunk-index matching, read-time thresholds) is merged and deployed, so the
fast lookup path exists: precomputed pairs in `resource_similarities`.

## Surface

```
type = resource AND SIMILAR TO resource(1234)
type = resource AND SIMILAR TO resource(1234) WITHIN 5
type = resource AND SIMILAR TO resource(1234) AND tags != "reviewed"
type = resource AND SIMILAR TO resource(1234) ORDER BY distance ASC LIMIT 20
type = resource AND (SIMILAR TO resource(1) OR SIMILAR TO resource(2))
type = resource AND NOT SIMILAR TO resource(1234)
```

- `SIMILAR TO resource(<id>)` is a WHERE-level predicate (a "primary", like
  `TEXT ~ "..."`), composing freely with `AND`/`OR`/`NOT` and parentheses.
- `<id>` is a positive integer resource ID. Only `resource(...)` is valid —
  notes/groups have no perceptual hashes.
- `WITHIN <d>` optionally overrides the primary distance threshold,
  `0 <= d <= 11`.
- `ORDER BY distance` (ASC/DESC) sorts by the perceptual distance to the
  target. Valid only when the WHERE clause contains **exactly one**
  `SIMILAR TO` predicate.

## Semantics (decisions, confirmed with user)

- **Reads precomputed pairs only.** The predicate is pure SQL over
  `resource_similarities` (`COALESCE(p_distance, hamming_distance)` is the
  effective distance), identical to the resource-page sidebar's read path
  (`getSimilarResourcesLimited`, `application_context/resource_crud_context.go:39`).
  It does NOT replicate the sidebar's zero-pairs fallback (exact `d_hash`
  match), and it never computes hashes at query time.
- **Default threshold = the runtime setting.** When `WITHIN` is omitted, the
  live `hash_similarity_threshold` runtime setting applies (default 10), so
  MRQL matches the sidebar and threshold tuning applies to saved queries
  instantly. Plumbed via `TranslateOptions` — the `mrql` package never reads
  settings itself.
- **The aHash secondary filter always applies.** The runtime
  `hash_ahash_threshold` (0 = disabled) adds
  `(a_distance IS NULL OR a_distance <= <a>)` exactly like the sidebar,
  whether or not `WITHIN` is present. `WITHIN` overrides only the primary
  distance. MRQL results therefore always agree with the similarity UI.
- **`WITHIN > 11` is a validation error.** Pairs are only stored up to
  `MaxStoredPDistance = 11` (`hash_worker`), so a larger radius would
  silently under-match. The cap is mirrored as a constant in `mrql`
  (no import of `hash_worker` from `mrql`); an equality test in
  `application_context` (which imports both) keeps them in sync.
- **Strict:** the target resource is never in its own result set (the pairs
  table has no self-pairs). Corollary: `NOT SIMILAR TO resource(N)` includes
  resource N itself.
- **Missing data degrades to empty, not error.** A nonexistent target ID, or
  a target with no hash / no pairs yet (unhashed, failed, flat, non-image),
  simply yields an empty match set.
- **Resource-only.** Explicit `type = note/group` + `SIMILAR TO` is a
  validation error; so is cross-entity use without a type guard
  (`EntityUnspecified`). In a type-guarded OR branch, per-branch validation
  accepts it, and the note/group entity clones translate it to `1 = 0`
  (never `TranslateError` — see the count-field fix, commit 8033bb87:
  a TranslateError would make `executeCrossEntity` drop the whole entity).

## Translation

Predicate (target ID as bind placeholders; validated integer thresholds
inlined, matching the sidebar code's style):

```sql
resources.id IN (
  SELECT rs.resource_id2 FROM resource_similarities rs
    WHERE rs.resource_id1 = ? AND <filter>
  UNION ALL
  SELECT rs.resource_id1 FROM resource_similarities rs
    WHERE rs.resource_id2 = ? AND <filter>
)
-- <filter> = COALESCE(rs.p_distance, rs.hamming_distance) <= <d>
--            [AND (rs.a_distance IS NULL OR rs.a_distance <= <a>)]
```

Both directions are needed because pairs are stored once with
`resource_id1 < resource_id2`; the composite indexes `idx_sim_r1_dist` /
`idx_sim_r2_dist` serve each arm. Dialect-neutral — no PG/SQLite branches.

`ORDER BY distance` resolves (in `resolveOrderByColumn`, the same hook
`<relation>.count` uses) to a correlated scalar subquery with the target
inlined as a literal (GORM `Order()` takes no bind values; the ID is a
validated integer):

```sql
COALESCE((
  SELECT MIN(COALESCE(rs.p_distance, rs.hamming_distance))
  FROM resource_similarities rs
  WHERE (rs.resource_id1 = <target> AND rs.resource_id2 = resources.id)
     OR (rs.resource_id2 = <target> AND rs.resource_id1 = resources.id)
), 255)
```

The outer `COALESCE(..., 255)` pins pairless rows (possible when the
`SIMILAR TO` sits under OR/NOT) to the end in ASC order on both dialects
(SQLite sorts NULLs first, Postgres last — the sentinel removes the
divergence). `MIN` is a scalarizer; the pair row is unique.

The translator discovers the target by scanning the WHERE AST for
`SimilarToExpr` nodes up front (deterministic, not dependent on WHERE
translation order). The validator guarantees exactly one exists when
`distance` is used as a sort key.

## Grammar / lexer

- `SIMILAR TO` becomes one merged token via the `ORDER BY`/`GROUP BY`
  precedent in `readWord` (`lexer.go:229`): `SIMILAR` followed by `TO` merges
  to `TokenSimilarTo`; `SIMILAR` alone stays a plain identifier, so field or
  meta keys named `similar` keep working.
- `WITHIN` is NOT a lexer keyword — the parser checks for a `TokenIdentifier`
  spelled `WITHIN` (case-insensitive) after the closing `)`, so `meta.within`
  and a field named `within` keep working.
- New AST node `SimilarToExpr{Token, TargetID int64, Within int}` with
  `Within = -1` when absent. Parsed by `parseSimilarTo` from `parsePrimary`
  (`parser.go:201`), modeled on `parseTextSearch`: expect identifier
  `resource` (any other word → targeted error), `(`, integer, `)`,
  optional `WITHIN` + integer.
- `distance` is not a keyword either: it gains meaning only as a
  single-part ORDER BY key when the query qualifies; elsewhere it stays an
  unknown field (same behavior as today).

## Validation rules

- `SimilarToExpr` in `validateNode`: entity must be `EntityResource`
  (per-branch extraction in OR/NOT already supplies the right type);
  `EntityUnspecified` → "SIMILAR TO requires type = resource".
- Target ID must be a positive integer (parser enforces integer; validator
  enforces > 0).
- `WITHIN` range 0..`maxSimilarityDistance` (11), else a positioned error
  naming the storage cap.
- `ORDER BY distance`: allowed iff entity is resource AND the WHERE clause
  contains exactly one `SimilarToExpr` (zero → "requires a SIMILAR TO
  predicate"; two+ → "ambiguous with multiple SIMILAR TO predicates").
  Rejected as an aggregated-GROUP-BY sort key (not an aggregate alias, the
  existing `validOrderKeys` check already handles it).
- `LintGeneratedQuery` gets a `SimilarToExpr` case (positive ID, WITHIN cap)
  so NL generation can emit it safely.

## Touch points

- `mrql/token.go`, `mrql/lexer.go` — `TokenSimilarTo` merged token.
- `mrql/ast.go` — `SimilarToExpr`.
- `mrql/parser.go` — `parsePrimary` case + `parseSimilarTo`.
- `mrql/validator.go` — `validateNode` case; ORDER BY `distance` rules;
  `maxSimilarityDistance` constant.
- `mrql/translator.go` — `translateNode` case + `translateSimilarTo`
  (with the `1 = 0` entity fallback); `resolveOrderByColumn` `distance` key;
  `TranslateOptions.SimilarityThreshold *int` (nil → 10, mirroring
  `similarityThresholds()`'s fallback) and `AHashThreshold uint64`
  (0 = disabled).
- `application_context/mrql_context.go` — fill the two new options from
  `ctx.similarityThresholds()` at the single `TranslateOptions` construction
  site (plus the plugin MRQL adapter if it builds its own).
- `application_context` — sync test: `hash_worker.MaxStoredPDistance ==
  mrql.MaxSimilarityDistance`.
- `mrql/completer.go` — `SIMILAR TO resource(` keyword suggestion at field
  positions (gated to resource-typed queries, like the recursive roots);
  `WITHIN` + post-value keywords after the closing `)`; `distance` in ORDER
  BY suggestions when the query contains a `SIMILAR TO`.
- `application_context/mrql_generation.go` — prompt rule + example mappings;
  `mrql/generation_lint.go` case.
- Docs: `.claude/skills/mahresources-cli/references/mrql.md`,
  `docs-site/docs/features/mrql.md`, `docs-site/docs/features/mrql-reference.md`.
- Tests: `mrql/similar_to_test.go` (SQLite: parser/validator/SQL-shape/
  execution against seeded pairs incl. both directions, aHash filter, OR/NOT
  composition, ORDER BY distance), `mrql/similar_to_pg_test.go`, lexer cases,
  completer cases, generation prompt/lint cases,
  `e2e/tests/mrql-similarity.spec.ts` (syntax accepted, validation errors
  surfaced; real matching is covered by the Go integration tests — e2e can't
  wait out the hash worker's poll interval).

## Explicitly out of scope (v1)

- On-the-fly matching for arbitrary images (needs Go-side popcount or a SQL
  popcount that doesn't exist); the predicate reads precomputed pairs only.
- `SIMILAR TO` for notes/groups, or by file path/upload.
- The sidebar's exact-`d_hash` zero-pairs fallback.
- Exposing the per-row distance as a selectable field (only sort order).
