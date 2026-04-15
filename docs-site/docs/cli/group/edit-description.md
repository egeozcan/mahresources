---
title: mr group edit-description
description: Edit a group's description
sidebar_label: edit-description
---

# mr group edit-description

Replace a Group's `Description` field. Takes the Group ID and the new
description as positional arguments. Sends `POST /v1/group/editDescription`
and returns `{id, ok}` on success. Descriptions are free-form text used
for human-readable context; for structured metadata use `edit-meta`.

## Usage

    mr group edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Update the description on group 42**

    mr group edit-description 42 "Our summer 2026 travel photos"


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
- [`mr group edit-name`](./edit-name.md)
- [`mr group edit-meta`](./edit-meta.md)
