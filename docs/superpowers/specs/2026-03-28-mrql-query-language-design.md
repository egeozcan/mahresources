# MRQL Query Language Design

**Date:** 2026-03-28
**Status:** Approved

## Overview

MRQL (mahresources query language) is a structured text-based query language for mahresources, inspired by Jira Query Language. It provides a unified way to search and filter resources, notes, and groups using explicit field-operator-value syntax with boolean logic, pattern matching, full-text search, relative dates, and relationship traversal.

## Scope

### v1 (this spec)

- Recursive descent parser in Go (`mrql/` package)
- Single-entity and cross-entity query support
- Dedicated query page at `/v1/mrql` with CodeMirror 6 editor
- Syntax highlighting, inline errors, autocompletion (static + dynamic values)
- Saved MRQL queries (separate from existing raw SQL saved queries)
- `mr mrql` CLI command with full output mode support
- Documentation site updates
- Comprehensive test coverage (unit, E2E browser, E2E CLI, accessibility)

### v2 (deferred)

- Aggregations / `GROUP BY` / `COUNT()`
- Sub-queries
- Recursive traversal (`ancestors`, `descendants`) for group hierarchies
- Perceptual hash similarity (`SIMILAR TO`)
- Cursor-based (keyset) pagination for deep result sets
- Integration into existing list views as an alternative to filter forms
- Global search (Cmd+K) enhancement with MRQL syntax

## Section 1: Query Language Grammar

### Entity selector

```
type = resource | note | group
```

When omitted, defaults to cross-entity search (runs against resources, notes, and groups).

### Fields per entity type

| Field | Resource | Note | Group | Type |
|---|---|---|---|---|
| `name` | yes | yes | yes | string |
| `description` | yes | yes | yes | string |
| `created` | yes | yes | yes | datetime |
| `updated` | yes | yes | yes | datetime |
| `tags` | yes | yes | yes | string (tag name) |
| `groups` / `group` | yes | yes | -- | string (group name) |
| `category` | yes | -- | yes | string |
| `meta.<key>` | yes | yes | yes | dynamic |
| `parent.<field>` | -- | -- | yes | traversal (one level) |
| `children.<field>` | -- | -- | yes | traversal (one level) |
| `contentType` | yes | -- | -- | string |
| `fileSize` | yes | -- | -- | number (bytes, supports units: `kb`, `mb`, `gb`) |
| `width` | yes | -- | -- | number (pixels) |
| `height` | yes | -- | -- | number (pixels) |
| `originalName` | yes | -- | -- | string |
| `hash` | yes | -- | -- | string |
| `noteType` | -- | yes | -- | string |
| `id` | yes | yes | yes | number |

### Operators

**Comparison:**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `=` | equals | `name = "sunset.jpg"` | exact match |
| `!=` | not equals | `category != "Archive"` | excludes exact match |
| `>` | greater than | `fileSize > 10mb` | file larger than 10 megabytes |
| `>=` | greater or equal | `created >= "2024-01-01"` | created on or after Jan 1 2024 |
| `<` | less than | `width < 1920` | image narrower than 1920px |
| `<=` | less or equal | `meta.rating <= 3` | rating metadata is 3 or below |

**Pattern matching (LIKE):**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `~` | LIKE | `name ~ "sun*"` | name starts with "sun" |
| `!~` | NOT LIKE | `name !~ "*draft*"` | name does not contain "draft" |

Wildcards: `*` matches any number of characters, `?` matches exactly one character. Translated to SQL `%` and `_` internally.

More examples:
- `name ~ "*sunset*"` -- name contains "sunset"
- `name ~ "IMG_????.jpg"` -- matches IMG_ + exactly 4 characters + .jpg
- `originalName ~ "*.png"` -- original filename ends with .png
- `contentType ~ "image/*"` -- any image MIME type

