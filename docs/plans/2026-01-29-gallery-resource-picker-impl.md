# Gallery Resource Picker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the comma-separated ID input in the gallery block with a visual resource picker modal.

**Architecture:** Create a standalone Alpine.js component (`resourcePicker`) that renders a modal with tabs (note's resources / all resources), thumbnail grid with multi-select, and tag/group filters using the existing autocompleter. The picker communicates via callback when user confirms selection.

**Tech Stack:** Alpine.js, existing autocompleter component, Tailwind CSS, existing `/v1/resources` API

---

### Task 1: Create resourcePicker.js Component

**Files:**
- Create: `src/components/blocks/resourcePicker.js`

**Step 1: Create the resourcePicker component file**

```javascript
// src/components/blocks/resourcePicker.js
import { abortableFetch } from '../../index.js';

export function resourcePicker() {
  return {
    isOpen: false,
    noteId: null,
    onConfirm: null,
    existingIds: new Set(), // IDs already in gallery

    // Tab state
    activeTab: 'note', // 'note' or 'all'

    // Resources data
    noteResources: [],
    allResources: [],
    loading: false,
    error: null,

    // Selection
    selectedIds: new Set(),

    // Search & filters
    searchQuery: '',
    selectedTagId: null,
    selectedGroupId: null,
    searchDebounceTimer: null,
    requestAborter: null,

    // ARIA live region
    liveRegion: null,

    init() {
      // Create ARIA live region
      this.liveRegion = document.createElement('div');
      this.liveRegion.setAttribute('role', 'status');
      this.liveRegion.setAttribute('aria-live', 'polite');
      this.liveRegion.setAttribute('aria-atomic', 'true');
      this.liveRegion.className = 'sr-only';
      this.$el.appendChild(this.liveRegion);
    },

    announce(message) {
      if (this.liveRegion) {
        this.liveRegion.textContent = '';
        setTimeout(() => {
          this.liveRegion.textContent = message;
        }, 50);
      }
    },

    open(noteId, existingIds, onConfirm) {
      this.noteId = noteId;
      this.existingIds = new Set(existingIds || []);
      this.onConfirm = onConfirm;
      this.selectedIds = new Set();
      this.searchQuery = '';
      this.selectedTagId = null;
      this.selectedGroupId = null;
      this.error = null;
      this.isOpen = true;

      // Determine default tab
      this.activeTab = this.noteId ? 'note' : 'all';

      // Load resources
      if (this.noteId) {
        this.loadNoteResources();
      }
      this.loadAllResources();

      // Focus search input after modal opens
      this.$nextTick(() => {
        this.$refs.searchInput?.focus();
        this.announce('Resource picker opened. Use tabs to switch between note resources and all resources.');
      });
    },

    close() {
      this.isOpen = false;
      this.noteResources = [];
      this.allResources = [];
      this.selectedIds = new Set();
      if (this.requestAborter) {
        this.requestAborter();
        this.requestAborter = null;
      }
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
        this.noteResources = await res.json();
        if (this.noteResources.length === 0 && this.activeTab === 'note') {
          this.activeTab = 'all';
        }
      } catch (err) {
        console.error('Error loading note resources:', err);
      }
    },

    async loadAllResources() {
      if (this.requestAborter) {
        this.requestAborter();
      }

      this.loading = true;
      this.error = null;

      const params = new URLSearchParams({ MaxResults: '50' });
      if (this.searchQuery.trim()) {
        params.set('name', this.searchQuery.trim());
      }
      if (this.selectedTagId) {
        params.set('Tags', this.selectedTagId);
      }
      if (this.selectedGroupId) {
        params.set('Groups', this.selectedGroupId);
      }

      const { abort, ready } = abortableFetch(`/v1/resources?${params}`);
      this.requestAborter = abort;

      try {
        const res = await ready;
        if (!res.ok) throw new Error('Failed to load resources');
        this.allResources = await res.json();
        this.announce(`${this.allResources.length} resources found.`);
      } catch (err) {
        if (err.name !== 'AbortError') {
          this.error = err.message || 'Failed to load resources';
          console.error('Error loading resources:', err);
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
        this.loadAllResources();
      }, 200);
    },

    onTagSelected(tag) {
      this.selectedTagId = tag?.ID || null;
      this.loadAllResources();
    },

    onTagCleared() {
      this.selectedTagId = null;
      this.loadAllResources();
    },

    onGroupSelected(group) {
      this.selectedGroupId = group?.ID || null;
      this.loadAllResources();
    },

    onGroupCleared() {
      this.selectedGroupId = null;
      this.loadAllResources();
    },

    toggleSelection(resourceId) {
      if (this.existingIds.has(resourceId)) return; // Can't select already-added

      if (this.selectedIds.has(resourceId)) {
        this.selectedIds.delete(resourceId);
      } else {
        this.selectedIds.add(resourceId);
      }
      // Trigger reactivity
      this.selectedIds = new Set(this.selectedIds);
      this.announce(`${this.selectedIds.size} resources selected.`);
    },

    isSelected(resourceId) {
      return this.selectedIds.has(resourceId);
    },

    isAlreadyAdded(resourceId) {
      return this.existingIds.has(resourceId);
    },

    get displayResources() {
      return this.activeTab === 'note' ? this.noteResources : this.allResources;
    },

    get hasNoteResources() {
      return this.noteResources.length > 0;
    },

    get selectionCount() {
      return this.selectedIds.size;
    }
  };
}
```

**Step 2: Run build to verify no syntax errors**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds (component not yet imported)

**Step 3: Commit**

```bash
git add src/components/blocks/resourcePicker.js
git commit -m "feat(gallery): add resourcePicker component skeleton"
```

---

### Task 2: Register resourcePicker in main.js

**Files:**
- Modify: `src/main.js:38` (import line)
- Modify: `src/main.js:89` (registration line)
- Modify: `src/components/blocks/index.js`

**Step 1: Add export to blocks/index.js**

Add after line 8:

```javascript
export { resourcePicker } from './resourcePicker.js';
```

**Step 2: Update import in main.js**

Change line 38 from:
```javascript
import { blockText, blockHeading, blockDivider, blockTodos, blockGallery, blockReferences, blockTable } from './components/blocks/index.js';
```

To:
```javascript
import { blockText, blockHeading, blockDivider, blockTodos, blockGallery, blockReferences, blockTable, resourcePicker } from './components/blocks/index.js';
```

**Step 3: Register component in main.js**

Add after line 89 (`Alpine.data('blockTable', blockTable);`):

```javascript
Alpine.data('resourcePicker', resourcePicker);
```

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add src/components/blocks/index.js src/main.js
git commit -m "feat(gallery): register resourcePicker component"
```

---

### Task 3: Update blockGallery to Support Picker

**Files:**
- Modify: `src/components/blocks/blockGallery.js`

**Step 1: Add noteId parameter and picker integration**

Replace the entire file with:

```javascript
// src/components/blocks/blockGallery.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockGallery(block, saveContentFn, getEditMode, noteId) {
  return {
    block,
    saveContentFn,
    getEditMode,
    noteId,
    resourceIds: [...(block?.content?.resourceIds || [])],
    resourceMeta: {}, // Cache for resource metadata (contentType, name, hash)

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    async init() {
      // Fetch metadata for all resources to enable lightbox
      await this.fetchResourceMeta();
    },

    async fetchResourceMeta() {
      if (this.resourceIds.length === 0) return;

      // Fetch metadata for resources we don't have yet
      const toFetch = this.resourceIds.filter(id => !this.resourceMeta[id]);
      if (toFetch.length === 0) return;

      try {
        const promises = toFetch.map(id =>
          fetch(`/v1/resource?id=${id}`).then(r => r.ok ? r.json() : null)
        );
        const results = await Promise.all(promises);
        results.forEach((res, i) => {
          if (res) {
            this.resourceMeta[toFetch[i]] = {
              contentType: res.ContentType || '',
              name: res.Name || '',
              hash: res.Hash || ''
            };
          }
        });
      } catch (err) {
        console.warn('Failed to fetch resource metadata for gallery:', err);
      }
    },

    openGalleryLightbox(index) {
      const lightbox = Alpine.store('lightbox');
      if (!lightbox) return;

      // Build items array from resourceIds with metadata
      const items = this.resourceIds.map(id => {
        const meta = this.resourceMeta[id] || {};
        const hash = meta.hash || '';
        const versionParam = hash ? `&v=${hash}` : '';
        return {
          id,
          viewUrl: `/v1/resource/view?id=${id}${versionParam}`,
          contentType: meta.contentType || 'image/jpeg', // Default to image
          name: meta.name || '',
          hash: hash
        };
      }).filter(item =>
        item.contentType?.startsWith('image/') ||
        item.contentType?.startsWith('video/')
      );

      if (items.length === 0) return;

      // Set items and open lightbox
      lightbox.items = items;
      lightbox.loadedPages = new Set([1]);
      lightbox.hasNextPage = false;
      lightbox.hasPrevPage = false;
      lightbox.open(index);
    },

    updateResourceIds(value) {
      // Parse comma-separated IDs
      this.resourceIds = value
        .split(',')
        .map(s => parseInt(s.trim(), 10))
        .filter(n => !isNaN(n) && n > 0);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
      // Fetch metadata for any new resources
      this.fetchResourceMeta();
    },

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

    addResources(ids) {
      this.resourceIds = [...new Set([...this.resourceIds, ...ids])];
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
      this.fetchResourceMeta();
    },

    removeResource(id) {
      this.resourceIds = this.resourceIds.filter(rid => rid !== id);
      this.saveContentFn(this.block.id, { resourceIds: this.resourceIds });
    }
  };
}
```

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add src/components/blocks/blockGallery.js
git commit -m "feat(gallery): add noteId param and openPicker method to blockGallery"
```

