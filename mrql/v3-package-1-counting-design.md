# MRQL Package 1 Design: Counting and Aggregation

Design for the four features in Package 1 of `v3-packages.md`:

- **1a** `HAVING` for aggregated GROUP BY
- **1b** relation fields `notes` and `resources` (currently missing from `fields.go`)
- **1c** relation counts: `tags.count > 5`
- **1d** date bucketing in GROUP BY: `created.month`

Motivating queries, none expressible today:

```
type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC
type = resource AND notes IS EMPTY AND created > -30d
type = group AND resources.count >= 100 ORDER BY resources.count DESC
type = note GROUP BY created.month COUNT() ORDER BY created.month ASC
type = resource AND tags.count = 0 AND groups IS EMPTY
```

The features interlock: 1b provides the relation descriptors that 1c counts over,
1a and 1d both extend the GROUP BY pipeline, and the two new dotted pseudo-fields
(`.count`, `.month`) share one validation pattern.

## Current state (code references)

- Field catalog: `mrql/fields.go:22-60`. Relations today: `tags` (all entities),
  `groups`/`group` (resource, note), `owner` (resource, note), `parent`,
  `children` (group). No `notes` on resource, no `resources` on note, no
  `resources`/`notes` on group, even though the models define these
  associations (`models/resource_model.go:39` `many2many:resource_notes`,
  `models/group_model.go:27-28` `many2many:groups_related_resources`,
  `groups_related_notes`).
- Relation translation is per-relation, near-duplicated:
  `translateTagComparison` (`mrql/translator.go:651`) and
  `translateGroupComparison` (`mrql/translator.go:700`) emit the same
  `id IN (SELECT jt.<col> FROM <junction> jt JOIN <other> x ON ... WHERE LOWER(x.name) ...)`
  shape. `translateRelationIn` (`mrql/translator.go:1046`) and
  `translateRelationIsEmpty` (`mrql/translator.go:1179`) repeat the same
  junction dispatch.
- GROUP BY has two modes (`mrql/translator.go:1498-1539`): aggregated
  (`translateAggregatedGroupBy`, `mrql/translator.go:1542`) when aggregates are
  present, bucketed otherwise (`TranslateGroupByKeys` / `TranslateGroupByBucket`
  in `mrql/translator_groupby.go`, driven by `executeBucketedQuery` in
  `application_context/mrql_context.go:277`). Both modes resolve field
  expressions through `groupByFieldExprs` (`mrql/translator.go:1664`,
  `mrql/translator_groupby.go:71,162`).
- There is no HAVING anywhere in the package.
- Aggregate SELECT aliases are `count` and `<func>_<field>` (e.g.
  `sum_fileSize`), built by `aggregateExpr` (`mrql/translator.go:1836`), and are
  the only valid aggregated-mode ORDER BY keys (`buildAggregateOrderKeys`,
  `mrql/validator.go:740`).
- Dotted fields already parse to multi-part `FieldExpr` (`parseField`,
  `mrql/parser.go:305`, up to 8 parts). Multi-part validation routes through
  `validateFieldExpr` (`mrql/validator.go:477`): `meta.*` is accepted, anything
  else must be a traversal chain rooted at `owner`/`parent`/`children`.
- The lexer only emits `TokenCount` etc. when the word is immediately followed
  by `(` (`mrql/lexer.go:272`), so `count` after a dot lexes as a plain
  identifier. No lexer change is needed for `.count`.

## Feature 1b: relation fields `notes` and `resources`

Foundation for 1c, and useful standalone (`notes IS EMPTY`).

### New fields

| Entity   | Field       | Junction table             | Entity column | Related table | Related column |
|----------|-------------|----------------------------|---------------|---------------|----------------|
| resource | `notes`     | `resource_notes`           | `resource_id` | `notes`       | `note_id`      |
| note     | `resources` | `resource_notes`           | `note_id`     | `resources`   | `resource_id`  |
| group    | `resources` | `groups_related_resources` | `group_id`    | `resources`   | `resource_id`  |
| group    | `notes`     | `groups_related_notes`     | `group_id`    | `notes`       | `note_id`      |

Add to `resourceFields`, `noteFields`, `groupFields` in `mrql/fields.go` with
`Type: FieldRelation`.

Semantics on Group are junction-based ("related"), mirroring how `groups` on a
resource uses `groups_related_resources` today. The ownership direction is
already queryable from the other side via `owner`. No `relatedGroups` field in
v1; typed `GroupRelation` links are a different concept and out of scope.

### Supported operations

