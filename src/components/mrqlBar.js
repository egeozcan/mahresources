import { createLiveRegion } from '../utils/ariaLiveRegion.js';

// The word currently being typed: a run of field-name characters ending at the
// cursor. Used to filter suggestions and to compute the replacement range when
// applying one (mirrors the CodeMirror completer's matchBefore on /mrql).
const WORD_RE = /[a-zA-Z_.]*$/;

// mrqlBar is the list-page MRQL filter bar (package 5). A plain <input> (not
// CodeMirror — the list pages are hot and must not pull the editor chunks) with
// server-driven autocomplete and validation in filter mode, wired as an ARIA
// combobox. Submitting navigates the surrounding list to ?mrql=<expr>.
export function mrqlBar({ entity = 'resource', value = '', error = '' } = {}) {
    return {
        entity,
        query: value,
        // error holds the current inline validation message, seeded from the
        // server fail-closed banner and replaced by client-side validation.
        error,
        suggestions: [],
        selectedIndex: -1,
        open: false,
        _completeTimer: null,
        _validateTimer: null,
        _liveRegion: null,

        init() {
            this._liveRegion = createLiveRegion();
        },

        destroy() {
            this._liveRegion?.destroy();
            clearTimeout(this._completeTimer);
            clearTimeout(this._validateTimer);
        },

        onInput() {
            this.scheduleComplete();
            this.scheduleValidate();
        },

        scheduleComplete() {
            clearTimeout(this._completeTimer);
            this._completeTimer = setTimeout(() => this.fetchSuggestions(), 150);
        },

        scheduleValidate() {
            clearTimeout(this._validateTimer);
            this._validateTimer = setTimeout(() => this.validate(), 500);
        },

        cursorPos() {
            return this.$refs.input ? this.$refs.input.selectionStart : this.query.length;
        },

        currentWord(cursor) {
            const m = this.query.slice(0, cursor).match(WORD_RE);
            return m ? m[0] : '';
        },

        async fetchSuggestions() {
            const cursor = this.cursorPos();
            try {
                const resp = await fetch('/v1/mrql/complete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ query: this.query, cursor, entityType: this.entity, filter: true }),
                });
                if (!resp.ok) { this.closeSuggestions(); return; }
                const data = await resp.json();
                let sugg = data.suggestions || [];
                // Narrow to the token currently being typed (the server returns
                // the full candidate list; the client filters, as /mrql does).
                const word = this.currentWord(cursor);
                if (word) {
                    const lw = word.toLowerCase();
                    sugg = sugg.filter((s) => s.value.toLowerCase().startsWith(lw));
                }
                this.suggestions = sugg.slice(0, 20);
                this.open = this.suggestions.length > 0;
                this.selectedIndex = this.open ? 0 : -1;
                if (this.open) {
                    this._liveRegion?.announce(
                        `${this.suggestions.length} suggestion${this.suggestions.length === 1 ? '' : 's'} available.`);
                }
            } catch (_) {
                this.closeSuggestions();
            }
        },

        async validate() {
            const query = this.query.trim();
            if (!query) { this.error = ''; return; }
            try {
                const resp = await fetch('/v1/mrql/validate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ query, entityType: this.entity, filter: true }),
                });
                if (!resp.ok) return;
                const data = await resp.json();
                if (data.valid) {
                    this.error = '';
                } else if (data.errors && data.errors.length > 0) {
                    this.error = data.errors.map((e) => e.message || 'Invalid filter').join('; ');
                } else {
                    this.error = 'Invalid filter';
                }
            } catch (_) {
                // Network error — leave the last known state untouched.
            }
        },

        navigateDown() {
            if (this.suggestions.length === 0) return;
            this.open = true;
            this.selectedIndex = (this.selectedIndex + 1) % this.suggestions.length;
            this.announceOption();
        },

        navigateUp() {
            if (this.suggestions.length === 0 || !this.open) return;
            this.selectedIndex = this.selectedIndex <= 0
                ? this.suggestions.length - 1
                : this.selectedIndex - 1;
            this.announceOption();
        },

        announceOption() {
            const s = this.suggestions[this.selectedIndex];
            if (s) {
                this._liveRegion?.announce(
                    `${s.value}${s.label ? ', ' + s.label : ''}, ${this.selectedIndex + 1} of ${this.suggestions.length}`);
            }
        },

        applySuggestion(i) {
            const s = this.suggestions[i];
            if (!s) return;
            const cursor = this.cursorPos();
            const before = this.query.slice(0, cursor);
            const wordMatch = before.match(WORD_RE);
            const start = cursor - (wordMatch ? wordMatch[0].length : 0);
            const after = this.query.slice(cursor);
            this.query = this.query.slice(0, start) + s.value + after;
            this.closeSuggestions();
            this.$nextTick(() => {
                const pos = start + s.value.length;
                if (this.$refs.input) {
                    this.$refs.input.focus();
                    this.$refs.input.setSelectionRange(pos, pos);
                }
                this.scheduleValidate();
            });
        },

        onEnter(e) {
            // With the suggestion popup open and an option highlighted, Enter
            // accepts it; otherwise it falls through to submit the GET form.
            if (this.open && this.selectedIndex >= 0) {
                e.preventDefault();
                this.applySuggestion(this.selectedIndex);
            }
        },

        onBlur() {
            // Delay so a mousedown on an option registers before the list hides.
            setTimeout(() => this.closeSuggestions(), 150);
        },

        closeSuggestions() {
            this.open = false;
            this.selectedIndex = -1;
            this.suggestions = [];
        },

        // submit navigates the current list to ?mrql=<expr>, preserving every
        // existing sidebar parameter and resetting to page 1. Clearing the input
        // and submitting removes the parameter.
        submit() {
            const params = new URLSearchParams(window.location.search);
            const val = this.query.trim();
            if (val) {
                params.set('mrql', val);
            } else {
                params.delete('mrql');
            }
            params.delete('page');
            const qs = params.toString();
            window.location.assign(window.location.pathname + (qs ? '?' + qs : ''));
        },

        // editorLink graduates the current filter to the full /mrql editor by
        // wrapping it with the page's implied entity type.
        editorLink() {
            const val = this.query.trim();
            const inner = val ? `type = ${this.entity} AND (${val})` : `type = ${this.entity}`;
            return '/mrql?q=' + encodeURIComponent(inner);
        },

        activeDescendant() {
            return this.open && this.selectedIndex >= 0
                ? `${this.$id('mrql-bar')}-opt-${this.selectedIndex}`
                : null;
        },
    };
}
