---
title: mr group create
description: Create a new group
sidebar_label: create
---

# mr group create

Create a new Group. `--name` is required; all other fields are
optional. Use `--owner-id` to place the new Group under an existing
parent (forming a subtree); use `--category-id` to attach a Category;
pass a JSON blob via `--meta` for free-form custom metadata. Sends
`POST /v1/group` and returns the persisted record.

## Usage

    mr group create

## Examples

**Create a top-level group**

    mr group create --name "Trips 2026"

**Create a child group with meta and a category**

    mr group create --name "Berlin" --owner-id 5 --category-id 2 --meta '{"city":"Berlin"}'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Group name (required) **(required)** |
| `--description` | string | `` | Group description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--meta` | string | `` | Meta JSON string |
| `--url` | string | `` | URL |
| `--owner-id` | uint | `0` | Owner group ID |
| `--category-id` | uint | `0` | Category ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Group object with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr group edit-name`](./edit-name.md)
- [`mr group edit-description`](./edit-description.md)
- [`mr group edit-meta`](./edit-meta.md)
- [`mr groups list`](../groups/list.md)
