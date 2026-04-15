---
title: mr query
description: Get, create, run, or delete a saved query
sidebar_label: query
---

# mr query

A Query is a saved, named search definition. Queries store SQL text
(with optional template interpolation) that can be re-executed on
demand against the mahresources database. Each Query has an ID, name,
description, the SQL Text itself, and an optional Template. Queries
are read-only: `run` executes against a read-only database handle and
returns rows as JSON objects.

Use the `query` subcommands to operate on a single query by ID:
`create` to register new SQL, `get` to fetch metadata, `edit-name` /
`edit-description` to update fields, `run` / `run-by-name` to execute,
and `schema` to inspect the available tables and columns when
authoring query text. Use `queries list` to discover existing queries.

## Usage

```bash
mr query
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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr queries list`](../queries/list.md)
- [`mr mrql`](../mrql/index.md)
- [`mr search`](../search.md)
