---
title: mr tags list
description: List tags
sidebar_label: list
---

# mr tags list

List Tags, optionally filtered by name or description. The `--name` and
`--description` flags do substring matching on the server. Results are
paginated via the global `--page` flag (default page size 50). Default
output is a table with ID, NAME, DESCRIPTION, and CREATED columns; pass
`--json` for the full array.

## Usage

```bash
mr tags list
```

## Examples

**List all tags (first page)**

```bash
mr tags list
```

**Filter by name substring**

```bash
mr tags list --name urgent
```

**JSON output piped into jq**

```bash
mr tags list --json | jq -r '.[].Name'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of Tag objects with ID, Name, Description, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr tag get`](../tag/get.md)
- [`mr tags timeline`](./timeline.md)
- [`mr resources list`](../resources/list.md)