Exactly the set relations support today: `=`, `!=`, `~`, `!~` (match by related
entity name, case-insensitive), `IN` / `NOT IN`, `IS [NOT] EMPTY`. Matching by
name is consistent with `tags` and `groups`; matching notes by name is the
right default since note names are the human handle.

### Implementation: generalize the junction translator

Introduce one descriptor type and a lookup, replacing the four copies of the
junction dispatch:

```go
type junctionRelation struct {
    junctionTable string // e.g. "resource_notes"
    entityCol     string // FK to the queried entity, e.g. "resource_id"
    relatedTable  string // e.g. "notes"
    relatedCol    string // FK to the related entity, e.g. "note_id"
}

// lookupJunction(entityType, fd.Column) -> (junctionRelation, bool)
```

The map covers existing relations (`tags` x3, `groups` x2) plus the four new
rows. `translateTagComparison` and `translateGroupComparison` collapse into one
`translateJunctionComparison(db, rel, op, val)`; `translateRelationIn` and
`translateRelationIsEmpty` switch their hard-coded `switch tc.entityType`
blocks to the same lookup. Generated SQL for existing queries must remain
byte-identical (assert in tests) so this is a pure refactor plus new rows.

`owner`, `parent`, `children` keep their FK-based paths
(`mrql/translator.go:630-641`), unchanged.

### Traversal interaction

`notes` and `resources` are terminal relation fields, not traversal roots.
`notes.name = "x"` is not part of this package (only `owner`/`parent`/
`children` chain, `mrql/translator.go:110`); the existing "unknown field"
validation error applies. `.count` (1c) is the one new dotted suffix.

## Feature 1c: relation counts

### Syntax

```
<relation>.count <op> <number>        in WHERE
ORDER BY <relation>.count [ASC|DESC]  in list mode
```

`<op>` is one of `=`, `!=`, `>`, `>=`, `<`, `<=`. `<relation>` is any
`FieldRelation` on the entity backed by a junction (`tags`, `groups`/`group`,
`notes`, `resources`) plus `children` on group. The value must be a
non-negative integer `NumberLiteral` without a unit.

Not supported, with targeted validator errors:

- `owner.count`, `parent.count`: single FK, not a collection. Error suggests
  `owner IS NULL` / `parent IS NULL`.
- `<relation>.count IN (...)`, `IS EMPTY/NULL`, `~`: error suggests a
  comparison operator.
- `.count` in GROUP BY: out of scope for v1 (grouping by a computed count is a
  histogram feature; revisit with demand).

### Parsing and lexing

None needed. `tags.count` parses as a 2-part `FieldExpr` today
(`mrql/parser.go:305`); `count` not followed by `(` lexes as an identifier
(`mrql/lexer.go:272`).

### Validation

In `validateFieldExpr` (`mrql/validator.go:477`), before the traversal-chain
branch: a 2-part field whose first part resolves to a countable relation (via
the 1b junction lookup, or `children`) and whose second part is `count` is a
valid count pseudo-field. `validateComparison` gains a branch enforcing the
operator and value rules above. ORDER BY validation reuses `validateFieldExpr`
(non-aggregated mode, `mrql/validator.go:75-88`), so `ORDER BY tags.count`
becomes valid with no extra work; aggregated GROUP BY mode keeps its own key
allowlist and rejects it.

### Translation

Correlated scalar subquery, same for both dialects:

```sql
-- junction-backed relations
(SELECT COUNT(*) FROM <junction> jt WHERE jt.<entityCol> = <table>.id) <op> ?

-- children (reverse FK on groups.owner_id)
(SELECT COUNT(*) FROM groups c WHERE c.owner_id = groups.id) <op> ?
```

Emitted from `translateComparisonExpr` (`mrql/translator.go:540`) when the
field is a count pseudo-field. `COUNT(*)` over an empty correlated set yields
0, so `tags.count = 0` matches entities with no junction rows without special
casing (unlike `NOT IN` emptiness). `resolveOrderByColumn`
(`mrql/translator.go:1414`) returns the same subquery expression for ORDER BY;
both SQLite and Postgres accept scalar subqueries in ORDER BY.

Junction tables carry composite primary keys on the two FK columns, so the
correlated `COUNT(*)` is an index-only range scan per row on both dialects.
For `ORDER BY <relation>.count` over large unfiltered tables the subquery runs
per candidate row; acceptable for v1, and the deployments-with-millions note in
CLAUDE.md is addressed by the existing default LIMIT
(`-mrql-default-limit`, default 500).

## Feature 1a: HAVING

