import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
    creatableEntitySelector,
    dynamicEntitySelector,
    singleEntitySelector,
    tagFieldSelector,
} from './dropdown.js';

type ProfiledSelector = ReturnType<typeof singleEntitySelector> & {
    $el: HTMLElement;
    $refs: Record<string, HTMLElement>;
    $watch: (name: string, callback: (...args: unknown[]) => void) => void;
    $nextTick: (callback?: () => void) => Promise<void> | void;
    $dispatch: ReturnType<typeof vi.fn>;
};

function createNode() {
    return {
        style: {},
        parentNode: null as ReturnType<typeof createNode> | null,
        setAttribute: vi.fn(),
        appendChild(child: ReturnType<typeof createNode>) {
            child.parentNode = this;
        },
        removeChild(child: ReturnType<typeof createNode>) {
            child.parentNode = null;
        },
    };
}

function mount(selector: ProfiledSelector): ProfiledSelector {
    const root = createNode() as unknown as HTMLElement & {
        closest: (query: string) => HTMLFormElement | null;
    };
    root.closest = vi.fn(() => null);
    selector.$el = root;
    selector.$refs = {};
    selector.$watch = vi.fn();
    selector.$nextTick = (callback) => callback ? callback() : Promise.resolve();
    selector.$dispatch = vi.fn();
    selector.init();
    return selector;
}

