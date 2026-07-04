# MRQL v3 Package 6: Ergonomics - Design

Implements the four ergonomics items from `v3-packages.md` (Package 6). Each is
small and independently shippable; they share no code beyond the usual
cross-cutting surfaces (completer, NL generation, docs).

```
created BETWEEN "2024-01-01" AND "2024-06-30"
fileSize NOT BETWEEN 1mb AND 10mb
type = resource AND tags IS EMPTY ORDER BY RANDOM() LIMIT 20
type = note AND TEXT ~ "kubernetes migration" ORDER BY RANK LIMIT 10
type = resource AND name ~* "^IMG_[0-9]{4}\.(jpe?g|png)$"        (PostgreSQL only)
```

None of the four exists in any form today: `rank` and `random` have zero hits
in `mrql/`, `between` appears only in comments, and the only `regexp` hit is
the Go stdlib import used for meta-key validation (translator.go:1065),
unrelated to any query operator. All four are greenfield.

## Current state (code references)

- Operators are scanned in `next()` (`mrql/lexer.go:83-130`); `~` maps to
  `TokenLike` (lexer.go:87-89) and `!~` to `TokenNotLike` (lexer.go:110-113).
  An unrecognized character becomes `TokenIllegal` (lexer.go:132-134).
- Keywords live in `keywordMap` (`mrql/lexer.go:341`), matched
  case-insensitively via `readWord`. Zero-arg functions (`NOW()`,
  `START_OF_DAY()`, ...) live in `knownFunctions` (lexer.go:369) and only
  match when followed by `()`.
- `parseFieldExpr` (`mrql/parser.go:408-446`) dispatches on the token after a
  field: the comparison-operator case at parser.go:417, `IN` at 420, `NOT`
  (which today must be followed by `IN`) at 423, `IS` at 436.
  `parseComparison` (parser.go:497-511) builds
  `ComparisonExpr{Field, Operator Token, Value}`; `parseValue`
  (parser.go:595) reads String/Number/RelDate/Func/Param values plus bare
  identifiers as string values (parser.go:619-623).
- `parseOrderBy` (`mrql/parser.go:687-717`) accepts only dotted fields via
  `parseField`, then optional ASC/DESC (default ASC). No function-call syntax
  exists in ORDER BY.
- The `distance` sort key is the precedent for a context-sensitive ORDER BY
  key: it is not a lexer keyword; it parses as a plain field and gains meaning
  in `Validate` (`mrql/validator.go:199-204` short-circuits into
  `validateDistanceOrderKey`, validator.go:233-264) and in
  `resolveOrderByColumn` (`mrql/translator.go:1638-1643`), which emits a
  correlated subquery with a `COALESCE(..., 255)` sentinel to pin pairless
  rows to the end on both dialects.
- GORM's `Order()` takes no bind values, so anything ORDER BY needs must be
  inlined as validated/escaped literals (documented at translator.go:710-716,
  the SIMILAR TO precedent).
- Operator-to-SQL mapping is the `sqlOperator` switch
  (`mrql/translator.go:1577`); dialect forks check the dialector name via
  `tc.isPostgres()` (translator.go:186-189) or inline, as `likeOperator()`
  does (translator.go:1600-1606).
- `TEXT ~` translation is self-contained in `translateTextSearch`
  (`mrql/translator.go:1442-1501`): Postgres probes
  `information_schema.columns` for `search_vector` then emits
  `search_vector @@ plainto_tsquery('english', ?)`, with an ILIKE fallback;
  SQLite sanitizes the term via `sanitizeFTS5` (translator.go:1715-1731,
  strips everything but alphanumerics, spaces, `.` and `,`), probes
  `sqlite_master` for `<table>_fts`, then emits
  `id IN (SELECT rowid FROM <table>_fts WHERE <table>_fts MATCH ?)`, with a
  LIKE fallback. No ranking is emitted anywhere.
- The `fts/` package already has rank expressions (`GetRankExpr`,
  `fts/provider.go:38-40`): `ts_rank(search_vector, plainto_tsquery(...))` on
  Postgres (`fts/postgres.go:151-183`) and a
  `(SELECT -bm25(<t>_fts) FROM <t>_fts WHERE rowid = <t>.id AND ... MATCH ?)`
  subquery on SQLite (`fts/sqlite.go:225-261`). The MRQL translator does not
  use `fts/` today; these serve as the reference shapes.
