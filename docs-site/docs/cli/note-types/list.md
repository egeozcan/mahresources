---
title: mr note-types list
description: List note types
sidebar_label: list
---

# mr note-types list

List Note Types, optionally filtered by name or description. The
`--name` and `--description` flags do substring matching on the server.
Results are paginated via the global `--page` flag (default page size
50). Default output is a table with ID, NAME, DESCRIPTION, and CREATED
columns; pass `--json` for the full array including MetaSchema,
SectionConfig, and the Custom* rendering fields.

## Usage

    mr note-types list

## Examples

**List all note types (first page)**

    mr note-types list

**Filter by name substring**

    mr note-types list --name meeting

**JSON output piped into jq to extract just names**

    mr note-types list --json | jq -r '.[].Name'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of NoteType objects with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type get`](../note-type/get.md)
- [`mr note-type create`](../note-type/create.md)
- [`mr notes list`](../notes/list.md)
