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
  [GROUP BY <field>[, <field>...] [<aggregates>]]
  [ORDER BY <field> [ASC|DESC]]
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
| `IS EMPTY` / `IS NOT EMPTY` | Value is empty/null or has content | `description IS NOT EMPTY` |
| `IS NULL` / `IS NOT NULL` | Meta key absent / present | `meta.rating IS NOT NULL` |
| `IN (...)` / `NOT IN (...)` | Set membership | `contentType IN ("image/png", "image/jpeg")` |
| `AND` `OR` `NOT` | Boolean logic (precedence: NOT > AND > OR) | `tags = "photo" AND NOT tags = "archived"` |

## Fields (by entity type)

**Common to all types:** `id`, `name`, `description`, `created`, `updated`, `tags`, `meta.<key>`, `TEXT` (full-text search).

**Resources only:** `groups` (alias `group`), `owner`, `category`, `contentType`, `fileSize`, `width`, `height`, `originalName`, `hash`.

**Notes only:** `groups` (alias `group`), `owner`, `noteType`.

**Groups only:** `category`, `parent`, `children`.

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

## Rendering

The `--render` CLI flag (and `render=1` query parameter on `POST /v1/mrql`) requests server-side template rendering via `CustomMRQLResult` templates defined on Category, Resource Category, or Note Type. Matching entities include a `renderedHTML` field in the response.

```bash
mr mrql --render 'type = resource AND tags = "photo"'
```

Entities without a `CustomMRQLResult` template omit `renderedHTML`.

## See Also

- [MRQL Query Language](./mrql.md) — conceptual overview with worked examples
- [Saved Queries](./saved-queries.md) — persisting and reusing queries
- CLI: [`mr mrql`](../cli/mrql/index.md), [`mr mrql run`](../cli/mrql/run.md), [`mr mrql list`](../cli/mrql/list.md)
