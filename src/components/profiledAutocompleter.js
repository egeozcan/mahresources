import {
    createCreatableEntityFieldProfile,
    createDynamicEntitySelectorProfile,
    createMultiEntityFieldProfile,
    createSingleEntityFieldProfile,
    createTagEditorProfile,
    createTagFieldProfile,
} from '../selector/index.ts';
import { selectorFieldAdapter } from './selectorFieldAdapter.js';

function mapTagOption(raw) {
    return { key: raw.ID, label: raw.Name, raw };
}

/**
 * The shared form templates spell "no limit" as `max=0`, while the core reads `maxSelected: 0`
 * literally as "nothing may be selected". Reconcile the two vocabularies here so no template or
 * call site has to know the difference.
 */
function unlimitedWhenZero(maximum) {
    return maximum || undefined;
}

/**
 * The shared partial can be rendered standalone through `/partials/autocompleter`, where the
 * initial selection can only arrive as a JSON string in a query parameter. Included from a page
 * template the same input is a real array. Both spellings describe the same selection.
 */
function normalizeSelected(selected) {
    if (typeof selected !== 'string') return selected || [];
    return selected ? JSON.parse(selected) : [];
}

/** Applies the input normalization every profile factory shares. */
function profileInputs({ selected, ...rest }) {
    return { ...rest, selected: normalizeSelected(selected) };
}

function createProfiledAutocompleter(profile, onChange, { creatable, maximum, itemHref = null }) {
    return selectorFieldAdapter({
        _profileBridge: {
            profile,
            onChange,
            creatable,
            maximum,
            itemHref,
        },
    });
}

/** Alpine rendering bridge for the explicit zero-or-one entity field profile. */
export function singleEntitySelector(arguments_) {
    const { onChange = null, ...rawOptions } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    return createProfiledAutocompleter(
        createSingleEntityFieldProfile(profileOptions),
        onChange,
        { creatable: false, maximum: 1 },
    );
}

/** Alpine rendering bridge for the explicit non-creatable multi-entity field profile. */
export function multiEntitySelector(arguments_) {
    const { onChange = null, ...rawOptions } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    const maximum = unlimitedWhenZero(profileOptions.maximum);
    return createProfiledAutocompleter(
        createMultiEntityFieldProfile({ ...profileOptions, maximum }),
        onChange,
        { creatable: false, maximum: maximum ?? 0 },
    );
}

/** Alpine rendering bridge for the explicit zero-or-one creatable entity field profile. */
export function creatableEntitySelector(arguments_) {
    const { onChange = null, ...rawOptions } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    return createProfiledAutocompleter(
        createCreatableEntityFieldProfile(profileOptions),
        onChange,
        { creatable: true, maximum: 1 },
    );
}

/** Alpine rendering bridge for the runtime-configured dynamic entity selector profile. */
export function dynamicEntitySelector(arguments_) {
    const { onChange = null, ...rawOptions } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    const maximum = unlimitedWhenZero(profileOptions.maximum);
    return createProfiledAutocompleter(
        createDynamicEntitySelectorProfile({ ...profileOptions, maximum }),
        onChange,
        { creatable: false, maximum: profileOptions.multiple ? (maximum ?? 0) : 1 },
    );
}

/** Alpine rendering bridge for the explicit creatable tag field profile. */
export function tagFieldSelector(arguments_) {
    const { onChange = null, ...rawOptions } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    const maximum = unlimitedWhenZero(profileOptions.maximum);
    return createProfiledAutocompleter(
        createTagFieldProfile({ ...profileOptions, maximum }),
        onChange,
        { creatable: true, maximum: maximum ?? 0 },
    );
}

/**
 * Association persistence for a tag editor that has only the endpoints, not a
 * domain object: the detail-page sidebar names the add/remove URLs and the entity
 * id, and this builds the adapter. The lightbox keeps its own adapter because it
 * also maintains a details cache and a recent-tag list.
 *
 * The abort signal is deliberately unused, exactly as in the lightbox: each write
 * names the entity up front, so a write begun just before the reader navigates
 * must still land rather than be aborted with the component.
 */
function tagAssociationFromUrls({ addUrl, removeUrl, entityId }) {
    // Writes for one tag are serialized. The profile invalidates a superseded
    // operation's *result*, but it cannot recall a request the server may already
    // have applied; without this, a rapid add-then-remove reaches the server in
    // whatever order the responses settle, and the UI can end up disagreeing with
    // the row. Chaining per tag makes the last transition the reader gave the last
    // one the server sees.
    const perTag = new Map();
    const post = (url, tagId) => {
        const body = new FormData();
        body.append('ID', String(entityId));
        body.append('EditedId', String(tagId));
        return fetch(url, {
            method: 'POST',
            body,
            headers: { Accept: 'application/json' },
        }).then((response) => {
            if (!response.ok) {
                throw new Error(`Could not update tags (${response.status})`);
            }
        });
    };
    const enqueue = (url, tag) => {
        const key = String(tag.ID);
        const previous = perTag.get(key) ?? Promise.resolve();
        const next = previous.catch(() => undefined).then(() => post(url, tag.ID));
        // Track a swallowed copy so one failed tag cannot poison the chain (or
        // reject unhandled) while the caller still gets the real rejection.
        const tracked = next.catch(() => undefined);
        perTag.set(key, tracked);
        void tracked.then(() => {
            if (perTag.get(key) === tracked) perTag.delete(key);
        });
        return next;
    };
    return {
        add: (tag) => enqueue(addUrl, tag),
        remove: (tag) => enqueue(removeUrl, tag),
    };
}

