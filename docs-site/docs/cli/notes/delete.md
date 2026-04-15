---
title: mr notes delete
description: Delete multiple notes
sidebar_label: delete
---

# mr notes delete

Bulk-delete Notes. Destructive: removes database rows for every Note
listed in `--ids` along with their tag/group/resource associations.
The current CLI has no dry-run; pipe `notes list --json` first if you
need to preview targets before deleting.

## Usage

```bash
mr notes delete
```

## Examples

**Delete specific notes**

```bash
mr notes delete --ids 42,43,44
```

**Delete the output of a filter query**

```bash
mr notes list --tags 7 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes delete --ids {}
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated note IDs to delete (required) **(required)** |
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

- [`mr note delete`](../note/delete.md)
- [`mr notes list`](./list.md)
