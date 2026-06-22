# MRQL — Mahresources Query Language

MRQL is the structured query DSL for mahresources. It runs server-side and returns matching entities (or aggregates). Reach for it instead of `resources list` / `notes list` when you need boolean logic, cross-entity search, relative dates, metadata predicates, group-subtree scoping, traversal across the group hierarchy, or `GROUP BY` aggregation.

> Field types verified against `mrql/fields.go`. Note that `category`, `noteType`, and `owner` are **numeric IDs**, not names — query them as `category = 3`, `owner = 12`. (Some older prose docs mislabel these as strings.) To match a category/owner by *name*, use `SCOPE "Name"` or traversal (`owner.name = "…"`).


## Query Shape

```
[type = "resource|note|group" AND] <conditions>
  [SCOPE <group-id-or-name>]
  [GROUP BY <field>[, <field>...] [<aggregates>]]
  [ORDER BY <field> [ASC|DESC] [, <field> [ASC|DESC]...]]
  [LIMIT <n>] [OFFSET <n>]
```

Entity type (`type = "..."`) is optional; omitting it triggers **cross-entity mode** (searches all types).

## Entity Types

| Type | Default | Notes |
|------|---------|-------|
| `resource` | No default | Files with metadata |
| `note` | No default | Text content |
| `group` | No default | Hierarchical collections |
| *(omitted)* | **Cross-entity** | Queries all types; only common fields allowed |

## Field Reference

### Common Fields (all entity types)

| Field | Type | Operators |
|-------|------|-----------|
| `id` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |
| `name` | string | `=`, `!=`, `~`, `!~` |
| `description` | string | `=`, `!=`, `~`, `!~`, `IS EMPTY`, `IS NOT EMPTY` |
| `created` | datetime | `=`, `!=`, `>`, `>=`, `<`, `<=`, `IS NULL`, `IS NOT NULL` |
| `updated` | datetime | `=`, `!=`, `>`, `>=`, `<`, `<=`, `IS NULL`, `IS NOT NULL` |
| `tags` | relation | `=`, `!=`, `~`, `!~`, `IS EMPTY`, `IS NOT EMPTY`, `IN`, `NOT IN` |
| `guid` | string | `=`, `!=`, `~`, `!~` |
| `meta.<key>` | mixed | `=`, `!=`, `>`, `>=`, `<`, `<=`, `~`, `!~`, `IS NULL`, `IS NOT NULL` |
| `TEXT` | full-text | `~` only (phrase queries, FTS5 syntax) |

### Resource-only Fields