### Syntax

```
GROUP BY <fields> <aggregates> HAVING <having-expr> [ORDER BY ...] [LIMIT ...]

having-expr    := having-and (OR having-and)*
having-and     := having-unary (AND having-unary)*
having-unary   := [NOT] having-primary
having-primary := "(" having-expr ")"
                | aggregate-func <op> <value>
```

`aggregate-func` is the existing production (`parseAggregateFunc`,
`mrql/parser.go:604`): `COUNT()` or `SUM|AVG|MIN|MAX(field)`. `<op>` is
`=`, `!=`, `>`, `>=`, `<`, `<=`.

Examples:

```
type = resource GROUP BY hash COUNT() HAVING COUNT() > 1
type = resource GROUP BY tags COUNT() SUM(fileSize) HAVING SUM(fileSize) > 1gb AND COUNT() >= 10
type = note GROUP BY noteType COUNT() HAVING NOT (COUNT() < 5)
```

### Scope rules

- **Aggregated mode only.** HAVING requires at least one aggregate in the
  GROUP BY clause. Bucketed mode (`executeBucketedQuery`,
  `application_context/mrql_context.go:277`) materializes buckets and items
  through a separate path; filtering buckets there is a stretch goal (see
  Phasing). Error when aggregates are absent:
  `HAVING requires at least one aggregate function in GROUP BY (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1)`.
- The HAVING expression may use aggregates that do not appear in the SELECT
  aggregate list (standard SQL; translation repeats the expression, so nothing
  depends on the SELECT list).
- Plain fields on the HAVING left side are rejected:
  `HAVING conditions must use aggregate functions; filter plain fields in the WHERE clause instead`.

### AST and parsing

- `token.go`: new `TokenHaving`; `keywordMap` gains `"HAVING"`
  (`mrql/lexer.go:299`).
- `ast.go`: `GroupByClause` gains `Having Node`
  (`mrql/ast.go:135`). New node for the leaf, since `ComparisonExpr` requires a
  `*FieldExpr` left side:

  ```go
  type HavingComparison struct {
      Agg      AggregateFunc
      Operator Token
      Value    Node // NumberLiteral, or date value for MIN/MAX on datetime
  }
  ```

  Boolean structure reuses `BinaryExpr` and `NotExpr`.
- `parseGroupBy` (`mrql/parser.go:569`): after the aggregate loop, if the next
  token is `TokenHaving`, parse the expression with a small dedicated
  recursive-descent parser (mirrors `parseOrExpr`/`parseAndExpr`/
  `parseNotExpr` but with `HavingComparison` leaves). A dedicated parser keeps
  aggregate-call left sides out of the main expression grammar.

### Validation

In `validateGroupBy` (`mrql/validator.go:622`), walk `gb.Having`:

- Each `HavingComparison.Agg` passes the same rules as SELECT aggregates
  (SUM/AVG numeric, MIN/MAX numeric-or-datetime, field exists;
  `mrql/validator.go:677-718`). Extract the existing per-aggregate checks into
  a helper shared by both loops.
- Value typing mirrors `validateComparisonValue` (`mrql/validator.go:458`):
  `NumberLiteral` for COUNT/SUM/AVG and for MIN/MAX on numeric fields;
  `StringLiteral`/`RelDateLiteral`/`FuncCall` additionally allowed for MIN/MAX
  on `FieldDateTime` fields (`HAVING MAX(created) < -1y` finds stale buckets).

### Translation

In `translateAggregatedGroupBy` (`mrql/translator.go:1542`), after the
`db.Group` calls: walk the Having tree, building one SQL string plus a value
slice, and apply with `db.Having(sql, vals...)` (GORM supports `Having` on
grouped queries). Leaves render via `aggregateExpr`
(`mrql/translator.go:1836`) dropping the alias:

```sql
HAVING COUNT(*) > ? AND SUM(CAST(... AS ...)) > ?
```

The aggregate expression is repeated rather than referencing the SELECT alias:
PostgreSQL does not permit SELECT aliases in HAVING (SQLite tolerates them;
emit the portable form).

Value resolution matches `resolveValue` (`mrql/translator.go:1304`): a
`NumberLiteral` with a size unit uses `.Raw` (bytes) when the aggregate field
column is `file_size`, so `HAVING SUM(fileSize) > 1gb` works; date values for
MIN/MAX resolve through the existing relative-date/function resolvers.

### Known caveat (pre-existing, documented not fixed)

