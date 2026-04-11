---
sidebar_position: 20
title: MRQL Query Language
---

# MRQL Query Language

MRQL (Mahresources Query Language) is a structured query language for searching across resources, notes, and groups with precise field-level filtering, ordering, and pagination.

## When to Use MRQL

| Use case | Best tool |
|----------|-----------|
| Quick keyword search | Global search (`Ctrl+K`) |
| Filter by one or two fields | Entity list filters |
| Complex multi-field conditions | **MRQL** |
| Date range + tag + file size combinations | **MRQL** |
| Reusable cross-entity queries | **MRQL + Saved Queries** |
| Raw SQL with joins and aggregates | Saved Queries (SQL) |

## Accessing MRQL

Navigate to `/mrql` in the web UI. The page provides:

- A syntax-highlighted editor with autocompletion (`Ctrl+Space`)
- Real-time validation with inline error markers
- **Run** button or `Ctrl+Enter` to execute
- **Save** to persist a query for later reuse
- **Saved Queries** panel listing all stored queries
- **Recent Queries** history (session-local)

## Syntax Reference

### Basic Structure

```
[type = "resource|note|group" AND] <conditions> [GROUP BY <field> [<aggregates>]] [ORDER BY <field> [ASC|DESC]] [LIMIT <n>] [OFFSET <n>]
```

Conditions are field-value comparisons joined with `AND`, `OR`, and `NOT`.

### Entity Selector

Use `type = "<value>"` anywhere in the query to target a specific entity type:

```
type = resource AND name ~ "photo"
type = note AND tags = "todo"
type = group AND category = "3"
```

Valid values: `resource`, `note`, `group`.

Omit the `type` selector entirely to search all entity types at once (cross-entity mode).

### Fields

**Common fields** (available on all entity types):

| Field | Type | Description |
|-------|------|-------------|
| `id` | number | Entity ID |
| `name` | string | Display name |
| `description` | string | Description or body text |
| `created` | datetime | Creation timestamp |
| `updated` | datetime | Last-updated timestamp |
| `tags` | relation | Associated tags (match by name) |
| `meta.<key>` | string/number | Dynamic metadata value |

**Resource-only fields:**

| Field | Type | Description |
|-------|------|-------------|
| `groups` / `group` | relation | Associated groups (match by name) |
| `owner` | relation | Owner group (match by name, supports traversal) |
| `category` | string | Resource category ID |
| `contentType` | string | MIME type (e.g. `image/png`) |
| `fileSize` | number | File size in bytes (supports `kb`, `mb`, `gb` units) |
| `width` | number | Image/video width in pixels |
| `height` | number | Image/video height in pixels |
| `originalName` | string | Original filename at upload |
| `hash` | string | Content hash |

**Note-only fields:**

| Field | Type | Description |
|-------|------|-------------|
| `groups` / `group` | relation | Associated groups (match by name) |
| `owner` | relation | Owner group (match by name, supports traversal) |
| `noteType` | string | Note type ID |

**Group-only fields:**

| Field | Type | Description |
|-------|------|-------------|
| `category` | string | Group category ID |
| `parent` | relation | Parent group (match by name) |
| `children` | relation | Child groups (match by name) |

### Comparison Operators

| Operator | Meaning | Example |
|----------|---------|---------|
| `=` | Equal (case-insensitive for strings) | `name = "Report"` |
| `!=` | Not equal | `contentType != "application/pdf"` |
| `>` | Greater than | `fileSize > 1mb` |
| `>=` | Greater than or equal | `created >= -30d` |
| `<` | Less than | `width < 800` |
| `<=` | Less than or equal | `fileSize <= 500kb` |

String comparisons with `=` and `!=` are always case-insensitive.

### Pattern Matching

The `~` operator performs a **contains** match by default. Without wildcards, the value is matched anywhere in the field:

```
contentType ~ "image"     # matches "image/png", "image/jpeg", etc.
name ~ "report"           # matches "Q1 Report", "Annual reporting", etc.
```

Use `*` for any sequence of characters and `?` for a single character to create anchored patterns:

```
name ~ "project*"         # starts with "project" (no implicit wrapping)
contentType ~ "image/*"   # matches "image/png" but not "text/image"
originalName ~ "*.jpg"    # ends with .jpg
name ~ "Q?-report"        # Q1-report, Q2-report, etc.
```

:::info Wildcard behavior
When your value contains **no** `*` or `?` wildcards, `~` automatically wraps it with `*` on both sides, making it a substring/contains match. As soon as you include any wildcard, the value is used as-is, giving you precise control over anchoring.
:::

