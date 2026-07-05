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

## Natural-Language Generation

When `DEEPSEEK_API_KEY` is configured, the `/mrql` editor can draft MRQL from a "Describe results" prompt. The server sends only the text you type and syntax-only MRQL instructions to DeepSeek. It does not send local tag lists, category names, note types, resource categories, saved queries, or database contents.

Generated MRQL is parsed, validated, and linted locally, then shown with an explanation. It is not executed until you press Run. Generation is CSRF-protected and requires write access when authentication is enabled.

## Filtering List Pages

The `/resources`, `/notes`, and `/groups` list pages carry a single-line MRQL filter bar above the list. Type a bare filter expression. The entity type is implied by the page, so you write only the conditions:

```
tags = "vacation" AND created > -30d
notes IS EMPTY AND fileSize > 10mb
descendants.category = "Archive"
```

Submitting sets `?mrql=<expr>` on the same list URL and ANDs the filter with every sidebar filter, the current sort, and pagination. The bar accepts the filter (WHERE-clause) grammar only. `ORDER BY`, `LIMIT`, `OFFSET`, `GROUP BY`, `SCOPE`, and `$name` parameters are rejected, and you do not write `type` (the page sets it). The `SIMILAR TO resource(N)` predicate is allowed.

An invalid expression fails closed: the page renders an error banner and zero results, never the unfiltered list, so a broken filter cannot widen a following bulk action.

Each bar has an **Edit in MRQL editor** link that opens `/mrql?q=type = <entity> AND (<expr>)`, graduating the current filter to the full editor where ordering, limits, grouping, and saving become available.

### JSON API

The list endpoints accept the same filter grammar as an `mrql` query parameter: `GET /v1/resources`, `GET /v1/notes`, and `GET /v1/groups` take `mrql=<expr>`. An invalid expression returns HTTP 400 with a positioned error.

### CLI

`mr resources list`, `mr notes list`, and `mr groups list` accept `--mrql "<expr>"`, applying the same filter grammar (type implied) alongside the other list flags:

```bash
mr resources list --mrql 'tags = "vacation" AND created > -30d'
```

## MRQL in Global Search

Global search (`Ctrl/Cmd+K`) recognizes MRQL:

- **Run a query.** Typing a valid MRQL query surfaces a pinned **Run MRQL query** row above the search results. Selecting it opens `/mrql?q=<query>`, which runs the query automatically. The row appears only when the query passes validation, so ordinary search terms are unaffected.
- **Open a saved query.** A saved MRQL query is findable by its name or description. Selecting it opens `/mrql?saved=<id>`, loading it into the editor. A parameterized query focuses its first empty parameter input instead of running immediately.

## Syntax Reference

### Basic Structure

