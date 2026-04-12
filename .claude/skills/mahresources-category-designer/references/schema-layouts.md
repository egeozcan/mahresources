# Schema, Layouts, and Slots

This file is the canonical reference for building Mahresources schemas and category-driven layouts from a data model.

## Code Paths That Define Current Behavior

If you need to verify behavior in code, start here:

- `server/template_handlers/template_filters/shortcode_tag.go`
- `templates/displayGroup.tpl`
- `templates/displayResource.tpl`
- `templates/displayNote.tpl`
- `templates/partials/group.tpl`
- `templates/partials/resource.tpl`
- `templates/partials/note.tpl`
- `models/section_config.go`
- `src/schema-editor/*`

Important mismatch with older prose docs:

- Current stored category templates are not re-rendered as nested Pongo2 templates.
- Treat them as raw HTML plus shortcode expansion.
- Use Alpine `entity` bindings only where the surrounding template provides `x-data`.

## Category-Like Definitions

| Definition | Governs | MetaSchema validates | Custom field family |
| --- | --- | --- | --- |
| `Category` | Groups | `group.Meta` | `group.Category.Custom*` |
| `ResourceCategory` | Resources | `resource.Meta` | `resource.ResourceCategory.Custom*` |
| `NoteType` | Notes | `note.Meta` | `note.NoteType.Custom*` |

## Custom Template Slots

### Category custom fields

These are stored on the category/type record itself:

- `CustomHeader`
- `CustomSidebar`
- `CustomSummary`
- `CustomAvatar`
- `CustomMRQLResult`

### Current render wiring

| Definition | `CustomHeader` | `CustomSidebar` | `CustomSummary` | `CustomAvatar` | `CustomMRQLResult` |
| --- | --- | --- | --- | --- | --- |
| `Category` | group detail page body top | group detail sidebar | group list cards | group cards (with fallback to initials avatar) | used by `[mrql]` |
| `ResourceCategory` | resource detail page body top | resource detail sidebar | resource list cards | resource cards | used by `[mrql]` |
| `NoteType` | note detail page body top | note detail sidebar | note list cards | note cards | used by `[mrql]` |

### Alpine `entity` availability

For the standard rendered custom fields:

- `CustomHeader` -- yes
- `CustomSidebar` -- yes
- `CustomSummary` -- yes
- `CustomAvatar` -- yes (all three entity types)
- `CustomMRQLResult` -- no automatic Alpine `entity` wrapper

Therefore:

- In `CustomHeader`, `CustomSidebar`, `CustomSummary`, and rendered avatars, you can write Alpine-aware markup such as:

```html
<div class="text-sm">
  <span x-text="entity.Name"></span>
</div>
```

- In `CustomMRQLResult`, do not assume `entity` exists. Use shortcodes instead:

```html
<div class="rounded border p-2">
  <strong>[property path="Name"]</strong>
  <span class="text-sm text-stone-500">[meta path="status" hide-empty=true]</span>
</div>
```

## Plugin Injection Slots

These are separate from category custom fields. They are for plugins using `mah.inject(...)`.

Current template slot names:

- `head`
- `page_top`
- `sidebar_top`
- `sidebar_bottom`
- `page_bottom`
- `scripts`
- `group_list_before`
- `group_list_after`
- `resource_list_before`
- `resource_list_after`
- `note_list_before`
- `note_list_after`
- `group_detail_before`
- `group_detail_after`
- `group_detail_sidebar`
- `resource_detail_before`
- `resource_detail_after`
- `resource_detail_sidebar`
- `note_detail_before`
- `note_detail_after`
- `note_detail_sidebar`

Do not confuse these plugin slots with `CustomHeader` or `CustomSidebar`. Category authors fill custom fields. Plugin authors inject into named slots.

## SectionConfig

Use `SectionConfig` to hide native sections that your custom layout already covers.

Defaults:

- missing boolean keys default to `true`
- missing collapsible states default to `"default"`

### Group `SectionConfig`

