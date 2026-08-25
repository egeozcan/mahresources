---
title: mr query run-by-name
description: Run a query by name
sidebar_label: run-by-name
---

# mr query run-by-name

Execute a saved query by its unique `Name` instead of its numeric
ID. Same semantics as `query run`: the query runs on the connection
`-db-readonly-dsn` names, `400` when the statement fails to prepare
or execute, `500` when it fails part-way through the result rows,
`404` when the name does not resolve, and the same
`{"columns": [...], "rows": [[...], ...]}` response. Useful in
scripts where the ID is not known ahead of time but the name is a
stable contract.

Renaming a query via `query edit-name` invalidates callers that
pointed at the old name, so prefer `query run <id>` for
long-running integrations.

## Usage

```bash
mr query run-by-name
```

## Examples

**Run by name**

```bash
mr query run-by-name --name "count-resources"
```

**Run by name and extract the first row's first column**

```bash
mr query run-by-name --name "count-resources" --json | jq '.rows[0][0]'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Query name (required) **(required)** |
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

- [`mr query run`](./run.md)
- [`mr query get`](./get.md)
- [`mr queries list`](../queries/list.md)