---

### Task 4: Convert resourcePicker to Alpine Store

**Files:**
- Modify: `src/components/blocks/resourcePicker.js`
- Modify: `src/main.js`

The picker needs to be a global store so it can be accessed from anywhere. Update the implementation.

**Step 1: Convert resourcePicker to store registration function**

Replace `src/components/blocks/resourcePicker.js`:

```javascript
// src/components/blocks/resourcePicker.js
import { abortableFetch } from '../../index.js';

export function registerResourcePickerStore(Alpine) {
  Alpine.store('resourcePicker', {
    isOpen: false,
    noteId: null,
    onConfirm: null,
    existingIds: new Set(),

    // Tab state
    activeTab: 'note',

    // Resources data
    noteResources: [],
    allResources: [],
    loading: false,
    error: null,

    // Selection
    selectedIds: new Set(),

    // Search & filters
    searchQuery: '',
    selectedTagId: null,
    selectedGroupId: null,
    searchDebounceTimer: null,
    requestAborter: null,

    open(noteId, existingIds, onConfirm) {
      this.noteId = noteId;
      this.existingIds = new Set(existingIds || []);
      this.onConfirm = onConfirm;
      this.selectedIds = new Set();
      this.searchQuery = '';
      this.selectedTagId = null;
      this.selectedGroupId = null;
      this.error = null;
      this.isOpen = true;

      this.activeTab = this.noteId ? 'note' : 'all';

      if (this.noteId) {
        this.loadNoteResources();
      }
      this.loadAllResources();
    },

    close() {
      this.isOpen = false;
      this.noteResources = [];
      this.allResources = [];
      this.selectedIds = new Set();
      if (this.requestAborter) {
        this.requestAborter();
        this.requestAborter = null;
      }
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
        this.noteResources = await res.json();
        if (this.noteResources.length === 0 && this.activeTab === 'note') {
          this.activeTab = 'all';
        }
      } catch (err) {
        console.error('Error loading note resources:', err);
      }
    },

    async loadAllResources() {
      if (this.requestAborter) {
        this.requestAborter();
      }

      this.loading = true;
      this.error = null;

      const params = new URLSearchParams({ MaxResults: '50' });
      if (this.searchQuery.trim()) {
        params.set('name', this.searchQuery.trim());
      }
      if (this.selectedTagId) {
        params.set('Tags', this.selectedTagId);
      }
      if (this.selectedGroupId) {
        params.set('Groups', this.selectedGroupId);
      }

      const { abort, ready } = abortableFetch(`/v1/resources?${params}`);
      this.requestAborter = abort;

      try {
        const res = await ready;
        if (!res.ok) throw new Error('Failed to load resources');
        this.allResources = await res.json();
      } catch (err) {
        if (err.name !== 'AbortError') {
          this.error = err.message || 'Failed to load resources';
          console.error('Error loading resources:', err);
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
        this.loadAllResources();
      }, 200);
    },

    setTagFilter(tagId) {
      this.selectedTagId = tagId;
      this.loadAllResources();
    },

    setGroupFilter(groupId) {
      this.selectedGroupId = groupId;
      this.loadAllResources();
    },

    clearTagFilter() {
      this.selectedTagId = null;
      this.loadAllResources();
    },

    clearGroupFilter() {
      this.selectedGroupId = null;
      this.loadAllResources();
    },

    toggleSelection(resourceId) {
      if (this.existingIds.has(resourceId)) return;

      if (this.selectedIds.has(resourceId)) {
        this.selectedIds.delete(resourceId);
      } else {
        this.selectedIds.add(resourceId);
      }
      this.selectedIds = new Set(this.selectedIds);
    },

    isSelected(resourceId) {
      return this.selectedIds.has(resourceId);
    },

    isAlreadyAdded(resourceId) {
      return this.existingIds.has(resourceId);
    },

    get displayResources() {
      return this.activeTab === 'note' ? this.noteResources : this.allResources;
    },

    get hasNoteResources() {
      return this.noteResources.length > 0;
    },

    get selectionCount() {
      return this.selectedIds.size;
    }
  });
}
```