- `validateValueType` (`mrql/validator.go:668-734`) is the per-value type
  check (non-FK numbers reject non-numeric, with an explicit carve-out for
  `_id`-column FK fields; datetimes accept string/reldate/func; strings and
  meta accept anything). It already handles `ParamRef` deferral.
- Completer operator suggestions are the `operators` slice
  (`mrql/completer.go:13-25`); ORDER BY context suggestions live in the
  switch at completer.go:634-662, where `distance` is conditionally appended
  when the query contains SIMILAR TO (completer.go:645-647).

## 6a. BETWEEN

### Syntax

```
field BETWEEN <value> AND <value>
field NOT BETWEEN <value> AND <value>
```

Inclusive on both ends, exactly like SQL. Pure sugar:
`f BETWEEN a AND b` is defined as `(f >= a AND f <= b)`, and
`f NOT BETWEEN a AND b` as `NOT (f >= a AND f <= b)`. It is allowed wherever
`>=`/`<=` are allowed today, which means dates, numbers (including unit
literals like `1mb`), strings (lexicographic, same as today's `>=`), meta
fields, and traversal chains. Bounds can be any `parseValue` value, including
`$params` and `NOW()`/`-7d`.

### Grammar / parsing

- `BETWEEN` is NOT added to `keywordMap`. Like `WITHIN` in package 3
  (parser-side `strings.EqualFold` on a plain identifier, `parser.go:382`
  precedent), it is matched case-insensitively as an identifier so field and
  meta keys named `between` keep working. There is no ambiguity: a bare
  identifier after a field is a parse error today (`parseFieldExpr` default
  case, parser.go:439-445).
- `parseFieldExpr` gains two hooks: an identifier spelled `BETWEEN` after the
  field dispatches to `parseBetween(field, negated=false)`; the existing
  `TokenNot` case (parser.go:423, currently "expected IN after field NOT")
  additionally accepts `BETWEEN` for the negated form.
- `parseBetween` = consume `BETWEEN`, `parseValue`, expect `TokenAnd`,
  `parseValue`. `AND` is already a distinct token and values are terminals,
  so the SQL grammar ambiguity between the range separator and boolean AND
  does not arise.
- **Desugared at parse time.** No new AST node: the parser returns
  `BinaryExpr(AND){ComparisonExpr(field, >=, lo), ComparisonExpr(field, <=, hi)}`,
  wrapped in `NotExpr` for the negated form, with synthesized operator tokens
  carrying the BETWEEN token's position. The two comparisons get separate
  copies of the `FieldExpr` (cheap; avoids shared-node aliasing). Everything
  downstream (validation including `validateValueType` per bound, params via
  `ParamRef`, translation, EXPLAIN, generation lint, cross-entity cloning,
  the list-page filter bar via `ParseFilter`) works with zero changes,
  because the desugared tree is made of existing node types.

The trade-off of parse-time desugar: `Complete()` and error positions see the
desugared tree, not a BETWEEN node. Positions are preserved on the
synthesized tokens, so validator errors still point into the query text.
This is acceptable for sugar; a dedicated `BetweenExpr` earns its keep only
if we later want to render queries back from the AST, which nothing does
today (saved queries store raw text).

### Validation / translation

None. That is the point of the desugar. `NOT (a AND b)` and SQL's
`NOT BETWEEN` have identical NULL semantics (both filter the row when the
column is NULL), so no fidelity is lost.

## 6b. ORDER BY RANDOM()

### Syntax

```
ORDER BY RANDOM()
ORDER BY name, RANDOM()          (random tiebreak within equal names)
```

- No ASC/DESC after `RANDOM()`: a direction token there is a parse error
  ("RANDOM() does not take a direction").
- Allowed at any position in the ORDER BY list. A trailing `RANDOM()` after
  other keys is a legitimate random tiebreak; a leading one makes later keys
  unreachable but is harmless.
- Rejected with GROUP BY (both modes): random ordering of aggregate buckets
  is meaningless in bucketed mode and clashes with the alias-based ORDER BY
  path in aggregated mode (`buildAggregatedGroupByDB`). Validation error.
- Works on both dialects unchanged: `RANDOM()` is native SQL on SQLite and
  PostgreSQL. No dialect fork.

### Grammar / parsing

- `RANDOM` is NOT added to `knownFunctions` (lexer.go:369). Adding it there
  would make `RANDOM()` a valid-looking value in WHERE (where it means
  nothing) and would shadow identifiers. Instead `parseOrderBy` handles it
  locally: after `parseField` returns a single-part field spelled `RANDOM`
  (case-insensitive) and the next token is `(`, consume `(` and expect `)`,
  and emit an `OrderByClause{Random: true}` with `Field` nil.
- `OrderByClause` (ast.go:179) gains `Random bool`.

### Validation

- In the `Validate` ORDER BY loop (validator.go:196-218): `Random` clauses
  skip field validation entirely; reject when `q.GroupBy != nil`.

### Translation

- In the flat-mode ORDER BY loop (translator.go:91-99): `Random` clauses emit
  `.Order("RANDOM()")` with no direction suffix.
- EXPLAIN works for free (the SQL shows `ORDER BY RANDOM()`).

### Known caveat (documented, not fixed)

`LIMIT`/`OFFSET` pagination re-rolls the ordering on every request, so page 2
of a random ordering is another random sample that can repeat page 1's rows.
That is the expected semantics of "give me N random items"; the docs say so
explicitly. The MRQL default LIMIT (500) applies as usual.

## 6c. ORDER BY RANK

### Syntax

```
type = note AND TEXT ~ "kubernetes" ORDER BY RANK
type = note AND TEXT ~ "kubernetes" ORDER BY RANK DESC   (least relevant first)
```

### Semantics (decisions)

- `RANK` is a context-sensitive single-part sort key, exactly like
  `distance`: not a lexer keyword, parses as a plain field, gains meaning
  during validation. Field or meta keys named `rank` keep working everywhere
  else.
- **Default direction (ASC) means most relevant first.** MRQL keeps its
  uniform "default ASC" grammar; instead, the rank expression is defined so
  that smaller sorts better (a rank: 1st, 2nd, 3rd). On SQLite the raw
  `bm25()` value is already smaller-is-better; on Postgres we emit
  `-ts_rank(...)`. `ORDER BY RANK` therefore does the thing users want with
  no direction, and no parser special-casing of defaults is needed.
- Requires **exactly one** `TEXT ~` predicate in the WHERE clause (its term
  defines the rank), mirroring `validateDistanceOrderKey`
  (validator.go:233-264): zero is "ORDER BY RANK requires a TEXT ~ predicate",
  two or more is "ambiguous with multiple TEXT predicates".
- Requires a determined single entity type. Cross-entity queries
  (`EntityUnspecified` with type-guarded OR branches) are rejected: bm25 and
  ts_rank values are not comparable across different corpora, so a merged
  cross-entity ranking would be fiction.
- Rejected with GROUP BY (same rule as `distance`, validator.go:241-247).
- **FTS unavailable is a translation error**, not a silent fallback. When the
  `search_vector` column (PG) or `<table>_fts` table (SQLite) is missing
  (`-skip-fts` deployments), `translateTextSearch` silently degrades to
  LIKE, but a rank over a LIKE has no value to order by. Emitting an
  arbitrary order would silently lie; error message:
  "ORDER BY RANK requires the full-text index (server started with FTS
  disabled)". The probe reuses the exact existence checks
  translateTextSearch already does (translator.go:1450-1454, 1480-1481).
