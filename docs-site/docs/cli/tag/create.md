---
title: mr tag create
description: Create a new tag
sidebar_label: create
---

# mr tag create

Create a new tag. `--name` is required and must be unique; `--description`
is optional free-form text. On success prints a confirmation line with
the new ID; pass the global `--json` flag to emit the full record for
scripting (e.g., piping the new ID into follow-up commands).

## Usage

    mr tag create

## Examples

**Create a tag with just a name**

    mr tag create --name "urgent"

**Create with a description and capture the ID via jq**

    ID=$(mr tag create --name "archived" --description "archived items" --json | jq -r .ID)


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Tag name (required) **(required)** |
| `--description` | string | `` | Tag description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr tag get`](./get.md)
- [`mr tag edit-name`](./edit-name.md)
- [`mr tags list`](../tags/list.md)
