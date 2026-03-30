# MRQL GROUP BY Support

**Date:** 2026-03-30
**Status:** Design approved

## Overview

Add GROUP BY with aggregate functions to MRQL, supporting two modes:
- **Aggregated mode** — when aggregate functions are specified, returns flat rows with computed values (COUNT, SUM, AVG, MIN, MAX)
- **Bucketed mode** — when no aggregates are specified, returns entity rows organized into groups

GROUP BY requires an explicit entity type (`type = "resource"`, etc.) and does not work in cross-entity mode.

## Syntax

Grammar extension:

```
[expression] [GROUP BY field1, field2, ...] [AGG1() AGG2(field) ...] [ORDER BY ...] [LIMIT n] [OFFSET n]
```

### Aggregated mode examples

```
type = "resource" GROUP BY contentType COUNT()
type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize) ORDER BY sum_fileSize DESC
type = "resource" GROUP BY meta.source COUNT()
type = "note" GROUP BY owner, noteType COUNT()
type = "resource" AND fileSize > 10mb GROUP BY contentType COUNT() MIN(fileSize) MAX(fileSize)
```

### Bucketed mode examples

```
type = "resource" GROUP BY contentType LIMIT 5
type = "resource" AND contentType = "image/*" GROUP BY meta.camera_model LIMIT 10
type = "note" GROUP BY owner, noteType ORDER BY owner ASC LIMIT 3
```

## Aggregate Functions

| Function | Argument | Allowed field types | Output key |
|----------|----------|-------------------|------------|
| `COUNT()` | none | n/a | `count` |
| `SUM(field)` | required | FieldNumber, FieldMeta (numeric cast) | `sum_{field}` |
| `AVG(field)` | required | FieldNumber, FieldMeta (numeric cast) | `avg_{field}` |
| `MIN(field)` | required | FieldNumber, FieldDateTime, FieldMeta | `min_{field}` |
| `MAX(field)` | required | FieldNumber, FieldDateTime, FieldMeta | `max_{field}` |

## Token Changes

New token types:
- `TokenGroupBy` — `GROUP BY` (two-word keyword, same merging pattern as `ORDER BY`)
- `TokenCount` — `COUNT`
- `TokenSum` — `SUM`
- `TokenAvg` — `AVG`
- `TokenMin` — `MIN`
- `TokenMax` — `MAX`

Aggregate names are recognized as keywords only when followed by `(` — the lexer peeks ahead (same approach as `ORDER BY` merging) and emits the aggregate token type only if `(` follows. Otherwise it emits `TokenIdentifier` so that fields named `count`, `min`, etc. are not broken.

## AST Changes

New AST nodes:

```go
// AggregateFunc represents COUNT(), SUM(field), AVG(field), etc.
type AggregateFunc struct {
    Token Token      // the function keyword token
    Name  string     // "COUNT", "SUM", "AVG", "MIN", "MAX"
    Field *FieldExpr // nil for COUNT(), required for others
}

// GroupByClause holds the GROUP BY fields and optional aggregates.
type GroupByClause struct {
    Fields     []*FieldExpr    // GROUP BY field1, field2, ...
    Aggregates []AggregateFunc // COUNT() SUM(fileSize) ... (may be empty)
}
```

Query struct addition:

```go
type Query struct {
    Where      Node
    GroupBy    *GroupByClause  // nil when no GROUP BY
    OrderBy    []OrderByClause
    Limit      int
    Offset     int
    EntityType EntityType
}
```

## Parser Changes

The parser's `parseQuery()` flow becomes:

