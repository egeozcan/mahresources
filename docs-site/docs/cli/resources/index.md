---
title: mr resources
description: List, merge, or bulk-edit resources
sidebar_label: resources
---

# mr resources

Discover and bulk-mutate Resources. The `resources` subcommands operate
on multiple Resources at once: `list` for filtered queries (with
pagination via global `--page`), `add-tags` / `remove-tags` /
`replace-tags` for bulk tag ops, `add-groups` / `add-meta` for bulk
annotation, and `delete` / `merge` for destructive operations.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not support
MRQL selectors on bulk commands — pipe from `resources list --json | jq`
to extract IDs when you need query-based selection.

## Usage

    mr resources

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

- [`mr resource get`](../resource/get.md)
