---
sidebar_position: 4
---

# Custom Templates

Categories (for Groups), Note Types (for Notes), and Resource Categories (for Resources) support custom HTML templates that create specialized views for different content types.

:::warning Security Notice

Custom templates execute arbitrary HTML and JavaScript. Only use them on **trusted network deployments**.

Do not allow untrusted users to create or edit Categories, Resource Categories, and Note Types with custom templates, as they could inject malicious scripts.

:::

## Template Locations

Each Category (for Groups), Resource Category (for Resources), and Note Type (for Notes) can define four custom templates:

| Template | Display Location |
|----------|-----------------|
| **CustomHeader** | Top of the entity detail page (body area) |
| **CustomSidebar** | Sidebar of the entity detail page |
| **CustomSummary** | Entity cards in list views |
| **CustomAvatar** | Avatar/icon when linking to the entity |

## How Custom Templates Are Rendered

Custom template content is processed in two ways:

- **Shortcodes** (`[meta]`, `[property]`, `[mrql]`, and plugin shortcodes) are expanded server-side.
- **Alpine.js directives** (`x-text`, `x-if`, `:class`, `@click`, etc.) work because the outer page template already wraps custom content in an `x-data` scope with the full entity available as `entity`.

:::caution Pongo2 expressions do not work

Custom template content is **not** evaluated as a Pongo2 template. Expressions like `{{ group.Name }}` or `{{ group|json }}` will appear as literal text in the rendered HTML. Use Alpine.js directives or shortcodes instead.

:::

## Accessing Entity Data

The entity is already available as `entity` in the Alpine.js scope. You do not need to add an `x-data` wrapper -- the outer template provides it.

```html
<!-- Your template content -- no x-data wrapper needed -->
<h2 x-text="entity.Name"></h2>
<p x-text="entity.Description"></p>
```

For Groups, the entity includes:
- `ID`, `Name`, `Description`, `URL`
- `CategoryId`, `OwnerId`
- `Meta` (JSON metadata object)
- `CreatedAt`, `UpdatedAt`

For Notes, the entity includes:
- `ID`, `Name`, `Description`
- `NoteTypeId`, `OwnerId`
- `Meta` (JSON metadata object)
- `StartDate`, `EndDate`
- `CreatedAt`, `UpdatedAt`

For Resources, the entity includes:
- `ID`, `Name`, `OriginalName`, `Description`
- `ResourceCategoryId`, `OwnerId`
- `Meta` (JSON metadata object)
- `Hash`, `ContentType`, `FileSize`, `Width`, `Height`
- `CreatedAt`, `UpdatedAt`

## Basic Examples

### Display Metadata Fields

If groups in a "Person" category have metadata like `{"birthDate": "1990-01-15", "occupation": "Engineer"}`:

```html
<dl class="grid grid-cols-2 gap-2">
  <dt class="font-medium">Birth Date</dt>
  <dd x-text="entity.Meta?.birthDate || 'Unknown'"></dd>

  <dt class="font-medium">Occupation</dt>
  <dd x-text="entity.Meta?.occupation || 'Unknown'"></dd>
</dl>
```

### Conditional Display

Show different content based on metadata:

```html
<template x-if="entity.Meta?.status === 'active'">
  <span class="px-2 py-1 bg-green-100 text-green-800 rounded">Active</span>
</template>
<template x-if="entity.Meta?.status === 'archived'">
  <span class="px-2 py-1 bg-gray-100 text-gray-600 rounded">Archived</span>
</template>
```

### Link to Related Content

Create links using entity data:

```html
<template x-if="entity.Meta?.website">
  <a :href="entity.Meta.website"
     class="text-blue-600 hover:underline"
     target="_blank">
    Visit Website
  </a>
</template>

<template x-if="entity.Meta?.relatedGroupId">
  <a :href="'/group?id=' + entity.Meta.relatedGroupId"
     class="text-blue-600 hover:underline">
    View Related Group
  </a>
</template>
```

## Advanced Examples

### Iterating Over Metadata Arrays

Use `x-for` to render lists, tables, or grids from array metadata. This pattern works for image galleries, badge lists, data tables, and any repeating content.

```html
<template x-if="entity.Meta?.records && entity.Meta.records.length > 0">
  <table class="w-full text-sm">
    <thead>
      <tr class="border-b">
        <th class="text-left py-2">Date</th>
        <th class="text-left py-2">Event</th>
        <th class="text-right py-2">Value</th>
      </tr>
    </thead>
    <tbody>
      <template x-for="record in entity.Meta.records" :key="record.date">
        <tr class="border-b">
          <td class="py-2" x-text="record.date"></td>
          <td class="py-2" x-text="record.event"></td>
          <td class="py-2 text-right" x-text="record.value"></td>
        </tr>
      </template>
    </tbody>
  </table>
</template>
```

