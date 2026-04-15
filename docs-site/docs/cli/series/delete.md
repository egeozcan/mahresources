---
title: mr series delete
description: Delete a series by ID
sidebar_label: delete
---

# mr series delete

Delete a series by ID. Destructive: removes the series row. Resources
previously attached to the series keep their bytes but have their
`SeriesId` cleared (the foreign key uses `ON DELETE SET NULL`). Deleting
a nonexistent ID returns exit code 1.

## Usage

    mr series delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a series by ID**

    mr series delete 42

**Delete and pipe the result to jq to confirm the response shape**

    mr series delete 42 --json | jq .


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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr series list`](./list.md)
- [`mr series get`](./get.md)
- [`mr series create`](./create.md)
