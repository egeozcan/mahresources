---
title: mr groups add-tags
description: Add tags to multiple groups
sidebar_label: add-tags
---

# mr groups add-tags

Attach one or more Tags to a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to add. The server merges the
requested tag links with whatever each Group already has; existing
links are unaffected, and no tag links are removed.

Verify the result by reading a target Group back with
`mr group get <id> --json | jq '.Tags'`.

## Usage

```bash
mr groups add-tags
```

## Examples

**Tag three groups with tag 5**

```bash
mr groups add-tags --ids 10,11,12 --tags 5
```

**Add multiple tags to one group**

```bash
mr groups add-tags --ids 10 --tags 5,6,7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated group IDs (required) **(required)** |
| `--tags` | string | `` | Comma-separated tag IDs (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Status object with ok (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr groups remove-tags`](./remove-tags.md)
- [`mr group get`](../group/get.md)
- [`mr tags list`](../tags/list.md)
