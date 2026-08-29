---
sidebar_position: 15
title: Built-in Plugins
---

# Built-in Plugins

Mahresources ships with six plugins in the `plugins/` directory. They are not enabled by default. Enable them from the plugin management page or via the API.

The data-views, meta-editors, and widgets plugins register shortcodes for use in a category's custom template slots (see [Custom Templates](./custom-templates.md)) and entity descriptions. Full interactive documentation with live previews is available on each plugin's documentation page after enabling.

All six ship an `api_version = 1` manifest, and enabling one is your consent to exactly the capabilities it declares. See [Plugin Permissions](./plugin-permissions.md) for what each capability grants.

| Plugin | Declared capabilities | Network |
|--------|-----------------------|---------|
| data-views | `db:read`, `inject`, `render` | none |
| meta-editors | `inject`, `render` | none |
| widgets | `db:read`, `render` | none |
| example-blocks | `render` | none |
| example-plugin | `hooks`, `inject`, `pages` | none |
| fal-ai | `db:read`, `db:write`, `http`, `image`, `actions`, `jobs`, `pages` | the `fal.run`, `fal.ai` and `fal.media` host families |

fal-ai is the only one that reaches the network.

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

AI-powered image processing using [fal.ai](https://fal.ai). Requires a FAL.AI API key configured in plugin settings. The plugin registers seven resource actions (available from the resource detail view, and some from resource cards in list views) plus a **Generate Image** page.

Supported input formats: PNG, JPEG, WebP, GIF, TIFF, BMP. SVG inputs are filtered out; **Vectorize** produces SVG output from a raster source.

### Actions

| Action | Placement | Description |
|--------|-----------|-------------|
| `colorize` | detail, card | Colorize black and white images (DDColor or Topaz Colorize) |
| `adjust` | detail, card | Correct exposure/lighting or white balance (Topaz Adjust) |
| `upscale` | detail, card | Increase resolution -- choose from several upscaling models |
| `restore` | detail, card | Restore and enhance old or damaged photos -- several restoration models |
| `edit` (AI Edit) | detail | Edit an image from a text prompt; supports multiple input images |
| `vectorize` | detail, card | Convert a raster image to an SVG (always creates a new resource) |
| `polish` | detail | Sharpening finishing pass (post-processing), typically run after a restore |

### Model options per action

Several actions expose a `model` selector that switches the underlying fal.ai endpoint. Each model has its own parameters, which appear only when that model is selected (see [Conditional parameters](#conditional-parameters) below).

- **`colorize` models:** `ddcolor` (default, fast photographic colorization) and `topaz_colorize` (Topaz Labs' current professional colorization pass).
- **`adjust` models:** `adjust_v2` (default: exposure and lighting) and `white_balance` (color-cast correction). Both preserve source resolution.
- **`upscale` models:** `clarity` (default), `crystal`, `esrgan`, `creative`, `seedvr`, `bria_creative`, `topaz` (Topaz Precision), `topaz_generative`, `topaz_creative`, `topaz_transparent`, `drct`, `aura_sr`. The August 2026 Topaz family is split by intent: faithful precision, missing-detail reconstruction, creative Bloom enhancement, or fixed 4x alpha-preserving PNG. `drct` and `aura_sr` remain degradation-aware choices for JPEG-compressed sources; `seedvr` offers a seamless-tiling endpoint for repeating textures.
- **`restore` models:** `photo_restoration` (default), `codeformer`, `swin2sr`, `nafnet_denoise`, `nafnet_deblur`, `topaz_restore`, and `topaz_denoise`. Topaz Restore offers Recover 3 and Dust-Scratch V2; Topaz Denoise ranges from Normal through generative Denoise Max. The older `photo_restoration` endpoint is still the combined color/scratch fix, but always reshapes to a fixed 4K aspect-ratio enum.
- **`edit` (AI Edit) models:** `flux2` (default), `flux2pro`, `nanobanana2`, `nanobananapro`, `nanobanana_lite`, `gptimage2`, `seedream5`, `grok2`, `muse`, `fibo15`, and `flux1dev`. Meta Muse focuses on precise coherent multi-reference edits; Bria Fibo Edit 1.5 is trained for commercially safe composites; Nano Banana Lite favors low latency. Each model reveals only its own live-schema controls, including seed, thinking, web search, system prompt, background, prompt expansion, and safety switches where supported.
- **`polish` models:** `post_processing` (default: basic, smart, or CAS sharpening) and `topaz_sharpen` (blur-specific Topaz presets plus generative Super Focus).

### Conditional parameters

Action parameters use `show_when` conditions, so the form reveals only the inputs relevant to the current selection. Every fal.ai action parameter carries concise inline help, and every model selector reveals a short strengths/tradeoffs card before its controls. Nested controls are also conditional: for example, Topaz Redefine reveals prompt/creativity/texture fields that other Topaz Generative presets do not use.

### Output mode

Every action except `vectorize` includes a **Save Result As** toggle:

- `version` (default) -- adds the result as a new version of the source resource.
- `clone` -- creates a new resource, copying the source's name (with an action suffix), description, owner, metadata, tags, groups, and notes.

`vectorize` always clones, since its SVG output cannot be a version of a raster source.

### Multiple input images

The `edit` (AI Edit) action accepts more than one input image through an `extra_images` entity-reference parameter (a resource picker, up to nine images). It defaults to the triggering resource, and the user can add or remove images. Every model except `flux1dev` consumes the extra images; `flux1dev` takes a single image. Per-model ceilings are enforced with explicit errors where fal.ai would truncate or reject opaquely: `grok2` accepts three; `flux2` and `fibo15` accept four. `seedream5` and `muse` accept ten, while `gptimage2` accepts sixteen (more than the picker allows).

### Generate Image page

The plugin also adds a **Generate Image** page (`/plugins/fal-ai/generate`, linked from the plugin menu) for text-to-image generation. It runs as an asynchronous job and supports `nanobanana2` (default), `nanobananapro`, `nanobanana_lite`, `gptimage2`, `seedream5`, `grok2`, `muse`, and `fibo15`. The withdrawn Imagen 4 preview endpoints were removed after their live schemas and queue routes began returning 404. Generated images are saved as new resources.

The page renders a documented union of prompt, sizing, format, seed, quality/background, safety, Nano Banana advanced controls, and Bria style preset. Each model receives only the fields its own schema declares:

| Setting | How each model receives it |
|---------|----------------------------|
| Resolution | Snapped to the nearest native tier. Nano Banana Pro has no 0.5K; Grok stops at 2K; Lite, GPT Image 2, Seedream, and Muse auto-size; Fibo maps the union to 1MP or 4MP. |
| Aspect ratio | Passed through where supported. GPT Image 2 and Seedream receive the closest `image_size` preset instead. |
| Safety tolerance | Sent to the Nano Banana family. Seedream maps levels 1-2 to its boolean checker; models without a tolerance field ignore it. |
| Format | Sent where supported. Seedream maps WebP to JPEG, while Fibo uses its endpoint-native result format. |
| Advanced controls | Seed, GPT/Grok quality, GPT background, Nano Banana thinking/system prompt/web search, and Fibo style preset are each sent only to models that declare them. |

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
