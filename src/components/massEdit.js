import { askToConfirm } from './confirmDialog.js';
import { focusedElement, restoreFocus } from '../utils/focus.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { findListContainer } from '../utils/listContainer.js';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';

// Mass Edit: one modal that applies several ops to many entities in one
// transaction, targeting either the checkbox selection or every entity matching
// the list page's current filter.
//
// The flow is deliberately two-phase: a DryRun probe resolves the count the
// confirmation dialog shows, and the confirmed submit sends that count back as
// ExpectedCount — the number in the dialog is the number the server acts on, or
// nobody acts.

const RELATION_FIELDS = ['TagIds', 'GroupIds', 'NoteIds', 'ResourceIds', 'RelatedGroupIds'];

// Which op sections an entity offers. The naming asymmetry is the wire
// format's: resources and notes name their related groups with GroupsOp;
// groups use RelatedGroupsOp.
const SECTIONS_BY_ENTITY = {
    resource: ['tags', 'groups', 'notes'],
    note: ['tags', 'groups', 'resources'],
    group: ['tags', 'relatedGroups', 'notes', 'resources'],
};

let _liveRegion = null;

export function massEditModal() {
    return {
        isOpen: false,
        entityType: null,
        target: 'ids',
        selectedIds: [],
        submitting: false,
        error: '',
        // Owner: 'set' or 'clear'.
        ownerMode: 'set',
        // Key rows for meta.removeKeys, one input each, datalist-backed.
        metaKeyRows: [''],
        _opener: null,

        init() {
            if (!_liveRegion) {
                _liveRegion = createLiveRegion();
            }
            window.addEventListener('mass-edit-open', (e) => this.open(e.detail || {}));
        },

        close() {
            this.isOpen = false;
            const opener = this._opener;
            this._opener = null;
            // Deferred a tick, like pluginActionModal does: restoring
            // synchronously happens while x-trap is still armed, which pulls
            // focus straight back inside before the x-if removes the subtree.
            this.$nextTick(() => restoreFocus(opener));
        },

        open({ entityType, target }) {
            if (!entityType) return;
            this._opener = focusedElement();
            this.entityType = entityType;
            this.target = target === 'filter' ? 'filter' : 'ids';
            if (this.target === 'ids') {
                this.selectedIds = [...(window.Alpine?.store('bulkSelection')?.selectedIds || [])];
                if (this.selectedIds.length === 0) {
                    // Opened with nothing selected: fall back to the filter so
                    // the reader is never shown an empty mode.
                    this.target = 'filter';
                }
            }
            this.ownerMode = 'set';
            this.metaKeyRows = [''];
            this.error = '';
            this.submitting = false;
            this.isOpen = true;
            this.$nextTick(() => {
                this.$root.querySelector('select, input:not([type=hidden])')?.focus();
                void this.loadMetaKeys();
            });
        },

        get sections() {
            return SECTIONS_BY_ENTITY[this.entityType] || [];
        },

        get totalCount() {
            const meta = document.querySelector('meta[name="x-total-count"]');
            return meta ? parseInt(meta.content, 10) || 0 : 0;
        },

        get currentFilter() {
            return window.location.search || '';
        },

        noun() {
            return this.entityType === 'group' ? 'groups' : `${this.entityType}s`;
        },

        // Picking ids in a section whose verb is still "unchanged" means the
        // reader wants that op: flip it to "add" rather than refusing the whole
        // submit later with "at least one operation".
        onTagsChange(change) { this.flipOp('TagsOp', change); },
        onGroupsChange(change) { this.flipOp('GroupsOp', change); },
        onNotesChange(change) { this.flipOp('NotesOp', change); },
        onResourcesChange(change) { this.flipOp('ResourcesOp', change); },
        onRelatedGroupsChange(change) { this.flipOp('RelatedGroupsOp', change); },

        flipOp(name, change) {
            if (!change || !change.added || change.added.length === 0) return;
            const select = this.$refs.form?.querySelector(`select[name="${name}"]`);
            if (select && !select.value) select.value = 'add';
        },

        metaOpValue() {
            return this.$refs.form?.querySelector('[name=MetaOp]')?.value || '';
        },

        fdMetaVisible() {
            return ['merge', 'replace'].includes(this.metaOpValue());
        },

        metaKeysVisible() {
            return this.metaOpValue() === 'removeKeys';
        },

        // The same key list the meta freeFields datalist uses.
        async loadMetaKeys() {
            const datalist = this.$root.querySelector('#mass-edit-meta-keys-list');
            if (!datalist || !this.entityType) return;
            try {
                const response = await fetch(`/v1/${this.noun()}/meta/keys`);
                if (!response.ok) return;
                const keys = await response.json();
                datalist.replaceChildren(...(Array.isArray(keys) ? keys : [])
                    .map((k) => {
                        const option = document.createElement('option');
                        option.value = typeof k === 'string' ? k : (k?.key || k?.Key || '');
                        return option;
                    }));
            } catch {
                // A missing datalist only costs suggestions.
            }
        },

        // Whether the confirmation dialog is mandatory: a filter target — the
        // set is only approximately known when the reader clicks — or any verb
        // that discards data. ownerSet reflects what the payload actually
        // carries, not the radio's default position.
        needsConfirm(ops, ownerSet) {
            if (this.target === 'filter') return true;
            if (this.ownerMode === 'clear') return true;
            return ['tags', 'groups', 'notes', 'resources', 'relatedGroups']
                .some((s) => ops[s] === 'replace') || ops.meta === 'removeKeys' || ops.meta === 'replace';
        },

        describeOps(ops, ownerSet) {
            const labels = [];
            if (ops.tags) labels.push(`${ops.tags} tags`);
            if (ops.groups) labels.push(`${ops.groups} groups`);
            if (ops.notes) labels.push(`${ops.notes} notes`);
            if (ops.resources) labels.push(`${ops.resources} resources`);
            if (ops.relatedGroups) labels.push(`${ops.relatedGroups} related groups`);
            if (ops.meta) labels.push(`${ops.meta} meta`);
            if (this.ownerMode === 'clear') labels.push('clear owner');
            else if (ownerSet) labels.push('set owner');
            return labels.join(', ');
        },

        // Build the wire payload from the form, including only the sections
        // whose verb is set — an empty verb means the op is absent, and sending
        // empty id lists alongside it would be an error.
        buildPayload(form, { dryRun, expectedCount }) {
            const fd = new FormData(form);
            const payload = new FormData();
            payload.set('Target', this.target);

            if (this.target === 'ids') {
                this.selectedIds.forEach((id) => payload.append('ID', String(id)));
            } else {
                payload.set('Filter', this.currentFilter);
                if (expectedCount != null) {
                    payload.set('ExpectedCount', String(expectedCount));
                }
            }

            const ops = {};
            const opFor = (section, field) => {
                const value = fd.get(field);
                if (value) ops[section] = String(value);
                return value ? String(value) : '';
            };

            if (this.sections.includes('tags')) ops.tags = opFor('tags', 'TagsOp');
            if (this.sections.includes('groups')) ops.groups = opFor('groups', 'GroupsOp');
            if (this.sections.includes('notes')) ops.notes = opFor('notes', 'NotesOp');
            if (this.sections.includes('resources')) ops.resources = opFor('resources', 'ResourcesOp');
            if (this.sections.includes('relatedGroups')) ops.relatedGroups = opFor('relatedGroups', 'RelatedGroupsOp');
            ops.meta = opFor('meta', 'MetaOp');

            // The verbs themselves ride the payload; an empty verb means the op
            // is absent.
            const opFields = {
                tags: 'TagsOp', groups: 'GroupsOp', notes: 'NotesOp',
                resources: 'ResourcesOp', relatedGroups: 'RelatedGroupsOp',
            };
            for (const [section, field] of Object.entries(opFields)) {
                if (ops[section]) payload.set(field, ops[section]);
            }

            RELATION_FIELDS.forEach((field) => {
                const section = {
                    TagIds: 'tags', GroupIds: 'groups', NoteIds: 'notes',
                    ResourceIds: 'resources', RelatedGroupIds: 'relatedGroups',
                }[field];
                if (!ops[section]) return;
                fd.getAll(field).filter(Boolean).forEach((id) => payload.append(field, String(id)));
            });

            let ownerSet = false;
            if (this.ownerMode === 'set') {
                const owner = fd.getAll('OwnerId').filter(Boolean);
                if (owner.length > 0) {
                    payload.set('OwnerOp', 'set');
                    payload.set('OwnerId', String(owner[0]));
                    ownerSet = true;
                }
            } else if (this.ownerMode === 'clear') {
                payload.set('OwnerOp', 'clear');
            }

            if (ops.meta === 'removeKeys') {
                payload.set('MetaOp', 'removeKeys');
                this.metaKeyRows.map((k) => k.trim()).filter(Boolean)
                    .forEach((k) => payload.append('MetaKeys', k));
            } else if (ops.meta) {
                const meta = fd.get('Meta');
                if (meta) {
                    payload.set('MetaOp', ops.meta);
                    payload.set('Meta', String(meta));
                }
            }

            if (dryRun) payload.set('DryRun', 'true');
            return { payload, ops, ownerSet };
        },

        async submit() {
            const form = this.$refs.form;
            if (!form) return;

            const { payload, ops, ownerSet } = this.buildPayload(form, { dryRun: false, expectedCount: null });
            const opCount = [...payload.keys()].filter((k) => k.endsWith('Op')).length;
            if (opCount === 0) {
                this.error = 'Choose at least one operation to apply.';
                return;
            }

            let matched = this.target === 'ids' ? this.selectedIds.length : this.totalCount;
            let expectedCount = null;
            if (this.target === 'filter') {
                // The count in the dialog is a fresh count, fetched immediately
                // before the confirm, and is what the server re-checks.
                const probe = this.buildPayload(form, { dryRun: true, expectedCount: null }).payload;
                probe.set('Target', this.target);
                try {
                    const response = await fetch(`/v1/${this.noun()}/massEdit`, { method: 'POST', body: probe });
                    if (!response.ok) throw new Error(await this.errorMessage(response));
                    const result = await response.json();
                    matched = result.matched;
                    expectedCount = result.matched;
                    // The probe's count is the handshake value: the server
                    // re-counts on the real submit and refuses on a mismatch.
                    payload.set('ExpectedCount', String(expectedCount));
                } catch (err) {
                    this.error = `Could not count the matching ${this.noun()}: ${err.message}`;
                    return;
                }
            }

            if (this.needsConfirm(ops, ownerSet)) {
                const scope = this.target === 'filter'
                    ? `all ${matched} ${this.noun()} matching the current filter`
                    : `the ${matched} selected ${this.noun()}`;
                const confirmed = await askToConfirm(
                    `Apply ${this.describeOps(ops, ownerSet)} to ${scope}. This cannot be undone.`,
                    { title: 'Mass edit', confirmLabel: 'Apply' },
                );
                if (!confirmed) return;
            }

            this.submitting = true;
            this.error = '';
            try {
                const response = await fetch(`/v1/${this.noun()}/massEdit`, { method: 'POST', body: payload });
                if (!response.ok) throw new Error(await this.errorMessage(response));
                const result = await response.json();
                await this.refreshList();
                _liveRegion?.announce(this.describeResult(result));
                this.close();
            } catch (err) {
                this.error = err.message;
                _liveRegion?.announce(`Mass edit failed: ${err.message}`);
            } finally {
                this.submitting = false;
            }
        },

        async errorMessage(response) {
            try {
                const body = await response.json();
                return body.error || `Server error: ${response.status}`;
            } catch {
                return `Server error: ${response.status}`;
            }
        },

        describeResult(result) {
            const parts = result.ops.map((op) => `${op.op}: ${op.rowsAffected} rows`);
            return `Mass edit applied to ${result.affected} ${result.entity}s (${parts.join(', ')})`;
        },

        // The same .body refetch + morph routine the bulk toolbar's
        // submitEditorForm uses, so the list updates in place.
        async refreshList() {
            const url = new URL(window.location);
            url.pathname = url.pathname + '.body';
            const refreshResponse = await fetch(url.toString());
            if (!refreshResponse.ok) {
                throw new Error(`Could not refresh list: ${refreshResponse.status}`);
            }
            const parser = new DOMParser();
            const refreshedDocument = parser.parseFromString(await refreshResponse.text(), 'text/html');
            const listContainer = findListContainer(document);
            const refreshedListContainer = findListContainer(refreshedDocument);
            if (!listContainer || !refreshedListContainer) {
                throw new Error('Could not find refreshed list');
            }
            window.Alpine.store('bulkSelection')?.deselectAll();
            Alpine.morph(listContainer, refreshedListContainer, morphOptionsWithShortcodeElements());
        },
    };
}
