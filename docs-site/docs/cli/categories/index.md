---
title: mr categories
description: List group categories
sidebar_label: categories
---

# mr categories

Discover and inspect Categories. The `categories` subcommands operate
across multiple categories: `list` for filtered queries (with pagination
via the global `--page` flag) and `timeline` for an ASCII histogram of
category creation activity.

The CLI has no bulk-mutate variants for categories; use the singular
`category` commands (`create`, `delete`, `edit-name`, `edit-description`)
and pipe `categories list --json` through `jq` when you need to derive
IDs from a filter.

## Usage

```bash
mr categories
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

- [`mr category get`](../category/get.md)
- [`mr groups list`](../groups/list.md)
- [`mr resource-category`](../resource-category/index.md)
