---
sidebar_position: 21
title: MRQL Reference
description: Quick-lookup DSL cheatsheet for MRQL queries
---

# MRQL Reference

A compact syntax reference for the Mahresources Query Language (MRQL). For the full conceptual overview with background and examples of when to use MRQL, see [MRQL Query Language](./mrql.md).

## Query Shape

```
[type = "resource|note|group" AND] <conditions>
  [SCOPE <group-id-or-name>]
  [GROUP BY <field>[, <field>...] [<aggregates>] [HAVING <aggregate-conditions>]]
  [ORDER BY <field> [ASC|DESC] | RANDOM() | RANK]
  [LIMIT <n>] [OFFSET <n>]
```

## Operators

| Operator | Meaning | Example |
|---|---|---|
| `=` | Equal (case-insensitive for strings) | `name = "Report"` |
| `!=` | Not equal | `contentType != "application/pdf"` |
| `>` `>=` `<` `<=` | Numeric / datetime comparisons | `fileSize > 1mb`, `created >= -30d` |
| `~` | Contains / wildcard pattern (case-insensitive) | `name ~ "project*"`, `contentType ~ "image"` |
| `!~` | Negated pattern match | `contentType !~ "image"` |
| `~*` / `!~*` | Case-insensitive POSIX regex match / negation (**PostgreSQL only**) | `name ~* "^IMG_[0-9]{4}"` |
| `BETWEEN ... AND ...` | Inclusive range (also `NOT BETWEEN`) | `created BETWEEN "2024-01-01" AND "2024-06-30"`, `fileSize NOT BETWEEN 1mb AND 10mb` |
| `IS EMPTY` / `IS NOT EMPTY` | Value is empty/null or has content | `description IS NOT EMPTY` |
| `IS NULL` / `IS NOT NULL` | Meta key absent / present | `meta.rating IS NOT NULL` |
| `IN (...)` / `NOT IN (...)` | Set membership | `contentType IN ("image/png", "image/jpeg")` |
| `AND` `OR` `NOT` | Boolean logic (precedence: NOT > AND > OR) | `tags = "photo" AND NOT tags = "archived"` |

## Fields (by entity type)

**Common to all types:** `id`, `name`, `description`, `created`, `updated`, `tags`, `guid` (stable UUIDv7), `meta.<key>`, `TEXT` (full-text search).

**Resources only:** `groups` (alias `group`), `owner`, `category`, `contentType`, `fileSize`, `width`, `height`, `originalName`, `hash`, `notes`.

**Notes only:** `groups` (alias `group`), `owner`, `noteType`, `resources`.

**Groups only:** `category`, `url`, `parent`, `children`, `resources`, `notes`.

Relation fields (`tags`, `groups`/`group`, `notes`, `resources`, `children`) match related entities by name with `=`, `!=`, `~`, `!~` and support `IS [NOT] EMPTY`. The junction-backed relations (`tags`, `groups`/`group`, `notes`, `resources`) additionally support `IN` / `NOT IN`; `children`, `owner`, and `parent` do not.

## Relation Counts

Compare how many related entities exist with `<relation>.count` and a comparison operator (`=`, `!=`, `>`, `>=`, `<`, `<=`) against a non-negative integer. Valid on `tags`, `groups`/`group`, `notes`, `resources`, and `children` (groups); also valid as an `ORDER BY` key.

```
type = resource AND tags.count = 0
type = group AND resources.count >= 100 ORDER BY resources.count DESC
type = resource AND notes.count >= 1 ORDER BY tags.count DESC
```

`owner` and `parent` are single references and cannot be counted — use `owner IS NULL` / `parent IS NULL` instead. `IN`, `IS EMPTY`, and `~` are not supported on `.count`.

## Relative Dates

| Literal | Meaning |
|---|---|
| `-7d` | 7 days ago |
| `-2w` | 2 weeks ago |
| `-3m` | 3 months ago |
| `-1y` | 1 year ago |