- **Empty-term no-op stays a no-op.** If the sanitized term is empty on
  SQLite, `translateTextSearch` drops the predicate (translator.go:1474-1476);
  ORDER BY RANK then also drops silently. The predicate and the ordering
  degrade together, never independently.

### Translation

The translator scans the WHERE AST up front for the single `TextSearchExpr`
(same pattern as `findSimilarTarget`, translator.go:151-172) and stores it on
`translateContext`. `resolveOrderByColumn` (translator.go:1630) gets a `rank`
case ahead of field lookup, gated on the stored text search, emitting inline
SQL (GORM `Order()` takes no binds, per the SIMILAR TO precedent):

SQLite (smaller bm25 = better match; bm25 of a match is negative, so the
sentinel 1e9 pins rows that do not match the FTS query, which can exist when
the TEXT predicate sits under OR/NOT, to the end):

```sql
COALESCE((
  SELECT bm25(resources_fts) FROM resources_fts
  WHERE rowid = resources.id AND resources_fts MATCH '<term>'
), 1e9)
```

Postgres (`ts_rank` of a non-match is 0, matches are > 0, so negation orders
matches first naturally; COALESCE covers NULL `search_vector` rows):

```sql
COALESCE(-ts_rank(resources.search_vector,
                  plainto_tsquery('english', '<term>')), 0)
```