| Field | Type | Operators |
|-------|------|-----------|
| `groups` / `group` | relation | `=`, `!=`, `~`, `!~`, `IS EMPTY`, `IS NOT EMPTY`, `IN`, `NOT IN` |
| `owner` | relation | `=`, `!=`, traversal supported |
| `category` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |
| `contentType` | string | `=`, `!=`, `~`, `!~` |
| `fileSize` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` (supports `kb`, `mb`, `gb` units) |
| `width` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |
| `height` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |
| `originalName` | string | `=`, `!=`, `~`, `!~` |
| `hash` | string | `=`, `!=`, `~`, `!~` |

### Note-only Fields

| Field | Type | Operators |
|-------|------|-----------|
| `groups` / `group` | relation | `=`, `!=`, `~`, `!~`, `IS EMPTY`, `IS NOT EMPTY`, `IN`, `NOT IN` |
| `owner` | relation | `=`, `!=`, traversal supported |
| `noteType` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |

### Group-only Fields

| Field | Type | Operators |
|-------|------|-----------|
| `category` | number | `=`, `!=`, `>`, `>=`, `<`, `<=` |
| `parent` | relation | `=`, `!=`, traversal supported, `IS EMPTY`, `IS NOT EMPTY` |
| `children` | relation | `=`, `!=`, `~`, `!~`, `IS EMPTY`, `IS NOT EMPTY`, `IN`, `NOT IN` |

## Operators

### Comparison

| Operator | Meaning | Example | Case-sensitive? |
|----------|---------|---------|---|
| `=` | Equal | `name = "Report"` | No (strings) |
| `!=` | Not equal | `contentType != "application/pdf"` | No (strings) |
| `>` | Greater than | `fileSize > 1mb`, `created > -30d` | N/A |
| `>=` | Greater or equal | `width >= 1920` | N/A |
| `<` | Less than | `fileSize < 500kb` | N/A |
| `<=` | Less or equal | `height <= 800` | N/A |

### Pattern Matching

| Operator | Meaning | Behavior | Example | Case-sensitive? |
|----------|---------|----------|---------|---|
| `~` | Contains / matches | Wraps with `*` unless wildcards present; regex-like | `name ~ "report"` (matches anywhere) | No |
| `!~` | Not contains / not matches | Negated `~` | `contentType !~ "image"` | No |

**Wildcard syntax** (when manually specified):
- `*` = any sequence of characters
- `?` = single character

When value has no `*` or `?`, it's automatically treated as substring (`*pattern*`).

### Set Operations

| Operator | Example |
|----------|---------|
| `IN (...)` | `contentType IN ("image/png", "image/jpeg")` |
| `NOT IN (...)` | `tags NOT IN ("draft", "archived")` |

### Existence & Null

| Operator | Meaning | Example |
|----------|---------|---------|
| `IS EMPTY` | String is empty or null | `description IS EMPTY` |
| `IS NOT EMPTY` | String has content | `tags IS NOT EMPTY` |
| `IS NULL` | Meta key absent / field null | `meta.rating IS NULL` |
| `IS NOT NULL` | Meta key present / field not null | `meta.rating IS NOT NULL` |

### Boolean Logic

| Operator | Precedence | Example |
|----------|-----------|---------|
| `NOT` | 1 (highest) | `NOT tags = "archived"` |
| `AND` | 2 | `type = resource AND tags = "photo"` |
| `OR` | 3 (lowest) | `tags = "urgent" OR tags = "blocked"` |

Use parentheses for explicit grouping: `(type = resource OR type = note) AND created > -7d`.

## Relative Dates

| Literal | Meaning |
|---------|---------|
| `-7d` | 7 days ago |
| `-2w` | 2 weeks ago |
| `-3m` | 3 months ago |
| `-1y` | 1 year ago |

### Date Functions

| Function | Returns |
|----------|---------|
| `NOW()` | Current timestamp |
| `START_OF_DAY()` | Midnight of today |
| `START_OF_WEEK()` | Midnight of this week's Monday |
| `START_OF_MONTH()` | Midnight of the 1st of this month |
| `START_OF_YEAR()` | Midnight of January 1 of this year |

## File Size Units

Applied to `fileSize` comparisons (case-insensitive):

| Unit | Multiplier |
|------|-----------|
| `kb` | 1,024 bytes |
| `mb` | 1,048,576 bytes |
| `gb` | 1,073,741,824 bytes |

Example: `fileSize > 10mb`, `fileSize <= 500kb`.

## String Escaping

Strings are double-quoted. Escape sequences:
- `\"` — literal quote
- `\\` — literal backslash

Example: `name = "O\"Brien"`, `originalName ~ "C:\\Users\\*"`.

## SCOPE — Filter to Group Subtree

Restricts results to entities owned by (or contained in) a specific group and its descendants.

```
type = "resource" SCOPE 42 ORDER BY created LIMIT 10
type = "note" SCOPE "My Project"
```

**By ID:** `SCOPE <number>` — group with that ID plus all descendants.

**By Name:** `SCOPE "name"` — case-insensitive lookup; errors with all matches if multiple groups share the name.

**Entity scoping:**
- **Resources / Notes:** Filtered by `owner_id` (group that owns them).
- **Groups:** Filtered by `id` (the group itself and all descendants).

**Default:** Omit `SCOPE` or use `SCOPE 0` for all matching entities.

**With GROUP BY:** Applied before grouping.

## GROUP BY — Two Modes

### Aggregated Mode

When aggregate functions are present, returns flat rows of computed values (one per unique group key combination).

```
type = resource GROUP BY contentType COUNT()
type = resource GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)
type = resource GROUP BY contentType COUNT() ORDER BY count DESC
type = note GROUP BY owner, noteType COUNT()
```

**Aggregate Functions:**

| Function | Argument | Valid On | Output Key |
|----------|----------|----------|-----------|
| `COUNT()` | none | any | `count` |
| `SUM(field)` | required | numeric, `meta.<key>` | `sum_{field}` |
| `AVG(field)` | required | numeric, `meta.<key>` | `avg_{field}` |
| `MIN(field)` | required | numeric, datetime, `meta.<key>` | `min_{field}` |
| `MAX(field)` | required | numeric, datetime, `meta.<key>` | `max_{field}` |

