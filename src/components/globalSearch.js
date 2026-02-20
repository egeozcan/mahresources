import { abortableFetch } from '../index.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

// Client-side search cache with TTL
const searchCache = new Map();
const CACHE_TTL = 30000;  // 30 seconds
const MAX_CACHE_SIZE = 50;

function getCachedResults(query) {
    const key = query.toLowerCase();
    const entry = searchCache.get(key);
    if (entry && Date.now() - entry.timestamp < CACHE_TTL) {
        // Move to end (most recently used) by re-inserting
        searchCache.delete(key);
        searchCache.set(key, entry);
        return entry.results;
    }
    // Clean up expired entry
    if (entry) {
        searchCache.delete(key);
    }
    return null;
}

function setCachedResults(query, results) {
    const key = query.toLowerCase();
    // Evict oldest entries if at capacity
    if (searchCache.size >= MAX_CACHE_SIZE) {
        const oldestKey = searchCache.keys().next().value;
        searchCache.delete(oldestKey);
    }
    searchCache.set(key, { results, timestamp: Date.now() });
}

export function globalSearch() {
    return {
        isOpen: false,
        query: '',
        results: [],
        selectedIndex: 0,
        loading: false,
        requestAborter: null,
        debounceTimer: null,
        _liveRegion: null,

        typeIcons: {
            resource: '\u{1F4C4}',
            note: '\u{1F4DD}',
            group: '\u{1F465}',
            tag: '\u{1F3F7}',
            category: '\u{1F4C1}',
            query: '\u{1F50D}',
            relationType: '\u{1F517}',
            noteType: '\u{1F4CB}'
        },

        typeLabels: {
            resource: 'Resource',
            note: 'Note',
            group: 'Group',
            tag: 'Tag',
            category: 'Category',
            query: 'Query',
            relationType: 'Relation Type',
            noteType: 'Note Type'
        },

        init() {
            this._liveRegion = createLiveRegion();

            this._keydownHandler = (e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
                    e.preventDefault();
                    this.toggle();
                }
            };
            document.addEventListener('keydown', this._keydownHandler);

            this.$watch('isOpen', (value) => {
                if (value) {
                    this.$nextTick(() => {
                        this.$refs.searchInput?.focus();
                    });
                    this.announce('Search dialog opened. Type to search resources, notes, groups, and tags.');
                }
            });
        },

        announce(message) {
            this._liveRegion?.announce(message);
        },

        destroy() {
            this._liveRegion?.destroy();
            if (this._keydownHandler) {
                document.removeEventListener('keydown', this._keydownHandler);
            }
            if (this.debounceTimer) {
                clearTimeout(this.debounceTimer);
            }
            if (this.requestAborter) {
                this.requestAborter();
                this.requestAborter = null;
            }
        },

        toggle() {
            this.isOpen = !this.isOpen;
            if (this.isOpen) {
                this.query = '';
                this.results = [];
                this.selectedIndex = 0;
            }
        },

        close() {
            this.isOpen = false;
            this.query = '';
            this.results = [];
        },

        search() {
            if (this.debounceTimer) {
                clearTimeout(this.debounceTimer);
            }

            if (this.requestAborter) {
                this.requestAborter();
                this.requestAborter = null;
            }

            const searchTerm = this.query.trim();

            // Require at least 2 characters to search
            if (searchTerm.length < 2) {
                this.results = [];
                return;
            }

            // Check client-side cache first
            const cached = getCachedResults(searchTerm);
            if (cached) {
                this.results = cached;
                this.selectedIndex = 0;
                if (this.results.length > 0) {
                    this.announce(`${this.results.length} result${this.results.length === 1 ? '' : 's'} found. Use arrow keys to navigate.`);
                } else {
                    this.announce('No results found.');
                }
                return;
            }

            // Adaptive debounce: longer delay for short queries
            const debounceTime = searchTerm.length < 3 ? 300 : 150;

            this.debounceTimer = setTimeout(() => {
                this.loading = true;

                const { abort, ready } = abortableFetch(
                    `/v1/search?q=${encodeURIComponent(searchTerm)}&limit=15`
                );
                this.requestAborter = abort;

                ready.then(response => response.json())
                    .then(data => {
                        if (this.query.trim() === searchTerm) {
                            this.results = data.results || [];
                            this.selectedIndex = 0;
                            // Cache the results
                            setCachedResults(searchTerm, this.results);
                            // Announce results for screen readers
                            if (this.results.length > 0) {
                                this.announce(`${this.results.length} result${this.results.length === 1 ? '' : 's'} found. Use arrow keys to navigate.`);
                            } else {
                                this.announce('No results found.');
                            }
                        }
                    })
                    .catch(err => {
                        if (err.name !== 'AbortError') {
                            console.error('Search error:', err);
                        }
                    })
                    .finally(() => {
                        this.loading = false;
                    });
            }, debounceTime);
        },

        navigateUp() {
            if (this.results.length === 0) return;
            this.selectedIndex = this.selectedIndex === 0
                ? this.results.length - 1
                : this.selectedIndex - 1;
            this.scrollToSelected();
            this.announceSelectedResult();
        },

        navigateDown() {
            if (this.results.length === 0) return;
            this.selectedIndex = (this.selectedIndex + 1) % this.results.length;
            this.scrollToSelected();
            this.announceSelectedResult();
        },

        announceSelectedResult() {
            const result = this.results[this.selectedIndex];
            if (result) {
                const typeLabel = this.getLabel(result.type);
                this.announce(`${result.name}, ${typeLabel}, ${this.selectedIndex + 1} of ${this.results.length}`);
            }
        },

        scrollToSelected() {
            this.$nextTick(() => {
                const container = this.$refs.resultsList;
                const selected = container?.querySelector('[data-selected="true"]');
                if (selected) {
                    selected.scrollIntoView({ block: 'nearest' });
                }
            });
        },

        selectResult() {
            const result = this.results[this.selectedIndex];
            if (result) {
                this.navigateTo(result.url);
            }
        },

        navigateTo(url) {
            // Only allow relative URLs or same-origin to prevent open redirect
            if (url && (url.startsWith('/') || url.startsWith('?') || url.startsWith('#'))) {
                this.close();
                window.location.href = url;
            } else {
                try {
                    const parsed = new URL(url, window.location.origin);
                    if (parsed.origin === window.location.origin) {
                        this.close();
                        window.location.href = url;
                    }
                } catch {
                    // Invalid URL, ignore
                }
            }
        },

        getIcon(type) {
            return this.typeIcons[type] || '\u{1F4CC}';
        },

        getLabel(type) {
            return this.typeLabels[type] || type;
        },

        highlightMatch(text, query) {
            if (!text || !query) return text;
            const escaped = this.escapeHTML(text);
            const escapedQuery = this.escapeHTML(query);
            const regex = new RegExp(`(${this.escapeRegex(escapedQuery)})`, 'gi');
            return escaped.replace(regex, '<mark class="bg-yellow-200">$1</mark>');
        },

        escapeHTML(str) {
            const div = document.createElement('div');
            div.textContent = str;
            return div.innerHTML;
        },

        escapeRegex(string) {
            return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        }
    }
}
