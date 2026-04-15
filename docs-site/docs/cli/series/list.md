---
title: mr series list
description: List series
sidebar_label: list
---

# mr series list

List Series, optionally filtered by name or slug. The `--name` and
`--slug` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, SLUG, and CREATED columns; pass
`--json` for the full array.

## Usage

```bash
mr series list
```

## Examples

**List all series (first page)**

```bash
mr series list
```

**Filter by name substring**

```bash
mr series list --name volume
```

**JSON output piped into jq**

```bash
mr series list --json | jq -r '.[].Name'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--slug` | string | `` | Filter by slug |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of Series objects with ID, Name, Slug, Meta, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr series get`](./get.md)
- [`mr series create`](./create.md)
- [`mr resources list`](../resources/list.md)
