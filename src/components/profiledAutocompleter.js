import {
    createCreatableEntityFieldProfile,
    createSingleEntityFieldProfile,
    createTagFieldProfile,
} from '../selector/index.ts';
import { legacyAutocompleterAdapter } from './legacyAutocompleterAdapter.js';

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

/** Alpine rendering bridge for the explicit zero-or-one creatable entity field profile. */
export function creatableEntitySelector(arguments_) {
    const { onChange = null, ...profileOptions } = arguments_;
    return createProfiledAutocompleter(
        createCreatableEntityFieldProfile(profileOptions),
        onChange,
        { creatable: true, maximum: 1 },
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