The `!~` operator is the negated form:

```
name !~ "draft*"          # does not start with "draft"
contentType !~ "image"    # does not contain "image"
```

Both `~` and `!~` are case-insensitive.

### Existence Checks

```
description IS EMPTY          # description is empty string or null
description IS NOT EMPTY      # description has a non-empty value
meta.rating IS NULL           # meta key not present
meta.rating IS NOT NULL       # meta key is present
tags IS EMPTY                 # no tags associated
```

### Set Operators

```
contentType IN ("image/png", "image/jpeg", "image/webp")
tags IN ("urgent", "review", "blocked")
contentType NOT IN ("video/mp4", "video/webm")
```

### Full-Text Search

Search indexed text across the entity's name, description, and content fields:

```
TEXT ~ "quarterly earnings"
type = note AND TEXT ~ "retrospective action items"
```

Full-text search uses the database's FTS5 index and supports phrase queries. It is only available when the server is started without `-skip-fts`.

### Boolean Logic

Combine conditions with `AND`, `OR`, and `NOT`. Use parentheses for explicit grouping.

**Operator precedence** (highest to lowest):

1. `NOT`
2. `AND`
3. `OR`

```
# AND binds tighter than OR:
type = resource AND (tags = "photo" OR tags = "video")

# NOT applies to the next expression:
type = resource AND NOT tags = "archived"

# Explicit grouping:
(type = resource OR type = note) AND created > -7d
```

### Case Sensitivity

All comparisons are case-insensitive. `name = "Report"` matches "report", "REPORT", and "Report". Pattern matching with `~` is also case-insensitive.

### String Escaping

Strings are double-quoted. Use `\"` to include a literal quote and `\\` for a backslash:

```
name = "O\"Brien"
originalName ~ "C:\\Users\\*"
```

## Relative Dates

Use relative date literals in datetime comparisons to express time offsets from the current moment:

| Literal | Meaning |
|---------|---------|
| `-7d` | 7 days ago |
| `-2w` | 2 weeks ago |
| `-3m` | 3 months ago |
| `-1y` | 1 year ago |
| `-30d` | 30 days ago |

```
created > -7d                  # created in the last 7 days
updated < -1y                  # not updated in over a year
created >= -3m AND created <= -1m   # created 1-3 months ago
```

## Date Functions

Use built-in functions for date boundaries:

| Function | Returns |
|----------|---------|
| `NOW()` | Current timestamp |
| `START_OF_DAY()` | Midnight of the current day |
| `START_OF_WEEK()` | Midnight of the current week's Monday |
| `START_OF_MONTH()` | Midnight of the first day of the current month |
| `START_OF_YEAR()` | Midnight of January 1 of the current year |

```
created >= START_OF_WEEK()     # created this week
updated < START_OF_MONTH()     # not updated this month
created >= START_OF_YEAR()     # created this year
```

## File Size Units

Numeric values for `fileSize` accept unit suffixes (case-insensitive):

| Suffix | Multiplier |
|--------|-----------|
| `kb` | 1,024 bytes |
| `mb` | 1,048,576 bytes |
| `gb` | 1,073,741,824 bytes |

```
fileSize > 10mb
fileSize < 500kb
fileSize >= 1gb
```

## Ordering and Pagination

```
ORDER BY <field> [ASC|DESC]
LIMIT <n>
OFFSET <n>
```

Multiple ORDER BY columns are supported:

```
type = resource ORDER BY created DESC LIMIT 20
type = note ORDER BY updated ASC, name ASC LIMIT 50 OFFSET 100
```

The default sort order when `ORDER BY` is omitted is implementation-defined (typically insertion order).

## Scope

The `SCOPE` clause filters query results to entities within a group's ownership subtree. Place `SCOPE` after the filter expression and before `GROUP BY`:

```
type = "resource" SCOPE 42 ORDER BY created LIMIT 10
type = "note" SCOPE "My Project"
```

### Scope by ID

`SCOPE <number>` filters to the group with that ID and all its descendants:

```
type = resource SCOPE 42
```

This returns all resources owned by group 42 or any group underneath it in the hierarchy.

### Scope by Name

`SCOPE "group name"` looks up the group by name (case-insensitive):

```
type = resource SCOPE "Vacation Photos"
```

If multiple groups share the same name, MRQL returns an error listing all matches with their IDs so you can switch to `SCOPE <id>`.

### Scope with GROUP BY

Scope is applied before grouping:

```
type = resource SCOPE 42 GROUP BY contentType COUNT()
```

### No Scope

