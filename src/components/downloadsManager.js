import { createLiveRegion } from '../utils/ariaLiveRegion.js';

/**
 * The /downloads page's actions: retry and delete, per card and in bulk.
 *
 * A store rather than an `x-data` component, because the two surfaces that call
 * it cannot be inside one component: the bulk toolbar renders in the template's
 * `prebody` block and the cards in `body`, and a Pongo2 block boundary is not
 * something an `x-data` subtree can span.
 *
 * Selection is not held here at all. It is the shared `bulkSelection` store, the
 * same one every entity list page uses, so Select All / Deselect All / shift-range
 * selection behave on this page exactly as they do on /notes. This used to be a
 * local Set, justified on the grounds that the shared store "carries entity
 * semantics" and a downloads row landing in it would offer "Add tags". It does
 * not: the store holds ids and nothing else, and the bulk *editors* that give
 * those ids meaning are a per-page include.
 *
 * Every action reloads the page afterwards. The list is server-rendered and a
 * retry changes a row's status, its attempt count and its position, so patching
 * the DOM by hand would be a second rendering of the same rules — and the one
 * that drifts. A reload is also what makes a deleted row's disappearance and the
 * count in the header agree.
 */
export function downloadsStore(Alpine) {
    return {
        busy: false,
        error: '',
        notice: '',
        _liveRegion: null,

        init() {
            // The outcome of the action that caused the reload. Without this the
            // result of pressing Retry is a page that looks the same apart from one
            // status badge, and for a screen-reader user it is nothing at all — the
            // announcement was torn down with the document that made it.
            try {
                const flash = sessionStorage.getItem('downloads-flash');
                if (flash) {
                    sessionStorage.removeItem('downloads-flash');
                    this.notice = flash;
                    this.announce(flash);
                }
            } catch {
                // sessionStorage throws in sandboxed contexts; there is simply no flash.
            }
        },

        announce(message) {
            // Created on first use rather than in init(): the store is registered on
            // every page in the app, and only this one ever announces anything.
            if (!this._liveRegion) {
                this._liveRegion = createLiveRegion();
            }
            this._liveRegion.announce(message);
        },

        /** The shared selection, as an array of history-row ids. */
        get selectedIds() {
            return [...(Alpine.store('bulkSelection')?.selectedIds ?? [])];
        },

        get selectedCount() {
            return this.selectedIds.length;
        },

        retry(ids) {
            return this.send('/v1/downloads/retry', ids, 'retried');
        },

        retrySelected() {
            return this.retry(this.selectedIds);
        },

        /**
         * Delete, once the reader has confirmed it.
         *
         * Deleting was the one destructive action in the app that asked nothing —
         * every other list confirms a bulk delete and names the count. The message
         * says what survives, because the obvious fear ("does this delete the file
         * I downloaded?") is the one the row itself cannot answer.
         */
        async remove(ids) {
            if (this.busy || ids.length === 0) return;

            const n = ids.length;
            const confirmed = await Alpine.store('confirmDialog').ask(
                `Delete ${n} download record${n === 1 ? '' : 's'}? ` +
                `${n === 1 ? 'The file it downloaded is' : 'The files they downloaded are'} not affected.`,
                { title: 'Delete downloads', confirmLabel: 'Delete' },
            );
            if (!confirmed) return;

            return this.send('/v1/downloads/delete', ids, 'deleted');
        },

        removeSelected() {
            return this.remove(this.selectedIds);
        },

        /**
         * Run one action and report what happened.
         *
         * The response carries an outcome per id, because a batch can be partly
         * refused — a completed download cannot be retried, a running one cannot be
         * deleted, and the queue can fill up part-way through. Reporting only the
         * count would leave the user to work out which rows were skipped by reading
         * the reloaded list. A batch refused *entirely* answers 409 and carries the
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
                    // the same reason repeated, and the reloaded list shows the rest.
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

export function registerDownloadsStore(Alpine) {
    Alpine.store('downloads', downloadsStore(Alpine));
}