### Dynamic Styling from Metadata

Combine `:class` bindings with metadata values for status badges, progress bars, or conditional formatting.

```html
<template x-if="entity.Meta?.progress !== undefined">
  <div class="mt-4">
    <div class="flex justify-between text-sm mb-1">
      <span>Progress</span>
      <span x-text="entity.Meta.progress + '%'"></span>
    </div>
    <div class="w-full bg-gray-200 rounded-full h-2">
      <div class="bg-blue-600 h-2 rounded-full"
           :style="'width: ' + entity.Meta.progress + '%'"></div>
    </div>
  </div>
</template>
```

## CustomSummary Example

The CustomSummary template appears in list views. Keep it compact:

```html
<div class="text-sm text-gray-600">
  <template x-if="entity.Meta?.status">
    <span class="inline-block px-2 py-0.5 text-xs rounded"
          :class="{
            'bg-green-100 text-green-800': entity.Meta.status === 'active',
            'bg-yellow-100 text-yellow-800': entity.Meta.status === 'pending',
            'bg-gray-100 text-gray-600': entity.Meta.status === 'archived'
          }"
          x-text="entity.Meta.status"></span>
  </template>
  <template x-if="entity.Meta?.priority">
    <span class="ml-2" x-text="'Priority: ' + entity.Meta.priority"></span>
  </template>
</div>
```

## CustomAvatar Example

The CustomAvatar template controls how the entity appears when linked:

```html
<template x-if="entity.Meta?.avatarUrl">
  <img :src="entity.Meta.avatarUrl"
       class="w-8 h-8 rounded-full object-cover">
</template>
<template x-if="!entity.Meta?.avatarUrl">
  <div class="w-8 h-8 rounded-full bg-gray-300 flex items-center justify-center">
    <span class="text-xs font-medium text-gray-600"
          x-text="entity.Name?.charAt(0) || '?'"></span>
  </div>
</template>
```

## Creating Categories with Templates

1. Navigate to **Categories**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields:
   - **Custom Header** - HTML for the detail page header
   - **Custom Sidebar** - HTML for the detail page sidebar
   - **Custom Summary** - HTML for list view cards
   - **Custom Avatar** - HTML for link avatars
5. Click **Submit**

## Creating Resource Categories with Templates

1. Navigate to **Resource Categories**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields:
   - **Custom Header** - HTML for the resource detail page header
   - **Custom Sidebar** - HTML for the resource detail page sidebar
   - **Custom Summary** - HTML for list view cards
   - **Custom Avatar** - HTML for link avatars
5. Optionally define a **MetaSchema** (JSON Schema for metadata validation)
6. Click **Submit**

## Creating Note Types with Templates

1. Navigate to **Note Types**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields
5. Click **Submit**

## Shortcodes

Shortcodes let you embed dynamic content in custom templates without writing Alpine.js code. Three built-in shortcodes are available:

- **`[meta]`** -- Schema-aware metadata display with optional inline editing
- **`[property]`** -- Entity field values (Name, CreatedAt, etc.)
- **`[mrql]`** -- Inline MRQL query results in various formats

Plugins can also register custom shortcodes via `mah.shortcode()`.

See the [Shortcodes](./shortcodes.md) page for full syntax, attributes, and examples.

## Section Configuration

Categories, Resource Categories, and Note Types can define a `sectionConfig` JSON field that controls which sections appear on entity detail pages.

### How It Works

When a category has a `sectionConfig` set, the detail page for entities in that category shows or hides sections accordingly. Any section not mentioned in the config defaults to visible. An empty config (or no config) shows all sections.

### Setting via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit an entry
3. Use the **Section Visibility** form to toggle sections on/off
4. Save

### JSON Format

The `sectionConfig` is a JSON object. Each key corresponds to a section on the detail page. Boolean keys default to `true` (visible). Object keys support a `state` field with collapsible behavior.

**Collapsible states:**

| State | Behavior |
|-------|----------|
| `"default"` | Follows the application default |
| `"open"` | Initially expanded |
| `"collapsed"` | Initially collapsed |
| `"off"` | Hidden entirely |

### Group Sections (via Category)

```json
{
  "tags": true,
  "timestamps": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "description": true,
  "merge": true,
  "clone": true,
  "treeLink": true,
  "owner": true,
  "breadcrumb": true,
  "ownEntities": {
    "state": "default",
    "ownNotes": true,
    "ownGroups": true,
    "ownResources": true
  },
  "relatedEntities": {
    "state": "default",
    "relatedNotes": true,
    "relatedGroups": true,
    "relatedResources": true
  },
  "relations": {
    "state": "default",
    "forwardRelations": true,
    "reverseRelations": true
  }
}
```

