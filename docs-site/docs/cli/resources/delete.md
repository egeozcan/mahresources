---
title: mr resources delete
description: Delete multiple resources
sidebar_label: delete
---

# mr resources delete

Bulk-delete Resources. Destructive: removes both the database rows and
the stored file bytes. Target Resources are selected via `--ids` (CSV
of unsigned ints). The current CLI has no dry-run; pipe
`resources list --json` first if you need to preview targets.

## Usage

    mr resources delete

## Examples

**Delete specific resources**

    mr resources delete --ids 42,43,44

**Delete the output of a filter query**

    mr resources list --tags 7 --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources delete --ids {}


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated resource IDs to delete (required) **(required)** |
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

- [`mr resource delete`](../resource/delete.md)
- [`mr resources merge`](./merge.md)
- [`mr resources list`](./list.md)
