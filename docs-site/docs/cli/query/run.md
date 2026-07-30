---
title: mr query run
description: Run a query by ID
sidebar_label: run
---

# mr query run

Execute a saved query by ID and return the result set. The query
runs against a read-only database handle: any attempt to write
(INSERT/UPDATE/DELETE/DDL) is rejected. Column names come verbatim
from the SELECT list, so use explicit column aliases (`select
count(*) as n ...`) to produce predictable ones.

The response is `{"columns": [...], "rows": [[...], ...]}`. Columns
keep the order the SELECT list names them, a repeated column name
appears twice rather than being merged, and `columns` is populated
even when no rows matched. Without `--json` the columns become the
header of a text table.

Returns `400 Bad Request` if the SQL fails to execute and `404 Not
Found` if the given ID does not exist. For templated queries, the
request body/form values are bound as named SQL parameters.

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
