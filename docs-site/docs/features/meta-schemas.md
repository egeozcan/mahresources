---
sidebar_position: 5
title: Meta Schemas
---

# Meta Schemas

Categories, Resource Categories, and Note Types can define a JSON Schema in their `metaSchema` field. The schema validates the `meta` field of entities in that category and drives structured form generation in the UI.

## How It Works

1. An administrator creates a Category, Resource Category, or Note Type with a `metaSchema` field containing a JSON Schema document
2. When creating or editing a Group (or Resource) in that Category, the UI renders form fields matching the schema instead of free-form key-value inputs
3. The schema validates metadata on save

## Which Entity Types Support It

| Entity Type | MetaSchema Field | Validates |
|-------------|-----------------|-----------|
| Category | `metaSchema` | Group `meta` fields |
| Resource Category | `metaSchema` | Resource `meta` fields |
| Note Type | `metaSchema` | Note `meta` fields |

All three entity types support MetaSchema. When set, the UI renders structured form fields and a schema-aware metadata panel on the entity detail page.

## Schema Format

The `metaSchema` field stores a standard JSON Schema document as a string:

```json
{
  "type": "object",
  "properties": {
    "email": {
      "type": "string",
      "format": "email",
      "description": "Primary email address"
    },
    "phone": {
      "type": "string",
      "pattern": "^\\+?[0-9-]+$"
    },
    "birthday": {
      "type": "string",
      "format": "date"
    }
  },
  "required": ["email"]
}
```

## Common Schema Patterns

### Contact Information (Person Category)

```json
{
  "type": "object",
  "properties": {
    "email": {"type": "string", "format": "email"},
    "phone": {"type": "string"},
    "birthday": {"type": "string", "format": "date"},
    "social": {
      "type": "object",
      "properties": {
        "twitter": {"type": "string"},
        "linkedin": {"type": "string"}
      }
    }
  },
  "required": ["email"]
}
```

### Project Tracking (Project Category)

```json
{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["planning", "active", "on-hold", "completed"]
    },
    "deadline": {"type": "string", "format": "date"},
    "budget": {"type": "number"},
    "lead": {"type": "string"}
  }
}
```

### Receipt Classification (Receipt Resource Category)

```json
{
  "type": "object",
  "properties": {
    "vendor": {"type": "string"},
    "amount": {"type": "number"},
    "currency": {
      "type": "string",
      "enum": ["USD", "EUR", "GBP"]
    },
    "date": {"type": "string", "format": "date"},
    "category": {
      "type": "string",
      "enum": ["office", "travel", "software", "other"]
    }
  },
  "required": ["vendor", "amount"]
}
```

## Setting a Schema

### Via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit a Category
3. Enter the JSON Schema in the **Meta Schema** field, or click **Visual Editor** to build the schema interactively
4. Save

### Via the API

```bash
curl -X POST http://localhost:8181/v1/category \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "Person",
    "Description": "Individual contacts",
    "MetaSchema": "{\"type\":\"object\",\"properties\":{\"email\":{\"type\":\"string\",\"format\":\"email\"}},\"required\":[\"email\"]}"
  }'
```

## Form Generation

When a Category has a schema defined, the Group, Resource, or Note create/edit form replaces free-form metadata inputs with structured fields:

- `string` properties render as text inputs
- `string` with `format: "email"` renders as an email input
- `string` with `format: "date"` renders as a date picker
- `string` with `enum` renders as a dropdown select
- `number` properties render as numeric inputs
- `integer` properties render as numeric inputs (whole numbers)
- `boolean` properties render as checkboxes
- `array` properties render as repeatable field groups
- `object` properties render as nested fieldsets
- `required` fields are marked as mandatory

The form component also supports `$ref` for reusable schema definitions, `oneOf`/`anyOf`/`allOf` for schema composition, `if/then/else` for conditional fields, and `additionalProperties` for free-form key-value editing within an object.

## Visual Schema Editor

