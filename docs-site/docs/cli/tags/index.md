---
title: mr tags
description: List, merge, or bulk-delete tags
sidebar_label: tags
---

# mr tags

Discover and bulk-manage Tags. The `tags` subcommands operate across
multiple tags: `list` for filtered queries (with pagination via global
`--page`), `merge` for folding one or more tags into a single winner,
`delete` for bulk removal, and `timeline` for an activity histogram.

Selection for destructive commands is by ID: `merge` uses
`--winner` / `--losers`, `delete` uses `--ids`. Pipe `tags list --json`
through `jq` when you need to derive IDs from a filter.

## Usage

    mr tags

## Examples


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

- [`mr tag get`](../tag/get.md)
- [`mr resources list`](../resources/list.md)
- [`mr notes list`](../notes/list.md)