Term inlining safety, per dialect:

- SQLite: the inlined term is the `sanitizeFTS5` output, whose alphabet is
  `[a-zA-Z0-9 .,]` (translator.go:1715-1731). It cannot contain a quote, so
  inlining inside `'...'` is SQL-injection-safe by construction. Note the
  sanitizer does not neutralize everything at the FTS5-MATCH level: the
  word-form operators `AND`/`OR`/`NOT`/`NEAR` pass through (they are
  alphanumeric), and a retained `.` or `,` can be an FTS5 MATCH syntax
  error. Both hazards already exist identically in today's `TEXT ~`
  predicate, which binds the same sanitized term; the rank subquery inlines
  that same string, so predicate and ordering always succeed or fail
  together, and the ordering never widens the attack or error surface.
- Postgres: the raw term is inlined as a standard SQL string literal with
  single quotes doubled (`'` becomes `''`). `standard_conforming_strings` is
  on by default on every supported PG version, so backslashes are literal and
  quote-doubling is sufficient. `plainto_tsquery` treats the string as plain
  text, so no tsquery-operator injection exists on top.

These shapes intentionally mirror `fts/sqlite.go:225-261` and
`fts/postgres.go:151-183` but stay self-contained in the translator, matching
how `translateTextSearch` is already self-contained rather than importing
`fts/` (whose `GetRankExpr` returns bind vars that `Order()` cannot take
anyway).

### Validation

- New `validateRankOrderKey` next to `validateDistanceOrderKey`, wired into
  the same short-circuit spot in `Validate` (validator.go:199-204): checks
  determined entity, no GROUP BY, exactly one `TextSearchExpr`
  (a `collectTextSearchExprs` sibling of `collectSimilarToExprs`).

## 6d. Regex match on Postgres

### Syntax

```
name ~* "^IMG_[0-9]{4}"          case-insensitive POSIX regex match
name !~* "\.(tmp|bak)$"          negated
```