Omitting `SCOPE` or using `SCOPE 0` returns all matching entities regardless of ownership.

### Entity Types

- **Resources and Notes:** Scope filters by `owner_id` -- entities owned by groups in the subtree.
- **Groups:** Scope filters by `id` -- the scoped group itself and all its descendants.

## GROUP BY and Aggregation

Group results by field values with optional aggregate functions. GROUP BY requires an explicit entity type (`type = "resource"`, `type = "note"`, or `type = "group"`).

```
type = "<entity>" [<conditions>] GROUP BY <field>[, <field>...] [<aggregates>] [ORDER BY ...] [LIMIT <n>]
```

### Two Modes

| Mode | Trigger | Returns |
|------|---------|---------|
| Aggregated | GROUP BY with aggregate functions | Flat rows with computed values |
| Bucketed | GROUP BY without aggregate functions | Entity rows organized into groups |

### Aggregate Functions

| Function | Argument | Field types | Output key |
|----------|----------|-------------|------------|
| `COUNT()` | none | n/a | `count` |
| `SUM(field)` | required | numeric, meta | `sum_{field}` |
| `AVG(field)` | required | numeric, meta | `avg_{field}` |
| `MIN(field)` | required | numeric, datetime, meta | `min_{field}` |
| `MAX(field)` | required | numeric, datetime, meta | `max_{field}` |

Aggregate functions are case-insensitive (`count()`, `COUNT()`, `Count()` all work).

### Aggregated Mode

When aggregate functions are present, GROUP BY returns flat rows of computed values, one row per unique combination of the grouped fields.

```
type = resource GROUP BY contentType COUNT()
type = resource GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)
type = resource GROUP BY contentType COUNT() ORDER BY count DESC
type = resource GROUP BY meta.source COUNT()
type = note GROUP BY owner, noteType COUNT()
type = resource AND fileSize > 10mb GROUP BY contentType MIN(fileSize) MAX(fileSize)
```

Each result row includes the grouped field values plus one key per aggregate function (e.g., `count`, `sum_fileSize`, `avg_fileSize`).

### Bucketed Mode

When no aggregate functions are specified, GROUP BY returns entities organized into named buckets, one bucket per unique value of the grouped field.

```
type = resource GROUP BY contentType LIMIT 5
type = resource GROUP BY meta.camera_model LIMIT 10
type = note GROUP BY owner ORDER BY name ASC LIMIT 3
```

In bucketed mode, `LIMIT` applies **per bucket** (maximum items per group), not to the total result set.

### ORDER BY with GROUP BY

- **Aggregated mode:** ORDER BY can reference group fields or aggregate output keys (`count`, `sum_fileSize`, etc.)
- **Bucketed mode:** ORDER BY applies to items within each bucket

### Constraints

- GROUP BY requires `type = "resource|note|group"` (cross-entity grouping is not supported)
- Traversal paths are supported: `owner.name`, `owner.parent.name`, `owner.meta.key`, etc.
- Maximum 1000 buckets in bucketed mode

## Traversal

MRQL supports filtering by properties of related groups through dotted field paths. Traversal works on:

- **Resources and notes:** `owner` accesses the owner group
- **Groups:** `parent` accesses the parent group, `children` accesses child groups

### Single-Level Traversal

```
type = resource AND owner.name = "Project Alpha"
type = resource AND owner.tags = "active"
type = resource AND owner.category = "3"
type = group AND parent.name = "Acme Corp"
type = group AND children.name ~ "Q*"
```

### Multi-Level Traversal

Chain traversal fields to reach groups further up or down the hierarchy. After the first step, you're always in group context, so `parent` and `children` are the valid intermediate steps:

```
type = resource AND owner.parent.name = "Acme Corp"
type = resource AND owner.parent.tags = "active"
type = note AND owner.children.name ~ "Sprint*"
type = group AND parent.parent.name = "Root"
type = group AND parent.parent.tags = "org-level"
```

Maximum traversal depth is 8 parts (7 traversal steps + 1 leaf field).

### Valid Traversal Subfields

At the end of a traversal chain, you can access any group field:

- **Scalar:** `name`, `description`, `category`, `id`, `created`, `updated`
- **Relation:** `tags` (match by tag name), `parent`, `children`
- **Meta:** `meta.<key>` (e.g., `owner.meta.region`)

Traversal fields follow the same operators as regular fields. Traversal deeper than 8 parts is not supported.

## Cross-Entity Queries

Omitting `type` causes MRQL to fan out the query across resources, notes, and groups simultaneously. Only common fields (`id`, `name`, `description`, `created`, `updated`, `tags`) are valid in cross-entity mode.

