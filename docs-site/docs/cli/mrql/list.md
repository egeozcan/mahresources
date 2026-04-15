---
title: mr mrql list
description: List saved MRQL queries
sidebar_label: list
---

# mr mrql list

List saved MRQL queries. Pagination is controlled via the global
`--page` flag (default page size 50). Use the global `--json` flag to
retrieve the raw array for scripting; the default table output shows
ID, name, description (truncated), and creation timestamp.

To execute a listed query, use `mrql run <name-or-id>`. To inspect the
stored MRQL text itself, use `mrql list --json` and extract the `.query`
field — there is no dedicated `mrql get` subcommand.

## Usage

```bash
mr mrql list
```

## Examples

**List all saved MRQL queries (first page)**

```bash
mr mrql list
```

**JSON + jq: print each saved query's id**

```bash
mr mrql list --json | jq -r '.[] | "\(.id)\t\(.name)\t\(.query)"'
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

Array of saved MRQL query objects with id, name, query, description, createdAt, updatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql save`](./save.md)
- [`mr mrql run`](./run.md)
- [`mr mrql delete`](./delete.md)