- New operators `~*` and `!~*`, matching PostgreSQL's own case-insensitive
  regex operators, so the MRQL spelling and the emitted SQL coincide.
  Case-insensitive was chosen to match the existing `~` operator's ILIKE
  semantics; there is no case-sensitive variant in v1 (PG's `~` spelling is
  already taken by MRQL's contains operator).
- Unlike `~`, the pattern is a real POSIX ERE: no MRQL wildcard conversion
  (`convertMRQLWildcards` does not apply), no implicit anchoring, no implicit
  `%...%` wrapping.
- **PostgreSQL only.** SQLite has no native regex; on SQLite the operator is
  a translation-time error: "regex match (~*) requires PostgreSQL". The
  validator stays dialect-blind (it has no db handle); the translator is
  where dialect exists (`isPostgres()`), and TranslateErrors surface as
  400s on the determined-entity path
  (`server/api_handlers/mrql_api_handlers.go` passes
  `http.StatusBadRequest`). The cross-entity executor, however, swallows
  per-entity TranslateErrors by design (`executeCrossEntity` skips the
  entity, `application_context/mrql_context.go:607-610`), which would turn
  a SQLite regex query without a `type =` filter into silent empty results.
  To keep the 400 in that path too, `application_context` gates up front:
  a small `mrql.ContainsRegexOperator(q)` AST scan runs before execution
  (both paths share the entry point), returning the same error when the
  dialect is SQLite. The per-comparison TranslateError stays as
  defense-in-depth for direct `Translate` callers.
- Allowed on string fields, meta fields (text extraction via `->>` /
  `json_extract`, the existing `metaJsonExpr` path), and traversal-chain
  string leaves. Rejected on number, datetime, and relation fields
  ("field %q is numeric/datetime and does not support regex match"; relation
  fields already enumerate their operators in the validator error at
  validator.go:420-429, which gains nothing since `~*` is simply not in the
  allowed set).
- Value must be a string (or a `$param` that binds to one).
- Invalid regex patterns are **not pre-validated**. Go's RE2 and PG's ARE
  dialects disagree (RE2 rejects backreferences that PG accepts), so
  pre-compiling with `regexp` would reject valid queries. The pattern goes
  to PG as a bind value; a PG "invalid regular expression" error (SQLSTATE
  2201B) comes back as a query-execution error. The MRQL run handler maps
  that SQLSTATE to a 400 with the PG message so the UI shows "invalid
  regular expression" instead of a 500.

### Grammar / lexing

- Extend the `~` case in `next()` (lexer.go:87-89) to peek one char for `*`,
  emitting `TokenRegex` (`~*`, length 2); same for the `!~` branch
  (lexer.go:110-113) emitting `TokenNotRegex` (`!~*`, length 3). Longest
  match first, so `~` and `!~` are unaffected.
- Parser: add both tokens to the comparison-operator case
  (parser.go:417); they flow through `parseComparison` into
  `ComparisonExpr.Operator` like every other operator.

### Translation

- `sqlOperator` (translator.go:1577) gains `TokenRegex` -> `~*` and
  `TokenNotRegex` -> `!~*`, but `translateComparisonExpr` guards with
  `isPostgres()` first and returns the SQLite TranslateError above. The
  pattern is a normal bind parameter (this is WHERE, not ORDER BY; binds are
  fine).

## Cross-cutting changes

- `mrql/completer.go`:
  - `operators` slice (completer.go:13): add
    `{Value: "BETWEEN", Label: "range (inclusive)"}` and
    `{Value: "~*", Label: "regex match (PostgreSQL)"}`, `!~*` likewise.
  - ORDER BY context (completer.go:634-662): always suggest `RANDOM()`;
    suggest `RANK` when the query contains a `TEXT ~` predicate (mirror the
    `distance`/SIMILAR TO gate at completer.go:645-647).
  - After `BETWEEN <value>`, suggest `AND`.
- `application_context/mrql_generation.go`: prompt rules + example mappings
  for all four ("photos between March and May" -> BETWEEN, "20 random
  unrated photos" -> ORDER BY RANDOM() LIMIT 20, "most relevant notes
  about X" -> TEXT ~ + ORDER BY RANK). The regex rule is included **only
  when the server runs Postgres** (the generation context lives in
  `application_context`, which knows the dialect), so the model is never
  taught syntax the deployment rejects.
- `mrql/generation_lint.go`: expected zero changes, verified by tests.
  BETWEEN desugars into existing nodes; `walkGeneratedNode`
  (defined generation_lint.go:62-74, invoked on `q.Where` only from
  `LintGeneratedQuery` at line 33) never dispatches on operator tokens and
  never sees ORDER BY clauses, so new operators and `Random` pass through
  untouched.
- `mrql/explain.go`: nothing expected; EXPLAIN goes through the same
  translator (DryRun/ToSQL). Covered by tests only.
- List-page filter bars (`ParseFilter`, parser.go:157-189): BETWEEN and
  `~*` work automatically (WHERE-level). RANDOM()/RANK are ORDER BY and
  therefore out of `ParseFilter`'s accepted grammar, unchanged.
- Docs: `docs-site/docs/features/mrql-reference.md` (Operators table rows
  for BETWEEN/`~*`; new ordering entries for RANDOM()/RANK),
  `docs-site/docs/features/mrql.md` (Pattern Matching and
  Ordering-and-Pagination sections), `.claude/skills/mahresources-cli/references/mrql.md`,
  `cmd/mr/commands/mrql_help/*.md` where grammar is shown (then
  `./mr docs lint` / `check-examples` must pass).

## New error messages

| Condition | Message |
|---|---|
| direction after RANDOM() | `RANDOM() does not take a direction` |
| RANDOM() with GROUP BY | `ORDER BY RANDOM() is not supported with GROUP BY` |
| RANK without TEXT | `ORDER BY RANK requires a TEXT ~ predicate in the query` |
| RANK with 2+ TEXT | `ORDER BY RANK is ambiguous with multiple TEXT predicates; use exactly one` |
| RANK with GROUP BY | `ORDER BY RANK is not supported with GROUP BY` |
| RANK cross-entity | `ORDER BY RANK requires a single entity type (add a type = ... filter)` |
| RANK, FTS disabled | `ORDER BY RANK requires the full-text index (server started with FTS disabled)` |
| `~*` on SQLite | `regex match (~*) requires PostgreSQL` |
| `~*` on non-string field | `field %q does not support regex match` |
| BETWEEN missing AND | `expected AND between BETWEEN bounds, got %q` |

## Testing plan (red then green, per feature)

- `mrql/between_test.go`: parser desugar shape (AST equality with the
  hand-built `>=`/`<=` tree, incl. NOT BETWEEN and position preservation),
  SQL shape, execution against seeded rows (dates incl. `-7d`/`NOW()`
  bounds, fileSize with units, meta values), `$param` bounds through
  `BindParams`, filter-bar acceptance via `ParseFilter`, `meta.between` as a
  key still parses.
- `mrql/order_random_test.go`: parse + validation (direction rejected,
  GROUP BY rejected), SQL contains `ORDER BY RANDOM()`, execution returns
  the full row set, EXPLAIN shape. No PG file needed (dialect-neutral SQL);
  one case in `translator_pg_test.go` for the emitted SQL.
- `mrql/order_rank_test.go` + `mrql/order_rank_pg_test.go`: validation
  matrix (no TEXT / two TEXTs / GROUP BY / cross-entity), SQL shape incl.
  the COALESCE sentinels and quote-doubling (hostile terms:
  `o'brien`, `"; DROP TABLE`, FTS operators), execution ordering against
  seeded FTS content on both dialects (FTS triggers populate synchronously
  on insert), empty-term no-op, FTS-disabled error (probe stubbed or
  `-skip-fts` style setup), and precedence pins: a single-part `rank` with
  exactly one TEXT predicate always means the relevance key (no entity has a
  real `rank` column, so nothing is shadowed; without a TEXT predicate it
  stays an unknown field, same as today), and `meta.rank` (two parts) is
  never captured.
- `mrql/regex_pg_test.go`: match/negation/case-insensitivity/meta fields/
  traversal leaf, invalid-pattern surfaces SQLSTATE 2201B as 400 (api test),
  param-bound pattern. SQLite rejection lives in `translator_test.go`
  (determined entity) plus an `application_context` test pinning the
  up-front gate: regex without a `type =` filter on SQLite returns the 400,
  not silent empty results.
- Lexer cases (`~*`, `!~*`, `~ *` with space stays TokenLike + illegal),
  completer cases, generation prompt/lint cases (regex rule present on PG,
  absent on SQLite).
- E2E: one spec exercising the resources filter bar with BETWEEN and the
  MRQL page with `ORDER BY RANDOM() LIMIT n` (result count only) and
  `TEXT ~ ... ORDER BY RANK` (top result is the seeded best match). Regex
  e2e only in the Postgres suite (`test:with-server:postgres`).
- Full suites per repo policy: `go test --tags 'json1 fts5' ./...`, the PG
  tagged run, and both e2e suites (`test:with-server:all`).

## Implementation order (each step red then green)

1. **6a BETWEEN** (parser-only + tests): smallest, zero translator risk.
2. **6b ORDER BY RANDOM()** (parser flag + validator guard + one Order call).
3. **6d regex** (lexer tokens + sqlOperator case + dialect guard + SQLSTATE
   mapping in the run handler).
4. **6c ORDER BY RANK** (validator + translator context + inline SQL +
   dialect tests): the largest, done last with the patterns from 2 and 3
   fresh.
5. Cross-cutting sweep: completer, generation prompt/lint, docs, CLI help,
   e2e. `./mr docs lint` and `./mr docs check-examples` green.

## Explicitly out of scope (v1)

- A dedicated `BetweenExpr` AST node or query re-rendering from the AST.
- Case-sensitive regex, regex on SQLite (registering a Go `regexp` UDF on
  the SQLite connection is feasible but diverges from PG's ARE dialect;
  revisit only if demand shows up), and Go-side pattern pre-validation.
- Exposing the rank value as a selectable/returned field (only sort order,
  same decision as `distance` in package 3).
- `ORDER BY RANK` for cross-entity queries (non-comparable scores).
- Weighted or bm25-parameterized ranking configuration; both dialects use their
  defaults (`plainto_tsquery('english', ...)`, unweighted `bm25()`).
- Stable random sampling (seeded RANDOM) or repeat-free random pagination.
