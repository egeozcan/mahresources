---
title: mr resource-category create
description: Create a new resource category
sidebar_label: create
---

# mr resource-category create

Create a new resource category. `--name` is required; all other flags
are optional, including a plain `--description`, a `--custom-*` flag for
every template slot, and structural fields (`--meta-schema`,
`--section-config`). Resource categories carry four slots the other
carriers do not: `--custom-preview` (above the built-in preview image),
`--custom-lightbox` (the lightbox details panel), and `--custom-cell`
(an extra column in the resources details table). Run
`mr resource-category create --help` for the full list with a one-line
description of where each renders. `--custom-css`
is injected as a `<style>` block on detail and list pages. On success
prints a confirmation line with the new ID; pass the global `--json`
flag to emit the full record for scripting.

## Usage

```bash
mr resource-category create
```

## Examples

**Create a resource category with just a name**

```bash
mr resource-category create --name "Photos"
```

**Create with a description and capture the ID via jq**

```bash
ID=$(mr resource-category create --name "Scans" --description "scanned documents" --json | jq -r .ID)
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Resource category name (required) **(required)** |
| `--description` | string | `` | Resource category description |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on resource detail pages for this category |
| `--custom-header` | string | `` | Rendered at the top of the resource detail page |
| `--custom-detail-footer` | string | `` | Rendered at the bottom of the resource detail page, below every built-in section |
| `--custom-sidebar` | string | `` | Rendered in the resource detail page sidebar |
| `--custom-summary` | string | `` | Rendered on resource cards in list views, below the title |
| `--custom-avatar` | string | `` | Replaces the default avatar on resource cards |
| `--custom-hover-card` | string | `` | Rendered in the hover card for a resource link; falls back to --custom-summary when unset |
| `--custom-list-header` | string | `` | Rendered above resource list pages filtered to exactly this resource category, against the resource category itself |
| `--custom-list-footer` | string | `` | Rendered below resource list pages filtered to exactly this resource category, against the resource category itself |
| `--custom-mrql-result` | string | `` | Template for rendering resources of this resource category in MRQL results |
| `--custom-css` | string | `` | CSS injected as a &lt;style&gt; block on the resource detail page and its list pages |
| `--custom-preview` | string | `` | Rendered above the built-in preview image, for file types it cannot show |
| `--custom-lightbox` | string | `` | Rendered in the lightbox details panel; falls back to --custom-sidebar when unset |
| `--custom-cell` | string | `` | Rendered as one extra cell per row in the resources details table |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-category get`](./get.md)
- [`mr resource-category edit-name`](./edit-name.md)
- [`mr resource-categories list`](../resource-categories/list.md)