**Step 2: Update main.js to register store**

Change import line 38 to remove resourcePicker from component imports:
```javascript
import { blockText, blockHeading, blockDivider, blockTodos, blockGallery, blockReferences, blockTable } from './components/blocks/index.js';
```

Add new import after line 38:
```javascript
import { registerResourcePickerStore } from './components/blocks/resourcePicker.js';
```

Add store registration after line 67 (`registerLightboxStore(Alpine);`):
```javascript
registerResourcePickerStore(Alpine);
```

Remove the `Alpine.data('resourcePicker', resourcePicker);` line added in Task 2.

**Step 3: Update blocks/index.js**

Remove the resourcePicker export added in Task 2.

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add src/components/blocks/resourcePicker.js src/components/blocks/index.js src/main.js
git commit -m "refactor(gallery): convert resourcePicker to Alpine store"
```

---

### Task 5: Add Resource Picker Modal Template

**Files:**
- Modify: `templates/partials/blockEditor.tpl`

**Step 1: Update gallery block edit mode UI**

Find the gallery block edit template (around line 179-190) and replace with:

```django
<template x-if="editMode">
    <div class="space-y-3">
        {# Selected resources preview #}
        <template x-if="resourceIds.length > 0">
            <div class="grid grid-cols-4 md:grid-cols-6 gap-2">
                <template x-for="(resId, idx) in resourceIds" :key="resId">
                    <div class="relative group aspect-square bg-gray-100 rounded overflow-hidden">
                        <img :src="'/v1/resource/preview?id=' + resId" class="w-full h-full object-cover">
                        <button
                            @click="removeResource(resId)"
                            class="absolute top-1 right-1 w-5 h-5 bg-red-500 text-white rounded-full text-xs opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
                            title="Remove"
                        >&times;</button>
                    </div>
                </template>
            </div>
        </template>
        {# Add resources button #}
        <button
            @click="openPicker()"
            type="button"
            class="w-full py-2 px-4 border-2 border-dashed border-gray-300 rounded-lg text-gray-500 hover:border-blue-400 hover:text-blue-500 transition-colors text-sm"
        >
            + Select Resources
        </button>
    </div>
</template>
```

**Step 2: Update blockGallery initialization to pass noteId**

Find line 164 and change from:
```django
<div x-data="blockGallery(block, (id, content) => updateBlockContent(id, content), () => editMode)">
```

To:
```django
<div x-data="blockGallery(block, (id, content) => updateBlockContent(id, content), () => editMode, noteId)" x-init="init()">
```

**Step 3: Add the resource picker modal at the end of blockEditor.tpl**

Add before the closing `</div>` of the block-editor div (after line ~492):

```django
{# Resource Picker Modal #}
<div x-show="$store.resourcePicker.isOpen"
     x-cloak
     class="fixed inset-0 z-50 overflow-y-auto"
     role="dialog"
     aria-modal="true"
     aria-labelledby="resource-picker-title"
     @keydown.escape.window="$store.resourcePicker.close()">
    {# Backdrop #}
    <div class="fixed inset-0 bg-black bg-opacity-50 transition-opacity"
         @click="$store.resourcePicker.close()"></div>

    {# Modal content #}
    <div class="flex min-h-full items-center justify-center p-4">
        <div class="relative bg-white rounded-lg shadow-xl w-full max-w-3xl max-h-[80vh] flex flex-col"
             @click.stop
             x-trap.noscroll="$store.resourcePicker.isOpen">
            {# Header #}
            <div class="flex items-center justify-between px-4 py-3 border-b border-gray-200">
                <h2 id="resource-picker-title" class="text-lg font-semibold text-gray-900">Select Resources</h2>
                <button @click="$store.resourcePicker.close()"
                        class="text-gray-400 hover:text-gray-600"
                        aria-label="Close">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
            </div>

            {# Tabs #}
            <div class="flex border-b border-gray-200 px-4" role="tablist">
                <button @click="$store.resourcePicker.activeTab = 'note'"
                        :class="$store.resourcePicker.activeTab === 'note' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                        class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                        :disabled="!$store.resourcePicker.noteId"
                        :class="{ 'opacity-50 cursor-not-allowed': !$store.resourcePicker.noteId }"
                        role="tab"
                        :aria-selected="$store.resourcePicker.activeTab === 'note'">
                    Note's Resources
                    <span x-show="$store.resourcePicker.noteResources.length > 0"
                          class="ml-1 text-xs bg-gray-100 px-1.5 py-0.5 rounded"
                          x-text="$store.resourcePicker.noteResources.length"></span>
                </button>
                <button @click="$store.resourcePicker.activeTab = 'all'"
                        :class="$store.resourcePicker.activeTab === 'all' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'"
                        class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors"
                        role="tab"
                        :aria-selected="$store.resourcePicker.activeTab === 'all'">
                    All Resources
                </button>
            </div>

            {# Filters (All Resources tab only) #}
            <div x-show="$store.resourcePicker.activeTab === 'all'" class="px-4 py-3 border-b border-gray-200 space-y-2">
                {# Search #}
                <div>
                    <input type="text"
                           x-model="$store.resourcePicker.searchQuery"
                           @input="$store.resourcePicker.onSearchInput()"
                           placeholder="Search by name..."
                           class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-blue-500 focus:border-blue-500">
                </div>
                {# Tag & Group filters #}
                <div class="flex gap-3">
                    {# Tag filter #}
                    <div class="flex-1"
                         x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/tags',
                             max: 1,
                             standalone: true,
                             onSelect: (tag) => $store.resourcePicker.setTagFilter(tag.ID),
                             onRemove: () => $store.resourcePicker.clearTagFilter()
                         })"
                         @resource-picker-closed.window="selectedResults = []">
                        <label class="block text-xs text-gray-500 mb-1">Tag</label>
                        <div class="relative">
                            <input x-ref="autocompleter"
                                   type="text"
                                   x-bind="inputEvents"
                                   class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                   :placeholder="selectedResults.length ? '' : 'Filter by tag...'"
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
                    {# Group filter #}
                    <div class="flex-1"
                         x-data="autocompleter({
                             selectedResults: [],
                             url: '/v1/groups',
                             max: 1,
                             standalone: true,
                             onSelect: (group) => $store.resourcePicker.setGroupFilter(group.ID),
                             onRemove: () => $store.resourcePicker.clearGroupFilter()
                         })"
                         @resource-picker-closed.window="selectedResults = []">
                        <label class="block text-xs text-gray-500 mb-1">Group</label>
                        <div class="relative">
                            <input x-ref="autocompleter"
                                   type="text"
                                   x-bind="inputEvents"
                                   class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                                   :placeholder="selectedResults.length ? '' : 'Filter by group...'"
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
                                        <span class="inline-flex items-center gap-1 px-2 py-0.5 bg-green-100 text-green-800 rounded text-xs">
                                            <span x-text="item.Name" class="truncate max-w-[100px]"></span>
                                            <button type="button" @click="removeItem(item)" class="hover:text-green-600">&times;</button>
                                        </span>
                                    </template>
                                </div>
                            </template>
                        </div>
                    </div>
                </div>
            </div>

            {# Resource grid #}
            <div class="flex-1 overflow-y-auto p-4">
                {# Loading state #}
                <div x-show="$store.resourcePicker.loading" class="flex items-center justify-center py-12 text-gray-500">
                    <svg class="animate-spin h-6 w-6 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Loading...
                </div>

                {# Error state #}
                <div x-show="$store.resourcePicker.error && !$store.resourcePicker.loading"
                     class="text-center py-12 text-red-600">
                    <p x-text="$store.resourcePicker.error"></p>
                    <button @click="$store.resourcePicker.loadAllResources()"
                            class="mt-2 text-sm text-blue-600 hover:underline">Try again</button>
                </div>

                {# Empty state #}
                <div x-show="!$store.resourcePicker.loading && !$store.resourcePicker.error && $store.resourcePicker.displayResources.length === 0"
                     class="text-center py-12 text-gray-500">
                    <template x-if="$store.resourcePicker.activeTab === 'note'">
                        <p>No resources attached to this note</p>
                    </template>
                    <template x-if="$store.resourcePicker.activeTab === 'all'">
                        <p>No resources found</p>
                    </template>
                </div>

                {# Resource grid #}
                <div x-show="!$store.resourcePicker.loading && $store.resourcePicker.displayResources.length > 0"
                     class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-3"
                     role="listbox"
                     aria-label="Available resources">
                    <template x-for="resource in $store.resourcePicker.displayResources" :key="resource.ID">
                        <div @click="$store.resourcePicker.toggleSelection(resource.ID)"
                             class="relative aspect-square bg-gray-100 rounded-lg overflow-hidden cursor-pointer transition-all"
                             :class="{
                                 'ring-2 ring-blue-500 ring-offset-2': $store.resourcePicker.isSelected(resource.ID),
                                 'opacity-50 cursor-not-allowed': $store.resourcePicker.isAlreadyAdded(resource.ID),
                                 'hover:ring-2 hover:ring-gray-300': !$store.resourcePicker.isSelected(resource.ID) && !$store.resourcePicker.isAlreadyAdded(resource.ID)
                             }"
                             role="option"
                             :aria-selected="$store.resourcePicker.isSelected(resource.ID)"
                             :aria-disabled="$store.resourcePicker.isAlreadyAdded(resource.ID)">
                            <img :src="'/v1/resource/preview?id=' + resource.ID"
                                 :alt="resource.Name || 'Resource ' + resource.ID"
                                 class="w-full h-full object-cover"
                                 loading="lazy">
                            {# Selection checkbox #}
                            <div x-show="$store.resourcePicker.isSelected(resource.ID)"
                                 class="absolute top-2 right-2 w-6 h-6 bg-blue-500 rounded-full flex items-center justify-center">
                                <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                                </svg>
                            </div>
                            {# Already added badge #}
                            <div x-show="$store.resourcePicker.isAlreadyAdded(resource.ID)"
                                 class="absolute inset-0 bg-black bg-opacity-40 flex items-center justify-center">
                                <span class="text-xs text-white bg-black bg-opacity-60 px-2 py-1 rounded">Added</span>
                            </div>
                            {# Resource name tooltip #}
                            <div class="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/60 to-transparent p-2">
                                <p class="text-xs text-white truncate" x-text="resource.Name || 'Unnamed'"></p>
                            </div>
                        </div>
                    </template>
                </div>
            </div>

            {# Footer #}
            <div class="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
                <span class="text-sm text-gray-600">
                    <span x-text="$store.resourcePicker.selectionCount"></span> selected
                </span>
                <div class="flex gap-2">
                    <button @click="$store.resourcePicker.close()"
                            type="button"
                            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">
                        Cancel
                    </button>
                    <button @click="$store.resourcePicker.confirm()"
                            type="button"
                            :disabled="$store.resourcePicker.selectionCount === 0"
                            class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
                        Confirm
                    </button>
                </div>
            </div>
        </div>
    </div>
</div>
```

**Step 4: Verify the changes**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add templates/partials/blockEditor.tpl
git commit -m "feat(gallery): add resource picker modal template"
```

---

### Task 6: Manual Testing

**Step 1: Start the server**

Run: `cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8181`

**Step 2: Test the resource picker**

1. Navigate to a note with a gallery block (or create one)
2. Click "Edit Blocks" to enter edit mode
3. Click "+ Select Resources" button on the gallery block
4. Verify modal opens with tabs
5. Test "All Resources" tab - search and filter by tag/group
6. Test selecting resources (click thumbnails)
7. Test that already-added resources show "Added" badge
8. Click "Confirm" and verify resources are added to gallery
9. Test removing resources from the gallery preview

**Step 3: Test accessibility**

1. Verify modal can be closed with Escape key
2. Verify focus is trapped in modal
3. Verify keyboard navigation works in grid

---

### Task 7: Final Commit and Build

**Step 1: Build production assets**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: All builds succeed

**Step 2: Run tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./...`
Expected: All tests pass

**Step 3: Commit built assets**

```bash
git add public/dist/main.js
git commit -m "build: update production JS bundle with resource picker"
```
