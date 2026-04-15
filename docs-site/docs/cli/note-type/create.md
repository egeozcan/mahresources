---
title: mr note-type create
description: Create a new note type
sidebar_label: create
---

# mr note-type create

Create a new note type. `--name` is required; all other fields are
optional. Pass a JSON Schema string to `--meta-schema` to constrain the
metadata shape of Notes of this type, and a JSON object to
`--section-config` to control which sections render on note detail
pages. The Custom* flags accept raw HTML or Pongo2 template strings
that the server injects into note pages and MRQL result cards.

On success prints a confirmation line with the new ID; pass the global
`--json` flag to emit the full created record for scripting.

## Usage

    mr note-type create

## Examples

**Create a minimal note type (name only)**

    mr note-type create --name "Meeting Minutes"

**Create with a JSON Schema constraining metadata**

    mr note-type create --name "Bug Report" \
      --meta-schema '{"type":"object","properties":{"severity":{"type":"string"}}}'

**Capture the new ID via jq for follow-up commands**

    NT=$(mr note-type create --name "Code Review" --json | jq -r .ID)


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Note type name (required) **(required)** |
| `--description` | string | `` | Note type description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | JSON Schema defining the metadata structure for notes of this type |
| `--section-config` | string | `` | JSON controlling which sections are visible on note detail pages |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering notes of this type in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type get`](./get.md)
- [`mr note-type edit`](./edit.md)
- [`mr note-types list`](../note-types/list.md)