**Existence / emptiness:**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `IS EMPTY` | field has no value / no associations | `tags IS EMPTY` | entity has no tags |
| `IS NOT EMPTY` | field has a value / has associations | `group IS NOT EMPTY` | entity belongs to at least one group |
| `IS NULL` | field is null / unset | `category IS NULL` | no category assigned |
| `IS NOT NULL` | field is not null | `meta.rating IS NOT NULL` | rating metadata exists |

`IS EMPTY` applies to relationship fields (tags, groups, notes, children, parent). `IS NULL` applies to scalar fields (category, noteType, meta keys). Both are valid on fields where the distinction is meaningful.

**Set operators:**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `IN (...)` | matches any value in set | `tags IN ("photo", "video")` | has tag "photo" OR tag "video" |
| `NOT IN (...)` | matches none in set | `category NOT IN ("Archive", "Trash")` | category is neither Archive nor Trash |

**Full-text search:**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `TEXT ~` | FTS5 indexed search | `TEXT ~ "quarterly review"` | full-text match across name, description, originalName |

`TEXT ~` is distinct from `~`. `TEXT ~` uses the FTS5 index for relevance-ranked full-text search. `~` is LIKE pattern matching on a specific field.

**Boolean logic:**

| Operator | Meaning | Example | Interpretation |
|---|---|---|---|
| `AND` | both conditions | `tags = "photo" AND created > -7d` | tagged photo AND created in last 7 days |
| `OR` | either condition | `tags = "photo" OR tags = "video"` | tagged photo OR video |
| `NOT` | negate condition | `NOT name ~ "*draft*"` | name does not contain draft |
| `(...)` | grouping | `(tags = "a" OR tags = "b") AND group = "X"` | either tag, but must be in group X |

**Operator precedence** (highest to lowest, following SQL convention):
1. `NOT`
2. `AND`
3. `OR`

Example: `a OR b AND NOT c` is evaluated as `a OR (b AND (NOT c))`. Use parentheses to override.

### Case sensitivity

All string comparisons (`=`, `!=`, `~`, `!~`, `IN`) are **case-insensitive** by default. This matches user expectations — `name = "project"` will match "Project", "PROJECT", etc.

Implementation: `=` and `!=` use `LOWER()` wrapping on both sides. `~` translates to `LIKE` which is already case-insensitive in SQLite; for PostgreSQL, `ILIKE` is used instead.

### String escaping

Strings are delimited by double quotes. To include a literal double quote inside a string, escape it with a backslash: `\"`. To include a literal backslash, use `\\`.

Examples:
- `description ~ "*said \"hello\"*"` -- matches descriptions containing `said "hello"`
- `name = "file\\backup"` -- matches the literal name `file\backup`

The lexer handles `\"` and `\\` as escape sequences within quoted strings. All other backslash sequences are treated as literal characters.

### Relative dates

Relative to the current time. Supported units: `d` (days), `w` (weeks), `m` (months), `y` (years).

| Expression | Meaning |
|---|---|
| `-7d` | 7 days ago |
| `-2w` | 2 weeks ago |
| `-3m` | 3 months ago |
| `-1y` | 1 year ago |

Example: `created > -30d` -- created in the last 30 days.

### Date functions

| Function | Meaning |
|---|---|
| `NOW()` | current timestamp |
| `START_OF_DAY()` | midnight today |
| `START_OF_WEEK()` | Monday 00:00 of current week |
| `START_OF_MONTH()` | 1st of current month 00:00 |
| `START_OF_YEAR()` | January 1st 00:00 of current year |

Example: `created >= START_OF_MONTH() AND created < NOW()` -- everything created this month so far.

### Ordering and pagination

```
ORDER BY field ASC|DESC [, field ASC|DESC ...]
LIMIT n
OFFSET n
```

Examples:
- `ORDER BY created DESC` -- newest first
- `ORDER BY name ASC, created DESC` -- alphabetical, then newest first within same name
- `LIMIT 50` -- return at most 50 results
- `LIMIT 50 OFFSET 100` -- skip first 100, return next 50

### Traversal (one level only)

