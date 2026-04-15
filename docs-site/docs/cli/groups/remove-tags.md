---
title: mr groups remove-tags
description: Remove tags from multiple groups
sidebar_label: remove-tags
---

# mr groups remove-tags

Detach one or more Tags from a set of Groups in a single request. Both
arguments are comma-separated ID lists: `--ids` selects the target
Groups and `--tags` selects the Tags to remove. Other tag links on the
targeted Groups are left untouched, and removing a tag that was never
linked is a no-op (not an error).

## Usage

    mr groups remove-tags

## Examples

**Remove tag 5 from three groups**

    mr groups remove-tags --ids 10,11,12 --tags 5

**Remove multiple tags from one group**

    mr groups remove-tags --ids 10 --tags 5,6,7


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

- [`mr groups add-tags`](./add-tags.md)
- [`mr group get`](../group/get.md)
- [`mr tags list`](../tags/list.md)
