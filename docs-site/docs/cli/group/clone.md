---
title: mr group clone
description: Clone a group
sidebar_label: clone
---

# mr group clone

Create a copy of an existing Group. The clone receives a new ID and
GUID but inherits the source Group's `Name`, `Description`, `Meta`,
`OwnerId`, `CategoryId`, and tag associations. Related resources,
notes, and sub-groups are NOT cloned — use `group export` + `group
import` for a deep subtree copy.

## Usage

    mr group clone <id>

Positional arguments:

- `<id>`


## Examples

**Clone group 42**

    mr group clone 42

**Clone and capture the new ID with jq**

    NEW=$(mr group clone 42 --json | jq -r '.ID')


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Group object for the newly-created clone (new ID, new guid; copied Name, Description, Meta, owner/category references)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr group create`](./create.md)
- [`mr group export`](./export.md)
