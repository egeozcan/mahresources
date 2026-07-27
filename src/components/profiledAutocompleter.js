import { createSingleEntityFieldProfile } from '../selector/index.ts';
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
