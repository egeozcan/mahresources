---
title: mr group edit-name
description: Edit a group's name
sidebar_label: edit-name
---

# mr group edit-name

Replace a Group's `Name` field. Takes the Group ID and the new name
as positional arguments. Sends `POST /v1/group/editName` and returns
`{id, ok}` on success. Use `group get` afterward to view the updated
record.

## Usage

```bash
mr group edit-name <id> <new-name>
```

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename group 42**

```bash
mr group edit-name 42 "Trips to Berlin"
```


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

Status object with id (uint) and ok (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr group edit-description`](./edit-description.md)
- [`mr group edit-meta`](./edit-meta.md)
