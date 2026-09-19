---
title: mr resource-category edit
description: Edit a resource category
sidebar_label: edit
---

# mr resource-category edit

Partially edit a Resource Category. `--id` is required and must be positive.
Only explicit flags change stored values; an explicit empty string clears a
field. Supports `--name`, `--description`, `--meta-schema`, `--section-config`
and all resource-category `--custom-*` template flags, including
`--custom-entity-picker-result` and `--custom-entity-picker-result-css`.
The picker template supplies content, not selection controls.

Use `--custom-entity-picker-result-file` or
`--custom-entity-picker-result-css-file` to read UTF-8 HTML or CSS from a file.
Each file flag is mutually exclusive with its corresponding inline flag;
an empty file clears the slot. Files are read before any HTTP request.

## Usage

```bash
mr resource-category edit
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | Carrier ID (required) **(required)** |
| `--name` | string | `` | Carrier name |
| `--description` | string | `` | Carrier description |
| `--meta-schema` | string | `` | JSON Schema defining member metadata |
| `--section-config` | string | `` | JSON controlling member detail-page sections |
| `--custom-header` | string | `` | Rendered at the top of the resource detail page |
| `--custom-header-css` | string | `` | Global CSS for CustomHeader |
| `--custom-detail-footer` | string | `` | Rendered at the bottom of the resource detail page, below every built-in section |
| `--custom-detail-footer-css` | string | `` | Global CSS for CustomDetailFooter |
| `--custom-sidebar` | string | `` | Rendered in the resource detail page sidebar |
| `--custom-sidebar-css` | string | `` | Global CSS for CustomSidebar |
| `--custom-summary` | string | `` | Rendered on resource cards in list views, below the title |
| `--custom-summary-css` | string | `` | Global CSS for CustomSummary |
| `--custom-avatar` | string | `` | Replaces the default avatar on resource cards |
| `--custom-avatar-css` | string | `` | Global CSS for CustomAvatar |
| `--custom-hover-card` | string | `` | Rendered in the hover card for a resource link; falls back to --custom-summary when unset |
| `--custom-hover-card-css` | string | `` | Global CSS for CustomHoverCard |
| `--custom-list-header` | string | `` | Rendered above resource list pages filtered to exactly this resource category, against the resource category itself |
| `--custom-list-header-css` | string | `` | Global CSS for CustomListHeader |
| `--custom-list-footer` | string | `` | Rendered below resource list pages filtered to exactly this resource category, against the resource category itself |
| `--custom-list-footer-css` | string | `` | Global CSS for CustomListFooter |
| `--custom-mrql-result` | string | `` | Template for rendering resources of this resource category in MRQL results |
| `--custom-mrql-result-css` | string | `` | Global CSS for CustomMRQLResult |
| `--custom-entity-picker-result` | string | `` | Template for resource content inside an entity picker result |
| `--custom-entity-picker-result-file` | string | `` | Read --custom-entity-picker-result from a UTF-8 file (mutually exclusive with the inline flag) |
| `--custom-entity-picker-result-css` | string | `` | Global CSS for CustomEntityPickerResult |
| `--custom-entity-picker-result-css-file` | string | `` | Read --custom-entity-picker-result-css from a UTF-8 file (mutually exclusive with the inline flag) |
| `--custom-css` | string | `` | CSS injected as a &lt;style&gt; block on the resource detail page and its list pages |
| `--custom-preview` | string | `` | Rendered above the built-in preview image, for file types it cannot show |
| `--custom-preview-css` | string | `` | Global CSS for CustomPreview |
| `--custom-lightbox` | string | `` | Rendered in the lightbox details panel; falls back to --custom-sidebar when unset |
| `--custom-lightbox-css` | string | `` | Global CSS for CustomLightbox |
| `--custom-cell` | string | `` | Rendered as one extra cell per row in the resources details table |
| `--custom-cell-css` | string | `` | Global CSS for CustomCell |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Updated ResourceCategory object with ID and template fields under --json; confirmation otherwise

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-category create`](./create.md)
- [`mr resource-category get`](./get.md)
- [`mr resource-category edit-name`](./edit-name.md)
- [`mr resource-category edit-description`](./edit-description.md)
