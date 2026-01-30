# Generic Entity Picker Design

## Overview

Refactor the resource picker modal into a generic entity picker that can be configured for any entity type (resources, groups, notes, tags, etc.). Initial implementation supports resources and groups, with the group picker replacing the comma-separated ID input in the references block.

## Goals

- Single reusable picker component for any entity type
- Configuration-driven behavior (no entity-specific logic in the picker itself)
- Maintain existing resource picker UX while adding group picker
- Design for future extensibility to other entity types

## Architecture

### Entity Configuration Object

Each entity type is defined by a configuration object:

```javascript
{
  entityType: 'resource' | 'group',
  entityLabel: 'Resources' | 'Groups',

  // API
  searchEndpoint: '/v1/resources' | '/v1/groups',
  searchParams: (query, filters) => URLSearchParams,

  // Filtering
  filters: [
    { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true },
    { key: 'category', label: 'Category', endpoint: '/v1/categories', multi: false }
  ],

  // Display
  renderItem: 'thumbnail' | 'groupCard',
  getItemId: (item) => number,
  getItemLabel: (item) => string,

  // Optional features
  tabs: null | [{ id, label, loadFn }],
  gridColumns: 'grid-cols-3 md:grid-cols-4 lg:grid-cols-5'
}
```

### Configuration Registry

Configurations live in a central registry for easy extension:

**Resource configuration** (migrated from existing):
```javascript
resourcePickerConfig: {
  entityType: 'resource',
  entityLabel: 'Resources',
  searchEndpoint: '/v1/resources',
  searchParams: (query, filters) => {
    const params = new URLSearchParams({ MaxResults: '50' });
    if (query) params.set('name', query);
    filters.tags?.forEach(id => params.append('Tags', id));
    if (filters.group) params.set('Groups', filters.group);
    return params;
  },
  filters: [
    { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true },
    { key: 'group', label: 'Group', endpoint: '/v1/groups', multi: false }
  ],
  tabs: [
    { id: 'note', label: "Note's Resources", loadFn: (noteId) => `/v1/resources?ownerId=${noteId}` },
    { id: 'all', label: 'All Resources', loadFn: null }
  ],
  renderItem: 'thumbnail',
  gridColumns: 'grid-cols-3 md:grid-cols-4 lg:grid-cols-5'
}
```

**Group configuration** (new):
```javascript
groupPickerConfig: {
  entityType: 'group',
  entityLabel: 'Groups',
  searchEndpoint: '/v1/groups',
  searchParams: (query, filters) => {
    const params = new URLSearchParams({ MaxResults: '50' });
    if (query) params.set('name', query);
    if (filters.category) params.set('categoryId', filters.category);
    return params;
  },
  filters: [
    { key: 'category', label: 'Category', endpoint: '/v1/categories', multi: false }
  ],
  tabs: null,
  renderItem: 'groupCard',
  gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4'
}
```

### Alpine Store

Single `entityPicker` store replaces `resourcePicker`:

```javascript
Alpine.store('entityPicker', {
  // Configuration
  config: null,

  // UI state
  isOpen: false,
  activeTab: null,
  loading: false,

  // Search state
  searchQuery: '',
  filters: {},
  results: [],

  // Selection state
  selectedIds: new Set(),
  existingIds: new Set(),

  // Callback
  onConfirm: null,

  // Methods
  open({ entityType, noteId, existingIds, onConfirm }) { ... },
  close() { ... },
  search() { ... },
  toggleSelection(id) { ... },
  confirm() { ... }
});
```

## Modal UI Structure

```
┌──────────────────────────────────────────────────────┐
│  Select {EntityLabel}                            [X] │
├──────────────────────────────────────────────────────┤
│  [Search input........................] [Filter ▼]   │
│                                                      │
│  [Tab 1] [Tab 2]  (if tabs configured)              │
├──────────────────────────────────────────────────────┤
│                                                      │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐                   │
│  │Item │ │Item │ │Item │ │Item │                   │
│  └─────┘ └─────┘ └─────┘ └─────┘                   │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐                   │
│  │Item │ │Item │ │Item │ │Item │                   │
│  └─────┘ └─────┘ └─────┘ └─────┘                   │
│                                                      │
├──────────────────────────────────────────────────────┤
│  {n} selected                    [Cancel] [Confirm]  │
└──────────────────────────────────────────────────────┘
```

- Title uses `config.entityLabel`
- Filter dropdowns generated from `config.filters` array
- Tabs rendered only if `config.tabs` is defined
- Grid columns use `config.gridColumns` class
- Item rendering dispatches to renderer based on `config.renderItem`

## Group Card Rendering

Compact card version of the group list view:

