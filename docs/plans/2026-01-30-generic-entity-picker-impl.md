# Generic Entity Picker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the resource picker into a generic entity picker that supports resources and groups via configuration.

**Architecture:** Single Alpine store with entity configuration objects. Existing resource picker behavior preserved exactly. New group picker replaces comma-separated ID input in references block.

**Tech Stack:** Alpine.js, Pongo2 templates, Playwright E2E tests

---

## Task 1: Create Entity Configuration Registry

**Files:**
- Create: `src/components/picker/entityConfigs.js`

**Step 1: Create the configuration file**

```javascript
// src/components/picker/entityConfigs.js

export const entityConfigs = {
  resource: {
    entityType: 'resource',
    entityLabel: 'Resources',
    searchEndpoint: '/v1/resources',
    searchParams: (query, filters) => {
      const params = new URLSearchParams({ MaxResults: '50' });
      if (query) params.set('name', query);
      if (filters.tags) {
        filters.tags.forEach(id => params.append('Tags', id));
      }
      if (filters.group) params.set('Groups', filters.group);
      return params;
    },
    filters: [
      { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true },
      { key: 'group', label: 'Group', endpoint: '/v1/groups', multi: false }
    ],
    tabs: [
      { id: 'note', label: "Note's Resources" },
      { id: 'all', label: 'All Resources' }
    ],
    renderItem: 'thumbnail',
    gridColumns: 'grid-cols-3 sm:grid-cols-4 md:grid-cols-5',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Resource ${item.ID}`
  },

  group: {
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
    gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Group ${item.ID}`
  }
};

export function getEntityConfig(entityType) {
  const config = entityConfigs[entityType];
  if (!config) {
    throw new Error(`Unknown entity type: ${entityType}`);
  }
  return config;
}
```

**Step 2: Verify file exists**

Run: `ls -la src/components/picker/`
Expected: `entityConfigs.js` listed

**Step 3: Commit**

```bash
git add src/components/picker/entityConfigs.js
git commit -m "feat(picker): add entity configuration registry

Defines configurations for resource and group pickers.
Each config specifies API endpoints, filters, display options.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Create Generic Entity Picker Store

**Files:**
- Create: `src/components/picker/entityPicker.js`

**Step 1: Create the store file**

```javascript
// src/components/picker/entityPicker.js
import { abortableFetch } from '../../index.js';
import { getEntityConfig } from './entityConfigs.js';

export function registerEntityPickerStore(Alpine) {
  Alpine.store('entityPicker', {
    // Configuration
    config: null,

    // UI state
    isOpen: false,
    activeTab: null,
    loading: false,
    error: null,

    // Context
    noteId: null,

    // Search state
    searchQuery: '',
    filterValues: {},
    results: [],
    tabResults: {}, // { note: [], all: [] } for resource picker

    // Selection state
    selectedIds: new Set(),
    existingIds: new Set(),

    // Callback
    onConfirm: null,

    // Internal
    searchDebounceTimer: null,
    requestAborter: null,

    open({ entityType, noteId = null, existingIds = [], onConfirm }) {
      this.config = getEntityConfig(entityType);
      this.noteId = noteId;
      this.existingIds = new Set(existingIds);
      this.onConfirm = onConfirm;
      this.selectedIds = new Set();
      this.searchQuery = '';
      this.filterValues = {};
      this.error = null;
      this.results = [];
      this.tabResults = {};
      this.isOpen = true;

      // Set initial tab
      if (this.config.tabs) {
        // For resources: start on 'note' tab if noteId provided, else 'all'
        this.activeTab = noteId ? this.config.tabs[0].id : this.config.tabs[1]?.id || this.config.tabs[0].id;
        // Load tab-specific data
        if (this.activeTab === 'note' && noteId) {
          this.loadNoteResources();
        }
      } else {
        this.activeTab = null;
      }

      // Load main results
      this.loadResults();
    },

    close() {
      this.isOpen = false;
      this.results = [];
      this.tabResults = {};
      this.selectedIds = new Set();
      this.config = null;
      if (this.requestAborter) {
        this.requestAborter();
        this.requestAborter = null;
      }
      // Dispatch event for filter cleanup
      window.dispatchEvent(new CustomEvent('entity-picker-closed'));
    },

    confirm() {
      if (this.onConfirm && this.selectedIds.size > 0) {
        this.onConfirm([...this.selectedIds]);
      }
      this.close();
    },

    async loadNoteResources() {
      if (!this.noteId) return;

      try {
        const res = await fetch(`/v1/resources?ownerId=${this.noteId}&MaxResults=100`);
        if (!res.ok) throw new Error('Failed to load note resources');
        this.tabResults.note = await res.json();
        if (this.tabResults.note.length === 0 && this.activeTab === 'note') {
          this.activeTab = 'all';
        }
      } catch (err) {
        console.error('Error loading note resources:', err);
      }
    },

    async loadResults() {
      if (this.requestAborter) {
        this.requestAborter();
      }

      this.loading = true;
      this.error = null;

      const params = this.config.searchParams(this.searchQuery.trim(), this.filterValues);
      const url = `${this.config.searchEndpoint}?${params}`;

      const { abort, ready } = abortableFetch(url);
      this.requestAborter = abort;

      try {
        const res = await ready;
        if (!res.ok) throw new Error(`Failed to load ${this.config.entityLabel.toLowerCase()}`);
        const data = await res.json();
        this.results = data;
        if (this.config.tabs) {
          this.tabResults.all = data;
        }
      } catch (err) {
        if (err.name !== 'AbortError') {
          this.error = err.message || `Failed to load ${this.config.entityLabel.toLowerCase()}`;
          console.error('Error loading results:', err);
        }
      } finally {
        this.loading = false;
      }
    },

    onSearchInput() {
      if (this.searchDebounceTimer) {
        clearTimeout(this.searchDebounceTimer);
      }
      this.searchDebounceTimer = setTimeout(() => {
        this.loadResults();
      }, 200);
    },

    setFilter(key, value) {
      if (value === null || value === undefined) {
        delete this.filterValues[key];
      } else {
        this.filterValues[key] = value;
      }
      this.loadResults();
    },

    addToFilter(key, value) {
      if (!this.filterValues[key]) {
        this.filterValues[key] = [];
      }
      if (!this.filterValues[key].includes(value)) {
        this.filterValues[key] = [...this.filterValues[key], value];
        this.loadResults();
      }
    },

    removeFromFilter(key, value) {
      if (this.filterValues[key]) {
        this.filterValues[key] = this.filterValues[key].filter(v => v !== value);
        if (this.filterValues[key].length === 0) {
          delete this.filterValues[key];
        }
        this.loadResults();
      }
    },

    toggleSelection(itemId) {
      if (this.existingIds.has(itemId)) return;

      if (this.selectedIds.has(itemId)) {
        this.selectedIds.delete(itemId);
      } else {
        this.selectedIds.add(itemId);
      }
      // Trigger reactivity
      this.selectedIds = new Set(this.selectedIds);
    },

    isSelected(itemId) {
      return this.selectedIds.has(itemId);
    },

    isAlreadyAdded(itemId) {
      return this.existingIds.has(itemId);
    },

    setActiveTab(tabId) {
      this.activeTab = tabId;
    },

    get displayResults() {
      if (this.config?.tabs && this.activeTab === 'note') {
        return this.tabResults.note || [];
      }
      return this.results;
    },

    get hasTabResults() {
      return this.tabResults.note?.length > 0;
    },

    get selectionCount() {
      return this.selectedIds.size;
    }
  });
}
```

**Step 2: Verify file exists**

Run: `ls -la src/components/picker/`
Expected: `entityPicker.js` and `entityConfigs.js` listed

**Step 3: Commit**

```bash
git add src/components/picker/entityPicker.js
git commit -m "feat(picker): add generic entity picker store

Configuration-driven Alpine store that works with any entity type.
Supports tabs, filters, search, and multi-selection.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Create Shared Metadata Utility

**Files:**
- Create: `src/components/picker/entityMeta.js`

**Step 1: Create the utility file**

```javascript
// src/components/picker/entityMeta.js

/**
 * Fetch metadata for entities to display in blocks.
 * Returns an object keyed by ID with entity-specific metadata.
 */
export async function fetchEntityMeta(entityType, ids) {
  if (!ids || ids.length === 0) return {};

  const fetchers = {
    resource: fetchResourceMeta,
    group: fetchGroupMeta
  };

  const fetcher = fetchers[entityType];
  if (!fetcher) {
    console.warn(`No metadata fetcher for entity type: ${entityType}`);
    return {};
  }

  return fetcher(ids);
}

async function fetchResourceMeta(ids) {
  const meta = {};
  const toFetch = ids.filter(id => id != null);
  if (toFetch.length === 0) return meta;

  try {
    const promises = toFetch.map(id =>
      fetch(`/v1/resource?id=${id}`).then(r => r.ok ? r.json() : null)
    );
    const results = await Promise.all(promises);
    results.forEach((res, i) => {
      if (res) {
        meta[toFetch[i]] = {
          contentType: res.ContentType || '',
          name: res.Name || '',
          hash: res.Hash || ''
        };
      }
    });
  } catch (err) {
    console.warn('Failed to fetch resource metadata:', err);
  }

  return meta;
}

async function fetchGroupMeta(ids) {
  const meta = {};
  const toFetch = ids.filter(id => id != null);
  if (toFetch.length === 0) return meta;

  try {
    const promises = toFetch.map(id =>
      fetch(`/v1/group?id=${id}`).then(r => r.ok ? r.json() : null)
    );
    const results = await Promise.all(promises);
    results.forEach((res, i) => {
      if (res) {
        meta[toFetch[i]] = {
          name: res.Name || '',
          breadcrumb: buildBreadcrumb(res),
          resourceCount: res.ResourceCount || 0,
          noteCount: res.NoteCount || 0,
          mainResourceId: res.MainResource?.ID || null,
          categoryName: res.Category?.Name || ''
        };
      }
    });
  } catch (err) {
    console.warn('Failed to fetch group metadata:', err);
  }

  return meta;
}

function buildBreadcrumb(group) {
  const parts = [];
  let current = group.Owner;
  let depth = 0;
  const maxDepth = 3;

  while (current && depth < maxDepth) {
    parts.unshift(current.Name);
    current = current.Owner;
    depth++;
  }

  if (current) {
    // More ancestors exist
    parts.unshift('...');
  }

  return parts.join(' > ');
}
```

**Step 2: Verify file exists**

Run: `ls -la src/components/picker/`
Expected: All three files listed

**Step 3: Commit**

```bash
git add src/components/picker/entityMeta.js
git commit -m "feat(picker): add shared entity metadata utility

Fetches display metadata for resources and groups.
Includes breadcrumb generation for group hierarchy.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Create Index File for Picker Module

**Files:**
- Create: `src/components/picker/index.js`

**Step 1: Create index file**

```javascript
// src/components/picker/index.js
export { entityConfigs, getEntityConfig } from './entityConfigs.js';
export { registerEntityPickerStore } from './entityPicker.js';
export { fetchEntityMeta } from './entityMeta.js';
```

**Step 2: Commit**

```bash
git add src/components/picker/index.js
git commit -m "feat(picker): add module index

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Update main.js to Use New Entity Picker

**Files:**
- Modify: `src/main.js`

**Step 1: Update imports**

Replace line 32:
```javascript
import { registerResourcePickerStore } from './components/blocks/resourcePicker.js';
```

With:
```javascript
import { registerEntityPickerStore } from './components/picker/index.js';
```

**Step 2: Update store registration**

Replace line 70:
```javascript
registerResourcePickerStore(Alpine);
```

With:
```javascript
registerEntityPickerStore(Alpine);
```

**Step 3: Verify build succeeds**

Run: `npm run build-js`
Expected: Build completes without errors

**Step 4: Commit**

```bash
git add src/main.js
git commit -m "refactor(main): switch to generic entity picker store

Replaces resourcePickerStore with entityPickerStore.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Update blockGallery to Use Entity Picker

**Files:**
- Modify: `src/components/blocks/blockGallery.js`

**Step 1: Update openPicker method**

Replace the `openPicker()` method (lines 47-56):
```javascript
    openPicker() {
      const picker = Alpine.store('resourcePicker');
      if (!picker) {
        console.error('resourcePicker store not found');
        return;
      }
      picker.open(this.noteId, this.resourceIds, (selectedIds) => {
        this.addResources(selectedIds);
      });
    },
```

With:
```javascript
    openPicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'resource',
        noteId: this.noteId,
        existingIds: this.resourceIds,
        onConfirm: (selectedIds) => {
          this.addResources(selectedIds);
        }
      });
    },