```
name ~ "budget*"                              # search all entity types
tags = "urgent" LIMIT 30                      # across all types
TEXT ~ "quarterly review" LIMIT 30            # full-text across all types
```

Results are returned grouped by entity type (resources, then notes, then groups). `ORDER BY`, `LIMIT`, and `OFFSET` apply globally across the merged result set. Cross-entity sorting supports the common fields: `name`, `created`, `updated`.

## Saved Queries

Any query can be saved for later reuse:

1. Write and run a query in the `/mrql` editor
2. Click **Save**, provide a name and optional description
3. The query appears in the **Saved Queries** panel

Saved queries can be:
- **Loaded** by clicking them in the panel (populates the editor)
- **Run directly** via the CLI with `mr mrql run <name-or-id>`
- **Deleted** by hovering a query and clicking the Delete button
- **Updated** via the API (`PUT /v1/mrql/saved?id=N`)

## Server-Side Rendering

The MRQL execute endpoints (`POST /v1/mrql` and `POST /v1/mrql/saved/run`) accept a `render=1` query parameter. When set, the server processes each result entity's `CustomMRQLResult` template (if defined on its Category, Resource Category, or Note Type) and populates a `renderedHTML` field in the JSON response.

```bash
curl -X POST "http://localhost:8181/v1/mrql?render=1" \
  -H "Content-Type: application/json" \
  -d '{"query": "type = resource AND tags = \"photos\""}'
```

Entities without a `CustomMRQLResult` template omit the `renderedHTML` field from the JSON response. The `/mrql` web UI uses this field to display custom-rendered results inline.

## Examples Cookbook

### Finding resources by type and size

```
type = resource AND contentType ~ "image/*" AND fileSize > 5mb
```

### Recently modified notes with a specific tag

```
type = note AND tags = "todo" AND updated > -7d ORDER BY updated DESC
```

### Resources added this week without any tags

```
type = resource AND tags IS EMPTY AND created >= START_OF_WEEK()
```

### Large video files

```
type = resource AND contentType ~ "video/*" AND fileSize > 500mb ORDER BY fileSize DESC
```

### Groups with no parent (top-level only)

```
type = group AND parent IS EMPTY
```

### Notes in a specific group updated recently

```
type = note AND groups = "Project Alpha" AND updated > -30d
```

### Resources matching multiple content types

```
type = resource AND contentType IN ("image/png", "image/jpeg", "image/webp", "image/gif")
```

### Resources with missing descriptions

```
type = resource AND description IS EMPTY
```

### Full-text search within a date range

```
type = note AND TEXT ~ "budget forecast" AND created >= -90d ORDER BY created DESC
```

### Groups in a specific category added this year

```
type = group AND category = "5" AND created >= START_OF_YEAR()
```

### Resources with metadata rating above threshold

```
type = resource AND meta.rating > 4
```

### Everything tagged "urgent" across all entity types

```
tags = "urgent" LIMIT 50
```

### Resources with a specific original filename pattern

```
type = resource AND originalName ~ "screenshot_*" ORDER BY created DESC
```

### High-resolution images from the last month

```
type = resource AND contentType ~ "image/*" AND width >= 1920 AND created > -30d
```

### Notes not updated in over six months

```
type = note AND updated < -180d ORDER BY updated ASC
```

### Groups with children named after a pattern

```
type = group AND children.name ~ "Q* 2025"
```

### Resources excluding drafts and archived

```
type = resource AND NOT (tags IN ("draft", "archived")) ORDER BY created DESC LIMIT 25
```

### Resources owned by a specific group

```
type = resource AND owner = "Project Alpha"
```

### Resources whose owner has a specific tag

```
type = resource AND tags = "photo" AND owner.tags = "active"
```

### Resources whose owner's parent matches

```
type = resource AND owner.parent.name = "Acme Corp"
```

### Groups with deeply nested parent

```
type = group AND parent.parent.name = "Root Organization"
```

### Count resources by content type

```
type = resource GROUP BY contentType COUNT() ORDER BY count DESC
```

### Total and average file size per content type

```
type = resource GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)
```

### Size extremes for large files by content type

```
type = resource AND fileSize > 10mb GROUP BY contentType MIN(fileSize) MAX(fileSize)
```

### Notes by owner and note type

```
type = note GROUP BY owner, noteType COUNT()
```

### Resources bucketed by content type (5 per bucket)

```
type = resource GROUP BY contentType LIMIT 5
```

### Resources bucketed by metadata field

```
type = resource GROUP BY meta.camera_model LIMIT 10
```
