---
title: mr note-type get
description: Get a note type by ID
sidebar_label: get
---

# mr note-type get

Get a note type by ID and print its core fields. The server has no
single-NoteType GET endpoint, so the CLI fetches the full list and
filters in-process; this is slower than a direct lookup on large
instances. The JSON output is a 5-key projection (ID, Name, Description,
CreatedAt, UpdatedAt); use `note-types list --json` when you need the
full record including MetaSchema, SectionConfig, or the Custom* fields.

## Usage

```bash
mr note-type get <id>
```

Positional arguments:

- `<id>`


## Examples

**Get a note type by ID (table output)**

```bash
mr note-type get 1
```

**Get as JSON and extract the name with jq**

```bash
mr note-type get 1 --json | jq -r .Name
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

NoteType projection with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type create`](./create.md)
- [`mr note-type edit`](./edit.md)
- [`mr note-types list`](../note-types/list.md)