```

**Step 2: Verify build succeeds**

Run: `npm run build-js`
Expected: Build completes without errors

**Step 3: Commit**

```bash
git add src/components/blocks/blockGallery.js
git commit -m "refactor(gallery): use generic entity picker

Updates openPicker() to use new entityPicker store API.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 7: Update blockReferences with Picker Support

**Files:**
- Modify: `src/components/blocks/blockReferences.js`

**Step 1: Replace entire file content**

```javascript
// src/components/blocks/blockReferences.js
import { fetchEntityMeta } from '../picker/index.js';

// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockReferences(block, saveContentFn, getEditMode) {
  return {
    block,
    saveContentFn,
    getEditMode,
    groupIds: [...(block?.content?.groupIds || [])],
    groupMeta: {},
    loadingMeta: false,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    async init() {
      await this.fetchGroupMeta();
    },

    async fetchGroupMeta() {
      if (this.groupIds.length === 0) return;

      this.loadingMeta = true;
      try {
        this.groupMeta = await fetchEntityMeta('group', this.groupIds);
      } catch (err) {
        console.warn('Failed to fetch group metadata:', err);
      } finally {
        this.loadingMeta = false;
      }
    },

    openPicker() {
      const picker = Alpine.store('entityPicker');
      if (!picker) {
        console.error('entityPicker store not found');
        return;
      }
      picker.open({
        entityType: 'group',
        existingIds: this.groupIds,
        onConfirm: (selectedIds) => {
          this.addGroups(selectedIds);
        }
      });
    },

    getGroupDisplay(id) {
      const meta = this.groupMeta[id];
      if (!meta) return { name: `Group ${id}`, breadcrumb: '' };
      return {
        name: meta.name || `Group ${id}`,
        breadcrumb: meta.breadcrumb || ''
      };
    },

    addGroups(ids) {
      this.groupIds = [...new Set([...this.groupIds, ...ids])];
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
      this.fetchGroupMeta();
    },

    removeGroup(id) {
      this.groupIds = this.groupIds.filter(gid => gid !== id);
      this.saveContentFn(this.block.id, { groupIds: this.groupIds });
    }
  };
}
```

