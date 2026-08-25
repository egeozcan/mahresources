---
title: mr note get
description: Get a note by ID
sidebar_label: get
---

# mr note get

Get a note by ID and print its metadata. Fetches the full record
including name, description, meta JSON, attached tags/groups/resources,
owner group, note type, optional start/end dates, and the share token
(when the note is currently shared). The default key/value table prints
ID, name, description and the created and updated timestamps, plus owner,
note type and share token when they are set; pass the global `--json`
flag for the full record.

## Usage

```bash
mr note get <id>
```

Positional arguments:

- `<id>`


## Examples

**Get a note by ID (table output)**

```bash
mr note get 42
```

**Get as JSON and extract the name with jq**

```bash
mr note get 42 --json | jq -r .Name
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

Note object with ID (uint), Name (string), Description (string), Meta (object), Tags ([]Tag), Groups ([]Group), Resources ([]Resource), OwnerId (*uint), NoteTypeId (*uint), shareToken (*string, omitempty)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note create`](./create.md)
- [`mr note edit-name`](./edit-name.md)
- [`mr notes list`](../notes/list.md)
