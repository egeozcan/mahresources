# models/types/json.go

Custom JSON type and query builder for GORM, forked from
[go-gorm/datatypes/json.go](https://github.com/go-gorm/datatypes/blob/master/json.go).

## What it does

### `JSON` type

A `json.RawMessage` wrapper that implements `driver.Valuer`, `sql.Scanner`,
`json.Marshaler`, `json.Unmarshaler`, and GORM's data type interfaces. Used as
the column type for `Meta` fields on resources, notes, groups, tags, and series.

### `JSONQueryExpression` query builder

Generates dialect-specific SQL for querying into JSON columns. Supports
SQLite/MySQL (`JSON_EXTRACT`) and PostgreSQL (`::jsonb`, `#>`, `?`).

**Operators:** `=`, `<>`, `LIKE`, `NOT LIKE`, `>`, `>=`, `<`, `<=`, `HAS_KEYS`

**Usage (via database scopes):**

```go
types.JSONQuery("meta").Operation(types.OperatorEquals, "hello", "key")
// SQLite: JSON_EXTRACT(`meta`, '$.key') = 'hello'

types.JSONQuery("meta").HasKey("nested", "field")
// SQLite: JSON_EXTRACT(`meta`, '$.nested.field') IS NOT NULL
```

Keys can be passed as separate args or as a dot-separated string (`"a.b.c"`),
which is auto-split.

## Changes from upstream

### Extended query builder

Upstream only has `Equals`, `Likes`, and `HasKey` methods. Our version replaces
these with a single generic `Operation()` method that supports all 8 comparison
operators plus `HAS_KEYS`, driven by `JsonOperation` constants. This is used by
the meta query system (`ColumnMeta` / `ParseMeta`) to support arbitrary
comparisons on JSON fields.

### Bug fixes not in upstream

- **LIKE value mutation on repeated `Build()` calls**: Upstream's `Likes` method
  doesn't wrap with `%` at all (caller must do it). Our `Operation()` wraps
  automatically, but the original implementation mutated `jsonQuery.value` in
  place. If GORM called `Build()` multiple times (count queries, subqueries),
  the value got double-wrapped (`%foo%` -> `%%foo%%`). Fixed by using a local
  variable.

- **SQLite/MySQL nil value handling**: Upstream's `Equals` doesn't handle nil.
  Our `Operation()` supports nil values (for `key:EQ:null` meta queries) and
  now correctly generates `IS NULL` / `IS NOT NULL` on SQLite/MySQL, matching
  the Postgres path. Before the fix, it produced `= NULL` which is always
  false in SQL.

### Improvements borrowed from upstream

These were applied after comparing with the current upstream version:

- **`Value()` simplified**: Removed unnecessary `MarshalJSON()` round-trip;
  `JSON` is already raw bytes, so `string(j)` is sufficient.

- **`Scan()` defensive copy**: `[]byte` values from the database driver are now
  copied to prevent aliasing the driver's reusable internal buffer.

- **`Scan()` skips re-parsing**: Removed the `json.Unmarshal` round-trip. The
  database already stores valid JSON, so re-parsing is wasted work and also
  destroys formatting (e.g., whitespace).

- **`GormValue()` empty check**: Empty `JSON` values now produce SQL `NULL`
  instead of the string `"null"`.