Functions: `NOW()`, `START_OF_DAY()`, `START_OF_WEEK()`, `START_OF_MONTH()`, `START_OF_YEAR()`.

## File Size Units

Accepted on `fileSize` comparisons (case-insensitive): `kb` = 1,024 bytes, `mb` = 1,048,576 bytes, `gb` = 1,073,741,824 bytes.

## CLI Invocation

```bash
# Positional query
mr mrql 'type = resource AND tags = "photo"'

# From a file
mr mrql -f query.mrql

# From stdin
echo 'tags = "photo"' | mr mrql -

# With paging
mr mrql --limit 10 --page 2 'type = note'

# Run a saved query by name or ID
mr mrql run "my-saved-query"
```

## SCOPE — Filter to Group Subtree

```
type = "resource" SCOPE 42 ORDER BY created LIMIT 10
type = "note" SCOPE "My Project"
type = "resource" SCOPE 7 GROUP BY contentType COUNT()
```

- `SCOPE <id>` — group with that ID plus all descendants.
- `SCOPE "name"` — lookup by name (case-insensitive); errors listing all matches if multiple groups share the name.
- Resources / notes scope by `owner_id`; groups scope by `id`.
- Omit `SCOPE` or use `SCOPE 0` for unfiltered queries.

## GROUP BY — Aggregated Mode

Aggregate functions present → flat rows of computed values.

```
type = resource GROUP BY contentType COUNT()
type = resource GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)
type = resource GROUP BY contentType COUNT() ORDER BY count DESC
type = note GROUP BY owner, noteType COUNT()
type = resource AND fileSize > 10mb GROUP BY contentType MIN(fileSize) MAX(fileSize)
```

Output keys: `count`, `sum_{field}`, `avg_{field}`, `min_{field}`, `max_{field}`.

### HAVING — Filter Aggregated Buckets

`HAVING` keeps only buckets whose aggregates match the condition. It requires at least one aggregate function in the `GROUP BY` clause (aggregated mode only) and accepts aggregate comparisons combined with `AND` / `OR` / `NOT` and parentheses. The aggregate in `HAVING` does not need to appear in the aggregate list.

```
type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC
type = resource GROUP BY tags COUNT() SUM(fileSize) HAVING SUM(fileSize) > 1gb AND COUNT() >= 10
type = note GROUP BY noteType COUNT() HAVING NOT (COUNT() < 5)
type = resource GROUP BY tags COUNT() HAVING MAX(created) < -1y
```

Plain fields are not allowed on the left side of `HAVING` conditions — filter them in the expression before `GROUP BY`.

Note: when `GROUP BY` includes a junction relation (e.g. `tags`), `COUNT()` counts join rows. Grouping by a single relation yields correct per-bucket entity counts; grouping by two relations simultaneously multiplies rows.

### Date Buckets

Group datetime fields (`created`, `updated`) by calendar period with a dotted suffix: `.day` (`YYYY-MM-DD`), `.week` (`YYYY-MM-DD`, Monday of the week), `.month` (`YYYY-MM`), `.year` (`YYYY`). Bucket labels sort chronologically. Valid only in `GROUP BY` (both modes) and as its `ORDER BY` key — use date ranges in the filter expression instead.

```
type = note GROUP BY created.month COUNT() ORDER BY created.month ASC
type = resource GROUP BY updated.week COUNT()
type = resource GROUP BY created.year
```

## GROUP BY — Bucketed Mode

No aggregate functions → entities organized into named buckets. `LIMIT` applies per bucket.

```
type = resource GROUP BY contentType LIMIT 5
type = resource GROUP BY meta.camera_model LIMIT 10
type = note GROUP BY owner ORDER BY name ASC LIMIT 3
```

CLI paging flags for bucketed mode:

```bash
mr mrql --buckets 10 --page 2 'type = resource GROUP BY contentType LIMIT 5'
mr mrql --offset 20 'type = resource GROUP BY contentType LIMIT 5'
```

## Traversal

Access properties of related groups via dotted paths. Max depth: 8 parts.