When GROUP BY includes a junction relation, `groupByRelationJoins`
(`mrql/translator.go:1691`) joins the junction table, so `COUNT(*)` counts join
rows. Grouping by a single relation yields correct per-bucket entity counts
(one row per entity per bucket); grouping by two relations simultaneously
multiplies rows. HAVING inherits this existing aggregated-mode behavior
unchanged. Add a note to the MRQL docs page.

## Feature 1d: date bucketing

### Syntax

```
GROUP BY created.month
GROUP BY updated.week COUNT()
... ORDER BY created.month ASC          (aggregated mode)
```

Suffixes: `day`, `week`, `month`, `year` on any `FieldDateTime` field of the
entity (`created`, `updated`; `mrql/fields.go:26-27`). Dot syntax is chosen
over function syntax (`MONTH(created)`) for consistency with `.count` and
because it needs no lexer change.

Valid only in GROUP BY fields and as aggregated-mode ORDER BY keys. In WHERE,
date ranges already cover filtering; `created.month = "2026-07"` stays a
validation error:
`date bucket fields are only valid in GROUP BY; use a date range in WHERE (created >= "2026-07-01" AND created < "2026-08-01")`.

### Validation

`validateGroupBy` currently sends 2-part fields into `validateFieldExpr`, which
rejects anything that is not `meta.*` or a traversal (`mrql/validator.go:490`).
Add a bucket pseudo-field recognizer alongside the `.count` one: 2-part chain,
first part `FieldDateTime` on the entity, second part in the suffix set. The
recognizer is accepted from GROUP BY context only. `buildAggregateOrderKeys`
picks the name up automatically via `AllFieldNames`
(`mrql/validator.go:740-745`), making `ORDER BY created.month` valid in
aggregated mode; bucketed-mode ORDER BY flows through the same alias map
(`buildGroupByAliasMap`, `mrql/translator.go:1623`).

### Translation

`groupByFieldExprs` (`mrql/translator.go:1664`) gains a bucket branch returning
the same expression for SELECT and GROUP BY. Bucket labels are strings whose
lexicographic order equals chronological order, so ordering by the alias needs
no extra machinery:

| Suffix  | Label format | PostgreSQL                                        | SQLite                                     |
|---------|--------------|---------------------------------------------------|--------------------------------------------|
| `day`   | `YYYY-MM-DD` | `to_char(created_at, 'YYYY-MM-DD')`               | `strftime('%Y-%m-%d', created_at)`         |
| `week`  | `YYYY-MM-DD` (Monday of the week) | `to_char(date_trunc('week', created_at), 'YYYY-MM-DD')` | `date(created_at, '-6 days', 'weekday 1')` |
| `month` | `YYYY-MM`    | `to_char(created_at, 'YYYY-MM')`                  | `strftime('%Y-%m', created_at)`            |
| `year`  | `YYYY`       | `to_char(created_at, 'YYYY')`                     | `strftime('%Y', created_at)`               |

`date_trunc('week', ...)` truncates to Monday; the SQLite expression
`date(x, '-6 days', 'weekday 1')` also resolves to the Monday on or before `x`
(back up six days, then advance to the next Monday), so week buckets agree
across dialects. Timestamps are bucketed as stored (UTC in practice); no
timezone parameter in v1.

The SELECT alias is the field name as written (`... AS "created.month"`);
quoted aliases containing a dot are legal in both dialects, and the existing
ORDER BY quoting (`mrql/translator.go:1597`) already wraps aliases in double
quotes.

Because both bucketed-mode translators resolve fields through
`groupByFieldExprs` (`mrql/translator_groupby.go:71` for keys,
`:162` for the per-bucket equality filter), bucketed mode
(`GROUP BY created.month` with no aggregates, listing items per month) works
with no additional code. Per-bucket filters compare
`<bucket expr> = <key value>` on the label string, which round-trips exactly.

## Cross-cutting changes

- **Autocomplete** (`mrql/completer.go`): suggest `HAVING` after aggregate
  functions (`postAggregateKeywords`, `mrql/completer.go:48`); suggest
  aggregate functions again after `HAVING`; suggest `<relation>.count` where
  relation fields are suggested; suggest `created.month` (and siblings) in
  GROUP BY field position; add the new `notes`/`resources` relation fields to
  the entity field suggestions (driven by `fields.go`, so mostly automatic).
- **NL generation**: the DeepSeek system prompt describing MRQL grammar
  (`application_context/mrql_generation.go`) must document HAVING, `.count`,
  bucket suffixes, and the new relation fields, and the post-generation linter
  (`mrql/generation_lint.go`) must accept them.