```json
{
  "ownEntities": {
    "state": "default",
    "ownNotes": true,
    "ownGroups": true,
    "ownResources": true
  },
  "relatedEntities": {
    "state": "default",
    "relatedGroups": true,
    "relatedResources": true,
    "relatedNotes": true
  },
  "relations": {
    "state": "default",
    "forwardRelations": true,
    "reverseRelations": true
  },
  "tags": true,
  "timestamps": true,
  "metaJson": true,
  "merge": true,
  "clone": true,
  "treeLink": true,
  "owner": true,
  "breadcrumb": true,
  "description": true,
  "metaSchemaDisplay": true
}
```

### Resource `SectionConfig`

```json
{
  "technicalDetails": {
    "state": "default"
  },
  "metadataGrid": true,
  "timestamps": true,
  "notes": true,
  "groups": true,
  "series": true,
  "similarResources": true,
  "versions": true,
  "tags": true,
  "metaJson": true,
  "previewImage": true,
  "imageOperations": true,
  "categoryLink": true,
  "fileSize": true,
  "owner": true,
  "breadcrumb": true,
  "description": true,
  "metaSchemaDisplay": true
}
```

### Note `SectionConfig`

```json
{
  "content": true,
  "groups": true,
  "resources": true,
  "timestamps": true,
  "tags": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "owner": true,
  "noteTypeLink": true,
  "share": true
}
```

Use cases:

- hide `metaJson` when the raw JSON sidebar adds noise
- hide `metaSchemaDisplay` only if you are intentionally hand-rendering everything
- hide `description` when the custom header fully replaces it
- hide resource `metadataGrid` if your resource header and schema display make it redundant
- hide note `share` only when public sharing and sidebar actions are not relevant

## Schema Features That Work Well

Prefer this supported subset unless the user needs something more advanced:

| Feature | Good for | Notes |
| --- | --- | --- |
| `type` | all fields | use plain scalar/object/array types |
| `title`, `description` | labels and help text | strongly recommended |
| `required` | mandatory fields | keep focused |
| `enum` | simple fixed choices | fine for raw values |
| `oneOf` + `const` + `title` | labeled enums | best for statuses and categories |
| `format` | typed strings | `date`, `date-time`, `time`, `email`, `uri`, `uuid`, `hostname`, `ipv4`, `ipv6`, `regex`, `json-pointer` are surfaced in the editor |
| string constraints | validation | `minLength`, `maxLength`, `pattern`, `default`, `const` |
| number constraints | validation | `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`, `default`, `const` |
| array constraints | lists | `items`, `minItems`, `maxItems`, `uniqueItems`, `contains`, `prefixItems` |
| object constraints | shape control | `properties`, `required`, `additionalProperties`, `minProperties`, `maxProperties`, `patternProperties` |
| `$defs` + `$ref` | reuse | supported and resolved by the UI |
| `allOf`, `oneOf`, `anyOf` | composition | supported, but keep readable |
| `if` / `then` / `else` | conditional branches | supported; be deliberate about stale-key cleanup |
| `x-display` | display control | Mahresources-specific and very useful |

## `x-display`

Use `x-display` when an object should render as one widget instead of being flattened into multiple rows.

Built-in values:

| Value | Effect |
| --- | --- |
| `url` | force URL renderer |
| `geo` | force geo renderer |
| `daterange` | force date range renderer |
| `dimensions` | force dimensions renderer |
| `raw` | disable shape detection and show key/value grid |
| `none` | same practical effect as `raw` |

Automatic shape detection also exists for these object shapes:

- URL-like objects with `href` and `host` or `hostname`
- geo objects with `latitude` and `longitude` or `lat` and `lng`
- date-range objects with `start` and `end`
- dimensions objects with `width` and `height`

Guidance:

- If an object already matches a built-in shape, you often do not need `x-display`.
- Use `x-display: "raw"` when an object happens to look URL-like or date-range-like but you want the plain flattened/grid view.
- Built-in plugins do not currently ship `mah.display_type(...)` renderers, so `plugin:...` `x-display` values are mainly for future or custom plugins.

## Data Model -> Schema Mapping

Use this mapping when translating a plain-language model:

| Data model shape | Schema pattern | Layout hint |
| --- | --- | --- |
| free text | `type: "string"` | use schema display or `[meta]` |
| short labeled state | labeled enum | use `[meta]` or `data-views:badge` |
| integer score | `type: "integer"` with bounds | use `[meta]`, `meter`, or `progress-input` |
| money | `type: "number"` | render with `data-views:format type="currency"` |
| yes/no | `type: "boolean"` | render with `[meta]` or `meta-editors:toggle` |
| URL | `type: "string", format: "uri"` or URL-shaped object | use schema display or `link-preview` |
| date or datetime | string + `format` | let schema display format it |
| repeated tags/labels | array of strings | use schema display pills or `data-views:list` |
| repeated records | array of objects | use schema display or `data-views:table` |
| nested location | object with `lat`/`lng` or `latitude`/`longitude` | use `x-display: "geo"` if needed |
| date span | object with `start` and `end` | use auto shape detection or `x-display: "daterange"` |
| flexible extras | object with `additionalProperties: true` | keep to a clearly named `extras`, `custom`, or `metadata` branch |

## Schema Authoring Heuristics

Recommended defaults:

- Start with:

```json
{
  "type": "object",
  "properties": {},
  "required": [],
  "additionalProperties": false
}
```

- Add `title` to almost every top-level field.
- Add `description` to fields that users might interpret differently than you do.
- Use a labeled enum for any field the UI will present as a badge, phase, status, priority, or mode.
- Use `additionalProperties: false` on nested objects that represent a stable sub-model.
- Keep truly unstructured spillover under one explicit property, not across the whole root object.

Conditional schema guidance:

- Use `if` / `then` / `else` when one discriminator governs a few dependent fields.
- Use `oneOf` / `anyOf` when the entire object has variant shapes.
- For nested objects that differ between branches, set `additionalProperties: false` on those nested object schemas if you want branch switching to strip stale keys cleanly.

## Layout Composition Heuristics

### `CustomHeader`

Put here:

- title-adjacent status
- 2-5 critical facts
- hero metrics
- quick identity hints

Good tools:

- `[property path="Name"]`
- `[meta path="status"]`
- `data-views:badge`
- `data-views:stat-card`
- `data-views:format`
- small `[mrql]` lists

### `CustomSidebar`

Put here:

- supporting metrics
- inline editors
- recent related items
- charts
- admin-only context

Good tools:

- `meta-editors:*`
- `data-views:count-badge`
- `data-views:bar-chart`
- `data-views:pie-chart`
- `data-views:json-tree`
- `[mrql]`

### `CustomSummary`

Put here:

- compact status
- one-line KPI
- 1-3 tiny badges

Avoid:

- long tables
- many interactive controls
- bulky charts

### `CustomAvatar`

Put here:

- small emblem
- tiny badge
- concise visual identifier

Avoid:

- anything wider than the card avatar area
- complex editors

### `CustomMRQLResult`

Use when an entity should render differently inside `[mrql]` results than on default cards.

Remember:

- no automatic Alpine `entity`
- use static HTML plus shortcodes
- keep it card-sized unless the user explicitly wants a richer result format

## Recommended Full Package Output

When the user wants a complete definition, produce:

```text
Definition type: Category | ResourceCategory | NoteType
Plugins required: [...]

MetaSchema:
{ ... }

SectionConfig:
{ ... }

CustomHeader:
`...`

CustomSidebar:
`...`

CustomSummary:
`...`

CustomAvatar:
`...`

CustomMRQLResult:
`...`
```

Omit fields that are intentionally blank.

## Validation Checklist

- Do the `MetaSchema` paths match every `[meta path="..."]` you used?
- Does the chosen entity type match the intended managed entity?
- Are the plugin shortcodes available from enabled built-in plugins?
- Are you depending on Alpine `entity` only in slots that actually provide it?
- Did you avoid promising group `CustomAvatar` behavior without template changes?
- Did `SectionConfig` hide redundant native sections?
- Is the schema readable enough that another agent can safely maintain it later?