```
type = resource AND owner.name = "Project Alpha"
type = resource AND owner.parent.name = "Acme Corp"
type = group AND parent.parent.name = "Root"
type = note AND owner.children.name ~ "Sprint*"
```

Valid leaf fields after traversal: group scalars (`name`, `description`, `category`, `id`, `created`, `updated`), relations (`tags`, `parent`, `children`), and meta (`meta.<key>`).

### Recursive Traversal — `ancestors.` / `descendants.`

Walk the group hierarchy transitively at any depth (no need to know how many
`parent.` steps to write). Valid on every entity type.

```
type = group AND ancestors.name = "Archive"        # groups anywhere below "Archive"
type = group AND descendants.tags = "wip"           # groups with a WIP-tagged descendant
type = resource AND ancestors.meta.region = "eu"    # resources under an EU group (via owner)
```

- Base group: the group itself, or (for resources/notes) the `owner` group.
- **Strict** — excludes the base group. Combine with `owner`/`parent` to include
  it: `owner.name = "Archive" OR ancestors.name = "Archive"`.
- Leaf is exactly one group field: a scalar, `tags`, or `meta.<key>`. No further
  chaining.
- Negation is existential: `ancestors.category != 3` = *no ancestor has category
  3*. Not supported: `IN`, `IS EMPTY`/`IS NULL`, `ORDER BY`, `GROUP BY`.

### Similarity Search — `SIMILAR TO`

Match resources perceptually similar to a target resource, from the
precomputed similarity pairs (the same data the resource page's similarity
sidebar reads). Resource entity only.

```
type = resource AND SIMILAR TO resource(1234)
type = resource AND SIMILAR TO resource(1234) WITHIN 2
type = resource AND SIMILAR TO resource(1234) ORDER BY distance ASC LIMIT 20
```

- Without `WITHIN`, the runtime `hash_similarity_threshold` setting applies
  (default 10); the `hash_ahash_threshold` secondary filter applies whenever set
  above 0 (its normal state), so results match the similarity sidebar. `WITHIN <d>` (0-11) overrides the
  primary distance; pairs are stored up to distance 11, so larger values are
  rejected.
- The target itself never matches. A nonexistent or unhashed target matches
  nothing.
- `ORDER BY distance` (ASC/DESC) sorts by the distance to the target and
  requires exactly one `SIMILAR TO` predicate. Rows without a stored pair
  (matched via other OR branches) sort last.

## Ordering Keys

Besides plain fields, `ORDER BY` accepts these context-sensitive keys:

```
type = resource AND tags IS EMPTY ORDER BY RANDOM() LIMIT 20
type = note AND TEXT ~ "kubernetes migration" ORDER BY RANK LIMIT 10
```

- `RANDOM()` — random order (or a random sample with `LIMIT`). Takes no
  `ASC`/`DESC`. Not allowed with `GROUP BY`. `LIMIT`/`OFFSET` re-roll the order
  on each request, so paging a random order can repeat rows — that is the
  expected "give me N random items" behavior.
- `RANK` — full-text relevance; most relevant first (no direction needed;
  `RANK DESC` reverses to least-relevant first). Requires exactly one `TEXT ~`
  predicate, a single entity type, and no `GROUP BY`. Errors if the server was
  started with full-text search disabled (`-skip-fts`).

## Parameters — `$name`

Placeholders in value positions only (comparison RHS, `IN (...)` items, `HAVING` RHS). Not in field names, `LIMIT`/`OFFSET`, `SCOPE`, `WITHIN`, or `GROUP BY` keys. `$name` inside a quoted string is literal.

```
type = "resource" AND tags = $tag AND created > $since
type = "resource" GROUP BY contentType COUNT() HAVING COUNT() > $min
```

- Binding is value-level (bind placeholders), never string interpolation — injection-safe.
- A supplied string coerces like a typed literal (`-7d`, `10mb`, `NOW()`, quoted-string unwraps); otherwise a plain string. Force a string with quotes: `--param n='"42"'`.
- Every placeholder must be supplied (missing → 400); unknown params rejected. Case-sensitive.

