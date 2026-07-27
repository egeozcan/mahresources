import { describe, expect, test, vi } from 'vitest';
import { autocompleter } from './dropdown.js';
import { normalizeLegacyAutocompleterConfig } from './legacyAutocompleterConfig';

describe('legacy autocompleter configuration normalization', () => {
    test('normalizes serialized selections and legacy source settings', () => {
        const config = normalizeLegacyAutocompleterConfig({
            selectedResults: '[{"ID":7,"Name":"Seven","Category":{"Name":"Number"}}]',
            min: '1',
            max: '2',
            ownerId: '42',
            url: '/v1/groups?limit=20',
            addUrl: '/v1/group',
            sortBy: 'most_used',
            elName: 'GroupIds',
        });

        expect(config.source).toMatchObject({
            searchUrl: '/v1/groups?limit=20',
            createUrl: '/v1/group',
            ownerId: 42,
            sortBy: 'most_used',
            dynamicFilters: [],
        });
        expect(config.selection).toEqual({
            initialValues: [
                { ID: 7, Name: 'Seven', Category: { Name: 'Number' } },
            ],
            minimum: 1,
            maximum: 2,
        });
        expect(config.source.mapOption(config.selection.initialValues[0])).toEqual({
            key: 7,
            label: 'Seven',
            raw: { ID: 7, Name: 'Seven', Category: { Name: 'Number' } },
        });
        expect(config.rendering.fieldName).toBe('GroupIds');
    });

    test('parses serialized dynamic filter declarations at the compatibility edge', () => {
        const config = normalizeLegacyAutocompleterConfig({
            selectedResults: [],
            url: '/v1/groups',
            filterEls: '[{"nameInput":"GroupRelationTypeId","nameGet":"RelationTypeId"},{"nameInput":"RelationSideFrom","nameGet":"RelationSide"}]',
        });

        expect(config.source.dynamicFilters).toEqual([
            { nameInput: 'GroupRelationTypeId', nameGet: 'RelationTypeId' },
            { nameInput: 'RelationSideFrom', nameGet: 'RelationSide' },
        ]);
    });

    test('preserves legacy array references, parser fallback, defaults, and rendering hooks', () => {
        const selectedResults = [{ ID: 'tag-1', Name: 'Tag one' }];
        const filters = [{ nameInput: 'OwnerId', nameGet: 'ownerId' }];
        const onSelect = vi.fn();
        const onRemove = vi.fn();
        const config = normalizeLegacyAutocompleterConfig({
            selectedResults,
            max: '0x10',
            min: undefined,
            ownerId: undefined,
            url: '/v1/tags',
            filterEls: filters,
            extraInfo: 'Category',
            standalone: true,
            commitOnSpace: true,
            onSelect,
            onRemove,
        });

        expect(config.selection.initialValues).toBe(selectedResults);
        expect(config.source.dynamicFilters).toBe(filters);
        expect(config.selection.maximum).toBe(16);
        expect(config.selection.minimum).toBe(0);
        expect(config.source.ownerId).toBe(0);
        expect(config.source.createUrl).toBe('');
        expect(config.rendering).toEqual({
            fieldName: undefined,
            extraInfo: 'Category',
            standalone: true,
            commitOnSpace: true,
            onSelect,
            onRemove,
        });

        const fallback = normalizeLegacyAutocompleterConfig({
            selectedResults: [],
            url: '/v1/tags',
            filterEls: 'OwnerId',
        });
        expect(fallback.source.dynamicFilters).toEqual(['OwnerId']);
    });

    test('keeps the public factory export and legacy Alpine properties behavior-compatible', () => {
        const selectedResults = [{ ID: 1, Name: 'Alpha' }];
        const onSelect = vi.fn();
        const component = autocompleter({
            selectedResults,
            min: '1',
            max: '3',
            ownerId: '9',
            url: '/v1/tags',
            addUrl: '/v1/tag',
            sortBy: 'most_used_resource',
            elName: 'TagIds',
            filterEls: '[{"nameInput":"OwnerId","nameGet":"ownerId"}]',
            extraInfo: 'Category',
            standalone: true,
            commitOnSpace: true,
            onSelect,
        });

        expect(component).toMatchObject({
            max: 3,
            min: 1,
            ownerId: 9,
            selectedResults,
            url: '/v1/tags',
            addUrl: '/v1/tag',
            sortBy: 'most_used_resource',
            filterEls: [{ nameInput: 'OwnerId', nameGet: 'ownerId' }],
            extraInfo: 'Category',
        });
        expect(component.selectedResults).toBe(selectedResults);
        expect(component.inputEvents['@keydown']).toBeTypeOf('function');
        expect(component.init).toBeTypeOf('function');
        expect(component.destroy).toBeTypeOf('function');
    });
});