- **Scope**: `SCOPE` composes untouched; it is applied as a WHERE-level CTE
  before grouping (`mrql/translator.go:1528-1531`) and before HAVING.
- **API surface**: no new endpoints. `/v1/mrql` and `/v1/mrql/validate`
  responses are shape-compatible; aggregated rows gain no new key kinds
  (HAVING only filters rows).
- **Auth**: no new access paths; everything flows through the existing
  translate pipeline, so group-limited principals keep their scope enforcement.
- **Docs**: MRQL docs page and `cmd/mr/commands/mrql_help/*.md` examples
  (CI enforces freshness via `./mr docs lint`); update `templates/mrql.tpl`
  help panel if it lists syntax.

## Error messages (new)

| Input | Error |
|---|---|
| `GROUP BY hash HAVING COUNT() > 1` (no aggregates) | `HAVING requires at least one aggregate function in GROUP BY (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1)` |
| `... HAVING name = "x"` | `HAVING conditions must use aggregate functions; filter plain fields in the WHERE clause instead` |
| `owner.count > 1` | `owner is a single reference and cannot be counted; use owner IS NULL / IS NOT NULL` |
| `tags.count IN (1, 2)` | `tags.count only supports comparison operators (=, !=, >, >=, <, <=)` |
| `tags.count > "many"` | `tags.count must be compared to a non-negative integer` |
| `created.month = "2026-07"` in WHERE | `date bucket fields are only valid in GROUP BY; use a date range in WHERE (created >= "2026-07-01" AND created < "2026-08-01")` |

All errors carry position and length for editor squiggles, matching the
existing `ValidationError` pattern.

## Testing plan (TDD, red then green)

Order of implementation follows the dependency chain: 1b, 1c, 1a, 1d.

1. **Refactor safety net first**: table-driven tests asserting the exact SQL
   currently generated for `tags`/`groups` comparisons, IN, and IS EMPTY on all
   three entities (extend `translator_test.go`), then perform the junction
   refactor against them.
2. **Unit tests per feature** in `mrql/`: lexer (HAVING keyword), parser
   (HAVING grammar including precedence and parenthesization, error positions),
   validator (every error-message row above, plus happy paths per entity),
   translator (generated SQL on SQLite and, via `translator_pg_test.go`
   patterns, Postgres; HAVING value binding including `1gb` byte conversion;
   count subqueries; bucket expressions per dialect).
3. **Execution tests** in `server/api_tests/`: seed entities, run the
   motivating queries end to end, assert row contents (duplicate-hash
   detection, notes-empty, per-month counts, count ordering). Run with
   `--tags 'json1 fts5'` and the Postgres variants
   (`--tags 'json1 fts5 postgres'`).
4. **Completer tests** (`mrql/completer_test.go`): HAVING and pseudo-field
   suggestions at the right cursor positions.
5. **Generation lint tests** (`mrql/generation_lint_test.go`): generated
   queries using the new syntax pass the linter.
6. **E2E**: one browser spec on the MRQL page running a HAVING query and a
   `created.month` aggregation, asserting the rendered table; one CLI spec via
   `mr mrql run`. Rebuild `./mahresources` before E2E (stale-binary pitfall).

## Phasing

1. **Phase 1 (1b)**: junction descriptor refactor + new relation fields.
   Pure additive query surface, no grammar change.
2. **Phase 2 (1c)**: `.count` pseudo-field, WHERE and ORDER BY.
3. **Phase 3 (1a)**: HAVING, aggregated mode.
4. **Phase 4 (1d)**: date bucket pseudo-fields, both GROUP BY modes.
5. **Stretch (post-package)**: HAVING in bucketed mode (filter the keys query
   in `TranslateGroupByKeys`; requires counting distinct base-entity ids when
   relation joins are present), and `.count` in GROUP BY (count histograms).

Each phase lands with its docs, completer, and generation-prompt updates.

## Open questions

1. Should `notes = "x"` on a resource also match note **content** rather than
   only the name? Recommendation: name only, consistent with other relations;
   content search stays `TEXT ~` on `type = note`.
2. `group.resources`/`group.notes` are junction-based ("related"). Is an
   ownership-based variant (`ownedResources.count`) needed? Recommendation:
   defer; `owner = "name"` from the resource side covers listing, and adding
   both would force users to understand the distinction up front.
3. Week bucket label: start-of-week date (`2026-06-29`) vs ISO week number
   (`2026-W27`). Recommendation: start-of-week date; it is unambiguous,
   sortable, and cheap to compute identically on both dialects.