```bash
mr mrql 'type = resource AND created > $since' --param since=-7d
mr mrql run monthly --param month=2026-07
```

API: `params` object on `POST /v1/mrql`; `param.<name>=value` query params on `POST /v1/mrql/saved/run`. Shortcodes: `param-<name>` attrs. `POST /v1/mrql/validate` returns a `params` array; saved-query responses carry a derived `params` array.

## EXPLAIN

`POST /v1/mrql/explain` / `mr mrql explain` — return the SQL a query would run, without executing it. Honours default `LIMIT`, `SCOPE`, and RBAC forced scope. One statement for flat/aggregated; three (resources/notes/groups) for cross-entity; bucketed shows the key-discovery query plus a fan-out note.

```bash
mr mrql explain 'type = resource AND fileSize > 1mb'
mr mrql explain --saved my-report --param since=-7d --json
```

Web: **Explain** button / `Mod-Shift-Enter`.

## Export

`GET|POST /v1/mrql/export` / `mr mrql export` — stream results as `format=csv` (default) or `format=json`. Same inputs as execution.

- CSV aggregated: group keys + aggregate aliases. Flat: fixed scalar columns per entity (`meta` as JSON string); single entity type only. Bucketed: bucket-key columns + flat item columns.
- JSON: the exact `/v1/mrql` body. Default-limit signalled via the `X-MRQL-Default-Limit-Applied` header.

```bash
mr mrql export 'type = resource' --format csv -o out.csv
mr mrql export --saved my-report --format json
```

## Rendering

The `--render` CLI flag (and `render=1` query parameter on `POST /v1/mrql`) requests server-side template rendering via `CustomMRQLResult` templates defined on Category, Resource Category, or Note Type. Matching entities include a `renderedHTML` field in the response.

```bash
mr mrql --render 'type = resource AND tags = "photo"'
```

Entities without a `CustomMRQLResult` template omit `renderedHTML`.

## List-Page Filter Bar

The `/resources`, `/notes`, and `/groups` pages (and their JSON list endpoints) accept a bare filter expression that ANDs with the page's sidebar filters, sort, and pagination. The entity type is implied.

```
tags = "vacation" AND created > -30d
notes IS EMPTY AND fileSize > 10mb
descendants.category = "Archive"
```

- Filter grammar only. No `ORDER BY`, `LIMIT`, `OFFSET`, `GROUP BY`, `SCOPE`, `$name` params, or `type`. `SIMILAR TO resource(N)` is allowed.
- Web: type in the bar above the list; submitting sets `?mrql=<expr>`. An invalid expression fails closed (error banner, zero results). The **Edit in MRQL editor** link opens `/mrql?q=type = <entity> AND (<expr>)`.
- API: `mrql=<expr>` on `GET /v1/resources`, `/v1/notes`, `/v1/groups`. Invalid returns HTTP 400 with a positioned error.
- CLI: `--mrql "<expr>"` on `mr resources list`, `mr notes list`, `mr groups list`.

```bash
mr resources list --mrql 'tags = "vacation" AND created > -30d'
```

## MRQL in Global Search

`Ctrl/Cmd+K` recognizes MRQL:

- A valid MRQL query pins a **Run MRQL query** row above the results; selecting it opens `/mrql?q=<query>` and runs it. Shown only when the query validates.
- Saved MRQL queries are findable by name or description; selecting one opens `/mrql?saved=<id>` in the editor (a parameterized query focuses its first empty parameter input instead of running).

## See Also

- [MRQL Query Language](./mrql.md) — conceptual overview with worked examples
- [Saved Queries (SQL)](./saved-queries.md) — the raw-SQL query runner, separate from MRQL saved queries
- CLI: [`mr mrql`](../cli/mrql/index.md), [`mr mrql run`](../cli/mrql/run.md), [`mr mrql explain`](../cli/mrql/explain.md), [`mr mrql export`](../cli/mrql/export.md), [`mr mrql list`](../cli/mrql/list.md)
