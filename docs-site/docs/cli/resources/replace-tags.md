---
title: mr resources replace-tags
description: Replace tags on multiple resources
sidebar_label: replace-tags
---

# mr resources replace-tags

Set the exact tag set on every Resource listed in `--ids` to the tags
in `--tags`. Any tag not in the list is removed; any tag in the list is
added. Use when you want exact-state semantics rather than delta
semantics. Pass `--tags ""` to clear all tags.

## Usage

    mr resources replace-tags

## Examples

**Replace tags with exactly [5**

    mr resources replace-tags --ids 1 --tags 5,7

**Clear all tags from a resource**

    mr resources replace-tags --ids 1 --tags ""


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
- [`mr resources remove-tags`](./remove-tags.md)
- [`mr tags list`](../tags/list.md)
