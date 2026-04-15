---
title: mr resources add-groups
description: Add groups to multiple resources
sidebar_label: add-groups
---

# mr resources add-groups

Add group IDs to every Resource listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required.

## Usage

```bash
mr resources add-groups
```

## Examples

**Add groups 2 and 3 to resources 1**

```bash
mr resources add-groups --ids 1,2 --groups 2,3
```

**Bulk from a list query**

```bash
mr resources list --content-type image/jpeg --json | jq -r 'map(.id) | join(",")' | xargs -I {} mr resources add-groups --ids {} --groups 7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated resource IDs (required) **(required)** |
| `--groups` | string | `` | Comma-separated group IDs (required) **(required)** |
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
- [`mr resources add-meta`](./add-meta.md)
- [`mr groups list`](../groups/list.md)
