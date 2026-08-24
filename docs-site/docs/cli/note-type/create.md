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
pages. There is a `--custom-*` flag for every note type template slot --
the detail page header, sidebar and footer, the list card summary, avatar
and hover card, the list page header and footer, and MRQL result cards.
Each accepts raw HTML or a template string that the server injects into
note pages and MRQL result cards; `--custom-css` is injected as a
`<style>` block on detail and list pages. Run
`mr note-type create --help` for the full list with a one-line
description of where each renders.

On success prints a confirmation line with the new ID; pass the global
`--json` flag to emit the full created record for scripting.

## Usage

```bash
mr note-type create
```

## Examples

**Create a minimal note type (name only)**

```bash
mr note-type create --name "Meeting Minutes"
```

**Create with a JSON Schema constraining metadata**

```bash
mr note-type create --name "Bug Report" \
  --meta-schema '{"type":"object","properties":{"severity":{"type":"string"}}}'
```

**Capture the new ID via jq for follow-up commands**

```bash
NT=$(mr note-type create --name "Code Review" --json | jq -r .ID)
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Note type name (required) **(required)** |
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

Created NoteType with ID, Name, Description, MetaSchema, SectionConfig, CustomHeader/CSS/Sidebar/Summary/Avatar/MRQLResult, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-type get`](./get.md)
- [`mr note-type edit`](./edit.md)
- [`mr note-types list`](../note-types/list.md)
