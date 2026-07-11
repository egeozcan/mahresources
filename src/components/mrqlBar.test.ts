import { describe, expect, test } from 'vitest';
import { expandGroupMRQLFromParams, formValuesToMRQL, mrqlToFormValues } from './mrqlBar.js';

describe('MRQL list form synchronization', () => {
    test('turns resource form values into canonical MRQL', () => {
        const values = new FormData();
        values.set('Name', 'summer "trip"');
        values.append('tags', '12');
        values.append('tags', '19');
        values.set('MinWidth', '800');

        const relations = new Map([['tags', ['vacation', 'favorite']]]);
        expect(formValuesToMRQL('resource', values, relations)).toBe(
            'name ~ "*summer \\"trip\\"*" AND width >= 800 AND tags = "vacation" AND tags = "favorite"',
        );
    });

    test('translates the canonical subset back to form values', () => {
        const result = mrqlToFormValues(
            'note',
            'name ~ "*meeting*" AND noteType = 4 AND owner = 9',
        );

        expect(result.compatible).toBe(true);
        expect(result.values.get('Name')).toEqual(['meeting']);
        expect(result.values.get('NoteTypeId')).toEqual(['4']);
        expect(result.values.get('ownerId')).toEqual(['9']);
    });

    test('marks exact relation names for ID resolution by the form', () => {
        const result = mrqlToFormValues('resource', 'tags = "vacation"');
        expect(result.compatible).toBe(true);
        expect(result.values.get('tags')).toEqual(['vacation']);
        expect(result.nameLookups.has('tags')).toBe(true);
    });

    test('quotes relation names even when the name is numeric', () => {
        const values = new FormData();
        values.set('tags', '42');
        expect(formValuesToMRQL('resource', values, new Map([['tags', ['2026']]])))
            .toBe('tags = "2026"');
    });

    test('represents group parent and child search switches in MRQL', () => {
        const values = new FormData();
        values.set('Name', 'alp');
        values.set('SearchParentsForName', '1');
        values.set('SearchChildrenForName', '1');
        values.set('tags', '2');
        values.set('SearchParentsForTags', '1');
        values.set('SearchChildrenForTags', '1');

        expect(formValuesToMRQL('group', values, new Map([['tags', ['vacation']]]))).toBe(
            '(name ~ "*alp*" OR parent.name ~ "*alp*" OR children.name ~ "*alp*") AND ' +
            '(tags = "vacation" OR parent.tags = "vacation" OR children.tags = "vacation")',
        );
    });

    test('translates expanded group searches back to their switches', () => {
        const result = mrqlToFormValues(
            'group',
            '(name ~ "*alp*" OR parent.name ~ "*alp*" OR children.name ~ "*alp*") AND ' +
            '(tags = "vacation" OR parent.tags = "vacation" OR children.tags = "vacation")',
        );

        expect(result.compatible).toBe(true);
        expect(result.values.get('Name')).toEqual(['alp']);
        expect(result.values.get('tags')).toEqual(['vacation']);
        expect(result.values.get('SearchParentsForName')).toEqual(['1']);
        expect(result.values.get('SearchChildrenForName')).toEqual(['1']);
        expect(result.values.get('SearchParentsForTags')).toEqual(['1']);
        expect(result.values.get('SearchChildrenForTags')).toEqual(['1']);
    });

    test('absorbs legacy hierarchy URL switches into an existing MRQL filter', () => {
        const values = new FormData();
        values.set('SearchParentsForName', '1');
        values.set('SearchChildrenForName', '1');
        values.set('SearchParentsForTags', '1');
        values.set('SearchChildrenForTags', '1');

        expect(expandGroupMRQLFromParams(
            'name ~ "*alp*" AND tags = "vacation" AND tags = "favorite" AND tags = "work"',
            values,
        )).toBe(
            '(name ~ "*alp*" OR parent.name ~ "*alp*" OR children.name ~ "*alp*") AND ' +
            '(tags = "vacation" OR parent.tags = "vacation" OR children.tags = "vacation") AND ' +
            '(tags = "favorite" OR parent.tags = "favorite" OR children.tags = "favorite") AND ' +
            '(tags = "work" OR parent.tags = "work" OR children.tags = "work")',
        );
    });

    test('rejects richer MRQL instead of partially changing the form', () => {
        expect(mrqlToFormValues('resource', 'name = "exact"').compatible).toBe(false);
        expect(mrqlToFormValues('resource', 'name ~ "*a*" OR tags = "x"').compatible).toBe(false);
        expect(mrqlToFormValues('resource', 'descendants.category = "Archive"').compatible).toBe(false);
    });

    test('round-trips the untagged resource toggle', () => {
        const values = new FormData();
        values.set('Untagged', '1');
        const query = formValuesToMRQL('resource', values);
        expect(query).toBe('tags IS EMPTY');
        expect(mrqlToFormValues('resource', query).values.get('Untagged')).toEqual(['1']);
    });
});
