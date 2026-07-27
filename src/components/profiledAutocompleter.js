import {
    createCreatableEntityFieldProfile,
    createDynamicEntitySelectorProfile,
    createMultiEntityFieldProfile,
    createSingleEntityFieldProfile,
    createTagEditorProfile,
    createTagFieldProfile,
} from '../selector/index.ts';
import { legacyAutocompleterAdapter } from './legacyAutocompleterAdapter.js';

function mapTagOption(raw) {
    return { key: raw.ID, label: raw.Name, raw };
}

function createProfiledAutocompleter(profile, onChange, { creatable, maximum }) {
    return legacyAutocompleterAdapter({
        _profileBridge: {
            profile,
            onChange,
            creatable,
            maximum,
        },
    });
}

/** Alpine rendering bridge for the explicit zero-or-one entity field profile. */
export function singleEntitySelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createSingleEntityFieldProfile(profileOptions),
        onChange,
        { creatable: false, maximum: 1 },
    );
}

/** Alpine rendering bridge for the explicit non-creatable multi-entity field profile. */
export function multiEntitySelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createMultiEntityFieldProfile(profileOptions),
        onChange,
        { creatable: false, maximum: profileOptions.maximum ?? 0 },
    );
}

/** Alpine rendering bridge for the explicit zero-or-one creatable entity field profile. */
export function creatableEntitySelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createCreatableEntityFieldProfile(profileOptions),
        onChange,
        { creatable: true, maximum: 1 },
    );
}

/** Alpine rendering bridge for the runtime-configured dynamic entity selector profile. */
export function dynamicEntitySelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createDynamicEntitySelectorProfile(profileOptions),
        onChange,
        { creatable: false, maximum: profileOptions.multiple ? (profileOptions.maximum ?? 0) : 1 },
    );
}

/** Alpine rendering bridge for the explicit creatable tag field profile. */
export function tagFieldSelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createTagFieldProfile(profileOptions),
        onChange,
        { creatable: true, maximum: 0 },
    );
}

/**
 * Alpine rendering bridge for the immediate tag-editor profile. The profile owns association
 * persistence and its pending/failed presentation, so the chip markup reads canonical string
 * keys from `pendingIds`/`failedIds` rather than any adapter-level optimistic tracking.
 */
export function tagEditorSelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    const profile = createTagEditorProfile(profileOptions);
    const base = createProfiledAutocompleter(profile, onChange, { creatable: true, maximum: 0 });
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
                reactive.pendingIds = new Set(snapshot.pendingKeys);
                reactive.failedIds = new Set(snapshot.failedKeys);
            };
            reactive._unsubscribeTagEditor = profile.subscribe(apply);
            apply(profile.getSnapshot());
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
                this._syncedEntityKey = entityKey;
                this.resetSelectedResults(values);
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
