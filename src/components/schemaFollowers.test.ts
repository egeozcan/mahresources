import { beforeEach, describe, expect, test, vi } from 'vitest';
import { selectorRegistry, type SelectorValue } from '../selector/selectorRegistry';
import { schemaMetaFields } from './schemaMetaFields.js';
import { schemaSearchFields } from './schemaSearchFields.js';

const OBJECT_SCHEMA = '{"type":"object","properties":{"isbn":{"type":"string"}}}';
const OTHER_OBJECT_SCHEMA = '{"type":"object","properties":{"pages":{"type":"number"}}}';

function form(): HTMLFormElement {
    return {} as HTMLFormElement;
}

/**
 * Mounts a component the way Alpine would: `$el` is the root element the template put the
 * data attributes on, and `closest('form')` resolves the owning form the registry keys on.
 */
function mount<T extends Record<string, any>>(
    component: T,
    { owner, dataset = {}, refs = {} }: {
        owner: HTMLFormElement;
        dataset?: Record<string, string>;
        refs?: Record<string, unknown>;
    },
): T {
    const mounted = component as T & Record<string, any>;
    mounted.$el = { dataset, closest: vi.fn(() => owner) } as unknown as HTMLElement;
    mounted.$refs = refs;
    mounted.$nextTick = (callback: () => void) => { callback(); };
    mounted.init();
    return mounted;
}

function change(values: SelectorValue[]) {
    return { values, added: values, removed: [] };
}

describe('schemaMetaFields', () => {
    test('adopts the object schema of the newly selected category', () => {
        const owner = form();
        const fields = mount(schemaMetaFields({ field: 'categoryId' }), { owner });

        selectorRegistry.notifyChange(owner, 'categoryId', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
        ]));

        expect(fields.currentSchema).toBe(OBJECT_SCHEMA);
    });

    // Bug 1 regression: an unparseable MetaSchema must not reach schema-form-mode, on the
    // server-rendered initial value or on a later selection.
    test.each([
        ['malformed JSON', '{not json'],
        ['a bare scalar', '"just a string"'],
        ['a blank schema', ''],
    ])('ignores an initial schema that is %s', (_label, initialSchema) => {
        const fields = mount(schemaMetaFields({ field: 'categoryId' }), {
            owner: form(),
            dataset: { initialSchema },
        });

        expect(fields.currentSchema).toBeNull();
    });

    test('falls back to free-form meta for a cleared, blank, or malformed schema', () => {
        const owner = form();
        const fields = mount(schemaMetaFields({ field: 'categoryId' }), {
            owner,
            dataset: { initialSchema: OBJECT_SCHEMA },
        });
        expect(fields.currentSchema).toBe(OBJECT_SCHEMA);

        selectorRegistry.notifyChange(owner, 'categoryId', change([{ ID: 2, Name: 'Plain' }]));
        expect(fields.currentSchema).toBeNull();

        selectorRegistry.notifyChange(owner, 'categoryId', change([
            { ID: 3, Name: 'Broken', MetaSchema: '{not json' },
        ]));
        expect(fields.currentSchema).toBeNull();

        selectorRegistry.notifyChange(owner, 'categoryId', change([]));
        expect(fields.currentSchema).toBeNull();
    });

    test('restores the server-rendered meta and marks user edits', () => {
        const owner = form();
        const fields = mount(schemaMetaFields({ field: 'categoryId' }), {
            owner,
            dataset: { initialMeta: '{"isbn":"123"}' },
        });

        expect(fields.currentMeta).toEqual({ isbn: '123' });
        expect(fields.metaEdited).toBe(false);

        fields.handleMetaChange({ detail: { value: { isbn: '456' } } });
        expect(fields.currentMeta).toEqual({ isbn: '456' });
        expect(fields.metaEdited).toBe(true);
    });

    test('stops following its field once destroyed', () => {
        const owner = form();
        const fields = mount(schemaMetaFields({ field: 'categoryId' }), { owner });

        fields.destroy();
        selectorRegistry.notifyChange(owner, 'categoryId', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
        ]));

        expect(fields.currentSchema).toBeNull();
    });
});

describe('schemaSearchFields', () => {
    let searchEditor: { setAttribute: ReturnType<typeof vi.fn> };

    beforeEach(() => {
        searchEditor = { setAttribute: vi.fn() };
    });

    test('publishes a single schema directly and several as a JSON array', () => {
        const owner = form();
        const fields = mount(schemaSearchFields({ field: 'categories' }), {
            owner,
            refs: { searchEditor },
        });

        selectorRegistry.notifyChange(owner, 'categories', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
        ]));
        expect(searchEditor.setAttribute).toHaveBeenLastCalledWith('schema', OBJECT_SCHEMA);

        selectorRegistry.notifyChange(owner, 'categories', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
            { ID: 2, Name: 'Manual', MetaSchema: OTHER_OBJECT_SCHEMA },
        ]));
        expect(searchEditor.setAttribute).toHaveBeenLastCalledWith(
            'schema', JSON.stringify([OBJECT_SCHEMA, OTHER_OBJECT_SCHEMA]),
        );
        expect(fields.schemas).toEqual([OBJECT_SCHEMA, OTHER_OBJECT_SCHEMA]);
    });

    test('suppresses schema fields when any selected category is unconstrained', () => {
        const owner = form();
        const fields = mount(schemaSearchFields({ field: 'categories' }), {
            owner,
            refs: { searchEditor },
        });

        selectorRegistry.notifyChange(owner, 'categories', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
            { ID: 2, Name: 'Plain' },
        ]));

        expect(fields.schemas).toEqual([]);
        expect(searchEditor.setAttribute).toHaveBeenLastCalledWith('schema', '');
    });

    test('hydrates from the server-rendered selection before any change arrives', () => {
        mount(
            schemaSearchFields({
                field: 'categories',
                initialCategories: [{ ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA }],
            }),
            { owner: form(), refs: { searchEditor } },
        );

        expect(searchEditor.setAttribute).toHaveBeenCalledWith('schema', OBJECT_SCHEMA);
    });

    test('stops following its field once destroyed', () => {
        const owner = form();
        const fields = mount(schemaSearchFields({ field: 'categories' }), {
            owner,
            refs: { searchEditor },
        });

        fields.destroy();
        selectorRegistry.notifyChange(owner, 'categories', change([
            { ID: 1, Name: 'Book', MetaSchema: OBJECT_SCHEMA },
        ]));

        expect(searchEditor.setAttribute).not.toHaveBeenCalled();
    });
});
