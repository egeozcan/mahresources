---
sidebar_position: 7
---

# Entity Picker

The entity picker is a reusable modal component for selecting entities in the block editor and other UI contexts. It ships with configurations for resources, groups, and notes. The shipped page UI opens only the resource and group pickers directly, but a plugin action's `entity_ref` param can open the picker for any of resource, note, or group. New entity types (e.g., tags, categories) can be added through configuration objects without modifying core logic.

## Using the Picker

### From Block Components

Open the picker from any block component using the Alpine store:

```javascript
openPicker() {
  Alpine.store('entityPicker').open({
    entityType: 'resource',      // 'resource' or 'group'
    noteId: this.noteId,         // optional, for resource context
    existingIds: this.selectedIds,
    onConfirm: (selectedIds) => {
      this.handleSelection(selectedIds);
    }
  });
}
```

### Configuration Options

| Option | Type | Description |
|--------|------|-------------|
| `entityType` | string | Required. Entity type key: `'resource'`, `'group'`, or `'note'` |
| `noteId` | number | Optional. Note ID for "Note's Resources" tab |
| `existingIds` | number[] | IDs already selected (shown as "Added") |
| `lockedFilters` | object | Optional. Filter values forced on every search (passed into `searchParams`), not user-editable. Defaults to `{}` |
| `multiSelect` | boolean | Optional. Whether more than one entity can be selected at once. Defaults to `true` |
| `onConfirm` | function | Callback receiving array of selected IDs |

### Multi-Selection

The picker supports selecting multiple entities at once. Click items to toggle their selection, then confirm the entire selection with the confirm button.

## Adding New Entity Types

The picker is designed to be extensible. To add support for a new entity type (e.g., notes, tags):

### 1. Add Configuration

Edit `src/components/picker/entityConfigs.js`. A `note` configuration (with a `noteCard` render mode) already ships in this file as a complete worked example. No shipped page UI opens the note picker directly, though a plugin action's `entity_ref` param can select `entityType: 'note'` (as well as `'resource'` or `'group'`). Use it as a reference for the shape of a config object:

```javascript
export const entityConfigs = {
  // existing configs...

  note: {
    entityType: 'note',
    entityLabel: 'Notes',
    searchEndpoint: '/v1/notes',
    maxResults: 50,
    searchParams: (query, filters, lockedFilters = {}, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.tags) filters.tags.forEach(id => params.append('Tags', id));
      if (lockedFilters.note_type_ids) {
        lockedFilters.note_type_ids.forEach(id => params.append('NoteTypeIds', id));
      }
      return params;
    },
    filters: [
      { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true }
    ],
    tabs: null,
    renderItem: 'noteCard',
    gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Note ${item.ID}`
  }
};
```

The `searchParams` function takes four arguments in order: `query`, `filters`, `lockedFilters`, and `maxResults`.

### 2. Add Render Mode (if needed)

If your entity needs a custom card display, add a new render mode to `templates/partials/entityPicker.tpl`:

```html
{# Results grid - Note cards #}
<div x-show="... && $store.entityPicker.config?.renderItem === 'noteCard'"
     ...>
  <template x-for="item in $store.entityPicker.displayResults" ...>
    <!-- Custom card markup -->
  </template>
</div>
```

### 3. Add Metadata Fetcher (optional)

If blocks need to display entity metadata, add a fetcher to `src/components/picker/entityMeta.js`:

```javascript
async function fetchNoteMeta(ids) {
  const meta = {};
  // ... fetch logic
  return meta;
}

// Add to fetchers map
const fetchers = {
  resource: fetchResourceMeta,
  group: fetchGroupMeta,
  note: fetchNoteMeta  // Add this
};
```

## Architecture

```
src/components/picker/
├── entityConfigs.js    # Entity type configurations
├── entityPicker.js     # Alpine store with picker logic
├── entityMeta.js       # Metadata fetching utilities
└── index.js            # Module exports

templates/partials/
└── entityPicker.tpl    # Modal template
```

The picker is entity-agnostic. All entity-specific behavior comes from configuration objects.

## Configuration Reference

### Entity Config Properties

| Property | Type | Description |
|----------|------|-------------|
| `entityType` | string | Unique identifier for this entity type |
| `entityLabel` | string | Display name for the modal title |
| `searchEndpoint` | string | API endpoint for searching entities |
| `maxResults` | number | Maximum results to fetch (default: 50) |
| `searchParams` | function | Builds URLSearchParams from `query`, `filters`, `lockedFilters`, and `maxResults` |
| `filters` | array | Filter definitions (see below) |
| `tabs` | array\|null | Tab definitions (null for no tabs) |
| `renderItem` | string | Render mode: `'thumbnail'`, `'groupCard'`, or `'noteCard'` |
| `gridColumns` | string | Tailwind grid classes for results layout |
| `getItemId` | function | Extracts ID from entity object |
| `getItemLabel` | function | Extracts display label from entity object |

### Filter Definition

```javascript
{
  key: 'tags',           // Key used in filterValues
  label: 'Tags',         // Display label
  endpoint: '/v1/tags',  // Autocomplete suggestions endpoint
  multi: true            // Allow multiple selections
}
```

### Tab Definition

```javascript
{
  id: 'note',                    // Tab identifier
  label: "Note's Resources"      // Display label
}
```

## Events

The picker dispatches a custom event when closed:

```javascript
window.dispatchEvent(new CustomEvent('entity-picker-closed'));
```

Filter autocompleters listen for this event to reset their state.

## Search Behavior

- **Debouncing**: Search input is debounced by 200ms to reduce API calls during typing
- **Request aborting**: Each new search cancels the previous in-flight request, preventing stale results from overwriting newer ones

## Metadata Caching

Entity metadata fetched for display in blocks is cached in memory to avoid redundant API requests. The cache has a 5-minute TTL and is automatically managed.

### Cache Behavior

- Metadata is cached per entity (keyed by `entityType:id`)
- Cache entries expire after 5 minutes
- Batched concurrency: maximum 5 concurrent metadata requests per batch
- Failed requests retry up to 2 times with 500ms linear backoff
- 4xx errors (client errors) are not retried
- Cache is cleared on page reload

### Clearing the Cache

If you need to force-refresh metadata (e.g., after editing an entity):

```javascript
import { clearMetaCache } from './components/picker/index.js';

// Clear all cached metadata
clearMetaCache();

// Clear only group metadata
clearMetaCache('group');

// Clear only resource metadata
clearMetaCache('resource');
```