/**
 * Alpine rendering bridge for the immediate tag-editor profile. The profile owns association
 * persistence and its pending/failed presentation, so the chip markup reads canonical string
 * keys from `pendingIds`/`failedIds` rather than any adapter-level optimistic tracking.
 *
 * `association` is an explicit adapter (the lightbox) or, for a caller that has only the
 * endpoints, `addUrl`/`removeUrl`/`entityId` (the detail sidebar).
 */
export function tagEditorSelector(arguments_) {
    const explicitAssociation = arguments_.association;
    const {
        onChange = null,
        addUrl = null,
        removeUrl = null,
        entityId = null,
        // A caller that passes its own association adapter owns its own failure
        // messages (the lightbox announces per tag); the built-in URL adapter has no
        // voice, so the profile announces the rollback for it.
        announceFailures = !explicitAssociation,
        // A tag chip stays a link to its own page. This is the profile's default rather
        // than a template parameter because every caller renders tags the same way; the
        // lightbox supplies custom chip markup and simply ignores it.
        itemHref = (item) => (item && item.ID != null ? `/tag?id=${item.ID}` : null),
        ...rawOptions
    } = arguments_;
    const profileOptions = profileInputs(rawOptions);
    const association = profileOptions.association
        || (addUrl && removeUrl && entityId != null
            ? tagAssociationFromUrls({ addUrl, removeUrl, entityId })
            : null);
    if (!association) {
        throw new Error('tagEditorSelector needs an association adapter or addUrl/removeUrl/entityId');
    }
    const profile = createTagEditorProfile({ ...profileOptions, association });
    const base = createProfiledAutocompleter(profile, onChange, { creatable: true, maximum: 0, itemHref });
    const baseInit = base.init;
    const baseDestroy = base.destroy;

    const extensions = {
        // Canonical string keys, published by the profile.
        pendingIds: new Set(),
        failedIds: new Set(),
        _unsubscribeTagEditor: null,
        _syncedEntityKey: undefined,

        init() {
            baseInit.call(this);
            const reactive = globalThis.Alpine?.$data?.(this.$el) || this;
            const apply = (snapshot) => {
                const nextFailed = new Set(snapshot.failedKeys);
                const newlyFailed = [...nextFailed].some((key) => !reactive.failedIds?.has(key));
                reactive.pendingIds = new Set(snapshot.pendingKeys);
                reactive.failedIds = nextFailed;
                // The optimistic chip already said "Added X". A rollback must not be the
                // silent half of that pair, or a screen-reader user is told a change
                // happened and never told it was undone.
                if (newlyFailed && announceFailures) {
                    reactive._liveRegion?.announce('Could not update tags; the change was undone.');
                }
            };
            reactive._unsubscribeTagEditor = profile.subscribe(apply);
            apply(profile.getSnapshot());
        },

        /** Whether this tag's association write is in flight (its chip is provisional). */
        isPending(item) {
            return this.pendingIds.has(String(item?.ID));
        },

        /** Whether this tag's last association write failed (its chip is rolled back). */
        isFailed(item) {
            return this.failedIds.has(String(item?.ID));
        },

        /**
         * Single entry point for domain-driven tag changes. A different owning entity is a
         * navigation: reset the whole selection and invalidate every in-flight write, because
         * those writes describe the entity we just left. The same entity means another surface
         * (suggested tags, quick slots, undo) changed the association set, so only the keys
         * that actually moved are reconciled.
         */
        syncEntityTags(entityKey, tags) {
            const values = tags || [];
            if (entityKey !== this._syncedEntityKey) {
                this._browseConfirmation?.destroy();
                this._browseConfirmation = null;
                // A real navigation (not the first adoption): a query typed for the previous
                // entity left its results open, filtered against that entity's tags. Clear it.
                if (this._syncedEntityKey !== undefined) {
                    this._clearInput?.();
                    this._core?.dispatch({ type: 'close' });
                }
                this._syncedEntityKey = entityKey;
                // Through the profile's own selector, so the navigation invalidates every
                // in-flight association write rather than only the keys that moved.
                profile.selector.dispatch({
                    type: 'replace-selection',
                    options: values.map(mapTagOption),
                    reason: 'reset',
                    silent: true,
                });
                return;
            }
            profile.syncAssociations(values.map(mapTagOption));
        },

        destroy() {
            this._unsubscribeTagEditor?.();
            this._unsubscribeTagEditor = null;
            baseDestroy.call(this);
            profile.destroy();
        },
    };

    // Property descriptors rather than a spread: the rendering adapter exposes accessors
    // (optionCount) that a spread would freeze into one-time values.
    return Object.defineProperties({}, {
        ...Object.getOwnPropertyDescriptors(base),
        ...Object.getOwnPropertyDescriptors(extensions),
    });
}