1. Parse filter expression (existing)
2. **Parse GROUP BY** — if next token is `TokenGroupBy`, consume comma-separated field expressions
3. **Parse aggregates** — greedily consume aggregate function tokens (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX` followed by `(`)
4. Parse ORDER BY (existing)
5. Parse LIMIT (existing)
6. Parse OFFSET (existing)

Aggregates are only valid after a GROUP BY clause. A bare aggregate without GROUP BY is a parse error.

## Validator Rules

### Entity type required
GROUP BY queries must have `type = "..."` set. Error: `"GROUP BY requires an explicit entity type"`.

### Group-by field validation
Each GROUP BY field must be valid for the entity type via `LookupField()`. Allowed:
- `FieldString` — contentType, originalName, hash, etc.
- `FieldNumber` — fileSize, width, height, category, etc.
- `FieldDateTime` — created, updated
- `FieldMeta` — meta.*
- `FieldRelation` — tags, owner, groups (grouped by name/ID)

FK traversal paths (e.g., `owner.parent.name`) are **not** allowed. Error: `"GROUP BY does not support traversal paths; use a direct field like 'owner' instead"`.

### Aggregate field validation
- `COUNT()` must have no field argument
- `SUM()`, `AVG()` require `FieldNumber` (or FieldMeta with numeric cast)
- `MIN()`, `MAX()` allow `FieldNumber`, `FieldDateTime` (or FieldMeta)
- Aggregate fields must be valid for the entity type

### ORDER BY interaction
- **Aggregated mode**: ORDER BY can reference group-by field names or aggregate output keys (`count`, `sum_fileSize`, etc.)
- **Bucketed mode**: ORDER BY applies to items within each bucket (standard entity field ordering)

## Translator Changes

### Aggregated mode (aggregates present)

Translates to SQL `SELECT group_fields, aggregates FROM table WHERE ... GROUP BY group_fields ORDER BY ... LIMIT ...`.

Example — `type = "resource" AND fileSize > 1mb GROUP BY contentType COUNT() SUM(fileSize)`:

```sql
-- SQLite
SELECT content_type AS "contentType", COUNT(*) AS "count", SUM(file_size) AS "sum_fileSize"
FROM resources WHERE file_size > 1048576
GROUP BY content_type

-- PostgreSQL (same structure, dialect-specific quoting)
SELECT content_type AS "contentType", COUNT(*) AS "count", SUM(file_size) AS "sum_fileSize"
FROM resources WHERE file_size > 1048576
GROUP BY content_type
```

Meta fields:
```sql
-- SQLite:  json_extract(meta, '$.source') AS "meta.source"
-- PG:      meta->>'source' AS "meta.source"
```

Relation fields (e.g., `GROUP BY tags`): JOIN through the junction table, GROUP BY tag name.

Relation fields (e.g., `GROUP BY owner`): JOIN to groups table, GROUP BY group name (select name for display, use FK column for grouping).

Returns `[]map[string]any` — a new return path distinct from typed entity results.

### Bucketed mode (no aggregates)

Two-query approach:
1. **Keys query**: `SELECT DISTINCT group_fields FROM table WHERE ... ORDER BY group_fields` — unique bucket keys
2. **Items query per bucket**: existing entity query with added `WHERE group_field = key`, applying LIMIT as per-bucket cap

Maximum 1000 buckets to prevent runaway queries.

### GORM integration
- Aggregated: `db.Select(...).Group(...).Find(&[]map[string]any{})`
- Bucketed: reuses existing `db.Find(&entities)` with additional WHERE clauses

## Execution Layer

### New result types

```go
// MRQLGroupedResult is returned when GROUP BY is present.
type MRQLGroupedResult struct {
    EntityType string           `json:"entityType"`
    Mode       string           `json:"mode"` // "aggregated" or "bucketed"
    Rows       []map[string]any `json:"rows,omitempty"`
    Groups     []MRQLBucket     `json:"groups,omitempty"`
    Warnings   []string         `json:"warnings,omitempty"`
}

type MRQLBucket struct {
    Key   map[string]any `json:"key"`
    Items any            `json:"items"` // []Resource, []Note, or []Group
}
```

### ExecuteMRQL flow

After parse and validate, if `parsed.GroupBy != nil`, branch to `executeGroupedQuery()` instead of `executeSingleEntity()` / `executeCrossEntity()`. The validator ensures entity type is set.

### LIMIT behavior
- **Aggregated mode**: LIMIT applies to number of aggregate rows (standard SQL)
- **Bucketed mode**: LIMIT applies per bucket
- Default limit (1000) applies to aggregate rows or per-bucket items respectively

## API Response

`POST /v1/mrql` returns `MRQLGroupedResult` when GROUP BY is present, `MRQLResult` otherwise. The `mode` field distinguishes response shapes. No breaking change to existing queries.

### Aggregated response example

```json
{
  "entityType": "resource",
  "mode": "aggregated",
  "rows": [
    {"contentType": "image/png", "count": 142, "sum_fileSize": 1073741824},
    {"contentType": "image/jpeg", "count": 87, "sum_fileSize": 524288000}
  ]
}
```

### Bucketed response example

```json
{
  "entityType": "resource",
  "mode": "bucketed",
  "groups": [
    {
      "key": {"contentType": "image/png"},
      "items": [
        {"id": 1, "name": "screenshot.png", "fileSize": 204800},
        {"id": 7, "name": "diagram.png", "fileSize": 102400}
      ]
    },
    {
      "key": {"contentType": "image/jpeg"},
      "items": [
        {"id": 3, "name": "photo.jpg", "fileSize": 3145728}
      ]
    }
  ]
}
```

## CLI Output (`mr mrql`)

- **Aggregated mode**: render as a table with columns for group keys + aggregates
- **Bucketed mode**: render each bucket with a header line showing the key, then entity rows underneath

## Autocompletion Changes

- After a filter expression: suggest `GROUP BY` alongside `ORDER BY`, `LIMIT`, `OFFSET`
- After `GROUP BY`: suggest field names for the current entity type
- After GROUP BY fields: suggest aggregate functions (`COUNT()`, `SUM()`, `AVG()`, `MIN()`, `MAX()`)
- Inside aggregate parentheses (SUM/AVG/MIN/MAX): suggest numeric fields (and datetime for MIN/MAX)
- After aggregates: suggest `ORDER BY`, `LIMIT`, `OFFSET`

## Documentation

Update `docs-site/docs/features/mrql.md` with:
- GROUP BY syntax section with both modes
- Aggregate functions table with field type requirements
- Examples for each mode
- Grouping by meta and relation fields
- ORDER BY interaction with grouped queries
- LIMIT behavior differences between modes

## Constraints

- GROUP BY requires explicit entity type — no cross-entity grouping
- FK traversal paths not allowed in GROUP BY fields
- Aggregates only valid with GROUP BY (no standalone aggregates)
- Maximum 1000 buckets in bucketed mode
- Both SQLite and PostgreSQL must be supported with dialect-specific SQL generation

## Testing

- **Lexer tests**: tokenization of GROUP BY, aggregate keywords
- **Parser tests**: GROUP BY with/without aggregates, multiple group fields, error cases
- **Validator tests**: entity type required, field type restrictions, ORDER BY interaction, traversal rejection
- **Translator tests**: SQL generation for both modes, both dialects, meta fields, relation fields
- **Comprehensive tests**: end-to-end with seeded data for both modes
- **API tests**: response shape for aggregated and bucketed modes
- **E2E tests**: CLI output formatting, web UI interaction
