---
title: mr resource-categories list
description: List resource categories
sidebar_label: list
---

# mr resource-categories list

List Resource Categories, optionally filtered by name or description.
The `--name` and `--description` flags do substring matching on the
server. Results are paginated via the global `--page` flag (default
page size 50). Default output is a table with ID, NAME, DESCRIPTION,
and CREATED columns; pass `--json` for the full array.

## Usage

```bash
mr resource-categories list
```

## Examples

**List all resource categories (first page)**

```bash
mr resource-categories list
```

**Filter by name substring**

```bash
mr resource-categories list --name photos
```

**JSON output piped into jq**

```bash
mr resource-categories list --json | jq -r '.[].Name'
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

Array of ResourceCategory objects with ID, Name, Description, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-category get`](../resource-category/get.md)
- [`mr resource-category create`](../resource-category/create.md)
- [`mr resources list`](../resources/list.md)