All aggregate functions are case-insensitive: `COUNT()`, `count()`, `Count()` all work.

### Bucketed Mode

When no aggregate functions are specified, returns entities organized into named buckets. `LIMIT` applies **per bucket**, not globally.

```
type = resource GROUP BY contentType LIMIT 5
type = resource GROUP BY meta.camera_model LIMIT 10
type = note GROUP BY owner ORDER BY name ASC LIMIT 3
```

**Constraints:**
- Maximum 1000 buckets per page.
- `LIMIT` caps items per bucket, not total results.
- `ORDER BY` applies to items within each bucket.

## ORDER BY and LIMIT

```
ORDER BY <field> [ASC|DESC] [, <field> [ASC|DESC]...]
LIMIT <n>
OFFSET <n>
```

**Multiple columns:** Supported; separate with commas.

**Direction:** `ASC` (ascending) or `DESC` (descending); defaults to `ASC` if omitted.

**Default LIMIT:** No `LIMIT` clause applies the server's configured default — the `-mrql-default-limit` flag (env `MRQL_DEFAULT_LIMIT`), which the standard binary sets to **500** (the internal code fallback is 1000 if the flag is left unwired).

**Offset semantics:**
- **Regular queries:** Traditional row offset.
- **Bucketed GROUP BY:** Bucket offset (cursor-based pagination).

## Traversal

Access related group properties via dotted field paths. Maximum depth: 8 parts (7 traversal steps + 1 leaf field).

**From Resources / Notes:**
```
type = resource AND owner.name = "Project Alpha"
type = resource AND owner.parent.name = "Acme Corp"
type = resource AND owner.parent.tags = "active"
type = note AND owner.children.name ~ "Sprint*"
```

**From Groups:**
```
type = group AND parent.name = "Root"
type = group AND parent.parent.name = "Organization"
type = group AND children.name ~ "Q*"
type = group AND children.parent.tags = "archived"
```

**Valid leaf fields after traversal:**
- **Scalars:** `name`, `description`, `category`, `id`, `created`, `updated`
- **Relations:** `tags` (by name), `parent`, `children`
- **Meta:** `meta.<key>`

All leaf fields support the same operators as direct access.

## Cross-Entity Queries

Omit `type =` to fan query across all entity types (resources, notes, groups simultaneously).

```
name ~ "budget*"
tags = "urgent" LIMIT 30
TEXT ~ "quarterly review"
```

**Constraints:**
- Only common fields allowed: `id`, `name`, `description`, `created`, `updated`, `tags`, `meta.<key>`, `TEXT`.
- `ORDER BY` supports: `name`, `created`, `updated`.
- `GROUP BY` not supported in cross-entity mode.
- Results grouped by entity type in response (resources, then notes, then groups).

## Full-Text Search

```
TEXT ~ "quarterly earnings"
type = note AND TEXT ~ "retrospective action items"
```

Searches indexed text across entity `name`, `description`, and content fields using FTS5. Supports phrase queries (double-quoted tokens) and boolean operators. Only available if server started without `-skip-fts`.

## Case Sensitivity

**All comparisons are case-insensitive:** `name = "Report"` matches "report", "REPORT", "Report".

Pattern matching with `~` and `!~` is also case-insensitive.

## Metadata Fields

Access dynamic metadata with `meta.<key>` syntax:

```
type = resource AND meta.rating > 4
type = resource GROUP BY meta.camera_model LIMIT 10
type = resource AND meta.location = "San Francisco"
```

`meta.<key>` supports all standard operators: `=`, `!=`, `>`, `>=`, `<`, `<=`, `~`, `!~`, `IS NULL`, `IS NOT NULL`.

---

# CLI Invocation

## mr mrql — Execute a query

```bash
# Positional argument
mr mrql 'type = resource AND tags = "photo"'

# From file
mr mrql -f query.mrql

# From stdin
echo 'tags = "photo"' | mr mrql -

# With pagination
mr mrql --limit 10 --page 2 'type = note'

# Bucketed GROUP BY pagination
mr mrql --buckets 10 --offset 20 'type = resource GROUP BY contentType LIMIT 5'

# Render custom templates
mr mrql --render 'type = resource AND tags = "photo"'

# JSON output for scripting
mr mrql --json 'type = resource' | jq '.resources[].ID'
```

