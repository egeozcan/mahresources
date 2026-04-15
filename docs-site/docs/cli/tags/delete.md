---
title: mr tags delete
description: Delete multiple tags
sidebar_label: delete
---

# mr tags delete

Bulk-delete Tags. Destructive: removes the tag rows and detaches them
from any Resources, Notes, or Groups they were attached to (the related
entities themselves are preserved). Target tags are selected via
`--ids` (CSV of unsigned ints). The current CLI has no dry-run; pipe
`tags list --json` first if you need to preview targets.

## Usage

```bash
mr tags delete
```

## Examples

**Delete specific tags**

```bash
mr tags delete --ids 42,43,44
```

**Delete all tags matching a name filter**

```bash
mr tags delete --ids $(mr tags list --name "obsolete-" --json | jq -r 'map(.ID) | join(",")')
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated tag IDs to delete (required) **(required)** |
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

- [`mr tag delete`](../tag/delete.md)
- [`mr tags merge`](./merge.md)
- [`mr tags list`](./list.md)
