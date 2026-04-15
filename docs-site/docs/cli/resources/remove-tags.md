---
title: mr resources remove-tags
description: Remove tags from multiple resources
sidebar_label: remove-tags
---

# mr resources remove-tags

Remove tag IDs from every Resource listed in `--ids`. Idempotent:
removing a tag that isn't attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

## Usage

```bash
mr resources remove-tags
```

## Examples

**Remove tag 5 from resources 1**

```bash
mr resources remove-tags --ids 1,2 --tags 5
```

**Remove multiple tags at once**

```bash
mr resources remove-tags --ids 1,2,3 --tags 5,6
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated resource IDs (required) **(required)** |
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

- [`mr resources add-tags`](./add-tags.md)
- [`mr resources replace-tags`](./replace-tags.md)
- [`mr tags list`](../tags/list.md)
