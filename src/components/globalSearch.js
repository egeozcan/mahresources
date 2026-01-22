import { abortableFetch } from '../index.js';

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
        liveRegion: null,
        announceTimeout: null,

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
            // Create ARIA live region for screen reader announcements
            this.liveRegion = document.createElement('div');
            this.liveRegion.setAttribute('role', 'status');
            this.liveRegion.setAttribute('aria-live', 'polite');
            this.liveRegion.setAttribute('aria-atomic', 'true');
            Object.assign(this.liveRegion.style, {
                position: 'absolute',
                width: '1px',
                height: '1px',
                padding: '0',
                margin: '-1px',
                overflow: 'hidden',
                clip: 'rect(0, 0, 0, 0)',
                whiteSpace: 'nowrap',
                border: '0'
            });
            document.body.appendChild(this.liveRegion);

            document.addEventListener('keydown', (e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
                    e.preventDefault();
                    this.toggle();
                }
            });

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
            if (this.liveRegion) {
                // Cancel any pending announcement to avoid race conditions
                if (this.announceTimeout) {
                    clearTimeout(this.announceTimeout);
                }
                this.liveRegion.textContent = '';
                // Small delay to ensure screen readers pick up the change
                this.announceTimeout = setTimeout(() => {
                    this.liveRegion.textContent = message;
                }, 50);
            }
        },

        destroy() {
            // Clean up live region when component is destroyed
            if (this.liveRegion && this.liveRegion.parentNode) {
                this.liveRegion.parentNode.removeChild(this.liveRegion);
            }
            if (this.announceTimeout) {
                clearTimeout(this.announceTimeout);
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
            this.close();
            window.location.href = url;
        },

        getIcon(type) {
            return this.typeIcons[type] || '\u{1F4CC}';
        },

        getLabel(type) {
            return this.typeLabels[type] || type;
        },

        highlightMatch(text, query) {
            if (!text || !query) return text;
            const regex = new RegExp(`(${this.escapeRegex(query)})`, 'gi');
            return text.replace(regex, '<mark class="bg-yellow-200">$1</mark>');
        },

        escapeRegex(string) {
            return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        }
    }
}