### Resource Sections (via Resource Category)

```json
{
  "metadataGrid": true,
  "timestamps": true,
  "notes": true,
  "groups": true,
  "tags": true,
  "versions": true,
  "similarResources": true,
  "series": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "description": true,
  "previewImage": true,
  "imageOperations": true,
  "categoryLink": true,
  "fileSize": true,
  "owner": true,
  "breadcrumb": true,
  "technicalDetails": {
    "state": "default"
  }
}
```

### Note Sections (via Note Type)

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

To hide a section, set its key to `false`. For example, to hide timestamps and the raw JSON sidebar on notes:

```json
{
  "timestamps": false,
  "metaJson": false
}
```

## Custom MRQL Result Templates

Categories, Resource Categories, and Note Types can define a `customMRQLResult` field containing a shortcode template that controls how entities of that type render in `[mrql]` shortcode results. The template is processed by the shortcode engine (not Pongo2), so `[meta]`, `[property]`, and nested `[mrql]` shortcodes work, but `{{ }}` expressions do not.

### How It Works

1. Set the `customMRQLResult` field on a Category, Resource Category, or Note Type
2. When an `[mrql]` shortcode query returns entities of that type, the custom template is used instead of the default card layout -- unless the `[mrql]` shortcode itself provides a [block template](./shortcodes.md#block-syntax), which takes precedence over all category-level templates
3. The template has access to the entity context, so shortcodes like `[meta]` and `[property]` work inside it

### Setting via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit an entry
3. Enter a template in the **Custom MRQL Result** textarea
4. Save

### Example

A Category with this `customMRQLResult`:

```html
<div class="flex items-center gap-2 p-2 border rounded">
  <strong>[property path="Name"]</strong>
  <span class="text-sm text-stone-500">[meta path="status"]</span>
</div>
```

When an `[mrql]` query returns groups in this category, each result renders using this template instead of the default link card.

### Format and Template Precedence

Template selection follows this priority:

1. **Block template** -- if the `[mrql]` shortcode uses block syntax with non-empty content, that block body is the per-item template. `customMRQLResult` and `format` are both ignored.
2. **Explicit `format`** -- `format="table"`, `format="list"`, or `format="compact"` override custom template rendering.
3. **`customMRQLResult`** -- when `format` is empty (auto) or `"custom"`, entities with a `customMRQLResult` use it; entities without one fall back to the default card layout.
4. **Default card layout** -- used when none of the above apply.

## Styling Tips

### Use Tailwind CSS

Tailwind CSS is included. Use utility classes for styling:

```html
<div class="p-4 bg-gray-50 rounded-lg shadow-sm">
  <h3 class="text-lg font-semibold text-gray-900">Title</h3>
  <p class="mt-2 text-gray-600">Description text</p>
</div>
```

### Responsive Design

Use Tailwind responsive prefixes:

```html
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
  <!-- Content adapts to screen size -->
</div>
```

## Nested Alpine.js Scopes

If you need additional reactive state (toggles, counters, etc.), create a nested `x-data` scope. The parent `entity` variable remains accessible:

```html
<div x-data="{ showDetails: false }">
  <button @click="showDetails = !showDetails" class="text-sm text-blue-600">
    Toggle Details
  </button>
  <div x-show="showDetails" class="mt-2">
    <p x-text="entity.Meta?.notes || 'No notes'"></p>
  </div>
</div>
```

## Debugging Templates

If a template does not render correctly:

1. Check the browser console for JavaScript errors
2. Verify the entity JSON is valid (view page source)
3. Test with a minimal template first, then add complexity
4. Use `x-text` to debug values: `<span x-text="JSON.stringify(entity.Meta)"></span>`

## MetaSchema for Validation

Categories, Resource Categories, and Note Types support a **MetaSchema** field -- a JSON Schema that validates metadata. This is separate from templates but works well together:

1. Define a MetaSchema to ensure required fields exist
2. Create templates that rely on those fields
3. Users get validation errors if metadata is incomplete

Example MetaSchema:

```json
{
  "type": "object",
  "required": ["status", "priority"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["active", "pending", "archived"]
    },
    "priority": {
      "type": "integer",
      "minimum": 1,
      "maximum": 5
    }
  }
}
```

## Related Pages

- [Meta Schemas](./meta-schemas.md) -- JSON Schema validation for entity metadata
- [Custom Block Types](./custom-block-types.md) -- structured content blocks within Notes
- [Entity Picker](./entity-picker.md) -- modal for selecting entities in block and form contexts
