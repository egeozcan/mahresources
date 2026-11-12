---
sidebar_position: 3
title: Saved Queries
---

# Saved Queries

Saved Queries execute raw SQL through a dedicated query runner and display results in a table or custom template.

## Query Properties

| Property | Description |
|----------|-------------|
| `name` | Unique identifier for the Query (used in the UI and URL) |
| `text` | SQL statement to execute |
| `template` | Optional HTML template for custom result rendering |
| `description` | Optional explanation of purpose |

Names must be unique across all Queries.

## How Queries Execute

1. The SQL in `text` is executed via `sqlx` using the configured query connection
2. Named parameters (`:paramName` syntax) are substituted from user input
3. Results are returned as `{columns, rows}` -- the `SELECT` list in its own order, and one array of values per row
4. If a `template` is defined, results render through it; otherwise a default table is used

Queries are read-only whatever `DB_READONLY_DSN` points at. On SQLite the connection refuses every write, DDL statement, `ATTACH`, `PRAGMA` and transaction statement, and refuses to read the credential, secret and audit tables (`users`, `sessions`, `api_tokens`, `plugin_states`, `plugin_kvs`, `runtime_settings`, `log_entries`, `user_settings`, `saved_searches`, `job_replay_envelopes`). On PostgreSQL each query runs as a single statement inside a `READ ONLY` transaction; which tables it can read depends on the role `DB_READONLY_DSN` connects as.

## Creating a Query

1. Navigate to **Queries** in the navigation menu
2. Click **Create**
3. Fill in the Name, Query text, and optionally a Template and Description
4. Click **Submit**

## Named Parameters

Use `:paramName` syntax in SQL. When a Query is run, input fields appear for each parameter.

```sql
SELECT r.id, r.name, r.content_type
FROM resources r
JOIN resource_tags rt ON r.id = rt.resource_id
JOIN tags t ON rt.tag_id = t.id
WHERE t.name = :tagName
ORDER BY r.created_at DESC
```

Running this Query prompts for a `tagName` value. The UI parses input as JSON, so:
- Numbers: `123`
- Strings: `"my value"` (with quotes) or bare text
- Booleans: `true` / `false`
- Null: `null`

### PostgreSQL Type Casts

PostgreSQL `::` casts work normally in saved queries. Write them as usual:

```sql
SELECT meta::jsonb
FROM resources
WHERE id = :id
```

The query runner escapes casts automatically before named-parameter binding.

## Running Queries

1. Navigate to the Query detail page
2. Fill in parameter values
3. Click **Run** (or press Enter in any parameter field)

Results display as a table with JSON formatting for complex fields, collapsible nested objects, clickable URLs, and copy-on-click cells. Results are also available as `window.results` in the browser console.

## Custom Result Templates

The `template` field accepts HTML with Alpine.js. The `results` variable contains the rows as
objects keyed by column name, which is what these templates have always iterated.

Two further variables carry the response exactly as the API sends it: `columns` (the `SELECT` list
in order) and `rows` (one array of values per row, index-aligned with `columns`). `results` is a
convenience projection built from those, and it is lossy in one case -- when a column name appears
twice, the object keeps only the last of the two values. Read `columns` and `rows` when that
matters.

```html
<div class="grid grid-cols-3 gap-4">
  <template x-for="item in results" :key="item.id">
    <div class="p-4 border rounded">
      <a :href="'/resource?id=' + item.id" class="text-blue-600" x-text="item.name"></a>
      <p class="text-sm text-gray-500" x-text="item.content_type"></p>
    </div>
  </template>
</div>
```

## Example Queries

### Resources Without Tags

```sql
SELECT r.id, r.name, r.created_at
FROM resources r
LEFT JOIN resource_tags rt ON r.id = rt.resource_id
WHERE rt.resource_id IS NULL
ORDER BY r.created_at DESC
```

### Resource Statistics by Content Type

```sql
SELECT
  content_type,
  COUNT(*) as count,
  SUM(file_size) as total_size,
  AVG(file_size) as avg_size
FROM resources
GROUP BY content_type
ORDER BY count DESC
```

### Tag Usage Counts

```sql
SELECT
  t.id,
  t.name,
  COUNT(DISTINCT rt.resource_id) as resource_count,
  COUNT(DISTINCT nt.note_id) as note_count,
  COUNT(DISTINCT gt.group_id) as group_count
FROM tags t
LEFT JOIN resource_tags rt ON t.id = rt.tag_id
LEFT JOIN note_tags nt ON t.id = nt.tag_id
LEFT JOIN group_tags gt ON t.id = gt.tag_id
GROUP BY t.id, t.name
ORDER BY resource_count DESC
```

![Saved query editor with SQL and results](/img/query-editor.png)

## Code Editor

The Query create and edit pages use a CodeMirror 6 editor with SQL syntax highlighting, bracket matching, and auto-closing brackets. The editor loads autocompletion data from the database schema endpoint (`/v1/query/schema`), providing table and column name suggestions as you type. Line numbers and undo history are included.

The editor syncs its content to a hidden form input on every change, so the SQL text is submitted with the form.

## Database Schema Endpoint

Retrieve the database schema to help build Queries:

```
GET /v1/query/schema
```

```bash
curl http://localhost:8181/v1/query/schema
```

This returns table and column definitions for the database, without the tables saved queries may not read. Group-limited accounts get `403`.

## API Endpoints