For groups, `parent.<field>` and `children.<field>` access immediate parent/child group fields:

- `parent.name = "Projects"` -- direct parent group is named "Projects"
- `parent.category = "Active"` -- direct parent group's category is "Active"
- `parent.tags = "priority"` -- direct parent group has tag "priority"
- `parent IS NULL` -- group has no parent (top-level group)
- `children.tags = "completed"` -- at least one direct child group has tag "completed"
- `children IS EMPTY` -- group has no child groups (leaf group)

**Depth restriction:** Only one level of traversal is allowed. The parser must reject multi-level traversal like `parent.parent.name` with a clear error: `"Multi-level traversal is not supported. Use 'parent.<field>' for one level. Recursive traversal (ancestors/descendants) is planned for v2."` This is validated at parse time, not execution time.

### Complete examples

```
-- All resources named like "sunset"
name ~ "*sunset*"

-- Photos from the last week
type = resource AND tags = "photo" AND created > -7d

-- Journal notes mentioning "meeting" (full-text)
type = note AND noteType = "journal" AND TEXT ~ "meeting"

-- Groups whose parent is in "Projects" category with active children
type = group AND parent.category = "Projects" AND children.tags = "active"

-- Large images, newest first, first page
type = resource AND contentType ~ "image/*" AND fileSize > 10mb ORDER BY created DESC LIMIT 50

-- Resources in Vacation group tagged beach or sunset, not archived
group = "Vacation" AND tags IN ("beach", "sunset") AND NOT category = "Archive"

-- Notes updated this month with high priority metadata
type = note AND updated >= START_OF_MONTH() AND meta.priority = "high"

-- Resources with specific original filename pattern
type = resource AND originalName ~ "IMG_2024*" AND width >= 3840 ORDER BY fileSize DESC

-- Untagged resources (cleanup candidates)
type = resource AND tags IS EMPTY AND created < -30d

-- Top-level groups (no parent)
type = group AND parent IS NULL

-- Notes with rating metadata set
type = note AND meta.rating IS NOT NULL ORDER BY meta.rating DESC

-- Resources with descriptions containing quotes
type = resource AND description ~ "*said \"hello\"*"
```

## Section 2: Parser & Execution Architecture

### Package structure

A hand-written recursive descent parser in a new `mrql/` package. No external parser generator -- keeps dependencies minimal and gives full control over error messages.

```
mrql/
├── lexer.go        # Tokenizer: keywords, operators, strings, numbers, identifiers
├── parser.go       # Recursive descent parser -> AST
├── ast.go          # AST node types (BinaryExpr, UnaryExpr, Field, Literal, FuncCall, etc.)
├── translator.go   # AST -> GORM scopes (reuses existing database_scopes where possible)
├── validator.go    # Field validation per entity type, type checking
├── completer.go    # Given a partial query + cursor position, return completion suggestions
└── mrql_test.go    # Table-driven tests for parse -> AST -> SQL round-trips
```

### Execution flow

```
User query string
    -> Lexer (tokens)
    -> Parser (AST)
    -> Validator (type-check fields against entity type, report errors with positions)
    -> Translator (AST -> GORM query with scopes)
    -> Database execution
    -> Results (typed entities or unified rows)
```

### Key design decisions

1. **Reuse existing scopes** -- the translator maps AST nodes to existing `database_scopes` functions wherever possible (tag filtering, meta queries, date ranges, FTS). New scopes only for things that don't exist yet.

2. **Error positions** -- the lexer and parser track character positions so errors can be reported as `"Unexpected token at position 34: expected field name, got ')'"`. This feeds directly into CodeMirror's error marking.

3. **Completion engine** -- `completer.go` takes a partial query string and cursor position, parses what it can, and returns context-aware suggestions (field names after `AND`, operators after a field, etc.). Dynamic value suggestions (tag names, group names) are fetched separately via API and merged client-side.

4. **SQL injection safety** -- the translator never interpolates user strings into SQL. All values become parameterized GORM `Where` arguments.

