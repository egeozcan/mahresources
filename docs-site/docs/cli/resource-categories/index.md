---
title: mr resource-categories
description: List resource categories
sidebar_label: resource-categories
---

# mr resource-categories

Discover ResourceCategories. The `resource-categories` subcommand group
currently exposes `list` for filtered queries against the full set of
resource categories, with pagination via the global `--page` flag.

Resource categories are the per-Resource taxonomy (compare `categories`
for per-Group). Use `resource-categories list --json | jq` to derive
IDs for scripting, and `resource-category` for single-category CRUD.

## Usage

```bash
mr resource-categories
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

- [`mr resource-category get`](../resource-category/get.md)
- [`mr resources list`](../resources/list.md)
- [`mr categories list`](../categories/list.md)
