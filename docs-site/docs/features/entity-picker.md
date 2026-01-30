---
sidebar_position: 7
---

# Entity Picker

The entity picker is a reusable modal component for selecting entities (resources, groups, etc.) in the block editor. It's designed to be extensible for additional entity types.

## Using the Picker

### From Block Components

Open the picker from any block component using the Alpine store:

```javascript
openPicker() {
  Alpine.store('entityPicker').open({
    entityType: 'resource',      // or 'group'
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
| `entityType` | string | Required. Entity type key (`'resource'` or `'group'`) |
| `noteId` | number | Optional. Note ID for "Note's Resources" tab |
| `existingIds` | number[] | IDs already selected (shown as "Added") |
| `onConfirm` | function | Callback receiving array of selected IDs |

## Adding New Entity Types

To add a new entity type (e.g., notes, tags):

### 1. Add Configuration

Edit `src/components/picker/entityConfigs.js`:

```javascript
export const entityConfigs = {
  // existing configs...

  note: {
    entityType: 'note',
    entityLabel: 'Notes',
    searchEndpoint: '/v1/notes',
    searchParams: (query, filters) => {
      const params = new URLSearchParams({ MaxResults: '50' });
      if (query) params.set('name', query);
      if (filters.noteType) params.set('noteTypeId', filters.noteType);
      return params;
    },
    filters: [
      { key: 'noteType', label: 'Note Type', endpoint: '/v1/noteTypes', multi: false }
    ],
    tabs: null,
    renderItem: 'noteCard',  // Add new render mode
    gridColumns: 'grid-cols-2 md:grid-cols-3',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name
  }
};
```

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

The picker is entity-agnostic. All entity-specific behavior comes from configuration objects, making it easy to extend without modifying core logic.

## Configuration Reference

### Entity Config Properties

| Property | Type | Description |
|----------|------|-------------|
| `entityType` | string | Unique identifier for this entity type |
| `entityLabel` | string | Display name for the modal title |
| `searchEndpoint` | string | API endpoint for searching entities |
| `maxResults` | number | Maximum results to fetch (default: 50) |
| `searchParams` | function | Builds URLSearchParams from query, filters, and maxResults |
| `filters` | array | Filter definitions (see below) |
| `tabs` | array\|null | Tab definitions (null for no tabs) |
| `renderItem` | string | Render mode: `'thumbnail'` or `'groupCard'` |
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

## Metadata Caching

Entity metadata fetched for display in blocks is cached in memory to avoid redundant API requests. The cache has a 5-minute TTL and is automatically managed.

### Cache Behavior

- Metadata is cached per entity (keyed by `entityType:id`)
- Cache entries expire after 5 minutes
- Failed requests retry up to 2 times with exponential backoff
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
