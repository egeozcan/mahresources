import { abortableFetch } from '../index.js';

export function globalSearch() {
    return {
        isOpen: false,
        query: '',
        results: [],
        selectedIndex: 0,
        loading: false,
        requestAborter: null,
        debounceTimer: null,

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
                }
            });
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

            if (searchTerm.length < 1) {
                this.results = [];
                return;
            }

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
            }, 150);
        },

        navigateUp() {
            if (this.results.length === 0) return;
            this.selectedIndex = this.selectedIndex === 0
                ? this.results.length - 1
                : this.selectedIndex - 1;
            this.scrollToSelected();
        },

        navigateDown() {
            if (this.results.length === 0) return;
            this.selectedIndex = (this.selectedIndex + 1) % this.results.length;
            this.scrollToSelected();
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
