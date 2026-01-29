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
