---
title: mr query schema
description: Show database table and column names for query building
sidebar_label: schema
---

# mr query schema

List the tables saved SQL may read and their columns, for use as a
reference when authoring query Text. The response is a single JSON
object whose keys are table names and whose values are arrays of
column name strings. The content tables (e.g. `resources`, `notes`,
`groups`), their join tables and, on SQLite, their FTS tables
appear; `users`, `sessions`, `api_tokens` and every other table
`query run` cannot read are left out. A group-limited account,
which cannot run saved SQL, gets `403`; on PostgreSQL with `-auth`,
a read-only role that fails the server's check gets `503`.

Handy as a quick discovery tool before writing a new saved query.

## Usage

```bash
mr query schema
```

## Examples

**Dump the full schema as JSON**

```bash
mr query schema
```

**List only the column names of the `resources` table**

```bash
mr query schema --json | jq -r '.resources[]'
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

Object mapping table name (string) to an array of column names (string[])

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query create`](./create.md)
- [`mr query run`](./run.md)
- [`mr mrql`](../mrql/index.md)
