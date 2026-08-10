import { createLiveRegion } from '../utils/ariaLiveRegion.js';

/**
 * The /downloads page: row selection, and the retry/delete calls behind the
 * per-row and bulk buttons.
 *
 * Selection is local rather than the global `bulkSelection` store. That store is
 * shared by the entity list pages and carries entity semantics (its ids feed the
 * bulk tag/meta/delete editors), so a downloads row landing in it would offer
 * "Add tags" for something that is not an entity.
 *
 * Every action reloads the page afterwards. The table is server-rendered and a
 * retry changes a row's status, its attempt count and its position, so patching
 * the DOM by hand would be a second rendering of the same rules — and the one
 * that drifts. A reload is also what makes a deleted row's disappearance and the
 * count in the header agree.
 */
export function downloadsManager() {
    return {
        selected: new Set(),
        busy: false,
        error: '',
        _liveRegion: null,

        notice: '',
        _root: null,

        init() {
            this._liveRegion = createLiveRegion();
            // Captured here, where $el is the component root. Inside a method $el is
            // whichever element called it — for toggleAll that is the header
            // checkbox, whose subtree holds no rows, so the select-all silently
            // selected nothing.
            this._root = this.$el;

            // The outcome of the action that caused the reload. Without this the
            // result of pressing Retry is a page that looks the same apart from one
            // status pill, and for a screen-reader user it is nothing at all — the
            // announcement was torn down with the document that made it.
            try {
                const flash = sessionStorage.getItem('downloads-flash');
                if (flash) {
                    sessionStorage.removeItem('downloads-flash');
                    this.notice = flash;
                    this.$nextTick(() => this.announce(flash));
                }
            } catch {
                // sessionStorage throws in sandboxed contexts; there is simply no flash.
            }
        },

        destroy() {
            this._liveRegion?.destroy();
        },

        announce(message) {
            this._liveRegion?.announce(message);
        },

        isSelected(id) {
            return this.selected.has(id);
        },

        toggle(id, checked) {
            if (checked) {
                this.selected.add(id);
            } else {
                this.selected.delete(id);
            }
            // Set mutations are not reactive on their own; reassigning is what tells
            // Alpine the derived counts changed.
            this.selected = new Set(this.selected);
        },

        toggleAll(checked) {
            const ids = this.rowIds();
            this.selected = checked ? new Set(ids) : new Set();
        },

        /** The ids of the rows currently rendered, in DOM order. */
        rowIds() {
            const root = this._root ?? this.$el;
            return Array.from(root.querySelectorAll('[data-testid="downloads-row-checkbox"]'))
                .map(cb => Number(cb.value))
                .filter(id => Number.isFinite(id) && id > 0);
        },

        get selectedCount() {
            return this.selected.size;
        },

        get allSelected() {
            const ids = this.rowIds();
            return ids.length > 0 && ids.every(id => this.selected.has(id));
        },

        retryOne(id) {
            return this.send('/v1/downloads/retry', [id], 'retried');
        },

        deleteOne(id) {
            return this.send('/v1/downloads/delete', [id], 'deleted');
        },

        retrySelected() {
            return this.send('/v1/downloads/retry', [...this.selected], 'retried');
        },

        deleteSelected() {
            return this.send('/v1/downloads/delete', [...this.selected], 'deleted');
        },

        /**
         * Run one action and report what happened.
         *
         * The response carries an outcome per id, because a batch can be partly
         * refused — a completed download cannot be retried, a running one cannot be
         * deleted, and the queue can fill up part-way through. Reporting only the
         * count would leave the user to work out which rows were skipped by reading
         * the reloaded table. A batch refused *entirely* answers 409 and carries the
         * same outcomes, so its distinct reasons are reported too rather than the
         * summary sentence alone.
         */
        async send(path, ids, verb) {
            if (this.busy || ids.length === 0) return;
            this.busy = true;
            this.error = '';

            try {
                const res = await fetch(path, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                    body: JSON.stringify({ ids }),
                });
                const payload = await res.json().catch(() => null);

                if (!res.ok) {
                    this.error = payload?.error || `Could not ${verb.replace(/ed$/, '')} the selected downloads.`;
                    // A wholly refused batch still answers per id, and each id can be
                    // refused for its own reason — "this one is running, that one
                    // already succeeded". Showing only the summary threw those away,
                    // which is exactly what the server stopped doing.
                    const reasons = [...new Set((payload?.results || [])
                        .filter(r => !r.ok && r.reason)
                        .map(r => r.reason))];
                    if (reasons.length > 0) {
                        this.error = reasons.join(' ');
                    }
                    this.announce(this.error);
                    return;
                }

                const done = payload?.[verb] ?? ids.length;
                const refused = (payload?.results || []).filter(r => !r.ok);
                let message = `${done} download${done === 1 ? '' : 's'} ${verb}.`;
                if (refused.length > 0) {
                    // The first reason rather than all of them: they are nearly always
                    // the same reason repeated, and the reloaded table shows the rest.
                    message += ` ${refused.length} skipped — ${refused[0].reason || 'not eligible'}.`;
                }
                this.announce(message);
                // Kept in sessionStorage across the reload, so the outcome is still on
                // screen after the page comes back rather than vanishing with it.
                try {
                    sessionStorage.setItem('downloads-flash', message);
                } catch {
                    // Private mode / sandboxed contexts throw on write; the announcement
                    // has already been made, so there is nothing to recover.
                }
                window.location.reload();
            } catch (err) {
                console.error(`${path} failed:`, err);
                this.error = 'The request failed. Check your connection and try again.';
                this.announce(this.error);
            } finally {
                this.busy = false;
            }
        },
    };
}