5. **FTS5 input sanitization** -- FTS5 `MATCH` has its own internal syntax (e.g., `NEAR()`, boolean operators). User input passed to `TEXT ~` must be sanitized before reaching the FTS5 engine: strip FTS5 operators and special characters, treating the input as a plain text phrase. This prevents malformed FTS input from causing query errors.

6. **Traversal depth enforcement** -- the parser rejects `parent.parent.*` or `children.children.*` at parse time with a descriptive error. This is a hard constraint, not a runtime check.

### Performance considerations

**Leading wildcard queries:** Patterns like `name ~ "*sunset*"` translate to `LIKE '%sunset%'`, which bypasses B-Tree indexes and causes full table scans. This is inherent to LIKE and acceptable for most use cases, but can be slow on tables with millions of rows. The query timeout (see Section 3) mitigates runaway scans. For substring search on large datasets, `TEXT ~` (FTS5) is the recommended alternative.

**Offset pagination:** `LIMIT n OFFSET n` is used for v1 pagination. Deep offsets (e.g., `OFFSET 100000`) force the database to scan and discard rows, which degrades with depth. This is acceptable for v1 where most queries won't paginate deeply. Cursor-based (keyset) pagination is flagged as a v2 optimization.

## Section 3: API Endpoints

### Query timeout

All MRQL query execution is wrapped in a context timeout, configurable via the `-mrql-query-timeout` flag / `MRQL_QUERY_TIMEOUT` env var (default: 10 seconds). If a query exceeds the timeout, the database context is cancelled and the API returns a `408 Request Timeout` with a message explaining the query was too expensive. This prevents accidental DoS from complex cross-entity queries, multiple OR conditions, or leading wildcard patterns on large tables.

### Query execution

```
POST /v1/mrql
Content-Type: application/json

{
  "query": "type = resource AND tags = \"photo\" AND created > -7d ORDER BY created DESC LIMIT 50"
}
```

Response: entity-typed results when single entity type, unified format when cross-entity.

### Autocompletion

```
POST /v1/mrql/complete
Content-Type: application/json

{
  "query": "type = resource AND tags = \"",
  "cursor": 35
}
```

Response:

```json
{
  "suggestions": [
    {"value": "photo", "type": "tag_value"},
    {"value": "video", "type": "tag_value"}
  ]
}
```

The completion endpoint calls `completer.go` for structural suggestions (fields, operators, keywords) and augments with dynamic values from the database (tag names, group names, category names, note types, meta keys).

### Syntax validation (lightweight, no execution)

```
POST /v1/mrql/validate

{
  "query": "type = resource AND AND"
}
```

Response:

```json
{
  "valid": false,
  "errors": [
    {"message": "Expected field name, got keyword AND", "position": 22, "length": 3}
  ]
}
```

### Saved MRQL queries

```
POST   /v1/mrql/saved          # Create saved query
GET    /v1/mrql/saved          # List saved queries
GET    /v1/mrql/saved/{id}     # Get saved query
PUT    /v1/mrql/saved/{id}     # Update saved query
DELETE /v1/mrql/saved/{id}     # Delete saved query
POST   /v1/mrql/saved/{id}/run # Execute a saved query
```

Saved query model:

```go
type SavedMRQLQuery struct {
    ID          uint   `json:"id"`
    Name        string `json:"name"`
    Query       string `json:"query"`
    Description string `json:"description"`
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## Section 4: Frontend -- Query Page & CodeMirror Integration

### Page route

`/v1/mrql` -- a new template-rendered page.

### Layout

```
+---------------------------------------------------+
|  MRQL Query                              [Docs]    |
+---------------------------------------------------+
|  +-----------------------------------+  [Run]      |
|  | CodeMirror editor                 |  [Save]     |
|  | type = resource AND tags = ...    |             |
|  +-----------------------------------+             |
+---------------------------------------------------+
|  Saved Queries: [Recent Photos v] [My Notes v]     |
+---------------------------------------------------+
|                                                    |
|  Results                            50 results     |
|  +------------------------------------------------+|
|  | (entity-aware cards or unified table)          ||
|  | ...                                            ||
|  +------------------------------------------------+|
|  [< Prev]  Page 1 of 3  [Next >]                  |
+---------------------------------------------------+
```

### CodeMirror 6 integration

- Custom language mode for MRQL syntax highlighting (keywords, operators, strings, numbers, field names)
- Bracket matching for parentheses
- Inline error markers from `/v1/mrql/validate` (red underlines with hover tooltips)
- Autocompletion popup triggered by typing, powered by:
  - Local: keywords (`AND`, `OR`, `NOT`, `ORDER BY`, `LIMIT`, etc.), operators, field names per entity type
  - Remote: tag names, group names, category names, note types, meta keys fetched from `/v1/mrql/complete`
- Ctrl+Enter / Cmd+Enter to execute query
- Query history via browser localStorage

### Bundling

CodeMirror 6 is modular -- only import what's needed. Adds ~30KB gzipped to the Vite bundle. Integrated via `src/components/mrqlEditor.js` and initialized on the query page.

### Results rendering

- Single entity type detected: render using existing entity card/row templates (resource thumbnails, note previews, group cards)
- Cross-entity or ambiguous: unified table with columns: Type, Name, Tags, Created, and a link to the entity's detail page

### Date input in autocompletion

When the cursor is in a position expecting a date value (after `created >=`, `updated <`, etc.), the autocompletion popup suggests:
- Relative date shortcuts: `-7d`, `-30d`, `-3m`, `-1y`
- Date functions: `NOW()`, `START_OF_DAY()`, `START_OF_MONTH()`, `START_OF_YEAR()`
- Format hint: `"YYYY-MM-DD"` as a placeholder showing the expected format

No date-picker widget — it adds complexity disproportionate to its value in a text-based query language. The relative date shortcuts and functions cover the most common cases without needing to pick a calendar date.

### Syntax help panel

A collapsible `[Docs]` button that shows a quick-reference of fields, operators, functions, and example queries inline. Static HTML, no extra API call.

## Section 5: CLI -- `mr mrql` Command

### Command structure

```bash
# Execute a query
mr mrql 'type = resource AND tags = "photo" AND created > -7d'

# Execute with ordering/pagination flags (alternative to in-query syntax)
mr mrql 'tags = "photo"' --limit 20 --page 2

# Read query from a file (avoids bash quoting issues)
mr mrql -f query.mrql

# Read query from stdin
cat query.mrql | mr mrql -
echo 'tags = "photo"' | mr mrql -

# Save a query
mr mrql save "recent photos" 'type = resource AND tags = "photo" AND created > -7d'

# List saved queries
mr mrql list

# Run a saved query by name or ID
mr mrql run "recent photos"
mr mrql run 42

# Delete a saved query
mr mrql delete 42
```

The `-f` flag and stdin (`-`) support avoids bash quoting hell for complex queries with escaped quotes or special characters. When `-f` or `-` is used, the query argument is ignored.

### Output modes

Same as all other `mr` commands:

```bash
# Default: formatted table
mr mrql 'tags = "photo"'
# ID    TYPE       NAME              CREATED
# 1234  resource   sunset.jpg        2026-03-15
# 5678  resource   beach.png         2026-03-12

# JSON output
mr mrql 'tags = "photo"' --json

# IDs only (for piping)
mr mrql 'tags = "photo"' --quiet
# 1234
# 5678

# No headers (for scripting)
mr mrql 'tags = "photo"' --no-header
```

### Piping to other commands

```bash
# Tag all matching resources
mr mrql 'type = resource AND contentType ~ "image/*" AND NOT tags = "processed"' --quiet | \
  xargs -I{} mr resource add-tags {} --tags "processed"

# Download all matches
mr mrql 'type = resource AND group = "Export"' --quiet | \
  xargs -I{} mr resource download {} --output ./export/

