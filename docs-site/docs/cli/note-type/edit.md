---
title: mr note-type edit
description: Edit a note type
sidebar_label: edit
---

# mr note-type edit

Edit a note type. `--id` is required; every other flag is optional and
only fields explicitly passed are modified (server-side PATCH
semantics). Use this command when you need to change the `MetaSchema`,
`SectionConfig`, or any of the Custom* rendering fields -- there is a
`--custom-*` flag for each template slot, listed with a one-line
description of where it renders under `mr note-type edit --help`. The
dedicated `edit-name` / `edit-description` commands only touch those two
scoped fields. `--custom-css` is injected as a `<style>` block on detail
and list pages.

Because only explicitly-passed flags are sent, passing a `--custom-*`
flag with an empty string is how a slot is cleared; omitting it leaves
the stored value alone.

## Usage

```bash
mr note-type edit
```

## Examples

**Swap the JSON Schema on note type 1**

```bash
mr note-type edit --id 1 \
  --meta-schema '{"type":"object","properties":{"priority":{"type":"string"}}}'
```

**Update the custom summary template and confirm via list**

```bash
mr note-type edit --id 1 --custom-summary "<div>{{ Note.Name }}</div>"
mr note-types list --json | jq '.[] | select(.ID == 1).CustomSummary'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | Note type ID (required) **(required)** |
| `--name` | string | `` | Note type name |
| `--description` | string | `` | Note type description |
| `--meta-schema` | string | `` | JSON Schema defining the metadata structure for notes of this type |
| `--section-config` | string | `` | JSON controlling which sections are visible on note detail pages |
| `--custom-header` | string | `` | Rendered at the top of the note detail page |
| `--custom-detail-footer` | string | `` | Rendered at the bottom of the note detail page, below every built-in section |
| `--custom-sidebar` | string | `` | Rendered in the note detail page sidebar |
| `--custom-summary` | string | `` | Rendered on note cards in list views, below the title |
| `--custom-avatar` | string | `` | Replaces the default avatar on note cards |
| `--custom-hover-card` | string | `` | Rendered in the hover card for a note link; falls back to --custom-summary when unset |
| `--custom-list-header` | string | `` | Rendered above note list pages filtered to exactly this note type, against the note type itself |
| `--custom-list-footer` | string | `` | Rendered below note list pages filtered to exactly this note type, against the note type itself |
| `--custom-mrql-result` | string | `` | Template for rendering notes of this note type in MRQL results |
| `--custom-css` | string | `` | CSS injected as a &lt;style&gt; block on the note detail page and its list pages |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Updated NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/CSS/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type edit-name`](./edit-name.md)
- [`mr note-type edit-description`](./edit-description.md)
- [`mr note-type get`](./get.md)
