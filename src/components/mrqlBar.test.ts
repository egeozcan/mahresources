import { describe, expect, test } from 'vitest';
import {
    expandGroupMRQLFromParams,
    expandResourceMRQLFromParams,
    formValuesToMRQL,
    mrqlToFormValues,
} from './mrqlBar.js';

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

    test('represents every resource toggle in MRQL', () => {
        const values = new FormData();
        values.set('ownerId', '4');
        values.set('IncludeSubgroups', '1');
        values.set('ShowWithSimilar', '1');
        values.set('Untagged', '1');

        expect(formValuesToMRQL('resource', values, new Map([['ownerId', ['Alpine Trip']]]))).toBe(
            '(owner = "Alpine Trip" OR ancestors.name = "Alpine Trip") AND ' +
            'tags IS EMPTY AND similarImages IS NOT EMPTY',
        );
    });

    test('translates every resource toggle back to the form', () => {
        const result = mrqlToFormValues(
            'resource',
            '(owner = "Alpine Trip" OR ancestors.name = "Alpine Trip") AND ' +
            'tags IS EMPTY AND similarImages IS NOT EMPTY',
        );
        expect(result.compatible).toBe(true);
        expect(result.values.get('ownerId')).toEqual(['Alpine Trip']);
        expect(result.values.get('IncludeSubgroups')).toEqual(['1']);
        expect(result.values.get('Untagged')).toEqual(['1']);
        expect(result.values.get('ShowWithSimilar')).toEqual(['1']);
    });

    test('absorbs a legacy IncludeSubgroups URL switch into owner MRQL', () => {
        const values = new FormData();
        values.set('IncludeSubgroups', '1');
        expect(expandResourceMRQLFromParams('name ~ "*lake*" AND owner = "Alpine Trip"', values)).toBe(
            'name ~ "*lake*" AND (owner = "Alpine Trip" OR ancestors.name = "Alpine Trip")',
        );
    });

    test('reflects standard and metadata sorts in MRQL', () => {
        const values = new FormData();
        values.set('Name', 'lake');
        values.append('SortBy', 'created_at desc');
        values.append('SortBy', "meta->>'rating' asc");
        expect(formValuesToMRQL('resource', values)).toBe(
            'name ~ "*lake*" ORDER BY created DESC, meta.rating ASC',
        );
    });

    test('translates MRQL ordering back to list sort values', () => {
        const result = mrqlToFormValues(
            'resource', 'name ~ "*lake*" ORDER BY fileSize DESC, updated ASC',
        );
        expect(result.compatible).toBe(true);
        expect(result.values.get('SortBy')).toEqual(['file_size desc', 'updated_at asc']);
        expect(result.values.get('Name')).toEqual(['lake']);
    });

    test('supports a sort-only MRQL representation', () => {
        const result = mrqlToFormValues('resource', 'ORDER BY created DESC');
        expect(result.compatible).toBe(true);
        expect(result.values.get('SortBy')).toEqual(['created_at desc']);
    });

    test('reflects free-field and schema metadata in canonical MRQL', () => {
        const values = new FormData();
        values.append('MetaQuery.0', 'keo:EQ:"meo"');
        values.append('MetaQuery.1', 'rating:GE:4');
        values.append('MetaQuery', 'archived:EQ:true');
        values.append('MetaQuery', 'caption:NL:"draft"');

        expect(formValuesToMRQL('group', values)).toBe(
            'meta.keo = "meo" AND meta.rating >= 4 AND ' +
            'meta.archived = true AND meta.caption !~ "*draft*"',
        );
    });

    test('preserves same-key metadata EQ values as an OR group', () => {
        const values = new FormData();
        values.append('MetaQuery', 'status:EQ:"new"');
        values.append('MetaQuery', 'status:EQ:"done"');

        const query = formValuesToMRQL('resource', values);
        expect(query).toBe('(meta.status = "new" OR meta.status = "done")');
        expect(mrqlToFormValues('resource', query).metadata).toEqual([
            { name: 'status', operation: 'EQ', value: 'new' },
            { name: 'status', operation: 'EQ', value: 'done' },
        ]);
    });

    test('translates MRQL metadata predicates back to form rows', () => {
        const result = mrqlToFormValues(
            'resource',
            'meta.keo = "meo" AND meta.rating >= 4 AND meta.caption !~ "*draft*" AND meta.archived = true',
        );
        expect(result.compatible).toBe(true);
        expect(result.metadata).toEqual([
            { name: 'keo', operation: 'EQ', value: 'meo' },
            { name: 'rating', operation: 'GE', value: 4 },
            { name: 'caption', operation: 'NL', value: 'draft' },
            { name: 'archived', operation: 'EQ', value: true },
        ]);
    });

    test('round-trips parent and child group metadata scopes', () => {
        const values = new FormData();
        values.append('MetaQuery.0', 'parent.region:EQ:"eu"');
        values.append('MetaQuery.1', 'child.priority:GT:2');
        const query = formValuesToMRQL('group', values);
        expect(query).toBe('parent.meta.region = "eu" AND children.meta.priority > 2');
        expect(mrqlToFormValues('group', query).metadata).toEqual([
            { name: 'parent.region', operation: 'EQ', value: 'eu' },
            { name: 'child.priority', operation: 'GT', value: 2 },
        ]);
    });

    test('round-trips null metadata values', () => {
        const values = new FormData();
        values.append('MetaQuery', 'reviewed:EQ:null');
        values.append('MetaQuery', 'deleted:NE:null');
        const query = formValuesToMRQL('note', values);
        expect(query).toBe('meta.reviewed IS NULL AND meta.deleted IS NOT NULL');
        expect(mrqlToFormValues('note', query).metadata).toEqual([
            { name: 'reviewed', operation: 'EQ', value: null },
            { name: 'deleted', operation: 'NE', value: null },
        ]);
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
