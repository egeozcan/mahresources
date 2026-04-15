---
title: mr groups
description: List, merge, or bulk-edit groups
sidebar_label: groups
---

# mr groups

Discover and bulk-mutate Groups. The `groups` subcommands operate on
multiple Groups at once: `list` for filtered queries (with pagination
via the global `--page` flag), `add-tags` / `remove-tags` for bulk
tag ops, `add-meta` for bulk metadata merges, `delete` / `merge` for
destructive consolidation, `meta-keys` to enumerate the observed meta
vocabulary, and `timeline` for an ASCII activity chart.

Most bulk-mutation commands select targets via `--ids=<csv>`; `merge`
uses `--winner` / `--losers` instead. The current CLI does not accept
MRQL selectors on bulk commands — pipe from `groups list --json | jq`
to extract IDs when you need query-based selection.

## Usage

    mr groups

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

- [`mr group get`](../group/get.md)
- [`mr group create`](../group/create.md)
- [`mr resources list`](../resources/list.md)
- [`mr mrql`](../mrql/index.md)
