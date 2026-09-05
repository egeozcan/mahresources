import { createSingleEntityFieldProfile } from './entityFieldProfiles.ts';
import { selectorFieldAdapter } from '../components/selectorFieldAdapter.js';

let nextSelectorId = 0;

/** Mount the shared field for a non-Alpine consumer, without joining a surrounding form. */
export async function mountSingleEntitySelector(container, {
    title, onChange, signal, ...options
}) {
    const Alpine = window.Alpine;
    const query = new URLSearchParams({
        profile: 'single', entity: options.entity, title,
        id: `entity-selector-${++nextSelectorId}`, standalone: 'true',
    });
    const response = await fetch('/partials/autocompleter?' + query, { signal });
    if (!response.ok) throw new Error('Unable to load entity selector');
    const html = await response.text();
    signal?.throwIfAborted();
    const template = document.createElement('template');
    template.innerHTML = html;
    const field = template.content.firstElementChild;
    if (!field?.matches('[data-selector-profile="single"]')) throw new Error('Invalid entity selector markup');

    const profile = createSingleEntityFieldProfile({ ...options, form: undefined });
    const adapter = selectorFieldAdapter({ _profileBridge: {
        profile, onChange, creatable: false, maximum: 1,
    } });
    // The server supplies identical markup/ARIA to ordinary fields. Domain inputs stay in
    // JavaScript values, never interpolated into executable Alpine expressions.
    field.setAttribute('x-data', 'mountedEntitySelector');
    const removeScope = Alpine.addScopeToNode(field, { mountedEntitySelector: () => adapter });
    const fieldset = document.createElement('fieldset');
    fieldset.append(field);
    Alpine.mutateDom(() => {
        container.replaceChildren(fieldset);
        Alpine.initTree(fieldset);
    });
    let destroyed = false;
    return {
        getRawValues: () => profile.selector.getSnapshot().selected.map(option => option.raw),
        replaceRawValues(values) {
            profile.selector.dispatch({ type: 'replace-selection', silent: true,
                options: values.map(raw => ({ key: raw.ID, label: raw.Name, raw })),
            });
        },
        replaceByKeys(keys) {
            const existing = new Map(profile.selector.getSnapshot().selected.map(option => [String(option.key), option]));
            profile.selector.dispatch({ type: 'replace-selection', silent: true, options: keys.map(key =>
                existing.get(String(key)) || { key, label: `#${key}`, raw: { ID: key, Name: `#${key}` } }),
            });
        },
        setDisabled(disabled) {
            fieldset.disabled = disabled;
            // Dropdown options are divs, so disabled fieldset alone cannot block them.
            fieldset.inert = disabled;
            if (disabled) profile.selector.dispatch({ type: 'close' });
        },
        destroy() {
            if (destroyed) return;
            destroyed = true;
            Alpine.destroyTree(fieldset);
            profile.selector.destroy();
            removeScope();
            fieldset.remove();
        },
    };
}