# Bulk delete notes older than a year
mr mrql 'type = note AND created < -1y AND tags = "temp"' --quiet | \
  xargs mr notes delete
```

### Implementation

New Cobra command in `cmd/mr/commands/mrql.go` with subcommands `save`, `list`, `run`, `delete`. Calls the same `/v1/mrql` and `/v1/mrql/saved` API endpoints.

## Section 6: Documentation Site Updates

### New page: `docs-site/docs/features/mrql.md`

Placed under "Advanced Features" in the sidebar. Contents:

1. **Overview** -- what MRQL is, when to use it vs. regular filters vs. global search
2. **Syntax reference** -- complete field list per entity type, all operators with examples, wildcards, relative dates, date functions, traversal, ordering, pagination
3. **Full-text search** -- how `TEXT ~` differs from `~`, when to use each
4. **Cross-entity queries** -- how omitting `type` searches across all entities
5. **Saved queries** -- creating, managing, and running saved MRQL queries
6. **Examples** -- a cookbook section with 15-20 real-world queries organized by use case (finding files, organizing content, cleanup tasks, etc.)

### Updated page: `docs-site/docs/features/cli.md`

Add `mr mrql` section:
- Command syntax and subcommands
- Output modes
- Piping examples (tag matches, bulk download, bulk delete)

### Sidebar update

`docs-site/sidebars.ts` -- add the new MRQL page to the Advanced Features section.

## Section 7: Testing Strategy

### Go unit tests (`mrql/`)

- **Lexer tests** -- tokenization of all token types, edge cases (escaped quotes with `\"` and `\\`, wildcards `*`/`?`, numbers with units like `10mb`, relative dates like `-7d`)
- **Parser tests** -- table-driven: query string -> expected AST. Cover every operator, nesting, precedence (`NOT` > `AND` > `OR`), IS EMPTY/IS NULL, error cases, multi-level traversal rejection (`parent.parent.name` -> descriptive error)
- **Validator tests** -- field existence per entity type, type mismatches (e.g., `fileSize = "abc"`), traversal on non-group entities, IS EMPTY on scalar fields, IS NULL on relationship fields
- **Translator tests** -- AST -> generated SQL verification. Ensure parameterized queries (no injection). Test that existing database scopes are reused correctly. Verify case-insensitive comparisons. Verify FTS5 input sanitization strips special FTS operators. Verify `*`/`?` wildcards translate to `%`/`_`
- **Completer tests** -- partial query + cursor position -> expected suggestions
- **Round-trip tests** -- query string -> parse -> translate -> execute against in-memory SQLite with test data -> verify result set

### E2E browser tests (`e2e/tests/mrql.spec.ts`)

- Page loads, CodeMirror editor renders
- Type a query, execute, verify results appear
- Autocompletion popup appears and selecting a suggestion inserts it
- Syntax error shows inline error marker
- Save a query, verify it appears in saved list, run it
- Cross-entity query returns mixed results in unified table
- Single-entity query renders entity-aware cards
- Pagination works
- Keyboard shortcut (Cmd+Enter) executes query

### E2E CLI tests (`e2e/tests/cli/cli-mrql.spec.ts`)

- `mr mrql 'name ~ "*test*"'` returns matching results
- `mr mrql ... --json` returns valid JSON
- `mr mrql ... --quiet` returns only IDs
- `mr mrql -f query.mrql` reads query from file
- `echo 'tags = "photo"' | mr mrql -` reads query from stdin
- `mr mrql save`, `mr mrql list`, `mr mrql run`, `mr mrql delete` lifecycle
- Piping: `mr mrql ... --quiet | xargs mr resource add-tags ...`
- Error handling: invalid syntax, unknown fields, query timeout

### Accessibility tests (`e2e/tests/accessibility/mrql-a11y.spec.ts`)

- axe-core scan on the MRQL page
- CodeMirror editor is keyboard navigable
- Error messages are announced to screen readers
- Results table has proper ARIA attributes
