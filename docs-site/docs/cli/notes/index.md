---
title: mr notes
description: List notes and bulk tag/group/meta operations
sidebar_label: notes
---

# mr notes

Discover and bulk-mutate Notes. The `notes` subcommands operate on
multiple Notes at once: `list` for filtered queries (with pagination
via global `--page`), `add-tags` / `remove-tags` for bulk tag ops,
`add-groups` / `add-meta` for bulk annotation, `delete` for destructive
bulk removal, `meta-keys` for discovering the meta-schema vocabulary,
and `timeline` for ASCII activity charts.

Bulk-mutation commands select targets via `--ids=<csv>`. The current
CLI does not support MRQL selectors on bulk commands — pipe from
`notes list --json | jq` to extract IDs when you need query-based
selection.

## Usage

    mr notes

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

- [`mr note get`](../note/get.md)
- [`mr groups list`](../groups/list.md)
- [`mr search`](../search.md)
- [`mr mrql`](../mrql/index.md)
