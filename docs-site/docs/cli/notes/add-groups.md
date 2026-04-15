---
title: mr notes add-groups
description: Add groups to multiple notes
sidebar_label: add-groups
---

# mr notes add-groups

Add group IDs to every Note listed in `--ids`. Idempotent. Both
`--ids` and `--groups` accept comma-separated unsigned integer lists
and are required. The linked Groups appear in the Note's `Groups`
array on subsequent `get` responses.

## Usage

    mr notes add-groups

## Examples

**Add groups 2 and 3 to notes 1**

    mr notes add-groups --ids 1,2 --groups 2,3

**Bulk from a list query**

    mr notes list --tags 5 --json | jq -r '[.[].ID] | join(",")' | xargs -I {} mr notes add-groups --ids {} --groups 7


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated note IDs (required) **(required)** |
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

- [`mr notes add-tags`](./add-tags.md)
- [`mr notes add-meta`](./add-meta.md)
- [`mr groups list`](../groups/list.md)