```
┌─────────────────────────────────┐
│ [Thumbnail]  Group Name         │
│              Parent > Child     │
│              3 resources, 2 notes│
└─────────────────────────────────┘
```

**Components:**
- **Thumbnail area** (left, ~60px): Group's main resource thumbnail if set, otherwise folder icon
- **Content area** (right):
  - Group name (bold, truncated if long)
  - Breadcrumb path in muted text (e.g., "Projects > 2024 > Archive")
  - Metadata line showing resource/note counts

**Breadcrumb generation:**
Traverse parent chain via `Owner` field. If path > 3 levels, show "... > Grandparent > Parent".

**Selection states:**
- Default: White background, subtle border
- Hover: Light gray background
- Selected: Blue border, checkmark overlay
- Already added: Grayed out with "Added" badge, not selectable

## Block Integration

### Gallery Block (unchanged UX)

```javascript
// blockGallery.js
openPicker() {
  Alpine.store('entityPicker').open({
    entityType: 'resource',
    noteId: this.noteId,
    existingIds: this.resourceIds,
    onConfirm: (selectedIds) => {
      this.addResources(selectedIds);
    }
  });
}
```

### References Block (new picker UX)

```javascript
// blockReferences.js
openPicker() {
  Alpine.store('entityPicker').open({
    entityType: 'group',
    existingIds: this.groupIds,
    onConfirm: (selectedIds) => {
      this.addGroups(selectedIds);
    }
  });
}
```

**Updated UI:**
- View mode: Pills showing group name + breadcrumb (fetched via metadata)
- Edit mode: Same pills with remove buttons + "Select Groups" button

### Shared Metadata Utility

```javascript
// src/components/picker/entityMeta.js
async function fetchEntityMeta(entityType, ids) {
  // Returns { [id]: metadata } for display
  // Resource: { contentType, name, hash }
  // Group: { name, breadcrumb, resourceCount, noteCount }
}
```

## File Structure

### New Files

```
src/components/picker/
  ├── entityPicker.js      # Alpine store
  ├── entityConfigs.js     # Configuration registry
  └── entityMeta.js        # Shared metadata fetching

templates/partials/
  └── entityPicker.tpl     # Generic modal template
```

### Modified Files

```
src/main.js
  - Remove resourcePickerStore import
  - Add entityPicker store import

src/components/blocks/blockGallery.js
  - Change openPicker() to use entityPicker
  - Use shared entityMeta

src/components/blocks/blockReferences.js
  - Remove text input logic
  - Add openPicker(), groupMeta state
  - Add fetchGroupMeta()

templates/partials/blockEditor.tpl
  - Update references block (pills + button)
  - Replace resource picker modal with entityPicker include
```

### Deleted Files

```
src/components/blocks/resourcePicker.js  # Replaced by entityPicker
```

## Error Handling

**Loading states:**
- Spinner while fetching search results
- Skeleton cards while loading metadata
- Graceful fallback if metadata fetch fails (show ID)

**Empty states:**
- "No results found" with prompt to adjust filters

**API errors:**
- Toast notification for network failures
- Retry button if initial load fails

**Edge cases:**
- Deleted entities: Show "Unknown" pill with remove option
- Large selections: Confirm button shows count ("Add 25 groups")
- Rapid filter changes: Abort previous request

## Accessibility

- `role="dialog"` and `aria-modal="true"` on modal
- Focus trap within modal
- Keyboard: Tab through items, Enter to toggle, Escape to close
- ARIA live regions for result count updates
- Tab list: `role="tablist"`, tabs with `aria-selected`
- Grid: `role="listbox"`, items: `role="option"` with `aria-selected`

## Documentation

### User Documentation (docs-site/)

Update existing user docs:
- How to use the group picker in references blocks
- Screenshots and step-by-step guide
- Note that resource picker works the same way

### Developer Documentation (docs-site/)

New developer guide:
- How to add a new entity type to the picker
- Configuration object structure and required fields
- The `entityConfigs.js` registry pattern
- How blocks integrate with the picker store

## E2E Tests

### New Test File: `e2e/tests/entity-picker.spec.ts`

- Open resource picker from gallery block
- Search resources, apply filters, select items, confirm
- Verify selected resources appear in gallery
- Open group picker from references block
- Search groups, filter by category, select items, confirm
- Verify selected groups appear as pills
- Test "already added" items are not selectable
- Test cancel discards selection
- Test empty state when no results
- Test keyboard navigation

### Updated Tests

- `notes.spec.ts` - Update tests interacting with gallery/references blocks

### Accessibility Tests

- `e2e/tests/accessibility/entity-picker.a11y.spec.ts`
- Verify modal ARIA attributes
- Test keyboard navigation
- Test focus management
