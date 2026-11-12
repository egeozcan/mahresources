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

Queries are read-only whatever `DB_READONLY_DSN` points at, and they read only **content tables**: Resources, Notes, Groups, Tags, Categories, Resource Categories, Note Types, Series, Queries, saved MRQL queries, template partials, versions, previews, hashes and similarities, relations, the tables that join them, and their full-text indexes. Every other table is off limits: accounts, sessions and API tokens, runtime settings, the audit log, per-user settings and saved searches, plugin state, Jobs, downloads, Reductions and template history. A table added in a future version is off limits until it is explicitly put on the list.

- **SQLite:** the connection refuses every write, DDL statement, `ATTACH`, `PRAGMA` (bar the read of `data_version` that full-text `MATCH` queries make internally) and transaction statement, refuses to read any table outside the list (and the `sqlite_master` catalogue), and refuses views. CTEs, subqueries and `json_each`/`json_tree` work.
- **PostgreSQL:** each query runs as a single statement inside a `READ ONLY` transaction, as the role `DB_READONLY_DSN` connects as. The server checks that role at startup and at most a minute before each run; see [the read-only role](#the-postgresql-read-only-role) below.

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

This returns table and column definitions for the tables saved queries may read. Group-limited accounts get `403`; on PostgreSQL with `-auth`, a read-only role that fails the server's check gets `503`.

## API Endpoints

:::warning
On SQLite the server enforces read-only access and the table deny list itself, on every connection it opens for saved queries, whatever the DSN contains. With `DB_READONLY_DSN` unset on SQLite, queries run against an empty private database and answer `no such table`; point it at the main database file (for example `file:/data/mahresources.db?mode=ro`) to query your data.

On PostgreSQL the server refuses writes with a read-only transaction, and checks that the role behind `DB_READONLY_DSN` can read the content tables and nothing else. With `-auth` on, running a query and `GET /v1/query/schema` answer `503` with the failed check while it does not; see [the read-only role](#the-postgresql-read-only-role). Multi-statement query text is refused on PostgreSQL.
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

Returns table and column definitions for constructing Queries, listing only the tables saved queries may read. Refused (`403`) to group-limited accounts, and (`503`) on PostgreSQL with `-auth` while the read-only role fails the server's check.

## The PostgreSQL read-only role

On PostgreSQL, which tables a saved query can read is decided by the grants of the role `DB_READONLY_DSN` connects as. The server prints the SQL that creates a correctly restricted role:

```bash
./mahresources -print-readonly-role-sql=mahres_readonly > readonly-role.sql
psql "$DB_DSN" -f readonly-role.sql
psql "$DB_DSN" -c '\password mahres_readonly'
```

Run it as the role that owns the tables (the `DB_DSN` role) or as a superuser, after the server has started once so every table exists, then point `DB_READONLY_DSN` at the new role. Running it again is safe and resets the role to exactly these grants. Tables a later version adds get no grant, so they stay unreadable.

The server checks the role through the read-only connection at startup and again at most a minute before a query runs, so a later `GRANT` is noticed. The check fails, and the log names which part failed, when the role:

- is the `DB_DSN` role, a member of it, or a member of any other role it could `SET ROLE` to;
- is a superuser or has `BYPASSRLS`;
- is a member of `pg_read_server_files`, `pg_write_server_files`, `pg_execute_server_program`, `pg_read_all_data` or `pg_write_all_data`;
- connects to a different database than `DB_DSN`;
- has `CREATE` on any schema (PostgreSQL 14 and older grant it on `public` to every role; revoke it from `PUBLIC` after granting it to the `DB_DSN` role);
- can write to, reference or trigger on any table, or use any sequence;
- can `SELECT` any column of a table or view outside the content tables, including through a grant to `PUBLIC`.

While the check fails:

- **with `-auth`**, saved queries and the schema listing are disabled (`503`, naming the failed check), and the Queries pages show a warning;
- **without `-auth`**, every request already acts as the administrator, so queries keep running; the server logs a warning at startup and the Queries pages show it.

## Security

- Saved queries cannot write, and they read only the content tables listed above. On SQLite the server enforces that itself; on PostgreSQL it enforces read-only transactions and refuses (under `-auth`) to run on a role that can read more.
- The content tables are not scoped by group: whoever can run a saved query sees every row of them. Restrict access to the Mahresources instance accordingly.
- With `-auth` enabled, group-limited users and guests cannot run saved queries at all: the runner refuses fail-closed, because arbitrary SQL cannot be confined to a group subtree. Every unscoped principal (admin, editor, or an unscoped user) can run them, and the results are not group-scoped. Creating, editing and deleting a Query requires the editor role.
