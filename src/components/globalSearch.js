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
        // Package 5b: when the typed query is a valid MRQL query, a pinned
        // "Run MRQL query" row is shown above the search results. It derives
        // from validation state, never from the /v1/search cache.
        mrqlRow: null,
        _mrqlTimer: null,

        typeIcons: {
            resource: '\u{1F4C4}',
            note: '\u{1F4DD}',
            group: '\u{1F465}',
            tag: '\u{1F3F7}',
            category: '\u{1F4C1}',
            resourceCategory: '\u{1F4C2}',
            query: '\u{1F50D}',
            relationType: '\u{1F517}',
            noteType: '\u{1F4CB}',
            mrqlQuery: '\u{1F4CA}',
            mrql: '\u{25B6}\u{FE0F}'
        },

        typeLabels: {
            resource: 'Resource',
            note: 'Note',
            group: 'Group',
            tag: 'Tag',
            category: 'Category',
            resourceCategory: 'Resource Category',
            query: 'Query',
            relationType: 'Relation Type',
            noteType: 'Note Type',
            mrqlQuery: 'Saved Query',
            mrql: 'MRQL'
        },

        // navResults is the navigable listbox contents: the pinned MRQL action
        // row (when present) followed by the search results. Rendering and
        // arrow-key navigation operate over this; the search cache holds only
        // `results`.
        get navResults() {
            return this.mrqlRow ? [this.mrqlRow, ...this.results] : this.results;
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
            if (this._mrqlTimer) {
                clearTimeout(this._mrqlTimer);
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
                this.mrqlRow = null;
                this.selectedIndex = 0;
            }
        },

        close() {
            this.isOpen = false;
            this.query = '';
            this.results = [];
            this.mrqlRow = null;
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

            // Package 5b: evaluate the MRQL interpretation alongside the search.
            this.evaluateMRQL(searchTerm);

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
                            // Cache non-empty results only. Caching an empty
                            // response would poison this term for the full TTL,
                            // leaving the user stuck on "No results found" even
                            // after the data exists (e.g. a just-created item or
                            // a transient backend hiccup).
                            if (this.results.length > 0) {
                                setCachedResults(searchTerm, this.results);
                            }
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
            const items = this.navResults;
            if (items.length === 0) return;
            this.selectedIndex = this.selectedIndex === 0
                ? items.length - 1
                : this.selectedIndex - 1;
            this.scrollToSelected();
            this.announceSelectedResult();
        },

        navigateDown() {
            const items = this.navResults;
            if (items.length === 0) return;
            this.selectedIndex = (this.selectedIndex + 1) % items.length;
            this.scrollToSelected();
            this.announceSelectedResult();
        },

        announceSelectedResult() {
            const items = this.navResults;
            const result = items[this.selectedIndex];
            if (result) {
                const typeLabel = this.getLabel(result.type);
                this.announce(`${result.name}, ${typeLabel}, ${this.selectedIndex + 1} of ${items.length}`);
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
            const result = this.navResults[this.selectedIndex];
            if (result) {
                this.navigateTo(result.url);
            }
        },

        // evaluateMRQL gates on a cheap heuristic (so ordinary search terms
        // never trigger a request), then debounced-validates the full grammar.
        // Only a valid query pins the "Run MRQL query" row; anything else clears
        // it silently (no error noise in the search modal).
        evaluateMRQL(term) {
            if (this._mrqlTimer) {
                clearTimeout(this._mrqlTimer);
                this._mrqlTimer = null;
            }
            if (!this.looksLikeMRQL(term)) {
                this.mrqlRow = null;
                return;
            }
            this._mrqlTimer = setTimeout(() => {
                fetch('/v1/mrql/validate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ query: term }),
                })
                    .then((resp) => (resp.ok ? resp.json() : null))
                    .then((data) => {
                        // Ignore stale responses (the query moved on).
                        if (this.query.trim() !== term) return;
                        if (data && data.valid) {
                            const wasAbsent = !this.mrqlRow;
                            this.mrqlRow = {
                                type: 'mrql',
                                id: 0,
                                name: 'Run MRQL query',
                                description: term,
                                url: '/mrql?q=' + encodeURIComponent(term),
                            };
                            if (wasAbsent) {
                                this.announce('Run MRQL query action available. It is the first result.');
                            }
                        } else {
                            this.mrqlRow = null;
                        }
                    })
                    .catch(() => {
                        // Network/parse error — leave no MRQL row.
                    });
            }, 200);
        },

        // looksLikeMRQL is the pre-network gate: the input plausibly is a query
        // (comparison operator, an IS/IN/EMPTY/SIMILAR keyword, or a leading
        // `type `). Ordinary search terms never match.
        looksLikeMRQL(term) {
            if (!term || term.length < 2) return false;
            return /[=~<>]/.test(term)
                || /\b(IS|IN|EMPTY|SIMILAR)\b/i.test(term)
                || /^type\s/i.test(term);
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