**Flags:**
- `-f, --file <path>` — Read query from file.
- `--limit <n>` — Per-bucket item cap (GROUP BY) or total result cap (regular queries).
- `--buckets <n>` — Maximum buckets per page (bucketed GROUP BY only).
- `--offset <n>` — Bucket page offset (bucketed GROUP BY pagination).
- `--page <n>` — Global page number (applies to all list-like commands).
- `--render` — Request server-side template rendering via `CustomMRQLResult`.
- `--json` — Output raw JSON.
- `--quiet` — Output only IDs.
- `--no-header` — Omit table headers.

## mr mrql save — Save a query

```bash
mr mrql save "my-query-name" 'type = resource AND tags = "photo"'
mr mrql save "large-files" 'type = resource AND fileSize > 100mb' --description "Files over 100MB"
```

**Flags:**
- `--description <text>` — Optional description for the saved query.

Returns the saved query's ID (for later use with `mrql delete`).

## mr mrql list — List saved queries

```bash
mr mrql list
mr mrql list --json | jq -r '.[] | "\(.id)\t\(.name)\t\(.query)"'
```

Returns paginated list of saved MRQL queries. Use `--page <n>` for pagination (default page size 50).

## mr mrql run — Execute a saved query

```bash
mr mrql run 42                                    # by ID
mr mrql run "my-query-name"                       # by name
mr mrql run "resources-by-type" --buckets 5       # with pagination
mr mrql run "recent-photos" --json | jq '.resources[].ID'  # JSON output
```

**Flags:**
- `--limit <n>` — Override the saved query's limit.
- `--buckets <n>` — Pagination for bucketed GROUP BY.
- `--offset <n>` — Bucket page offset.
- `--render` — Enable template rendering.
- `--page <n>` — Global page.

Returns the same response shape as `mrql` (standard or grouped result).

## mr mrql delete — Delete a saved query

```bash
mr mrql delete 42
mr mrql list --json | jq -r '.[] | select(.name == "my-query") | .id' | xargs mr mrql delete
```

Accepts only numeric ID (not name). Destructive.

## mr search — Full-text search

```bash
mr search "invoice"                      # search all entities
mr search "invoice" --types resources    # resources only
mr search "invoice" --types resources,notes  # multiple types
mr search "report" --limit 5 --json | jq '.total'  # cap results
```

**Flags:**
- `--types <types>` — Comma-separated entity types (e.g., `resources,notes`). Default: all types.
- `--limit <n>` — Maximum results (default 20).
- `--json` — Raw JSON output.
- `--quiet` — IDs only.

Returns: `{query, total, results: [{id, type, name, score, description, url, extra}]}`.

Supports FTS5 syntax: phrase queries (`"exact phrase"`), boolean operators, prefix matching (`invoice*`).

---

## Saved MRQL Queries vs. Saved SQL Queries

| Aspect | `mr mrql save/run` | `mr query create/run` |
|--------|-------------------|----------------------|
| Language | MRQL (high-level DSL) | Raw SQL (read-only) |
| Scope | Structured entity queries | Full database access |
| Use | Common cross-entity searches | Complex aggregations, joins, custom logic |

Both are persisted; use `mrql` for most user-facing queries, `query` for advanced analytics.

---

## Server Configuration

The default query limit is controlled by the `-mrql-default-limit` flag / `MRQL_DEFAULT_LIMIT` environment variable, which the standard server binary sets to **500** (the internal code fallback is 1000 if left unwired). Queries without an explicit `LIMIT` clause apply this default.

## Response Shapes

### Standard Result (no GROUP BY or non-aggregated with GROUP BY)

```json
{
  "entityType": "resource",
  "resources": [{id, name, createdAt}, ...],
  "notes": [],
  "groups": []
}
```

### Grouped Result (aggregated GROUP BY)

```json
{
  "entityType": "resource",
  "mode": "aggregated",
  "rows": [{groupField1, groupField2, count, sum_fileSize, ...}, ...]
}
```

### Grouped Result (bucketed GROUP BY)

```json
{
  "entityType": "resource",
  "mode": "bucketed",
  "groups": [
    {
      "key": {contentType: "image/png"},
      "items": [{id, name, createdAt}, ...]
    },
    ...
  ],
  "totalGroups": 150,
  "nextOffset": 10
}
```

## Rendering

Pass `--render` or use `render=1` query parameter on `POST /v1/mrql` to populate `renderedHTML` fields via `CustomMRQLResult` templates defined on Category, Resource Category, or Note Type. Entities without a template omit the field.
