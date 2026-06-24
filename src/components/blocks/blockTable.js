// src/components/blocks/blockTable.js
// editMode is passed as a getter function to maintain reactivity with parent scope

// Module-level cache for query results with stale-while-revalidate pattern
const queryCache = new Map();
const CACHE_TTL = 30000;       // 30s - data considered expired
const STALE_THRESHOLD = 10000; // 10s - trigger background refresh
const MAX_CACHE_SIZE = 50;     // LRU eviction limit

function getCacheKey(blockId, queryId, params) {
  return `${blockId}:${queryId}:${JSON.stringify(params || {})}`;
}

// Compare two table cell values for sorting. Null/undefined sort as empty (NOT
// as 0 — the old `|| ''` mis-bucketed the numeric value 0). When both values
// parse as finite numbers they compare numerically (so '100' sorts after '20');
// otherwise they compare as natural-ordered strings.
function compareCellValues(a, b) {
  const av = a == null ? '' : a;
  const bv = b == null ? '' : b;
  const an = typeof av === 'number' ? av : (av !== '' && isFinite(Number(av)) ? Number(av) : null);
  const bn = typeof bv === 'number' ? bv : (bv !== '' && isFinite(Number(bv)) ? Number(bv) : null);
  if (an !== null && bn !== null) return an - bn;
  return String(av).localeCompare(String(bv), undefined, { numeric: true });
}

function getCacheEntry(key) {
  return queryCache.get(key);
}

function setCacheEntry(key, data) {
  // LRU eviction if cache is full
  if (queryCache.size >= MAX_CACHE_SIZE) {
    const oldestKey = queryCache.keys().next().value;
    queryCache.delete(oldestKey);
  }
  queryCache.set(key, {
    data,
    timestamp: Date.now()
  });
}

function isCacheFresh(entry) {
  return entry && (Date.now() - entry.timestamp) < STALE_THRESHOLD;
}

function isCacheStale(entry) {
  return entry && (Date.now() - entry.timestamp) >= STALE_THRESHOLD && (Date.now() - entry.timestamp) < CACHE_TTL;
}

function isCacheExpired(entry) {
  return !entry || (Date.now() - entry.timestamp) >= CACHE_TTL;
}