```
[type = "resource|note|group" AND] <conditions> [GROUP BY <field> [<aggregates>] [HAVING <aggregate-conditions>]] [ORDER BY <field> [ASC|DESC]] [LIMIT <n>] [OFFSET <n>]
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
| `guid` | string | Stable UUIDv7 identifier |
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
| `notes` | relation | Linked notes (match by name) |

**Note-only fields:**

| Field | Type | Description |
|-------|------|-------------|
| `groups` / `group` | relation | Associated groups (match by name) |
| `owner` | relation | Owner group (match by name, supports traversal) |
| `noteType` | string | Note type ID |
| `resources` | relation | Linked resources (match by name) |

**Group-only fields:**

| Field | Type | Description |
|-------|------|-------------|
| `category` | string | Group category ID |
| `parent` | relation | Parent group (match by name) |
| `children` | relation | Child groups (match by name) |
| `resources` | relation | Related resources (match by name) |
| `notes` | relation | Related notes (match by name) |

Relation fields also support `.count` comparisons against a non-negative integer — `tags.count = 0`, `resources.count >= 100` — with `=`, `!=`, `>`, `>=`, `<`, `<=`, in filters and `ORDER BY`. `owner` and `parent` are single references and cannot be counted (use `IS NULL`).

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

#### Regex Matching (PostgreSQL only)

On PostgreSQL deployments, `~*` and `!~*` match against a case-insensitive POSIX regular expression. Unlike `~`, the pattern is a real regex — no `*`/`?` wildcard shortcuts, no implicit anchoring or `%...%` wrapping:

```
name ~* "^IMG_[0-9]{4}\.(jpe?g|png)$"    # names like IMG_0421.jpg
originalName !~* "\.(tmp|bak)$"          # not ending in .tmp or .bak
```

Allowed on string and `meta.<key>` fields (and string traversal leaves like `owner.name`). Not on numeric, datetime, or relation fields. On SQLite (no native regex) `~*`/`!~*` return an error. An invalid pattern surfaces the database's "invalid regular expression" message.

### Ranges — `BETWEEN`

`BETWEEN` matches an inclusive range on both ends; `NOT BETWEEN` is the complement. It works wherever `>=`/`<=` do — dates, numbers (including size units), strings (lexicographic), and `meta.<key>`. Bounds can be any value, including relative dates, `NOW()`, and `$params`:

```
created BETWEEN "2024-01-01" AND "2024-06-30"
fileSize NOT BETWEEN 1mb AND 10mb
created BETWEEN -30d AND NOW()
```

`f BETWEEN a AND b` is exactly `(f >= a AND f <= b)`.

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

On SQLite the search uses the FTS5 index; on PostgreSQL it matches a `tsvector` column via `plainto_tsquery`. Both backends AND the search terms together, so every word must match, rather than matching the value as an exact phrase. When the full-text index is unavailable (for example the server was started with `-skip-fts`), `TEXT ~` falls back to a case-insensitive substring match on name and description.

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

### Random Order — `RANDOM()`

`ORDER BY RANDOM()` returns rows in a random order — handy for a random sample with `LIMIT`:

```
type = resource AND tags IS EMPTY ORDER BY RANDOM() LIMIT 20
type = note ORDER BY name, RANDOM()        # random tiebreak within equal names
```

`RANDOM()` takes no `ASC`/`DESC` and cannot be combined with `GROUP BY`. Because the order is re-rolled on every request, paging past the first page (`LIMIT`/`OFFSET`) draws a fresh random sample that can repeat earlier rows — this is the expected "give me N random items" behavior, not stable pagination.

### Relevance Order — `RANK`

`ORDER BY RANK` sorts full-text results by relevance, most relevant first (no direction needed; `RANK DESC` reverses to least-relevant first):

```
type = note AND TEXT ~ "kubernetes migration" ORDER BY RANK LIMIT 10
```

`RANK` requires exactly one `TEXT ~` predicate (its term defines the relevance), a single entity type, and no `GROUP BY`. It errors if the server was started with full-text search disabled (`-skip-fts`) — a relevance sort over the non-indexed fallback would be meaningless.

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

Add `HAVING` after the aggregate list to keep only buckets whose aggregates match; conditions use aggregate functions (never plain fields) and combine with `AND` / `OR` / `NOT`:

```
type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC
type = resource GROUP BY tags COUNT() HAVING SUM(fileSize) > 1gb AND COUNT() >= 10
```

Datetime fields can be bucketed by calendar period with `.day`, `.week` (Monday start), `.month`, or `.year` — valid in GROUP BY (both modes) and its ORDER BY only:

```
type = note GROUP BY created.month COUNT() ORDER BY created.month ASC
```

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

### Recursive Traversal: `ancestors.` / `descendants.`

Multi-level traversal (`parent.parent.name`) requires you to know the depth. When you want to match at *any* depth, use the `ancestors.` and `descendants.` roots, which walk the group hierarchy transitively. They are valid on every entity type.

```
type = group AND ancestors.name = "Archive"        # groups anywhere below "Archive"
type = group AND descendants.tags = "wip"           # groups with a WIP-tagged descendant, at any depth
type = resource AND ancestors.meta.region = "eu"    # resources whose owner sits under an EU group
```

- **Base group.** For a group, itself; for a resource or note, its `owner` group.
- **Strict.** `ancestors`/`descendants` exclude the base group. A resource stored directly in "Archive" does *not* match `ancestors.name = "Archive"` -- write `owner.name = "Archive" OR ancestors.name = "Archive"` for "in Archive or anywhere below it".
- **One leaf field.** The predicate takes exactly one group field: a scalar (`name`, `category`, `id`, ...), `tags`, or `meta.<key>`. Chaining further (`ancestors.parent.name`) is not supported.
- **Existential negation.** `ancestors.category != 3` means *no ancestor has category 3* (and owner-less rows, which have no ancestors, match). `IN`, `IS EMPTY`/`IS NULL`, `ORDER BY`, and `GROUP BY` are not supported on these roots.

### Similarity Search: `SIMILAR TO`

`SIMILAR TO resource(<id>)` matches resources that are perceptually similar to the target resource. It reads the precomputed similarity pairs -- the same data behind the resource page's similarity sidebar -- so it is fast at any library size and never computes hashes at query time.

```
type = resource AND SIMILAR TO resource(1234)                    # similar images, runtime threshold
type = resource AND SIMILAR TO resource(1234) WITHIN 2           # near-duplicates only
type = resource AND SIMILAR TO resource(1234) AND tags != "reviewed"
type = resource AND SIMILAR TO resource(1234) ORDER BY distance ASC LIMIT 20
```

- **Thresholds.** Without `WITHIN`, the live `hash_similarity_threshold` runtime setting applies (default 10), and the `hash_ahash_threshold` secondary filter applies whenever set above 0 (its normal state) -- MRQL results match the similarity sidebar exactly, and tuning the settings applies to saved queries instantly. `WITHIN <d>` overrides the primary distance; the valid range is 0-11 because pairs are only stored up to distance 11.
- **The target never matches itself.** Consequently `NOT SIMILAR TO resource(N)` includes resource N.
- **Missing data means empty, not an error.** A nonexistent target, a non-image, or a resource the hash worker has not processed yet matches nothing.
- **Sorting.** `ORDER BY distance` (ASC or DESC) sorts by the perceptual distance to the target and requires exactly one `SIMILAR TO` predicate in the query. Rows matched by other OR branches that have no stored pair sort last.
- **Resource entity only.** `type = note/group` queries reject it; in a type-guarded OR, the similarity branch simply matches nothing for other entities.

## Cross-Entity Queries

Omitting `type` causes MRQL to fan out the query across resources, notes, and groups simultaneously. Only common fields (`id`, `name`, `description`, `created`, `updated`, `tags`, `guid`, `meta.<key>`) and `TEXT ~` full-text search are valid in cross-entity mode.

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

## Parameterized Queries (Reports)

A query may contain `$name` placeholders in **value positions** — anywhere a
literal value is accepted (comparison right-hand side, `IN (...)` list items, and
`HAVING` comparison right-hand side). A placeholder name is `[a-zA-Z_][a-zA-Z0-9_]*`.
Placeholders are **not** allowed in field names, `LIMIT`/`OFFSET`, `SCOPE`,
`WITHIN`, or `GROUP BY` keys. A `$name` inside a quoted string stays literal text.

```
type = resource AND tags = $tag AND created > $since
type = note AND name ~ $needle LIMIT 50
type = resource GROUP BY contentType COUNT() HAVING COUNT() > $min
tags IN ($a, $b)
```

Parameters make a saved query reusable as a **report**: save it once with
placeholders, then supply values at run time.

- **Binding is at the value level, never string interpolation** — bound values
  translate to database bind placeholders exactly like typed literals, so they are
  injection-safe by construction. `tag = $t` with `t = 'x" OR 1=1'` is just an
  unusual tag string that matches nothing.
- **Value coercion mirrors the lexer.** A supplied string behaves as if typed at
  that position: a bare number (`42`, `10mb`), a relative date (`-7d`), or a date
  function (`NOW()`) is parsed as that literal; anything else becomes a plain
  string. Force a string by wrapping in quotes (CLI: `--param n='"42"'`).
- **Every placeholder must be supplied.** A missing value is a 400 error listing
  the missing name; an unknown/extra parameter is also rejected (typo protection).
  Names are case-sensitive.
- **Saving is allowed with unbound placeholders** — validation accepts a
  placeholder against any field type and re-checks compatibility once bound.

On the `/mrql` page, one labeled input appears per placeholder above the Run
button; loading a saved report focuses the first empty input instead of running.

From the CLI, bind with repeatable `--param`:

```bash
mr mrql 'type = resource AND created > $since' --param since=-7d
mr mrql run monthly-report --param month=2026-07
```

Via the API, `POST /v1/mrql` accepts a `params` object; `POST /v1/mrql/saved/run`
also accepts `param.<name>=<value>` query parameters. In `[mrql]` shortcodes and
plugin `mah.db.mrql`, supply `param-<name>` attributes / a `params` table.

## EXPLAIN

`POST /v1/mrql/explain` (and `mr mrql explain`) returns the SQL statement(s) a
query would run **without executing it**. The reported SQL reflects what would
actually run: the default `LIMIT` is applied, `SCOPE` is resolved, and RBAC forced
scoping is included. A flat single-entity query yields one statement; a
cross-entity query yields one per entity table; aggregated `GROUP BY` yields one;
bucketed `GROUP BY` shows the key-discovery query and notes the per-bucket fan-out.

```bash
mr mrql explain 'type = resource AND fileSize > 1mb'
mr mrql explain --saved my-report --param since=-7d --json
```

On the `/mrql` page, the **Explain** button (or `Mod-Shift-Enter`) opens a panel
above the results showing the interpolated SQL per statement, with a toggle for the
raw parameterized SQL and its bind variables.

## Export

`GET|POST /v1/mrql/export` (and `mr mrql export`) streams query results as a
download. `format=csv` (default) or `format=json`. The same inputs as execution
apply (`query` or `id`/`name`, `params`, `limit`, `page`, `buckets`, `offset`).

- **CSV — aggregated**: the `GROUP BY` keys then the aggregate aliases, in query order.
- **CSV — flat**: a fixed scalar column set per entity (`meta` as a JSON string).
  CSV requires a single entity type — use `format=json` for cross-entity results.
- **CSV — bucketed**: the bucket-key columns prepended to the flat item columns.
- **JSON**: the exact `/v1/mrql` response body as a download.

When no explicit `LIMIT` is present the default is applied and reported via the
`X-MRQL-Default-Limit-Applied` response header.

```bash
mr mrql export 'type = resource' --format csv -o resources.csv
mr mrql export --saved my-report --format json --param since=-7d
```

The `/mrql` results header has **Export CSV** / **Export JSON** buttons that
re-submit the current query and parameters.

## See Also

- [MRQL Reference](./mrql-reference.md) — compact syntax cheatsheet for quick lookup
- [Saved Queries](./saved-queries.md) — persisting MRQL queries for reuse
