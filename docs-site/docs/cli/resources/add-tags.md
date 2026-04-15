---
title: mr resources add-tags
description: Add tags to multiple resources
sidebar_label: add-tags
---

# mr resources add-tags

Add tag IDs to every Resource listed in `--ids`. Idempotent: adding a
tag that's already attached is a no-op. Both `--ids` and `--tags`
accept comma-separated unsigned integer lists and are required.

## Usage

```bash
mr resources add-tags
```

## Examples

**Add tag 5 to resources 1**

```bash
mr resources add-tags --ids 1,2,3 --tags 5
```

**Add multiple tags at once**

```bash
mr resources add-tags --ids 1,2,3 --tags 5,6,7
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

- [`mr resources remove-tags`](./remove-tags.md)
- [`mr resources replace-tags`](./replace-tags.md)
- [`mr tags list`](../tags/list.md)