export function blockTable(block, saveContentFn, saveStateFn, getEditMode) {
  return {
    block,
    saveContentFn,
    saveStateFn,
    getEditMode,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },

    // Manual mode data
    columns: JSON.parse(JSON.stringify(block?.content?.columns || [])),
    rows: JSON.parse(JSON.stringify(block?.content?.rows || [])),

    // Query mode configuration
    queryId: block?.content?.queryId || null,
    queryParams: JSON.parse(JSON.stringify(block?.content?.queryParams || {})),
    isStatic: block?.content?.isStatic || false,

    // Query mode data
    queryColumns: [],
    queryRows: [],
    queryLoading: false,
    queryError: null,
    isRefreshing: false,
    lastFetchTime: null,


    // Sorting state
    sortColumn: block?.state?.sortColumn || '',
    sortDirection: block?.state?.sortDirection || 'asc',

    // Computed: whether we're in query mode
    get isQueryMode() {
      return this.queryId != null;
    },

    // Normalize a column: if it's a plain string, convert to {id, label} object
    _normalizeColumns(cols) {
      if (!cols || !cols.length) return cols;
      return cols.map((col, idx) =>
        typeof col === 'string' ? { id: `col_${idx}`, label: col } : col
      );
    },

    // Normalize rows: if they are arrays, convert to objects using normalized column IDs
    _normalizeRows(rows, normalizedCols) {
      if (!rows || !rows.length) return rows;
      // If the first row is an array, all rows are arrays
      if (!Array.isArray(rows[0])) return rows;
      return rows.map((row, rowIdx) => {
        const obj = { id: `row_${rowIdx}` };
        normalizedCols.forEach((col, colIdx) => {
          obj[col.id] = row[colIdx] !== undefined ? row[colIdx] : '';
        });
        return obj;
      });
    },

    // Computed: which columns to display
    get displayColumns() {
      const raw = this.isQueryMode ? this.queryColumns : this.columns;
      return this._normalizeColumns(raw);
    },

    // Computed: which rows to display (sorted)
    get displayRows() {
      const rawRows = this.isQueryMode ? this.queryRows : this.rows;
      const cols = this.displayColumns;
      const rows = this._normalizeRows(rawRows, cols);
      if (!this.sortColumn) return rows;

      const col = cols.find(c => c.id === this.sortColumn);
      if (!col) return rows;

      return [...rows].sort((a, b) => {
        const cmp = compareCellValues(a[this.sortColumn], b[this.sortColumn]);
        return this.sortDirection === 'asc' ? cmp : -cmp;
      });
    },

    // Legacy getter for backwards compatibility
    get sortedRows() {
      return this.displayRows;
    },

    // Initialize: fetch query data if in query mode
    init() {
      if (this.isQueryMode) {
        this.fetchQueryData();
      }
    },

    toggleSort(colId) {
      this.sortDirection = this.sortColumn === colId && this.sortDirection === 'asc' ? 'desc' : 'asc';
      this.sortColumn = colId;
      this.saveStateFn(this.block.id, { sortColumn: this.sortColumn, sortDirection: this.sortDirection });
    },

    saveContent() {
      if (this.isQueryMode) {
        this.saveContentFn(this.block.id, {
          queryId: this.queryId,
          queryParams: this.queryParams,
          isStatic: this.isStatic
        });
      } else {
        this.saveContentFn(this.block.id, { columns: this.columns, rows: this.rows });
      }
    },

    // Fetch query data with stale-while-revalidate caching
    async fetchQueryData(forceRefresh = false) {
      if (!this.queryId) return;

      const cacheKey = getCacheKey(this.block.id, this.queryId, this.queryParams);
      const cacheEntry = getCacheEntry(cacheKey);

      // Check cache state
      if (!forceRefresh) {
        if (isCacheFresh(cacheEntry)) {
          // Fresh cache hit - use cached data
          this.applyQueryData(cacheEntry.data);
          return;
        }

        if (isCacheStale(cacheEntry) && !this.isStatic) {
          // Stale cache - show cached data immediately, refresh in background
          this.applyQueryData(cacheEntry.data);
          this.backgroundRefresh(cacheKey);
          return;
        }

        if (isCacheStale(cacheEntry) && this.isStatic) {
          // Static mode with stale cache - just use cached data, no auto-refresh
          this.applyQueryData(cacheEntry.data);
          return;
        }
      }

      // Cache miss or expired - fetch blocking
      this.queryLoading = true;
      this.queryError = null;

      try {
        const data = await this.fetchFromServer();
        setCacheEntry(cacheKey, data);
        this.applyQueryData(data);
      } catch (err) {
        this.queryError = err.message || 'Failed to load query data';
        console.error('Table block query fetch error:', err);
      } finally {
        this.queryLoading = false;
      }
    },

    // Background refresh for stale-while-revalidate
    async backgroundRefresh(cacheKey) {
      if (this.isRefreshing) return;

      this.isRefreshing = true;
      try {
        const data = await this.fetchFromServer();
        setCacheEntry(cacheKey, data);
        this.applyQueryData(data);
      } catch (err) {
        console.error('Background refresh failed:', err);
        // Don't show error for background refresh failures
      } finally {
        this.isRefreshing = false;
      }
    },

    // Fetch data from the server
    async fetchFromServer() {
      const params = new URLSearchParams({ blockId: this.block.id });
      // Add query params to URL
      for (const [key, value] of Object.entries(this.queryParams || {})) {
        params.append(key, value);
      }

      const response = await fetch(`/v1/note/block/table/query?${params}`);
      if (!response.ok) {
        if (response.status === 404) {
          throw new Error('Query unavailable');
        }
        const errData = await response.json().catch(() => ({}));
        throw new Error(errData.error || `HTTP ${response.status}`);
      }
      return response.json();
    },

    // Apply query data to component state
    applyQueryData(data) {
      this.queryColumns = data.columns || [];
      this.queryRows = data.rows || [];
      this.lastFetchTime = data.cachedAt ? new Date(data.cachedAt) : new Date();
    },

    // Manual refresh button handler
    async manualRefresh() {
      await this.fetchQueryData(true);
    },

    // Format last fetch time for display
    get lastFetchTimeFormatted() {
      if (!this.lastFetchTime) return '';
      const now = new Date();
      const diff = Math.floor((now - this.lastFetchTime) / 1000);
      if (diff < 60) return 'just now';
      if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
      return this.lastFetchTime.toLocaleTimeString();
    },

    // --- Query Selection Methods ---

    // Selected query name for display (stored when query is selected)
    selectedQueryName: block?.content?.queryId ? null : null, // Will be set on select

    selectQuery(query) {
      this.queryId = query.ID || query.id;
      this.selectedQueryName = query.Name || query.name;
      this.queryParams = {};
      this.isStatic = false;
      // Clear manual data
      this.columns = [];
      this.rows = [];
      this.saveContent();
      // Fetch the query data
      this.fetchQueryData(true);
    },

    clearQuery() {
      this.queryId = null;
      this.queryParams = {};
      this.isStatic = false;
      this.queryColumns = [];
      this.queryRows = [];
      this.queryError = null;
      this.lastFetchTime = null;
      // Initialize with empty manual data
      this.columns = [];
      this.rows = [];
      this.saveContent();
    },

    toggleStatic() {
      this.isStatic = !this.isStatic;
      this.saveContent();
    },

    updateQueryParam(key, value) {
      if (value === '' || value === null || value === undefined) {
        delete this.queryParams[key];
      } else {
        this.queryParams[key] = value;
      }
      this.queryParams = { ...this.queryParams }; // trigger reactivity
      this.saveContent();
      // Refresh data with new params
      this.fetchQueryData(true);
    },

    // Rename a query param's key while preserving its value and order. Editing
    // the key is what makes a freshly-added param usable (its name must match the
    // placeholder the saved query expects).
    renameQueryParam(oldKey, newKey) {
      newKey = (newKey || '').trim();
      if (newKey === oldKey) return;
      const value = this.queryParams[oldKey];
      const next = {};
      for (const [k, v] of Object.entries(this.queryParams)) {
        if (k === oldKey) {
          if (newKey) next[newKey] = value; // drop the entry entirely if cleared
        } else if (k !== newKey) {
          next[k] = v;
        }
      }
      this.queryParams = next;
      this.saveContent();
      this.fetchQueryData(true);
    },

    removeQueryParam(key) {
      delete this.queryParams[key];
      this.queryParams = { ...this.queryParams }; // trigger reactivity
      this.saveContent();
      this.fetchQueryData(true);
    },

    addQueryParam() {
      const key = `param_${Object.keys(this.queryParams).length + 1}`;
      this.queryParams[key] = '';
      this.queryParams = { ...this.queryParams }; // trigger reactivity
    },

    // --- Manual Mode Methods ---

    addColumn() {
      const newCol = { id: crypto.randomUUID(), label: 'New Column' };
      this.columns = [...this.columns, newCol];
      this.saveContent();
    },

    removeColumn(idx) {
      const removedCol = this.columns[idx];
      this.columns = this.columns.filter((_, i) => i !== idx);
      // Also remove the column data from rows
      if (removedCol) {
        this.rows = this.rows.map(row => {
          const newRow = { ...row };
          delete newRow[removedCol.id];
          return newRow;
        });
      }
      this.saveContent();
    },

    addRow() {
      const newRow = { id: crypto.randomUUID() };
      this.rows = [...this.rows, newRow];
      this.saveContent();
    },

    removeRow(idx) {
      this.rows = this.rows.filter((_, i) => i !== idx);
      this.saveContent();
    }
  };
}