describe('profiled autocompleter bridge', () => {
    beforeEach(() => {
        vi.stubGlobal('document', {
            activeElement: null,
            createElement: vi.fn(() => createNode()),
            body: createNode(),
            querySelectorAll: vi.fn(() => []),
        });
        vi.stubGlobal('window', {
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            dispatchEvent: vi.fn(),
        });
        vi.stubGlobal('fetch', vi.fn());
    });

    afterEach(() => {
        vi.restoreAllMocks();
        vi.unstubAllGlobals();
    });

    test('single-entity callers receive one atomic replacement change without custom window events', () => {
        const alpha = { ID: 1, Name: 'Alpha' };
        const beta = { ID: 2, Name: 'Beta' };
        const onChange = vi.fn();
        const selector = mount(singleEntitySelector({
            entity: 'resource',
            selected: [alpha],
            onChange,
        }) as ProfiledSelector);

        selector._core.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });

        expect(onChange).toHaveBeenCalledTimes(1);
        expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
            previous: [{ key: 1, label: 'Alpha', raw: alpha }],
            current: [{ key: 2, label: 'Beta', raw: beta }],
            removed: [{ key: 1, label: 'Alpha', raw: alpha }],
            added: [{ key: 2, label: 'Beta', raw: beta }],
            reason: 'select',
        }));
        expect(window.dispatchEvent).not.toHaveBeenCalled();
        selector.destroy();
    });

    test('block editor query selector translates one atomic change into selected or cleared events', () => {
        const markup = readFileSync(
            new URL('../../templates/partials/blockEditor.tpl', import.meta.url),
            'utf8',
        );

        expect(markup).toContain('singleEntitySelector({');
        expect(markup).toContain("entity: 'query'");
        expect(markup).toContain('onChange: (change) =>');
        expect(markup).toContain("$dispatch('table-query-selected', change.added[0].raw)");
        expect(markup).toContain("$dispatch('table-query-cleared')");
        expect(markup).not.toContain('onSelect: (q)');
        expect(markup).not.toContain('onRemove: ()');
        expect(markup).not.toContain("url: '/v1/queries'");
        expect(markup).not.toContain('standalone: true');
    });

    test('paste upload uses explicit profiles and derives store values from atomic changes', () => {
        const markup = readFileSync(
            new URL('../../templates/partials/pasteUpload.tpl', import.meta.url),
            'utf8',
        );

        expect(tagFieldSelector).toBeTypeOf('function');
        expect(creatableEntitySelector).toBeTypeOf('function');
        expect(markup).toContain('tagFieldSelector({');
        expect(markup).toContain("usage: 'resource'");
        expect(markup).toContain('singleEntitySelector({');
        expect(markup).toContain("entity: 'resourceCategory'");
        expect(markup).toContain('creatableEntitySelector({');
        expect(markup).toContain("entity: 'series'");
        expect(markup.match(/onChange: \(change\) =>/g)).toHaveLength(3);
        expect(markup).toContain('$store.pasteUpload.tags = change.current.map');
        expect(markup).toContain('$store.pasteUpload.categoryId = change.current[0]?.raw.ID || null');
        expect(markup).toContain('$store.pasteUpload.seriesId = change.current[0]?.raw.ID || null');
        expect(markup).not.toContain('x-effect="$store.pasteUpload');
        expect(markup).not.toContain('selectedResults: []');
        expect(markup).not.toContain('standalone: true');
    });

    test('entity picker filters use the dynamic profile and translate changes through the store', () => {
        const markup = readFileSync(
            new URL('../../templates/partials/entityPicker.tpl', import.meta.url),
            'utf8',
        );

        expect(markup).toContain('dynamicEntitySelector({');
        expect(markup).toContain('searchUrl: filter.endpoint');
        expect(markup).toContain('multiple: filter.multi');
        expect(markup).toContain('$store.entityPicker.applyFilterChange(filter.key, filter.multi, change)');
        // The runtime picker keeps its close-time reset but no longer restores state through
        // legacy flags or per-item select/remove callbacks.
        expect(markup).toContain("@entity-picker-closed.window=\"resetSelectedResults([])\"");
        expect(markup).not.toContain('standalone: true');
        expect(markup).not.toContain('onSelect: (item)');
        expect(markup).not.toContain('onRemove: (item)');
        expect(markup).not.toContain('selectedResults: []');
        expect(markup).not.toContain('url: filter.endpoint');
    });

    test('a dynamic single-value filter selector replaces its selection atomically', () => {
        const alpha = { ID: 1, Name: 'Alpha' };
        const beta = { ID: 2, Name: 'Beta' };
        const onChange = vi.fn();
        const selector = mount(dynamicEntitySelector({
            searchUrl: '/v1/categories',
            multiple: false,
            onChange,
        }) as ProfiledSelector);

        expect(selector.max).toBe(1);

        selector._core.dispatch({
            type: 'select-option',
            option: { key: alpha.ID, label: alpha.Name, raw: alpha },
        });
        selector._core.dispatch({
            type: 'select-option',
            option: { key: beta.ID, label: beta.Name, raw: beta },
        });

        expect(onChange).toHaveBeenCalledTimes(2);
        expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({
            current: [{ key: 2, label: 'Beta', raw: beta }],
            removed: [{ key: 1, label: 'Alpha', raw: alpha }],
            added: [{ key: 2, label: 'Beta', raw: beta }],
        }));
        // Runtime filters never serialize into a form, so no aggregate form event is dispatched.
        expect(selector.$dispatch).not.toHaveBeenCalled();
        selector.destroy();
    });

    test('a dynamic multi-value filter selector accumulates and clears through commands', () => {
        const alpha = { ID: 1, Name: 'Alpha' };
        const beta = { ID: 2, Name: 'Beta' };
        const onChange = vi.fn();
        const selector = mount(dynamicEntitySelector({
            searchUrl: '/v1/tags',
            multiple: true,
            onChange,
        }) as ProfiledSelector);

        expect(selector.max).toBe(0);

        for (const raw of [alpha, beta]) {
            selector._core.dispatch({
                type: 'select-option',
                option: { key: raw.ID, label: raw.Name, raw },
            });
        }
        expect(selector.selectedResults).toEqual([alpha, beta]);

        // The picker's close-time reset is silent: it clears state without re-notifying filters.
        onChange.mockClear();
        selector.resetSelectedResults([]);
        expect(selector.selectedResults).toEqual([]);
        expect(onChange).not.toHaveBeenCalled();
        selector.destroy();
    });

    test('compare templates use single-entity profiles and local atomic listeners', () => {
        const resourceCompare = readFileSync(
            new URL('../../templates/compare.tpl', import.meta.url),
            'utf8',
        );
        const groupCompare = readFileSync(
            new URL('../../templates/groupCompare.tpl', import.meta.url),
            'utf8',
        );

        for (const markup of [resourceCompare, groupCompare]) {
            expect(markup).toContain('singleEntitySelector({');
            expect(markup).toContain('onChange: (change) =>');
            expect(markup).not.toContain('standalone:');
            expect(markup).not.toContain('dispatchOnSelect');
            expect(markup).not.toMatch(/@[a-z0-9-]+\.window=/);
        }
    });
});
