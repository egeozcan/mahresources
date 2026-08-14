---
sidebar_position: 15
title: Built-in Plugins
---

# Built-in Plugins

Mahresources ships with six plugins in the `plugins/` directory. They are not enabled by default. Enable them from the plugin management page or via the API.

The data-views, meta-editors, and widgets plugins register shortcodes for use in custom template fields (CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, CustomMRQLResult) and entity descriptions. Full interactive documentation with live previews is available on each plugin's documentation page after enabling.

## data-views

Data visualization shortcodes for rendering metadata values, charts, and tables. The chart shortcodes render as pure HTML/CSS or SVG with no JavaScript. Two shortcodes are exceptions: `qr-code` renders client-side via an injected JavaScript encoder, and `json-tree` uses Alpine.js directives for its collapse/expand behavior.

| Shortcode | Description |
|-----------|-------------|
| `badge` | Colored status badge from a meta field value |
| `format` | Formatted value display (currency, percent, date, filesize, number, duration) |
| `stat-card` | Card with label, value, and optional icon |
| `meter` | Horizontal gauge bar with min/max/value |
| `sparkline` | Inline SVG sparkline from an array meta field |
| `table` | Table of entities owned by the group, or of an MRQL query result (columns via `cols`/`labels`) |
| `list` | Vertical list from an array meta field |
| `count-badge` | Badge showing the count of items in an array meta field |
| `embed` | Text content of a resource (by ID or path), base64-decoded into a scrollable code block |
| `image` | Image display from a meta field containing a URL or resource ID |
| `barcode` | Code 128 barcode SVG from a meta field value |
| `qr-code` | QR code SVG from a meta field value or literal string |
| `link-preview` | Card linking to a URL, showing the URL and its domain from a meta field |
| `json-tree` | Collapsible JSON tree view of a meta field |
| `bar-chart` | Horizontal bar chart from an object or array meta field |
| `pie-chart` | SVG pie chart from an object or array meta field |
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
| `markdown` | Markdown text editor (monospace textarea, debounced auto-save, no rendered preview) |

Usage: `[plugin:meta-editors:slider path="rating" min=0 max=10 step=1]`

## widgets

Dashboard-style shortcodes for category custom templates. These query owned entities to build summaries, galleries, and hierarchy views.

| Shortcode | Description |
|-----------|-------------|
| `summary` | Entity count dashboard (owned resources, notes, and sub-groups) |
| `gallery` | Thumbnail grid of owned image resources with lightbox (for a group that owns no images, falls back to its group-related resources) |
| `progress` | Progress bar driven by a meta field value |
| `activity` | Timeline of recently updated owned entities |
| `tree` | Group hierarchy visualization (ancestors and children) |

Usage: `[plugin:widgets:summary]`

## example-blocks

Demonstrates custom plugin block types for the note block editor via `mah.block_type()`.

| Block Type | Description |
|------------|-------------|
| `counter` | A click counter block with label editing and +1 increment |

Usage: Enable the plugin, then add a "Counter" block in the note block editor.

## example-plugin

Reference implementation demonstrating the plugin API: injections, hooks, pages, menus, settings, and the database/HTTP/KV APIs. Most API calls are commented out to serve as copy-paste examples.

| Feature | Description |
|---------|-------------|
| Page injection | Footer banner controlled by a boolean setting |
| Hooks | Logs note and resource creation events |
| Custom page | `/plugins/example-plugin/info` displays the greeting setting |
| Menu item | "Plugin Info" links to the custom page |

## fal-ai

