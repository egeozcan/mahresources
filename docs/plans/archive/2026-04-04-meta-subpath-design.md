# Meta Subpath Support in MRQL

## Summary

Enable dotted subpath navigation when querying nested JSON in meta fields. Currently `meta.a.b` is rejected because the meta key validator only allows alphanumeric characters and underscores. This change allows querying into nested JSON objects at any depth.

## Scope

- Dot-only subpath navigation (e.g., `meta.a.b.c`)
- Works everywhere meta fields are used: comparisons, IN, IS NULL, IS EMPTY, ORDER BY, traversal chains
- No array indexing (future extension)
- No JSON array containment (future extension)

## Syntax

No new syntax. The parser already handles `meta.a.b.c` as a multi-part dotted field (`Parts: ["meta", "a", "b", "c"]`). The only parser-level change is bumping `maxFieldParts` from 5 to 8 to accommodate deeper paths and traversal+subpath combinations.

**Examples:**

| MRQL | Parts | Depth |
|------|-------|-------|
| `meta.a.b = 1` | `[meta, a, b]` | 3 |
| `meta.config.database.host = "localhost"` | `[meta, config, database, host]` | 4 |
| `owner.meta.a.b = 1` | `[owner, meta, a, b]` | 4 |
| `parent.parent.meta.a.b.c = "x"` | `[parent, parent, meta, a, b, c]` | 6 |

## Key Validation

Replace whole-path validation with per-segment validation. The existing `isValidMetaKey` function (which checks `^[a-zA-Z0-9_]+$`) is applied to each individual segment rather than the full dotted path. This maintains injection safety while allowing dots.

A new helper extracts the subpath segments from the parsed field parts (everything after `meta`) and validates each one. This helper is shared by all translation call sites.

**Valid:** `meta.a.b`, `meta.config_v2.host`
**Invalid:** `meta.a-b.c` (hyphen), `meta.a b` (space)

## SQL Translation

A shared helper builds the JSON extraction expression from subpath segments, replacing the current inline key-based construction at each call site.

### SQLite

Dots in `json_extract` paths work natively:

```sql
-- meta.a.b.c = 1
json_extract(resources.meta, '$.a.b.c')

-- numeric type filtering
json_type(resources.meta, '$.a.b.c') IN ('integer', 'real')
```

### PostgreSQL

Chained arrow operators for nested paths. Intermediate steps use `->` (returns JSON), final step uses `->>` (returns text):

```sql
-- meta.a.b.c (text extraction)
resources.meta->'a'->'b'->>'c'

-- meta.a.b.c (numeric cast)
CASE WHEN resources.meta->'a'->'b'->>'c' ~ '^-{0,1}[0-9]+(\.[0-9]+){0,1}$'
     THEN (resources.meta->'a'->'b'->>'c')::numeric
     ELSE NULL END
```

### Affected Call Sites

All in `translator.go`, all switching from inline key extraction to the shared helper:

1. **`translateMetaComparison`** — direct meta comparisons (`meta.a.b = 1`)
2. **`translateChainedMetaComparison`** — traversal chains (`owner.meta.a.b = 1`)
3. **`translateInExpr`** — meta IN expressions (`meta.a.b in (1, 2)`)
4. **`translateIsExpr`** — IS NULL / IS EMPTY (`meta.a.b is null`)
5. **`resolveOrderByColumn`** — ORDER BY (`order by meta.a.b`)

## Validator Changes

### Direct meta fields (`validateFieldExpr`)

No change needed. The validator already accepts any field with `meta` prefix — `meta.a.b.c` passes the existing `prefix == "meta"` check.

### Traversal chains with meta leaves (`validateTraversalChain`)

Current check: `part == "meta" && i == len(f.Parts)-2` — only allows exactly one part after `meta`.

New check: `part == "meta" && i < len(f.Parts)-1` — once `meta` is encountered in the chain, everything after it is the subpath. Validation of intermediates stops at that point (the translator validates individual segments).

## Field Lookup

`LookupField` in `fields.go` does a prefix check `fieldName[:5] == "meta."` and returns a synthetic `FieldDef`. This already works for `meta.a.b.c` — no change needed.

## Testing

### Parser tests
- `meta.a.b.c` parses to 4 parts
- 8-part chains work; 9-part chains are rejected

### Validator tests
- `meta.a.b` valid on all entity types
- `owner.meta.a.b` valid
- `parent.parent.meta.a.b.c` valid

### Translator tests (SQLite + Postgres)
- All 5 call sites with subpaths: comparison operators, IN, IS NULL, IS EMPTY, ORDER BY
- Traversal chain + subpath combinations
- Segment validation rejects invalid characters (e.g., `meta.a-b.c` fails)

## Non-Goals

- Array indexing (`meta.items[0].name`) — can be added later without breaking changes
- JSON array containment (`"foo" in meta.a.b`) — separate feature
- Schema validation of meta structure — meta remains schemaless
