---
title: mr notes remove-tags
description: Remove tags from multiple notes
sidebar_label: remove-tags
---

# mr notes remove-tags

Remove tag IDs from every Note listed in `--ids`. Idempotent: removing
a tag that isn't attached is a no-op. Both `--ids` and `--tags` accept
comma-separated unsigned integer lists and are required.

## Usage

```bash
mr notes remove-tags
```

## Examples

**Remove tag 5 from notes 1**

```bash
mr notes remove-tags --ids 1,2 --tags 5
```

**Remove multiple tags at once**

```bash
mr notes remove-tags --ids 1,2,3 --tags 5,6
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated note IDs (required) **(required)** |
| `--tags` | string | `` | Comma-separated tag IDs (required) **(required)** |
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

- [`mr notes add-tags`](./add-tags.md)
- [`mr notes list`](./list.md)
- [`mr tags list`](../tags/list.md)
