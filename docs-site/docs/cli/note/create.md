---
title: mr note create
description: Create a new note
sidebar_label: create
---

# mr note create

Create a new Note. Only `--name` is required; every other field is
optional. Use `--tags`, `--groups`, and `--resources` (comma-separated
unsigned integer IDs) to link the new Note to existing entities at
creation time. Use `--meta` to attach free-form JSON metadata, and
`--owner-id` / `--note-type-id` to set the owner group and note type
respectively. The created record is returned; capture `.ID` from JSON
output for use in follow-up commands.

## Usage

```bash
mr note create
```

## Examples

**Create a minimal note**

```bash
mr note create --name "shopping list"
```

**Create with description**

```bash
mr note create --name "meeting-notes" --description "Q2 planning" --tags 5,6 --owner-id 42
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Note name (required) **(required)** |
| `--description` | string | `` | Note description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--resources` | string | `` | Comma-separated resource IDs |
| `--meta` | string | `` | Meta JSON string |
| `--owner-id` | uint | `0` | Owner group ID |
| `--note-type-id` | uint | `0` | Note type ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note get`](./get.md)
- [`mr note edit-name`](./edit-name.md)
- [`mr note edit-meta`](./edit-meta.md)
- [`mr notes list`](../notes/list.md)
