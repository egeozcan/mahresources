---
title: mr note-type edit
description: Edit a note type
sidebar_label: edit
---

# mr note-type edit

Edit a note type. `--id` is required; every other flag is optional and
only fields explicitly passed are modified (server-side PATCH
semantics). Use this command when you need to change the `MetaSchema`,
`SectionConfig`, or any of the Custom* rendering fields; the dedicated
`edit-name` / `edit-description` commands only touch those two scoped
fields.

## Usage

    mr note-type edit

## Examples

**Swap the JSON Schema on note type 1**

    mr note-type edit --id 1 \
      --meta-schema '{"type":"object","properties":{"priority":{"type":"string"}}}'

**Update the custom summary template and confirm via list**

    mr note-type edit --id 1 --custom-summary "<div>{{ Note.Name }}</div>"
    mr note-types list --json | jq '.[] | select(.ID == 1).CustomSummary'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | Note type ID (required) **(required)** |
| `--name` | string | `` | Note type name |
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

Updated NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type edit-name`](./edit-name.md)
- [`mr note-type edit-description`](./edit-description.md)
- [`mr note-type get`](./get.md)
