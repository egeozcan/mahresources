---
sidebar_position: 15
title: Built-in Plugins
---

# Built-in Plugins

Mahresources ships with three plugins in the `plugins/` directory. They are not enabled by default. Enable them from the plugin management page or via the API.

Each plugin registers shortcodes for use in custom template fields (CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, CustomMRQLResult) and entity descriptions. Full interactive documentation with live previews is available on each plugin's documentation page after enabling.

## data-views

Data visualization shortcodes for rendering metadata values, charts, tables, and conditional content. All charts use pure HTML/CSS or SVG with no JavaScript dependencies.

| Shortcode | Description |
|-----------|-------------|
| `badge` | Colored status badge from a meta field value |
| `format` | Formatted value display (currency, percent, date, filesize, number, duration) |
| `stat-card` | Card with label, value, and optional icon |
| `meter` | Horizontal gauge bar with min/max/value |
| `sparkline` | Inline SVG sparkline from an array meta field |
| `table` | HTML table from an array-of-objects meta field |
| `list` | Vertical list from an array meta field |
| `count-badge` | Badge showing the count of items in an array meta field |
| `embed` | Inline embed of a resource by ID (image, video, audio, iframe) |
| `image` | Image display from a meta field containing a URL or resource ID |
| `barcode` | Code 128 barcode SVG from a meta field value |
| `qr-code` | QR code SVG from a meta field value or literal string |
| `link-preview` | Card with title, URL, and optional description from a meta field |
| `json-tree` | Collapsible JSON tree view of a meta field |
| `bar-chart` | Horizontal bar chart from an object or array meta field |
| `pie-chart` | SVG pie chart from an object or array meta field |
| `conditional` | Show or hide content based on a meta field value (if/then/else) |
| `timeline-chart` | Horizontal timeline from an array of date-range objects |

Usage: `[plugin:data-views:badge path="status"]`

## meta-editors

Inline editing shortcodes for entity metadata fields. Each shortcode renders an Alpine.js component that saves changes via the `editMeta` API endpoint. Changes persist immediately without a full page reload.

| Shortcode | Description |
|-----------|-------------|
| `slider` | Range slider with min/max/step |
| `stepper` | Increment/decrement numeric input |
| `star-rating` | Clickable star rating (1-N) |
| `toggle` | Boolean on/off switch |
| `multi-select` | Checkbox group for selecting multiple values from a list |
| `button-group` | Single-select button row |
| `color-picker` | Color input with hex value |
| `tags-input` | Free-form tag chips with add/remove |
| `textarea` | Multi-line text editor |
| `date-picker` | Date input |
| `date-range` | Start and end date inputs |
| `status-badge` | Clickable badge that cycles through defined statuses |
| `progress-input` | Editable progress bar (0-100) |
| `key-value` | Add/edit/remove key-value pairs |
| `checklist` | Checkbox list with add/remove |
| `url-input` | URL input with validation and clickable link |
| `markdown` | Markdown text editor with preview |

Usage: `[plugin:meta-editors:slider path="rating" min=0 max=10 step=1]`

## widgets

Dashboard-style shortcodes for category custom templates. These query owned entities to build summaries, galleries, and hierarchy views.

| Shortcode | Description |
|-----------|-------------|
| `summary` | Entity count dashboard (owned resources, notes, and sub-groups) |
| `gallery` | Thumbnail grid of owned image resources with lightbox |
| `progress` | Progress bar driven by a meta field value |
| `activity` | Timeline of recently updated owned entities |
| `tree` | Group hierarchy visualization (ancestors and children) |

Usage: `[plugin:widgets:summary]`

## Enabling a Plugin

Via the UI:

1. Navigate to the plugin management page
2. Click **Enable** on the plugin

Via the API:

```bash
curl -X POST http://localhost:8181/v1/plugin/enable -d "name=data-views"
```

Via the CLI:

```bash
mr plugin enable data-views
```

## Plugin Documentation Pages

After enabling a plugin, its documentation page shows all registered shortcodes with descriptions, parameters, and live previews using example data. Access it from the plugin management page by clicking the plugin name.
