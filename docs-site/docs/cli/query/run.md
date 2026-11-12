---
title: mr query run
description: Run a query by ID
sidebar_label: run
---

# mr query run

Execute a saved query by ID and return the result set. The query
runs read-only whatever `-db-readonly-dsn` names, and reads only
content tables (resources, notes, groups, tags, categories, note
types, series, queries, their versions, previews, relations, join
tables and full-text indexes); `query schema` lists them. Accounts,
sessions, API tokens, settings, the audit log, plugin state, Jobs
and every table not on that list are off limits. On SQLite the
connection refuses every write, DDL statement, `ATTACH`, `PRAGMA`
(bar the read of `data_version` full-text search makes),
transaction statement, view and read of a table off the list. On
PostgreSQL the query runs as a single statement in a `READ ONLY`
transaction as the role `-db-readonly-dsn` connects as, and the
server checks that role's grants; with `-auth` on it answers `503`,
naming the failed check, until the role can read nothing else
(`mahresources -print-readonly-role-sql=<role>` prints the SQL that
creates one). With no `-db-readonly-dsn` on SQLite the connection
opens an empty private database, so every query answers
`no such table`. Column
names come verbatim from the SELECT list, so use explicit column
aliases (`select count(*) as n ...`) to produce predictable ones.

The response is `{"columns": [...], "rows": [[...], ...]}`. Columns
keep the order the SELECT list names them, a repeated column name
appears twice rather than being merged, and `columns` is populated
even when no rows matched. Without `--json` the columns become the
header of a text table.

Returns `400 Bad Request` if the statement fails to prepare or
execute (which is how a refused write or a refused table arrives:
`not authorized`, or `access to users.password_hash is
prohibited`), `500` if it fails part-way through the result rows,
and `404 Not Found` if the given ID does not exist. A group-limited
account cannot run saved SQL at all: the refusal arrives as `403`
with `saved SQL queries are not available for group-limited
accounts`. Use MRQL (`mr mrql run`) there, which is scoped at the
executor.

A query whose text uses named parameters (`:since`) needs a value
for each of them. The API reads those from the request body or the
query string, and this command sends neither, so such a query fails
with `could not find name ...`.

## Usage

```bash
mr query run <id>
```

Positional arguments:

- `<id>`


## Examples

**Run a query by ID and print a text table**

```bash
mr query run 42
```

**Run and extract the first row's first column with jq**

```bash
mr query run 42 --json | jq '.rows[0][0]'
```

**Address a column by name rather than by position**

```bash
mr query run 42 --json | jq '.rows[0][(.columns|index("n"))]'
```


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

An object with `columns` (the selected column names, in SELECT order) and `rows` (one array of values per row, index-aligned with columns)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query run-by-name`](./run-by-name.md)
- [`mr query schema`](./schema.md)
- [`mr query get`](./get.md)