**Step 2: Verify build succeeds**

Run: `npm run build-js`
Expected: Build completes without errors

**Step 3: Commit**

```bash
git add src/components/blocks/blockReferences.js
git commit -m "refactor(references): add entity picker and metadata support

- Adds openPicker() method using entityPicker store
- Fetches group metadata for display
- Removes comma-separated ID input logic

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Create Entity Picker Template Partial

**Files:**
- Create: `templates/partials/entityPicker.tpl`

**Step 1: Create the template file**

```html
{# Generic Entity Picker Modal #}
<div x-show="$store.entityPicker.isOpen"
     x-cloak
     class="fixed inset-0 z-50 overflow-y-auto"
     role="dialog"
     aria-modal="true"
     aria-labelledby="entity-picker-title"
     @keydown.escape.window="$store.entityPicker.close()">
    {# Backdrop #}
    <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
         @click="$store.entityPicker.close()"></div>

    {# Modal content #}
    <div class="flex min-h-full items-center justify-center p-4">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-3xl max-h-[80vh] flex flex-col"
             @click.stop
             x-trap.noscroll="$store.entityPicker.isOpen">
            {# Header #}
            <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                <h2 id="entity-picker-title" class="text-lg font-semibold text-gray-900">
                    Select <span x-text="$store.entityPicker.config?.entityLabel || 'Items'"></span>
                </h2>
                <button @click="$store.entityPicker.close()"
                        class="text-gray-400 hover:text-gray-600"
                        aria-label="Close">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
            </div>

            {# Tabs (if configured) #}
            <template x-if="$store.entityPicker.config?.tabs">
                <div class="flex border-b border-gray-200 px-4" role="tablist">
                    <template x-for="tab in $store.entityPicker.config.tabs" :key="tab.id">
                        <button @click="$store.entityPicker.setActiveTab(tab.id)"
                                :class="$store.entityPicker.activeTab === tab.id ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                                class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                                :disabled="tab.id === 'note' && !$store.entityPicker.noteId"
                                :class="{ 'opacity-50 cursor-not-allowed': tab.id === 'note' && !$store.entityPicker.noteId }"
                                role="tab"
                                :aria-selected="$store.entityPicker.activeTab === tab.id">
                            <span x-text="tab.label"></span>
                            <span x-show="tab.id === 'note' && $store.entityPicker.hasTabResults"
                                  class="ml-1 text-xs bg-gray-100 px-1.5 py-0.5 rounded"
                                  x-text="$store.entityPicker.tabResults.note?.length || 0"></span>
                        </button>
                    </template>
                </div>
            </template>

            {# Filters #}
            <div x-show="!$store.entityPicker.config?.tabs || $store.entityPicker.activeTab === 'all'"
                 class="px-4 py-3 border-b border-gray-200 space-y-2">
                {# Search #}
                <div>
                    <input type="text"
                           x-model="$store.entityPicker.searchQuery"
                           @input="$store.entityPicker.onSearchInput()"
                           placeholder="Search by name..."
                           class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-blue-500 focus:border-blue-500">
                </div>
                {# Dynamic filters based on config #}
                <template x-if="$store.entityPicker.config?.filters?.length > 0">
                    <div class="flex gap-3">
                        <template x-for="filter in $store.entityPicker.config.filters" :key="filter.key">
                            <div class="flex-1"
                                 x-data="autocompleter({
                                     selectedResults: [],
                                     url: filter.endpoint,
                                     max: filter.multi ? 0 : 1,
                                     standalone: true,
                                     onSelect: (item) => filter.multi
                                         ? $store.entityPicker.addToFilter(filter.key, item.ID)
                                         : $store.entityPicker.setFilter(filter.key, item.ID),
                                     onRemove: (item) => filter.multi
                                         ? $store.entityPicker.removeFromFilter(filter.key, item.ID)
                                         : $store.entityPicker.setFilter(filter.key, null)
                                 })"
                                 @entity-picker-closed.window="selectedResults = []">
                                <label class="block text-xs text-gray-500 mb-1" x-text="filter.label"></label>
                                <div class="relative">
                                    <input x-ref="autocompleter"
                                           type="text"
                                           x-bind="inputEvents"
                                           class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                           :placeholder="'Filter by ' + filter.label.toLowerCase() + '...'"
                                           autocomplete="off">
                                    <template x-if="dropdownActive && results.length > 0">
                                        <div class="absolute z-30 mt-1 w-full bg-white border border-gray-200 rounded shadow-lg max-h-40 overflow-y-auto">
                                            <template x-for="(result, index) in results" :key="result.ID">
                                                <div class="px-3 py-1.5 cursor-pointer text-sm"
                                                     :class="{'bg-blue-500 text-white': index === selectedIndex, 'hover:bg-gray-50': index !== selectedIndex}"
                                                     @mousedown="pushVal"
                                                     @mouseover="selectedIndex = index"
                                                     x-text="result.Name"></div>
                                            </template>
                                        </div>
                                    </template>
                                    <template x-if="selectedResults.length > 0">
                                        <div class="flex flex-wrap gap-1 mt-1">
                                            <template x-for="item in selectedResults" :key="item.ID">
                                                <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-xs">
                                                    <span x-text="item.Name" class="truncate max-w-[100px]"></span>
                                                    <button type="button" @click="removeItem(item)" class="hover:text-blue-600">&times;</button>
                                                </span>
                                            </template>
                                        </div>
                                    </template>
                                </div>
                            </div>
                        </template>
                    </div>
                </template>
            </div>

            {# Results grid #}
            <div class="flex-1 overflow-y-auto p-4">
                {# Loading state #}
                <div x-show="$store.entityPicker.loading" class="flex items-center justify-center py-12 text-gray-500">
                    <svg class="animate-spin h-6 w-6 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Loading...
                </div>

                {# Error state #}
                <div x-show="$store.entityPicker.error && !$store.entityPicker.loading"
                     class="text-center py-12 text-red-600">
                    <p x-text="$store.entityPicker.error"></p>
                    <button @click="$store.entityPicker.loadResults()"
                            class="mt-2 text-sm text-blue-600 hover:underline">Try again</button>
                </div>

                {# Empty state #}
                <div x-show="!$store.entityPicker.loading && !$store.entityPicker.error && $store.entityPicker.displayResults.length === 0"
                     class="text-center py-12 text-gray-500">
                    <p>No <span x-text="$store.entityPicker.config?.entityLabel?.toLowerCase() || 'items'"></span> found</p>
                </div>

                {# Results grid - Resource thumbnails #}
                <div x-show="!$store.entityPicker.loading && $store.entityPicker.displayResults.length > 0 && $store.entityPicker.config?.renderItem === 'thumbnail'"
                     :class="$store.entityPicker.config?.gridColumns || 'grid-cols-3'"
                     class="grid gap-3"
                     role="listbox"
                     :aria-label="'Available ' + ($store.entityPicker.config?.entityLabel?.toLowerCase() || 'items')">
                    <template x-for="item in $store.entityPicker.displayResults" :key="$store.entityPicker.config.getItemId(item)">
                        <div @click="$store.entityPicker.toggleSelection($store.entityPicker.config.getItemId(item))"
                             class="relative aspect-square bg-gray-100 rounded-lg overflow-hidden cursor-pointer transition-all"
                             :class="{
                                 'ring-2 ring-blue-500 ring-offset-2': $store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)),
                                 'opacity-50 cursor-not-allowed': $store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item)),
                                 'hover:ring-2 hover:ring-gray-300': !$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)) && !$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))
                             }"
                             role="option"
                             :aria-selected="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                             :aria-disabled="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))">
                            <img :src="'/v1/resource/preview?id=' + $store.entityPicker.config.getItemId(item)"
                                 :alt="$store.entityPicker.config.getItemLabel(item)"
                                 class="w-full h-full object-cover"
                                 loading="lazy">
                            {# Selection checkbox #}
                            <div x-show="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                                 class="absolute top-2 right-2 w-6 h-6 bg-blue-500 rounded-full flex items-center justify-center">
                                <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                </svg>
                            </div>
                            {# Already added badge #}
                            <div x-show="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))"
                                 class="absolute inset-0 bg-black bg-opacity-40 flex items-center justify-center">
                                <span class="text-xs text-white bg-black bg-opacity-60 px-2 py-1 rounded">Added</span>
                            </div>
                            {# Name tooltip #}
                            <div class="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/60 to-transparent p-2">
                                <p class="text-xs text-white truncate" x-text="$store.entityPicker.config.getItemLabel(item)"></p>
                            </div>
                        </div>
                    </template>
                </div>

                {# Results grid - Group cards #}
                <div x-show="!$store.entityPicker.loading && $store.entityPicker.displayResults.length > 0 && $store.entityPicker.config?.renderItem === 'groupCard'"
                     :class="$store.entityPicker.config?.gridColumns || 'grid-cols-2'"
                     class="grid gap-3"
                     role="listbox"
                     :aria-label="'Available ' + ($store.entityPicker.config?.entityLabel?.toLowerCase() || 'items')">
                    <template x-for="item in $store.entityPicker.displayResults" :key="$store.entityPicker.config.getItemId(item)">
                        <div @click="$store.entityPicker.toggleSelection($store.entityPicker.config.getItemId(item))"
                             class="flex items-start gap-3 p-3 border rounded-lg cursor-pointer transition-all"
                             :class="{
                                 'ring-2 ring-blue-500 border-blue-500 bg-blue-50': $store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)),
                                 'opacity-50 cursor-not-allowed bg-gray-50': $store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item)),
                                 'border-gray-200 hover:border-gray-300 hover:bg-gray-50': !$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item)) && !$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))
                             }"
                             role="option"
                             :aria-selected="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                             :aria-disabled="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))">
                            {# Thumbnail or icon #}
                            <div class="w-14 h-14 flex-shrink-0 bg-gray-100 rounded overflow-hidden">
                                <template x-if="item.MainResource?.ID">
                                    <img :src="'/v1/resource/preview?id=' + item.MainResource.ID"
                                         class="w-full h-full object-cover"
                                         loading="lazy">
                                </template>
                                <template x-if="!item.MainResource?.ID">
                                    <div class="w-full h-full flex items-center justify-center text-gray-400">
                                        <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                        </svg>
                                    </div>
                                </template>
                            </div>
                            {# Content #}
                            <div class="flex-1 min-w-0">
                                <div class="flex items-start justify-between">
                                    <p class="font-medium text-gray-900 truncate" x-text="item.Name || 'Unnamed Group'"></p>
                                    {# Selection indicator #}
                                    <div x-show="$store.entityPicker.isSelected($store.entityPicker.config.getItemId(item))"
                                         class="ml-2 w-5 h-5 bg-blue-500 rounded-full flex items-center justify-center flex-shrink-0">
                                        <svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                        </svg>
                                    </div>
                                    {# Already added badge #}
                                    <span x-show="$store.entityPicker.isAlreadyAdded($store.entityPicker.config.getItemId(item))"
                                          class="ml-2 text-xs bg-gray-200 text-gray-600 px-1.5 py-0.5 rounded flex-shrink-0">Added</span>
                                </div>
                                {# Breadcrumb #}
                                <p x-show="item.Owner?.Name" class="text-xs text-gray-500 truncate" x-text="item.Owner?.Name"></p>
                                {# Metadata #}
                                <div class="flex items-center gap-2 mt-1 text-xs text-gray-400">
                                    <span x-show="item.ResourceCount > 0" x-text="item.ResourceCount + ' resources'"></span>
                                    <span x-show="item.NoteCount > 0" x-text="item.NoteCount + ' notes'"></span>
                                    <span x-show="item.Category?.Name" class="px-1.5 py-0.5 bg-gray-100 rounded" x-text="item.Category?.Name"></span>
                                </div>
                            </div>
                        </div>
                    </template>
                </div>
            </div>

            {# Footer #}
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                <span class="text-sm text-gray-600">
                    <span x-text="$store.entityPicker.selectionCount"></span> selected
                </span>
                <div class="flex gap-2">
                    <button @click="$store.entityPicker.close()"
                            type="button"
                            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
                        Cancel
                    </button>
                    <button @click="$store.entityPicker.confirm()"
                            type="button"
                            :disabled="$store.entityPicker.selectionCount === 0"
                            class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                        Confirm
                    </button>
                </div>
            </div>
        </div>
    </div>
</div>
```

**Step 2: Commit**

```bash
git add templates/partials/entityPicker.tpl
git commit -m "feat(picker): add generic entity picker modal template

Supports both thumbnail and groupCard render modes.
Dynamically generates filters from config.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 9: Update blockEditor.tpl to Use Entity Picker

**Files:**
- Modify: `templates/partials/blockEditor.tpl`

**Step 1: Update references block template (lines 209-237)**

Replace:
```html
                    {# References block #}
                    <template x-if="block.type === 'references'">
                        <div x-data="blockReferences(block, (id, content) => updateBlockContent(id, content), () => editMode)">
                            <template x-if="!editMode && groupIds.length > 0">
                                <div class="flex flex-wrap gap-2">
                                    <template x-for="gId in groupIds" :key="gId">
                                        <a :href="'/group?id=' + gId" class="inline-flex items-center px-3 py-1 bg-blue-100 text-blue-700 rounded-full text-sm hover:bg-blue-200">
                                            Group <span x-text="gId" class="ml-1 font-medium"></span>
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && groupIds.length === 0">
                                <p class="text-gray-400 text-sm">No groups selected</p>
                            </template>
                            <template x-if="editMode">
                                <div>
                                    <p class="text-sm text-gray-500 mb-2">Group IDs (comma-separated):</p>
                                    <input
                                        type="text"
                                        :value="groupIds.join(', ')"
                                        @blur="updateGroupIds($event.target.value)"
                                        class="w-full p-2 border border-gray-300 rounded"
                                        placeholder="e.g., 1, 2, 3"
                                    >
                                </div>
                            </template>
                        </div>
                    </template>
```

With:
```html
                    {# References block #}
                    <template x-if="block.type === 'references'">
                        <div x-data="blockReferences(block, (id, content) => updateBlockContent(id, content), () => editMode)" x-init="init()">
                            <template x-if="!editMode && groupIds.length > 0">
                                <div class="flex flex-wrap gap-2">
                                    <template x-for="gId in groupIds" :key="gId">
                                        <a :href="'/group?id=' + gId"
                                           class="inline-flex items-center gap-1 px-3 py-1.5 bg-blue-50 text-blue-700 rounded-lg text-sm hover:bg-blue-100 border border-blue-200">
                                            <svg class="w-4 h-4 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                            </svg>
                                            <span class="font-medium" x-text="getGroupDisplay(gId).name"></span>
                                            <span x-show="getGroupDisplay(gId).breadcrumb" class="text-blue-400 text-xs" x-text="'in ' + getGroupDisplay(gId).breadcrumb"></span>
                                        </a>
                                    </template>
                                </div>
                            </template>
                            <template x-if="!editMode && groupIds.length === 0">
                                <p class="text-gray-400 text-sm">No groups selected</p>
                            </template>
                            <template x-if="editMode">
                                <div class="space-y-3">
                                    {# Selected groups preview #}
                                    <template x-if="groupIds.length > 0">
                                        <div class="flex flex-wrap gap-2">
                                            <template x-for="gId in groupIds" :key="gId">
                                                <div class="inline-flex items-center gap-1 px-3 py-1.5 bg-blue-50 text-blue-700 rounded-lg text-sm border border-blue-200">
                                                    <svg class="w-4 h-4 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                                                    </svg>
                                                    <span class="font-medium" x-text="getGroupDisplay(gId).name"></span>
                                                    <button @click="removeGroup(gId)"
                                                            class="ml-1 w-4 h-4 rounded-full bg-blue-200 text-blue-600 hover:bg-blue-300 flex items-center justify-center text-xs"
                                                            title="Remove">&times;</button>
                                                </div>
                                            </template>
                                        </div>
                                    </template>
                                    {# Add groups button #}
                                    <button
                                        @click="openPicker()"
                                        type="button"
                                        class="w-full py-2 px-4 border-2 border-dashed border-gray-300 rounded-lg text-gray-500 hover:border-blue-400 hover:text-blue-500 transition-colors text-sm"
                                    >
                                        + Select Groups
                                    </button>
                                </div>
                            </template>
                        </div>
                    </template>
```

**Step 2: Replace resource picker modal (lines 509-758)**

Replace the entire `{# Resource Picker Modal #}` section with:
```html
    {# Entity Picker Modal #}
    {% include "partials/entityPicker.tpl" %}
```

**Step 3: Verify build succeeds**

Run: `npm run build`
Expected: Build completes without errors

**Step 4: Commit**

```bash
git add templates/partials/blockEditor.tpl
git commit -m "refactor(blockEditor): use generic entity picker

- Updates references block to show group names with breadcrumbs
- Adds picker button for group selection
- Replaces resource picker modal with generic entity picker

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Delete Old Resource Picker File

**Files:**
- Delete: `src/components/blocks/resourcePicker.js`

**Step 1: Delete the file**

Run: `rm src/components/blocks/resourcePicker.js`

**Step 2: Verify build still works**

Run: `npm run build-js`
Expected: Build completes without errors

**Step 3: Commit**

```bash
git add -A
git commit -m "refactor(picker): remove old resourcePicker.js

Replaced by generic entityPicker store.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 11: Add E2E Tests for Entity Picker

**Files:**
- Create: `e2e/tests/20-entity-picker.spec.ts`

**Step 1: Create test file**

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Entity Picker - Resource Selection', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Picker Test Category', 'Category for picker tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Picker Test Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Picker Test Note',
      description: 'Note for testing entity picker',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create a resource to select
    const resource = await apiClient.createResource({
      name: 'Test Resource for Picker',
      groupId: ownerGroupId,
    });
    resourceId = resource.ID;
  });

  test('should open resource picker from gallery block', async ({ page, baseURL, apiClient }) => {
    // Create a gallery block
    await apiClient.createBlock(noteId, 'gallery', 'n', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    // Click Select Resources button
    await page.locator('button:has-text("Select Resources")').click();

    // Modal should open
    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Resources")')).toBeVisible();
  });

  test('should search resources in picker', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'o', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    // Switch to All Resources tab
    await page.locator('button:has-text("All Resources")').click();

    // Search for resource
    const searchInput = page.locator('[role="dialog"] input[placeholder="Search by name..."]');
    await searchInput.fill('Test Resource');

    // Wait for results
    await page.waitForTimeout(300); // Debounce wait

    // Should show matching resource
    await expect(page.locator('[role="option"]')).toBeVisible();
  });

  test('should select and confirm resources', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'p', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();
    await page.locator('button:has-text("All Resources")').click();

    // Click on a resource to select it
    const resourceOption = page.locator('[role="option"]').first();
    await resourceOption.click();

    // Selection count should update
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Confirm selection
    await page.locator('button:has-text("Confirm")').click();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should cancel selection without adding', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'q', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();
    await page.locator('button:has-text("All Resources")').click();

    // Select a resource
    await page.locator('[role="option"]').first().click();
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Cancel
    await page.locator('button:has-text("Cancel")').click();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should close picker with escape key', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'gallery', 'r', { resourceIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    await expect(page.locator('[role="dialog"]')).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});

test.describe('Entity Picker - Group Selection', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let selectableGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Group Picker Category', 'For group picker tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Group Picker Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a group to select
    const selectableGroup = await apiClient.createGroup({
      name: 'Selectable Test Group',
      categoryId: categoryId,
    });
    selectableGroupId = selectableGroup.ID;

    const note = await apiClient.createNote({
      name: 'Group Picker Test Note',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should open group picker from references block', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'n', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    await expect(page.locator('[role="dialog"]')).toBeVisible();
    await expect(page.locator('h2:has-text("Select Groups")')).toBeVisible();
  });

  test('should display groups as cards', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'o', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Group cards should be visible (not thumbnails)
    const groupCard = page.locator('[role="option"]').first();
    await expect(groupCard).toBeVisible();

    // Should contain group name text
    await expect(groupCard.locator('p.font-medium')).toBeVisible();
  });

  test('should select and confirm groups', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'p', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Select a group
    await page.locator('[role="option"]').first().click();
    await expect(page.locator('text=1 selected')).toBeVisible();

    // Confirm
    await page.locator('button:has-text("Confirm")').click();

    // Modal closes and group appears in block
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should filter groups by category', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 'q', { groupIds: [] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Category filter should be visible
    const categoryFilter = page.locator('label:has-text("Category")').locator('..').locator('input');
    await expect(categoryFilter).toBeVisible();
  });

  test('should show already added groups as disabled', async ({ page, baseURL, apiClient }) => {
    // Create block with a group already added
    await apiClient.createBlock(noteId, 'references', 'r', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Groups")').click();

    // Find the already-added group and check for "Added" badge
    const addedBadge = page.locator('[role="option"]').filter({ hasText: 'Selectable Test Group' }).locator('text=Added');
    await expect(addedBadge).toBeVisible();
  });

  test('should remove group from references block', async ({ page, baseURL, apiClient }) => {
    await apiClient.createBlock(noteId, 'references', 's', { groupIds: [selectableGroupId] });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await page.locator('button:has-text("Edit Blocks")').click();

    // Find remove button on the group pill
    const removeButton = page.locator('.block-content').filter({ hasText: 'references' }).locator('button[title="Remove"]');
    await removeButton.click();

    // Group should be removed
    await expect(page.locator('.block-content').filter({ hasText: 'references' }).locator('text=Selectable Test Group')).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (selectableGroupId) await apiClient.deleteGroup(selectableGroupId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
```

**Step 2: Run tests to verify they pass**

Run: `cd e2e && npm run test:with-server -- --grep "Entity Picker"`
Expected: All entity picker tests pass

**Step 3: Commit**

```bash
git add e2e/tests/20-entity-picker.spec.ts
git commit -m "test(e2e): add entity picker tests

Tests resource picker from gallery block and
group picker from references block.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 12: Add Accessibility Tests

**Files:**
- Modify: `e2e/tests/accessibility/02-a11y-components.spec.ts`

**Step 1: Add entity picker accessibility test**

Add to the file after existing tests:

```typescript
test.describe('Entity Picker Accessibility', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('A11y Picker Category', 'For a11y tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'A11y Picker Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'A11y Picker Test Note',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create a gallery block
    await apiClient.createBlock(note.ID, 'gallery', 'n', { resourceIds: [] });
  });

  test('entity picker modal should be accessible', async ({ page, baseURL, makeAxeBuilder }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode and open picker
    await page.locator('button:has-text("Edit Blocks")').click();
    await page.locator('button:has-text("Select Resources")').click();

    await expect(page.locator('[role="dialog"]')).toBeVisible();

    const accessibilityScanResults = await makeAxeBuilder()
      .include('[role="dialog"]')
      .analyze();

    expect(accessibilityScanResults.violations).toEqual([]);
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
```

**Step 2: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: Tests pass

**Step 3: Commit**

```bash
git add e2e/tests/accessibility/02-a11y-components.spec.ts
git commit -m "test(a11y): add entity picker accessibility tests

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 13: Add User Documentation

**Files:**
- Modify: `docs-site/docs/user-guide/organizing-with-groups.md`

**Step 1: Add section about references blocks**

Add to the end of the file:

```markdown
## Adding Groups to Notes via References Block

Notes can reference groups using the **References block** in the block editor. This creates visual links to related groups.

### Adding Groups

1. Open a note and click **Edit Blocks**
2. Add a **References** block or find an existing one
3. Click **+ Select Groups**
4. In the picker modal:
   - Search by group name
   - Filter by category
   - Click groups to select them
   - Click **Confirm** to add

### Viewing References

In view mode, referenced groups appear as clickable pills showing:
- Group name
- Parent group breadcrumb (if applicable)

Click any group pill to navigate to that group's detail page.

### Removing Groups

In edit mode:
1. Find the group pill you want to remove
2. Click the **×** button on the pill
```

**Step 2: Commit**

```bash
git add docs-site/docs/user-guide/organizing-with-groups.md
git commit -m "docs(user): add references block documentation

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 14: Add Developer Documentation

**Files:**
- Create: `docs-site/docs/features/entity-picker.md`

**Step 1: Create developer documentation**

```markdown
---
sidebar_position: 5
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
```

**Step 2: Update sidebar**

If needed, add to `docs-site/sidebars.js` to include the new page.

**Step 3: Commit**

```bash
git add docs-site/docs/features/entity-picker.md
git commit -m "docs(dev): add entity picker developer guide

Documents how to use and extend the entity picker.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 15: Run Full Test Suite

**Step 1: Run all Go tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Run full E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass (264+ tests)

**Step 3: Build production bundle**

Run: `npm run build`
Expected: Build completes successfully

**Step 4: Commit any fixes if needed**

---

## Task 16: Final Cleanup and Summary Commit

**Step 1: Review all changes**

Run: `git log --oneline master..HEAD`
Expected: See all commits from this implementation

**Step 2: Verify working directory is clean**

Run: `git status`
Expected: Clean working directory

**Step 3: Done!**

The implementation is complete. The worktree can be merged to master when ready.