Instead of writing JSON Schema by hand, you can use the visual editor to build schemas interactively.

### Opening the Editor

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit a Category
3. Click the **Visual Editor** button next to the Meta Schema field
4. The editor opens in a modal with three tabs

### Editor Tabs

**Edit Schema** — The visual builder with a tree view on the left and a property editor on the right. Click nodes in the tree to edit their type, constraints, and metadata. Use the "+ Property" button to add new fields.

![Schema Editor Modal](/img/schema-editor-modal.png)

**Preview Form** — Shows a live preview of the form that will be generated from your schema. This is exactly what users will see when creating or editing entities in this category.

![Schema Editor Preview](/img/schema-editor-preview.png)

**Raw JSON** — The full JSON Schema as editable text. Changes here sync with the visual editor. Use this for advanced schemas that the visual editor doesn't fully support.

### Building a Schema

1. Click **+ Property** in the tree toolbar
2. Select the new property in the tree
3. Set its name, type, and constraints in the detail panel
4. Check **Required** if the field is mandatory
5. For enum fields: choose "string" type, then add enum values in the Enum Values section
6. For nested objects: choose "object" type, then add child properties
7. Click **Preview Form** to verify the form looks right
8. Click **Apply Schema** to save

### Composition Keywords

The editor supports `oneOf`, `anyOf`, `allOf`, and `$ref` for advanced schema patterns:

- Use `$defs` to define reusable schema fragments
- Use `$ref` to reference definitions
- Use `oneOf`/`anyOf` for variant types (e.g., a "contact" field that can be email or phone)

![Schema Composition](/img/schema-editor-composition.png)

## Detail View Display

When a Category has a schema defined and a Group (or Resource) has metadata, the detail page renders a structured metadata panel just below the description. This replaces the need to scroll to the sidebar JSON table.

### Smart Hybrid Layout

Fields are displayed in a responsive grid layout:

- **Short values** (strings, numbers, booleans, enums) appear in a multi-column grid
- **Long values** (text exceeding ~80 characters, arrays) appear in full-width rows below the grid
- **Empty/null fields** are hidden by default, with a "Show N hidden fields" toggle at the bottom

### Type-Aware Rendering

Each field renders according to its schema type:

| Schema Type | Display |
|-------------|---------|
| `string` | Plain text |
| `string` with `format: "email"` | Clickable `mailto:` link |
| `string` with `format: "uri"` | Clickable link |
| `string` with `format: "date"` | Formatted date |
| `string` with `enum` | Colored pill/badge |
| `string` with labeled enum (`oneOf` + `const` + `title`) | Pill showing the human-readable title |
| `integer` / `number` | Monospace text |
| `boolean` | "Yes" / "No" |
| `array` of scalars | Inline pills |
| Nested `object` | Flattened with dot notation (e.g., "Address > City") |

### Labels and Tooltips

- If a property has a `title` field, it is used as the display label (falling back to the raw key name)
- If a property has a `description` field, it appears as a tooltip on hover over the label
- Clicking a label copies the dot-notation meta path (e.g., `address.city`)
- Clicking a value copies the value to the clipboard

### Built-in Shape Detection

Object values are automatically detected and rendered as smart widgets when they match well-known shapes:

| Shape | Detection Keys | Display |
|-------|---------------|---------|
| **URL / Location** | `href` + `host` or `hostname` | Clickable link with host subtitle |
| **GeoLocation** | `latitude` + `longitude` (or `lat` + `lng`) | Coordinates with OpenStreetMap link |
| **Date Range** | `start` + `end` (both valid dates) | Formatted range "Mar 15, 2024 — Apr 1, 2024" |
| **Dimensions** | `width` + `height` (both numbers) | "1920 x 1080" |

Shape detection runs automatically. To override it, use the `x-display` annotation (see below).

### The `x-display` Schema Annotation

Add `x-display` to any schema property to control how it renders in the detail view:

```json
{
  "type": "object",
  "properties": {
    "location": {
      "type": "object",
      "x-display": "geo",
      "properties": { "lat": { "type": "number" }, "lng": { "type": "number" } }
    },
    "raw_data": {
      "type": "object",
      "x-display": "raw",
      "properties": { "href": { "type": "string" }, "host": { "type": "string" } }
    }
  }
}
```

**Built-in values:**

| Value | Effect |
|-------|--------|
| `"url"` | Force URL renderer |
| `"geo"` | Force GeoLocation renderer |
| `"daterange"` | Force Date Range renderer |
| `"dimensions"` | Force Dimensions renderer |
| `"raw"` or `"none"` | Opt out of shape detection; show as key-value grid |

**Plugin values:** `x-display` values starting with `plugin:` route to a plugin's custom renderer. See [Plugin Lua API](./plugin-lua-api.md#mahdisplay_type----custom-display-renderers) for details.

```json
{
  "images": {
    "type": "object",
    "x-display": "plugin:my-plugin:image-grid"
  }
}
```

When `x-display` is set on an object property, the object is NOT flattened into individual fields. Instead, the entire object is passed to the renderer as a whole.

### Sidebar JSON Table

The raw JSON table in the sidebar remains available for power users and for fields not covered by the schema. The structured metadata panel does not replace it.

## Search Integration

When a Category has a schema defined, the list page search form automatically renders typed filter fields based on the schema properties.

![Schema Search Fields](/img/schema-search-fields.png)

- **String fields** render as text inputs with a LIKE operator by default
- **Number fields** render as numeric inputs with comparison operators (`=`, `≠`, `>`, `≥`, `<`, `≤`)
- **Enum fields** render as checkboxes (≤6 values) or multi-select dropdowns (>6 values)
- **Boolean fields** render as three-state radio buttons (Any / Yes / No)

When multiple categories are selected, only fields common to all selected categories are shown. Fields that exist in some but not all categories are hidden.

Schema-driven filter fields appear alongside the existing free-form metadata filters. The free-form filters are automatically adjusted to exclude fields already covered by the schema filters.

### Known Limitations

**Mixed-type enums with identical string representations** — Enum schemas that contain values of different JSON types which stringify the same way (e.g., `enum: [1, "1"]` or `enum: [null, "null"]`) cannot be distinguished in the search UI. HTML form controls carry string values only, so selecting "1" from such an enum will always submit the non-string variant (numeric `1`). Avoid mixing types that collide when converted to strings; use a single consistent type per enum instead.

**Variant scoring does not penalize extra properties** — When the form renders `oneOf`/`anyOf` variants, it picks the best-matching branch by scoring discriminator fields (`const`, `enum`) and key overlap. Extra properties beyond what a variant declares do not reduce its score, even if the variant has `additionalProperties: false`. This is intentional: the scoring function selects the correct *variant*, not validates data. Extra keys are handled separately by `stripStaleKeys` on schema switch and by server-side validation.

**Conditional branch data cleanup with `additionalProperties: true`** — When a schema uses `if/then/else` and the user switches between branches, data from the inactive branch is automatically removed for fields declared exclusively in that branch. For nested objects under shared keys, stale nested keys are only cleaned up when the nested schema sets `additionalProperties: false`. If both branches share an object property whose schema allows additional properties (the default), keys introduced by one branch will persist after switching to the other. To ensure clean branch transitions for nested objects, set `additionalProperties: false` on nested object schemas that differ between branches.

## Free-Form Metadata

Groups without a Category (or with a Category that has no schema) display a free-form metadata editor. This editor renders dynamic key-value fields where you can add, remove, and edit metadata entries. Each field has a key name and a value. The editor handles type coercion for numeric, boolean, null, and date values automatically.

When a schema defines `additionalProperties`, the form includes a free-form key-value section below the structured fields. This lets users add metadata beyond what the schema specifies.

The free-form editor can also load field suggestions from a remote URL, providing autocomplete for key names based on existing metadata patterns in the database.