AI-powered image processing using [fal.ai](https://fal.ai). Requires a FAL.AI API key configured in plugin settings. The plugin registers six resource actions (available from the resource detail view, and some from resource cards in list views) plus a **Generate Image** page.

Supported input formats: PNG, JPEG, WebP, GIF, TIFF, BMP. The UI filter that decides which resources surface the actions also includes `image/svg+xml`, so the actions appear on SVG resources even though SVG is not a supported input format and the action fails at runtime.

### Actions

| Action | Placement | Description |
|--------|-----------|-------------|
| `colorize` | detail, card | Colorize black and white images (DDColor) |
| `upscale` | detail, card | Increase resolution -- choose from several upscaling models |
| `restore` | detail, card | Restore and enhance old or damaged photos -- several restoration models |
| `edit` (AI Edit) | detail | Edit an image from a text prompt; supports multiple input images |
| `vectorize` | detail, card | Convert a raster image to an SVG (always creates a new resource) |
| `polish` | detail | Sharpening finishing pass (post-processing), typically run after a restore |

### Model options per action

Several actions expose a `model` selector that switches the underlying fal.ai endpoint. Each model has its own parameters, which appear only when that model is selected (see [Conditional parameters](#conditional-parameters) below).

- **`upscale` models:** `clarity` (default), `crystal`, `esrgan`, `creative`, `seedvr`, `bria_creative`, `topaz`, `drct`, `aura_sr`. `drct` and `aura_sr` are degradation-aware and handle JPEG-compressed sources better than pure super-resolution models. `crystal` is Clarity AI's newer upscaler, tuned for facial detail and portrait photography; its `creativity` parameter is `0` by default, which is a faithful upscale. `seedvr` has a **Seamless Tiling** toggle that switches to the seamless endpoint, for textures and repeating patterns.
- **`restore` models:** `photo_restoration` (default -- the only one that removes scratches and fixes color fading in one pass), `codeformer` (face-focused), `swin2sr` (non-portrait scenes), `nafnet_denoise` (compression/ISO artifacts), `nafnet_deblur` (motion blur). The `photo_restoration` model always reshapes output to a fixed aspect-ratio enum; its `aspect_ratio` parameter defaults to `auto`, which picks the enum closest to the source's actual dimensions.
- **`edit` (AI Edit) models:** `flux2` (default), `flux2pro`, `nanobanana2`, `nanobananapro`, `gptimage2`, `seedream5`, `grok2`, `flux1dev`. Their schemas differ in more than naming, and the exposed parameters follow each one: `gptimage2` and `seedream5` size their output with an `image_size` enum instead of an aspect ratio, `seedream5` takes a boolean safety checker rather than a numeric tolerance, and `gptimage2` and `grok2` have no safety parameter at all (both expose a `quality` dial instead).

### Conditional parameters

Action parameters use `show_when` conditions, so the form reveals only the inputs relevant to the current selection. For example, choosing the `topaz` upscale model surfaces Topaz-specific controls (model preset, upscale factor, subject detection, face enhancement, output format), while the Clarity controls stay hidden. The `restore` and `polish` actions behave the same way for their model and sharpen-mode selectors.

### Output mode

Every action except `vectorize` includes a **Save Result As** toggle:

- `version` (default) -- adds the result as a new version of the source resource.
- `clone` -- creates a new resource, copying the source's name (with an action suffix), description, owner, metadata, tags, groups, and notes.

`vectorize` always clones, since its SVG output cannot be a version of a raster source.

### Multiple input images

The `edit` (AI Edit) action accepts more than one input image through an `extra_images` entity-reference parameter (a resource picker, up to nine images). It defaults to the triggering resource, and the user can add or remove images. Every model except `flux1dev` consumes the extra images; `flux1dev` takes a single image. The picker's nine-image maximum is one number shared by every model, so per-model limits are applied when the request is built: `grok2` accepts three and the action fails with a message naming the count if it is given more, `seedream5` uses the last ten of whatever it is sent, and `gptimage2` accepts sixteen (more than the picker allows).

### Generate Image page

The plugin also adds a **Generate Image** page (`/plugins/fal-ai/generate`, linked from the plugin menu) for text-to-image generation. It runs as an asynchronous job and supports the `nanobanana2` (default), `nanobananapro`, `gptimage2`, `seedream5`, `grok2`, `imagen4`, `imagen4_fast`, and `imagen4_ultra` models. Generated images are saved as new resources.

The page renders a single form -- prompt, model, resolution, aspect ratio, safety tolerance -- for every model, but the endpoints behind it do not share one input schema, so each model receives only the fields its own schema declares:

| Setting | How each model receives it |
|---------|----------------------------|
| Resolution | Snapped to the nearest value the model accepts. `nanobananapro` has no `0.5K`; `imagen4`, `imagen4_ultra` and `grok2` stop at `2K` (`grok2` spells them lowercase); `imagen4_fast`, `gptimage2` and `seedream5` have no resolution field at all. |
| Aspect ratio | Passed through where the model has an `aspect_ratio` field. `gptimage2` and `seedream5` do not, so it maps to their `image_size` enum (`3:2` and `2:3` take the closest landscape/portrait size). `imagen4*` supports only `1:1`, `16:9`, `9:16`, `4:3` and `3:4`, so `3:2` and `2:3` snap to `4:3` and `3:4`. |
| Safety tolerance | Sent as `safety_tolerance` to `nanobanana2`, `nanobananapro` and `imagen4*`. `seedream5` has a boolean safety checker instead, enabled only for the two strictest settings. `gptimage2` and `grok2` have no safety parameter and ignore it. |

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

After enabling a plugin, its documentation page shows all registered shortcodes with descriptions, parameters, and live previews using example data. Access it from the plugin management page by clicking the **View documentation** link shown under the plugin (present only when the plugin ships docs).
