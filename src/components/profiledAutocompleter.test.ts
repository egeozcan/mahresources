import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { singleEntitySelector } from './dropdown.js';

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
