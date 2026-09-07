import { askToConfirm } from './confirmDialog.js';

const endpoint = '/v1/account/saved-searches';

async function request(path, method = 'GET', body) {
    const response = await fetch(path, {
        method,
        headers: { Accept: 'application/json', ...(body ? { 'Content-Type': 'application/json' } : {}) },
        ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || data.message || `Request failed (${response.status}). Please try again.`);
    }
    return response.status === 204 ? null : response.json();
}

export function savedSearches(family) {
    return {
        family,
        open: false,
        loading: false,
        busy: false,
        searches: [],
        error: '',
        status: '',
        dialog: '',
        name: '',
        selected: null,
        appliedURL: '',
        _opener: null,
        _loadVersion: 0,

        async toggle() {
            if (this.open) return this.close();
            this.open = true;
            await this.load();
        },

        close() {
            if (this.dialog || this.busy) return;
            this.open = false;
            this.$nextTick(() => this.$refs.trigger.focus());
        },

        async load() {
            const version = ++this._loadVersion;
            this.loading = true;
            this.error = '';
            try {
                const searches = await request(`${endpoint}?family=${encodeURIComponent(this.family)}`);
                // Reopening can start another load. An older response must not
                // overwrite it or any mutations made after it finishes.
                if (version === this._loadVersion) this.searches = searches;
            } catch (error) {
                if (version === this._loadVersion) this.error = `Could not load saved searches. ${error.message}`;
            } finally {
                if (version === this._loadVersion) this.loading = false;
            }
        },

        startDialog(search = null) {
            this._opener = document.activeElement;
            this.selected = search;
            this.name = search?.name || '';
            this.appliedURL = window.location.pathname + window.location.search;
            this.error = '';
            this.status = '';
            this.dialog = search ? 'Rename saved search' : 'Save current search';
        },

        closeDialog() {
            if (this.busy) return;
            this.dialog = '';
            this.error = '';
            this.$nextTick(() => (this._opener?.isConnected ? this._opener : this.$refs.trigger)?.focus());
        },

        remember(search) {
            this.searches = [...this.searches.filter(item => item.id !== search.id), search]
                .sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()) || a.id - b.id);
        },

        async save() {
            if (this.busy || !this.name.trim()) return;
            this.busy = true;
            this.error = '';
            try {
                const search = this.selected
                    ? await request(`${endpoint}/${this.selected.id}`, 'PATCH', { name: this.name.trim() })
                    : await request(endpoint, 'POST', { name: this.name.trim(), url: this.appliedURL });
                this.remember(search);
                this.status = this.selected ? 'Saved search renamed.' : 'Search saved.';
                this.busy = false;
                this.closeDialog();
            } catch (error) {
                this.error = `Could not save search. ${error.message}`;
            } finally {
                this.busy = false;
            }
        },

        async replace(search) {
            if (this.busy) return;
            const url = window.location.pathname + window.location.search;
            if (!await askToConfirm(`Replace “${search.name}” with the currently applied search?`)) return;
            this.busy = true;
            this.error = '';
            this.status = '';
            try {
                this.remember(await request(`${endpoint}/${search.id}`, 'PATCH', { url }));
                this.status = 'Saved search replaced.';
            } catch (error) {
                this.error = `Could not replace search. ${error.message}`;
            } finally {
                this.busy = false;
            }
        },

        async remove(search) {
            if (this.busy) return;
            // Resolve refs while the x-for row that invoked this method is attached.
            // After deletion, Alpine's event scope can no longer find its parent refs.
            const focusTarget = this.$refs.save;
            if (!await askToConfirm(`Delete saved search “${search.name}”?`)) return;
            this.busy = true;
            this.error = '';
            this.status = '';
            try {
                await request(`${endpoint}/${search.id}`, 'DELETE');
                this.searches = this.searches.filter(item => item.id !== search.id);
                this.status = 'Saved search deleted.';
                this.busy = false;
                this.$nextTick(() => focusTarget.focus());
            } catch (error) {
                this.error = `Could not delete search. ${error.message}`;
            } finally {
                this.busy = false;
            }
        },
    };
}
