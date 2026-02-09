---
sidebar_position: 4
---

# Custom Templates

Categories (for Groups), Note Types (for Notes), and Resource Categories (for Resources) support custom HTML templates that let you create specialized views for different types of content. Templates can display entity data dynamically and include interactive elements using Alpine.js.

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

## Accessing Entity Data

Templates have access to the entity data through Alpine.js. The entity is available as a JavaScript object:

```html
<div x-data="{ entity: {{ group|json }} }">
  <!-- Your template here -->
  <h2 x-text="entity.Name"></h2>
  <p x-text="entity.Description"></p>
</div>
```

For Groups, the entity includes:
- `ID`, `Name`, `Description`
- `CategoryId`, `OwnerId`
- `Meta` (JSON metadata object)
- `CreatedAt`, `UpdatedAt`

For Notes, the entity includes:
- `ID`, `Name`, `Description`
- `NoteTypeId`, `OwnerId`
- `Meta` (JSON metadata object)
- `StartDate`, `EndDate`
- `CreatedAt`, `UpdatedAt`

## Basic Examples

### Display Metadata Fields

If groups in a "Person" category have metadata like `{"birthDate": "1990-01-15", "occupation": "Engineer"}`:

```html
<div x-data="{ entity: {{ group|json }} }">
  <dl class="grid grid-cols-2 gap-2">
    <dt class="font-medium">Birth Date</dt>
    <dd x-text="entity.Meta?.birthDate || 'Unknown'"></dd>

    <dt class="font-medium">Occupation</dt>
    <dd x-text="entity.Meta?.occupation || 'Unknown'"></dd>
  </dl>
</div>
```

### Conditional Display

Show different content based on metadata:

```html
<div x-data="{ entity: {{ group|json }} }">
  <template x-if="entity.Meta?.status === 'active'">
    <span class="px-2 py-1 bg-green-100 text-green-800 rounded">Active</span>
  </template>
  <template x-if="entity.Meta?.status === 'archived'">
    <span class="px-2 py-1 bg-gray-100 text-gray-600 rounded">Archived</span>
  </template>
</div>
```

### Link to Related Content

Create links using entity data:

```html
<div x-data="{ entity: {{ group|json }} }">
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
</div>
```

## Advanced Examples

### Image Gallery from Metadata

Display images stored in metadata:

```html
<div x-data="{ entity: {{ group|json }} }">
  <template x-if="entity.Meta?.images && entity.Meta.images.length > 0">
    <div class="grid grid-cols-3 gap-2">
      <template x-for="img in entity.Meta.images" :key="img">
        <img :src="img" class="w-full h-32 object-cover rounded">
      </template>
    </div>
  </template>
</div>
```

### Progress Bar

Display a progress indicator:

```html
<div x-data="{ entity: {{ group|json }} }">
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
</div>
```

### Custom Data Table

Render a table from array metadata:

```html
<div x-data="{ entity: {{ group|json }} }">
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
</div>
```

### Styled Badge List

Display tags or categories from metadata:

```html
<div x-data="{ entity: {{ group|json }} }">
  <template x-if="entity.Meta?.labels && entity.Meta.labels.length > 0">
    <div class="flex flex-wrap gap-2 mt-2">
      <template x-for="label in entity.Meta.labels" :key="label">
        <span class="px-2 py-1 text-xs bg-indigo-100 text-indigo-800 rounded-full"
              x-text="label"></span>
      </template>
    </div>
  </template>
</div>
```

## CustomSummary Example

The CustomSummary template appears in list views. Keep it compact:

```html
<div x-data="{ entity: {{ group|json }} }" class="text-sm text-gray-600">
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
<div x-data="{ entity: {{ group|json }} }">
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
</div>
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

## Styling Tips

### Use Tailwind CSS

Mahresources includes Tailwind CSS. Use Tailwind utility classes for styling:

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

### Dark Mode Compatibility

If you plan to add dark mode support later, use semantic color classes:

```html
<div class="bg-white text-gray-900">
  <!-- Later can be: bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 -->
</div>
```

## Debugging Templates

If a template does not render correctly:

1. Check the browser console for JavaScript errors
2. Verify the entity JSON is valid (view page source)
3. Test with a minimal template first, then add complexity
4. Use `x-text` to debug values: `<span x-text="JSON.stringify(entity.Meta)"></span>`

## MetaSchema for Validation

Categories also support a **MetaSchema** field - a JSON Schema that validates the metadata of groups in that category. This is separate from templates but works well together:

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
