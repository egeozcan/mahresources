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
      // Clean up pending debounce timer
      if (this.searchDebounceTimer) {
        clearTimeout(this.searchDebounceTimer);
        this.searchDebounceTimer = null;
      }
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

      const maxResults = this.config.maxResults || 50;
      const params = this.config.searchParams(this.searchQuery.trim(), this.filterValues, maxResults);
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
