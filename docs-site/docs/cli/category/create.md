---
title: mr category create
description: Create a new category
sidebar_label: create
---

# mr category create

Create a new Category. `--name` is required; `--description` is optional
free-form text. A `--custom-*` flag exists for every category template
slot -- the detail page header, sidebar and footer, the list card summary,
avatar and hover card, the list page header and footer, the Own Entities
section body, MRQL result cards, and the CSS that styles them. Each takes
an HTML or template string applied to Groups in this category, except
`--custom-css`, which is injected as a `<style>` block on detail and list
pages. Run `mr category create --help` for the full list with a one-line
description of where each renders. `--meta-schema` and
`--section-config` take JSON strings
controlling structured metadata and which sections render on group
detail pages. On success prints a confirmation line with the new ID;
pass the global `--json` flag to emit the full record for scripting.

## Usage

```bash
mr category create
```

## Examples

**Create a category with just a name**

```bash
mr category create --name "Project"
```

**Create with a description and capture the ID via jq**

```bash
ID=$(mr category create --name "Location" --description "Places you know about" --json | jq -r .ID)
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Category name (required) **(required)** |
| `--description` | string | `` | Category description |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on group detail pages for this category |
| `--custom-header` | string | `` | Rendered at the top of the group detail page |
| `--custom-detail-footer` | string | `` | Rendered at the bottom of the group detail page, below every built-in section |
| `--custom-sidebar` | string | `` | Rendered in the group detail page sidebar |
| `--custom-summary` | string | `` | Rendered on group cards in list views, below the title |
| `--custom-avatar` | string | `` | Replaces the default avatar on group cards |
| `--custom-hover-card` | string | `` | Rendered in the hover card for a group link; falls back to --custom-summary when unset |
| `--custom-list-header` | string | `` | Rendered above group list pages filtered to exactly this category, against the category itself |
| `--custom-list-footer` | string | `` | Rendered below group list pages filtered to exactly this category, against the category itself |
| `--custom-mrql-result` | string | `` | Template for rendering groups of this category in MRQL results |
| `--custom-css` | string | `` | CSS injected as a &lt;style&gt; block on the group detail page and its list pages |
| `--custom-own-entities` | string | `` | Replaces the body of the group detail page's Own Entities section |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created Category object with ID (uint), Name (string), Description (string), MetaSchema, sectionConfig, CustomHeader/DetailFooter/Sidebar/Summary/Avatar/HoverCard/OwnEntities/ListHeader/ListFooter/MRQLResult/CSS, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr category get`](./get.md)
- [`mr category edit-name`](./edit-name.md)
- [`mr categories list`](../categories/list.md)
