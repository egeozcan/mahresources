# MRQL v3 Capability Packages

Proposed follow-up work for MRQL, grouped into deliverable packages. Supersedes the
remaining items in `v2-plans.md`. Several v2 items have already shipped and are not
repeated here: GROUP BY with aggregates (bucketed and aggregated modes), the SCOPE
clause with an inline recursive CTE, FK traversal chains up to 8 segments including
chained meta (`owner.meta.rating`), saved queries, autocomplete, and DeepSeek
natural-language generation.

Packages are ordered by usefulness-to-effort ratio. Each package is independently
shippable. Design documents live next to this file (`v3-package-<n>-*.md`).

## Package 1: Counting and Aggregation

Design: `v3-package-1-counting-design.md`

### 1a. HAVING for GROUP BY

The most glaring gap given aggregates already exist.
`type = resource GROUP BY hash COUNT() HAVING COUNT() > 1` is the natural
"find exact duplicates" query, and "tags used more than N times" is a classic
library-hygiene query. Neither is expressible today (`grep HAVING mrql/` comes
back empty). Parser work is small; the aggregated-mode translator
(`translateAggregatedGroupBy`) already builds the SELECT expressions a HAVING
would reference.

### 1b. Relation fields that exist in the models but not in `fields.go`

Resources have a many-to-many with Notes, and Groups own resources/notes, but you
cannot write `notes IS EMPTY` on a resource or `resources IS EMPTY` on a group.
This blocks the very common "find orphaned/unannotated things" workflows and is
cheap: it is just new `FieldDef` entries plus junction-table subqueries of the
same shape as the existing `tags`/`groups` handling.

### 1c. Relation counts as comparable values

`tags.count > 5`, `children.count = 0`, `resources.count >= 100`. Today relations
only support equality and emptiness. A COUNT-subquery comparison unlocks curation
queries ("heavily tagged", "big groups") without needing GROUP BY mode, and
composes with the rest of a WHERE clause.

### 1d. Date bucketing in GROUP BY

`GROUP BY created` currently groups by exact timestamp, which is useless for
stats. A bucket suffix (`created.month`) with dialect-specific truncation
(`to_char` on PG, `strftime` on SQLite) turns MRQL into a real analytics tool
("uploads per month", "notes created per week"). Pairs naturally with the
existing aggregate mode.

## Package 2: Hierarchy Traversal

### 2a. `ancestors.` / `descendants.` recursive traversal

`parent.parent.name` requires knowing the depth; `ancestors.category = "Archive"`
does not. The recursive-CTE machinery already exists in `ApplyScopeCTE`
(`mrql/scope.go`), so this is mostly plumbing it into the traversal-chain
translator as a new chain root.

## Package 3: Similarity Search

### 3a. `SIMILAR TO resource(N)` perceptual-hash predicate

This was deferred in v2 plans, but similarity v2 (pHash + chunk-index matching,
read-time thresholds) has since merged and deployed, so the fast lookup path now
exists. `type = resource AND SIMILAR TO resource(1234) AND tags != "reviewed"`
would combine similarity with regular filters in a way the dedicated similarity
UI cannot. Optional distance: `SIMILAR TO resource(1234) WITHIN 5`.

## Package 4: Saved Queries as Reports

### 4a. Parameterized saved queries

Saved queries are static strings. Named placeholders
(`tags = $tag AND created > $since`) with values supplied at run time
(`/v1/mrql/saved/run` body, `mr mrql run name --param tag=x`) would turn saved
queries into reusable reports and make them far more useful from plugins and
`CustomMRQLResult` templates. Substitution must happen at the AST/value level,
not string interpolation, to stay injection-safe.

### 4b. EXPLAIN endpoint and result export

Both cheap and high-leverage for a power-user feature: `POST /v1/mrql/explain`
returning the generated SQL (the translator already produces a `*gorm.DB`;
`ToSQL` gets the rest), and CSV/JSON export of results, especially valuable now
that GROUP BY produces tabular aggregate data.

## Package 5: Adoption Surfaces

Design: `v3-package-5-adoption-design.md`

### 5a. Query bar on the list pages, MRQL in Cmd+K

The biggest adoption lever is not language features: MRQL currently lives on its
own page, while daily browsing happens on `/resources`, `/notes`, `/groups`. An
MRQL input on those pages (auto-scoped, so `type =` is implied) and MRQL
acceptance in the Cmd+K modal would put the feature where users already are. The
`mrqlEditor.js` Alpine component and the `/v1/mrql/complete` endpoint are
reusable as-is.

## Package 6: Ergonomics

Smaller wins, individually shippable:

- `BETWEEN` sugar for dates and numbers.
- `ORDER BY RANDOM()` (genuinely useful for media libraries: "20 random unrated
  photos").
- FTS relevance ranking (`ORDER BY RANK` when `TEXT ~` is present; both `bm25()`
  and `ts_rank` are available).
- Regex match on Postgres.

## Explicitly deferred

- **Sub-queries** (`group IN (SELECT ...)`): adds a lot of grammar and validator
  complexity; packages 1 and 2 cover most of the real use cases.
- **True UNION ALL cross-entity queries** and **keyset pagination**: performance
  work with no new expressiveness, worth doing only once cross-entity queries see
  real use.
