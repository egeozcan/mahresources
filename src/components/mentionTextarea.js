import { abortableFetch } from '../index.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

/**
 * mentionTextarea - Alpine.js component that adds @-mention autocomplete to textareas.
 *
 * On `@` keypress (preceded by whitespace or at start of text), starts capturing a query.
 * After 2+ characters, calls `/v1/search?q={query}&types={allowedTypes}&limit=10` with debounce.
 * Shows a floating dropdown with results. Arrow keys navigate, Enter/click selects, Escape dismisses.
 * On selection, inserts `@[type:id:name]` at cursor position, replacing `@query`.
 *
 * @param {string} allowedTypes - Comma-separated string of entity types (e.g. "resource,group,tag")
 * @returns {object} Alpine data object
 */
export function mentionTextarea(allowedTypes = '') {
    return {
        mentionActive: false,
        mentionQuery: '',
        mentionResults: [],
        mentionSelectedIndex: 0,
        mentionLoading: false,
        mentionStart: -1,

        _requestAborter: null,
        _debounceTimer: null,
        _liveRegion: null,

        typeIcons: {
            resource: '\u{1F4C4}',
            note: '\u{1F4DD}',
            group: '\u{1F465}',
            tag: '\u{1F3F7}',
            category: '\u{1F4C1}',
        },

        typeLabels: {
            resource: 'Resource',
            note: 'Note',
            group: 'Group',
            tag: 'Tag',
            category: 'Category',
        },

        init() {
            this._liveRegion = createLiveRegion();
        },

        destroy() {
            this._liveRegion?.destroy();
            if (this._debounceTimer) {
                clearTimeout(this._debounceTimer);
            }
            if (this._requestAborter) {
                this._requestAborter();
                this._requestAborter = null;
            }
        },

        onKeydown(e) {
            if (!this.mentionActive || this.mentionResults.length === 0) return;

            if (e.key === 'ArrowDown') {
                e.preventDefault();
                this.mentionSelectedIndex = (this.mentionSelectedIndex + 1) % this.mentionResults.length;
                this._scrollToSelected();
                this._announceSelected();
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                this.mentionSelectedIndex = this.mentionSelectedIndex === 0
                    ? this.mentionResults.length - 1
                    : this.mentionSelectedIndex - 1;
                this._scrollToSelected();
                this._announceSelected();
            } else if (e.key === 'Enter') {
                e.preventDefault();
                this.selectMention(this.mentionResults[this.mentionSelectedIndex]);
            } else if (e.key === 'Escape') {
                e.preventDefault();
                this.closeMention();
            }
        },

        onInput(e) {
            const textarea = this.$refs.mentionInput;
            if (!textarea) return;

            const value = textarea.value;
            const cursorPos = textarea.selectionStart;

            // Find the last @ before cursor that is preceded by whitespace or at start
            let atPos = -1;
            for (let i = cursorPos - 1; i >= 0; i--) {
                if (value[i] === '@') {
                    // Check if preceded by whitespace or at start
                    if (i === 0 || /\s/.test(value[i - 1])) {
                        atPos = i;
                    }
                    break;
                }
                // Stop searching if we hit a newline (mention doesn't span lines)
                if (value[i] === '\n') {
                    break;
                }
            }

            if (atPos === -1) {
                if (this.mentionActive) {
                    this.closeMention();
                }
                return;
            }

            const query = value.substring(atPos + 1, cursorPos);

            // Close if query contains a newline
            if (query.includes('\n')) {
                if (this.mentionActive) {
                    this.closeMention();
                }
                return;
            }

            this.mentionStart = atPos;
            this.mentionQuery = query;
            this.mentionActive = true;

            if (query.length >= 2) {
                this._searchMentions(query);
            } else {
                this.mentionResults = [];
            }
        },

        selectMention(result) {
            if (!result) return;

            const textarea = this.$refs.mentionInput;
            if (!textarea) return;

            const value = textarea.value;
            const marker = `@[${result.type}:${result.id}:${result.name}]`;

            // Replace @query with the marker
            const before = value.substring(0, this.mentionStart);
            const after = value.substring(this.mentionStart + 1 + this.mentionQuery.length);
            const newValue = before + marker + after;

            textarea.value = newValue;

            // Set cursor position after the inserted marker
            const newCursorPos = this.mentionStart + marker.length;
            textarea.selectionStart = newCursorPos;
            textarea.selectionEnd = newCursorPos;

            // Trigger input event so Alpine's x-model picks up the change
            textarea.dispatchEvent(new Event('input', { bubbles: true }));

            this._liveRegion?.announce(`Inserted mention: ${result.name}`);

            this.closeMention();
            textarea.focus();
        },

        closeMention() {
            this.mentionActive = false;
            this.mentionQuery = '';
            this.mentionResults = [];
            this.mentionSelectedIndex = 0;
            this.mentionStart = -1;
            this.mentionLoading = false;

            if (this._debounceTimer) {
                clearTimeout(this._debounceTimer);
                this._debounceTimer = null;
            }
            if (this._requestAborter) {
                this._requestAborter();
                this._requestAborter = null;
            }
        },

        getDropdownStyle() {
            const textarea = this.$refs.mentionInput;
            if (!textarea) return '';

            // Position below the textarea
            const rect = textarea.getBoundingClientRect();
            const parentRect = textarea.offsetParent?.getBoundingClientRect() || { top: 0, left: 0 };

            return `position: absolute; top: ${textarea.offsetTop + textarea.offsetHeight}px; left: ${textarea.offsetLeft}px; z-index: 50; min-width: 300px; max-width: 400px;`;
        },

        get activeDescendantId() {
            if (!this.mentionActive || this.mentionResults.length === 0) return '';
            const r = this.mentionResults[this.mentionSelectedIndex];
            return r ? `mention-option-${r.type}-${r.id}` : '';
        },

        getIcon(type) {
            return this.typeIcons[type] || '\u{1F4CC}';
        },

        getLabel(type) {
            return this.typeLabels[type] || type;
        },

        highlightMatch(text, query) {
            if (!text || !query) return this.escapeHTML(text || '');
            const escaped = this.escapeHTML(text);
            const escapedQuery = this.escapeHTML(query);
            const regex = new RegExp(`(${this._escapeRegex(escapedQuery)})`, 'gi');
            return escaped.replace(regex, '<mark class="bg-yellow-200">$1</mark>');
        },

        escapeHTML(str) {
            if (!str) return '';
            const div = document.createElement('div');
            div.textContent = str;
            return div.innerHTML;
        },

        _escapeRegex(string) {
            return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        },

        _searchMentions(query) {
            if (this._debounceTimer) {
                clearTimeout(this._debounceTimer);
            }
            if (this._requestAborter) {
                this._requestAborter();
                this._requestAborter = null;
            }

            const debounceTime = query.length < 3 ? 300 : 150;

            this._debounceTimer = setTimeout(() => {
                this.mentionLoading = true;

                let url = `/v1/search?q=${encodeURIComponent(query)}&limit=10`;
                if (allowedTypes) {
                    url += `&types=${encodeURIComponent(allowedTypes)}`;
                }

                const { abort, ready } = abortableFetch(url);
                this._requestAborter = abort;

                ready.then(response => response.json())
                    .then(data => {
                        if (this.mentionQuery === query) {
                            this.mentionResults = data.results || [];
                            this.mentionSelectedIndex = 0;

                            if (this.mentionResults.length > 0) {
                                this._liveRegion?.announce(
                                    `${this.mentionResults.length} suggestion${this.mentionResults.length === 1 ? '' : 's'}. Use arrow keys to navigate.`
                                );
                            } else {
                                this._liveRegion?.announce('No suggestions found.');
                            }
                        }
                    })
                    .catch(err => {
                        if (err.name !== 'AbortError') {
                            console.error('Mention search error:', err);
                        }
                    })
                    .finally(() => {
                        this.mentionLoading = false;
                    });
            }, debounceTime);
        },

        _scrollToSelected() {
            this.$nextTick(() => {
                const dropdown = this.$refs.mentionDropdown;
                const selected = dropdown?.querySelector('[data-mention-selected="true"]');
                if (selected) {
                    selected.scrollIntoView({ block: 'nearest' });
                }
            });
        },

        _announceSelected() {
            const result = this.mentionResults[this.mentionSelectedIndex];
            if (result) {
                const typeLabel = this.getLabel(result.type);
                this._liveRegion?.announce(
                    `${result.name}, ${typeLabel}, ${this.mentionSelectedIndex + 1} of ${this.mentionResults.length}`
                );
            }
        },
    };
}