:::warning
On SQLite the server enforces read-only access and the table deny list itself, on every connection it opens for saved queries, whatever the DSN contains. With `DB_READONLY_DSN` unset on SQLite, queries run against an empty private database and answer `no such table`; point it at the main database file (for example `file:/data/mahresources.db?mode=ro`) to query your data.

On PostgreSQL the server refuses writes with a read-only transaction, but it cannot hide tables. Point `DB_READONLY_DSN` at a role without `SELECT` on `users`, `sessions`, `api_tokens`, `plugin_states`, `plugin_kvs`, `runtime_settings`, `log_entries`, `user_settings`, `saved_searches` and `job_replay_envelopes`; the server does not check the role's grants. Multi-statement query text is refused on PostgreSQL.
:::

### List Queries

```
GET /v1/queries
```

```bash
curl http://localhost:8181/v1/queries
```

`GET /v1/queries/timeline` returns per-bucket created and updated counts for Queries, not the Query records themselves.

### Get a Query

```
GET /v1/query?id={id}
```

```bash
curl http://localhost:8181/v1/query?id=3
```

### Create or Update a Query

```
POST /v1/query
```

```bash
curl -X POST http://localhost:8181/v1/query \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "Name=Recent+Resources&Text=SELECT+id,name+FROM+resources+ORDER+BY+created_at+DESC+LIMIT+50"
```

Include `ID` to update an existing Query. In a partial JSON update, fields you omit keep their stored values; a form submission can clear `Template` and `Description`.

### Delete a Query

```
POST /v1/query/delete
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID to delete |

```bash
curl -X POST "http://localhost:8181/v1/query/delete" \
  -d "id=3"
```

### Edit Query Name or Description Inline

```
POST /v1/query/editName
POST /v1/query/editDescription
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID |
| `Name` or `Description` | string | New value |

### Run a Query

```
POST /v1/query/run
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID to execute |
| `name` | string | Alternative: run by Query name instead of ID |
| (body) | JSON | Named parameter values |

```bash
curl -X POST "http://localhost:8181/v1/query/run?id=3" \
  -H "Content-Type: application/json" \
  -d '{"tagName": "photography"}'
```

The response is an object with a `columns` list and a `rows` list:

```json
{
  "columns": ["zebra", "apple"],
  "rows": [
    ["Sunset over the pier", 41],
    ["Harbour at dawn", 58]
  ]
}
```

`columns` is the query's `SELECT` list in the order it was written, and each
entry of `rows` is index-aligned with it. Three consequences:

- Column order is exact. `SELECT name AS zebra, id AS apple` returns
  `["zebra", "apple"]` in that order, not alphabetically.
- A repeated column name (`SELECT id, id`, or a join of two tables that both
  have `id`) appears twice in `columns` and keeps both values.
- `columns` is populated even when the query matched nothing, so a client can
  still draw the table header for an empty result.

A cell's JSON type is decided by its **column**, not by what its contents happen
to spell:

- A column declared `json` or `jsonb` (Postgres), or `JSON` (the SQLite type
  mahresources' `meta` / `section_config` columns are declared with), holds a JSON
  document and is inlined as structure -- including when the document is a scalar,
  so `'123'::jsonb` is the number `123`.
- Every other column is its text. A `text` column spelling `123` stays the string
  `"123"`; so does a `numeric`, a `uuid`, an array (`{a,b}`), and a `bytea` whose
  bytes happen to read as JSON.
- Bytes that are not valid UTF-8 -- a `bytea` of real binary -- are base64-encoded,
  which is the only representation JSON has for them.
- `NULL` is `null`.

SQLite has no declared type for a computed column, so an expression there is
always text: `SELECT json_group_array(name)` returns a string on SQLite while
`SELECT json_agg(name)` returns structure on Postgres. Select the column itself,
or parse client-side, if you need the document.

:::note

Before this rule, a value was typed by sniffing its bytes: anything that parsed
as a JSON object or array became one. That made the same document have two types
depending on how the driver handed it over -- on SQLite,
`SELECT json_object('a',1)` was the string `"{\"a\":1}"` while
`SELECT CAST(json_object('a',1) AS BLOB)` was the object `{"a":1}` -- and it left
scalar `jsonb` values as quoted strings. If you were relying on a `bytea` or BLOB
being re-parsed into structure, parse it yourself.

:::

:::note Changed response shape

This endpoint used to return a bare JSON array of row objects
(`[{"zebra": …, "apple": …}]`). It changed to `{columns, rows}` because a JSON
object cannot carry column order: JavaScript's `Object.keys()` enumerates
integer-like keys first and in numeric order, so a query selecting columns named
`2024` and `2023` came back re-sorted whatever the server wrote. Update any
client that indexes rows by key.

:::

### Get Database Schema

```
GET /v1/query/schema
```

Returns table and column definitions for constructing Queries, leaving out the tables saved queries may not read. Refused (`403`) to group-limited accounts.

## Security

- Saved queries cannot write. On SQLite they also cannot `ATTACH` another database, change a `PRAGMA`, or read the credential, secret and audit tables listed above. On PostgreSQL, hiding those tables is up to the role behind `DB_READONLY_DSN`.
- Results may expose any other data in the database, unscoped by group; restrict access to the Mahresources instance accordingly
- With `-auth` enabled, group-limited users and guests cannot run saved queries at all: the runner refuses fail-closed, because arbitrary SQL cannot be confined to a group subtree. Every unscoped principal (admin, editor, or an unscoped user) can run them, and the results are not group-scoped. Creating, editing and deleting a Query requires the editor role.
